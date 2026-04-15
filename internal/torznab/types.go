// Package torznab implements Torznab/Newznab XML protocol types.
package torznab

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ── Capabilities document (returned for ?t=caps) ────────────────────────────

// Caps is the XML capabilities document.
type Caps struct {
	XMLName    xml.Name       `xml:"caps"`
	Server     CapsServer     `xml:"server"`
	Limits     CapsLimits     `xml:"limits"`
	Searching  CapsSearching  `xml:"searching"`
	Categories CapsCategories `xml:"categories"`
}

type CapsServer struct {
	Title string `xml:"title,attr"`
}

type CapsLimits struct {
	Max     int `xml:"max,attr"`
	Default int `xml:"default,attr"`
}

type CapsSearching struct {
	Search     *CapsSearchType `xml:"search,omitempty"`
	TVSearch   *CapsSearchType `xml:"tv-search,omitempty"`
	MovieSearch *CapsSearchType `xml:"movie-search,omitempty"`
}

type CapsSearchType struct {
	Available       string `xml:"available,attr"`
	SupportedParams string `xml:"supportedParams,attr"`
}

type CapsCategories struct {
	Categories []CapsCategory `xml:"category"`
}

type CapsCategory struct {
	ID     string           `xml:"id,attr"`
	Name   string           `xml:"name,attr"`
	Subcats []CapsCategory  `xml:"subcat,omitempty"`
}

// ── Search results feed (returned for ?t=search etc.) ───────────────────────

// Feed is the Torznab RSS search results feed.
type Feed struct {
	XMLName  xml.Name `xml:"rss"`
	Version  string   `xml:"version,attr"`
	AtomNS   string   `xml:"xmlns:atom,attr,omitempty"`
	TorznabNS string  `xml:"xmlns:torznab,attr"`
	Channel  Channel  `xml:"channel"`
}

type Channel struct {
	Title string `xml:"title"`
	Items []Item `xml:"item"`
}

// Item is a single search result. Custom XML marshaling is used to
// produce <torznab:attr> elements in the correct namespace.
type Item struct {
	Title     string
	GUID      string
	Link      string // detail page URL
	PubDate   string
	Size      int64
	Enclosure *Enclosure
	Attrs     map[string]string // torznab attributes (seeders, peers, etc.)
}

type Enclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

// MarshalXML produces the Torznab-compatible XML for an Item.
func (item Item) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name = xml.Name{Local: "item"}
	if err := e.EncodeToken(start); err != nil {
		return err
	}

	writeElement(e, "title", item.Title)
	writeElement(e, "guid", item.GUID)
	writeElement(e, "link", item.Link)
	if item.PubDate != "" {
		writeElement(e, "pubDate", item.PubDate)
	}
	writeElement(e, "size", strconv.FormatInt(item.Size, 10))

	if item.Enclosure != nil {
		encType := item.Enclosure.Type
		if encType == "" {
			encType = "application/x-bittorrent"
		}
		if err := e.EncodeToken(xml.StartElement{
			Name: xml.Name{Local: "enclosure"},
			Attr: []xml.Attr{
				{Name: xml.Name{Local: "url"}, Value: item.Enclosure.URL},
				{Name: xml.Name{Local: "length"}, Value: strconv.FormatInt(item.Enclosure.Length, 10)},
				{Name: xml.Name{Local: "type"}, Value: encType},
			},
		}); err != nil {
			return err
		}
		if err := e.EncodeToken(xml.EndElement{Name: xml.Name{Local: "enclosure"}}); err != nil {
			return err
		}
	}

	// Torznab attributes
	torznabNS := "http://torznab.com/schemas/2015/feed"
	for name, value := range item.Attrs {
		if err := e.EncodeToken(xml.StartElement{
			Name: xml.Name{Space: torznabNS, Local: "attr"},
			Attr: []xml.Attr{
				{Name: xml.Name{Local: "name"}, Value: name},
				{Name: xml.Name{Local: "value"}, Value: value},
			},
		}); err != nil {
			return err
		}
		if err := e.EncodeToken(xml.EndElement{Name: xml.Name{Space: torznabNS, Local: "attr"}}); err != nil {
			return err
		}
	}

	return e.EncodeToken(xml.EndElement{Name: xml.Name{Local: "item"}})
}

func writeElement(e *xml.Encoder, name, value string) {
	start := xml.StartElement{Name: xml.Name{Local: name}}
	// The outer MarshalXML returns any encoder error via its final
	// EncodeToken call, so these helpers ignore errors by design —
	// a broken encoder will surface at the next checked call.
	_ = e.EncodeToken(start)
	_ = e.EncodeToken(xml.CharData(value))
	_ = e.EncodeToken(xml.EndElement{Name: xml.Name{Local: name}})
}

// ── Builders ────────────────────────────────────────────────────────────────

