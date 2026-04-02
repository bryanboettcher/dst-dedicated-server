package main

import (
	"sync"
	"time"
)

// Player represents a connected player tracked by their Klei User ID.
type Player struct {
	KleiID   string `json:"klei_id"`
	Name     string `json:"name"`
	LastSeen time.Time `json:"-"`
}

// PlayerTracker maintains a map of online players keyed by Klei User ID.
// Updated by the observer (join/leave events) and periodic c_listplayers() polls.
type PlayerTracker struct {
	mu      sync.RWMutex
	players map[string]*Player
	maxAge  time.Duration
}

func NewPlayerTracker() *PlayerTracker {
	return &PlayerTracker{
		players: make(map[string]*Player),
		maxAge:  60 * time.Second, // age out after missing 2-3 poll cycles
	}
}

// Seen updates or adds a player from a c_listplayers() response or join event.
func (pt *PlayerTracker) Seen(kleiID, name string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if p, ok := pt.players[kleiID]; ok {
		p.Name = name
		p.LastSeen = time.Now()
	} else {
		pt.players[kleiID] = &Player{
			KleiID:   kleiID,
			Name:     name,
			LastSeen: time.Now(),
		}
	}
}

// Remove immediately removes a player (leave event).
func (pt *PlayerTracker) Remove(kleiID string) {
	pt.mu.Lock()
	delete(pt.players, kleiID)
	pt.mu.Unlock()
}

// Prune removes players not seen since the cutoff.
// Called after processing a c_listplayers() response.
func (pt *PlayerTracker) Prune() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	cutoff := time.Now().Add(-pt.maxAge)
	for id, p := range pt.players {
		if p.LastSeen.Before(cutoff) {
			delete(pt.players, id)
		}
	}
}

// Count returns the number of tracked players.
func (pt *PlayerTracker) Count() int {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return len(pt.players)
}

// List returns a snapshot of all tracked players.
func (pt *PlayerTracker) List() []Player {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	result := make([]Player, 0, len(pt.players))
	for _, p := range pt.players {
		result = append(result, *p)
	}
	return result
}

// Clear removes all players (used on server restart).
func (pt *PlayerTracker) Clear() {
	pt.mu.Lock()
	pt.players = make(map[string]*Player)
	pt.mu.Unlock()
}
