package kit

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

// FilterControl is the control a filter shows (SPEC 5.2).
type FilterControl string

const (
	FilterText     FilterControl = "text"
	FilterNumber   FilterControl = "number"
	FilterDate     FilterControl = "date"
	FilterSelect   FilterControl = "select"
	FilterCheckbox FilterControl = "checkbox"
)

// FilterSpec is a filter's control: its label, its kind, whether it may
// be cleared, and a select's options.
type FilterSpec struct {
	// Label is shown with the control and names it.
	Label   string
	Control FilterControl
	// Required keeps the filter from being cleared: an emptied text or
	// number goes back to its value, a date and a select offer no "Any",
	// and a check box binds false when unchecked.
	Required bool
	// Options are a select's values, each shown as it is.
	Options []string
}

// FilterField is a filter as Filter built it in this frame. Changed is
// set in the frame the user gave it a new value.
type FilterField struct {
	Element
	Changed bool
}

// Filter shows a filter's control with its label above it, editing
// *value, the query parameter it binds. The values are plain: a string
// for text and select, an int64 or a float64 for a number, an ISO date
// (2026-11-20) for a date, a bool for a check box, and nil, SQL's NULL,
// for a cleared optional filter. Text and numbers commit on Enter or as
// the focus leaves them, and Escape puts the value back; a date opens a
// calendar, and a choice in it commits.
func Filter(c *Context, spec FilterSpec, value *any) FilterField {
	var f FilterField
	side := ui.LeftToRight
	if RightToLeft(spec.Label) {
		side = ui.RightToLeft
	}
	if spec.Control == FilterCheckbox {
		f.Element = ui.Row(c).Height(36).AlignItems(ui.Center).Direction(side).Children(func() {
			f.Changed = filterCheck(c, spec, value)
		})
		return f
	}
	bad := ""
	f.Element = Field(c, spec.Label, "", "", func() {
		switch spec.Control {
		case FilterNumber:
			f.Changed, bad = filterText(c, spec, value, true)
		case FilterDate:
			f.Changed = filterDate(c, spec, value)
		case FilterSelect:
			f.Changed = filterSelect(c, spec, value)
		default:
			f.Changed, bad = filterText(c, spec, value, false)
		}
	})
	// Why the last commit went back shows in red under the control, and
	// is read with it.
	f.Error(bad).Direction(side).Width(filterWidth(spec.Control))
	return f
}

// FilterBar lays a page's filters out in a row that wraps, as a search
// landmark named label.
func FilterBar(c *Context, label string, fn func()) Element {
	return ui.Row(c).Gap(12).Wrap().AlignItems(ui.End).Role(ui.RoleSearch).Label(label).Children(fn)
}

func filterWidth(k FilterControl) float32 {
	if k == FilterNumber {
		return 140
	}
	return 200
}

// filterTextState is a text or number filter's text while it is edited,
// the text of its value (shown), and why the last commit went back.
type filterTextState struct {
	text, shown, bad string
}

// filterText is a text or a number filter: a field committing on Enter or
// as the focus leaves it. An emptied optional field binds nil; an emptied
// required one, or a number that is none, goes back, and bad is why,
// until the next commit or Escape.
func filterText(c *Context, spec FilterSpec, value *any, number bool) (changed bool, bad string) {
	shown := plainText(*value)
	box := ui.Column(c).AlignItems(ui.Stretch)
	st := ui.Local(box, "filter", func() filterTextState { return filterTextState{text: shown, shown: shown} })
	if st.shown != shown {
		st.shown, st.text, st.bad = shown, shown, ""
	}
	box.Children(func() {
		var in Element
		if number {
			in = field(c, ui.TextInput(c, &st.text)).Height(36).Label(spec.Label).
				TextAlign(ui.End).FontFeatures("tnum").Direction(ui.LeftToRight)
		} else {
			in = TextInput(c, &st.text, spec.Label)
		}
		if !spec.Required {
			in.Placeholder("Any")
		}
		in.OnTextKey(func(k ui.TextKey) bool {
			if k.Key != ui.KeyEscape || k.Mods != 0 || k.Text() == st.shown {
				return false
			}
			k.SetText(st.shown)
			st.bad = ""
			return true
		})
		if in.Submitted() || in.FocusLeft() {
			v, why := readFilter(st.text, formatLocale(c), spec.Required, number)
			switch {
			case why != "":
				st.bad, st.text = why, st.shown
			case !sameFilter(v, *value):
				*value, changed = v, true
				st.bad, st.shown = "", plainText(v)
				st.text = st.shown
			default:
				st.bad, st.text = "", st.shown
			}
		}
		in.Invalid(st.bad != "")
	})
	return changed, st.bad
}

// readFilter reads a text or number filter's text: the value to bind, or
// why it can't be. A number is read as locale writes numbers
// (readNumber), and one that is none is refused with an example.
func readFilter(text, locale string, required, number bool) (any, string) {
	t := strings.TrimSpace(text)
	if t == "" {
		if required {
			return nil, "Required"
		}
		return nil, ""
	}
	if !number {
		return t, ""
	}
	if v, ok := readNumber(t, locale); ok {
		return v, ""
	}
	return nil, "Not a number; write it as " + i18n.FormatNumber(locale, 1234.5, i18n.NumberOptions{MinFractionDigits: 1})
}

