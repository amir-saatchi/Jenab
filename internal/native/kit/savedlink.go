package kit

import "github.com/egoist/mygo/ui"

// SavedLinkFace is what a saved link's row shows (SPEC 4.6, P3-11).
type SavedLinkFace struct {
	Title string // "" shows Host in its place
	Host  string // the URL's host, or a mailto address
	Note  string
	Tags  []string
	// Saved says who saved it and when, as "Saved by you · Nov 16, 2026".
	Saved string
	// Blocked marks a link that doesn't open in the system, one SafeLink
	// refuses, with a badge saying so.
	Blocked bool
}

// SavedLink is a saved link's face in the Links tab, for a TreeRows.Item:
// its title, its host under it, its note in up to two lines, its tags as
// badges and who saved it when. The title and the note each take the
// direction of their first strong letter (5.8), so a Persian title starts
// on the right; the host is a URL's and runs left to right.
func SavedLink(c *Context, f SavedLinkFace) Element {
	k := TokensOf(c)
	title := f.Title
	if title == "" {
		title = f.Host
	}
	return ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).PaddingY(4).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ownDirection(ui.Text(c, isolate(title)).FontSize(14).FontWeight(500).SingleLine().Grow(1).Shrink(1).MinWidth(0), title)
			if f.Blocked {
				Badge(c, "Doesn't open", Amber).Shrink(0).Tooltip("Only http, https and mailto links open from the app.")
			}
		})
		if f.Title != "" && f.Host != "" {
			ui.Text(c, isolate(f.Host)).FontSize(12).TextColor(k.MutedForeground).SingleLine().Direction(ui.LeftToRight).
				TextAlign(ui.Start).Shrink(1).MinWidth(0)
		}
		if f.Note != "" {
			ownDirection(ui.Text(c, isolate(f.Note)).FontSize(13).MaxLines(2), f.Note)
		}
		if len(f.Tags) > 0 {
			ui.Row(c).Wrap().Gap(4).Children(func() {
				for _, t := range f.Tags {
					c.Key(t)
					Badge(c, isolate(t), Neutral)
				}
			})
		}
		if f.Saved != "" {
			ui.Text(c, f.Saved).FontSize(12).TextColor(k.MutedForeground).SingleLine().Shrink(1).MinWidth(0)
		}
	})
}

// isolate wraps data in First Strong Isolate and Pop Directional Isolate
// (U+2068, U+2069), so direction controls in a saved title or host can't
// reorder what is drawn after it (SPEC 5.8), as chat/logic.Isolate does.
func isolate(s string) string { return "⁨" + s + "⁩" }

// ownDirection lays text out in the direction of its first strong letter,
// as dir="auto": right to left text starts on the right of its box.
func ownDirection(e Element, s string) Element {
	if RightToLeft(s) {
		return e.Direction(ui.RightToLeft).TextAlign(ui.Start)
	}
	return e.Direction(ui.LeftToRight).TextAlign(ui.Start)
}

// OpenLink opens url in the system if SafeLink allows it, an http, https
// or mailto address, and reports whether it did. It is the one way the
// screens open a link from data, as a saved link (SPEC 4.5).
func OpenLink(c *Context, url string) bool {
	if !SafeLink(url) {
		return false
	}
	c.OpenURL(url)
	return true
}
