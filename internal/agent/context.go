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
	"github.com/amir-saatchi/jenab/internal/id"
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
// the end, so provider caches and the model's thinking keep working
// (3.1): the system blocks and the tools stay as the cut set them, and a
// change reaches the model as a note at the end.
type window struct {
	start      int                // the first turn in the window; 0 before the first cut
	stubBelow  int                // tool results of turns before this are stubs
	thinkBelow int                // thinking of turns before this is left out: it was made with another start (TASK-047)
	stubbed    map[string]bool    // tool calls whose results in-turn trimming made stubs
	chatList   string             // Mother's chat list, refreshed at cuts (8.6)
	system     []provider.Block   // blocks 1–5, built at cuts and not changed until the next one
	day        string             // the date line in block 1
	date       string             // the date line the model has, in block 1 or a note
	notes      []winNote          // what changed since the cut, in the order it was told (TASK-047)
	mem        map[string]memSeen // memory sections as the chat has seen them, by target; set at cuts (3.3)
	schema     int                // the schema version of the project card the model has, in block 4 or a note
	cardFail   int                // the turn whose card failed: it is tried again in the next turn
	skills     map[string]bool    // the skills in block 1, from the last cut
	tools      []tool.Tool        // the tools in the requests, set at cuts (TASK-033); nil for all
	tablesAt   int                // the schema version at the cut while tools wait for a first table; -1 when none wait
	switches   map[string]bool    // the tools with a Settings switch and whether each was on at the cut (8.10)
	last       time.Time          // the last request
	fired      map[int]bool       // turns whose number check fired (SPIKE-032)
	tainted    []taint            // numbers the chat's write calls saved without a source, and where (SPIKE-032)
	cards      map[int]chat.Part  // history cards by the turn whose message has them, kept until a cut drops the turn (3.6); a part without text for none
	carded     map[int][]int      // the turns those cards show
}

// winNote is a message the app adds to the window between cuts: a new
// project card, a new date, the last request's line. It isn't stored; it
// goes after the message it was added after in every request until the
// next cut, which folds what it says into the system blocks. A restart
// starts with a cut, so nothing is lost.
type winNote struct {
	after id.Message // the last message when it was added
	msg   chat.Message
}

// withNotes is ms with the window's notes in their places.
func withNotes(ms []chat.Message, ns []winNote) []chat.Message {
	if len(ns) == 0 {
		return ms
	}
	out := make([]chat.Message, 0, len(ms)+len(ns))
	j := 0
	for _, m := range ms {
		for j < len(ns) && ns[j].after < m.ID {
			out = append(out, ns[j].msg)
			j++
		}
		out = append(out, m)
	}
	for _, n := range ns[j:] {
		out = append(out, n.msg)
	}
	return out
}

// cutIfDue cuts the window at the start of a turn when it has passed
// history_max_turns or history_max_tokens, or when a tool was turned on
// or off. A chat with no request yet in this process (after a restart or
// Clear) cuts too: its cache is gone. After the cache expired (idle), it
// cuts only when something the system blocks or the tool list hold has
// changed; otherwise the window and its prefix stay, and a new date comes
// as a note (TASK-047). The system blocks are built at a cut and kept
// until the next one (3.1), so a role change takes effect then.
func (r *runner) cutIfDue(ctx context.Context, t *turn) error {
	if r.o.d.Plain {
		return r.plainWindow(ctx, t)
	}
	w := &r.cs.win
	c := t.set.Context
	due := r.cs.takeCut() || w.last.IsZero() || w.system == nil || r.switched(t)
	if !due {
		ms, _, err := r.p.Chats.Messages(ctx, t.ch.ID, w.start, t.n-1)
		if err != nil {
			return err
		}
		prev := shape(withNotes(withCards(ms, w.cards), w.notes), w)
		due = t.n-w.start > c.HistoryMaxTurns || provider.EstimateTokens(provider.Request{Messages: prev}) > c.HistoryMaxTokens
	}
	if !due && r.o.d.Now().Sub(w.last) > cacheIdle {
		steady, err := r.steady(ctx, t)
		if err != nil {
			return err
		}
		due = !steady
	}
	if !due {
		return nil
	}
	w.start, w.stubBelow, w.thinkBelow, w.stubbed, w.notes = max(1, t.n-c.HistoryMinTurns), t.n, t.n, map[string]bool{}, nil
	w.dropCards()
	r.dropSkills(ctx, t)
	if t.ch.Kind == chat.KindMother {
		list, err := r.chatList(ctx)
		if err != nil {
			return err
		}
		w.chatList = list
	}
	sys, schema, err := r.system(ctx, t)
	if err != nil {
		return err
	}
	w.system, w.schema, w.day, w.date = sys, schema, today(r.o.d.Now()), today(r.o.d.Now())
	r.offerTools(ctx, t)
	w.skills = map[string]bool{}
	for _, name := range t.ch.Skills {
		if _, ok := r.o.skills().Get(t.ch.Kind, name); ok {
			w.skills[name] = true
		}
	}
	return nil
}

