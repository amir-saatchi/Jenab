package web

import (
	"bytes"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/text/unicode/norm"
)

// maxTitle bounds a page title, in characters.
const maxTitle = 300

// readable is the main content of a decoded HTML page as Markdown, and its
// title (SPEC 3.7, SPIKE-015).
func readable(page []byte, u *url.URL) (title, text string, err error) {
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return "", "", err
	}
	art, err := readability.FromDocument(doc, u)
	if err != nil {
		return "", "", err
	}
	title = art.Title()
	if art.Node != nil {
		dropData(art.Node)
		boundTables(art.Node)
		md, err := newConverter().ConvertNode(art.Node, converter.WithDomain(u.Scheme+"://"+u.Host))
		if err != nil {
			return "", "", err
		}
		text = tidy(string(md))
	}
	return cleanTitle(title), text, nil
}

// newConverter keeps tables that have line breaks in cells; the defaults
// skip them, which drops most Wikipedia tables (SPIKE-015).
func newConverter() *converter.Converter {
	return converter.NewConverter(converter.WithPlugins(
		base.NewBasePlugin(),
		commonmark.NewCommonmarkPlugin(),
		table.NewTablePlugin(
			table.WithNewlineBehavior(table.NewlineBehaviorPreserve),
			table.WithCellPaddingBehavior(table.CellPaddingBehaviorMinimal),
			table.WithSpanCellBehavior(table.SpanBehaviorMirror),
			table.WithSkipEmptyRows(true),
			table.WithHeaderPromotion(true),
		),
	))
}

// dropData removes data: URLs, which can be megabytes of base64 (an
// inlined screenshot): such an image becomes its alt text, such a link
// its text.
func dropData(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode {
			switch {
			case c.Data == "img" && isData(attr(c, "src")):
				if alt := strings.TrimSpace(attr(c, "alt")); alt != "" {
					n.InsertBefore(&html.Node{Type: html.TextNode, Data: alt}, c)
				}
				n.RemoveChild(c)
			case c.Data == "a" && isData(attr(c, "href")):
				c.Attr = without(c.Attr, "href")
				dropData(c)
			default:
				dropData(c)
			}
		}
		c = next
	}
}

func isData(s string) bool {
	s = strings.TrimSpace(s)
	return len(s) >= 5 && strings.EqualFold(s[:5], "data:")
}

func without(as []html.Attribute, key string) []html.Attribute {
	var out []html.Attribute
	for _, a := range as {
		if a.Key != key {
			out = append(out, a)
		}
	}
	return out
}

// Bounds for a Markdown table. The table plugin pads every row to the
// widest one and copies a spanning cell into each cell it covers, so a
// small hostile table (rowspan=999999) can take gigabytes.
const (
	maxCells     = 250_000 // rows × columns, spans counted
	maxCols      = 1000
	maxSpanCells = 50_000  // cells covered by spans
	maxSpanText  = 2 << 20 // bytes copied into them
)

// boundTables turns each table over the bounds into plain blocks, which
// keeps its text.
func boundTables(root *html.Node) {
	var tables, over []*html.Node
	walkElements(root, func(n *html.Node) bool {
		if n.Data == "table" {
			tables = append(tables, n)
		}
		return true
	})
	if len(tables) == 0 {
		return
	}
	sizes := cellText(root)
	for _, t := range tables {
		if !tableFits(t, sizes) {
			over = append(over, t)
		}
	}
	for _, t := range over {
		flatten(t)
	}
}

// tableFits measures t's own rows as the table plugin lays them out: a
// row is as wide as its cells plus the cells reaching down into it from
// above. Nested tables sit in cells, so they are measured on their own.
func tableFits(t *html.Node, sizes map[*html.Node]int) bool {
	type down struct{ row, rows, cols int }
	var widths []int
	var downs []down
	spanCells, spanText := 0, 0
	walkElements(t, func(n *html.Node) bool {
		switch {
		case n == t:
			return true
		case n.Data != "tr":
			return true
		}
		w := 0
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode || c.Data != "td" && c.Data != "th" {
				continue
			}
			rs, cs := span(c, "rowspan"), span(c, "colspan")
			w += cs
			if rs > 1 {
				downs = append(downs, down{len(widths), rs, cs})
			}
			if a := rs * cs; a > 1 {
				spanCells += a - 1
				spanText += (a - 1) * (sizes[c] + 1)
			}
		}
		widths = append(widths, w)
		return false
	})
	if spanCells > maxSpanCells || spanText > maxSpanText {
		return false
	}
	rows := len(widths)
	for _, d := range downs {
		rows = max(rows, d.row+d.rows)
	}
	if rows > maxCells {
		return false
	}
	extra := make([]int, rows+1)
	for _, d := range downs {
		extra[d.row+1] += d.cols
		extra[d.row+d.rows] -= d.cols
	}
	cols, add := 0, 0
	for y := range rows {
		add += extra[y]
		w := add
		if y < len(widths) {
			w += widths[y]
		}
		cols = max(cols, w)
	}
	return cols <= maxCols && rows*cols <= maxCells
}

// cellText is the text size of every table cell, from one walk.
func cellText(root *html.Node) map[*html.Node]int {
	sizes := map[*html.Node]int{}
	var walk func(*html.Node) int
	walk = func(n *html.Node) int {
		size := 0
		if n.Type == html.TextNode {
			size = len(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			size += walk(c)
		}
		if n.Type == html.ElementNode && (n.Data == "td" || n.Data == "th") {
			sizes[n] = size
		}
		return size
	}
	walk(root)
	return sizes
}

// span is a rowspan or colspan as the table plugin reads it, at most
// maxCells so products stay small.
func span(n *html.Node, key string) int {
	v, err := strconv.Atoi(attr(n, key))
	if err != nil || v < 1 {
		return 1
	}
	return min(v, maxCells)
}

// flatten makes t's own table elements plain blocks: rows become
// paragraphs, cells spans with a space after each.
func flatten(t *html.Node) {
	walkElements(t, func(n *html.Node) bool {
		if n != t && n.Data == "table" {
			return false
		}
		switch n.Data {
		case "table", "thead", "tbody", "tfoot", "caption", "colgroup", "col":
			n.Data, n.DataAtom = "div", atom.Div
		case "tr":
			n.Data, n.DataAtom = "p", atom.P
		case "td", "th":
			n.Data, n.DataAtom = "span", atom.Span
			n.AppendChild(&html.Node{Type: html.TextNode, Data: " "})
		}
		n.Attr = nil
		return true
	})
}

// walkElements calls f on n and the elements under it, going into an
// element's children when f returns true.
func walkElements(n *html.Node, f func(*html.Node) bool) {
	if n.Type == html.ElementNode && !f(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkElements(c, f)
	}
}

var blankLines = regexp.MustCompile(`\n{3,}`)

func tidy(s string) string {
	return strings.TrimSpace(blankLines.ReplaceAllString(s, "\n\n"))
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// cleanTitle is NFC with the ZWNJ kept, on one line, at most maxTitle
// characters.
func cleanTitle(s string) string {
	s = strings.Join(strings.Fields(norm.NFC.String(s)), " ")
	if r := []rune(s); len(r) > maxTitle {
		s = string(r[:maxTitle-1]) + "…"
	}
	return s
}