// numberWriting is how a locale writes numbers: its group separator and
// decimal point, and whether it groups by two before the last three, as
// India does (12,34,567).
type numberWriting struct {
	group, point rune
	twos         bool
}

// writingOf learns locale's numberWriting from how it writes 1234567.5.
func writingOf(locale string) numberWriting {
	s := []rune(latinDigits(numberMarks.Replace(i18n.FormatNumber(locale, 1234567.5, i18n.NumberOptions{MinFractionDigits: 1}))))
	w := numberWriting{group: ',', point: '.'}
	lead, grouped := 0, false
	for i, r := range s {
		switch {
		case r >= '0' && r <= '9':
			if !grouped {
				lead++
			}
		case i == len(s)-2:
			w.point = r
		default:
			w.group, grouped = r, true
		}
	}
	w.twos = lead == 2
	return w
}

// numberMarks are the bidi marks i18n writes around a number's sign.
var numberMarks = strings.NewReplacer("\u200e", "", "\u200f", "", "\u061c", "")

// spaceLike are the spaces a locale grouping with one may be typed with.
const spaceLike = " \u00a0\u202f"

// readNumber reads s as locale writes numbers: its decimal point, and its
// group separator only where its grouping puts it, by threes (1,234,567)
// or, in India, by twos before the last three (12,34,567), so "1,5" is no
// number in English, rather than 15, and "1.5" none in German. Also read
// are Persian and Arabic digits, decimal point and group separator; any
// space where locale groups with one; a comma where locale groups with
// neither a comma nor a point and its decimal point is no comma (as in
// Persian and Swiss German); and a "." where locale's decimal point is
// another and its group no ".". A sign is - or the minus −, first. The
// value is an int64 when it is a whole number that fits, else a float64.
func readNumber(s, locale string) (any, bool) {
	w := writingOf(locale)
	group, point := string(w.group), string(w.point)
	s = numberMarks.Replace(latinDigits(s))
	s = strings.NewReplacer("\u066b", point, "\u066c", group).Replace(s)
	switch {
	case strings.ContainsRune(spaceLike, w.group):
		s = strings.Map(func(r rune) rune {
			if strings.ContainsRune(spaceLike, r) {
				return w.group
			}
			return r
		}, s)
	case w.group == '\'' || w.group == '\u2019':
		// Swiss German groups with an apostrophe, typed either way.
		s = strings.NewReplacer("'", group, "\u2019", group).Replace(s)
	}
	if w.group != ',' && w.group != '.' && w.point != ',' {
		s = strings.ReplaceAll(s, ",", group)
	}
	neg := false
	for _, minus := range []string{"-", "\u2212"} {
		if rest, ok := strings.CutPrefix(s, minus); ok {
			neg, s = true, rest
			break
		}
	}
	whole, frac, hasPoint := strings.Cut(s, point)
	if !hasPoint && w.point != '.' && w.group != '.' {
		whole, frac, hasPoint = strings.Cut(s, ".")
	}
	if hasPoint && !digitsOnly(frac) {
		return nil, false
	}
	whole, ok := ungroup(whole, w)
	if !ok {
		return nil, false
	}
	if neg {
		whole = "-" + whole
	}
	if !hasPoint {
		if n, err := strconv.ParseInt(whole, 10, 64); err == nil {
			return n, true
		}
	}
	t := whole
	if hasPoint {
		t += "." + frac
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil || math.IsInf(f, 0) {
		return nil, false
	}
	return f, true
}

// digitsOnly reports whether s is one or more Latin digits.
func digitsOnly(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// ungroup takes the group separators out of a number's whole part, which
// is digits alone, or grouped as w groups: by threes, the first of one to
// three digits, or, in India, by twos, the first of one or two, before
// the last three.
func ungroup(s string, w numberWriting) (string, bool) {
	parts := strings.Split(s, string(w.group))
	for _, p := range parts {
		if !digitsOnly(p) {
			return "", false
		}
	}
	if len(parts) == 1 {
		return s, true
	}
	if len(parts[len(parts)-1]) != 3 {
		return "", false
	}
	size := 3
	if w.twos {
		size = 2
	}
	for i, p := range parts[:len(parts)-1] {
		if i == 0 && len(p) > size || i > 0 && len(p) != size {
			return "", false
		}
	}
	return strings.Join(parts, ""), true
}

// sameFilter reports whether a and b bind the same value.
func sameFilter(a, b any) bool {
	return (a == nil) == (b == nil) && plainText(a) == plainText(b)
}

// filterDateState is whether a date filter's calendar shows.
type filterDateState struct{ open bool }

// filterDate is a date filter: a button showing the date, in the user's
// regional formats, that opens a calendar, and beside it a button that
// clears an optional one.
func filterDate(c *Context, spec FilterSpec, value *any) bool {
	k := TokensOf(c)
	day, set := TimeOf(*value)
	shown := "Any date"
	if set {
		shown = MediumDate(c, day)
	} else {
		day = time.Now()
	}
	row := ui.Row(c).Gap(4).AlignItems(ui.Center)
	st := ui.Local(row, "filterDate", func() filterDateState { return filterDateState{} })
	changed := false
	row.Children(func() {
		b := ui.ButtonBase(c).Height(36).PaddingX(12).Gap(8).Grow(1).Shrink(1).MinWidth(0).Radius(Radius*0.8).
			Border(1, k.Input).Background(c.Theme().Surface).AlignItems(ui.Center).FontSize(14).
			Label(spec.Label).Value(shown).HasPopup(ui.PopupGrid).Expanded(st.open)
		if b.Hovered() {
			b.Background(k.Accent)
		}
		b.Children(func() {
			ui.Icon(c, IconCalendar).Size(16, 16).Shrink(0).TextColor(k.MutedForeground)
			t := ui.Text(c, shown).SingleLine().Shrink(1).FontFeatures("tnum")
			if !set {
				t.TextColor(k.MutedForeground)
			}
		})
		if b.Clicked() {
			st.open = !st.open
		}
		ui.Popover(c, b, &st.open, func() {
			d := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local)
			cal := ui.Calendar(c, &d).Browse().AutoFocus().Label(spec.Label)
			if cal.Changed() {
				*value, changed, st.open = d.Format(time.DateOnly), true, false
			}
		})
		if set && !spec.Required {
			x := IconButton(c, IconX, "Clear "+spec.Label, Ghost).Size(28, 28)
			if x.Clicked() {
				*value, changed = nil, true
			}
		}
	})
	return changed
}

// anyOption is the value of a select's "Any", which binds nil.
const anyOption = "\x00any"

// filterSelect is a select filter: a drop-down of its options, with "Any"
// first when it is optional.
func filterSelect(c *Context, spec FilterSpec, value *any) bool {
	fell := false
	if *value == nil && spec.Required && len(spec.Options) > 0 {
		// A required select with no value takes its first option, as a
		// drop-down shows one, and binds it.
		*value, fell = spec.Options[0], true
	}
	opts := make([]PickerOption, 0, len(spec.Options)+1)
	if !spec.Required {
		opts = append(opts, PickerOption{Value: anyOption, Label: "Any"})
	}
	for _, o := range spec.Options {
		opts = append(opts, PickerOption{Value: o, Label: o})
	}
	sel := anyOption
	if *value != nil {
		sel = plainText(*value)
	}
	p := Picker(c, spec.Label, &sel, opts).Height(36)
	if *value != nil && RightToLeft(sel) {
		p.Direction(ui.RightToLeft)
	}
	if !p.Changed() {
		return fell
	}
	var v any = sel
	if sel == anyOption {
		v = nil
	}
	if sameFilter(v, *value) {
		return fell
	}
	*value = v
	return true
}

// filterCheck is a check box filter. Checked binds true; unchecked binds
// false when it is required, and nil, no filter, when it is optional. A
// required one with no value binds false, as it shows.
func filterCheck(c *Context, spec FilterSpec, value *any) bool {
	fell := false
	if *value == nil && spec.Required {
		*value, fell = false, true
	}
	on, _ := boolOf(*value)
	on = on && *value != nil
	box := ui.Checkbox(c, &on, spec.Label).FontSize(14).FontWeight(500)
	if !box.Changed() {
		return fell
	}
	switch {
	case on:
		*value = true
	case spec.Required:
		*value = false
	default:
		*value = nil
	}
	return true
}

// RelativeDate reads a date filter's default (SPEC 5.2): an ISO date, or
// today, today-7d, today-3m, today-1y, start_of_month or start_of_year,
// counted from today. It returns the day at midnight in today's location.
// Months and years back from a day the shorter month lacks run on, as
// SQLite's date modifiers do: March 31 less one month is March 3.
func RelativeDate(expr string, today time.Time) (time.Time, bool) {
	y, m, d := today.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, today.Location())
	s := strings.TrimSpace(expr)
	switch s {
	case "today":
		return day, true
	case "start_of_month":
		return time.Date(y, m, 1, 0, 0, 0, 0, today.Location()), true
	case "start_of_year":
		return time.Date(y, 1, 1, 0, 0, 0, 0, today.Location()), true
	}
	if n, ok := strings.CutPrefix(s, "today-"); ok && len(n) >= 2 {
		count, err := strconv.Atoi(n[:len(n)-1])
		if err != nil || count < 0 || n[0] == '+' {
			return time.Time{}, false
		}
		switch n[len(n)-1] {
		case 'd':
			return day.AddDate(0, 0, -count), true
		case 'm':
			return day.AddDate(0, -count, 0), true
		case 'y':
			return day.AddDate(-count, 0, 0), true
		}
		return time.Time{}, false
	}
	t, err := time.ParseInLocation(time.DateOnly, s, today.Location())
	return t, err == nil
}
