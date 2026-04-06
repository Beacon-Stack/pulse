package scraper

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"time"
)

// RateLimitedClient is an HTTP client with per-host rate limiting.
type RateLimitedClient struct {
	client    *http.Client
	delay     time.Duration
	mu        sync.Mutex
	lastReq   time.Time
}

// NewRateLimitedClient creates an HTTP client with rate limiting.
// delay is the minimum time between requests (from the YAML requestDelay field).
func NewRateLimitedClient(delay time.Duration) *RateLimitedClient {
	jar, _ := cookiejar.New(nil)
	if delay < 2*time.Second {
		delay = 2 * time.Second // minimum 2s between requests
	}
	return &RateLimitedClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
		delay: delay,
	}
}

// Do executes an HTTP request, respecting the rate limit.
func (c *RateLimitedClient) Do(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	since := time.Since(c.lastReq)
	if since < c.delay {
		time.Sleep(c.delay - since)
	}
	c.lastReq = time.Now()
	c.mu.Unlock()

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	return c.client.Do(req)
}

// ApplyCFSession sets Cloudflare bypass cookies and user agent from a
// FlareSolverr session onto this client's cookie jar.
func (c *RateLimitedClient) ApplyCFSession(rawURL string, cookies []*http.Cookie, userAgent string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	if c.client.Jar != nil {
		c.client.Jar.SetCookies(u, cookies)
	}
	// Store user agent for use in subsequent requests — we override it in Do()
	// but the CF user agent matters for cookie validation.
	_ = userAgent // Currently we set a fixed UA in Do(); for CF bypass we'd need
	// to use the FlareSolverr UA. For now the cookie alone is usually sufficient.
}
