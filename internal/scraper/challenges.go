package scraper

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Challenge tracking: how FlareSolverr's Cloudflare solves have gone per
// site, recently. When a site's challenges keep failing, the egress IP
// is the usual culprit (Cloudflare rates datacenter and much-used VPN
// IPs harder), and a new VPN IP or region is the fix the dashboard can
// offer. Only failures FlareSolverr itself reported count: FlareSolverr
// being unreachable or overloaded is a different problem a new IP
// doesn't solve.

const (
	// challengeWindow is how far back solve outcomes are considered.
	challengeWindow = 2 * time.Hour
	// troubleMinFailures is how many failed solves in the window make a
	// site worth flagging — one or two is ordinary Cloudflare noise.
	troubleMinFailures = 3
)

// SiteTrouble is one site whose Cloudflare challenges keep failing.
type SiteTrouble struct {
	Host        string    `json:"host"`
	Failures    int       `json:"failures"`
	Successes   int       `json:"successes"`
	LastError   string    `json:"last_error"`
	LastFailure time.Time `json:"last_failure"`
}

type solveOutcome struct {
	at  time.Time
	ok  bool
	err string
}

type challengeStats struct {
	mu       sync.Mutex
	now      func() time.Time
	outcomes map[string][]solveOutcome // host → oldest first
}

func newChallengeStats() *challengeStats {
	return &challengeStats{now: time.Now, outcomes: make(map[string][]solveOutcome)}
}

func (c *challengeStats) record(host string, ok bool, errMsg string) {
	if host == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	list := append(c.prune(c.outcomes[host], now), solveOutcome{at: now, ok: ok, err: errMsg})
	c.outcomes[host] = list
}

// prune drops outcomes older than the window. Caller holds mu.
func (c *challengeStats) prune(list []solveOutcome, now time.Time) []solveOutcome {
	cut := 0
	for cut < len(list) && now.Sub(list[cut].at) > challengeWindow {
		cut++
	}
	return list[cut:]
}

// troubles lists sites with at least troubleMinFailures failed solves in
// the window and at least twice as many failures as successes, worst
// first. A site that fails sometimes but mostly solves isn't flagged.
func (c *challengeStats) troubles() []SiteTrouble {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	var out []SiteTrouble
	for host, list := range c.outcomes {
		list = c.prune(list, now)
		c.outcomes[host] = list
		t := SiteTrouble{Host: host}
		for _, o := range list {
			if o.ok {
				t.Successes++
				continue
			}
			t.Failures++
			t.LastError = o.err
			t.LastFailure = o.at
		}
		if t.Failures >= troubleMinFailures && t.Failures >= 2*t.Successes {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Failures != out[j].Failures {
			return out[i].Failures > out[j].Failures
		}
		return out[i].Host < out[j].Host
	})
	return out
}

// reset forgets every outcome: after a new VPN IP, the old IP's record
// says nothing about the new one.
func (c *challengeStats) reset() {
	c.mu.Lock()
	c.outcomes = make(map[string][]solveOutcome)
	c.mu.Unlock()
}

// solverReported reports whether err is FlareSolverr's own verdict on a
// page (it ran the browser and couldn't get through), as opposed to a
// failure to talk to FlareSolverr at all.
func solverReported(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "FlareSolverr error:")
}
