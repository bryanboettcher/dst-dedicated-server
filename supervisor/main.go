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

	// Parse shard config (server.ini) for is_master and server_port
	shardRoot := os.Getenv("SHARD_ROOT")
	shardCfg := ParseShardConfig(shardRoot + "/server.ini")

	// Allow env override for game port
	gamePort := shardCfg.Port
	if p := os.Getenv("DST_GAME_PORT"); p != "" {
		gamePort = p
	}

	slog.Info("shard config", "is_master", shardCfg.IsMaster, "game_port", gamePort)

	// Build the supervisor
	sup := &Supervisor{
		ClusterName:     clusterName,
		ShardName:       shardName,
		IsMaster:        shardCfg.IsMaster,
		AdminToken:      os.Getenv("DST_ADMIN_TOKEN"),
		env:             env,
		shutdownTimeout: *shutdownTimeout,
	}
	sup.Health = NewHealthChecker(gamePort, &sup.State)

	// Start HTTP server immediately so probes work during install
	go StartHTTP(*httpAddr, sup)

	// Phase: prepare
	sup.State.Set(StatePreparing)
	if err := RunScript("/usr/local/bin/prepare.sh", false, env); err != nil {
		slog.Error("prepare failed", "error", err)
		os.Exit(1)
	}

	// Phase: install
	sup.State.Set(StateInstalling)
	if err := RunScript("/usr/local/bin/install.sh", true, env); err != nil {
		slog.Error("install failed", "error", err)
		os.Exit(1)
	}

	// Phase: start DST
	sup.State.Set(StateStarting)
	dst, err := StartDST(env)
	if err != nil {
		slog.Error("failed to start DST", "error", err)
		os.Exit(1)
	}

	sup.SetProcess(dst)
	sup.ServerStart = time.Now()

	// Start A2S health checking — transitions Starting → Running on first response
	go sup.Health.Run()
	slog.Info("DST server launched, waiting for A2S readiness")

	// Wait for either a signal or the DST process to exit
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-sigCh:
		slog.Info("received signal, initiating graceful shutdown", "signal", sig)
		sup.GracefulShutdownFromSignal()

	case err := <-sup.WaitForExit():
		if err != nil {
			slog.Error("DST server exited with error", "error", err)
			sup.State.Set(StateStopped)
			os.Exit(1)
		}
		slog.Info("DST server exited cleanly")
		sup.State.Set(StateStopped)
	}
}

func setDefault(key, value string) {
	if os.Getenv(key) == "" {
		os.Setenv(key, value)
	}
}
