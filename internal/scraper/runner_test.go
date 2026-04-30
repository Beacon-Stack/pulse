package scraper

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// ── normalizeCardigannQuirks ────────────────────────────────────────────────

// Real-world regression: Prowlarr-Indexers v11/1337x.yml has an extra
// closing paren on its TV-search path. Without normalization, Go's
// text/template parser fails with "unexpected right paren" and that
// search path returns 0 results forever. Verify the fixup applies.
func TestNormalizeCardigannQuirks_Fixes1337xTVSearchPaths(t *testing.T) {
	// Excerpt of the broken upstream YAML (4 paths; only the TV one has
	// the typo).
	in := []byte(`
search:
  paths:
    - path: "{{ if and (.Keywords) (eq .Config.disablesort .False) }}sort-{{ else }}{{ end }}cat/Movies/1/"
    - path: "{{ if and (.Keywords) (eq .Config.disablesort .False)) }}sort-{{ else }}{{ end }}cat/TV/2/"
    - path: "{{ if and (.Keywords) (eq .Config.disablesort .False) }}sort-{{ else }}{{ end }}cat/Music/3/"
`)
	out := normalizeCardigannQuirks(in)
	s := string(out)

	// Bug fixed.
	if strings.Contains(s, ".False))") {
		t.Error("normalize did not strip the extra paren — typo still present")
	}
	// Correctly-shaped templates left alone.
	if c := strings.Count(s, ".False)"); c != 3 {
		t.Errorf("expected 3 occurrences of `.False)` after normalize, got %d", c)
	}
}

// Defensive: a template that already has the correct shape (single
// `.False)`) must NOT be altered. This prevents the workaround from
// double-stripping if the upstream YAML is ever fixed.
func TestNormalizeCardigannQuirks_LeavesCorrectTemplatesAlone(t *testing.T) {
	in := []byte(`path: "{{ if and (.Keywords) (eq .Config.disablesort .False) }}stuff"`)
	out := normalizeCardigannQuirks(in)
	if string(out) != string(in) {
		t.Errorf("correct template was modified:\n  in:  %s\n  out: %s", in, out)
	}
}

// End-to-end: the broken 1337x YAML, after normalization, must produce
// a template that EvalTemplate can parse without error. This is the
// behavioral guarantee — not just string editing, the actual fix.
func TestNormalizeCardigannQuirks_ResultParsesCleanly(t *testing.T) {
	rawTemplate := `{{ if and (.Keywords) (eq .Config.disablesort .False)) }}sort-{{ else }}{{ end }}/x`
	rawYAML := []byte("path: \"" + rawTemplate + "\"\n")

	// Sanity check: the raw template fails to parse before fixup.
	_, err := EvalTemplate(rawTemplate, NewTemplateContext(nil, "test"))
	if err == nil {
		t.Fatal("expected the broken template to fail before fixup; if this passes, the test is no longer testing the right thing")
	}

	// After normalization the YAML is rewritten and the template is
	// recoverable. Re-extract the path from the normalized YAML and
	// confirm it parses.
	normalized := string(normalizeCardigannQuirks(rawYAML))
	// crude extraction — the test YAML has exactly one quoted path
	start := strings.Index(normalized, `"`) + 1
	end := strings.LastIndex(normalized, `"`)
	fixedTemplate := normalized[start:end]

	if _, err := EvalTemplate(fixedTemplate, NewTemplateContext(nil, "test")); err != nil {
		t.Errorf("normalized template still fails to parse: %v", err)
	}
}

// ── Boolean coercion in NewRunner ───────────────────────────────────────────

// Headline regression — the Nyaa.si "<no value>" titles. The upstream
// YAML's `sonarr_compatibility` setting defaults to bool false, but
// fmt.Sprintf("%v", false) produces "false", and Go's `if` treats any
// non-empty string as truthy. The result was that every Nyaa search
// silently routed through the wrong branch of the title template and
// produced "<no value>" titles for 150 results per query.
func TestNewRunner_BoolFalseDefaultBecomesEmptyString(t *testing.T) {
	def := &Definition{
		Settings: []SettingField{{Name: "sonarr_compatibility", Type: "checkbox", Default: false}},
	}
	r := NewRunner(def, nil, nil, slog.Default())
	if got := r.settings["sonarr_compatibility"]; got != "" {
		t.Errorf("bool false default = %q, want empty string (truthy in template = bug)", got)
	}
}

