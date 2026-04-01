package main

import (
	"bytes"
	"sync"
)

const maxPartialLen = 16 * 1024 // 16KB — treat hitting this as an implicit newline

// LogBuffer is a thread-safe ring buffer that stores the last N lines.
// Handles partial lines across Write calls, with a cap on partial line length.
type LogBuffer struct {
	mu      sync.RWMutex
	lines   []string
	pos     int
	size    int
	wrapped bool

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

// PrefixWriter returns an io.Writer that prefixes each line with the given
// tag (e.g. "[stdout]") before writing to the LogBuffer. Each PrefixWriter
// maintains its own partial line buffer, so stdout and stderr can be
// processed by independent goroutines without sharing mutable state.
func (lb *LogBuffer) PrefixWriter(prefix string) *prefixWriter {
	return &prefixWriter{
		lb:     lb,
		prefix: prefix,
	}
}

// prefixWriter is a per-stream writer that handles partial line buffering
// independently, then delegates completed lines to the shared LogBuffer.
type prefixWriter struct {
	lb      *LogBuffer
	prefix  string
	partial []byte
}

func (pw *prefixWriter) Write(p []byte) (int, error) {
	var completed []string

	remaining := p
	for len(remaining) > 0 {
		i := bytes.IndexByte(remaining, '\n')
		if i < 0 {
			// No newline — accumulate into partial buffer
			pw.partial = append(pw.partial, remaining...)
			// If partial exceeds cap, flush it as a line
			if len(pw.partial) >= maxPartialLen {
				completed = append(completed, pw.prefix+string(pw.partial))
				pw.partial = pw.partial[:0]
			}
			break
		}

		// Found a newline — complete the line
		var line string
		if len(pw.partial) > 0 {
			pw.partial = append(pw.partial, remaining[:i]...)
			line = pw.prefix + string(pw.partial)
			pw.partial = pw.partial[:0]
		} else {
			seg := remaining[:i]
			if len(seg) > 0 {
				line = pw.prefix + string(seg)
			}
		}
		remaining = remaining[i+1:]

		if line == "" {
			continue
		}
		completed = append(completed, line)
	}

	if len(completed) > 0 {
		pw.lb.store(completed)
	}

	return len(p), nil
}

// store appends completed lines to the ring buffer and fans out to subscribers.
func (lb *LogBuffer) store(lines []string) {
	lb.mu.Lock()
	for _, line := range lines {
		lb.lines[lb.pos] = line
		lb.pos = (lb.pos + 1) % lb.size
		if lb.pos == 0 {
			lb.wrapped = true
		}
	}
	lb.mu.Unlock()

	lb.subMu.RLock()
	for _, line := range lines {
		for ch := range lb.subs {
			select {
			case ch <- line:
			default:
			}
		}
	}
	lb.subMu.RUnlock()
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
