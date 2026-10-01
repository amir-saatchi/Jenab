// Package corpus holds the fixture list shared by the fetcher and the scorer.
package corpus

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Page is one fixture. Fetched pages have a URL; derived pages are built by the
// fetcher from another fixture (legacy-encoding copies).
type Page struct {
	ID    string
	URL   string
	Group string // news, wiki, docs, blog, forum, table, banner, js, large, legacy
	Lang  string // en, de, fa, ja

	// Derived fixtures: source fixture, target encoding, and whether a
	// <meta charset> is written into the copy (false = HTTP header only).
	From     string
	Encoding string
	Meta     bool

	Title   string     // substring the extracted title must contain
	Must    []string   // phrases from the main text
	MustNot []string   // boilerplate phrases (menu, footer, cookie text)
	Row     [][]string // table pages: groups of cells that must share one line
	Notes   string
}

// Meta is written next to every fixture as <id>.json.
type Meta struct {
	ID            string `json:"id"`
	URL           string `json:"url"`
	FinalURL      string `json:"final_url"`
	Status        int    `json:"status"`
	FetchedAt     string `json:"fetched_at"`
	ContentType   string `json:"content_type"`
	HeaderCharset string `json:"header_charset"`
	MetaCharset   string `json:"meta_charset"`
	Bytes         int    `json:"bytes"`
	Robots        string `json:"robots"`
	DerivedFrom   string `json:"derived_from,omitempty"`
	Note          string `json:"note,omitempty"`
}

func LoadMeta(dir, id string) (Meta, error) {
	var m Meta
	b, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(b, &m)
	return m, err
}

func SaveMeta(dir string, m Meta) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, m.ID+".json"), append(b, '\n'), 0o644)
}
