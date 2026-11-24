package scraper

import (
	"strconv"
	"strings"
)

// mapCategories maps Newznab category IDs to site-specific IDs.
func (r *Runner) mapCategories(newznabCats []int) []string {
	if len(newznabCats) == 0 {
		return nil
	}
	var siteCats []string
	for _, nc := range newznabCats {
		ncStr := strconv.Itoa(nc)
		for _, cm := range r.def.Caps.CategoryMappings {
			// Match if the Newznab cat prefix matches
			catParts := strings.Split(cm.Cat, "/")
			if len(catParts) > 0 {
				base := newznabBase(catParts[0])
				if base == ncStr || strings.HasPrefix(ncStr, base[:len(base)-1]) {
					siteCats = append(siteCats, cm.ID)
				}
			}
		}
	}
	return siteCats
}

// resolveCategoryID maps a site category ID back to a Newznab category ID string.
func (r *Runner) resolveCategoryID(siteCat string) string {
	for _, cm := range r.def.Caps.CategoryMappings {
		if cm.ID == siteCat {
			return newznabBase(strings.Split(cm.Cat, "/")[0])
		}
	}
	return "8000" // Other
}

// newznabBase returns the base Newznab category number for a category name.
func newznabBase(name string) string {
	switch strings.ToLower(name) {
	case "movies":
		return "2000"
	case "tv":
		return "5000"
	case "audio":
		return "3000"
	case "books":
		return "7000"
	case "xxx":
		return "6000"
	case "pc":
		return "4000"
	default:
		return "8000"
	}
}