func TestNewRunner_BoolTrueDefaultStaysTruthy(t *testing.T) {
	def := &Definition{
		Settings: []SettingField{{Name: "prefer_magnet_links", Type: "checkbox", Default: true}},
	}
	r := NewRunner(def, nil, nil, slog.Default())
	if got := r.settings["prefer_magnet_links"]; got != "true" {
		t.Errorf("bool true default = %q, want \"true\"", got)
	}
}

// User-supplied "false" for a checkbox setting must be folded to "" —
// otherwise users overriding a true default to false would re-introduce
// the same truthy-string bug.
func TestNewRunner_UserFalseStringIsNormalized(t *testing.T) {
	def := &Definition{
		Settings: []SettingField{{Name: "x", Type: "checkbox", Default: true}},
	}
	r := NewRunner(def, map[string]string{"x": "false"}, nil, slog.Default())
	if got := r.settings["x"]; got != "" {
		t.Errorf("user 'false' = %q, want empty string", got)
	}
}

// Settings NOT typed as checkbox must keep literal values like "0"
// intact. Cardigann YAML uses "0" as a filter ID ("no filter") and as
// a category ID; folding it to "" would silently break category
// targeting and filter selection.
func TestNewRunner_NonCheckboxZeroPreservesLiteral(t *testing.T) {
	def := &Definition{
		Settings: []SettingField{{Name: "filter-id", Type: "select", Default: "0"}},
	}
	r := NewRunner(def, map[string]string{"filter-id": "0"}, nil, slog.Default())
	if got := r.settings["filter-id"]; got != "0" {
		t.Errorf("non-checkbox 'filter-id'='0' = %q, want \"0\" (preserve literal)", got)
	}
}

// Non-bool config values (sort fields, category IDs, sitelinks) must
// pass through unchanged — the normalizer should only touch checkbox-
// shaped values.
func TestNewRunner_NonBoolStringsPassThrough(t *testing.T) {
	def := &Definition{
		Settings: []SettingField{{Name: "sort", Default: "seeders"}, {Name: "cat-id", Default: "1_2"}},
	}
	r := NewRunner(def, map[string]string{"sort": "leechers"}, nil, slog.Default())
	if got := r.settings["sort"]; got != "leechers" {
		t.Errorf("user-supplied sort = %q, want \"leechers\"", got)
	}
	if got := r.settings["cat-id"]; got != "1_2" {
		t.Errorf("default cat-id = %q, want \"1_2\"", got)
	}
}

// End-to-end check: with sonarr_compatibility=false, a Cardigann
// `{{ if .Config.sonarr_compatibility }}TRUE{{ else }}FALSE{{ end }}`
// template must enter the FALSE branch.
func TestNewRunner_BoolFalseRoutesToFalseBranch(t *testing.T) {
	def := &Definition{
		Settings: []SettingField{{Name: "sonarr_compatibility", Default: false}},
	}
	r := NewRunner(def, nil, nil, slog.Default())

	ctx := NewTemplateContext(r.settings, "")
	got, err := EvalTemplate(`{{ if .Config.sonarr_compatibility }}TRUE{{ else }}FALSE{{ end }}`, ctx)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if got != "FALSE" {
		t.Errorf("template branch = %q, want FALSE — bool false leaked as truthy string", got)
	}
}

// ── path.Inputs query-string building ───────────────────────────────────────

// Headline regression — Nyaa.si search ignored the q parameter because
// Pulse never built path.Inputs into the URL. Result: every search
// returned the homepage's latest 75 torrents, irrespective of query.
func TestBuildQueryString_EvaluatesPathInputsAgainstContext(t *testing.T) {
	def := &Definition{
		Search: SearchBlock{
			Inputs: map[string]string{
				"f": "{{ .Config.filter }}",
				"c": "{{ .Config.cat }}",
			},
		},
	}
	r := NewRunner(def, map[string]string{"filter": "0", "cat": "1_2"}, nil, slog.Default())
	ctx := NewTemplateContext(r.settings, "jujutsu kaisen")

	pathInputs := map[string]string{"q": "{{ .Keywords }}"}
	got := r.buildQueryString(pathInputs, ctx)

	// Sorted iteration — c, f, q
	want := "c=1_2&f=0&q=jujutsu+kaisen"
	if got != want {
		t.Errorf("query string = %q, want %q", got, want)
	}
}

// Empty values must be skipped — otherwise unset checkboxes leak as
// `&strip_s01=` and the indexer may interpret them as "yes".
func TestBuildQueryString_SkipsEmptyValues(t *testing.T) {
	def := &Definition{Search: SearchBlock{}}
	r := NewRunner(def, map[string]string{"x": ""}, nil, slog.Default())
	ctx := NewTemplateContext(r.settings, "")

	got := r.buildQueryString(map[string]string{
		"q":         "real",
		"strip_s01": "{{ .Config.x }}", // empty
		"empty":     "",
	}, ctx)
	want := "q=real"
	if got != want {
		t.Errorf("query string = %q, want %q", got, want)
	}
}

