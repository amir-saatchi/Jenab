package docx

import (
	"bytes"
	"context"
	"encoding/xml"
	"strconv"
	"strings"
)

// The document's content is read into blocks: paragraphs and tables. A
// top-level block is written out as soon as it is read, so only one is
// held at a time.

type block interface{ isBlock() }

type para struct {
	style   string
	outline int // the paragraph's own w:outlineLvl + 1; 0 if unset
	num     *numRef
	merge   bool // its mark is a tracked deletion: it joins the next paragraph
	text    *bytes.Buffer
	after   []block // the text boxes in it, written after it
}

type table struct{ rows []*trow }

type trow struct {
	cells         []*tcell
	before, after int // w:gridBefore, w:gridAfter
	deleted       bool
}

type tcell struct {
	blocks []block
	span   int // w:gridSpan
	vmerge int // 0: none, 1: starts a vertical merge, 2: continues one
}

func (*para) isBlock()  {}
func (*table) isBlock() {}

// field is a complex field (w:fldChar) being read.
type field struct {
	instr  strings.Builder
	result bool          // past w:fldChar separate
	buf    *bytes.Buffer // where its result is written
	start  int           // where its result starts in buf
}

// extractor reads one document.
type extractor struct {
	ctx    context.Context
	p      *pkg
	styles *styles
	num    *numbering
	links  map[string]string // hyperlink relationship ID → URL, of the part being read
	inNote bool              // reading a footnote or endnote
	fields []*field

	notes   []noteKey // in the order they are referenced
	noteNum map[noteKey]int

	md       markdown
	pending  *para // a paragraph whose mark was deleted
	counters map[string]*counter
	cols     [9]int // where each list level's text starts
	colsSet  [9]bool
	list     string // the ID of the list the last item was in
	copied   int    // bytes of table text copied into spanned cells, over the document
}

type noteKey struct {
	end bool // an endnote
	id  string
}

type counter struct {
	vals [9]int
	set  [9]bool
}

func (p *pkg) extract(ctx context.Context, main string) (string, error) {
	e := &extractor{ctx: ctx, p: p, styles: &styles{byID: map[string]*style{}},
		noteNum: map[noteKey]int{}, counters: map[string]*counter{}}
	rels, err := p.rels(main)
	if err != nil {
		return "", err
	}
	var footnotes, endnotes string
	e.links = map[string]string{}
	planned := []string{main}
	for _, r := range rels {
		if !r.external {
			switch r.typ[strings.LastIndexByte(r.typ, '/')+1:] {
			case "styles", "numbering", "footnotes", "endnotes":
				planned = append(planned, r.target)
			}
		}
	}
	if err := p.reserve(planned...); err != nil {
		return "", err
	}
	for _, r := range rels {
		typ := r.typ[strings.LastIndexByte(r.typ, '/')+1:]
		switch {
		case r.external && typ == "hyperlink":
			e.links[r.id] = r.target
		case r.external:
		case typ == "styles":
			if _, err := p.readXML(r.target, func(t *tokens) (err error) { e.styles, err = readStyles(t); return err }); err != nil {
				return "", err
			}
		case typ == "numbering":
			if _, err := p.readXML(r.target, func(t *tokens) (err error) { e.num, err = readNumbering(t); return err }); err != nil {
				return "", err
			}
		case typ == "footnotes":
			footnotes = r.target
		case typ == "endnotes":
			endnotes = r.target
		}
	}
	found, err := p.readXML(main, func(t *tokens) error {
		for {
			tok, err := t.next()
			if tok == nil || err != nil {
				return err
			}
			s, ok := tok.(xml.StartElement)
			switch {
			case !ok, s.Name.Local == "document":
			case s.Name.Local == "body":
				if err := e.blocks(t, e.top, nil); err != nil {
					return err
				}
			default:
				if err := t.skip(); err != nil {
					return err
				}
			}
		}
	})
	if err != nil {
		return "", err
	}
	if !found {
		return "", broken("its document part %s is missing", main)
	}
	e.flush()
	if err := e.writeNotes(footnotes, endnotes); err != nil {
		return "", err
	}
	return e.md.String(), nil
}

// blocks reads the block-level children of the element just started:
// paragraphs, tables and the elements that wrap them. special sees each
// child first; it returns true when it has read it.
func (e *extractor) blocks(t *tokens, emit func(block), special func(xml.StartElement) (bool, error)) error {
	return children(t, func(s xml.StartElement) error {
		if special != nil {
			if done, err := special(s); done || err != nil {
				return err
			}
		}
		switch s.Name.Local {
		case "p":
			p, err := e.para(t)
			if err != nil {
				return err
			}
			emit(p)
			return nil
		case "tbl":
			tb, err := e.table(t)
			if err != nil {
				return err
			}
			emit(tb)
			return nil
		case "sdt":
			return children(t, func(s xml.StartElement) error {
				if s.Name.Local == "sdtContent" {
					return e.blocks(t, emit, nil)
				}
				return t.skip()
			})
		case "customXml", "ins", "moveTo", "smartTag":
			return e.blocks(t, emit, nil)
		case "AlternateContent":
			return alternate(t, func() error { return e.blocks(t, emit, nil) })
		}
		return t.skip() // deletions, section properties, bookmarks and the rest
	})
}

