package main

import (
	"embed"
	"flag"
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

func main() {
	listenAddr := flag.String("listen", ":3000", "HTTP listen address for the web UI")
	backendURL := flag.String("backend", "http://localhost:8080", "Supervisor backend URL")
	flag.Parse()

	if v := os.Getenv("DST_WEBUI_LISTEN"); v != "" {
		*listenAddr = v
	}
	if v := os.Getenv("DST_BACKEND_URL"); v != "" {
		*backendURL = v
	}

	backend, err := url.Parse(*backendURL)
	if err != nil {
		slog.Error("invalid backend URL", "url", *backendURL, "error", err)
		os.Exit(1)
	}

	proxy := httputil.NewSingleHostReverseProxy(backend)

	mux := http.NewServeMux()

	// Proxy API and health endpoints to supervisor
	for _, prefix := range []string{"/api/", "/healthz", "/readyz", "/startupz", "/status", "/metrics"} {
		mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
			proxy.ServeHTTP(w, r)
		})
	}

	// SSE: stream /status updates to the SPA every 5 seconds
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Send immediately, then every 5s
		send := func() {
			resp, err := http.Get(*backendURL + "/status")
			if err != nil {
				io.WriteString(w, "event: error\ndata: "+err.Error()+"\n\n")
				flusher.Flush()
				return
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			io.WriteString(w, "data: "+string(body)+"\n\n")
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
		// SPA fallback
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	slog.Info("dst-webui starting", "listen", *listenAddr, "backend", *backendURL)
	if err := http.ListenAndServe(*listenAddr, mux); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
