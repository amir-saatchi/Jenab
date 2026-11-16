package kit

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

// Markdown shows a chat's Markdown in Jenab's theme. Keep one per message:
// it keeps what the view knows of the text between frames.
//
// Code blocks put tab stops every 2 columns (CodeTabSize, from MyGo v0.8.1,
// https://github.com/egoist/mygo/pull/1217): without it, a tab advances
// as DirectWrite sets it, and v0.7.4 drew tabs as nothing.
type Markdown struct {
	// OnLink opens a link the user clicks; nil does what MyGo does.
	OnLink func(url string)
	// ShareSelection leaves the selection to a Selectable element around
	// the view, as the thread's, so that one runs across messages and
	// copies them as text, as a browser does. Without it the view has a
	// selection of its own, which copies as text, HTML and Markdown.
	ShareSelection bool
	// Own, when set and OnLink isn't, takes the links the app opens
	// itself, returning true for those, as jenab:// links (SPEC 4.5); the
	// others open as MyGo opens them.
	Own func(url string) bool
	// CodeLink makes inline code a link where it returns one, as code that
	// holds just a jenab:// link.
	CodeLink func(code string) (dest string, ok bool)

	opts markdown.Options
	// nested builds the tables, lists and items that block turns, sharing
	// the selection of the view around them.
	nested markdown.Options
	// rtl holds the directions of the nested views being built, the
	// innermost last.
	rtl []bool
}

// View shows src, Markdown that is complete.
func (m *Markdown) View(c *Context, src string) Element {
	return markdown.View(c, m.prepare(c, src), &m.opts)
}

// Stream shows src, Markdown still being written: pass the reply so far
// each frame, as it grows.
func (m *Markdown) Stream(c *Context, src string) Element {
	return markdown.Stream(c, m.prepare(c, src), &m.opts)
}

func (m *Markdown) prepare(c *Context, src string) string {
	th := markdownTheme(c.Theme().Dark)
	for _, o := range []*markdown.Options{&m.opts, &m.nested} {
		o.Theme, o.OnLink, o.CodeBlock, o.Block = th, m.OnLink, m.codeBlock, m.block
		o.CodeLink = m.CodeLink
		if own := m.Own; own != nil && m.OnLink == nil {
			o.OnLink = func(url string) {
				if !own(url) {
					c.OpenURL(url)
				}
			}
		}
	}
	m.opts.ShareSelection = m.ShareSelection
	m.nested.ShareSelection = true
	return src
}

// block gives tables, lists and list items the direction of their first
// strong character, as the frontend's dir={dirOf(node)} does (SPEC 5.8):
// a Persian table's columns run right to left, and a Persian item has its
// marker on the right. MyGo lays them out in the direction around them,
// which the kit's Root keeps left to right, so one whose direction
// differs is built again as Markdown of its own, in a column of its
// direction. Its paragraphs still take their own text's direction.
func (m *Markdown) block(c *ui.Context, d *parser.Document, i int) bool {
	b := d.Block(i)
	if b.Kind != parser.List && b.Kind != parser.Table {
		return false
	}
	src := d.Source()
	outer := len(m.rtl) > 0 && m.rtl[len(m.rtl)-1]
	rtl := RightToLeft(src[b.Start:b.End])
	if b.Kind == parser.Table {
		if rtl == outer {
			return false
		}
		return m.nestedBlock(c, rtl, src, b, "")
	}
	var items []*parser.Block
	same := true
	for j := range d.Children(i) {
		it := d.Block(j)
		items = append(items, it)
		same = same && RightToLeft(src[it.Start:it.End]) == rtl
	}
	switch {
	case same && rtl == outer:
		return false
	case same:
		return m.nestedBlock(c, rtl, src, b, "")
	}
	// Items of both directions: each is a list of its own, numbered as
	// it was, spaced as MyGo spaces items.
	gap := markdownGap(c, b.Tight)
	views := make([]func(), 0, len(items))
	for k, it := range items {
		number := ""
		if b.Ordered {
			number = strconv.Itoa(b.Number + k)
		}
		item, ok := dedent(src, it, number)
		if !ok {
			return false
		}
		views = append(views, func() {
			el := m.nestedView(c, RightToLeft(src[it.Start:it.End]), item)
			if k > 0 {
				el.Margin(gap, 0, 0, 0)
			}
		})
	}
	ui.Column(c).AlignSelf(ui.Stretch).Children(func() {
		for _, v := range views {
			v()
		}
	})
	return true
}

// nestedBlock builds block b of src as Markdown of its own in direction
// rtl; it reports false, for MyGo to build the block, when b's lines
// can't be taken out of what holds them.
func (m *Markdown) nestedBlock(c *ui.Context, rtl bool, src string, b *parser.Block, number string) bool {
	text, ok := dedent(src, b, number)
	if !ok {
		return false
	}
	m.nestedView(c, rtl, text)
	return true
}

func (m *Markdown) nestedView(c *ui.Context, rtl bool, src string) ui.Element {
	dir := ui.LeftToRight
	if rtl {
		dir = ui.RightToLeft
	}
	m.rtl = append(m.rtl, rtl)
	defer func() { m.rtl = m.rtl[:len(m.rtl)-1] }()
	return ui.Column(c).Direction(dir).AlignSelf(ui.Stretch).Children(func() { markdown.View(c, src, &m.nested) })
}

