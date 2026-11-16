package kit

import "github.com/egoist/mygo/ui"

// The chat's pieces (SPEC 5.12, 8.3): the thread's rows and cards and the
// composer, as frontend/src/components/ui's message, bubble, marker,
// card, item, attachment and input-group.

// SmallButton is a small button, as shadcn's size="xs": 24 px high, small
// text, with icon before label (nil for none). A label alone names it.
func SmallButton(c *Context, icon *SVG, label string, v Variant) Element {
	b := button(c, v).Height(24).PaddingX(8).Gap(4).FontSize(12).Radius(Radius * 0.6)
	if v == Ghost {
		b.TextColor(TokensOf(c).MutedForeground)
	}
	return b.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).Size(12, 12)
		}
		ui.Text(c, label).SingleLine()
	})
}

// SmallSpinner is a spinner the size of a small button's icon, as one
// that stands in for the icon while the button's work runs.
func SmallSpinner(c *Context, label string) Element {
	return ui.Spinner(c).Size(12, 12).Label(label).TextColor(TokensOf(c).MutedForeground)
}

// Disclosure is a small button that shows and hides what panel builds
// below it while *open, as the thinking and tool parts of a message: label
// names it, trigger builds what the button shows, and a chevron after it
// turns as it opens. outline gives the button a border.
func Disclosure(c *Context, label string, open *bool, outline bool, trigger, panel func()) Element {
	k := TokensOf(c)
	p := ui.CollapsibleBase(c, open)
	t := p.Trigger.Height(24).PaddingX(8).Gap(6).Radius(Radius*0.6).AlignItems(ui.Center).FontSize(12).
		TextColor(k.MutedForeground).Label(label).AlignSelf(ui.Start).MaxWidthPercent(100)
	if outline {
		t.Border(1, k.Input)
	}
	if t.Hovered() {
		t.Background(k.Accent)
	}
	t.Children(func() {
		trigger()
		ui.Icon(c, IconChevronRight).Size(12, 12).Rotate(90 * p.Progress()).Shrink(0)
	})
	p.Panel(func() {
		ui.Column(c).PaddingY(6).AlignItems(ui.Stretch).Children(panel)
	})
	return p.Trigger
}

// Mono is text in the code font, as a tool's name.
func Mono(c *Context, s string) Element {
	return ui.Text(c, s).Font(FontMono).FontSize(12)
}

// Pre is a block of text kept as written, in the code font, wrapping and
// selectable, as a tool's input and output.
func Pre(c *Context, s string) Element {
	return ui.Text(c, s).Font(FontMono).FontSize(12).PreserveSpaces().Selectable()
}

// Detail is a block muted and indented under its disclosure, with a rule
// on its start edge, as a thinking part's text.
func Detail(c *Context, fn func()) Element {
	k := TokensOf(c)
	return ui.Column(c).MarginStart(12).PaddingStart(12).BorderColor(k.Border).BorderStartWidth(1).
		Gap(6).TextColor(k.MutedForeground).FontSize(13).Children(fn)
}

// Panel is a muted box with a border, as a tool call's input and output.
func Panel(c *Context, fn func()) Element {
	k := TokensOf(c)
	return ui.Column(c).MarginStart(12).Gap(6).Padding(10).Radius(Radius*0.8).Border(1, k.Border).
		Background(k.Muted.Alpha(0.4)).Children(fn)
}

// Message is a row of the thread: a user message ends on the window's
// end edge (end), the agent's work starts on its start edge. A message
// is an article for assistive technology, named by label.
func Message(c *Context, label string, end bool, fn func()) Element {
	col := ui.Column(c).Gap(8).Role(ui.RoleArticle).Label(label).AlignItems(ui.Stretch)
	if end {
		col.AlignItems(ui.End)
	}
	return col.Children(fn)
}

// Bubble is the user's text in a filled bubble, as shadcn's Bubble; a
// message that joins the running turn is dashed (outline). Its text takes
// the direction of its words.
func Bubble(c *Context, text string, outline bool) Element {
	k := TokensOf(c)
	b := ui.Box(c).Padding(10, 14).Radius(Radius*1.6).MaxWidthPercent(80)
	if outline {
		b.Border(1, k.Border).BorderStyle(ui.BorderDashed)
	} else {
		b.Background(k.Secondary).TextColor(k.SecondaryForeground)
	}
	return b.Children(func() { ui.Text(c, text).PreserveSpaces() })
}

// Footer is the small row under a message, with its actions and time.
func Footer(c *Context, fn func()) Element {
	return ui.Row(c).Gap(4).AlignItems(ui.Center).FontSize(12).TextColor(TokensOf(c).MutedForeground).Wrap().Children(fn)
}

