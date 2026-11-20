package scraper

import (
	"errors"
	"testing"
	"time"
)

func statsAt(now *time.Time) *challengeStats {
	c := newChallengeStats()
	c.now = func() time.Time { return *now }
	return c
}

func TestChallengeStats_FlagsSitesThatKeepFailing(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	c := statsAt(&now)
	for i := 0; i < 3; i++ {
		c.record("1337x.to", false, "FlareSolverr error: Error solving the challenge")
		now = now.Add(time.Minute)
	}
	c.record("thepiratebay.org", true, "")
	got := c.troubles()
	if len(got) != 1 || got[0].Host != "1337x.to" || got[0].Failures != 3 {
		t.Fatalf("troubles = %+v, want 1337x.to with 3 failures", got)
	}
	if got[0].LastError == "" || got[0].LastFailure.IsZero() {
		t.Errorf("trouble %+v is missing its last error", got[0])
	}
}

func TestChallengeStats_NoiseIsNotTrouble(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	c := statsAt(&now)
	// Two failures: noise.
	c.record("a.example", false, "x")
	c.record("a.example", false, "x")
	// Three failures but mostly solving: not the IP.
	for i := 0; i < 3; i++ {
		c.record("b.example", false, "x")
	}
	for i := 0; i < 2; i++ {
		c.record("b.example", true, "")
	}
	if got := c.troubles(); len(got) != 0 {
		t.Errorf("troubles = %+v, want none", got)
	}
}

func TestChallengeStats_OldOutcomesAgeOut(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	c := statsAt(&now)
	for i := 0; i < 5; i++ {
		c.record("1337x.to", false, "x")
	}
	now = now.Add(challengeWindow + time.Minute)
	if got := c.troubles(); len(got) != 0 {
		t.Errorf("troubles after the window = %+v, want none", got)
	}
}

func TestChallengeStats_ResetForgetsEverything(t *testing.T) {
	now := time.Now()
	c := statsAt(&now)
	for i := 0; i < 5; i++ {
		c.record("1337x.to", false, "x")
	}
	c.reset()
	if got := c.troubles(); len(got) != 0 {
		t.Errorf("troubles after reset = %+v, want none", got)
	}
}

func TestSolverReported(t *testing.T) {
	if !solverReported(errors.New("FlareSolverr error: Error: Error solving the challenge. Timeout after 120.0 seconds.")) {
		t.Error("FlareSolverr's own failure must count")
	}
	if solverReported(errors.New("FlareSolverr request failed: context deadline exceeded")) {
		t.Error("not reaching FlareSolverr is not a challenge failure")
	}
	if solverReported(nil) {
		t.Error("nil is not a failure")
	}
}