// dedent returns the lines of block b of src as Markdown that stands
// alone: the lines after its first lose the indent of its first column.
// number, when set, replaces an ordered item's number. It reports false
// when a line doesn't start with that indent, as in a quote, where the
// block can't be taken out.
func dedent(src string, b *parser.Block, number string) (string, bool) {
	col := b.Start - (strings.LastIndexByte(src[:b.Start], '\n') + 1)
	text := strings.TrimRight(src[b.Start:b.End], "\n")
	if number != "" {
		n := 0
		for n < len(text) && text[n] >= '0' && text[n] <= '9' {
			n++
		}
		text = number + text[n:]
	}
	if col == 0 {
		return text, true
	}
	indent := strings.Repeat(" ", col)
	lines := strings.Split(text, "\n")
	for k := 1; k < len(lines); k++ {
		switch {
		case strings.TrimSpace(lines[k]) == "":
			lines[k] = ""
		case strings.HasPrefix(lines[k], indent):
			lines[k] = lines[k][col:]
		default:
			return "", false
		}
	}
	return strings.Join(lines, "\n"), true
}

// markdownGap is the space MyGo puts between a list's items, in DIPs.
func markdownGap(c *ui.Context, tight bool) float32 {
	size := c.Theme().FontSize
	if size == 0 {
		size = 14
	}
	if tight {
		return 0.25 * size // markdown.Theme's ItemSpacing
	}
	return 0.75 * size // its BlockSpacing
}

var markdownThemes [2]*markdown.Theme

// markdownTheme is the Markdown theme of the light or the dark tokens, in
// the look of the frontend's typeset.css (TASK-054):
//   - a quote's text is the text's color, by a 2 px bar;
//   - links are of weight 500. The frontend underlines them in the
//     text's color; MyGo underlines a Markdown link only while the
//     pointer is on it and has no setting for more, so they stay blue to
//     stand out;
//   - a table's header has no fill, and cells have the frontend's room,
//     1 em across and 0.75 em down. The frontend draws only lines between
//     rows and under the table; MyGo draws a frame and lines between
//     columns too, in one color, with no setting to leave them out.
func markdownTheme(dark bool) *markdown.Theme {
	i, k := 0, &Light
	if dark {
		i, k = 1, &Dark
	}
	if markdownThemes[i] == nil {
		markdownThemes[i] = &markdown.Theme{
			Font: FontSans, CodeFont: FontMono,
			Text: k.Foreground, Muted: k.MutedForeground, Link: k.Tone(Blue).Text, LinkWeight: 500,
			Code: k.Foreground, CodeBackground: k.Muted,
			CodeBlockBackground: k.Muted.Alpha(0.5), CodeBlockBorder: k.Border,
			QuoteBar: k.Border, QuoteText: k.Foreground, QuoteBarWidth: 2, Rule: k.Border,
			// Not the zero color, which MyGo takes as its own header fill.
			TableBorder: k.Border, TableHeader: k.Background.Alpha(0),
			TableCellPadding: [2]float32{1, 0.75},
		}
	}
	return markdownThemes[i]
}

// codeBlock builds a code block as MyGo's DefaultCodeBlock does, in the
// kit's colors, with tab stops every 2 columns. It shows no language, as
// the frontend's; a screen reader still hears it ("Code, go").
func (m *Markdown) codeBlock(c *ui.Context, b *markdown.CodeBlockView) {
	k := TokensOf(c)
	box := ui.Column(c).Direction(ui.LeftToRight).Background(k.Muted.Alpha(0.5)).Border(1, k.Border).
		Radius(Radius).AlignSelf(ui.Stretch).Clip().TabSize(CodeTabSize)
	label := "Code"
	if b.Language != "" {
		label += ", " + b.Language
	}
	box.Role(ui.RoleGroup).Label(label)
	box.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Padding(5, 6, 0, 12).Children(func() {
			ui.Spacer(c)
			m.copyButton(c, b)
		})
		b.Body(c)
	})
}

func (m *Markdown) copyButton(c *ui.Context, b *markdown.CodeBlockView) {
	k := TokensOf(c)
	btn := ui.ButtonBase(c).Padding(3, 6).Radius(5).Gap(4).Label("Copy code").Disabled(b.Open)
	copiedAt := ui.Local(btn, "copied", func() time.Time { return time.Time{} })
	copied := !copiedAt.IsZero() && c.Now().Sub(*copiedAt) < 2*time.Second
	if btn.Hovered() && !b.Open {
		btn.Background(k.Foreground.Alpha(0.08))
	}
	btn.Children(func() {
		icon, text := IconCopy, "Copy"
		if copied {
			icon, text = IconCheck, "Copied"
		}
		ui.Icon(c, icon).Size(13, 13).TextColor(k.MutedForeground)
		ui.Text(c, text).FontSize(11).TextColor(k.MutedForeground)
	})
	if btn.Clicked() {
		c.WriteClipboard(b.Code())
		*copiedAt = c.Now()
		c.After(2 * time.Second)
	}
}
