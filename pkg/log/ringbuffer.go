package log

// ringbuffer.go — fixed-size, thread-safe circular buffer of log
// entries. Backs the in-app "Logs" panel for the case where the
// container's stdout via the Docker socket isn't reachable (running
// outside Docker, socket not mounted, etc). When Docker stdout is
// available, the UI prefers reading from there because the history
// is much longer; the ring buffer is the fallback.

import (
	"sync"
	"time"
)

// Entry is a single log entry stored in the ring buffer. The shape
// mirrors what the slog JSON handler emits to stdout — same field
// names so the in-app Logs UI can render Docker-stdout entries and
// ring-buffer entries through one component.
type Entry struct {
	Time    time.Time      `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"msg"`
	Fields  map[string]any `json:"fields,omitempty"`
}

// RingBuffer is a fixed-size, thread-safe circular buffer. Append is
// O(1); reads copy the snapshot under lock so iteration is safe even
// while writes continue.
type RingBuffer struct {
	mu      sync.Mutex
	entries []Entry
	head    int
	full    bool
}

// NewRingBuffer creates a buffer that holds up to size entries before
// it starts overwriting the oldest.
func NewRingBuffer(size int) *RingBuffer {
	if size <= 0 {
		size = 1000
	}
	return &RingBuffer{entries: make([]Entry, size)}
}

// Add appends an entry. Overwrites the oldest when full.
func (rb *RingBuffer) Add(e Entry) {
	rb.mu.Lock()
	rb.entries[rb.head] = e
	rb.head++
	if rb.head == len(rb.entries) {
		rb.head = 0
		rb.full = true
	}
	rb.mu.Unlock()
}

// Entries returns a snapshot of all buffered entries, oldest first.
func (rb *RingBuffer) Entries() []Entry {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	n := rb.lenLocked()
	out := make([]Entry, n)
	if n == 0 {
		return out
	}
	if rb.full {
		copied := copy(out, rb.entries[rb.head:])
		copy(out[copied:], rb.entries[:rb.head])
	} else {
		copy(out, rb.entries[:rb.head])
	}
	return out
}

// Len returns how many entries are currently stored.
func (rb *RingBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.lenLocked()
}

func (rb *RingBuffer) lenLocked() int {
	if rb.full {
		return len(rb.entries)
	}
	return rb.head
}