// steady reports whether a cut now would build the same system blocks,
// but for the date, and the same tool list (TASK-047). The project card
// counts as the same while the schema version is: its counts are "as of"
// the cut anyway (3.2). Anything else that changed, such as memory, the
// notes, a loaded skill or Mother's chat list, lets the cut go ahead.
func (r *runner) steady(ctx context.Context, t *turn) (bool, error) {
	w := &r.cs.win
	if w.schema == noCard {
		return false, nil
	}
	if r.o.d.Card != nil {
		v, err := r.p.DB.SchemaVersion(ctx)
		if err != nil || v != w.schema {
			return false, nil
		}
	}
	card := ""
	if i := slices.IndexFunc(w.system, func(b provider.Block) bool { return b.Name == cardBlock }); i >= 0 {
		card = w.system[i].Text
	}
	if t.ch.Kind == chat.KindMother {
		list, err := r.chatList(ctx)
		if err != nil {
			return false, err
		}
		if list != w.chatList {
			return false, nil
		}
	}
	sys, _ := r.blocks(ctx, t, card)
	sys[0].Text = strings.Replace(sys[0].Text, today(r.o.d.Now()), w.day, 1)
	if !slices.Equal(sys, w.system) {
		return false, nil
	}
	return slices.Equal(toolNamesOf(r.toolList(ctx, t)), toolNamesOf(w.tools)), nil
}

// request builds one request: the system blocks, the window and the
// current turn, with the window's notes. The last request of a turn keeps
// the tools, so the cached prefix holds, but asks for an answer without
// calls (8.3); its line stays in the window as a note.
func (r *runner) request(ctx context.Context, t *turn, last bool) (provider.Request, error) {
	w := &r.cs.win
	plain := r.o.d.Plain
	var changes []chat.Part
	if !plain {
		changes = r.cardChange(ctx, t)
		r.joinTableTools(ctx, t)
		r.memoryNotices(ctx, t)
		r.spikeNotices(ctx, t)
	}
	if d := today(r.o.d.Now()); w.date != "" && d != w.date {
		changes = append(changes, notePart(dateNote(d)))
		w.date = d
	}
	if last && !t.lastNoted {
		changes = append(changes, chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: lastRequest}})
		t.lastNoted = true
	}
	ms, _, err := r.p.Chats.Messages(ctx, t.ch.ID, w.start, t.n)
	if err != nil {
		return provider.Request{}, err
	}
	for _, m := range ms {
		if m.Turn == t.n && m.Role == chat.RoleUser && m.ID > t.seen {
			t.seen = m.ID
		}
	}
	if len(changes) > 0 && len(ms) > 0 {
		w.notes = append(w.notes, winNote{after: ms[len(ms)-1].ID, msg: chat.Message{Chat: t.ch.ID, Turn: t.n, Role: chat.RoleUser, Parts: changes}})
	}
	if !plain {
		r.findCards(ctx, t, ms)
		ms = withCards(ms, w.cards)
	}
	ms = withNotes(ms, w.notes)
	msgs := shape(ms, w)
	if !plain && r.trim(t, msgs) {
		msgs = shape(ms, w)
	}
	req := provider.Request{Model: t.model, System: w.system, Messages: msgs, Thinking: true, Image: r.image}
	tools := w.tools
	if plain || tools == nil {
		tools = r.o.chatTools(t.ch)
	}
	for _, tl := range tools {
		// Jenab reads a switch at the cut, plain at each request (8.10).
		if !plain && w.tools != nil || tl.Spec().IsOn() {
			req.Tools = append(req.Tools, toolDef(tl))
		}
	}
	if last {
		req.ToolChoice = provider.ToolsNone
	}
	w.last = r.o.d.Now()
	return req, nil
}

