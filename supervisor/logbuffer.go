package main

import (
	"bytes"
	"sync"
)

// LogBuffer is a thread-safe ring buffer that stores the last N lines.
// Implements io.Writer — handles partial lines across Write calls.
type LogBuffer struct {
	mu      sync.RWMutex
	lines   []string
	pos     int
	size    int
	wrapped bool

	// Buffered partial line (no trailing newline yet)
	partial []byte

	// Subscribers for live streaming
	subMu sync.RWMutex
	subs  map[chan string]struct{}
}

func NewLogBuffer(capacity int) *LogBuffer {
	return &LogBuffer{
		lines: make([]string, capacity),
		size:  capacity,
		subs:  make(map[chan string]struct{}),
	}
}

// Write implements io.Writer. Buffers partial lines across calls and only
// stores complete lines (terminated by \n) into the ring buffer.
func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	lb.mu.Lock()

	var completed []string

	remaining := p
	for len(remaining) > 0 {
		i := bytes.IndexByte(remaining, '\n')
		if i < 0 {
			// No newline — accumulate into partial buffer
			lb.partial = append(lb.partial, remaining...)
			break
		}

		// Found a newline — complete the line
		var line string
		if len(lb.partial) > 0 {
			lb.partial = append(lb.partial, remaining[:i]...)
			line = string(lb.partial)
			lb.partial = lb.partial[:0] // reset without deallocating
		} else {
			line = string(remaining[:i])
		}
		remaining = remaining[i+1:]

		if line == "" {
			continue
		}

		lb.lines[lb.pos] = line
		lb.pos = (lb.pos + 1) % lb.size
		if lb.pos == 0 {
			lb.wrapped = true
		}
		completed = append(completed, line)
	}

	lb.mu.Unlock()

	// Fan out completed lines to subscribers outside the buffer lock
	if len(completed) > 0 {
		lb.subMu.RLock()
		for _, line := range completed {
			for ch := range lb.subs {
				select {
				case ch <- line:
				default:
				}
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
	if lb.wrapped {
		total = lb.size
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
		start += lb.size
	}
	for i := 0; i < n; i++ {
		result[i] = lb.lines[(start+i)%lb.size]
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
