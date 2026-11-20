package kit

import "github.com/egoist/mygo/ui"

// StatSpec is what a stat block shows (SPEC 5.10): one number, how it
// changed, and a caption.
type StatSpec struct {
	// Label names the block for assistive technology, as its view's
	// title.
	Label string
	// Value is the number, in Format with Options (SPEC 5.3).
	Value   any
	Format  CellFormat
	Options FormatOptions
	// Change, when not nil, is how the value changed, as against
	// yesterday: above zero in green with an arrow up, below in red with
	// an arrow down, zero in the muted colour. It is written in
	// ChangeFormat, a number when that is "", with its sign.
	Change       any
	ChangeFormat CellFormat
	// Caption is short text under the number; "" for none.
	Caption string
	// State is the block's loading, error and notice.
	State BlockState
}

// StatBlock shows one number large, in figures of one width, with its
// change and caption under it, as a group named by its label. A value
// that can't be shown, as NULL, NaN or an infinity, is a dash.
func StatBlock(c *Context, spec StatSpec) Element {
	return BlockBody(c, spec.Label, spec.State, func() {
		k := TokensOf(c)
		locale := formatLocale(c)
		value := formatValue(locale, spec.Value, spec.Format, spec.Options, false)
		if value == "" {
			value = NoNumber
		}
		dir := ui.LeftToRight
		if !spec.Format.Numeric() && RightToLeft(value) {
			dir = ui.RightToLeft
		}
		// The block lines up from the side its label reads from, as a
		// table from its headings.
		side := ui.LeftToRight
		if RightToLeft(spec.Label + " " + spec.Caption) {
			side = ui.RightToLeft
		}
		col := ui.Column(c).Gap(4).AlignItems(ui.Start).Direction(side).Role(ui.RoleGroup).Label(spec.Label)
		col.Children(func() {
			ui.Text(c, value).FontSize(30).FontWeight(600).FontFeatures("tnum").
				Direction(dir).Shrink(1).MinWidth(0)
			if spec.Change != nil {
				statChange(c, k, locale, spec)
			}
			if spec.Caption != "" {
				t := ui.Text(c, spec.Caption).FontSize(13).TextColor(k.MutedForeground)
				if RightToLeft(spec.Caption) {
					t.Direction(ui.RightToLeft)
				} else {
					t.Direction(ui.LeftToRight)
				}
			}
		})
	})
}

// statChange is a stat's change: signed, green with an arrow up above
// zero, red with an arrow down below it, muted at zero, and read out with
// Up, Down or No change. A number with no ChangeFormat is written as a
// number. A change that is no number, as a word or NaN, is shown muted as
// it is (NaN as a dash), with no arrow and no word.
func statChange(c *Context, k *Tokens, locale string, spec StatSpec) {
	f, number := floatOf(spec.Change)
	format := spec.ChangeFormat
	if format == "" && number {
		format = FormatNumber
	}
	text := formatValue(locale, spec.Change, format, FormatOptions{}, true)
	color, icon, word := k.MutedForeground, (*SVG)(nil), ""
	switch {
	case !number:
	case f > 0:
		color, icon, word = k.Tone(Green).Text, IconArrowUp, "Up "
	case f < 0:
		color, icon, word = k.Tone(Red).Text, IconArrowDown, "Down "
	default:
		word = "No change "
	}
	ui.Row(c).Gap(4).AlignItems(ui.Center).Direction(ui.LeftToRight).TextColor(color).
		Label(word + text).Children(func() {
		if icon != nil {
			ui.Icon(c, icon).Size(14, 14).Shrink(0)
		}
		ui.Text(c, text).FontSize(14).FontWeight(500).FontFeatures("tnum")
	})
}
