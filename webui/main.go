package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

// shard represents a named DST supervisor backend.
type shard struct {
	Name    string
	URL     *url.URL
	Proxy   *httputil.ReverseProxy
	RawURL  string
}

func main() {
	listenAddr := flag.String("listen", ":3002", "HTTP listen address for the web UI")
	backendsFlag := flag.String("backends", "", "Comma-separated shard backends: name=url,name=url")
	flag.Parse()

	if v := os.Getenv("DST_WEBUI_LISTEN"); v != "" {
		*listenAddr = v
	}

	// Parse backends from flag or env
	backendsStr := *backendsFlag
	if v := os.Getenv("DST_BACKENDS"); v != "" {
		backendsStr = v
	}
	// Fallback: single backend for backward compat
	if backendsStr == "" {
		if v := os.Getenv("DST_BACKEND_URL"); v != "" {
			backendsStr = "default=" + v
		} else {
			backendsStr = "default=http://localhost:8081"
		}
	}

	shards, err := parseBackends(backendsStr)
	if err != nil {
		slog.Error("invalid backends config", "error", err)
		os.Exit(1)
	}

	shardMap := make(map[string]*shard)
	for i := range shards {
		shardMap[shards[i].Name] = &shards[i]
	}

	slog.Info("configured shards", "count", len(shards))
	for _, s := range shards {
		slog.Info("  shard", "name", s.Name, "url", s.RawURL)
	}

	mux := http.NewServeMux()

	// GET /shards — returns list of configured shard names
	mux.HandleFunc("GET /shards", func(w http.ResponseWriter, r *http.Request) {
		names := make([]string, len(shards))
		for i, s := range shards {
			names[i] = s.Name
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(names)
	})

	// Per-shard proxy: /shard/{name}/api/... → backend
	mux.HandleFunc("/shard/{name}/{path...}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		s, ok := shardMap[name]
		if !ok {
			http.Error(w, "unknown shard: "+name, http.StatusNotFound)
			return
		}
		// Rewrite path: strip /shard/{name} prefix
		r.URL.Path = "/" + r.PathValue("path")
		s.Proxy.ServeHTTP(w, r)
	})

	// SSE: stream aggregated status from all shards
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		send := func() {
			result := make(map[string]json.RawMessage)
			for _, s := range shards {
				resp, err := http.Get(s.RawURL + "/status")
				if err != nil {
					errJSON, _ := json.Marshal(map[string]string{
						"state": "unreachable",
						"error": err.Error(),
					})
					result[s.Name] = errJSON
					continue
				}
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				result[s.Name] = body
			}
			data, _ := json.Marshal(result)
			io.WriteString(w, "data: "+string(data)+"\n\n")
			flusher.Flush()
		}

		send()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				send()
			}
		}
	})

	// Serve embedded static files with SPA fallback
	staticFS, _ := fs.Sub(staticFiles, "static")
	fileServer := http.FileServer(http.FS(staticFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if f, err := staticFS.Open(path); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	slog.Info("dst-webui starting", "listen", *listenAddr)
	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// parseBackends parses "name=url,name=url" into shard configs.
func parseBackends(s string) ([]shard, error) {
	var shards []shard
	for _, entry := range strings.Split(s, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid backend entry (expected name=url): %q", entry)
		}
		name := strings.TrimSpace(parts[0])
		rawURL := strings.TrimSpace(parts[1])
		u, err := url.Parse(rawURL)
		if err != nil {
			return nil, fmt.Errorf("invalid URL for %q: %w", name, err)
		}
		shards = append(shards, shard{
			Name:   name,
			URL:    u,
			Proxy:  httputil.NewSingleHostReverseProxy(u),
			RawURL: rawURL,
		})
	}
	if len(shards) == 0 {
		return nil, fmt.Errorf("no backends configured")
	}
	return shards, nil
}
