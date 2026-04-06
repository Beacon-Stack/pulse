package scraper

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// TemplateContext holds the variables available in Cardigann templates.
type TemplateContext struct {
	Config     map[string]string // user settings (sort, downloadlink, etc.)
	Keywords   string            // search query (after keywordsfilters)
	Query      QueryContext
	Categories []string           // site category IDs
	Result     map[string]string // for self-referencing fields
	True       bool
	False      bool
	Today      TodayContext
}

// QueryContext holds search query parameters.
type QueryContext struct {
	Type   string
	Q      string
	IMDBID string
	TMDBID string
	TVDBID string
	Season string
	Ep     string
	Year   string
	Page   int
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
			Q: query,
		},
		Result: map[string]string{},
		True:   true,
		False:  false,
		Today:  TodayContext{Year: 2026},
	}
}

// templateFuncs provides the function map used by Cardigann templates.
// Importantly, "eq" is overridden with a type-coercing version because
// Cardigann YAML often compares string config values with boolean literals
// (e.g., `eq .Config.disablesort .False`).
var templateFuncs = template.FuncMap{
	"join":       strings.Join,
	"replace":    strings.ReplaceAll,
	"tolower":    strings.ToLower,
	"toupper":    strings.ToUpper,
	"trimspace":  strings.TrimSpace,
	"contains":   strings.Contains,
	"hasprefix":  strings.HasPrefix,
	"hassuffix":  strings.HasSuffix,
	"re_replace": func(pattern, replacement, value string) string { return filterReReplace(value, pattern, replacement) },
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
func EvalTemplate(tmplStr string, ctx *TemplateContext) (string, error) {
	if tmplStr == "" {
		return "", nil
	}

	// Quick check — if no template markers, return as-is.
	if !strings.Contains(tmplStr, "{{") {
		return tmplStr, nil
	}

	t, err := template.New("").Funcs(templateFuncs).Parse(tmplStr)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, ctx); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// EvalTemplateOr evaluates a template, returning the fallback on error.
func EvalTemplateOr(tmplStr string, ctx *TemplateContext, fallback string) string {
	result, err := EvalTemplate(tmplStr, ctx)
	if err != nil || result == "" {
		return fallback
	}
	return result
}
