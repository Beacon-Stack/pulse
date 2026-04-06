package scraper

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/tidwall/gjson"
)

// ExtractFieldHTML extracts a field value from an HTML selection using the FieldBlock definition.
func ExtractFieldHTML(row *goquery.Selection, field FieldBlock, ctx *TemplateContext) string {
	var value string

	if field.Text != "" {
		// Text template — evaluate with current context
		value = EvalTemplateOr(field.Text, ctx, "")
	} else if field.Selector != "" {
		sel := row.Find(field.Selector)
		if sel.Length() == 0 {
			return ""
		}

		// Remove unwanted children before extracting text
		if field.Remove != "" {
			sel.Find(field.Remove).Remove()
		}

		if field.Attribute != "" {
			value, _ = sel.First().Attr(field.Attribute)
		} else {
			value = strings.TrimSpace(sel.First().Text())
		}
	}

	// Apply case mapping
	if len(field.Case) > 0 {
		if mapped, ok := field.Case[value]; ok {
			value = mapped
		}
	}

	// Apply filters
	if len(field.Filters) > 0 {
		value = ApplyFilters(value, field.Filters)
	}

	return value
}

// ExtractFieldJSON extracts a field value from a JSON result using gjson paths.
func ExtractFieldJSON(jsonStr string, field FieldBlock, ctx *TemplateContext) string {
	var value string

	if field.Text != "" {
		value = EvalTemplateOr(field.Text, ctx, "")
	} else if field.Selector != "" {
		result := gjson.Get(jsonStr, field.Selector)
		value = result.String()
	}

	if len(field.Case) > 0 {
		if mapped, ok := field.Case[value]; ok {
			value = mapped
		}
	}

	if len(field.Filters) > 0 {
		value = ApplyFilters(value, field.Filters)
	}

	return value
}

// FindRowsHTML finds result rows in an HTML document using the rows selector.
func FindRowsHTML(doc *goquery.Document, rows RowsBlock) *goquery.Selection {
	sel := doc.Find(rows.Selector)
	if rows.After > 0 && sel.Length() > rows.After {
		sel = sel.Slice(rows.After, sel.Length())
	}
	return sel
}

// FindRowsJSON extracts result items from a JSON response.
func FindRowsJSON(body string, rows RowsBlock) []string {
	selector := strings.TrimSpace(rows.Selector)

	// Handle Cardigann's "$" selector which means "the entire response is the array"
	// gjson doesn't understand "$", so parse the array directly.
	if selector == "" || selector == "$" || selector == "@this" {
		result := gjson.Parse(body)
		if !result.IsArray() {
			return nil
		}
		var items []string
		result.ForEach(func(_, value gjson.Result) bool {
			items = append(items, value.Raw)
			return true
		})
		return items
	}

	results := gjson.Get(body, selector)
	if !results.IsArray() {
		return nil
	}
	var items []string
	results.ForEach(func(_, value gjson.Result) bool {
		items = append(items, value.Raw)
		return true
	})
	return items
}
