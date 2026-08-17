package scraper

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Regression suite for the 2026-08-17 "half the indexer fleet silently
// broken" incident. Each test locks down one engine defect found in the
// audit; the incident indexer is named in the comment so future readers
// can weigh the blast radius before touching the behavior.

// rowsPage is a minimal one-row HTML result page.
const rowsPage = `<table><tr class="row"><td class="title">Severance S02E01 1080p</td></tr></table>`

func simpleDef(serverURL, path string, pathInputs map[string]string) *Definition {
	return &Definition{
		ID: "t", Name: "T", Type: "public",
		Links: []string{serverURL},
		Search: SearchBlock{
			Paths: []SearchPath{{Path: path, Inputs: pathInputs}},
			Rows:  RowsBlock{Selector: "tr.row"},
			Fields: map[string]FieldBlock{
				"title": {SelectorBlock: SelectorBlock{Selector: "td.title"}},
			},
		},
	}
}

// The single defect that broke TorrentDownload, kickasstorrents.to,
// Torrent[CORE], and The Pirate Bay: keywords substituted into the path
// template reached the wire with a raw space (`GET /search?q=Severance
// S02E01`), and servers rejected the request line with HTTP 400.
func TestSearch_KeywordsInPathTemplateAreURLEncoded(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := *r
		captured = &req
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(rowsPage))
	}))
	t.Cleanup(srv.Close)

	def := simpleDef(srv.URL, "search?q={{ .Keywords }}", nil)
	r := NewRunner(def, nil, nil, slog.Default())

	results, err := r.Search(context.Background(), "Severance S02E01", nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if captured == nil {
		t.Fatal("server never hit")
	}
	if got := captured.URL.Query().Get("q"); got != "Severance S02E01" {
		t.Errorf("decoded q = %q, want %q", got, "Severance S02E01")
	}
	if strings.Contains(captured.URL.RawQuery, " ") {
		t.Errorf("raw query %q contains a literal space — request line would be rejected with HTTP 400", captured.URL.RawQuery)
	}
}

// Same defect via an absolute-URL path template (The Pirate Bay's
// definition builds the full apibay.org URL in the path).
func TestSearch_AbsoluteURLPathIsEncoded(t *testing.T) {
	var rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(rowsPage))
	}))
	t.Cleanup(srv.Close)

	def := simpleDef("http://unused.invalid", srv.URL+"/q.php?q={{ .Keywords }}&cat=205,208", nil)
	r := NewRunner(def, nil, nil, slog.Default())

	if _, err := r.Search(context.Background(), "Severance S02E01", nil); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if strings.Contains(rawQuery, " ") {
		t.Errorf("raw query %q contains a literal space", rawQuery)
	}
}

// When every search path fails, Search must return the error — returning
// (nil, nil) is how HTTP 400s were masked as healthy empty feeds for
// weeks.
func TestSearch_AllPathsFailReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	def := simpleDef(srv.URL, "search?q={{ .Keywords }}", nil)
	r := NewRunner(def, nil, nil, slog.Default())

	results, err := r.Search(context.Background(), "anything", nil)
	if err == nil {
		t.Fatalf("Search returned nil error with %d results; want the HTTP 400 surfaced", len(results))
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error %q should mention the HTTP status", err)
	}
}

// Partial failure keeps the successful path's results (availability over
// completeness), so one flaky mirror doesn't zero out an indexer.
func TestSearch_PartialPathFailureKeepsResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/bad") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(rowsPage))
	}))
	t.Cleanup(srv.Close)

	def := simpleDef(srv.URL, "good", nil)
	def.Search.Paths = append(def.Search.Paths, SearchPath{Path: "bad"})
	r := NewRunner(def, nil, nil, slog.Default())

	results, err := r.Search(context.Background(), "x", nil)
	if err != nil {
		t.Fatalf("Search: %v (partial failure must not fail the whole search)", err)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
}

// A template error in a path input must fail the path — silently
// dropping the input is how Nyaa's path #2 lost its `q` and dumped the
// site homepage (75 unrelated rows) into every search.
func TestSearch_InputTemplateErrorFailsPath(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(rowsPage))
	}))
	t.Cleanup(srv.Close)

	def := simpleDef(srv.URL, "/", map[string]string{"q": "{{ if }}"}) // unparseable, even after normalization
	r := NewRunner(def, nil, nil, slog.Default())

	if _, err := r.Search(context.Background(), "Severance S02E01", nil); err == nil {
		t.Fatal("Search returned nil error for an unparseable input template")
	}
	if hit {
		t.Error("request was sent despite the input template failing — degraded URL must not be fetched")
	}
}

