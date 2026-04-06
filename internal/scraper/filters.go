package scraper

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ApplyFilters runs a sequence of filters on a value.
func ApplyFilters(value string, filters []FilterDef) string {
	for _, f := range filters {
		value = applyFilter(value, f)
	}
	return value
}

func applyFilter(value string, f FilterDef) string {
	switch f.Name {
	case "querystring":
		return filterQuerystring(value, filterArgsString(f.Args))
	case "regexp":
		return filterRegexp(value, filterArgsString(f.Args))
	case "re_replace":
		args := filterArgsSlice(f.Args)
		if len(args) >= 2 {
			return filterReReplace(value, args[0], args[1])
		}
		return value
	case "replace":
		args := filterArgsSlice(f.Args)
		if len(args) >= 2 {
			return filterReplace(value, args[0], args[1])
		}
		return value
	case "split":
		args := filterArgsSlice(f.Args)
		if len(args) >= 2 {
			idx, _ := strconv.Atoi(args[1])
			return filterSplit(value, args[0], idx)
		}
		return value
	case "trim":
		arg := filterArgsString(f.Args)
		if arg != "" {
			return strings.Trim(value, arg)
		}
		return strings.TrimSpace(value)
	case "prepend":
		return filterArgsString(f.Args) + value
	case "append":
		return value + filterArgsString(f.Args)
	case "tolower":
		return strings.ToLower(value)
	case "toupper":
		return strings.ToUpper(value)
	case "urlencode":
		return url.QueryEscape(value)
	case "urldecode":
		decoded, err := url.QueryUnescape(value)
		if err != nil {
			return value
		}
		return decoded
	case "htmldecode":
		return filterHTMLDecode(value)
	case "dateparse":
		return filterDateparse(value, filterArgsString(f.Args))
	case "fuzzytime":
		return filterFuzzytime(value)
	case "timeago", "reltime":
		return filterFuzzytime(value)
	case "timeparse":
		return filterDateparse(value, filterArgsString(f.Args))
	case "strdump":
		// Debug filter — passthrough
		return value
	default:
		// Unknown filter — passthrough
		return value
	}
}

// ── Filter implementations ───────────────────────────────────────────────────

func filterQuerystring(value, param string) string {
	u, err := url.Parse(value)
	if err != nil {
		return ""
	}
	return u.Query().Get(param)
}

func filterRegexp(value, pattern string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return value
	}
	m := re.FindStringSubmatch(value)
	if len(m) > 1 {
		return m[1]
	}
	if len(m) > 0 {
		return m[0]
	}
	return ""
}

func filterReReplace(value, pattern, replacement string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return value
	}
	return re.ReplaceAllString(value, replacement)
}

func filterReplace(value, old, new string) string {
	return strings.ReplaceAll(value, old, new)
}

func filterSplit(value, sep string, index int) string {
	parts := strings.Split(value, sep)
	if index < 0 || index >= len(parts) {
		return ""
	}
	return parts[index]
}

func filterHTMLDecode(value string) string {
	r := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&#39;", "'",
		"&#x27;", "'",
		"&nbsp;", " ",
	)
	return r.Replace(value)
}

// filterDateparse converts a date string using a .NET-style format to RFC3339.
// Common Cardigann formats: "MMM. d yy", "htt MMM. d", "yyyy-MM-dd HH:mm:ss"
func filterDateparse(value, format string) string {
	goFormat := dotnetToGoFormat(format)
	t, err := time.Parse(goFormat, strings.TrimSpace(value))
	if err != nil {
		// Try common fallback formats
		for _, fallback := range []string{
			"Jan 2, 2006",
			"Jan 2 2006",
			"2006-01-02",
			"2006-01-02 15:04:05",
			"01/02/2006",
			"02 Jan 2006",
			time.RFC1123,
		} {
			if t2, err2 := time.Parse(fallback, strings.TrimSpace(value)); err2 == nil {
				return t2.UTC().Format(time.RFC3339)
			}
		}
		return value
	}
	// If the year is zero, assume current year
	if t.Year() == 0 {
		t = t.AddDate(time.Now().Year(), 0, 0)
	}
	return t.UTC().Format(time.RFC3339)
}

// filterFuzzytime parses relative time expressions like "2 hours ago", "5 min ago", "Yesterday"
func filterFuzzytime(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	now := time.Now().UTC()

	if v == "now" || v == "just now" {
		return now.Format(time.RFC3339)
	}
	if v == "today" {
		return now.Truncate(24 * time.Hour).Format(time.RFC3339)
	}
	if v == "yesterday" {
		return now.Add(-24 * time.Hour).Truncate(24 * time.Hour).Format(time.RFC3339)
	}

	// Try "N unit(s) ago" pattern
	re := regexp.MustCompile(`(\d+)\s*(second|minute|min|hour|day|week|month|year)s?\s*ago`)
	m := re.FindStringSubmatch(v)
	if len(m) == 3 {
		n, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "second":
			return now.Add(-time.Duration(n) * time.Second).Format(time.RFC3339)
		case "minute", "min":
			return now.Add(-time.Duration(n) * time.Minute).Format(time.RFC3339)
		case "hour":
			return now.Add(-time.Duration(n) * time.Hour).Format(time.RFC3339)
		case "day":
			return now.Add(-time.Duration(n) * 24 * time.Hour).Format(time.RFC3339)
		case "week":
			return now.Add(-time.Duration(n) * 7 * 24 * time.Hour).Format(time.RFC3339)
		case "month":
			return now.AddDate(0, -n, 0).Format(time.RFC3339)
		case "year":
			return now.AddDate(-n, 0, 0).Format(time.RFC3339)
		}
	}

	// Try parsing as a time today (e.g., "12:25am")
	for _, tf := range []string{"3:04pm", "3:04 pm", "15:04"} {
		if t, err := time.Parse(tf, strings.TrimSpace(value)); err == nil {
			return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC).Format(time.RFC3339)
		}
	}

	return value
}

// dotnetToGoFormat converts common .NET DateTime format strings to Go format.
func dotnetToGoFormat(format string) string {
	r := strings.NewReplacer(
		"yyyy", "2006",
		"yy", "06",
		"MMMM", "January",
		"MMM", "Jan",
		"MM", "01",
		"dd", "02",
		"d", "2",
		"HH", "15",
		"hh", "03",
		"htt", "03",     // Cardigann-specific: hour + am/pm
		"mm", "04",
		"ss", "05",
		"tt", "PM",
		"fff", "000",
	)
	return r.Replace(format)
}

// ── Argument helpers ─────────────────────────────────────────────────────────

func filterArgsString(args interface{}) string {
	switch v := args.(type) {
	case string:
		return v
	case []interface{}:
		if len(v) > 0 {
			return fmt.Sprintf("%v", v[0])
		}
	case []string:
		if len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

func filterArgsSlice(args interface{}) []string {
	switch v := args.(type) {
	case []interface{}:
		out := make([]string, len(v))
		for i, a := range v {
			out[i] = fmt.Sprintf("%v", a)
		}
		return out
	case []string:
		return v
	case string:
		return []string{v}
	}
	return nil
}
