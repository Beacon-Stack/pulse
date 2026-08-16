package indexer

import (
	"archive/zip"
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testTime() time.Time {
	return time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
}

const testDefYAML = `---
id: testidx
name: TestIdx
description: A test indexer
language: en-US
type: public
links:
  - https://testidx.example/
caps:
  categorymappings:
    - {cat: TV}
`

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// makeZip builds an in-memory zip with the given path → content entries.
func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("creating zip entry %q: %v", name, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("writing zip entry %q: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("closing zip: %v", err)
	}
	return buf.Bytes()
}

func TestParseDefinitionsZip(t *testing.T) {
	zipData := makeZip(t, map[string]string{
		// GitHub zipball layout: repo-prefixed path.
		"Prowlarr-Indexers-abc123/definitions/v11/testidx.yml": testDefYAML,
		// Non-v11 and non-yml entries are skipped, not errors.
		"Prowlarr-Indexers-abc123/definitions/v10/old.yml": testDefYAML,
		"Prowlarr-Indexers-abc123/README.md":               "hi",
		// Broken YAML counts as a parse error.
		"Prowlarr-Indexers-abc123/definitions/v11/broken.yml": "{{{not yaml",
	})

	entries, rawYAML, errCount, err := parseDefinitionsZip(zipData)
	if err != nil {
		t.Fatalf("parseDefinitionsZip: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != "testidx" {
		t.Fatalf("expected 1 entry (testidx), got %+v", entries)
	}
	if _, ok := rawYAML["testidx"]; !ok {
		t.Fatalf("raw YAML for testidx not stored; keys: %v", rawYAML)
	}
	if errCount != 1 {
		t.Fatalf("expected 1 parse error (broken.yml), got %d", errCount)
	}
}

func TestParseDefinitionsZip_SnapshotLayout(t *testing.T) {
	// The repacked snapshot has no repo prefix.
	zipData := makeZip(t, map[string]string{
		"definitions/v11/testidx.yml": testDefYAML,
	})
	entries, rawYAML, _, err := parseDefinitionsZip(zipData)
	if err != nil || len(entries) != 1 || len(rawYAML) != 1 {
		t.Fatalf("snapshot layout not accepted: entries=%d raw=%d err=%v", len(entries), len(rawYAML), err)
	}
}

func TestLoadLocal_EmbeddedSnapshot(t *testing.T) {
	pc := NewProwlarrCatalog(testLogger())
	var gotRaw map[string][]byte
	pc.SetUpdateFunc(func(raw map[string][]byte) { gotRaw = raw })

	source, err := pc.LoadLocal()
	if err != nil {
		t.Fatalf("LoadLocal: %v", err)
	}
	if source != "embedded" {
		t.Fatalf("expected source embedded, got %q", source)
	}
	// The snapshot ships hundreds of definitions; a low floor catches an
	// accidentally truncated or empty snapshot without pinning the count.
	if len(gotRaw) < 400 {
		t.Fatalf("embedded snapshot suspiciously small: %d definitions", len(gotRaw))
	}
	if len(pc.Entries()) < 400 {
		t.Fatalf("catalog entries suspiciously small: %d", len(pc.Entries()))
	}
}

func TestLoadLocal_DiskCachePreferred(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "prowlarr-definitions.zip")
	zipData := makeZip(t, map[string]string{
		"definitions/v11/testidx.yml": testDefYAML,
	})
	if err := os.WriteFile(cachePath, zipData, 0o644); err != nil {
		t.Fatal(err)
	}

	pc := NewProwlarrCatalog(testLogger())
	pc.SetCachePath(cachePath)
	var gotRaw map[string][]byte
	pc.SetUpdateFunc(func(raw map[string][]byte) { gotRaw = raw })

	source, err := pc.LoadLocal()
	if err != nil {
		t.Fatalf("LoadLocal: %v", err)
	}
	if source != "disk-cache" {
		t.Fatalf("expected source disk-cache, got %q", source)
	}
	if _, ok := gotRaw["testidx"]; !ok {
		t.Fatalf("update func did not receive cached definition; keys: %d", len(gotRaw))
	}
	if pc.LastFetched().IsZero() {
		t.Fatal("LastFetched should reflect the cache file mtime")
	}
}

func TestLoadLocal_CorruptCacheFallsBackToEmbedded(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "prowlarr-definitions.zip")
	if err := os.WriteFile(cachePath, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}

	pc := NewProwlarrCatalog(testLogger())
	pc.SetCachePath(cachePath)
	source, err := pc.LoadLocal()
	if err != nil {
		t.Fatalf("LoadLocal: %v", err)
	}
	if source != "embedded" {
		t.Fatalf("expected fallback to embedded, got %q", source)
	}
}

func TestSaveCache_RoundTrip(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "sub", "prowlarr-definitions.zip")
	zipData := makeZip(t, map[string]string{
		"definitions/v11/testidx.yml": testDefYAML,
	})

	pc := NewProwlarrCatalog(testLogger())
	pc.SetCachePath(cachePath)
	pc.saveCache(zipData)

	pc2 := NewProwlarrCatalog(testLogger())
	pc2.SetCachePath(cachePath)
	source, err := pc2.LoadLocal()
	if err != nil {
		t.Fatalf("LoadLocal after saveCache: %v", err)
	}
	if source != "disk-cache" {
		t.Fatalf("expected disk-cache after saveCache, got %q", source)
	}
}

func TestApply_NotifiesUpdateFuncWithCopy(t *testing.T) {
	pc := NewProwlarrCatalog(testLogger())
	var got map[string][]byte
	pc.SetUpdateFunc(func(raw map[string][]byte) { got = raw })

	raw := map[string][]byte{"testidx": []byte(testDefYAML)}
	pc.apply([]CatalogEntry{{ID: "testidx", Name: "TestIdx", Protocol: "torrent", URLs: []string{"https://testidx.example"}}}, raw, testTime())

	if got == nil {
		t.Fatal("update func not called")
	}
	// Mutating the callback's map must not affect the catalog's copy.
	delete(got, "testidx")
	if _, ok := pc.AllRawYAML()["testidx"]; !ok {
		t.Fatal("update func received the catalog's own map, not a copy")
	}

	// A nil rawYAML keeps existing definitions (individual-fetch path).
	got = nil
	pc.apply([]CatalogEntry{{ID: "testidx", Name: "TestIdx", Protocol: "torrent", URLs: []string{"https://testidx.example"}}}, nil, testTime())
	if _, ok := got["testidx"]; !ok {
		t.Fatal("nil rawYAML should keep existing definitions and still notify")
	}
}