// path.Inputs must override search-block-level inputs of the same key.
// Cardigann YAML sometimes specifies a default at the search level and
// then overrides it per-path (e.g. `p` for pagination).
func TestBuildQueryString_PathInputsOverrideBlockInputs(t *testing.T) {
	def := &Definition{Search: SearchBlock{Inputs: map[string]string{"p": "1", "q": "from_block"}}}
	r := NewRunner(def, nil, nil, slog.Default())
	ctx := NewTemplateContext(r.settings, "")

	got := r.buildQueryString(map[string]string{"p": "2"}, ctx)
	want := "p=2&q=from_block"
	if got != want {
		t.Errorf("query string = %q, want %q", got, want)
	}
}

// ── Runner.Search end-to-end ─────────────────────────────────────────────────

// End-to-end test: composes the cdecd4b fixes (bool coercion,
// missingkey=zero, iterative pass-2, filter-arg template eval, and
// path.Inputs query-string building) through Runner.Search against a
// real httptest backend. Each individual fix has its own unit test;
// this is the contract that they all stay wired together so a
// re-arrangement of the runner doesn't silently lose one of them.
//
// Failure modes this catches that the unit tests miss:
//   - tmplCtx not propagated from Search→executePath→parseHTML→extractRow
//   - search-block-level inputs lost on path execution
//   - keyword filters applied to a different ctx than the row context
//   - parseHTML/extractRow integration drift (e.g. SearchResult.Title
//     populated but the FILTER on the title lost its template eval)
func TestRunnerSearch_EndToEnd_ComposesAllFixes(t *testing.T) {
	// Capture the request the runner makes so we can verify the
	// query-string was built from path.Inputs.
	var capturedRawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/html")
		// Two rows; one matches the keyword filter year-replace, one doesn't.
		_, _ = w.Write([]byte(`
<table>
  <tr class="row">
    <td class="title">Inception 2010 1080p [GROUP]</td>
    <td class="seeds">42</td>
  </tr>
  <tr class="row">
    <td class="title">Inception 2010 720p</td>
    <td class="seeds">7</td>
  </tr>
</table>
`))
	}))
	t.Cleanup(server.Close)

	// The Definition exercises every cdecd4b fix in one chain:
	//
	//   - sonarr_compat is a bool false default → must coerce to ""
	//     (truthy "false" string would route into the wrong if-branch
	//     of `title` below).
	//   - title_phase1 is selector-only → pass 1.
	//   - title_phase2 references .Result.title_phase1 → pass 2,
	//     must wait until phase1 is populated (random map order).
	//   - title is `{{ if .Config.sonarr_compat }}…{{ else }}{{ .Result.title_phase2 }}{{ end }}`
	//     — verifies bool coercion sends us into the ELSE branch and
	//     the title_phase2 chain survives the iterative-pass-2 loop.
	//   - title_phase2's filter has a template inside its args
	//     (`{{ if .Config.uppercase_titles }}…{{ else }}…{{ end }}`)
	//     — without filter-arg template eval the literal `{{ }}`
	//     directives leak into the rendered title.
	//   - search.inputs builds `?cat=tv` and path.inputs adds `?q=...`
	//     — the captured RawQuery below verifies both were merged.
	def := &Definition{
		ID:    "test",
		Name:  "Test Indexer",
		Type:  "public",
		Links: []string{server.URL},
		Settings: []SettingField{
			{Name: "sonarr_compat", Type: "checkbox", Default: false},
			{Name: "uppercase_titles", Type: "checkbox", Default: false},
		},
		Search: SearchBlock{
			Paths:  []SearchPath{{Path: "/search", Inputs: map[string]string{"q": "{{ .Keywords }}"}}},
			Inputs: map[string]string{"cat": "tv"},
			Rows:   RowsBlock{Selector: "tr.row"},
			Fields: map[string]FieldBlock{
				"title_phase1": {SelectorBlock: SelectorBlock{Selector: "td.title"}},
				"title_phase2": {SelectorBlock: SelectorBlock{
					Text: "{{ .Result.title_phase1 }}",
					Filters: []FilterDef{{
						Name: "re_replace",
						Args: []interface{}{
							` ?\[GROUP\]`,
							"{{ if .Config.uppercase_titles }} [UPPER]{{ else }}{{ end }}",
						},
					}},
				}},
				"title": {SelectorBlock: SelectorBlock{
					Text: "{{ if .Config.sonarr_compat }}SONARR_BRANCH{{ else }}{{ .Result.title_phase2 }}{{ end }}",
				}},
				"seeders": {SelectorBlock: SelectorBlock{Selector: "td.seeds"}},
			},
		},
	}

	r := NewRunner(def, nil, nil, slog.Default())
	results, err := r.Search(context.Background(), "Inception 2010", nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// Two rows on the page → two results out.
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2 — body=%q", len(results), capturedRawQuery)
	}

	// path.Inputs + search.inputs must both reach the wire (sorted
	// keys: cat, q). Bug #6 from cdecd4b.
	wantQuery := "cat=tv&q=Inception+2010"
	if capturedRawQuery != wantQuery {
		t.Errorf("captured query = %q, want %q (path.Inputs + search.inputs not merged)", capturedRawQuery, wantQuery)
	}

	// Title must be the cleaned-up form: bool coercion routed into the
	// ELSE branch (giving us .Result.title_phase2), the filter-arg
	// template eval evaluated to "" (uppercase_titles=false), the
	// re_replace dropped " [GROUP]", and the iterative pass-2 had to
	// resolve title_phase1 → title_phase2 → title across random map
	// order.
	want0 := "Inception 2010 1080p"
	want1 := "Inception 2010 720p"
	if results[0].Title != want0 || results[1].Title != want1 {
		t.Errorf("titles = [%q, %q], want [%q, %q]",
			results[0].Title, results[1].Title, want0, want1)
	}

	// Seeders must round-trip from the simple selector field.
	if results[0].Seeders != 42 {
		t.Errorf("results[0].Seeders = %d, want 42", results[0].Seeders)
	}
	if results[1].Seeders != 7 {
		t.Errorf("results[1].Seeders = %d, want 7", results[1].Seeders)
	}
}

