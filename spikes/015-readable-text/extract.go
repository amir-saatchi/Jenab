package main

import (
	"bytes"
	"net/url"
	"strings"

	readeck "codeberg.org/readeck/go-readability/v2"
	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	shiori "github.com/go-shiori/go-readability"
	distiller "github.com/markusmobius/go-domdistiller"
	"github.com/markusmobius/go-trafilatura/v2"
	"golang.org/x/net/html"
)

// Result is what html.extract would return: title, text, links.
type Result struct {
	Title string
	Text  string
	Node  *html.Node // extracted content, for links and the Markdown step
	Err   error
}

type Extractor struct {
	Name string
	// Run gets the page bytes. utf8 is true when Go has already decoded them.
	Run func(b []byte, u *url.URL, utf8 bool) Result
}

var extractors = []Extractor{
	{"trafilatura", func(b []byte, u *url.URL, utf8 bool) Result { return runTrafilatura(b, u, utf8, false) }},
	{"trafilatura+fallback", func(b []byte, u *url.URL, utf8 bool) Result { return runTrafilatura(b, u, utf8, true) }},
	{"readeck v2", runReadeck},
	{"go-shiori", runShiori},
	{"domdistiller", runDistiller},
	{"no extraction (whole page → Markdown)", runWholePage},
}

func runTrafilatura(b []byte, u *url.URL, utf8, fallback bool) Result {
	opts := trafilatura.Options{
		OriginalURL:    u,
		IncludeLinks:   true,
		EnableFallback: fallback,
	}
	if utf8 {
		opts.InputEncoding = "utf-8"
	}
	res, err := trafilatura.Extract(bytes.NewReader(b), opts)
	if err != nil {
		return Result{Err: err}
	}
	text := res.ContentText
	node := res.ContentNode
	if strings.TrimSpace(res.CommentsText) != "" {
		text += "\n\n" + res.CommentsText
		if node != nil && res.CommentsNode != nil {
			wrap := &html.Node{Type: html.ElementNode, Data: "div"}
			wrap.AppendChild(cloneNode(node))
			wrap.AppendChild(cloneNode(res.CommentsNode))
			node = wrap
		}
	}
	return Result{Title: res.Metadata.Title, Text: text, Node: node}
}

func runReadeck(b []byte, u *url.URL, _ bool) Result {
	art, err := readeck.FromReader(bytes.NewReader(b), u)
	if err != nil {
		return Result{Err: err}
	}
	var sb strings.Builder
	if art.Node != nil {
		_ = art.RenderText(&sb)
	}
	return Result{Title: art.Title(), Text: sb.String(), Node: art.Node}
}

func runShiori(b []byte, u *url.URL, _ bool) Result {
	art, err := shiori.FromReader(bytes.NewReader(b), u)
	if err != nil {
		return Result{Err: err}
	}
	return Result{Title: art.Title, Text: art.TextContent, Node: art.Node}
}

func runDistiller(b []byte, u *url.URL, _ bool) Result {
	res, err := distiller.ApplyForReader(bytes.NewReader(b), &distiller.Options{OriginalURL: u, SkipPagination: true})
	if err != nil {
		return Result{Err: err}
	}
	return Result{Title: res.Title, Text: res.Text, Node: res.Node}
}

// runWholePage converts the whole document to Markdown without any main-text
// extraction, to show what an extractor removes. Expects UTF-8 input.
func runWholePage(b []byte, u *url.URL, _ bool) Result {
	doc, err := html.Parse(bytes.NewReader(b))
	if err != nil {
		return Result{Err: err}
	}
	md, err := toMarkdown(doc, u)
	if err != nil {
		return Result{Err: err}
	}
	return Result{Title: docTitle(doc), Text: md, Node: doc}
}

func newConverter() *converter.Converter {
	return converter.NewConverter(converter.WithPlugins(
		base.NewBasePlugin(),
		commonmark.NewCommonmarkPlugin(),
		// Defaults skip any table with a line break in a cell (most Wikipedia tables).
		table.NewTablePlugin(
			table.WithNewlineBehavior(table.NewlineBehaviorPreserve),
			table.WithCellPaddingBehavior(table.CellPaddingBehaviorMinimal),
			table.WithSpanCellBehavior(table.SpanBehaviorMirror),
			table.WithSkipEmptyRows(true),
			table.WithHeaderPromotion(true),
		),
	))
}

func toMarkdown(n *html.Node, u *url.URL) (string, error) {
	if n == nil {
		return "", nil
	}
	opts := []converter.ConvertOptionFunc{}
	if u != nil {
		opts = append(opts, converter.WithDomain(u.Scheme+"://"+u.Host))
	}
	out, err := newConverter().ConvertNode(n, opts...)
	return string(out), err
}

var _ = htmltomarkdown.ConvertNode // plain CommonMark variant, kept for reference

func docTitle(doc *html.Node) string {
	var t string
	var walk func(*html.Node) bool
	walk = func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "title" {
			t = textOf(n)
			return true
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if walk(c) {
				return true
			}
		}
		return false
	}
	walk(doc)
	return strings.TrimSpace(t)
}

func textOf(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

// visibleText is all text outside script/style/template: the reference used
// to check that every phrase really is on the page.
func visibleText(doc *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "template", "svg", "noscript":
				return
			}
		}
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && blockTags[n.Data] {
			sb.WriteByte('\n')
		}
	}
	walk(doc)
	return sb.String()
}

var blockTags = map[string]bool{}

func init() {
	for _, t := range strings.Fields(`address article aside blockquote br caption dd details div dl dt
		fieldset figcaption figure footer form h1 h2 h3 h4 h5 h6 header hr li main nav ol option p pre
		section summary table tbody td tfoot th thead title tr ul select textarea`) {
		blockTags[t] = true
	}
}

// links returns the distinct absolute http(s) links in n, and how many hrefs
// were already absolute in the library output.
func links(n *html.Node, base *url.URL) (out []string, absolute, total int) {
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key != "href" {
					continue
				}
				h := strings.TrimSpace(a.Val)
				if h == "" || strings.HasPrefix(h, "#") || strings.HasPrefix(h, "javascript:") || strings.HasPrefix(h, "mailto:") {
					continue
				}
				total++
				pu, err := url.Parse(h)
				if err != nil {
					continue
				}
				if pu.IsAbs() {
					absolute++
				}
				if base != nil {
					pu = base.ResolveReference(pu)
				}
				if pu.Scheme != "http" && pu.Scheme != "https" {
					continue
				}
				pu.Fragment = ""
				s := pu.String()
				if !seen[s] {
					seen[s] = true
					out = append(out, s)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	if n != nil {
		walk(n)
	}
	return
}

func cloneNode(n *html.Node) *html.Node {
	c := &html.Node{Type: n.Type, DataAtom: n.DataAtom, Data: n.Data, Namespace: n.Namespace}
	c.Attr = append(c.Attr, n.Attr...)
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		c.AppendChild(cloneNode(ch))
	}
	return c
}
