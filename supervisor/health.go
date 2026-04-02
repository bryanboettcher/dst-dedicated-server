package main

import (
	"log/slog"
	"sync"
	"time"
)

// HealthChecker periodically probes the DST server via A2S and updates
// the state machine and cached server info. Waits for the query port to
// be discovered by the Observer before starting to probe.
type HealthChecker struct {
	state    *StateManager
	interval time.Duration
	timeout  time.Duration

	portCh chan string // receives the query port from the observer

	mu       sync.RWMutex
	addr     string
	lastInfo *A2SInfo
	lastErr  error
	lastPoll time.Time
}

func NewHealthChecker(state *StateManager, interval, timeout time.Duration) *HealthChecker {
	return &HealthChecker{
		state:    state,
		interval: interval,
		timeout:  timeout,
		portCh:   make(chan string, 1),
	}
}

// SetQueryPort is called by the Observer when it discovers the
// SteamMasterServerPort from DST's stdout.
func (hc *HealthChecker) SetQueryPort(port string) {
	// Non-blocking send — if a port was already set (restart case),
	// drain and replace.
	select {
	case <-hc.portCh:
	default:
	}
	hc.portCh <- port
}

// Run starts the probe loop. It blocks until the query port is discovered,
// then probes periodically. Call in a goroutine.
func (hc *HealthChecker) Run() {
	// Wait for the observer to discover the query port
	port := <-hc.portCh
	addr := "127.0.0.1:" + port

	hc.mu.Lock()
	hc.addr = addr
	hc.mu.Unlock()

	slog.Info("health checker starting", "addr", addr)

	ticker := time.NewTicker(hc.interval)
	defer ticker.Stop()

	// Probe immediately, then on tick
	hc.probe(addr)
	for {
		select {
		case newPort := <-hc.portCh:
			// Port changed (restart). Update and probe immediately.
			addr = "127.0.0.1:" + newPort
			hc.mu.Lock()
			hc.addr = addr
			hc.mu.Unlock()
			slog.Info("health checker port updated", "addr", addr)
			hc.probe(addr)
		case <-ticker.C:
			hc.probe(addr)
		}
	}
}

func (hc *HealthChecker) probe(addr string) {
	s := hc.state.Get()
	if s != StateStarting && s != StateRunning {
		return
	}

	info, err := QueryA2SInfo(addr, hc.timeout)
	hc.mu.Lock()
	hc.lastPoll = time.Now()
	hc.lastInfo = info
	hc.lastErr = err
	hc.mu.Unlock()

	if err != nil {
		slog.Debug("A2S probe failed", "addr", addr, "error", err)
		return
	}

	if s == StateStarting {
		hc.state.Set(StateRunning)
		slog.Info("DST server is ready (A2S responding)",
			"addr", addr,
			"name", info.Name,
			"players", info.Players,
			"max_players", info.MaxPlayers,
		)
	}
}

// Info returns the last successful A2S query result, or nil.
func (hc *HealthChecker) Info() *A2SInfo {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.lastInfo
}

// Healthy returns true if the last A2S probe succeeded.
func (hc *HealthChecker) Healthy() bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.lastErr == nil && hc.lastInfo != nil
}
