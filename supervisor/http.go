package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type statusResponse struct {
	State  string `json:"state"`
	Uptime string `json:"uptime,omitempty"`
}

// StartHTTP starts the health/management HTTP server.
// It blocks, so call it in a goroutine.
func StartHTTP(addr string, state *StateManager, startTime *time.Time, clusterName, shardName string) {
	mux := http.NewServeMux()

	// Liveness: is the supervisor process alive?
	// Returns 200 as long as this process is running.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Readiness: is the DST server accepting connections?
	// Phase 1: considers RUNNING state sufficient.
	// Phase 2 will add real A2S probe.
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if state.Get() == StateRunning {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(state.Get().String()))
	})

	// Startup: has the DST binary been launched?
	// Returns 200 once we're past the install phase.
	mux.HandleFunc("GET /startupz", func(w http.ResponseWriter, r *http.Request) {
		s := state.Get()
		if s >= StateStarting && s <= StateRunning {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(s.String()))
	})

	// Status: JSON blob for dashboards (Homarr, etc.)
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		resp := statusResponse{
			State: state.Get().String(),
		}
		if startTime != nil && !startTime.IsZero() {
			resp.Uptime = time.Since(*startTime).Truncate(time.Second).String()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// Prometheus metrics in text exposition format.
	// No client library needed — gauges are just "name value\n".
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		s := state.Get()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		// State as a labeled gauge (1 for current state, 0 for all others)
		for _, candidate := range []ServerState{
			StatePreparing, StateInstalling, StateStarting,
			StateRunning, StateStopping, StateStopped,
		} {
			val := 0
			if candidate == s {
				val = 1
			}
			fmt.Fprintf(w, "dst_server_state{state=%q} %d\n", candidate.String(), val)
		}

		// Uptime in seconds (0 if server hasn't started yet)
		uptime := 0.0
		if startTime != nil && !startTime.IsZero() && s >= StateStarting {
			uptime = time.Since(*startTime).Seconds()
		}
		fmt.Fprintf(w, "dst_server_uptime_seconds %.1f\n", uptime)

		// Server info gauge (always 1, carries labels for Prometheus service discovery)
		fmt.Fprintf(w, "dst_server_info{cluster=%q,shard=%q} 1\n", clusterName, shardName)
	})

	slog.Info("starting HTTP server", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("HTTP server failed", "error", err)
	}
}