// Marker is a note from the app between messages, as shadcn's Marker:
// short text centered between two rules, long text wrapped with no rules.
// warn puts an amber warning icon before it.
func Marker(c *Context, text string, warn bool) Element {
	k := TokensOf(c)
	long := len([]rune(text)) > 70
	row := ui.Row(c).Gap(10).AlignItems(ui.Center).FontSize(12).TextColor(k.MutedForeground).Role(ui.RoleText).Label(text)
	return row.Children(func() {
		if !long {
			ui.Box(c).Height(1).Grow(1).Background(k.Border)
		}
		if warn {
			ui.Icon(c, IconWarning).Size(14, 14).TextColor(k.Tone(Amber).Text).Shrink(0)
		}
		t := ui.Text(c, text).Shrink(1)
		if long {
			t.Grow(1)
		}
		if !long {
			ui.Box(c).Height(1).Grow(1).Background(k.Border)
		}
	})
}

// Card is a card in the thread, as shadcn's Card size="sm": a border, the
// card color, rounded corners. fn builds its sections.
func Card(c *Context, label string, fn func()) Element {
	k := TokensOf(c)
	return ui.Column(c).Gap(12).Padding(16).Radius(Radius*1.2).Border(1, k.Border).Background(k.Card).
		TextColor(k.CardForeground).AlignItems(ui.Stretch).Role(ui.RoleGroup).Label(label).Children(fn)
}

// Kicker is a card's small capitals line above its title, as "Approval ·
// Schema change".
func Kicker(c *Context, s string) Element {
	return ui.Text(c, s).FontSize(11).FontWeight(500).LetterSpacing(0.6).TextColor(TokensOf(c).MutedForeground)
}

// CardTitle is a card's title, a heading for assistive technology.
func CardTitle(c *Context, s string) Element {
	return ui.Text(c, s).FontSize(16).FontWeight(600).Role(ui.RoleHeading).Level(3)
}

// Record is a card that was answered or stopped, in one outlined line:
// icon in tone, then fn's text.
func Record(c *Context, icon *SVG, tone Tone, fn func()) Element {
	k := TokensOf(c)
	row := ui.Row(c).Gap(8).Padding(6, 10).Radius(Radius*0.8).Border(1, k.Border).AlignItems(ui.Center).
		AlignSelf(ui.Start).FontSize(13)
	return row.Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(k.Tone(tone).Text).Shrink(0)
		ui.Row(c).Gap(4).Shrink(1).Wrap().Children(fn)
	})
}

// Strip is a muted one-line bar above the composer, as the waiting, retry
// and doing bars: icon in tone (nil for a spinner), text, and the actions
// fn builds at its end.
func Strip(c *Context, icon *SVG, tone Tone, text string, fn func()) Element {
	k := TokensOf(c)
	row := ui.Row(c).Gap(10).Padding(6, 10).Radius(Radius*0.8).Background(k.Muted).AlignItems(ui.Center).
		FontSize(13).Role(ui.RoleStatus).Label(text)
	return row.Children(func() {
		if icon == nil {
			ui.Spinner(c).Size(16, 16).TextColor(k.MutedForeground)
		} else {
			ui.Icon(c, icon).Size(16, 16).TextColor(k.Tone(tone).Text).Shrink(0)
		}
		ui.Text(c, text).Grow(1).Shrink(1)
		if fn != nil {
			fn()
		}
	})
}

// RadioGroup is a group of the radios fn builds, one Tab stop, named by
// label.
func RadioGroup(c *Context, label string, fn func()) Element {
	return ui.RadioGroup(c, fn).Label(label).Gap(6)
}

// Radio chooses value into *selected, showing label and, below it,
// description (or nothing).
func Radio(c *Context, selected *string, value, label, description string) Element {
	if description == "" {
		return ui.Radio(c, selected, value, label)
	}
	var r Element
	ui.Column(c).Gap(2).Children(func() {
		r = ui.Radio(c, selected, value, label).Description(description)
		ui.Text(c, description).FontSize(12).TextColor(TokensOf(c).MutedForeground).MarginStart(24)
	})
	return r
}

// Checkbox toggles *checked, showing label.
func Checkbox(c *Context, checked *bool, label string) Element {
	return ui.Checkbox(c, checked, label)
}

