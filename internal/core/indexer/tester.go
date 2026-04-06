package indexer

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TestResult holds the outcome of probing an indexer.
type TestResult struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	Duration string `json:"duration"`
}

// TestIndexer probes an indexer URL to validate connectivity and credentials.
// For Torznab/Newznab it performs a caps query; for generic URLs it checks HTTP status.
func TestIndexer(ctx context.Context, kind, url, apiKey string) TestResult {
	start := time.Now()

	client := &http.Client{Timeout: 15 * time.Second}

	switch kind {
	case "torznab", "newznab":
		return testTorznab(ctx, client, url, apiKey, start)
	default:
		return testGenericHTTP(ctx, client, url, apiKey, start)
	}
}

func testTorznab(ctx context.Context, client *http.Client, baseURL, apiKey string, start time.Time) TestResult {
	// Torznab/Newznab caps endpoint: /api?t=caps&apikey=...
	capsURL := baseURL
	if capsURL != "" && capsURL[len(capsURL)-1] != '/' {
		capsURL += "/"
	}
	capsURL = baseURL + "?t=caps"
	if apiKey != "" {
		capsURL += "&apikey=" + apiKey
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, capsURL, nil)
	if err != nil {
		return TestResult{Success: false, Message: fmt.Sprintf("Invalid URL: %v", err), Duration: since(start)}
	}

	resp, err := client.Do(req)
	if err != nil {
		return TestResult{Success: false, Message: fmt.Sprintf("Connection failed: %v", err), Duration: since(start)}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return TestResult{Success: false, Message: fmt.Sprintf("Failed to read response: %v", err), Duration: since(start)}
	}

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return TestResult{Success: false, Message: "Authentication failed — check your API key", Duration: since(start)}
	}

	if resp.StatusCode >= 400 {
		return TestResult{Success: false, Message: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200)), Duration: since(start)}
	}

	// Try to parse as Torznab/Newznab caps XML
	var caps struct {
		XMLName xml.Name `xml:"caps"`
		Server  struct {
			Title string `xml:"title,attr"`
		} `xml:"server"`
	}
	if err := xml.Unmarshal(body, &caps); err != nil {
		// Not valid caps XML, but the HTTP request succeeded
		return TestResult{Success: true, Message: "Connected (non-standard response)", Duration: since(start)}
	}

	title := caps.Server.Title
	if title == "" {
		title = "indexer"
	}
	return TestResult{Success: true, Message: fmt.Sprintf("Connected to %s", title), Duration: since(start)}
}

func testGenericHTTP(ctx context.Context, client *http.Client, url, apiKey string, start time.Time) TestResult {
	if url == "" {
		return TestResult{Success: false, Message: "URL is required", Duration: since(start)}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return TestResult{Success: false, Message: fmt.Sprintf("Invalid URL: %v", err), Duration: since(start)}
	}
	// Many torrent sites block requests without a browser-like user-agent.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Pulse/1.0)")
	if apiKey != "" {
		req.Header.Set("X-Api-Key", apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return TestResult{Success: false, Message: fmt.Sprintf("Connection failed: %v", err), Duration: since(start)}
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	// For generic site checks, any response (even 403/503) means the site is
	// reachable. Only network errors or timeouts count as failures.
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return TestResult{Success: true, Message: fmt.Sprintf("Connected (HTTP %d)", resp.StatusCode), Duration: since(start)}
	}
	// 403/503 are common for torrent sites behind Cloudflare — site is up.
	if resp.StatusCode == 403 || resp.StatusCode == 503 {
		return TestResult{Success: true, Message: fmt.Sprintf("Site reachable (HTTP %d — likely Cloudflare protected)", resp.StatusCode), Duration: since(start)}
	}
	if resp.StatusCode == 401 {
		return TestResult{Success: false, Message: "Authentication required", Duration: since(start)}
	}
	return TestResult{Success: false, Message: fmt.Sprintf("HTTP %d", resp.StatusCode), Duration: since(start)}
}

func since(start time.Time) string {
	return time.Since(start).Truncate(time.Millisecond).String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
