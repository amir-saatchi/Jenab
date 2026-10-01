package main

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

func scriptShare(b []byte) float64 {
	doc, err := html.Parse(bytes.NewReader(b))
	if err != nil || len(b) == 0 {
		return 0
	}
	n := 0
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.ElementNode && x.Data == "script" {
			n += len(textOf(x))
			return
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return float64(n) / float64(len(b))
}

// selectorCase: html.extract with mode=selector. Want is a phrase the joined
// text of the matched elements must contain.
type selectorCase struct {
	ID       string
	Selector string
	Want     string
}

var selectorCases = []selectorCase{
	{"wiki-en", "#firstHeading", "Bitcoin"},
	{"wiki-en", "table.infobox th", "Original author"},
	{"wiki-fa", "#firstHeading", "بیت‌کوین"},
	{"table-ecb-rates", "table.forextable tbody tr td.currency", "USD"},
	{"table-ecb-rates", "table.forextable tr:has(td.currency:contains('JPY')) td.spot", ""},
	{"table-govuk-holidays", "#england-and-wales table caption", "England and Wales"},
	{"table-wiki-population", "table.wikitable tbody tr:nth-child(3) td", ""},
	{"news-bbc-fa", "h1", ""},
	{"news-tagesschau-de", "h1", ""},
	{"docs-go-effective", "h2#names", "Names"},
	{"github-readme", "article.markdown-body h2", ""},
	{"forum-hn", ".commtext", ""},
	{"large-whatwg-html", "h2", ""},
}

var badSelectors = []string{"div[", "a:has(", "p >> span", "::before", "#", "td:nth-child(x)"}

func selectorSection(fx []*Fixture, p func(string, ...any)) {
	p("## 9. Selector mode (goquery + cascadia)")
	p("")
	p("Each selector is compiled with `cascadia.Compile` (so errors are visible) and run with goquery on the Go-decoded page. Want = phrase the matched text must contain (empty = only needs a match). Time includes parsing the page.")
	p("")
	p("| Page | Selector | Matches | Want found | First match (60 chars) | Time |")
	p("|---|---|---|---|---|---|")
	byID := map[string]*Fixture{}
	for _, f := range fx {
		byID[f.ID] = f
	}
	for _, c := range selectorCases {
		f := byID[c.ID]
		if f == nil {
			continue
		}
		t := time.Now()
		sel, err := cascadia.Compile(c.Selector)
		if err != nil {
			p("| %s | `%s` | compile error: %v | | | |", c.ID, c.Selector, err)
			continue
		}
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(f.UTF8))
		if err != nil {
			p("| %s | `%s` | parse error: %v | | | |", c.ID, c.Selector, err)
			continue
		}
		m := doc.FindMatcher(sel)
		var texts []string
		m.Each(func(_ int, s *goquery.Selection) { texts = append(texts, strings.TrimSpace(s.Text())) })
		el := time.Since(t)
		joined := normalize(strings.Join(texts, " "))
		found := "—"
		if c.Want != "" {
			found = "no"
			if strings.Contains(joined, normalize(c.Want)) {
				found = "yes"
			}
		}
		first := ""
		if len(texts) > 0 {
			first = normalize(texts[0])
			if r := []rune(first); len(r) > 60 {
				first = string(r[:60]) + "…"
			}
		}
		p("| %s | `%s` | %d | %s | %s | %s |", c.ID, strings.ReplaceAll(c.Selector, "|", "\\|"), m.Length(), found, strings.ReplaceAll(first, "|", "\\|"), dur(float64(el.Nanoseconds())))
	}
	p("")
	p("Invalid selectors: `cascadia.Compile` error vs what `goquery.Find` does with the same string.")
	p("")
	p("| Selector | cascadia.Compile | goquery Find matches |")
	p("|---|---|---|")
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader("<div><p>x</p></div>"))
	for _, s := range badSelectors {
		_, err := cascadia.Compile(s)
		e := "ok"
		if err != nil {
			e = "error: " + strings.ReplaceAll(err.Error(), "|", "\\|")
		}
		n := "panic"
		func() {
			defer func() { _ = recover() }()
			n = fmt.Sprint(doc.Find(s).Length())
		}()
		p("| `%s` | %s | %s |", s, e, n)
	}
	p("")
}