// ── Filter-arg template evaluation ──────────────────────────────────────────

// Cardigann YAML routinely embeds Go templates inside re_replace
// replacement strings. Without evaluating the template against the row
// context, the directive leaks into the output as literal text — Nyaa.si
// title_phase2 has
//
//	args: ["...", "$1$2$3{{ if and (.Config.radarr_compatibility) (.Result.title_keyword_year) }} ...{{ else }}$4{{ end }}"]
//
// and after the boolean-coercion + missingkey=zero fixes, the literal
// template chars still appeared in scraped titles until ApplyFilters
// learned to evaluate args against the context.
func TestApplyFilters_EvaluatesTemplatesInArgs(t *testing.T) {
	ctx := NewTemplateContext(map[string]string{"radarr_compatibility": "true"}, "")
	ctx.Result = map[string]string{"title_keyword_year": "2024"}

	filters := []FilterDef{{
		Name: "re_replace",
		Args: []interface{}{
			`(.+?)\[`,
			"$1{{ if .Config.radarr_compatibility }} {{ .Result.title_keyword_year }} [{{ else }}[{{ end }}",
		},
	}}

	got := ApplyFilters("Some Show [1080p]", filters, ctx)
	want := "Some Show  2024 [1080p]"
	if got != want {
		t.Errorf("template-in-args = %q, want %q", got, want)
	}
}

// Without the boolean fix, .Config.radarr_compatibility="false" was a
// truthy string and selected the radarr-only branch. End-to-end check
// that with bool-coerced "" and template-evaluated args, the false
// branch is selected.
func TestApplyFilters_FalseConfigSelectsElseBranch(t *testing.T) {
	settings := map[string]string{"radarr_compatibility": ""} // bool false → ""
	ctx := NewTemplateContext(settings, "")
	ctx.Result = map[string]string{"title_keyword_year": "2024"}

	filters := []FilterDef{{
		Name: "re_replace",
		Args: []interface{}{
			`(.+?)\[`,
			"$1{{ if .Config.radarr_compatibility }} {{ .Result.title_keyword_year }} [{{ else }}[{{ end }}",
		},
	}}

	got := ApplyFilters("Some Show [1080p]", filters, ctx)
	want := "Some Show [1080p]"
	if got != want {
		t.Errorf("false branch = %q, want %q", got, want)
	}
}

// Nil context (download-extraction path) must not crash and must skip
// template eval entirely so static replacements still work.
func TestApplyFilters_NilContextSkipsTemplateEval(t *testing.T) {
	filters := []FilterDef{{
		Name: "replace",
		Args: []interface{}{"foo", "bar"},
	}}
	got := ApplyFilters("foo baz", filters, nil)
	if got != "bar baz" {
		t.Errorf("nil-ctx replace = %q, want %q", got, "bar baz")
	}
}