// Nyaa.si path #2 verbatim from Prowlarr-Indexers v11 nyaasi.yml: the
// regex escapes are invalid Go string escapes and previously failed the
// template parse. After Cardigann normalization the q input must survive
// and reach the wire.
func TestSearch_NyaaPathTwoTemplateKeepsQuery(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := *r
		captured = &req
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(rowsPage))
	}))
	t.Cleanup(srv.Close)

	nyaaQ := `{{ if .Keywords }}{{ re_replace .Keywords "\b0(\d{1})\b" "$1" }}{{ else }}{{ end }}`
	def := simpleDef(srv.URL, "/", map[string]string{"q": nyaaQ})
	r := NewRunner(def, nil, nil, slog.Default())

	if _, err := r.Search(context.Background(), "Severance S02E01", nil); err != nil {
		t.Fatalf("Search: %v", err)
	}
	got := captured.URL.Query().Get("q")
	// \b0(\d{1})\b strips the leading zero of a standalone two-digit
	// token; "S02E01" has no standalone 0-prefixed token, so the query
	// passes through unchanged.
	if got != "Severance S02E01" {
		t.Errorf("q = %q, want %q — the Cardigann regex-escape template must evaluate, not vanish", got, "Severance S02E01")
	}
}

// Dashed .Config keys are legal in Cardigann and a lexer error in Go
// templates; they must resolve, not silently discard the user's setting
// (Nyaa's filter-id / cat-id).
func TestEvalTemplate_DashedConfigKeys(t *testing.T) {
	ctx := NewTemplateContext(map[string]string{"filter-id": "2", "cat-id": "1_2"}, "")
	got, err := EvalTemplate(`f={{ .Config.filter-id }}&c={{ .Config.cat-id }}`, ctx)
	if err != nil {
		t.Fatalf("EvalTemplate: %v", err)
	}
	if got != "f=2&c=1_2" {
		t.Errorf("got %q, want %q", got, "f=2&c=1_2")
	}
}

// re_replace uses Cardigann's argument order (value, pattern,
// replacement). TorrentDownload's search path depends on it:
// `search{{ re_replace .Config.sort "_" "" }}` must yield "searchd" for
// sort="d" — the reversed order silently produced the wrong URL.
func TestReReplace_CardigannArgumentOrder(t *testing.T) {
	ctx := NewTemplateContext(map[string]string{"sort": "added_desc"}, "")
	got, err := EvalTemplate(`{{ re_replace .Config.sort "_" "" }}`, ctx)
	if err != nil {
		t.Fatalf("EvalTemplate: %v", err)
	}
	if got != "addeddesc" {
		t.Errorf("re_replace = %q, want %q", got, "addeddesc")
	}
}

// The $raw pseudo-input is a pre-formatted query fragment (Torrent
// Downloads' category list). It must be spliced into the query string,
// not escaped into a bogus `%24raw=` parameter.
func TestBuildQueryString_RawInput(t *testing.T) {
	def := &Definition{Search: SearchBlock{}}
	r := NewRunner(def, nil, nil, slog.Default())
	ctx := NewTemplateContext(r.settings, "x")
	ctx.Categories = []string{"5", "8"}

	got, err := r.buildQueryString(map[string]string{
		"$raw": "{{ range .Categories }}s_cat={{.}}&{{end}}",
		"q":    "{{ .Keywords }}",
	}, ctx)
	if err != nil {
		t.Fatalf("buildQueryString: %v", err)
	}
	if strings.Contains(got, "%24raw") || strings.Contains(got, "$raw") {
		t.Errorf("query %q leaks the $raw key", got)
	}
	if !strings.Contains(got, "s_cat=5&s_cat=8") {
		t.Errorf("query %q missing spliced raw categories", got)
	}
	if !strings.Contains(got, "q=x") {
		t.Errorf("query %q missing regular input", got)
	}
}

