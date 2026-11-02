package main

import (
	"html/template"
	"regexp"
	"strconv"
	"strings"
)

// The scenario pages use a small part of Markdown: headings, paragraphs,
// pipe tables and lists, with **bold** and `code` inside. toHTML turns that
// part into HTML; anything else stays text, escaped.

var (
	heading   = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	ordered   = regexp.MustCompile(`^\d+[.)]\s+(.*)$`)
	unordered = regexp.MustCompile(`^[-*+]\s+(.*)$`)
	separator = regexp.MustCompile(`^\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?$`)
	strong    = regexp.MustCompile(`\*\*(.+?)\*\*`)
	code      = regexp.MustCompile("`([^`]+)`")
)

// toHTML converts the Markdown md to HTML, and returns the text of its
// first heading as the title.
func toHTML(md string) (body template.HTML, title string) {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var b strings.Builder
	for i := 0; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		switch {
		case line == "":
			i++
		case heading.MatchString(line):
			m := heading.FindStringSubmatch(line)
			if title == "" {
				title = m[2]
			}
			n := strconv.Itoa(len(m[1]))
			b.WriteString("<h" + n + ">" + inline(m[2]) + "</h" + n + ">\n")
			i++
		case isRow(line) && i+1 < len(lines) && separator.MatchString(strings.TrimSpace(lines[i+1])):
			i = table(&b, lines, i)
		case ordered.MatchString(line):
			i = list(&b, lines, i, "ol", ordered)
		case unordered.MatchString(line):
			i = list(&b, lines, i, "ul", unordered)
		default:
			i = paragraph(&b, lines, i)
		}
	}
	return template.HTML(b.String()), title
}

// starts reports whether line begins a block other than a paragraph.
func starts(line string) bool {
	return line == "" || heading.MatchString(line) || isRow(line) ||
		ordered.MatchString(line) || unordered.MatchString(line)
}

func isRow(line string) bool { return strings.HasPrefix(line, "|") }

func paragraph(b *strings.Builder, lines []string, i int) int {
	var text []string
	for ; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if len(text) > 0 && starts(line) {
			break
		}
		text = append(text, line)
	}
	b.WriteString("<p>" + inline(strings.Join(text, " ")) + "</p>\n")
	return i
}

func list(b *strings.Builder, lines []string, i int, tag string, item *regexp.Regexp) int {
	b.WriteString("<" + tag + ">\n")
	for ; i < len(lines); i++ {
		m := item.FindStringSubmatch(strings.TrimSpace(lines[i]))
		if m == nil {
			break
		}
		b.WriteString("<li>" + inline(m[1]) + "</li>\n")
	}
	b.WriteString("</" + tag + ">\n")
	return i
}

// table writes the pipe table starting at lines[i], its header row, and
// returns the line after it.
func table(b *strings.Builder, lines []string, i int) int {
	b.WriteString("<table>\n<thead>\n")
	row(b, lines[i], "th")
	b.WriteString("</thead>\n<tbody>\n")
	for i += 2; i < len(lines) && isRow(strings.TrimSpace(lines[i])); i++ {
		row(b, lines[i], "td")
	}
	b.WriteString("</tbody>\n</table>\n")
	return i
}

func row(b *strings.Builder, line, cell string) {
	b.WriteString("<tr>")
	for _, c := range cells(line) {
		b.WriteString("<" + cell + ">" + inline(c) + "</" + cell + ">")
	}
	b.WriteString("</tr>\n")
}

// cells splits a pipe table row into its trimmed cells.
func cells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	cs := strings.Split(line, "|")
	for i, c := range cs {
		cs[i] = strings.TrimSpace(c)
	}
	return cs
}

// inline escapes s and turns **bold** and `code` into elements.
func inline(s string) string {
	s = template.HTMLEscapeString(s)
	s = code.ReplaceAllString(s, "<code>$1</code>")
	return strong.ReplaceAllString(s, "<strong>$1</strong>")
}