// notePart is a note's notice part. The model reads it; the chat doesn't
// store it.
func notePart(text string) chat.Part {
	return chat.Part{Kind: chat.PartNotice, Notice: &chat.Notice{Kind: chat.NoticeReminder, Text: text}}
}

// dateNote tells the model the day changed since block 1 was built
// (TASK-047). The same for every model (1).
func dateNote(today string) string { return "[the date changed: " + today + "]" }

// cardNote is a project card that changed since block 4 was built, such
// as after a migration (3.1, TASK-047). The same for every model (1).
func cardNote(card string) string {
	if card == "" {
		return "[the project card changed: there is none now]"
	}
	return "[the project card changed; this one replaces the one before]\n" + card
}

// Plain's window (Deps.Plain): it drops the oldest turns once the
// history passes plainFull of the model's window, until it is under
// plainKeep of it.
const (
	plainFull = 0.8
	plainKeep = 0.5
)

// plainWindow is the plain chat harness's window (Deps.Plain): every
// turn from the first, tool results as they came, and block 1 alone with
// the date of the first turn, as a chat app's system prompt has today's
// date. Block 1 is built again when the role or the loaded skills change;
// a new day comes as a note (TASK-047). Only when the history nears the
// model's window are the oldest turns dropped, with no summary and no
// line about them; the thinking before that point is left out then.
func (r *runner) plainWindow(ctx context.Context, t *turn) error {
	w := &r.cs.win
	w.start = max(w.start, 1)
	text := r.block1(t)
	if now := today(r.o.d.Now()); w.system == nil || w.system[0].Text != strings.Replace(text, now, w.day, 1) {
		w.system, w.day, w.date = []provider.Block{{Text: text, Name: "System prompt", Cache: true}}, now, now
	}
	if t.window <= 0 {
		return nil
	}
	ms, _, err := r.p.Chats.Messages(ctx, t.ch.ID, w.start, t.n-1)
	if err != nil {
		return err
	}
	size := func(from int) int {
		var keep []chat.Message
		for _, m := range withNotes(ms, w.notes) {
			if m.Turn >= from {
				keep = append(keep, m)
			}
		}
		return provider.EstimateTokens(provider.Request{System: w.system, Messages: shape(keep, w)})
	}
	if float64(size(w.start)) <= plainFull*float64(t.window) {
		return nil
	}
	for w.start < t.n && float64(size(w.start)) > plainKeep*float64(t.window) {
		w.start++
	}
	w.thinkBelow = t.n
	w.notes = slices.DeleteFunc(w.notes, func(n winNote) bool { return n.msg.Turn < w.start })
	return nil
}