// alternate reads mc:AlternateContent: the first mc:Choice with read, and
// mc:Fallback only when there is no choice, so nothing is read twice.
func alternate(t *tokens, read func() error) error {
	chosen := false
	return children(t, func(s xml.StartElement) error {
		if !chosen && (s.Name.Local == "Choice" || s.Name.Local == "Fallback") {
			chosen = true
			return read()
		}
		return t.skip()
	})
}

// para reads a w:p.
func (e *extractor) para(t *tokens) (*para, error) {
	p := &para{text: &bytes.Buffer{}}
	err := e.inline(t, p, p.text, true)
	return p, err
}

// pPr is what a paragraph's properties say.
type pPr struct {
	style   string
	outline int
	num     *numRef
	merge   bool
}

func readPPr(t *tokens, pp *pPr) error {
	return children(t, func(s xml.StartElement) error {
		switch s.Name.Local {
		case "pStyle":
			pp.style = attr(s, "val")
		case "outlineLvl":
			if v, err := strconv.Atoi(attr(s, "val")); err == nil && v >= 0 && v <= 9 {
				pp.outline = v + 1
			}
		case "numPr":
			n := &numRef{}
			err := children(t, func(s xml.StartElement) error {
				switch s.Name.Local {
				case "numId":
					n.id = attr(s, "val")
				case "ilvl":
					if v, err := strconv.Atoi(attr(s, "val")); err == nil && v >= 0 && v <= 8 {
						n.ilvl, n.hasLv = v, true
					}
				}
				return t.skip()
			})
			pp.num = n
			return err
		case "rPr": // the paragraph mark's run properties
			return children(t, func(s xml.StartElement) error {
				if s.Name.Local == "del" || s.Name.Local == "moveFrom" {
					pp.merge = true
				}
				return t.skip()
			})
		}
		return t.skip() // and w:pPrChange, the properties before a tracked change
	})
}

// inline reads the children of a paragraph, or of an element inside one,
// into buf. Only the text of runs is written, so the markup of unknown
// wrappers is passed through.
func (e *extractor) inline(t *tokens, p *para, buf *bytes.Buffer, top bool) error {
	return children(t, func(s xml.StartElement) error {
		switch s.Name.Local {
		case "pPr":
			if !top {
				return t.skip()
			}
			var pp pPr
			if err := readPPr(t, &pp); err != nil {
				return err
			}
			p.style, p.outline, p.num, p.merge = pp.style, pp.outline, pp.num, pp.merge
			return nil
		case "r":
			return e.run(t, p, buf)
		case "hyperlink":
			var sub bytes.Buffer
			if err := e.inline(t, p, &sub, false); err != nil {
				return err
			}
			url := ""
			if id := attr(s, "id"); id != "" {
				url = e.links[id]
			}
			writeLink(buf, sub.String(), url)
			return nil
		case "fldSimple":
			var sub bytes.Buffer
			if err := e.inline(t, p, &sub, false); err != nil {
				return err
			}
			writeLink(buf, sub.String(), hyperlinkField(attr(s, "instr")))
			return nil
		case "sdt":
			return children(t, func(s xml.StartElement) error {
				if s.Name.Local == "sdtContent" {
					return e.inline(t, p, buf, false)
				}
				return t.skip()
			})
		case "del", "moveFrom", "rPr", "sdtPr", "customXmlPr", "smartTagPr":
			return t.skip() // tracked deletions are left out: changes read as accepted
		case "oMath", "oMathPara":
			buf.WriteString("[equation]")
			return t.skip()
		case "AlternateContent":
			return alternate(t, func() error { return e.inline(t, p, buf, false) })
		}
		// w:ins, w:moveTo, w:smartTag, w:customXml, w:dir, w:bdo and
		// unknown wrappers.
		return e.inline(t, p, buf, false)
	})
}

