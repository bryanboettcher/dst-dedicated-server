package main

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"syscall"
	"time"
)

// Supervisor holds all shared state for the DST server lifecycle.
type Supervisor struct {
	State       StateManager
	Health      *HealthChecker
	Logs        *LogBuffer
	ServerStart time.Time
	ClusterName string
	ShardName   string
	IsMaster    bool
	AdminToken  string

	mu              sync.Mutex
	dst             *DSTProcess
	env             []string
	shutdownTimeout time.Duration
}

// SetProcess sets the managed DST process (called after launch).
func (s *Supervisor) SetProcess(p *DSTProcess) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dst = p
}

// SendCommand sends a console command to the DST server.
func (s *Supervisor) SendCommand(cmd string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dst == nil {
		return fmt.Errorf("server not running")
	}
	return s.dst.SendCommand(cmd)
}

// Save triggers c_save() on the DST server.
func (s *Supervisor) Save() error {
	slog.Info("sending c_save()")
	return s.SendCommand("c_save()")
}

// Rollback triggers c_rollback() with an optional number of days.
func (s *Supervisor) Rollback(days int) error {
	if days > 0 {
		slog.Info("sending c_rollback()", "days", days)
		return s.SendCommand(fmt.Sprintf("c_rollback(%d)", days))
	}
	slog.Info("sending c_rollback()")
	return s.SendCommand("c_rollback()")
}

// Shutdown performs a graceful save + shutdown of the DST server.
func (s *Supervisor) Shutdown() error {
	s.mu.Lock()
	dst := s.dst
	s.mu.Unlock()

	if dst == nil {
		return fmt.Errorf("server not running")
	}

	s.State.Set(StateStopping)

	slog.Info("sending c_save()")
	if err := dst.SendCommand("c_save()"); err != nil {
		slog.Warn("failed to send save command", "error", err)
	}
	time.Sleep(5 * time.Second)

	slog.Info("sending c_shutdown()")
	if err := dst.SendCommand("c_shutdown()"); err != nil {
		slog.Warn("failed to send shutdown command", "error", err)
	}

	select {
	case <-dst.Wait():
		slog.Info("DST server exited after graceful shutdown")
	case <-time.After(s.shutdownTimeout):
		slog.Warn("DST did not exit in time, sending SIGKILL", "timeout", s.shutdownTimeout)
		dst.Signal(syscall.SIGKILL)
		<-dst.Wait()
	}

	s.State.Set(StateStopped)
	return nil
}

// Restart performs a save + stop + relaunch cycle without restarting the container.
func (s *Supervisor) Restart() error {
	s.mu.Lock()
	dst := s.dst
	s.mu.Unlock()

	if dst == nil {
		return fmt.Errorf("server not running")
	}

	s.State.Set(StateStopping)

	// Save and stop
	slog.Info("restart: sending c_save()")
	dst.SendCommand("c_save()")
	time.Sleep(5 * time.Second)

	slog.Info("restart: sending c_shutdown()")
	dst.SendCommand("c_shutdown()")

	select {
	case <-dst.Wait():
		slog.Info("restart: DST server stopped")
	case <-time.After(s.shutdownTimeout):
		slog.Warn("restart: forcing SIGKILL")
		dst.Signal(syscall.SIGKILL)
		<-dst.Wait()
	}

	// Relaunch
	s.State.Set(StateStarting)
	newDst, err := StartDST(s.env, s.Logs)
	if err != nil {
		s.State.Set(StateStopped)
		return fmt.Errorf("restart failed: %w", err)
	}

	s.mu.Lock()
	s.dst = newDst
	s.mu.Unlock()

	s.ServerStart = time.Now()
	slog.Info("restart: DST server relaunched, waiting for A2S readiness")
	return nil
}

// WaitForExit returns the channel for the current DST process exit.
func (s *Supervisor) WaitForExit() <-chan error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dst == nil {
		ch := make(chan error, 1)
		ch <- fmt.Errorf("no process")
		return ch
	}
	return s.dst.Wait()
}

// CheckAuth validates the admin token for management endpoints.
// Returns true if auth is disabled (no token set) or the token matches.
func (s *Supervisor) CheckAuth(r string) bool {
	if s.AdminToken == "" {
		return true
	}
	return r == "Bearer "+s.AdminToken
}

// GracefulShutdownFromSignal handles SIGTERM/SIGINT.
func (s *Supervisor) GracefulShutdownFromSignal() {
	s.Shutdown()
	os.Exit(0)
}
