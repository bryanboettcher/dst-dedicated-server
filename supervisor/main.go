package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	httpAddr := flag.String("http", ":8080", "HTTP health/management listen address")
	shutdownTimeout := flag.Duration("shutdown-timeout", 30*time.Second, "max time to wait for DST to exit gracefully")
	flag.Parse()

	slog.Info("dst-supervisor starting")

	// Set up environment (same defaults as entrypoint.sh)
	setDefault("INSTALL_ROOT", "/opt/dst_server")
	setDefault("MODS_PATH", "/dst/mods")
	setDefault("CONFIG_PATH", "/dst/config")
	setDefault("CLUSTER_NAME", "DST_Cluster")
	setDefault("SHARD_NAME", "Overworld")

	configPath := os.Getenv("CONFIG_PATH")
	clusterName := os.Getenv("CLUSTER_NAME")
	shardName := os.Getenv("SHARD_NAME")

	setDefault("CLUSTER_ROOT", configPath+"/"+clusterName)
	setDefault("SHARD_ROOT", os.Getenv("CLUSTER_ROOT")+"/"+shardName)
	setDefault("USER_ROOT", "/home/dst")

	env := os.Environ()

	// Start zombie reaper (we are PID 1)
	ReapZombies()

	// State tracking
	var state StateManager
	var serverStart time.Time

	// Determine game port for A2S probing (default 10999)
	gamePort := "10999"
	if p := os.Getenv("DST_GAME_PORT"); p != "" {
		gamePort = p
	}

	// Health checker for A2S probing
	health := NewHealthChecker(gamePort, &state)

	// Start HTTP server immediately so probes work during install
	go StartHTTP(*httpAddr, &state, &serverStart, clusterName, shardName, health)

	// Phase: prepare
	state.Set(StatePreparing)
	if err := RunScript("/usr/local/bin/prepare.sh", false, env); err != nil {
		slog.Error("prepare failed", "error", err)
		os.Exit(1)
	}

	// Phase: install
	state.Set(StateInstalling)
	if err := RunScript("/usr/local/bin/install.sh", true, env); err != nil {
		slog.Error("install failed", "error", err)
		os.Exit(1)
	}

	// Phase: start DST
	state.Set(StateStarting)
	dst, err := StartDST(env)
	if err != nil {
		slog.Error("failed to start DST", "error", err)
		os.Exit(1)
	}

	serverStart = time.Now()

	// Start A2S health checking — this will transition from Starting → Running
	// once the DST server responds to queries.
	go health.Run()
	slog.Info("DST server launched, waiting for A2S readiness")

	// Wait for either a signal or the DST process to exit
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-sigCh:
		slog.Info("received signal, initiating graceful shutdown", "signal", sig)
		gracefulShutdown(dst, &state, *shutdownTimeout)

	case err := <-dst.Wait():
		if err != nil {
			slog.Error("DST server exited with error", "error", err)
			state.Set(StateStopped)
			os.Exit(1)
		}
		slog.Info("DST server exited cleanly")
		state.Set(StateStopped)
	}
}

func gracefulShutdown(dst *DSTProcess, state *StateManager, timeout time.Duration) {
	state.Set(StateStopping)

	// Send save command
	slog.Info("sending c_save()")
	if err := dst.SendCommand("c_save()"); err != nil {
		slog.Warn("failed to send save command", "error", err)
	}

	// Give save a moment to flush
	time.Sleep(5 * time.Second)

	// Send shutdown command
	slog.Info("sending c_shutdown()")
	if err := dst.SendCommand("c_shutdown()"); err != nil {
		slog.Warn("failed to send shutdown command", "error", err)
	}

	// Wait for exit or timeout
	select {
	case <-dst.Wait():
		slog.Info("DST server exited after graceful shutdown")
	case <-time.After(timeout):
		slog.Warn("DST did not exit in time, sending SIGKILL", "timeout", timeout)
		dst.Signal(syscall.SIGKILL)
		<-dst.Wait()
	}

	state.Set(StateStopped)
}

func setDefault(key, value string) {
	if os.Getenv(key) == "" {
		os.Setenv(key, value)
	}
}
