package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type statusResponse struct {
	State      string `json:"state"`
	Uptime     string `json:"uptime,omitempty"`
	ServerName string `json:"server_name,omitempty"`
	Map        string `json:"map,omitempty"`
	Players    *int   `json:"players,omitempty"`
	MaxPlayers *int   `json:"max_players,omitempty"`
	Cluster    string `json:"cluster"`
	Shard      string `json:"shard"`
}

// StartHTTP starts the health/management HTTP server.
// It blocks, so call it in a goroutine.
func StartHTTP(addr string, state *StateManager, startTime *time.Time, clusterName, shardName string, health *HealthChecker) {
	mux := http.NewServeMux()

	// Liveness: is the supervisor process alive?
	// Returns 200 as long as this process is running.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Readiness: is the DST server accepting connections?
	// Gated on actual A2S probe response.
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if state.Get() == StateRunning && health.Healthy() {
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
			State:   state.Get().String(),
			Cluster: clusterName,
			Shard:   shardName,
		}
		if startTime != nil && !startTime.IsZero() {
			resp.Uptime = time.Since(*startTime).Truncate(time.Second).String()
		}
		if info := health.Info(); info != nil {
			resp.ServerName = info.Name
			resp.Map = info.Map
			players := int(info.Players)
			maxPlayers := int(info.MaxPlayers)
			resp.Players = &players
			resp.MaxPlayers = &maxPlayers
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// Prometheus metrics in text exposition format.
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

		// Uptime in seconds
		uptime := 0.0
		if startTime != nil && !startTime.IsZero() && s >= StateStarting {
			uptime = time.Since(*startTime).Seconds()
		}
		fmt.Fprintf(w, "dst_server_uptime_seconds %.1f\n", uptime)

		// Player counts from A2S
		if info := health.Info(); info != nil {
			fmt.Fprintf(w, "dst_server_players %d\n", info.Players)
			fmt.Fprintf(w, "dst_server_max_players %d\n", info.MaxPlayers)
		}

		// Server info gauge (always 1, carries labels for Prometheus service discovery)
		fmt.Fprintf(w, "dst_server_info{cluster=%q,shard=%q} 1\n", clusterName, shardName)
	})

	slog.Info("starting HTTP server", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("HTTP server failed", "error", err)
	}
}