// run reads a w:r.
func (e *extractor) run(t *tokens, p *para, buf *bytes.Buffer) error {
	hidden := false
	return children(t, func(s xml.StartElement) error {
		switch s.Name.Local {
		case "rPr":
			return children(t, func(s xml.StartElement) error {
				if s.Name.Local == "vanish" && on(attr(s, "val")) {
					hidden = true // hidden text, as Word doesn't show it
				}
				return t.skip() // and w:rPrChange, the properties before a change
			})
		case "fldChar":
			e.fieldChar(attr(s, "fldCharType"), buf)
			return t.skip()
		case "instrText":
			return e.text(t, func(s string) {
				if n := len(e.fields); n > 0 && !e.fields[n-1].result {
					e.fields[n-1].instr.WriteString(s)
				}
			})
		}
		if hidden || e.inInstr() {
			return t.skip()
		}
		switch s.Name.Local {
		case "t":
			return e.text(t, func(s string) { buf.WriteString(s) })
		case "tab", "ptab":
			buf.WriteByte('\t')
		case "br", "cr":
			buf.WriteByte('\n')
		case "noBreakHyphen":
			buf.WriteByte('-')
		case "drawing", "pict", "object":
			return e.graphic(t, p, buf)
		case "footnoteReference", "endnoteReference":
			if !e.inNote {
				buf.WriteString(e.noteMark(noteKey{end: s.Name.Local == "endnoteReference", id: attr(s, "id")}))
			}
		case "AlternateContent":
			return alternate(t, func() error { return e.run(t, p, buf) })
		case "ruby":
			return children(t, func(s xml.StartElement) error {
				if s.Name.Local == "rubyBase" {
					return e.inline(t, p, buf, false)
				}
				return t.skip() // the reading guide
			})
		}
		return t.skip() // w:delText, w:sym, comment marks and the rest
	})
}

