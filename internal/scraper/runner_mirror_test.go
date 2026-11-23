package scraper

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const mirrorPage = `<table>
  <tr class="row"><td class="title">Dune 2021 1080p</td><td><a class="dl" href="/get/1">dl</a></td></tr>
</table>`

// site is a fake indexer address that answers with a status (200 serves
// mirrorPage) and counts its hits.
type site struct {
	srv    *httptest.Server
	hits   atomic.Int32
	status int
	paths  []string
}

func newSite(t *testing.T, status int) *site {
	t.Helper()
	s := &site{status: status}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		s.paths = append(s.paths, r.URL.RequestURI())
		if s.status != http.StatusOK {
			w.WriteHeader(s.status)
			return
		}
		_, _ = w.Write([]byte(mirrorPage))
	}))
	t.Cleanup(s.srv.Close)
	return s
}

// deadAddress is an address nothing listens on (connection refused).
func deadAddress(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close()
	return addr
}

func mirrorRunner(t *testing.T, links []string, paths ...string) *Runner {
	t.Helper()
	if len(paths) == 0 {
		paths = []string{"/search?q={{ .Keywords }}"}
	}
	var sp []SearchPath
	for _, p := range paths {
		sp = append(sp, SearchPath{Path: p})
	}
	def := &Definition{
		ID: "mirrors", Name: "Mirrors", Type: "public", Links: links,
		Search: SearchBlock{
			Paths: sp,
			Rows:  RowsBlock{Selector: "tr.row"},
			Fields: map[string]FieldBlock{
				"title":    {SelectorBlock: SelectorBlock{Selector: "td.title"}},
				"download": {SelectorBlock: SelectorBlock{Selector: "a.dl", Attribute: "href"}},
			},
		},
	}
	r := NewRunner(def, nil, nil, slog.Default())
	r.client.delay = 0 // no politeness delay in tests
	return r
}

func TestMirror_GoneSiteFallsBackAndRemembers(t *testing.T) {
	primary := newSite(t, http.StatusGone)
	mirror := newSite(t, http.StatusOK)
	r := mirrorRunner(t, []string{primary.srv.URL, mirror.srv.URL})

	res, err := r.Search(context.Background(), "dune", nil)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("results = %d, want 1", len(res))
	}
	// Relative links resolve against the address that answered.
	if want := mirror.srv.URL + "/get/1"; res[0].Download != want {
		t.Errorf("download = %q, want %q", res[0].Download, want)
	}
	if r.CurrentBase() != mirror.srv.URL {
		t.Errorf("current base = %q, want the mirror", r.CurrentBase())
	}

	// The next search starts at the mirror; the dead primary isn't asked.
	before := primary.hits.Load()
	if _, err := r.Search(context.Background(), "dune", nil); err != nil {
		t.Fatal(err)
	}
	if primary.hits.Load() != before {
		t.Error("second search went to the dead primary again")
	}
}

func TestMirror_UnreachableAndServerErrorsFallBack(t *testing.T) {
	for name, primary := range map[string]string{
		"connection refused": deadAddress(t),
		"HTTP 502":           newSite(t, http.StatusBadGateway).srv.URL,
	} {
		mirror := newSite(t, http.StatusOK)
		r := mirrorRunner(t, []string{primary, mirror.srv.URL})
		if res, err := r.Search(context.Background(), "dune", nil); err != nil || len(res) != 1 {
			t.Errorf("%s: results %d, err %v; want the mirror's result", name, len(res), err)
		}
	}
}

func TestMirror_NoFallbackForOrdinaryErrors(t *testing.T) {
	for name, status := range map[string]int{"HTTP 404": http.StatusNotFound, "HTTP 403": http.StatusForbidden} {
		primary := newSite(t, status)
		mirror := newSite(t, http.StatusOK)
		r := mirrorRunner(t, []string{primary.srv.URL, mirror.srv.URL})
		if _, err := r.Search(context.Background(), "dune", nil); err == nil {
			t.Errorf("%s: want the error, not a fallback", name)
		}
		if mirror.hits.Load() != 0 {
			t.Errorf("%s: mirror was tried", name)
		}
	}
}

func TestMirror_AllAddressesFail(t *testing.T) {
	r := mirrorRunner(t, []string{newSite(t, http.StatusGone).srv.URL, deadAddress(t), newSite(t, http.StatusServiceUnavailable).srv.URL})
	_, err := r.Search(context.Background(), "dune", nil)
	if err == nil {
		t.Fatal("want an error")
	}
	// The primary's error, plus how many others were tried.
	if !strings.Contains(err.Error(), "HTTP 410") || !strings.Contains(err.Error(), "also tried 2 other") {
		t.Errorf("error = %q", err)
	}
}

func TestMirror_SitelinkFollowsTheMirror(t *testing.T) {
	primary := newSite(t, http.StatusGone)
	mirror := newSite(t, http.StatusOK)
	// An absolute path built from sitelink, as many definitions do.
	r := mirrorRunner(t, []string{primary.srv.URL, mirror.srv.URL}, "{{ .Config.sitelink }}search?q={{ .Keywords }}")
	if _, err := r.Search(context.Background(), "dune", nil); err != nil {
		t.Fatalf("search: %v", err)
	}
	if mirror.hits.Load() == 0 {
		t.Error("sitelink still pointed at the dead primary")
	}
}

func TestSearchPageLabels(t *testing.T) {
	// One search page: the error is the cause itself, no template text.
	r := mirrorRunner(t, []string{newSite(t, http.StatusNotFound).srv.URL})
	_, err := r.Search(context.Background(), "dune", nil)
	if err == nil || err.Error() != "HTTP 404" {
		t.Errorf("single page error = %v, want \"HTTP 404\"", err)
	}
	// Several: labelled by number.
	r = mirrorRunner(t, []string{newSite(t, http.StatusNotFound).srv.URL}, "/a?q={{ .Keywords }}", "/b?q={{ .Keywords }}")
	_, err = r.Search(context.Background(), "dune", nil)
	if err == nil || !strings.Contains(err.Error(), "search page 2: HTTP 404") || strings.Contains(err.Error(), "{{") {
		t.Errorf("multi page error = %v", err)
	}
}