// shape is the window's messages as the model sees them: tool results of
// turns before the last cut, and those trimmed in a turn, as stubs (3.7).
// A skill's text joins the result after it; before the last cut, a skill
// in block 1 leaves a stub (8.9). A tool call without a result, left by a
// crash, gets one, so every provider accepts the history.
func shape(ms []chat.Message, w *window) []chat.Message {
	newest := newestOfSeries(ms)
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
			// Thinking made before the last cut was made with another
			// start of the request: no provider needs it, and some leave
			// it out again in every request (3.8, TASK-047).
			if m.Turn < w.thinkBelow && slices.ContainsFunc(m.Parts, isThinking) {
				if m.Parts = slices.DeleteFunc(slices.Clone(m.Parts), isThinking); len(m.Parts) == 0 {
					continue
				}
			}
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
					switch {
					case s.Series != "" && s.Stub != "" && newest[s.Series] != s.CallID:
						s.Text = s.Stub // a newer one replaced it, such as a desktop snapshot (8.10)
					case m.Turn < w.stubBelow || w.stubbed[r.CallID]:
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
			// The number check's mark is for the user only (SPIKE-032),
			// and so is a refresh's card. History cards' mark is for the
			// app only (3.6).
			hidden := func(p chat.Part) bool { return notForModel(p) || cardMark(p) }
			if slices.ContainsFunc(m.Parts, hidden) {
				if m.Parts = slices.DeleteFunc(slices.Clone(m.Parts), hidden); len(m.Parts) == 0 {
					continue
				}
			}
			// A quote is plain text to the model (TASK-019).
			if i := slices.IndexFunc(m.Parts, func(p chat.Part) bool { return p.Quote != nil }); i >= 0 {
				m.Parts = slices.Clone(m.Parts)
				m.Parts[i] = quotePart(m.Chat, m.Parts[i].Quote)
			}
		}
		out = append(out, m)
	}
	closeOpen()
	return out
}

func isThinking(p chat.Part) bool { return p.Thinking != nil }

