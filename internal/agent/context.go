package agent

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// The system prompt (3.1, block 1). The text is the same for every model
// (1). Mother's delegation guidance comes with create_chat and
// send_to_chat (Phase 5).
var (
	//go:embed prompts/base.md
	basePrompt string
	//go:embed prompts/rule.md
	rulePrompt string // the prompt rule for requests with several parts (8.3, SPIKE-023)
	//go:embed prompts/mother.md
	motherPrompt string
)

// cacheIdle is how long a provider keeps a prompt cache. After a longer
// pause the next turn cuts the window, since the cache is lost anyway
// (3.6).
const cacheIdle = 5 * time.Minute

// window is a chat's history window (3.6). Between cuts it only grows at
// the end, so provider caches keep working (3.1).
type window struct {
	start     int              // the first turn in the window; 0 before the first cut
	stubBelow int              // tool results of turns before this are stubs
	stubbed   map[string]bool  // tool calls whose results in-turn trimming made stubs
	chatList  string           // Mother's chat list, refreshed at cuts (8.6)
	system    []provider.Block // blocks 1–5, built at cuts
	last      time.Time        // the last request
}

// cutIfDue cuts the window at the start of a turn when it has passed
// history_max_turns or history_max_tokens, or when the cache has expired.
// A chat with no request yet in this process (after a restart or Clear)
// cuts too: its cache is gone as well. The system blocks are built at a
// cut and kept until the next one (3.1), so a role change takes effect
// then.
func (r *runner) cutIfDue(ctx context.Context, t *turn) error {
	w := &r.cs.win
	c := t.set.Context
	due := w.last.IsZero() || time.Since(w.last) > cacheIdle
	if !due {
		ms, _, err := r.p.Chats.Messages(ctx, t.ch.ID, w.start, t.n-1)
		if err != nil {
			return err
		}
		prev := shape(ms, w)
		due = t.n-w.start > c.HistoryMaxTurns || provider.EstimateTokens(provider.Request{Messages: prev}) > c.HistoryMaxTokens
	}
	if !due {
		return nil
	}
	w.start, w.stubBelow, w.stubbed = max(1, t.n-c.HistoryMinTurns), t.n, map[string]bool{}
	if t.ch.Kind == chat.KindMother {
		list, err := r.chatList(ctx)
		if err != nil {
			return err
		}
		w.chatList = list
	}
	sys, err := r.system(ctx, t)
	if err != nil {
		return err
	}
	w.system = sys
	return nil
}

// request builds one request: the system blocks, the window and the
// current turn. The last request of a turn has no tools (8.3).
func (r *runner) request(ctx context.Context, t *turn, last bool) (provider.Request, error) {
	w := &r.cs.win
	ms, _, err := r.p.Chats.Messages(ctx, t.ch.ID, w.start, t.n)
	if err != nil {
		return provider.Request{}, err
	}
	for _, m := range ms {
		if m.Turn == t.n && m.Role == chat.RoleUser && m.ID > t.seen {
			t.seen = m.ID
		}
	}
	msgs := shape(ms, w)
	if r.trim(t, msgs) {
		msgs = shape(ms, w)
	}
	req := provider.Request{Model: t.model, System: w.system, Messages: msgs, Thinking: true, Image: r.image}
	if last {
		req.Messages = append(req.Messages, chat.Message{Chat: t.ch.ID, Turn: t.n, Role: chat.RoleUser,
			Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: lastRequest}}}})
	} else {
		for _, tl := range r.o.d.Tools.For(t.ch.Kind) {
			s := tl.Spec()
			req.Tools = append(req.Tools, provider.ToolDef{Name: s.Name, Description: s.Description, Schema: s.Schema})
		}
	}
	w.last = time.Now()
	return req, nil
}

// shape is the window's messages as the model sees them: tool results of
// turns before the last cut, and those trimmed in a turn, as stubs (3.7).
// A tool call without a result, left by a crash, gets one, so every
// provider accepts the history.
func shape(ms []chat.Message, w *window) []chat.Message {
	out := make([]chat.Message, 0, len(ms))
	var open []string // calls of the last assistant message without results
	var from chat.Message
	closeOpen := func() {
		if len(open) == 0 {
			return
		}
		var parts []chat.Part
		for _, c := range open {
			parts = append(parts, chat.Part{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: c, Text: "no result: the app closed before this call finished", IsError: true}})
		}
		out = append(out, chat.Message{Chat: from.Chat, Turn: from.Turn, Role: chat.RoleTool, Parts: parts})
		open = nil
	}
	for _, m := range ms {
		switch m.Role {
		case chat.RoleAssistant:
			closeOpen()
			from = m
			for _, p := range m.Parts {
				if p.ToolCall != nil {
					open = append(open, p.ToolCall.ID)
				}
			}
		case chat.RoleTool:
			parts := make([]chat.Part, len(m.Parts))
			for i, p := range m.Parts {
				if r := p.ToolResult; r != nil {
					open = without(open, r.CallID)
					if m.Turn < w.stubBelow || w.stubbed[r.CallID] {
						s := *r
						s.Text = tool.Stub(s)
						p.ToolResult = &s
					}
				}
				parts[i] = p
			}
			m.Parts = parts
		default:
			closeOpen()
		}
		out = append(out, m)
	}
	closeOpen()
	return out
}

