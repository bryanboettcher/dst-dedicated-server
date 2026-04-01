package main

import (
	"log/slog"
	"sync"
	"time"
)

// HealthChecker periodically probes the DST server via A2S and updates
// the state machine and cached server info.
type HealthChecker struct {
	addr     string
	state    *StateManager
	interval time.Duration
	timeout  time.Duration

	mu       sync.RWMutex
	lastInfo *A2SInfo
	lastErr  error
	lastPoll time.Time
}

func NewHealthChecker(gamePort string, state *StateManager) *HealthChecker {
	return &HealthChecker{
		addr:     "127.0.0.1:" + gamePort,
		state:    state,
		interval: 10 * time.Second,
		timeout:  3 * time.Second,
	}
}

// Run starts the probe loop. It blocks, so call it in a goroutine.
// It only probes while the server is in Starting or Running state.
func (hc *HealthChecker) Run() {
	ticker := time.NewTicker(hc.interval)
	defer ticker.Stop()

	for range ticker.C {
		s := hc.state.Get()
		if s != StateStarting && s != StateRunning {
			continue
		}

		info, err := QueryA2SInfo(hc.addr, hc.timeout)
		hc.mu.Lock()
		hc.lastPoll = time.Now()
		hc.lastInfo = info
		hc.lastErr = err
		hc.mu.Unlock()

		if err != nil {
			slog.Debug("A2S probe failed", "error", err)
			// Don't downgrade from Running to Starting — transient UDP loss is normal.
			// Kubernetes liveness probe handles real crashes.
			continue
		}

		if s == StateStarting {
			hc.state.Set(StateRunning)
			slog.Info("DST server is ready (A2S responding)",
				"name", info.Name,
				"players", info.Players,
				"max_players", info.MaxPlayers,
			)
		}
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
