package v1

import (
	"encoding/xml"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestXMLError_EscapesSpecialCharacters is a regression test for the
// "XML syntax error on line 2" failure observed downstream in Pilot.
//
// xmlError used to fmt-interpolate the description straight into an XML
// attribute. When the description embedded quotes — as it does for
// `fmt.Sprintf("No catalog definition found for indexer %q ...", name)`,
// which renders `"1337x"` — the inner quotes closed the attribute early and
// produced invalid XML. Pilot's torznab decoder then reported a cryptic
// "attribute name without = in element" / "invalid XML name: 1337x" instead
// of surfacing the real "No catalog definition found" message.
//
// The fix routes the description through encoding/xml so it is escaped. This
// test pins that contract: the response must be well-formed XML that a strict
// decoder accepts, and the description must round-trip verbatim.
func TestXMLError_EscapesSpecialCharacters(t *testing.T) {
	cases := []struct {
		name string
		desc string
	}{
		{"quotes", `No catalog definition found for indexer "1337x" — it may have been removed from Prowlarr`},
		{"ampersand", "search failed: rate & limit exceeded"},
		{"angle brackets", "unexpected <html> response from upstream"},
		{"plain", "Indexer not found"},
	}

	h := &TorznabHandler{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.xmlError(rec, 100, tc.desc)

			body := rec.Body.Bytes()

			// 1. A strict decoder must accept the whole document — this is
			//    exactly what failed before the fix.
			dec := xml.NewDecoder(strings.NewReader(string(body)))
			for {
				_, err := dec.Token()
				if err != nil {
					if err.Error() == "EOF" {
						break
					}
					t.Fatalf("response is not well-formed XML: %v\nbody:\n%s", err, body)
				}
			}

			// 2. The description must round-trip unchanged after unescaping.
			var got torznabError
			if err := xml.Unmarshal(body, &got); err != nil {
				t.Fatalf("unmarshal error doc: %v\nbody:\n%s", err, body)
			}
			if got.Code != 100 {
				t.Errorf("code = %d, want 100", got.Code)
			}
			if got.Description != tc.desc {
				t.Errorf("description round-trip mismatch:\n got: %q\nwant: %q", got.Description, tc.desc)
			}
		})
	}
}