// ── missingkey=zero on EvalTemplate ─────────────────────────────────────────

// Default Go text/template renders missing map keys as the literal
// "<no value>". Cardigann YAML chains fields where intermediate ones
// are legitimately empty (e.g. Nyaa's title_phase1, optional, only
// matches PuyaSubs releases), so without missingkey=zero those empties
// poison the chain and downstream templates emit "<no value>".
func TestEvalTemplate_MissingKeyRendersEmpty(t *testing.T) {
	ctx := NewTemplateContext(nil, "")
	ctx.Result = map[string]string{} // explicitly empty

	got, err := EvalTemplate(`prefix-{{ .Result.absent }}-suffix`, ctx)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if got != "prefix--suffix" {
		t.Errorf("missing-key render = %q, want %q (no <no value> leak)", got, "prefix--suffix")
	}
}

// ── Iterative pass 2 in extractRow ──────────────────────────────────────────

// Pass 2 must reach a fixed point regardless of map iteration order.
// Build a three-hop chain feeding into `title` (a SearchResult-bearing
// field) and confirm the SUT's extractRow renders the upstream value
// — not "<no value>" or the partial mid-chain value.
//
// The chain:
//
//	a     (selector ".a")              → "A_VALUE"
//	b     (text "{{ .Result.a }}")     → "A_VALUE"
//	title (text "{{ .Result.b }}-c")   → "A_VALUE-c"
//
// Go map iteration is randomized: in a single-pass implementation,
// `title` may be processed before `b`, leaving .Result.b empty and
// the rendered title "-c" (or "<no value>-c" without missingkey=zero).
// The 100 trials below exercise enough random orderings to surface
// the bug consistently if the iterative-fixed-point pass is broken.
//
// Verified by mutation (commit 09a9254/cdecd4b history):
//   - Cap `resultPassMaxIterations` to 1 → fails (chain doesn't resolve)
//   - Remove the iterative loop entirely (single pass) → fails
//   - Drop missingkey=zero on EvalTemplate → renders "<no value>-c"
//     (caught here AND by TestEvalTemplate_MissingKeyRendersEmpty)
func TestExtractRow_ChainedResultFieldsResolveInOrderIndependent(t *testing.T) {
	def := &Definition{
		Search: SearchBlock{
			Rows: RowsBlock{Selector: "div"},
			Fields: map[string]FieldBlock{
				"a":     {SelectorBlock: SelectorBlock{Selector: ".a"}},
				"b":     {SelectorBlock: SelectorBlock{Text: "{{ .Result.a }}"}},
				"title": {SelectorBlock: SelectorBlock{Text: "{{ .Result.b }}-c"}},
			},
		},
	}
	r := NewRunner(def, nil, nil, slog.Default())
	html := `<div><span class="a">A_VALUE</span></div>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))

	const trials = 100
	for i := 0; i < trials; i++ {
		// Fresh ctx per trial so .Result from the prior trial's
		// fixed-point loop can't carry forward and mask a regression.
		ctx := NewTemplateContext(r.settings, "")
		row := doc.Find("div").First()

		result := r.extractRow(row, ctx)

		if result.Title != "A_VALUE-c" {
			t.Fatalf("trial %d: extractRow's iterative pass-2 broke the chain — Title = %q, want %q",
				i, result.Title, "A_VALUE-c")
		}
	}
}

// ParseDefinition is the public seam — it takes raw YAML bytes and
// returns a Definition. The normalizer runs inside ParseDefinition, so
// a YAML carrying the 1337x typo must round-trip into a usable
// Definition.
func TestParseDefinition_AppliesQuirkFixupForBrokenTemplate(t *testing.T) {
	in := []byte(`---
id: test
name: Test
type: public
search:
  paths:
    - path: "{{ if and (.Keywords) (eq .Config.disablesort .False)) }}sort-{{ else }}{{ end }}cat/TV/2/"
search.rows:
  selector: ""
search.fields: {}
`)
	def, err := ParseDefinition(in)
	if err != nil {
		t.Fatalf("ParseDefinition rejected broken-template YAML: %v", err)
	}
	if def == nil || len(def.Search.Paths) == 0 {
		t.Fatalf("expected at least one path; got %+v", def)
	}
	if strings.Contains(def.Search.Paths[0].Path, ".False))") {
		t.Errorf("path was not normalized inside ParseDefinition: %q", def.Search.Paths[0].Path)
	}
}
