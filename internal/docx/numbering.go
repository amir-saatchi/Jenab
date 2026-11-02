package docx

import (
	"encoding/xml"
	"strconv"
)

// lvl is one level of a list definition.
type lvl struct {
	format   string // w:numFmt: "bullet", "decimal", "none", …
	start    int
	hasStart bool
}

type abstractNum struct {
	levels    map[int]lvl
	styleLink string // w:numStyleLink: the list is defined by a numbering style
}

type num struct {
	abstract  string
	overrides map[int]lvl // w:lvlOverride
}

type numbering struct {
	abstracts map[string]*abstractNum
	nums      map[string]*num
}

// readNumbering reads numbering.xml.
func readNumbering(t *tokens) (*numbering, error) {
	n := &numbering{abstracts: map[string]*abstractNum{}, nums: map[string]*num{}}
	for {
		tok, err := t.next()
		if tok == nil || err != nil {
			return n, err
		}
		e, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch e.Name.Local {
		case "abstractNum":
			a := &abstractNum{levels: map[int]lvl{}}
			if err := readAbstract(t, a); err != nil {
				return n, err
			}
			if id := attr(e, "abstractNumId"); n.abstracts[id] == nil {
				n.abstracts[id] = a
			}
		case "num":
			nm := &num{overrides: map[int]lvl{}}
			if err := readNum(t, nm); err != nil {
				return n, err
			}
			if id := attr(e, "numId"); n.nums[id] == nil {
				n.nums[id] = nm
			}
		}
	}
}

func readAbstract(t *tokens, a *abstractNum) error {
	return children(t, func(e xml.StartElement) error {
		switch e.Name.Local {
		case "numStyleLink":
			a.styleLink = attr(e, "val")
		case "lvl":
			i, err := strconv.Atoi(attr(e, "ilvl"))
			if err != nil || i < 0 || i > 8 {
				return t.skip()
			}
			var l lvl
			if err := readLvl(t, &l); err != nil {
				return err
			}
			a.levels[i] = l
			return nil
		}
		return t.skip()
	})
}

func readLvl(t *tokens, l *lvl) error {
	return children(t, func(e xml.StartElement) error {
		switch e.Name.Local {
		case "start":
			if v, err := strconv.Atoi(attr(e, "val")); err == nil {
				l.start, l.hasStart = v, true
			}
		case "numFmt":
			l.format = attr(e, "val")
		}
		return t.skip()
	})
}

func readNum(t *tokens, nm *num) error {
	return children(t, func(e xml.StartElement) error {
		switch e.Name.Local {
		case "abstractNumId":
			nm.abstract = attr(e, "val")
		case "lvlOverride":
			i, err := strconv.Atoi(attr(e, "ilvl"))
			if err != nil || i < 0 || i > 8 {
				return t.skip()
			}
			var l lvl
			err = children(t, func(e xml.StartElement) error {
				switch e.Name.Local {
				case "startOverride":
					if v, err := strconv.Atoi(attr(e, "val")); err == nil {
						l.start, l.hasStart = v, true
					}
				case "lvl":
					var d lvl
					if err := readLvl(t, &d); err != nil {
						return err
					}
					l.format = d.format
					if !l.hasStart {
						l.start, l.hasStart = d.start, d.hasStart
					}
					return nil
				}
				return t.skip()
			})
			nm.overrides[i] = l
			return err
		}
		return t.skip()
	})
}

// children calls fn for each child element of the element just started;
// fn must read the child to its end, with t.skip if nothing else.
func children(t *tokens, fn func(xml.StartElement) error) error {
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
			if err := fn(v); err != nil {
				return err
			}
		}
	}
}

// level returns the definition of list numID at ilvl. A list defined by a
// numbering style is followed through that style once.
func (n *numbering) level(numID string, ilvl int, st *styles) (lvl, bool) {
	if n == nil {
		return lvl{}, false
	}
	nm := n.nums[numID]
	if nm == nil {
		return lvl{}, false
	}
	a := n.abstracts[nm.abstract]
	if a != nil && a.styleLink != "" && st != nil {
		if s := st.byID[a.styleLink]; s != nil && s.num != nil {
			if nm2 := n.nums[s.num.id]; nm2 != nil && n.abstracts[nm2.abstract] != nil {
				a = n.abstracts[nm2.abstract]
			}
		}
	}
	var out lvl
	if a != nil {
		out = a.levels[ilvl]
	}
	if o, ok := nm.overrides[ilvl]; ok {
		if o.format != "" {
			out.format = o.format
		}
		if o.hasStart {
			out.start, out.hasStart = o.start, true
		}
	}
	if !out.hasStart {
		out.start = 1
	}
	return out, a != nil || len(nm.overrides) > 0
}
