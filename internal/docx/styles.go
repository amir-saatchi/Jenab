package docx

import (
	"encoding/xml"
	"strconv"
	"strings"
)

// style is a paragraph style from styles.xml: what headings and lists
// need of it.
type style struct {
	name    string // w:name in lower case: "heading 1", "title"; never the ID
	basedOn string
	outline int // w:outlineLvl + 1; 0 if unset
	num     *numRef
}

// numRef is a paragraph's w:numPr.
type numRef struct {
	id    string // "0" turns numbering off
	ilvl  int
	hasLv bool
}

type styles struct {
	byID    map[string]*style
	defPara string // the default paragraph style's ID
}

// readStyles reads the paragraph styles of styles.xml.
func readStyles(t *tokens) (*styles, error) {
	s := &styles{byID: map[string]*style{}}
	for {
		tok, err := t.next()
		if tok == nil || err != nil {
			return s, err
		}
		e, ok := tok.(xml.StartElement)
		if !ok || e.Name.Local != "style" {
			continue
		}
		if typ := attr(e, "type"); typ != "" && typ != "paragraph" && typ != "numbering" {
			if err := t.skip(); err != nil {
				return s, err
			}
			continue
		}
		st := &style{}
		sid := attr(e, "styleId")
		if hasAttr(e, "default") && on(attr(e, "default")) && attr(e, "type") == "paragraph" && s.defPara == "" {
			s.defPara = sid
		}
		if err := readStyle(t, st); err != nil {
			return s, err
		}
		if _, dup := s.byID[sid]; !dup {
			s.byID[sid] = st
		}
	}
}

func readStyle(t *tokens, st *style) error {
	for {
		tok, err := t.next()
		if err != nil {
			return err
		}
		switch v := tok.(type) {
		case nil:
			return broken("%s ends too early", t.part)
		case xml.EndElement:
			return nil
		case xml.StartElement:
			switch v.Name.Local {
			case "name":
				st.name = strings.ToLower(strings.TrimSpace(attr(v, "val")))
			case "basedOn":
				st.basedOn = attr(v, "val")
			case "pPr":
				var pp pPr
				if err := readPPr(t, &pp); err != nil {
					return err
				}
				st.outline, st.num = pp.outline, pp.num
				continue
			}
			if err := t.skip(); err != nil {
				return err
			}
		}
	}
}

// on reads an OOXML on/off value: absent or empty is on.
func on(v string) bool {
	switch strings.ToLower(v) {
	case "0", "false", "off":
		return false
	}
	return true
}

// chain calls fn for the style id and the styles it is based on, until fn
// returns true. A loop or a long chain stops it.
func (s *styles) chain(id string, fn func(*style) bool) {
	for range 32 {
		st := s.byID[id]
		if st == nil || fn(st) {
			return
		}
		id = st.basedOn
	}
}

// heading is the heading level of a paragraph, 1 to 6, or 0. The
// paragraph's own outline level comes first, then its style's name
// ("heading 2", "Title") or outline level, following basedOn. Never the
// style ID: a German Word names the Heading 1 style's ID "berschrift1".
func (s *styles) heading(p *para) int {
	if p.outline > 0 {
		return level(p.outline)
	}
	id := p.style
	if id == "" {
		id = s.defPara
	}
	out := 0
	s.chain(id, func(st *style) bool {
		if st.name == "title" {
			out = 1
			return true
		}
		if n, ok := strings.CutPrefix(st.name, "heading "); ok {
			if v, err := strconv.Atoi(n); err == nil && v >= 1 && v <= 9 {
				out = level(v)
				return true
			}
		}
		if st.outline > 0 {
			out = level(st.outline)
			return true
		}
		return false
	})
	return out
}

// level turns an outline level (1 to 10, where 10 is body text) into a
// Markdown heading level, or 0.
func level(outline int) int {
	if outline >= 1 && outline <= 6 {
		return outline
	}
	return 0
}

// numbering is a paragraph's list, from its own w:numPr or its style's.
func (s *styles) numbering(p *para) *numRef {
	var out *numRef
	if p.num != nil && p.num.id != "" {
		out = &numRef{id: p.num.id, ilvl: p.num.ilvl, hasLv: p.num.hasLv}
	}
	id := p.style
	if id == "" {
		id = s.defPara
	}
	if out == nil || !out.hasLv {
		s.chain(id, func(st *style) bool {
			if st.num == nil {
				return false
			}
			if out == nil {
				if st.num.id == "" {
					return false
				}
				out = &numRef{id: st.num.id, ilvl: st.num.ilvl, hasLv: st.num.hasLv}
			} else if st.num.hasLv {
				out.ilvl, out.hasLv = st.num.ilvl, true
			}
			return out.hasLv
		})
	}
	if out == nil || out.id == "0" {
		return nil
	}
	return out
}
