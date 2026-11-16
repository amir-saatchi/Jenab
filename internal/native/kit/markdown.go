package kit

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

// Markdown shows a chat's Markdown in Jenab's theme. Keep one per message:
// it keeps what the view knows of the text between frames.
//
// MyGo v0.7.4 draws a tab as nothing, in code blocks as in any text, so
// indented Go code loses its indentation. The kit expands tabs to tab
// stops of 4 columns before MyGo sees the text (ExpandTabs), and its code
// blocks' Copy button copies the code with its tabs.
//
// Upstream: tab stops for text came in https://github.com/egoist/mygo/pull/1217
// (for https://github.com/egoist/mygo/issues/1182), after v0.7.4. On a
// MyGo with it, TestMyGoDropsTabs fails, and this fix can go.
type Markdown struct {
	// OnLink opens a link the user clicks; nil does what MyGo does.
	OnLink func(url string)

	opts     markdown.Options
	src, out string // the last text and its tabs expanded
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
	m.opts.Theme = markdownTheme(c.Theme().Dark)
	m.opts.OnLink = m.OnLink
	m.opts.CodeBlock = m.codeBlock
	if src != m.src {
		m.src, m.out = src, ExpandTabs(src)
	}
	return m.out
}

var markdownThemes [2]*markdown.Theme

// markdownTheme is the Markdown theme of the light or the dark tokens.
func markdownTheme(dark bool) *markdown.Theme {
	i, k := 0, &Light
	if dark {
		i, k = 1, &Dark
	}
	if markdownThemes[i] == nil {
		markdownThemes[i] = &markdown.Theme{
			Font: FontSans, CodeFont: FontMono,
			Text: k.Foreground, Muted: k.MutedForeground, Link: k.Tone(Blue).Text,
			Code: k.Foreground, CodeBackground: k.Muted,
			CodeBlockBackground: k.Muted.Alpha(0.5), CodeBlockBorder: k.Border,
			QuoteBar: k.Border, QuoteText: k.MutedForeground, Rule: k.Border,
			TableBorder: k.Border, TableHeader: k.Muted.Alpha(0.5),
		}
	}
	return markdownThemes[i]
}

// codeBlock builds a code block as MyGo's DefaultCodeBlock does, in the
// kit's colors, but its Copy button copies the code as written, with its
// tabs.
func (m *Markdown) codeBlock(c *ui.Context, b *markdown.CodeBlockView) {
	k := TokensOf(c)
	box := ui.Column(c).Direction(ui.LeftToRight).Background(k.Muted.Alpha(0.5)).Border(1, k.Border).
		Radius(Radius).AlignSelf(ui.Stretch).Clip()
	label := "Code"
	if b.Language != "" {
		label += ", " + b.Language
	}
	box.Role(ui.RoleGroup).Label(label)
	box.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Padding(5, 6, 0, 12).Children(func() {
			ui.Text(c, b.Language).FontSize(11).TextColor(k.MutedForeground).Unselectable()
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
		code := b.Code()
		if m.src != m.out {
			code = m.written(b.Index)
		}
		c.WriteClipboard(code)
		*copiedAt = c.Now()
		c.After(2 * time.Second)
	}
}

// written is code block i as the text has it, with its tabs. Expanding
// tabs to stops of 4 leaves the blocks as they were (CommonMark reads tabs
// as stops of 4 for block structure), so block i is the same block.
func (m *Markdown) written(i int) string {
	d := parser.New(parser.Default)
	d.Set(m.src)
	if i < d.Len() && d.Block(i).Kind == parser.CodeBlock {
		return d.Lines(i)
	}
	e := parser.New(parser.Default)
	e.Set(m.out)
	return e.Lines(i)
}

// ExpandTabs replaces each tab with the spaces to the next tab stop, every
// 4 columns from the start of its line. Text that grows by appending
// expands to text that grows by appending, so streaming still works.
func ExpandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 16)
	col := 0
	for _, r := range s {
		switch r {
		case '\t':
			n := 4 - col%4
			b.WriteString("    "[:n])
			col += n
		case '\n':
			b.WriteRune(r)
			col = 0
		default:
			b.WriteRune(r)
			col++
		}
	}
	return b.String()
}
