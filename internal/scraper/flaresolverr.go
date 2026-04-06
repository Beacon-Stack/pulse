package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	cfSessionTTL    = 25 * time.Minute  // CF cookies last ~30 min; refresh at 25
	solveTimeout    = 120000            // milliseconds — generous for hard challenges
	flareMaxTimeout = 150 * time.Second // HTTP client timeout for FlareSolverr
)

// FlareSolverr is a client for the FlareSolverr Cloudflare bypass proxy.
type FlareSolverr struct {
	apiURL string
	client *http.Client
	logger *slog.Logger

	mu       sync.RWMutex
	sessions map[string]*cfSession // domain → session
}

// cfSession holds cached Cloudflare bypass cookies for a domain.
type cfSession struct {
	cookies   []fsCookie
	userAgent string
	created   time.Time
}

// fsCookie matches FlareSolverr's cookie JSON format.
type fsCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
}

// fsRequest is the FlareSolverr API request body.
type fsRequest struct {
	Cmd        string `json:"cmd"`
	URL        string `json:"url"`
	MaxTimeout int    `json:"maxTimeout"`
}

// fsResponse is the FlareSolverr API response.
type fsResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Solution struct {
		URL       string     `json:"url"`
		Status    int        `json:"status"`
		Response  string     `json:"response"`
		Cookies   []fsCookie `json:"cookies"`
		UserAgent string     `json:"userAgent"`
	} `json:"solution"`
}

// NewFlareSolverr creates a FlareSolverr client. Returns nil if URL is empty.
func NewFlareSolverr(apiURL string, logger *slog.Logger) *FlareSolverr {
	if apiURL == "" {
		return nil
	}
	return &FlareSolverr{
		apiURL:   strings.TrimRight(apiURL, "/"),
		client:   &http.Client{Timeout: flareMaxTimeout},
		logger:   logger,
		sessions: make(map[string]*cfSession),
	}
}

// Solve sends a URL to FlareSolverr to bypass Cloudflare and returns the
// page HTML, cookies, and user agent string.
func (f *FlareSolverr) Solve(ctx context.Context, targetURL string) (html string, cookies []fsCookie, userAgent string, err error) {
	f.logger.Info("flaresolverr: solving challenge",
		"url", targetURL,
	)

	body, err := json.Marshal(fsRequest{
		Cmd:        "request.get",
		URL:        targetURL,
		MaxTimeout: solveTimeout,
	})
	if err != nil {
		return "", nil, "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.apiURL+"/v1", bytes.NewReader(body))
	if err != nil {
		return "", nil, "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", nil, "", fmt.Errorf("FlareSolverr request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10 MB max
	if err != nil {
		return "", nil, "", fmt.Errorf("reading response: %w", err)
	}

	var fsResp fsResponse
	if err := json.Unmarshal(respBody, &fsResp); err != nil {
		return "", nil, "", fmt.Errorf("decoding response: %w", err)
	}

	if fsResp.Status != "ok" {
		return "", nil, "", fmt.Errorf("FlareSolverr error: %s", fsResp.Message)
	}

	// Cache the session
	domain := extractDomainFromURL(targetURL)
	if domain != "" {
		f.mu.Lock()
		f.sessions[domain] = &cfSession{
			cookies:   fsResp.Solution.Cookies,
			userAgent: fsResp.Solution.UserAgent,
			created:   time.Now(),
		}
		f.mu.Unlock()
	}

	f.logger.Info("flaresolverr: challenge solved",
		"url", targetURL,
		"cookies", len(fsResp.Solution.Cookies),
		"status", fsResp.Solution.Status,
	)

	return fsResp.Solution.Response, fsResp.Solution.Cookies, fsResp.Solution.UserAgent, nil
}

// GetSession returns the cached CF session for a domain, if valid.
func (f *FlareSolverr) GetSession(domain string) (*cfSession, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	sess, ok := f.sessions[domain]
	if !ok || time.Since(sess.created) > cfSessionTTL {
		return nil, false
	}
	return sess, true
}

// ToHTTPCookies converts FlareSolverr cookies to standard http.Cookie objects.
func (sess *cfSession) ToHTTPCookies() []*http.Cookie {
	var out []*http.Cookie
	for _, c := range sess.cookies {
		out = append(out, &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			HttpOnly: c.HTTPOnly,
			Secure:   c.Secure,
		})
	}
	return out
}

func extractDomainFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
