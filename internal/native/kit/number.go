package kit

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// NumberRead is what ReadNumber made of a whole-number field's text.
type NumberRead int

const (
	// NumberSame is the value shown: nothing to save.
	NumberSame NumberRead = iota
	// NumberBad is no whole number, or one below the least.
	NumberBad
	// NumberNew is a value to save.
	NumberNew
)

// ReadNumber reads what was typed in a whole-number field that shows
// shown (the old frontend's readNumber): the same, bad (not a whole
// number, or below min), or the value to save. With optional, an empty
// field is nil. Persian (۱۲) and Arabic (١٢) digits read as 0 to 9.
func ReadNumber(text, shown string, min int, optional bool) (NumberRead, *int) {
	t := latinDigits(strings.TrimSpace(text))
	switch {
	case t == shown:
		return NumberSame, nil
	case t == "":
		if optional {
			return NumberNew, nil
		}
		return NumberBad, nil
	}
	n, err := strconv.Atoi(t)
	if err != nil || n < min {
		return NumberBad, nil
	}
	return NumberNew, &n
}

// latinDigits writes Persian and Arabic-Indic digits as 0 to 9.
func latinDigits(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= '۰' && r <= '۹':
			return '0' + r - '۰'
		case r >= '٠' && r <= '٩':
			return '0' + r - '٠'
		}
		return r
	}, s)
}

// Number is a whole-number field as NumberInput built it in this frame.
// Committed is set in the frame the user committed a new value, Value
// then (nil for an emptied optional field); a failed save calls Failed.
type Number struct {
	Element
	Committed bool
	Value     *int
	st        *numberState
}

type numberState struct {
	text, shown string
	bad         string // the tooltip while the field is marked
	// sent is the value last committed, as text, while sending holds:
	// until the value shown changes or its save fails, so Enter and then
	// leaving the field commit it once.
	sent    string
	sending bool
}

// NumberSpec is what a number field takes: the least value, and whether
// it may be left empty (Optional) to use a default.
type NumberSpec struct {
	Min      int
	Optional bool
}

// NumberInput edits a whole number, value (nil for none), as Settings'
// number input: it commits on Enter or as the focus leaves it, and
// Escape puts the value back. A value below the least goes back, and the
// field is marked "At least N" until the next commit. label names it.
func NumberInput(c *Context, value *int, label string, spec NumberSpec) Number {
	shown := numberText(value)
	box := ui.Box(c)
	st := ui.Local(box, "number", func() numberState { return numberState{text: shown, shown: shown} })
	if st.shown != shown {
		st.shown, st.text, st.sending = shown, shown, false
	}
	n := Number{st: st}
	box.Children(func() {
		in := field(c, ui.TextInput(c, &st.text)).Height(36).Width(96).Label(label).
			TextAlign(ui.End).FontFeatures("tnum").Direction(ui.LeftToRight)
		in.OnTextKey(func(k ui.TextKey) bool {
			if k.Key != ui.KeyEscape || k.Mods != 0 || strings.TrimSpace(k.Text()) == st.shown {
				return false
			}
			k.SetText(st.shown)
			return true
		})
		if in.Submitted() || in.FocusLeft() {
			switch r, v := ReadNumber(st.text, st.shown, spec.Min, spec.Optional); r {
			case NumberBad:
				st.bad, st.text = "At least "+strconv.Itoa(spec.Min), st.shown
			case NumberNew:
				st.bad = ""
				if sent := numberText(v); !st.sending || sent != st.sent {
					st.sent, st.sending = sent, true
					n.Committed, n.Value = true, v
				}
			}
		}
		if st.bad != "" {
			in.Invalid(true).Tooltip(st.bad)
		}
		n.Element = in
	})
	return n
}

// Failed marks the field as not saved and puts back the value shown, as
// the commit's save failed; the caller shows the error.
func (n Number) Failed() {
	if n.st != nil {
		n.st.bad, n.st.text, n.st.sending = "Not saved", n.st.shown, false
	}
}

// numberText is v as a number field shows it: empty for none.
func numberText(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}
