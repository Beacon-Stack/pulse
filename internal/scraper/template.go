package scraper

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// TemplateContext holds the variables available in Cardigann templates.
type TemplateContext struct {
	Config     map[string]string // user settings (sort, downloadlink, etc.)
	Keywords   string            // search query (after keywordsfilters)
	Query      QueryContext
	Categories []string          // site category IDs
	Result     map[string]string // for self-referencing fields
	True       bool
	False      bool
	Today      TodayContext

	// siteBase is the site address this search runs against (a mirror
	// when the primary is down). Not visible to templates.
	siteBase string
}

// base returns the address this context's search runs against.
func (c *TemplateContext) base(fallback string) string {
	if c != nil && c.siteBase != "" {
		return c.siteBase
	}
	return fallback
}

// QueryContext holds search query parameters.
type QueryContext struct {
	Type     string
	Q        string
	Keywords string // same as the top-level Keywords; some definitions use .Query.Keywords
	IMDBID   string
	TMDBID   string
	TVDBID   string
	Season   string
	Ep       string
	Year     string
	Page     int
}

// TodayContext provides date helpers.
type TodayContext struct {
	Year int
}

// NewTemplateContext creates a context with sensible defaults.
func NewTemplateContext(config map[string]string, query string) *TemplateContext {
	return &TemplateContext{
		Config:   config,
		Keywords: query,
		Query: QueryContext{
			Q:        query,
			Keywords: query,
		},
		Result: map[string]string{},
		True:   true,
		False:  false,
		Today:  TodayContext{Year: time.Now().Year()},
	}
}

// templateFuncs provides the function map used by Cardigann templates.
// Importantly, "eq" is overridden with a type-coercing version because
// Cardigann YAML often compares string config values with boolean literals
// (e.g., `eq .Config.disablesort .False`).
var templateFuncs = template.FuncMap{
	"join":      strings.Join,
	"replace":   strings.ReplaceAll,
	"tolower":   strings.ToLower,
	"toupper":   strings.ToUpper,
	"trimspace": strings.TrimSpace,
	"contains":  strings.Contains,
	"hasprefix": strings.HasPrefix,
	"hassuffix": strings.HasSuffix,
	// Cardigann's calling convention is `re_replace <value> <pattern>
	// <replacement>` (e.g. `re_replace .Config.sort "_" ""`). Getting this
	// order wrong silently corrupts every definition that uses it.
	"re_replace": func(value, pattern, replacement string) string { return filterReReplace(value, pattern, replacement) },
	"eq":         coercingEq,
	"ne":         func(a, b interface{}) bool { return !coercingEq(a, b) },
}

// coercingEq compares two values with type coercion.
// Handles the common Cardigann case of comparing string "false" with bool false.
func coercingEq(a, b interface{}) bool {
	// Normalize both to strings for comparison.
	as := fmt.Sprintf("%v", a)
	bs := fmt.Sprintf("%v", b)
	return as == bs
}

// EvalTemplate evaluates a Go template string with the given context.
//
// `missingkey=zero` makes references to absent map keys render as the
// element type's zero value (empty string for map[string]string) rather
// than the literal "<no value>" Go's default produces. Cardigann YAML
// frequently chains fields (e.g. Nyaa's title_phase1 → title_phase2 →
// title) where intermediate fields legitimately have no value for some
// rows; without this option the unrendered "<no value>" string leaks
// into the final title and the consumer sees `<no value>` everywhere.
func EvalTemplate(tmplStr string, ctx *TemplateContext) (string, error) {
	if tmplStr == "" {
		return "", nil
	}

	// Quick check — if no template markers, return as-is.
	if !strings.Contains(tmplStr, "{{") {
		return tmplStr, nil
	}

	t, err := template.New("").Option("missingkey=zero").Funcs(templateFuncs).Parse(tmplStr)
	if err != nil {
		// Cardigann definitions are written for Prowlarr's C# engine, whose
		// template dialect is looser than Go's text/template. Retry once
		// with known dialect gaps normalized — silently dropping the value
		// here is what let Nyaa's second search path degrade to the
		// unfiltered homepage.
		t, err = template.New("").Option("missingkey=zero").Funcs(templateFuncs).Parse(normalizeCardigannTemplate(tmplStr))
		if err != nil {
			return "", err
		}
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// dashedConfigKeyRe matches `.Config.some-key` field accesses whose key
// contains a dash — legal in Cardigann, a lexer error in Go templates
// (the `-id` reads as the start of a number literal).
var dashedConfigKeyRe = regexp.MustCompile(`\.Config\.([A-Za-z0-9_]+(?:-[A-Za-z0-9_]+)+)`)

// actionStringRe matches double-quoted string literals inside a template
// action, honoring backslash escapes.
var actionStringRe = regexp.MustCompile(`"(?:\\.|[^"\\])*"`)

// actionRe matches whole template actions so string-literal fixes stay
// inside {{ ... }} and never touch literal text.
var actionRe = regexp.MustCompile(`\{\{(?:[^}]|\}[^}])*\}\}`)

// normalizeCardigannTemplate rewrites Cardigann template constructs that
// Go's text/template rejects:
//
//  1. `.Config.filter-id` → `(index .Config "filter-id")` — dashed keys.
//  2. Regex escapes in string literals: `"\b0(\d{1})\b"` is invalid as a Go
//     string (\b at string level is fine but \d is not a legal escape), so
//     the whole template fails to parse. Prowlarr's engine passes the
//     backslashes through to the regex. Doubling only the invalid escapes
//     (`\d` → `\\d`) makes strconv.Unquote yield the original `\d` while
//     leaving already-valid escapes like `\\s` untouched.
func normalizeCardigannTemplate(src string) string {
	src = dashedConfigKeyRe.ReplaceAllString(src, `(index .Config "$1")`)
	return actionRe.ReplaceAllStringFunc(src, func(action string) string {
		return actionStringRe.ReplaceAllStringFunc(action, fixInvalidStringEscapes)
	})
}

// fixInvalidStringEscapes takes a double-quoted string literal (quotes
// included). If Go's strconv.Unquote accepts it, it passes through
// unchanged. If not, the author was writing regex-style escapes (`\d`,
// `\b`, `\1`, …) with Prowlarr's C#-engine semantics, where the backslash
// reaches the regex literally — so EVERY escape is made literal by
// doubling its backslash. A bare `\b` in such a string means regex
// word-boundary, never backspace. Only `\"` (structural — terminates the
// literal otherwise) and `\\` (already an escaped backslash) are kept.
func fixInvalidStringEscapes(lit string) string {
	if _, err := strconv.Unquote(lit); err == nil {
		return lit
	}
	inner := lit[1 : len(lit)-1]
	var b strings.Builder
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if c != '\\' || i+1 >= len(inner) {
			b.WriteByte(c)
			continue
		}
		next := inner[i+1]
		if next == '"' || next == '\\' {
			b.WriteByte(c)
			b.WriteByte(next)
		} else {
			b.WriteString(`\\`)
			b.WriteByte(next)
		}
		i++
	}
	return `"` + b.String() + `"`
}

// EvalTemplateOr evaluates a template, returning the fallback on error.
func EvalTemplateOr(tmplStr string, ctx *TemplateContext, fallback string) string {
	result, err := EvalTemplate(tmplStr, ctx)
	if err != nil || result == "" {
		return fallback
	}
	return result
}