// text reads an element's character data.
func (e *extractor) text(t *tokens, fn func(string)) error {
	for {
		tok, err := t.next()
		if err != nil {
			return err
		}
		switch v := tok.(type) {
		case nil:
			return broken("%s ends too early", t.part)
		case xml.CharData:
			fn(string(v))
		case xml.StartElement:
			if err := t.skip(); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

func (e *extractor) inInstr() bool {
	n := len(e.fields)
	return n > 0 && !e.fields[n-1].result
}

// maxFields bounds nested fields; a file that never ends its fields
// can't grow the stack.
const maxFields = 64

// fieldChar follows a complex field: its instruction is not text, and a
// HYPERLINK field's result becomes a link.
func (e *extractor) fieldChar(typ string, buf *bytes.Buffer) {
	n := len(e.fields)
	switch typ {
	case "begin":
		if n < maxFields {
			e.fields = append(e.fields, &field{})
		}
	case "separate":
		if n > 0 {
			f := e.fields[n-1]
			f.result, f.buf, f.start = true, buf, buf.Len()
		}
	case "end":
		if n == 0 {
			return
		}
		f := e.fields[n-1]
		e.fields = e.fields[:n-1]
		if url := hyperlinkField(f.instr.String()); url != "" && f.buf == buf && f.start <= buf.Len() {
			text := string(buf.Bytes()[f.start:])
			buf.Truncate(f.start)
			writeLink(buf, text, url)
		}
	}
}

// hyperlinkField returns the URL of a HYPERLINK field's instruction, or
// "" for any other field and for a link inside the document (\l).
func hyperlinkField(instr string) string {
	args := fieldArgs(instr)
	if len(args) == 0 || !strings.EqualFold(args[0], "HYPERLINK") {
		return ""
	}
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch {
		case strings.EqualFold(a, `\l`):
			return ""
		case strings.EqualFold(a, `\o`), strings.EqualFold(a, `\t`):
			i++
		case strings.HasPrefix(a, `\`):
		default:
			return a
		}
	}
	return ""
}

// fieldArgs splits a field instruction into words and quoted strings.
func fieldArgs(s string) []string {
	var out []string
	for {
		s = strings.TrimLeft(s, " \t\r\n")
		if s == "" {
			return out
		}
		if s[0] == '"' {
			end := strings.IndexByte(s[1:], '"')
			if end < 0 {
				return append(out, s[1:])
			}
			out = append(out, s[1:1+end])
			s = s[2+end:]
			continue
		}
		end := strings.IndexAny(s, " \t\r\n")
		if end < 0 {
			return append(out, s)
		}
		out = append(out, s[:end])
		s = s[end:]
	}
}

// writeLink writes text as a Markdown link to url, or as text when
// there is no URL or no text.
func writeLink(buf *bytes.Buffer, text, url string) {
	url = strings.TrimSpace(url)
	if url == "" || strings.TrimSpace(text) == "" || strings.ContainsAny(text, "\n") {
		buf.WriteString(text)
		return
	}
	// Spaces at the ends stay outside the brackets.
	inner := strings.TrimSpace(text)
	i := strings.Index(text, inner)
	lead, trail := text[:i], text[i+len(inner):]
	inner = strings.NewReplacer(`[`, `\[`, `]`, `\]`).Replace(inner)
	url = strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29", "<", "%3C", ">", "%3E", "\t", "%09").Replace(url)
	buf.WriteString(lead + "[" + inner + "](" + url + ")" + trail)
}

// noteMark is the Markdown mark of a footnote or endnote, numbered in the
// order of the references.
func (e *extractor) noteMark(k noteKey) string {
	n, ok := e.noteNum[k]
	if !ok {
		n = len(e.notes) + 1
		e.noteNum[k] = n
		e.notes = append(e.notes, k)
	}
	return "[^" + strconv.Itoa(n) + "]"
}

// graphic reads a w:drawing, w:pict or w:object. Text boxes in it are
// read as blocks after the paragraph; a picture or chart is
// [image: alt text], an embedded equation [equation].
func (e *extractor) graphic(t *tokens, p *para, buf *bytes.Buffer) error {
	var alt string
	image, box, equation := false, false, false
	for depth := 1; depth > 0; {
		tok, err := t.next()
		if err != nil {
			return err
		}
		switch v := tok.(type) {
		case nil:
			return broken("%s ends too early", t.part)
		case xml.EndElement:
			depth--
		case xml.StartElement:
			depth++
			switch v.Name.Local {
			case "docPr", "shape":
				if alt == "" {
					alt = attr(v, "descr")
				}
				if alt == "" {
					alt = attr(v, "alt")
				}
				if alt == "" {
					alt = attr(v, "title")
				}
			case "pic", "blip", "chart", "imagedata":
				image = true
			case "OLEObject":
				if strings.HasPrefix(strings.ToLower(attr(v, "ProgID")), "equation") {
					equation = true
				}
			case "txbxContent":
				box = true
				depth--
				if err := e.blocks(t, func(b block) { p.after = append(p.after, b) }, nil); err != nil {
					return err
				}
			case "Fallback": // the same shape again, for older readers
				depth--
				if err := t.skip(); err != nil {
					return err
				}
			}
		}
	}
	switch {
	case equation:
		buf.WriteString("[equation]")
	case image || !box:
		alt = strings.Join(strings.Fields(alt), " ")
		if alt == "" {
			buf.WriteString("[image]")
		} else {
			buf.WriteString("[image: " + strings.NewReplacer("[", "(", "]", ")").Replace(alt) + "]")
		}
	}
	return nil
}

// table reads a w:tbl. Rows and cells inside content controls and custom
// XML are read as if they weren't wrapped.
func (e *extractor) table(t *tokens) (*table, error) {
	tb := &table{}
	var rows func() error
	rows = func() error {
		return children(t, func(s xml.StartElement) error {
			switch s.Name.Local {
			case "tr":
				r, err := e.row(t)
				if err == nil {
					tb.rows = append(tb.rows, r)
				}
				return err
			case "sdt":
				return children(t, func(s xml.StartElement) error {
					if s.Name.Local == "sdtContent" {
						return rows()
					}
					return t.skip()
				})
			case "customXml":
				return rows()
			}
			return t.skip()
		})
	}
	return tb, rows()
}

func (e *extractor) row(t *tokens) (*trow, error) {
	r := &trow{}
	var cells func() error
	cells = func() error {
		return children(t, func(s xml.StartElement) error {
			switch s.Name.Local {
			case "trPr":
				return children(t, func(s xml.StartElement) error {
					switch s.Name.Local {
					case "gridBefore":
						r.before = gridCount(attr(s, "val"))
					case "gridAfter":
						r.after = gridCount(attr(s, "val"))
					case "del":
						r.deleted = true // a deleted row, left out
					}
					return t.skip()
				})
			case "tc":
				c, err := e.cell(t)
				if err == nil {
					r.cells = append(r.cells, c)
				}
				return err
			case "sdt":
				return children(t, func(s xml.StartElement) error {
					if s.Name.Local == "sdtContent" {
						return cells()
					}
					return t.skip()
				})
			case "customXml":
				return cells()
			}
			return t.skip()
		})
	}
	return r, cells()
}

func (e *extractor) cell(t *tokens) (*tcell, error) {
	c := &tcell{span: 1}
	err := e.blocks(t, func(b block) { c.blocks = append(c.blocks, b) }, func(s xml.StartElement) (bool, error) {
		if s.Name.Local != "tcPr" {
			return false, nil
		}
		return true, children(t, func(s xml.StartElement) error {
			switch s.Name.Local {
			case "gridSpan":
				c.span = max(1, gridCount(attr(s, "val")))
			case "vMerge":
				if attr(s, "val") == "restart" {
					c.vmerge = 1
				} else {
					c.vmerge = 2
				}
			}
			return t.skip()
		})
	})
	return c, err
}

// gridCount reads a count of grid columns. A hostile count is cut, and
// the table bounds then turn the table into paragraphs.
func gridCount(v string) int {
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return min(n, maxCols+1)
}
