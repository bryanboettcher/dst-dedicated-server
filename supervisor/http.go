package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
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
	IsMaster   bool   `json:"is_master"`
}

type apiResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// StartHTTP starts the health/management HTTP server.
// It blocks, so call it in a goroutine.
func StartHTTP(addr string, sup *Supervisor) {
	mux := http.NewServeMux()

	// --- Health probes ---

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if sup.State.Get() == StateRunning && sup.Health.Healthy() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(sup.State.Get().String()))
	})

	mux.HandleFunc("GET /startupz", func(w http.ResponseWriter, r *http.Request) {
		s := sup.State.Get()
		if s >= StateStarting && s <= StateRunning {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(s.String()))
	})

	// --- Status ---

	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		resp := statusResponse{
			State:    sup.State.Get().String(),
			Cluster:  sup.ClusterName,
			Shard:    sup.ShardName,
			IsMaster: sup.IsMaster,
		}
		if !sup.ServerStart.IsZero() {
			resp.Uptime = time.Since(sup.ServerStart).Truncate(time.Second).String()
		}
		if info := sup.Health.Info(); info != nil {
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

	// --- Prometheus metrics ---

	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		s := sup.State.Get()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

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

		uptime := 0.0
		if !sup.ServerStart.IsZero() && s >= StateStarting {
			uptime = time.Since(sup.ServerStart).Seconds()
		}
		fmt.Fprintf(w, "dst_server_uptime_seconds %.1f\n", uptime)

		if info := sup.Health.Info(); info != nil {
			fmt.Fprintf(w, "dst_server_players %d\n", info.Players)
			fmt.Fprintf(w, "dst_server_max_players %d\n", info.MaxPlayers)
		}

		isMaster := 0
		if sup.IsMaster {
			isMaster = 1
		}
		fmt.Fprintf(w, "dst_server_info{cluster=%q,shard=%q} 1\n", sup.ClusterName, sup.ShardName)
		fmt.Fprintf(w, "dst_server_is_master %d\n", isMaster)
	})

	// --- Logs ---

	// GET /api/logs?lines=N — returns last N lines (default 100) as JSON array.
	mux.HandleFunc("GET /api/logs", func(w http.ResponseWriter, r *http.Request) {
		n := 100
		if v := r.URL.Query().Get("lines"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
				n = parsed
			}
		}
		lines := sup.Logs.Last(n)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lines)
	})

	// GET /api/logs/stream — SSE stream of new log lines as they arrive.
	mux.HandleFunc("GET /api/logs/stream", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch := sup.Logs.Subscribe()
		defer sup.Logs.Unsubscribe(ch)

		for {
			select {
			case <-r.Context().Done():
				return
			case line, ok := <-ch:
				if !ok {
					return
				}
				io.WriteString(w, "data: "+line+"\n\n")
				flusher.Flush()
			}
		}
	})

	// --- Management API ---

	mux.HandleFunc("POST /api/save", sup.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if err := sup.Save(); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, apiResponse{OK: true, Message: "save triggered"})
	}))

	mux.HandleFunc("POST /api/shutdown", sup.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		// Respond before shutting down so the client gets a response
		writeJSON(w, http.StatusOK, apiResponse{OK: true, Message: "shutdown initiated"})
		go sup.Shutdown()
	}))

	mux.HandleFunc("POST /api/restart", sup.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		go func() {
			if err := sup.Restart(); err != nil {
				slog.Error("restart failed", "error", err)
			}
		}()
		writeJSON(w, http.StatusOK, apiResponse{OK: true, Message: "restart initiated"})
	}))

	mux.HandleFunc("POST /api/rollback/{days...}", sup.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		daysStr := r.PathValue("days")
		days := 0
		if daysStr != "" {
			var err error
			days, err = strconv.Atoi(daysStr)
			if err != nil || days < 0 {
				writeJSON(w, http.StatusBadRequest, apiResponse{Error: "days must be a non-negative integer"})
				return
			}
		}
		if err := sup.Rollback(days); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiResponse{Error: err.Error()})
			return
		}
		msg := "rollback triggered"
		if days > 0 {
			msg = fmt.Sprintf("rollback triggered (%d days)", days)
		}
		writeJSON(w, http.StatusOK, apiResponse{OK: true, Message: msg})
	}))

	mux.HandleFunc("POST /api/console", sup.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Error: "failed to read body"})
			return
		}
		cmd := strings.TrimSpace(string(body))
		if cmd == "" {
			writeJSON(w, http.StatusBadRequest, apiResponse{Error: "empty command"})
			return
		}
		if err := sup.SendCommand(cmd); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, apiResponse{OK: true, Message: "command sent"})
	}))

	slog.Info("starting HTTP server", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("HTTP server failed", "error", err)
	}
}

// requireAuth wraps a handler with bearer token validation.
func (sup *Supervisor) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sup.CheckAuth(r.Header.Get("Authorization")) {
			writeJSON(w, http.StatusUnauthorized, apiResponse{Error: "unauthorized"})
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
