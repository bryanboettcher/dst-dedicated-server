package main

import (
	"strings"
	"sync"
)

// LogBuffer is a thread-safe ring buffer that stores the last N lines.
type LogBuffer struct {
	mu    sync.RWMutex
	lines []string
	pos   int
	cap   int
	full  bool

	// Subscribers for live streaming
	subMu sync.RWMutex
	subs  map[chan string]struct{}
}

func NewLogBuffer(capacity int) *LogBuffer {
	return &LogBuffer{
		lines: make([]string, capacity),
		cap:   capacity,
		subs:  make(map[chan string]struct{}),
	}
}

// Write implements io.Writer. Splits input on newlines and stores each line.
func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	text := string(p)
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			continue
		}
		lb.mu.Lock()
		lb.lines[lb.pos] = line
		lb.pos = (lb.pos + 1) % lb.cap
		if lb.pos == 0 {
			lb.full = true
		}
		lb.mu.Unlock()

		// Notify subscribers (non-blocking)
		lb.subMu.RLock()
		for ch := range lb.subs {
			select {
			case ch <- line:
			default:
			}
		}
		lb.subMu.RUnlock()
	}
	return len(p), nil
}

// Last returns the most recent n lines in chronological order.
func (lb *LogBuffer) Last(n int) []string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	total := lb.pos
	if lb.full {
		total = lb.cap
	}
	if n > total {
		n = total
	}
	if n == 0 {
		return nil
	}

	result := make([]string, n)
	start := lb.pos - n
	if start < 0 {
		start += lb.cap
	}
	for i := 0; i < n; i++ {
		result[i] = lb.lines[(start+i)%lb.cap]
	}
	return result
}

// Subscribe returns a channel that receives new log lines as they arrive.
func (lb *LogBuffer) Subscribe() chan string {
	ch := make(chan string, 64)
	lb.subMu.Lock()
	lb.subs[ch] = struct{}{}
	lb.subMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (lb *LogBuffer) Unsubscribe(ch chan string) {
	lb.subMu.Lock()
	delete(lb.subs, ch)
	lb.subMu.Unlock()
	close(ch)
}
