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
	start     int                // the first turn in the window; 0 before the first cut
	stubBelow int                // tool results of turns before this are stubs
	stubbed   map[string]bool    // tool calls whose results in-turn trimming made stubs
	chatList  string             // Mother's chat list, refreshed at cuts (8.6)
	system    []provider.Block   // blocks 1–5, built at cuts
	mem       map[string]memSeen // memory sections as the chat has seen them, by target; set at cuts (3.3)
	schema    int                // the schema version the project card was built at
	cardFail  int                // the turn whose card failed: it is tried again in the next turn
	skills    map[string]bool    // the skills in block 1, from the last cut
	tools     []tool.Tool        // the tools in the requests, set at cuts (TASK-033); nil for all
	toolsAt   int                // the schema version tools was set at, while a tool waits for a table; -1 for none
	last      time.Time          // the last request
	fired     map[int]bool       // turns whose number check fired (SPIKE-032)
	tainted   []taint            // numbers the chat's write calls saved without a source, and where (SPIKE-032)
	cards     map[int]chat.Part  // history cards by the turn whose message has them, kept until a cut drops the turn (3.6); a part without text for none
	carded    map[int][]int      // the turns those cards show
}

// cutIfDue cuts the window at the start of a turn when it has passed
// history_max_turns or history_max_tokens, or when the cache has expired.
// A chat with no request yet in this process (after a restart or Clear)
// cuts too: its cache is gone as well. The system blocks are built at a
// cut and kept until the next one (3.1), so a role change takes effect
// then.
func (r *runner) cutIfDue(ctx context.Context, t *turn) error {
	if r.o.d.Plain {
		return r.plainWindow(ctx, t)
	}
	w := &r.cs.win
	c := t.set.Context
	due := r.cs.takeCut() || w.last.IsZero() || r.o.d.Now().Sub(w.last) > cacheIdle
	if !due {
		ms, _, err := r.p.Chats.Messages(ctx, t.ch.ID, w.start, t.n-1)
		if err != nil {
			return err
		}
		prev := shape(withCards(ms, w.cards), w)
		due = t.n-w.start > c.HistoryMaxTurns || provider.EstimateTokens(provider.Request{Messages: prev}) > c.HistoryMaxTokens
	}
	if !due {
		return nil
	}
	w.start, w.stubBelow, w.stubbed = max(1, t.n-c.HistoryMinTurns), t.n, map[string]bool{}
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
	w.system, w.schema = sys, schema
	r.offerTools(ctx, t)
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
	plain := r.o.d.Plain
	if !plain {
		r.refreshCard(ctx, t)
		r.refreshTools(ctx, t)
		r.memoryNotices(ctx, t)
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
	if !plain {
		r.findCards(ctx, t, ms)
		ms = withCards(ms, w.cards)
	}
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
		if tl.Spec().IsOn() { // a switch applies from the next request (8.10)
			req.Tools = append(req.Tools, toolDef(tl))
		}
	}
	if last {
		req.ToolChoice = provider.ToolsNone
		req.Messages = append(req.Messages, chat.Message{Chat: t.ch.ID, Turn: t.n, Role: chat.RoleUser,
			Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: lastRequest}}}})
	}
	w.last = r.o.d.Now()
	return req, nil
}

// Plain's window (Deps.Plain): it drops the oldest turns once the
// history passes plainFull of the model's window, until it is under
// plainKeep of it.
const (
	plainFull = 0.8
	plainKeep = 0.5
)

// plainWindow is the plain chat harness's window (Deps.Plain): every
// turn from the first, tool results as they came, and block 1 alone,
// built again when the date changes, as a chat app's system prompt has
// today's date. Only when the history nears the model's window are the
// oldest turns dropped, with no summary and no line about them.
func (r *runner) plainWindow(ctx context.Context, t *turn) error {
	w := &r.cs.win
	w.start = max(w.start, 1)
	sys := []provider.Block{{Text: r.block1(t), Name: "System prompt", Cache: true}}
	if w.system == nil || w.system[0].Text != sys[0].Text {
		w.system = sys
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
		for _, m := range ms {
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
	blocks := []provider.Block{{Text: r.block1(t), Name: "System prompt, role and skills"}}
	mem, seen := r.memoryBlocks(ctx)
	blocks = append(blocks, mem...)
	r.cs.win.mem = seen
	card, schema, _ := r.card(ctx, t)
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
	return blocks, schema, nil
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
// leaves the block out.
func (r *runner) notesBlock(ctx context.Context, t *turn) string {
	n, err := r.p.Chats.Notes(ctx, t.ch.ID)
	if err != nil {
		r.o.d.Log.Error("agent: reading the session notes", "project", r.cs.key.p, "chat", r.cs.key.c, "err", err)
		return ""
	}
	return tool.NotesBlock(n)
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

// refreshCard rebuilds the project card in the window's system blocks
// when a migration has run since it was built, by this chat or another:
// the one change between cuts that doesn't wait for the next one (3.1).
// Plain writes leave it alone, so the cached prefix holds. When the card
// fails, the old block stays and the next turn tries again.
func (r *runner) refreshCard(ctx context.Context, t *turn) {
	w := &r.cs.win
	if r.o.d.Card == nil || w.system == nil || w.cardFail == t.n {
		return
	}
	v, err := r.p.DB.SchemaVersion(ctx)
	if err != nil {
		r.cardFailed(ctx, t, err)
		return
	}
	if v == w.schema {
		return
	}
	card, v, ok := r.card(ctx, t)
	if !ok {
		return
	}
	// A new slice, as the traces of earlier requests hold the old one.
	sys := slices.Clone(w.system)
	i := slices.IndexFunc(sys, func(b provider.Block) bool { return b.Name == cardBlock })
	switch {
	case i >= 0 && card == "":
		sys = slices.Delete(sys, i, i+1)
	case i >= 0:
		sys[i].Text = card
	case card != "":
		sys = slices.Insert(sys, afterMemory(sys), provider.Block{Text: card, Name: cardBlock})
	}
	for j := range sys {
		sys[j].Cache = j == len(sys)-1
	}
	w.system, w.schema = sys, v
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
