package agent

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/appinfo"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// The system prompt (3.1, block 1). The text is the same for every model
// (1). Lines about a kind of work come only with the tools that do it, so
// the model isn't told of work it can't start.
var (
	//go:embed prompts/base.md
	basePrompt string
	//go:embed prompts/rule.md
	rulePrompt string // the prompt rule for requests with several parts (8.3, SPIKE-023); with a background tool
	//go:embed prompts/mother.md
	motherPrompt string
	//go:embed prompts/delegate.md
	delegatePrompt string // Mother's delegation guidance (8.6); with create_chat
)

// promptNames fills in the names the prompt files leave as {app} and {mother}.
var promptNames = strings.NewReplacer("{app}", appinfo.Name, "{mother}", appinfo.Mother)

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
	skills    map[string]bool  // the skills in block 1, from the last cut
	last      time.Time        // the last request
	fired     map[int]bool     // turns whose number check fired (SPIKE-032)
	tainted   []taint          // numbers the chat's write calls saved without a source, and where (SPIKE-032)
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
	w.skills = map[string]bool{}
	for _, name := range t.ch.Skills {
		if _, ok := r.o.skills().Get(t.ch.Kind, name); ok {
			w.skills[name] = true
		}
	}
	return nil
}

// request builds one request: the system blocks, the window and the
// current turn. The last request of a turn keeps the tools, so the cached
// prefix holds, but asks for an answer without calls (8.3).
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
	for _, tl := range r.o.d.Tools.For(t.ch.Kind) {
		s := tl.Spec()
		req.Tools = append(req.Tools, provider.ToolDef{Name: s.Name, Description: s.Description, Schema: s.Schema})
	}
	if last {
		req.ToolChoice = provider.ToolsNone
		req.Messages = append(req.Messages, chat.Message{Chat: t.ch.ID, Turn: t.n, Role: chat.RoleUser,
			Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: lastRequest}}}})
	}
	w.last = time.Now()
	return req, nil
}

// shape is the window's messages as the model sees them: tool results of
// turns before the last cut, and those trimmed in a turn, as stubs (3.7).
// A skill's text joins the result after it; before the last cut, a skill
// in block 1 leaves a stub (8.9). A tool call without a result, left by a
// crash, gets one, so every provider accepts the history.
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
			parts := make([]chat.Part, 0, len(m.Parts))
			var skills []string // skill texts waiting for their result
			for _, p := range m.Parts {
				if n := p.Notice; n != nil && n.Kind == chat.NoticeSkillText {
					if m.Turn < w.stubBelow && w.skills[n.Skill] {
						skills = append(skills, "[skill "+n.Skill+": moved into the system prompt]")
					} else {
						skills = append(skills, n.Text)
					}
					continue
				}
				if r := p.ToolResult; r != nil {
					open = without(open, r.CallID)
					s := *r
					if m.Turn < w.stubBelow || w.stubbed[r.CallID] {
						s.Text = tool.Stub(s)
					}
					for _, k := range skills {
						if s.Text != "" {
							s.Text += "\n\n"
						}
						s.Text += k
					}
					p.ToolResult, skills = &s, nil
				}
				parts = append(parts, p)
			}
			m.Parts = parts
		default:
			closeOpen()
			// The number check's mark is for the user only (SPIKE-032).
			if slices.ContainsFunc(m.Parts, isMark) {
				if m.Parts = slices.DeleteFunc(slices.Clone(m.Parts), isMark); len(m.Parts) == 0 {
					continue
				}
			}
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
// system prompt, the date, the role, the skills and Mother's chat list.
// The cache point is after the last block.
func (r *runner) system(ctx context.Context, t *turn) ([]provider.Block, error) {
	tools := r.o.d.Tools.For(t.ch.Kind)
	var b strings.Builder
	b.WriteString(base(tools))
	if slices.ContainsFunc(tools, func(tl tool.Tool) bool { return tl.Spec().Background }) {
		b.WriteString("\n\n" + strings.TrimSpace(rulePrompt))
	}
	if r.o.d.Grounding.Compute && slices.ContainsFunc(tools, func(tl tool.Tool) bool { return tl.Spec().Name == "query" }) {
		b.WriteString("\n\n" + computeLine) // SPIKE-032 round 2, spike-only
	}
	if t.ch.Kind == chat.KindMother {
		b.WriteString("\n\n" + promptNames.Replace(strings.TrimSpace(motherPrompt)))
		if slices.ContainsFunc(tools, func(tl tool.Tool) bool { return tl.Spec().Name == "create_chat" }) {
			b.WriteString("\n\n" + strings.TrimSpace(delegatePrompt))
		}
	}
	b.WriteString("\n\n" + today(r.o.d.Now()))
	if role := strings.TrimSpace(t.ch.Role); role != "" {
		b.WriteString("\n\nThis chat's role:\n" + role)
	}
	// The loaded skills come after the role, and the skill list ends the
	// block (8.9).
	if s := r.o.skills().Block(t.ch.Kind, t.ch.Skills); s != "" {
		b.WriteString("\n\n" + s)
	}
	blocks := []provider.Block{{Text: b.String(), Name: "System prompt, role and skills"}}
	if f := r.o.d.Card; f != nil {
		card, err := f(ctx, r.cs.key.p)
		if err != nil {
			return nil, err
		}
		if card = strings.TrimSpace(card); card != "" {
			blocks = append(blocks, provider.Block{Text: card, Name: "Project card"})
		}
	}
	w := &r.cs.win
	if t.ch.Kind == chat.KindMother {
		blocks = append(blocks, provider.Block{Text: w.chatList, Name: "Chat list"})
	}
	if n := w.start - 1; n > 0 {
		blocks = append(blocks, provider.Block{Text: earlierTurns(n), Name: "Earlier turns"})
	}
	blocks[len(blocks)-1].Cache = true
	return blocks, nil
}

// areaText is how block 1 names each area (tool.AreaNames' order).
var areaText = []string{"tables in a local database", "pipelines that fetch data", "views"}

// base is the system prompt's start. It names the parts of a project the
// tools work on, and says the others aren't in this version, so the model
// doesn't build them from files (TASK-014).
func base(tools []tool.Tool) string {
	var has tool.Areas
	for _, tl := range tools {
		has |= tl.Spec().Areas
	}
	var parts, missing []string
	for i, s := range areaText {
		if has&(1<<i) != 0 {
			parts = append(parts, s)
		} else {
			missing = append(missing, tool.AreaNames[i])
		}
	}
	areas, lacks := "", ""
	if len(parts) > 0 {
		areas = ": " + list(parts, "and")
	}
	if len(missing) > 0 {
		lacks = "This version has no " + list(missing, "or") + " yet. "
	}
	return strings.NewReplacer("{areas}", areas, "{missing}", lacks).Replace(promptNames.Replace(strings.TrimSpace(basePrompt)))
}

// list joins words as a sentence does: "a", "a and b", "a, b, and c".
func list(words []string, conj string) string {
	switch n := len(words); n {
	case 1:
		return words[0]
	case 2:
		return words[0] + " " + conj + " " + words[1]
	default:
		return strings.Join(words[:n-1], ", ") + ", " + conj + " " + words[n-1]
	}
}

// today is block 1's date line. It has the date and not the time, so it
// changes only on a new day, and the next cut picks it up (3.1). The
// offset stands for the time zone, as Windows has no IANA name for it.
func today(now time.Time) string {
	return fmt.Sprintf("Today is %s, %s, in the user's time zone (UTC%s).", now.Weekday(), now.Format(time.DateOnly), now.Format("-07:00"))
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
