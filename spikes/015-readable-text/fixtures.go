package main

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"burrow/spikes/readable/corpus"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/unicode/norm"
)

const fixtureDir = "fixtures"

type Fixture struct {
	corpus.Page
	Meta    corpus.Meta
	Raw     []byte // bytes as served
	UTF8    []byte // decoded by Go from Content-Type header, BOM or <meta>
	Charset string // encoding Go chose
	URL     *url.URL
	Visible string // normalized visible text of the whole page
}

func loadFixtures() ([]*Fixture, error) { return loadFixturesOnly("") }

// loadFixturesOnly loads one fixture by id, or all when id is empty.
func loadFixturesOnly(id string) ([]*Fixture, error) {
	var out []*Fixture
	for _, p := range corpus.Pages {
		if id != "" && p.ID != id {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(fixtureDir, p.ID+".html"))
		if err != nil {
			return nil, fmt.Errorf("%s: %w (run go run ./fetch)", p.ID, err)
		}
		m, err := corpus.LoadMeta(fixtureDir, p.ID)
		if err != nil {
			return nil, err
		}
		f := &Fixture{Page: p, Meta: m, Raw: raw}
		f.URL, _ = url.Parse(m.FinalURL)
		f.UTF8, f.Charset = decode(raw, m.ContentType)
		if doc, err := html.Parse(bytes.NewReader(f.UTF8)); err == nil {
			f.Visible = normalize(visibleText(doc))
		}
		out = append(out, f)
	}
	return out, nil
}

// decode is the rule Go would apply before extraction: HTTP charset, then BOM,
// then <meta> prescan, then a UTF-8 check (x/net/html/charset.DetermineEncoding).
func decode(raw []byte, contentType string) ([]byte, string) {
	_, name, _ := charset.DetermineEncoding(raw, contentType)
	r, err := charset.NewReader(bytes.NewReader(raw), contentType)
	if err != nil {
		return raw, "error"
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return raw, "error"
	}
	return b, name
}

var arabicToPersian = strings.NewReplacer("ي", "ی", "ى", "ی", "ك", "ک")

// normalize: NFC, Arabic yeh/kaf to Persian, lower case, whitespace (incl. NBSP)
// collapsed to one space. ZWNJ is kept.
func normalize(s string) string {
	s = norm.NFC.String(s)
	s = arabicToPersian.Replace(s)
	s = strings.ToLower(s)
	var sb strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsSpace(r) || r == ' ' || r == ' ' {
			space = true
			continue
		}
		if r == '­' { // soft hyphen
			continue
		}
		if space && sb.Len() > 0 {
			sb.WriteByte(' ')
		}
		space = false
		sb.WriteRune(r)
	}
	return sb.String()
}
