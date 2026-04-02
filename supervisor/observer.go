package main

import (
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Observer watches DST's stdout (via LogBuffer subscription) for known
// patterns and feeds discovered runtime state into the Supervisor.
type Observer struct {
	sup      *Supervisor
	patterns []pattern
}

type pattern struct {
	re      *regexp.Regexp
	handler func(matches []string)
	once    bool
	fired   bool
}

func NewObserver(sup *Supervisor) *Observer {
	o := &Observer{sup: sup}
	o.registerPatterns()
	return o
}

func (o *Observer) registerPatterns() {
	// Port announcements — DST prints these during startup.
	// The A2S query port is game_port + 1 (DST convention, confirmed by
	// node-gamedig and python-a2s). SteamMasterServerPort is for master
	// server registration, not queries.
	o.addOnce(`ServerPort:\s*(\d+)`, func(m []string) {
		gamePort, _ := strconv.Atoi(m[1])
		queryPort := fmt.Sprintf("%d", gamePort+1)
		slog.Info("observer: discovered game port", "game_port", m[1], "query_port", queryPort)
		o.sup.SetRuntimeField("game_port", m[1])
		o.sup.Health.SetQueryPort(queryPort)
	})

	o.addOnce(`SteamMasterServerPort:\s*(\d+)`, func(m []string) {
		slog.Info("observer: discovered master server port", "port", m[1])
	})

	o.addOnce(`SteamAuthPort:\s*(\d+)`, func(m []string) {
		slog.Info("observer: discovered auth port", "port", m[1])
	})

	// Server ready signals — these transition Starting → Running.
	// Master shards: "Server registered via geo DNS" is DST's declaration
	// that the server is in the lobby and ready for players.
	o.addOnce(`Server registered via geo DNS in (.+)`, func(m []string) {
		region := strings.TrimSpace(m[1])
		slog.Info("observer: server registered", "region", region)
		o.sup.SetRuntimeField("region", region)
		if o.sup.State.Get() == StateStarting {
			o.sup.State.Set(StateRunning)
			slog.Info("observer: server ready (geo DNS registered)")
		}
	})

	// Secondary shards: never emit geo DNS. Their readiness signal is
	// "[Shard] secondary shard LUA is now ready!" which fires after
	// connecting to the master and completing world load.
	o.addOnce(`\[Shard\] secondary shard LUA is now ready`, func(m []string) {
		slog.Info("observer: secondary shard ready")
		if o.sup.State.Get() == StateStarting {
			o.sup.State.Set(StateRunning)
			slog.Info("observer: server ready (secondary shard LUA ready)")
		}
	})

	o.addOnce(`Online Server Started on port:\s*(\d+)`, func(m []string) {
		slog.Info("observer: server is online", "port", m[1])
	})

	// Shard connection status — informational, surfaces in logs
	o.add(`\[Shard\] Connection to master failed`, func(m []string) {
		slog.Warn("observer: shard connection to master failed (retrying)")
	})

	// Player join/leave — immediate tracker updates.
	// Join format: "[Join Announcement] PlayerName"
	// We don't have the KU_ ID from the join message alone, but we do from
	// the auth line that precedes it: "Client authenticated: (KU_abc123) PlayerName"
	o.add(`Client authenticated:\s*\((KU_\w+)\)\s*(.+)`, func(m []string) {
		name := strings.TrimSpace(m[2])
		slog.Info("observer: player joined", "klei_id", m[1], "name", name)
		o.sup.Players.Seen(m[1], name)
	})

	o.add(`\[Leave Announcement\]\s*(.+)`, func(m []string) {
		name := strings.TrimSpace(m[1])
		slog.Info("observer: player left", "name", name)
		// We don't have the KU_ ID in the leave message, but the next
		// c_listplayers() poll will prune them via age-out.
	})

	// c_listplayers() response lines — format: "[1] (KU_abc123) PlayerName"
	o.add(`\[\d+\]\s+\((KU_\w+)\)\s+(.+)`, func(m []string) {
		name := strings.TrimSpace(m[2])
		o.sup.Players.Seen(m[1], name)
	})
}

func (o *Observer) add(pattern string, handler func([]string)) {
	o.patterns = append(o.patterns, newPattern(pattern, handler, false))
}

func (o *Observer) addOnce(pattern string, handler func([]string)) {
	o.patterns = append(o.patterns, newPattern(pattern, handler, true))
}

func newPattern(pat string, handler func([]string), once bool) pattern {
	return pattern{
		re:      regexp.MustCompile(pat),
		handler: handler,
		once:    once,
	}
}

// Run subscribes to the LogBuffer and matches each line against registered
// patterns. Also polls c_listplayers() periodically for player reconciliation.
// Blocks, so call in a goroutine.
func (o *Observer) Run(logs *LogBuffer) {
	ch := logs.Subscribe()
	defer logs.Unsubscribe(ch)

	// Start player poll in a separate goroutine
	go o.pollPlayers()

	for line := range ch {
		// Strip the [stdout]/[stderr] prefix for matching
		clean := line
		if idx := strings.Index(line, "] "); idx >= 0 && idx < 10 {
			clean = line[idx+2:]
		}

		for i := range o.patterns {
			p := &o.patterns[i]
			if p.once && p.fired {
				continue
			}
			if matches := p.re.FindStringSubmatch(clean); matches != nil {
				p.handler(matches)
				if p.once {
					p.fired = true
				}
			}
		}
	}
}

// pollPlayers periodically sends c_listplayers() and prunes stale entries.
func (o *Observer) pollPlayers() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if o.sup.State.Get() != StateRunning {
			continue
		}
		if err := o.sup.SendCommand("c_listplayers()"); err != nil {
			slog.Debug("observer: failed to poll players", "error", err)
			continue
		}
		// Give DST a moment to print the response, then prune
		time.Sleep(2 * time.Second)
		o.sup.Players.Prune()
	}
}

// Reset clears the "fired" state on all one-shot patterns.
// Called on restart so the observer re-discovers ports from the new process.
func (o *Observer) Reset() {
	for i := range o.patterns {
		o.patterns[i].fired = false
	}
}
