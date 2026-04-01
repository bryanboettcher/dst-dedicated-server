package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type mockLogBuffer struct {
	mu    sync.RWMutex
	lines []string
	subMu sync.RWMutex
	subs  map[chan string]struct{}
}

func newMockLogBuffer() *mockLogBuffer {
	return &mockLogBuffer{subs: make(map[chan string]struct{})}
}

func (lb *mockLogBuffer) add(line string) {
	lb.mu.Lock()
	lb.lines = append(lb.lines, line)
	if len(lb.lines) > 500 {
		lb.lines = lb.lines[len(lb.lines)-500:]
	}
	lb.mu.Unlock()
	lb.subMu.RLock()
	for ch := range lb.subs {
		select {
		case ch <- line:
		default:
		}
	}
	lb.subMu.RUnlock()
}

func (lb *mockLogBuffer) last(n int) []string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	if n > len(lb.lines) {
		n = len(lb.lines)
	}
	return append([]string{}, lb.lines[len(lb.lines)-n:]...)
}

func (lb *mockLogBuffer) subscribe() chan string {
	ch := make(chan string, 64)
	lb.subMu.Lock()
	lb.subs[ch] = struct{}{}
	lb.subMu.Unlock()
	return ch
}

func (lb *mockLogBuffer) unsubscribe(ch chan string) {
	lb.subMu.Lock()
	delete(lb.subs, ch)
	lb.subMu.Unlock()
	close(ch)
}

type shardState struct {
	mu         sync.RWMutex
	State      string
	Name       string
	Map        string
	Players    int
	MaxPlayers int
	Cluster    string
	Shard      string
	IsMaster   bool
	StartTime  time.Time
	logs       *mockLogBuffer
}

func (s *shardState) status() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := map[string]any{
		"state":       s.State,
		"cluster":     s.Cluster,
		"shard":       s.Shard,
		"server_name": s.Name,
		"map":         s.Map,
		"players":     s.Players,
		"max_players": s.MaxPlayers,
		"is_master":   s.IsMaster,
	}
	if !s.StartTime.IsZero() {
		m["uptime"] = time.Since(s.StartTime).Truncate(time.Second).String()
	}
	return m
}

func main() {
	shards := map[string]*shardState{
		"Overworld": {
			State:      "running",
			Name:       "Wilson's Wilderness",
			Map:        "forest",
			Players:    3,
			MaxPlayers: 16,
			Cluster:    "DST_Cluster",
			Shard:      "Overworld",
			IsMaster:   true,
			StartTime:  time.Now().Add(-2 * time.Hour),
			logs:       newMockLogBuffer(),
		},
		"Caves": {
			State:      "running",
			Name:       "Wilson's Wilderness",
			Map:        "cave",
			Players:    1,
			MaxPlayers: 16,
			Cluster:    "DST_Cluster",
			Shard:      "Caves",
			IsMaster:   false,
			StartTime:  time.Now().Add(-2 * time.Hour),
			logs:       newMockLogBuffer(),
		},
	}

	// Drift player counts
	go func() {
		for range time.Tick(8 * time.Second) {
			for _, s := range shards {
				s.mu.Lock()
				if s.State == "running" {
					delta := rand.Intn(3) - 1
					s.Players = max(0, min(s.MaxPlayers, s.Players+delta))
				}
				s.mu.Unlock()
			}
		}
	}()

	// Generate fake log lines
	mockMessages := []string{
		"[Shard] Secondary shard is now connected",
		"[Workshop] ModIndex: Load sequence complete",
		"Sim paused",
		"Sim unpaused",
		"[Steam] AuthenticateUserTicket success",
		"New incoming connection from |KU_abc123",
		"[Join Announcement] Wilson joined the server",
		"[Leave Announcement] Willow left the server",
		"Serializing world: session/save",
		"[200] Account Communication Success",
		"[Autosave] Saving world...",
		"[Autosave] Save complete.",
		"QueryServerComplete noridge",
		"CURL ERROR: operation timedout (retrying)",
	}
	go func() {
		for range time.Tick(3 * time.Second) {
			for _, s := range shards {
				s.mu.RLock()
				running := s.State == "running"
				s.mu.RUnlock()
				if running {
					msg := mockMessages[rand.Intn(len(mockMessages))]
					s.logs.add(fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg))
				}
			}
		}
	}()

	mux := http.NewServeMux()

	// Each shard gets its own set of endpoints under its "port" path.
	// The webui mock serves all shards on one port, each under /shard/{name}/...
	// BUT it also serves them at the root for direct single-shard compat.
	// For the multi-shard mock, we register per-shard routes.
	for name, state := range shards {
		registerShardRoutes(mux, "/"+name, state)
	}

	// Also register a flat set for the first shard (backward compat)
	registerShardRoutes(mux, "", shards["Overworld"])

	addr := ":8081"
	if v := os.Getenv("MOCK_LISTEN"); v != "" {
		addr = v
	}
	slog.Info("mock supervisor starting", "addr", addr, "shards", len(shards))
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("failed to start", "error", err)
		os.Exit(1)
	}
}