// rows.filters andmatch keeps only rows whose title contains every
// keyword term — the client-side guard Prowlarr applies against sites
// that fuzzy-match or ignore the query.
func TestSearch_AndMatchDropsUnrelatedRows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<table>
<tr class="row"><td class="title">Severance S02E01 1080p WEB</td></tr>
<tr class="row"><td class="title">Totally Unrelated Anime BD</td></tr>
</table>`))
	}))
	t.Cleanup(srv.Close)

	def := simpleDef(srv.URL, "search?q={{ .Keywords }}", nil)
	def.Search.Rows.Filters = []FilterDef{{Name: "andmatch"}}
	r := NewRunner(def, nil, nil, slog.Default())

	results, err := r.Search(context.Background(), "Severance S02E01", nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || !strings.Contains(results[0].Title, "Severance") {
		t.Errorf("andmatch results = %v, want only the Severance row", results)
	}
}

// Field-level default: (Nyaa's `seeders: default: 0`) substitutes when
// extraction yields nothing.
func TestExtractField_DefaultValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(rowsPage)) // no td.seeds present
	}))
	t.Cleanup(srv.Close)

	def := simpleDef(srv.URL, "/", nil)
	def.Search.Fields["seeders"] = FieldBlock{
		SelectorBlock: SelectorBlock{Selector: "td.seeds"},
		Default:       0,
	}
	r := NewRunner(def, nil, nil, slog.Default())

	results, err := r.Search(context.Background(), "x", nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Seeders != 0 {
		t.Fatalf("results = %+v, want one row with Seeders=0 from the default", results)
	}
}

// Definition-supplied search headers must reach the wire (kickass .ws
// pins a custom User-Agent).
func TestSearch_DefinitionHeadersApplied(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(rowsPage))
	}))
	t.Cleanup(srv.Close)

	def := simpleDef(srv.URL, "/", nil)
	def.Search.Headers = map[string][]string{"User-Agent": {"CardigannBot/1.0"}}
	r := NewRunner(def, nil, nil, slog.Default())

	if _, err := r.Search(context.Background(), "x", nil); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotUA != "CardigannBot/1.0" {
		t.Errorf("User-Agent = %q, want the definition's header", gotUA)
	}
}

// Structured query params must be visible to definitions via .Query.*.
func TestSearchWithQuery_PopulatesQueryContext(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := *r
		captured = &req
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(rowsPage))
	}))
	t.Cleanup(srv.Close)

	def := simpleDef(srv.URL, "/", map[string]string{
		"season": "{{ .Query.Season }}",
		"ep":     "{{ .Query.Ep }}",
		"kw":     "{{ .Query.Keywords }}",
	})
	r := NewRunner(def, nil, nil, slog.Default())

	_, err := r.SearchWithQuery(context.Background(), SearchQuery{Keywords: "Severance", Season: 2, Ep: 1})
	if err != nil {
		t.Fatalf("SearchWithQuery: %v", err)
	}
	q := captured.URL.Query()
	if q.Get("season") != "2" || q.Get("ep") != "1" || q.Get("kw") != "Severance" {
		t.Errorf("query = %q, want season=2 ep=1 kw=Severance", captured.URL.RawQuery)
	}
}

// A Cloudflare challenge solve must survive the search caller hanging up
// (Pilot's per-indexer timeout is 25s; solves take 60-120s). When the
// caller's context propagated into the solve, the solve aborted, the
// session was never cached, and every later search hit a cold session —
// CF-walled indexers were stuck in a permanent timeout loop.
func TestSolveWithFlareSolverr_SurvivesCallerCancellation(t *testing.T) {
	// Fake FlareSolverr: waits 300ms (past the caller's cancellation),
	// then returns a solved page containing one row.
	fsrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(`{"status":"ok","message":"","solution":{"url":"","status":200,"response":"` +
			`<table><tr class=\"row\"><td class=\"title\">Severance S02E01</td></tr></table>` +
			`","cookies":[{"name":"cf_clearance","value":"tok"}],"userAgent":"UA"}}`))
	}))
	t.Cleanup(fsrv.Close)

	// Fake Cloudflare-walled site.
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "cloudflare")
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(site.Close)

	def := simpleDef(site.URL, "search?q={{ .Keywords }}", nil)
	fs := NewFlareSolverr(fsrv.URL, slog.Default())
	r := NewRunner(def, nil, fs, slog.Default())

	// Cancel the caller's context shortly after the scrape starts —
	// mid-solve, like an impatient client.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	results, err := r.Search(ctx, "Severance S02E01", nil)
	if err != nil {
		t.Fatalf("Search: %v — the detached solve should have completed despite cancellation", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1 from the solved page", len(results))
	}
	if _, ok := fs.GetSession(extractDomainFromURL(site.URL)); !ok {
		t.Error("solved session was not cached — the next search would hit a cold session again")
	}
}

// The runner cache must be keyed on settings too: keyed on catalogID
// alone, a settings change in Pulse (fixing Nyaa's invalid cat-id)
// never reached the scraper until a restart.
func TestGetRunner_SettingsChangeInvalidatesCache(t *testing.T) {
	e := NewEngine(slog.Default(), nil)
	e.LoadDefinitions(map[string][]byte{
		"testidx": []byte("id: testidx\nname: Test\nlinks:\n  - https://example.org\n"),
	})

	r1, err := e.GetRunner("testidx", `{"cat-id":"0"}`)
	if err != nil {
		t.Fatalf("GetRunner: %v", err)
	}
	r1again, err := e.GetRunner("testidx", `{"cat-id":"0"}`)
	if err != nil {
		t.Fatalf("GetRunner: %v", err)
	}
	if r1 != r1again {
		t.Error("same settings should hit the cached runner")
	}

	r2, err := e.GetRunner("testidx", `{"cat-id":"0_0"}`)
	if err != nil {
		t.Fatalf("GetRunner: %v", err)
	}
	if r1 == r2 {
		t.Error("changed settings returned the stale cached runner")
	}
	if got := r2.settings["cat-id"]; got != "0_0" {
		t.Errorf("new runner settings cat-id = %q, want 0_0", got)
	}
}
