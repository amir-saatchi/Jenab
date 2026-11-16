package chat

import (
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// The chat's jenab:// links (SPEC 4.5, TASK-056): in an answer, a link
// written as a link, bare, or alone in inline code opens the file next to
// the chat (state.MarkLinks, state.CodeLink); in the user's message, only
// a bare link does.

// own takes the links the chat opens itself: jenab:// links, and the
// others when OnLink is set.
func (s *Screen) own(url string) bool {
	switch {
	case state.IsLink(url):
		s.st.OpenLink(url, nil)
	case s.OnLink != nil:
		s.OnLink(url)
	default:
		return false
	}
	return true
}

// bubble is the user's text, its bare jenab:// links links.
func (s *Screen) bubble(c *kit.Context, text string, outline bool) kit.Element {
	pieces := state.SplitLinks(text)
	links := false
	parts := make([]kit.TextPart, len(pieces))
	for i, p := range pieces {
		parts[i] = kit.TextPart{Text: p.Text, Link: p.Link}
		links = links || p.Link
	}
	if !links {
		return kit.Bubble(c, text, outline)
	}
	return kit.LinkBubble(c, parts, outline, func(l string) { s.st.OpenLink(l, nil) })
}

// storedAs is the line under a tool's output stored in the bucket, its key
// opening it next to the chat.
func (s *Screen) storedAs(c *kit.Context, key string) {
	parts := []kit.TextPart{{Text: "The full output is stored as "}, {Text: key, Link: true}, {Text: "."}}
	kit.LinkText(c, parts, s.st.OpenHere).FontSize(12).TextColor(kit.TokensOf(c).MutedForeground)
}