// Attachment is a file attached in the composer, as shadcn's Attachment
// size="sm": its icon, name and a line about it, and a button removing
// it, which Removed reports. failed shows the line in red.
func Attachment(c *Context, icon *SVG, name, line string, failed, busy bool) (removed bool) {
	k := TokensOf(c)
	box := ui.Row(c).Gap(8).Padding(6, 8).Radius(Radius*0.8).Border(1, k.Border).AlignItems(ui.Center).MaxWidth(256).
		Role(ui.RoleGroup).Label(name)
	box.Children(func() {
		if busy {
			ui.Spinner(c).Size(16, 16).TextColor(k.MutedForeground)
		} else {
			ui.Icon(c, icon).Size(16, 16).TextColor(k.MutedForeground).Shrink(0)
		}
		ui.Column(c).Shrink(1).Grow(1).Children(func() {
			ui.Text(c, name).FontSize(13).FontWeight(500).SingleLine()
			l := ui.Text(c, line).FontSize(12).SingleLine().TextColor(k.MutedForeground)
			if failed {
				l.TextColor(k.Tone(Red).Text)
			}
		})
		x := IconButton(c, IconX, "Remove "+name, Ghost).Size(24, 24)
		removed = x.Clicked()
	})
	return removed
}

// QuoteChip is the passage a message replies to, above the composer's
// input (TASK-019): who wrote it and up to two lines of it, with a button
// removing it, which it reports.
func QuoteChip(c *Context, who, text string) (removed bool) {
	k := TokensOf(c)
	box := ui.Row(c).Gap(8).Padding(6, 8).Radius(Radius*0.8).Background(k.Muted).AlignItems(ui.Start).
		Role(ui.RoleGroup).Label("Replying to " + who)
	box.Children(func() {
		ui.Column(c).Gap(2).Grow(1).Shrink(1).Children(func() {
			ui.Text(c, "Replying to "+who).FontSize(12).TextColor(k.MutedForeground)
			ui.Text(c, text).FontSize(13).MaxLines(2).PaddingStart(10).BorderColor(k.Primary.Alpha(0.4)).BorderStartWidth(2)
		})
		removed = IconButton(c, IconX, "Remove the quote", Ghost).Size(24, 24).Clicked()
	})
	return removed
}

// Quote is a sent message's quote, above its text: a button going to the
// quoted message, which it reports.
func Quote(c *Context, text string) (clicked bool) {
	k := TokensOf(c)
	b := ui.ButtonBase(c).Label("Go to the quoted text: " + text).MaxWidthPercent(80).AlignSelf(ui.End).
		TextColor(k.MutedForeground).FontSize(13)
	if b.Hovered() {
		b.TextColor(k.Foreground)
	}
	b.Children(func() {
		ui.Text(c, text).MaxLines(3).PaddingStart(10).BorderColor(k.Primary.Alpha(0.4)).BorderStartWidth(2)
	})
	return b.Clicked()
}

// InputGroup is the composer's box, as shadcn's InputGroup: a border in
// the input color that shows the ring while what is in it has the focus,
// with fn's rows in it.
func InputGroup(c *Context, fn func()) Element {
	k := TokensOf(c)
	box := ui.Column(c).Radius(Radius*0.8).Border(1, k.Input).Background(c.Theme().Surface).AlignItems(ui.Stretch)
	if box.FocusWithin() {
		box.Border(1, k.Ring).Shadow(0, 0, 0, 3, k.Ring.Alpha(0.5))
	}
	return box.Children(fn)
}

// ComposerText is the composer's text area, borderless in its group: it
// grows with its text from 2 lines to about 10, and takes the direction of
// its words. label names it.
func ComposerText(c *Context, value *string, label string) Element {
	k := TokensOf(c)
	return ui.TextArea(c, value).Label(label).Lines(2, 10).Padding(12, 12, 4, 12).FontSize(14).
		Background(ui.Transparent).BorderWidth(0).FocusRing(false).PlaceholderColor(k.MutedForeground)
}

// Selectable lets the user select and copy the text in e, across the
// texts in it.
func Selectable(e Element) Element { return e.Selectable() }

// MenuOn opens the menu build fills from the button e, below it.
func MenuOn(e Element, build func(m *Menu)) Element { return e.PopoverMenuButton(build) }

// MenuButtonGhost is a ghost button showing icon and label with a chevron
// after them, opening the menu build fills, as the composer's model and
// approval pickers. name names it, for when label is not enough.
func MenuButtonGhost(c *Context, icon *SVG, label, name string, build func(m *Menu)) Element {
	b := button(c, Ghost).Height(32).PaddingX(8).Gap(6).FontSize(13).Label(name)
	b.Children(func() {
		ui.Icon(c, icon).Size(14, 14)
		ui.Text(c, label).SingleLine().MaxWidth(160)
		ui.Icon(c, IconChevronDown).Size(14, 14)
	})
	return b.PopoverMenuButton(build)
}
