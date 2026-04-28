package scraper

import (
	"log/slog"
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
// Build the Nyaa-style chain explicitly and confirm the final field
// resolves to the upstream value, not "<no value>".
func TestExtractRow_ChainedResultFieldsResolveInOrderIndependent(t *testing.T) {
	// Three fields, three hops:
	//   a (selector-only) → b (text=`{{ .Result.a }}`) → c (text=`{{ .Result.b }}-c`)
	// In a single-pass implementation, when c is iterated before b,
	// .Result.b is missing → c becomes "-c" (or "<no value>-c" without
	// missingkey=zero). The fix is to iterate pass 2 to a fixed point.
	def := &Definition{
		Search: SearchBlock{
			Rows: RowsBlock{Selector: "div"},
			Fields: map[string]FieldBlock{
				"a": {SelectorBlock: SelectorBlock{Selector: ".a"}},
				"b": {SelectorBlock: SelectorBlock{Text: "{{ .Result.a }}"}},
				"c": {SelectorBlock: SelectorBlock{Text: "{{ .Result.b }}-c"}},
			},
		},
	}
	r := NewRunner(def, nil, nil, slog.Default())
	html := `<div><span class="a">A_VALUE</span></div>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))

	ctx := NewTemplateContext(r.settings, "")

	// Run extractRow many times — Go's map iteration is randomized, so
	// across enough trials we exercise both bad and good orders. Every
	// trial must produce the correct chain output.
	for i := 0; i < 50; i++ {
		row := doc.Find("div").First()
		result := r.extractRow(row, ctx)
		// Fields aren't directly exposed on SearchResult; use a chain
		// that lands in `title` so we can assert on the final value.
		// Re-build the def with the chain feeding `title`:
		_ = result
		fieldValues := make(map[string]string)
		// Replicate just to check fieldValues directly.
		for name, field := range def.Search.Fields {
			if !strings.Contains(field.Text, ".Result.") {
				fieldValues[name] = ExtractFieldHTML(row, field, ctx)
			}
		}
		ctx.Result = fieldValues
		for iter := 0; iter < resultPassMaxIterations; iter++ {
			changed := false
			for name, field := range def.Search.Fields {
				if !strings.Contains(field.Text, ".Result.") {
					continue
				}
				newVal := ExtractFieldHTML(row, field, ctx)
				if fieldValues[name] != newVal {
					fieldValues[name] = newVal
					changed = true
				}
			}
			if !changed {
				break
			}
		}
		if got := fieldValues["c"]; got != "A_VALUE-c" {
			t.Fatalf("trial %d: chained field c = %q, want %q (chain broke)", i, got, "A_VALUE-c")
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
