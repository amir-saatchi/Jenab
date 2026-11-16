package state

import (
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

// Stream is an answer still streaming (Status.Streaming): its finished
// parts and the text of the part being streamed. React applies deltas once
// a frame (state/stream.ts); here each delta is applied as it comes, since
// the agent sends at most one per chat every 16 ms (SPEC 5.8) and
// Window.Update draws at most one frame per refresh.
type Stream struct {
	Message id.Message
	Parts   []chat.Part
	Index   int // the part being streamed
	Kind    chat.PartKind
	Text    string
}

// FromLive is the stream a snapshot found, or nil when nothing streams.
func FromLive(l agent.Live) *Stream {
	if l.Streaming == "" {
		return nil
	}
	return &Stream{Message: l.Streaming, Parts: l.Parts, Index: len(l.Parts), Kind: l.Kind, Text: l.Text}
}

// AddDelta applies one delta, as the Go side builds the answer: a higher
// part index finishes the part before, and a new kind at the same index
// starts its text again. A delta for a part before the one streaming came
// late, and that part is already finished, so it is dropped and s returned
// as it is. Otherwise the result is a new Stream.
func AddDelta(s *Stream, d chat.Delta) *Stream {
	var n Stream
	switch {
	case s == nil || s.Message != d.Message:
		n = Stream{Message: d.Message, Index: d.Part, Kind: d.Kind}
	case d.Part < s.Index:
		return s
	default:
		n = *s
	}
	if d.Part > n.Index {
		if n.Text != "" {
			n.Parts = append(append([]chat.Part(nil), n.Parts...), textPart(n.Kind, n.Text))
		}
		n.Index, n.Kind, n.Text = d.Part, d.Kind, ""
	} else if d.Kind != n.Kind {
		n.Kind, n.Text = d.Kind, ""
	}
	n.Text += d.Text
	return &n
}

func textPart(kind chat.PartKind, text string) chat.Part {
	if kind == chat.PartThinking {
		return chat.Part{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: text}}
	}
	return chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: text}}
}

// utf16Len is the length of s in UTF-16 units, as JavaScript counts and as
// Delta.Offset is given.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r >= 0x10000 {
			n++
		}
	}
	return n
}

// utf16Cut is s without its first n UTF-16 units. A cut inside a
// surrogate pair drops the whole character, which the text before already
// held half of.
func utf16Cut(s string, n int) string {
	for i := 0; n > 0 && i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		n--
		if r >= 0x10000 {
			n--
		}
		i += size
		if n <= 0 {
			return s[i:]
		}
	}
	if n > 0 {
		return ""
	}
	return s
}