// newestOfSeries is the call id of each series' newest result in ms.
func newestOfSeries(ms []chat.Message) map[string]string {
	var out map[string]string
	for _, m := range ms {
		for _, p := range m.Parts {
			if r := p.ToolResult; r != nil && r.Series != "" {
				if out == nil {
					out = map[string]string{}
				}
				out[r.Series] = r.CallID
			}
		}
	}
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

// system is blocks 1–5 of the context (3.1) for the window, and the
// schema version its project card was built at. It has the system
// prompt, the date, the role, the skills, memory, the project card,
// Mother's chat list and the session notes. The cache point is after the
// last block.
func (r *runner) system(ctx context.Context, t *turn) ([]provider.Block, int, error) {
	card, schema, _ := r.card(ctx, t)
	blocks, seen := r.blocks(ctx, t, card)
	r.cs.win.mem = seen
	return blocks, schema, nil
}

// blocks are blocks 1–5 with the given card, and the memory sections
// they show.
func (r *runner) blocks(ctx context.Context, t *turn, card string) ([]provider.Block, map[string]memSeen) {
	blocks := []provider.Block{{Text: r.block1(t), Name: "System prompt, role and skills"}}
	mem, seen := r.memoryBlocks(ctx)
	blocks = append(blocks, mem...)
	if card != "" {
		blocks = append(blocks, provider.Block{Text: card, Name: cardBlock})
	}
	w := &r.cs.win
	if t.ch.Kind == chat.KindMother {
		blocks = append(blocks, provider.Block{Text: w.chatList, Name: "Chat list"})
	}
	if notes := r.notesBlock(ctx, t); notes != "" {
		blocks = append(blocks, provider.Block{Text: notes, Name: notesBlockName})
	}
	if n := w.start - 1; n > 0 {
		blocks = append(blocks, provider.Block{Text: earlierTurns(n), Name: "Earlier turns"})
	}
	blocks[len(blocks)-1].Cache = true
	return blocks, seen
}

// block1 is block 1 (3.1): the system prompt with today's date, the role,
// the loaded skills and the skill list.
func (r *runner) block1(t *turn) string {
	tools := r.o.chatTools(t.ch)
	var b strings.Builder
	b.WriteString(baseOf(r.o.d.Tools.For(t.ch.Kind), tools))
	if slices.ContainsFunc(tools, func(tl tool.Tool) bool { return tl.Spec().Background }) {
		b.WriteString("\n\n" + r.o.rule())
	}
	if r.o.d.Grounding.Compute && slices.ContainsFunc(tools, func(tl tool.Tool) bool { return tl.Spec().Name == "query" }) {
		b.WriteString("\n\n" + computeLine) // SPIKE-032 round 2, spike-only
	}
	if f := r.o.d.ToolIndex; f != nil {
		if s := strings.TrimSpace(f(t.ch.Kind)); s != "" {
			b.WriteString("\n\n" + s) // SPIKE-028, spike-only
		}
	}
	if t.ch.Kind == chat.KindMother {
		b.WriteString("\n\n" + promptNames.Replace(strings.TrimSpace(motherPrompt)))
		if slices.ContainsFunc(tools, func(tl tool.Tool) bool { return tl.Spec().Name == "create_chat" }) {
			b.WriteString("\n\n" + delegation(r.o.d.DelegateLine))
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
	return b.String()
}

// cardBlock is the project card's block name, as the turn inspector shows
// it.
const cardBlock = "Project card"

// notesBlockName is block 5's name, as the turn inspector shows it.
const notesBlockName = "Session notes"

// notesBlock is block 5, the chat's session notes (3.4), as they are at
// the cut. The chat's own edits until the next cut are in its turns as
// tool calls, so they need no notice; Refresh memory ends with a cut
// (3.5). Like memory, the notes are best effort: a failure is logged and
// leaves the block out. Deps.Notes, when set, gives the block instead
// (SPIKE-034, spike-only).
func (r *runner) notesBlock(ctx context.Context, t *turn) string {
	if f := r.o.d.Notes; f != nil {
		s, err := f(ctx, r.p, t.ch)
		if err != nil {
			r.o.d.Log.Error("agent: building the notes block", "project", r.cs.key.p, "chat", r.cs.key.c, "err", err)
			return ""
		}
		return strings.TrimSpace(s)
	}
	n, err := r.p.Chats.Notes(ctx, t.ch.ID)
	if err != nil {
		r.o.d.Log.Error("agent: reading the session notes", "project", r.cs.key.p, "chat", r.cs.key.c, "err", err)
		return ""
	}
	return tool.NotesBlock(n)
}

// NoticeSessionUpdate is a sub-session's reply sent up to its parent
// through Deps.Notices (SPIKE-034 round 2, spike-only).
const NoticeSessionUpdate chat.NoticeKind = "session_update"

// spikeNotices writes Deps.Notices' notices into turn t, as one user
// message like the memory-changed notices. It is best effort.
func (r *runner) spikeNotices(ctx context.Context, t *turn) {
	f := r.o.d.Notices
	if f == nil {
		return
	}
	ns, err := f(ctx, r.p, t.ch)
	if err != nil {
		r.o.d.Log.Error("agent: reading the spike's notices", "project", r.cs.key.p, "chat", r.cs.key.c, "err", err)
		return
	}
	if len(ns) == 0 {
		return
	}
	parts := make([]chat.Part, len(ns))
	for i := range ns {
		parts[i] = chat.Part{Kind: chat.PartNotice, Notice: &ns[i]}
	}
	if _, err := r.o.write(ctx, r.cs, r.p, chat.Message{Chat: t.ch.ID, Turn: t.n, Role: chat.RoleUser, Parts: parts}); err != nil {
		r.o.d.Log.Error("agent: writing the spike's notices", "project", r.cs.key.p, "chat", r.cs.key.c, "err", err)
	}
}

// noCard is the schema version of a card that failed, so the next turn
// tries again.
const noCard = -1

// card is the project card (3.2), and the schema version read before it
// was built, so a migration that lands meanwhile rebuilds it again. The
// card is best effort: a failure is logged, and ok is false.
func (r *runner) card(ctx context.Context, t *turn) (card string, schema int, ok bool) {
	f := r.o.d.Card
	if f == nil {
		return "", 0, true
	}
	v, err := r.p.DB.SchemaVersion(ctx)
	if err == nil {
		if card, err = f(ctx, r.p); err == nil {
			r.cardWorks()
			return strings.TrimSpace(card), v, true
		}
	}
	r.cardFailed(ctx, t, err)
	return "", noCard, false
}

// cardFailed keeps the card from being tried again in turn t, and logs
// the error once per project until a card works again: a cause that
// lasts would otherwise log on every turn of every chat.
func (r *runner) cardFailed(ctx context.Context, t *turn, err error) {
	r.cs.win.cardFail = t.n
	if ctx.Err() != nil {
		return
	}
	r.o.mu.Lock()
	logged := r.o.cardDown[r.p.ID]
	r.o.cardDown[r.p.ID] = true
	r.o.mu.Unlock()
	if !logged {
		r.o.d.Log.Error("agent: building the project card", "project", r.cs.key.p, "chat", r.cs.key.c, "err", err)
	}
}

func (r *runner) cardWorks() {
	r.o.mu.Lock()
	delete(r.o.cardDown, r.p.ID)
	r.o.mu.Unlock()
}

// cardChange is a note part with the project card when a migration has
// run since the model got the card, by this chat or another: the system
// blocks stay as they are, so the cached prefix and the model's thinking
// hold (3.1, TASK-047). Plain writes leave it alone; the card's counts
// are "as of" the cut. When the card fails, nothing is told and the next
// turn tries again.
func (r *runner) cardChange(ctx context.Context, t *turn) []chat.Part {
	w := &r.cs.win
	if r.o.d.Card == nil || w.system == nil || w.cardFail == t.n {
		return nil
	}
	v, err := r.p.DB.SchemaVersion(ctx)
	if err != nil {
		r.cardFailed(ctx, t, err)
		return nil
	}
	if v == w.schema {
		return nil
	}
	card, v, ok := r.card(ctx, t)
	if !ok {
		return nil
	}
	w.schema = v
	return []chat.Part{notePart(cardNote(card))}
}

// areaText is how block 1 names each area (tool.AreaNames' order).
var areaText = []string{"tables in a local database", "pipelines that fetch data", "views"}

// base is the system prompt's start. It names the parts of a project the
// tools work on, and says the others aren't in this version, so the model
// doesn't build them from files (TASK-014).
func base(tools []tool.Tool) string { return baseOf(tools, tools) }

// baseOf is base with the areas of the registered tools and the other
// lines of the chat's own: they differ only under Deps.ChatTools
// (SPIKE-044), where a chat without a pipeline tool is still told the
// project has pipelines.
func baseOf(registered, tools []tool.Tool) string {
	var has tool.Areas
	for _, tl := range registered {
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
	answer := ""
	if slices.ContainsFunc(tools, func(tl tool.Tool) bool { return tl.Spec().Answers }) {
		answer = "\n" + answerLine
	}
	return strings.NewReplacer("{areas}", areas, "{missing}", lacks, "{answer}", answer).Replace(promptNames.Replace(strings.TrimSpace(basePrompt)))
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
		s := firstLine(notes.Content)
		if s == "" {
			s = firstLine(notes.Summary)
		}
		if s != "" {
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

// delegation is Mother's delegation guidance, with SPIKE-044's line after
// its first item when the hook sets one.
func delegation(line string) string {
	g := strings.TrimSpace(delegatePrompt)
	line = strings.TrimSpace(line)
	if line == "" {
		return g
	}
	i := strings.Index(g, "\n- ")
	if i < 0 {
		return g + "\n- " + line
	}
	j := strings.Index(g[i+1:], "\n")
	if j < 0 {
		return g + "\n- " + line
	}
	j += i + 1
	return g[:j] + "\n- " + line + g[j:]
}
