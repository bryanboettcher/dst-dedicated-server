package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// DSTProcess manages the DST dedicated server as a child process.
type DSTProcess struct {
	cmd       *exec.Cmd
	stdinPipe io.WriteCloser
	done      chan error // closed (with exit error or nil) when the process exits
}

// getCredential returns the syscall.Credential for the dst user,
// honoring PUID/PGID overrides.
func getCredential() (*syscall.Credential, error) {
	uid := uint32(1000)
	gid := uint32(1000)

	if u, err := exec.Command("id", "-u", "dst").Output(); err == nil {
		if v, err := strconv.ParseUint(string(u[:len(u)-1]), 10, 32); err == nil {
			uid = uint32(v)
		}
	}
	if g, err := exec.Command("id", "-g", "dst").Output(); err == nil {
		if v, err := strconv.ParseUint(string(g[:len(g)-1]), 10, 32); err == nil {
			gid = uint32(v)
		}
	}

	return &syscall.Credential{Uid: uid, Gid: gid}, nil
}

// RunScript executes a shell script, optionally as the dst user.
func RunScript(path string, asUser bool, env []string) error {
	cmd := exec.Command("/bin/bash", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	if asUser {
		cred, err := getCredential()
		if err != nil {
			return fmt.Errorf("get credential: %w", err)
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: cred,
		}
	}

	slog.Info("running script", "path", path, "as_user", asUser)
	return cmd.Run()
}

// StartDST launches the DST dedicated server binary and returns a handle
// with a stdin pipe for console commands. If logs is non-nil, stdout/stderr
// are each drained by independent goroutines that write to os.Stdout/os.Stderr
// first (source of truth for kubectl logs), then to the LogBuffer second.
// DST writes to OS pipes and is never blocked by our log processing.
func StartDST(env []string, logs *LogBuffer) (*DSTProcess, error) {
	installRoot := os.Getenv("INSTALL_ROOT")
	configPath := os.Getenv("CONFIG_PATH")
	clusterName := os.Getenv("CLUSTER_NAME")
	shardName := os.Getenv("SHARD_NAME")

	binary := installRoot + "/bin64/dontstarve_dedicated_server_nullrenderer_x64"

	cmd := exec.Command(binary,
		"-persistent_storage_root", configPath,
		"-conf_dir", "",
		"-cluster", clusterName,
		"-shard", shardName,
		"-ugc_directory", installRoot+"/ugc_mods",
	)
	cmd.Dir = installRoot + "/bin64"
	cmd.Env = env

	cred, err := getCredential()
	if err != nil {
		return nil, fmt.Errorf("get credential: %w", err)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: cred,
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}

	if logs != nil {
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			return nil, fmt.Errorf("create stdout pipe: %w", err)
		}
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			return nil, fmt.Errorf("create stderr pipe: %w", err)
		}

		// Each goroutine drains its pipe independently.
		// os.Stdout/os.Stderr is written first — it is the source of truth.
		// LogBuffer write is second — if it's slow, only the goroutine stalls,
		// not DST. The OS pipe buffer (64KB) absorbs bursts.
		go drainPipe(stdoutPipe, os.Stdout, logs.PrefixWriter("[stdout] "))
		go drainPipe(stderrPipe, os.Stderr, logs.PrefixWriter("[stderr] "))
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	slog.Info("starting DST server",
		"binary", binary,
		"cluster", clusterName,
		"shard", shardName,
	)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start DST: %w", err)
	}

	p := &DSTProcess{
		cmd:       cmd,
		stdinPipe: stdinPipe,
		done:      make(chan error, 1),
	}

	go func() {
		p.done <- cmd.Wait()
		close(p.done)
	}()

	return p, nil
}

// drainPipe reads from src and writes to primary first, then secondary.
// primary is os.Stdout or os.Stderr (source of truth, must not be blocked).
// secondary is the LogBuffer's PrefixWriter (best-effort).
func drainPipe(src io.Reader, primary io.Writer, secondary io.Writer) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			primary.Write(buf[:n])
			secondary.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// SendCommand writes a console command to the DST server's stdin.
func (p *DSTProcess) SendCommand(cmd string) error {
	_, err := fmt.Fprintln(p.stdinPipe, cmd)
	return err
}

// Signal sends an OS signal to the DST process.
func (p *DSTProcess) Signal(sig os.Signal) error {
	if p.cmd.Process == nil {
		return fmt.Errorf("process not started")
	}
	return p.cmd.Process.Signal(sig)
}

// Wait returns the channel that receives the process exit result.
func (p *DSTProcess) Wait() <-chan error {
	return p.done
}

// ReapZombies starts a goroutine that continuously reaps orphaned child processes.
// Required when running as PID 1.
func ReapZombies() {
	go func() {
		for {
			var status syscall.WaitStatus
			pid, err := syscall.Wait4(-1, &status, 0, nil)
			if err != nil {
				if err == syscall.ECHILD {
					syscall.Wait4(-1, &status, 0, nil)
				}
				_ = pid
			}
		}
	}()
}