// NewFeed creates a new search results feed.
func NewFeed(title string, items []Item) Feed {
	return Feed{
		Version:   "2.0",
		TorznabNS: "http://torznab.com/schemas/2015/feed",
		Channel: Channel{
			Title: title,
			Items: items,
		},
	}
}

// NewCaps creates a capabilities document from category mappings and search modes.
func NewCaps(title string, cats []CapsCategory, modes map[string][]string) Caps {
	caps := Caps{
		Server: CapsServer{Title: title},
		Limits: CapsLimits{Max: 100, Default: 50},
		Categories: CapsCategories{Categories: cats},
	}

	if params, ok := modes["search"]; ok {
		caps.Searching.Search = &CapsSearchType{
			Available: "yes", SupportedParams: joinParams(params),
		}
	}
	if params, ok := modes["tv-search"]; ok {
		caps.Searching.TVSearch = &CapsSearchType{
			Available: "yes", SupportedParams: joinParams(params),
		}
	}
	if params, ok := modes["movie-search"]; ok {
		caps.Searching.MovieSearch = &CapsSearchType{
			Available: "yes", SupportedParams: joinParams(params),
		}
	}

	return caps
}

func joinParams(params []string) string {
	out := ""
	for i, p := range params {
		if i > 0 {
			out += ","
		}
		out += p
	}
	return out
}

// FormatPubDate formats a time string for Torznab pubDate field (RFC1123Z).
// Handles RFC3339 dates, relative dates ("3 days", "1 week, 2 days"), and "now".
func FormatPubDate(dateStr string) string {
	dateStr = strings.ReplaceAll(dateStr, "\u00a0", " ") // normalize nbsp
	dateStr = strings.TrimSpace(dateStr)
	if dateStr == "" {
		return ""
	}

	// RFC3339 (absolute)
	if t, err := time.Parse(time.RFC3339, dateStr); err == nil {
		return t.Format(time.RFC1123Z)
	}

	// Already RFC1123Z
	if t, err := time.Parse(time.RFC1123Z, dateStr); err == nil {
		return t.Format(time.RFC1123Z)
	}

	// "now" or "today"
	lower := strings.ToLower(dateStr)
	if lower == "now" || lower == "today" {
		return time.Now().UTC().Format(time.RFC1123Z)
	}

	// Relative date: "3 days", "1 week, 2 days", "1 month", etc.
	if t, ok := parseRelativeDate(lower); ok {
		return t.Format(time.RFC1123Z)
	}

	// Return as-is if nothing matched
	return dateStr
}

// parseRelativeDate converts strings like "3 days", "1 week, 2 days",
// "1 month" into an absolute time by subtracting from now.
func parseRelativeDate(s string) (time.Time, bool) {
	now := time.Now().UTC()
	total := time.Duration(0)
	matched := false

	re := regexp.MustCompile(`(\d+)\s*(year|month|week|day|hour|min(?:ute)?)s?`)
	for _, m := range re.FindAllStringSubmatch(s, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		matched = true
		switch m[2] {
		case "year":
			now = now.AddDate(-n, 0, 0)
		case "month":
			now = now.AddDate(0, -n, 0)
		case "week":
			total += time.Duration(n) * 7 * 24 * time.Hour
		case "day":
			total += time.Duration(n) * 24 * time.Hour
		case "hour":
			total += time.Duration(n) * time.Hour
		case "min", "minute":
			total += time.Duration(n) * time.Minute
		}
	}

	if !matched {
		return time.Time{}, false
	}
	return now.Add(-total), true
}

// ResultToItem converts a scraper SearchResult to a Torznab Item.
func ResultToItem(r interface{ GetTitle() string }, baseURL string) Item {
	// This is a placeholder — the actual conversion is done in the handler
	return Item{}
}

// SearchResultToItem converts a scraper.SearchResult to a torznab.Item.
func SearchResultToItem(title, guid, link, pubDate string, size int64, downloadURL string, seeders, leechers int, dvf, uvf float64, category string) Item {
	attrs := map[string]string{
		"category": category,
		"size":     strconv.FormatInt(size, 10),
	}
	if seeders > 0 {
		attrs["seeders"] = strconv.Itoa(seeders)
	}
	if leechers > 0 {
		attrs["peers"] = strconv.Itoa(leechers)
	}
	if dvf != 1.0 {
		attrs["downloadvolumefactor"] = fmt.Sprintf("%.1f", dvf)
	}
	if uvf != 1.0 {
		attrs["uploadvolumefactor"] = fmt.Sprintf("%.1f", uvf)
	}

	var enc *Enclosure
	if downloadURL != "" {
		enc = &Enclosure{
			URL:    downloadURL,
			Length: size,
			Type:   "application/x-bittorrent",
		}
	}

	return Item{
		Title:     title,
		GUID:      guid,
		Link:      link,
		PubDate:   FormatPubDate(pubDate),
		Size:      size,
		Enclosure: enc,
		Attrs:     attrs,
	}
}
