package auth

import (
	"testing"
	"time"
)

func TestAttemptLimiter(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	l := newAttemptLimiter()
	l.now = func() time.Time { return now }

	for i := 0; i < freeAttempts-1; i++ {
		l.fail("a")
	}
	if l.wait("a") != 0 {
		t.Fatal("the first few misses shouldn't lock anything")
	}
	l.fail("a")
	if d := l.wait("a"); d != firstLockout {
		t.Fatalf("after %d misses wait = %v, want %v", freeAttempts, d, firstLockout)
	}
	if l.wait("b") != 0 {
		t.Fatal("another address must not be held up")
	}
	now = now.Add(firstLockout)
	if l.wait("a") != 0 {
		t.Fatal("the lock should have run out")
	}
	l.fail("a")
	if d := l.wait("a"); d != 2*firstLockout {
		t.Fatalf("the next miss should double the wait, got %v", d)
	}
	for i := 0; i < 20; i++ {
		l.fail("a")
	}
	if d := l.wait("a"); d != maxLockout {
		t.Fatalf("wait is capped at %v, got %v", maxLockout, d)
	}
	l.ok("a")
	if l.wait("a") != 0 {
		t.Fatal("a correct password clears the count")
	}
	// Old misses are forgotten.
	for i := 0; i < freeAttempts; i++ {
		l.fail("c")
	}
	now = now.Add(forgetAfter + time.Second)
	if l.wait("c") != 0 {
		t.Fatal("misses older than forgetAfter shouldn't count")
	}
}
