package log

import (
	"sync"
	"testing"
	"time"
)

func TestRingBuffer_FillAndOverflow(t *testing.T) {
	rb := NewRingBuffer(3)
	if rb.Len() != 0 {
		t.Fatalf("fresh buffer Len = %d, want 0", rb.Len())
	}

	rb.Add(Entry{Message: "a"})
	rb.Add(Entry{Message: "b"})
	rb.Add(Entry{Message: "c"})
	if rb.Len() != 3 {
		t.Fatalf("after 3 adds Len = %d, want 3", rb.Len())
	}

	got := rb.Entries()
	if len(got) != 3 || got[0].Message != "a" || got[2].Message != "c" {
		t.Errorf("Entries before overflow = %+v, want a/b/c chronological", got)
	}

	// Overwrite — oldest ("a") should be dropped.
	rb.Add(Entry{Message: "d"})
	got = rb.Entries()
	if len(got) != 3 || got[0].Message != "b" || got[2].Message != "d" {
		t.Errorf("Entries after overflow = %+v, want b/c/d", got)
	}

	// More overwrites.
	rb.Add(Entry{Message: "e"})
	rb.Add(Entry{Message: "f"})
	got = rb.Entries()
	if got[0].Message != "d" || got[2].Message != "f" {
		t.Errorf("Entries after multi-wrap = %+v, want d/e/f", got)
	}
}

func TestRingBuffer_DefaultSizeOnInvalidInput(t *testing.T) {
	for _, n := range []int{0, -1, -100} {
		rb := NewRingBuffer(n)
		if cap(rb.entries) != 1000 {
			t.Errorf("NewRingBuffer(%d) capacity = %d, want 1000 default", n, cap(rb.entries))
		}
	}
}

func TestRingBuffer_ConcurrentAddSafe(t *testing.T) {
	// 100 goroutines each adding 100 entries = 10000 adds against a
	// 200-slot buffer. We just need it not to race or panic.
	rb := NewRingBuffer(200)
	var wg sync.WaitGroup
	for g := 0; g < 100; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				rb.Add(Entry{Time: time.Now(), Message: "x"})
			}
		}(g)
	}
	// Concurrent readers too.
	for r := 0; r < 10; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = rb.Entries()
			}
		}()
	}
	wg.Wait()
	if rb.Len() != 200 {
		t.Errorf("Len after concurrent fill = %d, want 200 (capped)", rb.Len())
	}
}
