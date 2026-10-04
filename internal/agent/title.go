package agent

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/store"
)

// titleTimeout bounds the title request.
const titleTimeout = time.Minute

// titlePrompt asks for a chat title. The same for every model (1).
const titlePrompt = "Write a title for this chat: at most six words, in the language of the user's message, with no quotes and no full stop. Answer with the title only."

// titleMaxTokens bounds the title answer; thinking models need room
// before the title.
const titleMaxTokens = 200

// maxTitleInput is how much of the user's message and of the answer the
// title request gets, in bytes.
const maxTitleInput = 2000

// startTitle names the chat in the background while the next turn may
// run. The runner waits for it before it ends.
func (r *runner) startTitle(ctx context.Context, wg *sync.WaitGroup, t *turn) {
	cs := r.cs
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.titling {
		return
	}
	cs.titling = true
	clears := cs.clears
	wg.Go(func() { // the title request, owned by the runner
		defer func() {
			if p := recover(); p != nil {
				r.o.d.Log.Error("agent: panic in the title", "chat", cs.key.c, "panic", p)
			}
			cs.mu.Lock()
			cs.titling = false
			cs.mu.Unlock()
		}()
		r.title(ctx, t.ch, t.n, clears)
	})
}

// title names a chat after its first turn with the fast model, or the
// chat's own model if fast is not set (8.6). A failure leaves the chat
// untitled until its next turn. A title for a chat cleared since its turn
// is dropped.
func (r *runner) title(ctx context.Context, ch chat.Chat, n, clears int) {
	ctx, cancel := context.WithTimeout(ctx, titleTimeout)
	defer cancel()
	ms, _, err := r.p.Chats.Messages(ctx, ch.ID, n, n)
	if err != nil {
		r.o.d.Log.Warn("agent: title", "chat", ch.ID, "err", err)
		return
	}
	var user, answer string
	for _, m := range ms {
		for _, p := range m.Parts {
			if p.Text == nil {
				continue
			}
			switch {
			case m.Role == chat.RoleUser && user == "":
				user = p.Text.Text
			case m.Role == chat.RoleAssistant:
				answer = p.Text.Text
			}
		}
	}
	if user == "" {
		return
	}
	model := "fast"
	if _, _, err := r.o.d.Models.Resolve(model); err != nil {
		model = ch.Model
	}
	req := provider.Request{
		Model:     model,
		MaxTokens: titleMaxTokens,
		System:    []provider.Block{{Text: titlePrompt}},
		Messages: []chat.Message{{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{
			Text: "User: " + cutBytes(user, maxTitleInput) + "\n\nAssistant: " + cutBytes(answer, maxTitleInput)}}}}},
	}
	var text strings.Builder
	for ev, err := range r.o.d.Models.Stream(ctx, limit.Background, req) {
		if err != nil {
			r.o.d.Log.Warn("agent: title", "chat", ch.ID, "err", err)
			return
		}
		if ev.Kind == provider.EventPart && ev.Part.Text != nil {
			text.WriteString(ev.Part.Text.Text)
		}
	}
	t := cleanTitle(text.String())
	if t == "" {
		return
	}
	cs := r.cs
	cs.mu.Lock()
	if cs.clears != clears {
		cs.mu.Unlock()
		return
	}
	cs.pub.Lock() // before mu is let go, so a Clear waits for the title
	cs.mu.Unlock()
	_, seq, err := r.p.Chats.SetTitle(ctx, ch.ID, t, false)
	if err == nil {
		cs.bumpSeq(seq)
	}
	cs.pub.Unlock()
	if err != nil {
		r.o.d.Log.Warn("agent: title", "chat", ch.ID, "err", err)
		return
	}
	// A status with the new seq makes the frontend read the chat again.
	cs.mu.Lock()
	r.o.publishStatus(cs)
	cs.mu.Unlock()
}

// titleMarks are trimmed from both ends of a title: quotes, Markdown
// marks, and direction marks and joiners (LRM, RLM, ALM, ZWNJ, ZWJ).
const titleMarks = " \t\"'`*#“”«»‘’„‹›\u200e\u200f\u061c\u200c\u200d"

// cleanTitle is the first line of a model's answer in NFC, without quotes,
// Markdown marks, direction marks or a full stop, at most store.MaxTitle
// characters.
func cleanTitle(s string) string {
	s = firstLine(norm.NFC.String(s))
	s = strings.TrimLeft(s, titleMarks)
	s = strings.TrimRight(s, titleMarks+".。۔…")
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > store.MaxTitle {
		s = string(r[:store.MaxTitle])
	}
	return s
}

// cutBytes is the start of s, at most n bytes, at a character boundary.
func cutBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
