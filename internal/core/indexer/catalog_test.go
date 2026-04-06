package indexer

import (
	"testing"
)

func TestCatalogNotEmpty(t *testing.T) {
	entries := Catalog()
	if len(entries) == 0 {
		t.Fatal("catalog should not be empty")
	}
}

func TestCatalogEntriesHaveRequiredFields(t *testing.T) {
	for _, e := range Catalog() {
		if e.ID == "" {
			t.Errorf("entry %q has empty ID", e.Name)
		}
		if e.Name == "" {
			t.Errorf("entry %q has empty Name", e.ID)
		}
		if e.Protocol == "" {
			t.Errorf("entry %q has empty Protocol", e.Name)
		}
		if e.Privacy == "" {
			t.Errorf("entry %q has empty Privacy", e.Name)
		}
		if len(e.Categories) == 0 {
			t.Errorf("entry %q has no Categories", e.Name)
		}
	}
}

func TestCatalogUniqueIDs(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range Catalog() {
		if seen[e.ID] {
			t.Errorf("duplicate catalog ID: %q", e.ID)
		}
		seen[e.ID] = true
	}
}

func TestFilterCatalogByProtocol(t *testing.T) {
	torrent := FilterCatalog(CatalogFilter{Protocol: "torrent"})
	usenet := FilterCatalog(CatalogFilter{Protocol: "usenet"})
	all := FilterCatalog(CatalogFilter{})

	if len(torrent)+len(usenet) != len(all) {
		t.Errorf("torrent(%d) + usenet(%d) != all(%d)", len(torrent), len(usenet), len(all))
	}

	for _, e := range torrent {
		if e.Protocol != "torrent" {
			t.Errorf("expected torrent, got %q for %s", e.Protocol, e.Name)
		}
	}
	for _, e := range usenet {
		if e.Protocol != "usenet" {
			t.Errorf("expected usenet, got %q for %s", e.Protocol, e.Name)
		}
	}
}

func TestFilterCatalogByPrivacy(t *testing.T) {
	pub := FilterCatalog(CatalogFilter{Privacy: "public"})
	if len(pub) == 0 {
		t.Error("expected some public indexers")
	}
	for _, e := range pub {
		if e.Privacy != "public" {
			t.Errorf("expected public, got %q for %s", e.Privacy, e.Name)
		}
	}
}

func TestFilterCatalogByCategory(t *testing.T) {
	movies := FilterCatalog(CatalogFilter{Category: "Movies"})
	if len(movies) == 0 {
		t.Error("expected some Movie indexers")
	}
	for _, e := range movies {
		found := false
		for _, c := range e.Categories {
			if c == "Movies" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("entry %q filtered for Movies but doesn't have Movies category", e.Name)
		}
	}
}

func TestFilterCatalogByQuery(t *testing.T) {
	results := FilterCatalog(CatalogFilter{Query: "pirate"})
	if len(results) == 0 {
		t.Error("expected to find 'pirate' in catalog")
	}

	// Should not find gibberish
	none := FilterCatalog(CatalogFilter{Query: "xyzzy99999"})
	if len(none) != 0 {
		t.Errorf("expected 0 results for gibberish query, got %d", len(none))
	}
}

func TestFilterCatalogCombined(t *testing.T) {
	results := FilterCatalog(CatalogFilter{
		Protocol: "torrent",
		Privacy:  "public",
		Category: "Movies",
	})
	if len(results) == 0 {
		t.Error("expected some public torrent movie indexers")
	}
	for _, e := range results {
		if e.Protocol != "torrent" || e.Privacy != "public" {
			t.Errorf("filter mismatch: %s protocol=%s privacy=%s", e.Name, e.Protocol, e.Privacy)
		}
	}
}