func registerShardRoutes(mux *http.ServeMux, prefix string, state *shardState) {
	mux.HandleFunc("GET "+prefix+"/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET "+prefix+"/readyz", func(w http.ResponseWriter, r *http.Request) {
		state.mu.RLock()
		s := state.State
		state.mu.RUnlock()
		if s == "running" {
			w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(s))
	})

	mux.HandleFunc("GET "+prefix+"/startupz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET "+prefix+"/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(state.status())
	})

	mux.HandleFunc("GET "+prefix+"/metrics", func(w http.ResponseWriter, r *http.Request) {
		state.mu.RLock()
		defer state.mu.RUnlock()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprintf(w, "dst_server_players %d\n", state.Players)
		fmt.Fprintf(w, "dst_server_max_players %d\n", state.MaxPlayers)
	})

	mux.HandleFunc("GET "+prefix+"/api/logs", func(w http.ResponseWriter, r *http.Request) {
		n := 100
		if v := r.URL.Query().Get("lines"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
				n = parsed
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(state.logs.last(n))
	})

	mux.HandleFunc("GET "+prefix+"/api/logs/stream", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch := state.logs.subscribe()
		defer state.logs.unsubscribe(ch)

		for {
			select {
			case <-r.Context().Done():
				return
			case line, ok := <-ch:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", line)
				flusher.Flush()
			}
		}
	})

	mux.HandleFunc("POST "+prefix+"/api/save", func(w http.ResponseWriter, r *http.Request) {
		slog.Info("mock: c_save()", "shard", state.Shard)
		writeAPI(w, true, "save triggered")
	})

	mux.HandleFunc("POST "+prefix+"/api/shutdown", func(w http.ResponseWriter, r *http.Request) {
		slog.Info("mock: shutdown", "shard", state.Shard)
		writeAPI(w, true, "shutdown initiated")
		go func() {
			state.mu.Lock()
			state.State = "stopping"
			state.mu.Unlock()
			time.Sleep(3 * time.Second)
			state.mu.Lock()
			state.State = "stopped"
			state.Players = 0
			state.mu.Unlock()
		}()
	})

	mux.HandleFunc("POST "+prefix+"/api/restart", func(w http.ResponseWriter, r *http.Request) {
		slog.Info("mock: restart", "shard", state.Shard)
		writeAPI(w, true, "restart initiated")
		go func() {
			state.mu.Lock()
			state.State = "stopping"
			state.mu.Unlock()
			time.Sleep(2 * time.Second)
			state.mu.Lock()
			state.State = "starting"
			state.Players = 0
			state.mu.Unlock()
			time.Sleep(3 * time.Second)
			state.mu.Lock()
			state.State = "running"
			state.StartTime = time.Now()
			state.mu.Unlock()
		}()
	})

	mux.HandleFunc("POST "+prefix+"/api/rollback/{days...}", func(w http.ResponseWriter, r *http.Request) {
		daysStr := r.PathValue("days")
		msg := "rollback triggered"
		if daysStr != "" {
			if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
				msg = fmt.Sprintf("rollback triggered (%d days)", d)
			}
		}
		slog.Info("mock: rollback", "shard", state.Shard, "days", daysStr)
		writeAPI(w, true, msg)
	})

	mux.HandleFunc("POST "+prefix+"/api/console", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		cmd := strings.TrimSpace(string(body))
		slog.Info("mock: console", "shard", state.Shard, "cmd", cmd)
		writeAPI(w, true, "command sent")
	})
}

func writeAPI(w http.ResponseWriter, ok bool, msg string) {
	w.Header().Set("Content-Type", "application/json")
	if ok {
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": msg})
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": msg})
	}
}