func without(ss []string, s string) []string {
	for i, x := range ss {
		if x == s {
			return append(ss[:i:i], ss[i+1:]...)
		}
	}
	return ss
}

// trim makes the current turn's tool previews stubs, all but the two
// newest, once the turn passes turn_trim_ratio of the model's window
// (3.7). It reports whether it changed anything.
func (r *runner) trim(t *turn, msgs []chat.Message) bool {
	if t.window <= 0 {
		return false
	}
	var cur []chat.Message
	for _, m := range msgs {
		if m.Turn == t.n {
			cur = append(cur, m)
		}
	}
	if float64(provider.EstimateTokens(provider.Request{Messages: cur})) <= t.set.Context.TurnTrimRatio*float64(t.window) {
		return false
	}
	var refs []string
	for _, m := range cur {
		for _, p := range m.Parts {
			if p.ToolResult != nil && p.ToolResult.Ref != "" {
				refs = append(refs, p.ToolResult.CallID)
			}
		}
	}
	changed := false
	for _, c := range refs[:max(0, len(refs)-2)] {
		if !r.cs.win.stubbed[c] {
			r.cs.win.stubbed[c] = true
			changed = true
		}
	}
	return changed
}

// system is blocks 1–5 of the context (3.1) for the window. Phase 1 has the
// system prompt, the role, the skills and Mother's chat list. The cache
// point is after the last block.
func (r *runner) system(ctx context.Context, t *turn) ([]provider.Block, error) {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(basePrompt) + "\n\n" + strings.TrimSpace(rulePrompt))
	if t.ch.Kind == chat.KindMother {
		b.WriteString("\n\n" + strings.TrimSpace(motherPrompt))
	}
	if role := strings.TrimSpace(t.ch.Role); role != "" {
		b.WriteString("\n\nThis chat's role:\n" + role)
	}
	if r.o.d.Skills != nil {
		s, err := r.o.d.Skills.Block(ctx, r.p, t.ch)
		if err != nil {
			return nil, err
		}
		if s = strings.TrimSpace(s); s != "" {
			b.WriteString("\n\n" + s)
		}
	}
	blocks := []provider.Block{{Text: b.String()}}
	w := &r.cs.win
	if t.ch.Kind == chat.KindMother {
		blocks = append(blocks, provider.Block{Text: w.chatList})
	}
	if n := w.start - 1; n > 0 {
		blocks = append(blocks, provider.Block{Text: earlierTurns(n)})
	}
	blocks[len(blocks)-1].Cache = true
	return blocks, nil
}

// earlierTurns is the line after the first cut (3.6).
func earlierTurns(n int) string {
	if n == 1 {
		return "This chat has 1 earlier turn. Use search_history or read_messages."
	}
	return fmt.Sprintf("This chat has %d earlier turns. Use search_history or read_messages.", n)
}

// chatList is Mother's list of the other chats (8.6): ID, title, the first
// line of the role, status, last activity and the first line of the
// session notes.
func (r *runner) chatList(ctx context.Context) (string, error) {
	cs, err := r.p.Chats.Chats(ctx)
	if err != nil {
		return "", err
	}
	act, err := r.p.Chats.LastActivity(ctx)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("Chat list")
	n := 0
	for _, c := range cs {
		if c.Kind == chat.KindMother || c.Archived {
			continue
		}
		n++
		title := c.Title
		if title == "" {
			title = "untitled"
		}
		fmt.Fprintf(&b, "\n- %s %q.", c.ID, title)
		if role := firstLine(c.Role); role != "" {
			fmt.Fprintf(&b, " Role: %s.", role)
		}
		status := r.o.ChatStatus(c.ID)
		if status == "" {
			status = "idle"
		}
		fmt.Fprintf(&b, " Status: %s.", status)
		if at, ok := act[c.ID]; ok {
			fmt.Fprintf(&b, " Last activity: %s.", at.Local().Format("2006-01-02 15:04"))
		}
		notes, err := r.p.Chats.Notes(ctx, c.ID)
		if err != nil {
			return "", err
		}
		if s := firstLine(notes.Content); s != "" {
			fmt.Fprintf(&b, " Notes: %s.", s)
		}
	}
	if n == 0 {
		return "Chat list: no other chats yet.", nil
	}
	return b.String(), nil
}

// firstLine is the first non-empty line of s, without a full stop at the
// end.
func firstLine(s string) string {
	for l := range strings.Lines(s) {
		if l = strings.TrimSpace(l); l != "" {
			return strings.TrimRight(l, ".")
		}
	}
	return ""
}

// image reads an image part's bytes from the bucket.
func (r *runner) image(ctx context.Context, ref string) ([]byte, error) {
	_, rc, err := r.p.DB.OpenObject(ctx, ref)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}
