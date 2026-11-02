package docx

import (
	"encoding/xml"
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// markdown is the text written so far. Blocks are apart by a blank line,
// list items and notes by a line break.
type markdown struct {
	b    strings.Builder
	last kind
}

type kind int

const (
	kindPara kind = iota + 1
	kindItem
	kindNote
)

func (m *markdown) block(k kind, s string) {
	if m.b.Len() > 0 {
		if k == m.last && k != kindPara {
			m.b.WriteString("\n")
		} else {
			m.b.WriteString("\n\n")
		}
	}
	m.b.WriteString(s)
	m.last = k
}

func (m *markdown) String() string {
	if m.b.Len() == 0 {
		return ""
	}
	return m.b.String() + "\n"
}

// Bounds for a Markdown table, as for web pages (SPEC 3.7, SPIKE-015). A
// table over them becomes paragraphs, which keep its text.
const (
	maxCells     = 250_000 // rows × columns, spans counted
	maxCols      = 1000
	maxSpanCells = 50_000  // cells covered by spans
	maxSpanText  = 2 << 20 // bytes copied into them, per table
	maxCopied    = 8 << 20 // and over the whole document
)

// top writes a block of the body. A paragraph whose mark was deleted
// waits for the next one, as the change reads accepted.
func (e *extractor) top(b block) {
	switch v := b.(type) {
	case *para:
		if e.pending != nil {
			e.pending.text.Write(v.text.Bytes())
			v.text = e.pending.text
			v.after = append(e.pending.after, v.after...)
			e.pending = nil
		}
		if v.merge {
			e.pending = v
			return
		}
		e.writePara(v)
	case *table:
		e.flush()
		e.writeTable(v)
	}
}

// flush writes a paragraph left waiting.
func (e *extractor) flush() {
	if p := e.pending; p != nil {
		e.pending = nil
		e.writePara(p)
	}
}

// clean is a paragraph's text as it is written: NFC, which keeps the
// ZWNJ, each line trimmed, without empty lines at the ends.
func clean(s string) string {
	s = norm.NFC.String(s)
	lines := strings.Split(s, "\n")
	out := lines[:0]
	for _, l := range lines {
		out = append(out, strings.Trim(l, " \t "))
	}
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

func (e *extractor) writePara(p *para) {
	text := clean(p.text.String())
	if h := e.styles.heading(p); h > 0 {
		if text != "" {
			e.endList()
			e.md.block(kindPara, strings.Repeat("#", h)+" "+strings.Join(strings.Fields(text), " "))
		}
	} else if marker, ilvl, list, ok := e.item(p); ok {
		if text != "" {
			if ilvl == 0 && e.list != "" && list != e.list {
				e.endList()
				e.md.last = 0 // another list starts: a blank line before it
			}
			e.list = list
			ind := 0
			for j := ilvl - 1; j >= 0; j-- {
				if e.colsSet[j] {
					ind = e.cols[j]
					break
				}
			}
			e.cols[ilvl], e.colsSet[ilvl] = ind+len(marker)+1, true
			for j := ilvl + 1; j < len(e.colsSet); j++ {
				e.colsSet[j] = false
			}
			pad := strings.Repeat(" ", ind)
			body := strings.ReplaceAll(text, "\n", "\n"+pad+strings.Repeat(" ", len(marker)+1))
			e.md.block(kindItem, pad+marker+" "+body)
		}
	} else if text != "" {
		e.endList()
		e.md.block(kindPara, text)
	}
	for _, b := range p.after {
		e.top(b)
	}
}

func (e *extractor) endList() { e.colsSet, e.list = [9]bool{}, "" }

// item returns a list paragraph's marker, "-" for bullets and the number
// with a "." for any numbered list, its level and its list's ID. ok is
// false when the paragraph is not in a list.
func (e *extractor) item(p *para) (marker string, ilvl int, list string, ok bool) {
	n := e.styles.numbering(p)
	if n == nil {
		return "", 0, "", false
	}
	l, ok := e.num.level(n.id, n.ilvl, e.styles)
	if !ok || l.format == "none" {
		return "", 0, "", false
	}
	ilvl = n.ilvl
	c := e.counters[n.id]
	if c == nil {
		c = &counter{}
		e.counters[n.id] = c
	}
	if c.set[ilvl] {
		c.vals[ilvl]++
	} else {
		c.vals[ilvl], c.set[ilvl] = l.start, true
	}
	for j := ilvl + 1; j < len(c.set); j++ {
		c.set[j] = false
	}
	if l.format == "bullet" || l.format == "" {
		return "-", ilvl, n.id, true
	}
	return strconv.Itoa(c.vals[ilvl]) + ".", ilvl, n.id, true
}

// cellText is the text of a cell's blocks on one line: paragraphs apart
// by <br>, nested tables flattened, pipes escaped.
func (e *extractor) cellText(blocks []block, sep string) string {
	var parts []string
	var walk func([]block)
	walk = func(bs []block) {
		for _, b := range bs {
			switch v := b.(type) {
			case *para:
				text := clean(v.text.String())
				if marker, _, _, ok := e.item(v); ok && text != "" && e.styles.heading(v) == 0 {
					text = marker + " " + text
				}
				if text != "" {
					parts = append(parts, text)
				}
				walk(v.after)
			case *table:
				for _, r := range v.rows {
					if r.deleted {
						continue
					}
					for _, c := range r.cells {
						if c.vmerge != 2 {
							walk(c.blocks)
						}
					}
				}
			}
		}
	}
	walk(blocks)
	s := strings.Join(parts, "\n")
	if sep == "<br>" {
		s = strings.ReplaceAll(s, "|", `\|`)
	}
	return strings.ReplaceAll(s, "\n", sep)
}

// writeTable writes a table as a Markdown table with the first row as
// its header. Spanned cells are copied into each cell they cover. A table
// over the bounds is written as paragraphs.
func (e *extractor) writeTable(tb *table) {
	e.endList()
	var rows []*trow
	cols := 0
	for _, r := range tb.rows {
		if r.deleted {
			continue
		}
		rows = append(rows, r)
		w := r.before + r.after
		for _, c := range r.cells {
			w += c.span
		}
		cols = max(cols, w)
	}
	if len(rows) == 0 || cols == 0 {
		return
	}
	if cols > maxCols || len(rows)*cols > maxCells {
		e.tableAsParagraphs(rows)
		return
	}
	grid := make([][]string, len(rows))
	spanCells, spanText := 0, 0
	for y, r := range rows {
		grid[y] = make([]string, cols)
		x := r.before
		for _, c := range r.cells {
			var text string
			if c.vmerge == 2 && y > 0 && x < cols {
				text = grid[y-1][x] // a merge goes on: the text above, mirrored
				spanCells += c.span
				spanText += c.span * (len(text) + 1)
			} else {
				text = e.cellText(c.blocks, "<br>")
				spanCells += c.span - 1
				spanText += (c.span - 1) * (len(text) + 1)
			}
			if spanCells > maxSpanCells || spanText > maxSpanText || e.copied+spanText > maxCopied {
				e.tableAsParagraphs(rows)
				return
			}
			for k := 0; k < c.span && x < cols; k++ {
				grid[y][x] = text
				x++
			}
		}
	}
	e.copied += spanText
	var b strings.Builder
	line := func(cells []string) {
		b.WriteString("|")
		for _, c := range cells {
			b.WriteString(" " + c + " |")
		}
	}
	line(grid[0])
	b.WriteString("\n|")
	for range cols {
		b.WriteString(" --- |")
	}
	for _, r := range grid[1:] {
		b.WriteString("\n")
		line(r)
	}
	e.md.block(kindPara, b.String())
}

// tableAsParagraphs writes each cell's text as a paragraph.
func (e *extractor) tableAsParagraphs(rows []*trow) {
	for _, r := range rows {
		for _, c := range r.cells {
			if c.vmerge == 2 {
				continue
			}
			if text := e.cellText(c.blocks, "\n"); text != "" {
				e.md.block(kindPara, text)
			}
		}
	}
}

// writeNotes reads the footnotes and endnotes that the text refers to
// and lists them at the end, as [^n]: text.
func (e *extractor) writeNotes(footnotes, endnotes string) error {
	if len(e.notes) == 0 {
		return nil
	}
	texts := map[noteKey]string{}
	e.inNote, e.fields = true, nil
	for _, part := range []struct {
		name string
		end  bool
	}{{footnotes, false}, {endnotes, true}} {
		if part.name == "" {
			continue
		}
		rels, err := e.p.rels(part.name)
		if err != nil {
			return err
		}
		e.links = map[string]string{}
		for _, r := range rels {
			if r.external && strings.HasSuffix(r.typ, "/hyperlink") {
				e.links[r.id] = r.target
			}
		}
		_, err = e.p.readXML(part.name, func(t *tokens) error {
			for {
				tok, err := t.next()
				if tok == nil || err != nil {
					return err
				}
				s, ok := tok.(xml.StartElement)
				if !ok || s.Name.Local != "footnote" && s.Name.Local != "endnote" {
					continue
				}
				k := noteKey{end: part.end, id: attr(s, "id")}
				if _, want := e.noteNum[k]; !want {
					if err := t.skip(); err != nil {
						return err
					}
					continue
				}
				var bs []block
				if err := e.blocks(t, func(b block) { bs = append(bs, b) }, nil); err != nil {
					return err
				}
				if _, dup := texts[k]; !dup {
					texts[k] = e.cellText(bs, "\n    ")
				}
			}
		})
		if err != nil {
			return err
		}
	}
	for i, k := range e.notes {
		e.md.block(kindNote, strings.TrimSpace("[^"+strconv.Itoa(i+1)+"]: "+texts[k]))
	}
	return nil
}
