package scraper

import (
	"context"
	"testing"
)

func TestTrace(t *testing.T) {
	tr := &Trace{}
	ctx := WithTrace(context.Background(), tr)
	if c, _ := tr.Challenge(); c {
		t.Fatal("new trace reports a challenge")
	}
	traceFrom(ctx).noteChallenge(true)
	if c, s := tr.Challenge(); !c || !s {
		t.Fatalf("after a solved challenge: %v/%v", c, s)
	}
	// One unsolved page makes the whole search unsolved.
	traceFrom(ctx).noteChallenge(false)
	traceFrom(ctx).noteChallenge(true)
	if c, s := tr.Challenge(); !c || s {
		t.Fatalf("after an unsolved page: %v/%v", c, s)
	}
	// No trace attached: noting is a no-op, not a panic.
	traceFrom(context.Background()).noteChallenge(true)
}
