package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// Markdown shows a chat's Markdown in Jenab's theme. Keep one per message:
// it keeps what the view knows of the text between frames.
//
// Code blocks put tab stops every 4 columns (TabSize, from MyGo v0.8.1,
// https://github.com/egoist/mygo/pull/1217): without it, a tab advances
// as DirectWrite sets it, and v0.7.4 drew tabs as nothing.
type Markdown struct {
	// OnLink opens a link the user clicks; nil does what MyGo does.
	OnLink func(url string)

	opts markdown.Options
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
	return src
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
// kit's colors, with tab stops every 4 columns.
func (m *Markdown) codeBlock(c *ui.Context, b *markdown.CodeBlockView) {
	k := TokensOf(c)
	box := ui.Column(c).Direction(ui.LeftToRight).Background(k.Muted.Alpha(0.5)).Border(1, k.Border).
		Radius(Radius).AlignSelf(ui.Stretch).Clip().TabSize(TabSize)
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
		c.WriteClipboard(b.Code())
		*copiedAt = c.Now()
		c.After(2 * time.Second)
	}
}
