package agent

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/appinfo"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// Refresh memory (SPEC 3.5). The messages after the chat's progress are
// split at turn boundaries into chunks; one background call per chunk
// lists what a later chat should know (map); one call merges the items,
// compares them with memory, writes the sections' new content and the
// notes' summary (merge). The edits are applied as one batch with one
// chip, after a card where the level or a section the user wrote asks for
// one; then the progress moves on and the chat's next turn cuts its
// window. Nothing is applied before the merge has finished, so a cancel
// before it changes nothing but the saved chunks.
//
// While a refresh runs the chat is busy: a message queues its turn until
// the refresh ends, Stop cancels it, and Clear and Delete are refused.

var (
	//go:embed prompts/refresh_map.md
	refreshMapPrompt string
	//go:embed prompts/refresh_merge.md
	refreshMergePrompt string
)

var (
	ErrRefreshing = errors.New("agent: memory is being refreshed")
	// ErrNothingToRefresh: the chat has no messages after its progress.
	ErrNothingToRefresh = errors.New("agent: there are no new turns to review")
)

const (
	chunkTurns         = 5     // turns per map call at most
	chunkTokens        = 12000 // a map call's conversation part, at most
	contextTokens      = 2000  // the turn before it, at most
	refreshMapTokens   = 4096
	refreshMergeTokens = 8192
	refreshTries       = 3   // tries of a call that fails on a retryable provider error
	reviewResultMax    = 200 // characters of a small tool result in a chunk
	reviewArgsMax      = 300 // characters of a call's arguments in a chunk
	// A refresh is long, and asks before it starts, with more map calls
	// than this or more input tokens.
	longCalls  = 2
	longTokens = 20000
)

var mapSchema = mustSchema("refresh_map", `{
	"type": "object",
	"properties": {
		"items": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"kind": {"type": "string", "enum": ["goal", "preference", "decision", "convention", "open_question", "progress"]},
					"text": {"type": "string", "minLength": 1},
					"scope": {"type": "string", "enum": ["user", "project", "chat"]},
					"source": {"type": "array", "items": {"type": "string"}}
				},
				"required": ["kind", "text", "scope", "source"]
			}
		}
	},
	"required": ["items"]
}`)

var mergeSchema = mustSchema("refresh_merge", `{
	"type": "object",
	"properties": {
		"items": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"kind": {"type": "string", "enum": ["goal", "preference", "decision", "convention", "open_question", "progress"]},
					"text": {"type": "string", "minLength": 1},
					"scope": {"type": "string", "enum": ["user", "project", "chat"]},
					"from": {"type": "array", "items": {"type": "string"}},
					"label": {"type": "string", "enum": ["present", "missing", "contradicts", "outdated"]}
				},
				"required": ["kind", "text", "scope", "from", "label"]
			}
		},
		"edits": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"scope": {"type": "string", "enum": ["user", "project"]},
					"section": {"type": "string", "minLength": 1},
					"content": {"type": "string"}
				},
				"required": ["scope", "section", "content"]
			}
		},
		"summary": {"type": "string"}
	},
	"required": ["items", "edits", "summary"]
}`)

// mergeAnswer is mergeSchema as the prompt shows it, checked leniently:
// the items help the model label and merge, but nothing reads them, so a
// wrong or missing list never fails a refresh whose edits and summary are
// right.
var mergeAnswer = jsonSchema{text: mergeSchema.text, sch: mustSchema("refresh_merge_check", `{
	"type": "object",
	"properties": {
		"edits": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"scope": {"type": "string", "enum": ["user", "project"]},
					"section": {"type": "string", "minLength": 1},
					"content": {"type": "string"}
				},
				"required": ["scope", "section", "content"]
			}
		},
		"summary": {"type": "string"}
	},
	"required": ["edits", "summary"]
}`).sch}

// RefreshOptions are a refresh's options. Full discards the progress and
// the saved chunks, and reviews the whole chat.
type RefreshOptions struct {
	Full bool `json:"full"`
}

// RefreshEstimate is what a refresh would cost, for the dialog before a
// long one.
type RefreshEstimate struct {
	Turns  int    `json:"turns"`  // turns to review
	Chunks int    `json:"chunks"` // the chunks they make
	Saved  int    `json:"saved"`  // chunks a cancelled refresh already reviewed
	Calls  int    `json:"calls"`  // the calls to make: a map call per chunk not saved, and the merge
	Tokens int    `json:"tokens"` // the calls' input, estimated
	Model  string `json:"model"`  // the model they use
	Long   bool   `json:"long"`   // ask before starting
}

// refreshState is a chat's refresh. cs.mu guards it.
type refreshState struct {
	on     bool
	cancel context.CancelCauseFunc
	// cutNext makes the next turn cut its window, so it reads memory and
	// the notes as the refresh left them.
	cutNext bool
}

// takeCut reports whether a refresh asked for a cut, and clears it.
func (cs *chatState) takeCut() bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cut := cs.refresh.cutNext
	cs.refresh.cutNext = false
	return cut
}

// stopRefresh cancels the chat's refresh. cs.mu is held.
func (cs *chatState) stopRefresh() {
	if cs.refresh.on && cs.refresh.cancel != nil {
		cs.refresh.cancel(errStopped)
	}
}

// notForModel is a part of a user message the model doesn't get: the
// number check's mark (SPIKE-032) and a refresh's card.
func notForModel(p chat.Part) bool { return isMark(p) || p.Approval != nil }

// refreshCard reports whether m is a refresh's card. It is added to the
// chat's last turn after the turn ended, so, like an aside notice, it
// doesn't say how that turn ended.
func refreshCard(m chat.Message) bool {
	return len(m.Parts) > 0 && !slices.ContainsFunc(m.Parts, func(p chat.Part) bool {
		return p.Approval == nil || id.Source(p.Approval.Target).Kind() != "refresh"
	})
}

// EstimateRefresh is what Refresh would do now.
func (o *Orchestrator) EstimateRefresh(ctx context.Context, p id.Project, c id.Chat, opt RefreshOptions) (RefreshEstimate, error) {
	if err := o.enter(); err != nil {
		return RefreshEstimate{}, err
	}
	defer o.leave()
	proj, err := o.d.Projects.Open(ctx, p)
	if err != nil {
		return RefreshEstimate{}, err
	}
	defer proj.Release()
	cs := o.state(p, c)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if err := o.load(ctx, cs, proj); err != nil {
		return RefreshEstimate{}, err
	}
	pl, err := o.planRefresh(ctx, proj, c, opt.Full, cs.turn)
	if err != nil {
		return RefreshEstimate{}, err
	}
	e := RefreshEstimate{Turns: pl.turns, Chunks: len(pl.chunks), Model: pl.model}
	mapPrompt := len(mapSystem()) / 4
	for _, k := range pl.chunks {
		if k.saved != nil {
			e.Saved++
			continue
		}
		e.Calls++
		e.Tokens += mapPrompt + (len(k.context)+len(k.text))/4
	}
	// The merge: its prompt, memory, the notes and some items per chunk.
	e.Calls++
	e.Tokens += len(mergeSystem(store.MaxSummaryTokens))/4 + store.ScopeUser.Cap() + store.ScopeProject.Cap() + store.MaxNotesTokens/2 + 300*len(pl.chunks)
	e.Long = e.Calls-1 > longCalls || e.Tokens > longTokens
	return e, nil
}

// Refresh starts a refresh of the chat's memory in the background. It
// fails with ErrInTurn while the chat is in a turn, ErrRefreshing while a
// refresh runs, and ErrNothingToRefresh when no message came after the
// last one.
func (o *Orchestrator) Refresh(ctx context.Context, p id.Project, c id.Chat, opt RefreshOptions) error {
	if err := o.enter(); err != nil {
		return err
	}
	defer o.leave()
	proj, err := o.d.Projects.Open(ctx, p)
	if err != nil {
		return err
	}
	defer func() {
		if proj != nil {
			proj.Release()
		}
	}()
	cs := o.state(p, c)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if err := o.load(ctx, cs, proj); err != nil {
		return err
	}
	switch {
	case cs.refresh.on:
		return ErrRefreshing
	case cs.running || cs.queued:
		return ErrInTurn
	}
	pl, err := o.planRefresh(ctx, proj, c, opt.Full, cs.turn)
	if err != nil {
		return err
	}
	rctx, cancel := context.WithCancelCause(o.ctx)
	cs.refresh = refreshState{on: true, cancel: cancel, cutNext: cs.refresh.cutNext} // a last refresh's cut still holds
	rf := &refresh{o: o, cs: cs, p: proj, plan: pl, full: opt.Full}
	for _, k := range pl.chunks {
		if k.saved == nil {
			rf.calls++
		}
	}
	rf.calls++ // the merge
	cs.doing = rf.progress()
	o.mu.Lock()
	o.add()
	o.mu.Unlock()
	go rf.run(rctx, cancel) // the chat's refresh, owned by the Orchestrator
	proj = nil
	o.publishStatus(cs)
	return nil
}

// refreshPlan is what a refresh reviews.
type refreshPlan struct {
	ch     chat.Chat
	model  string
	turn   int        // the chat's last turn at the start; the card and the chip go there
	upTo   id.Message // the last message reviewed
	turns  int
	chunks []chunk
	// untrusted: the reviewed turns, or the turn given as context, hold
	// results of tools that read untrusted content (8.8), so the refresh
	// asks before it saves edits.
	untrusted bool
}

// chunk is one map call's part of the chat.
type chunk struct {
	first, last id.Message
	text        string
	context     string                // the turn before, read-only (SPEC 3.5 step 2)
	labels      map[string]id.Message // m1, m2… to the messages
	saved       *store.ReviewChunk
}

// reviewTurn is a turn's messages as a chunk shows them.
type reviewTurn struct {
	n    int
	msgs []reviewMsg
}

type reviewMsg struct {
	id    id.Message
	lines []string
}

func (t reviewTurn) size() int {
	n := 0
	for _, m := range t.msgs {
		for _, l := range m.lines {
			n += len(l) + 5
		}
	}
	return n / 4
}

// planRefresh reads what a refresh would review. cs.mu is held.
func (o *Orchestrator) planRefresh(ctx context.Context, proj *project.Project, c id.Chat, full bool, last int) (refreshPlan, error) {
	ch, err := proj.Chats.Chat(ctx, c)
	if err != nil {
		return refreshPlan{}, err
	}
	// The store's last turn too: the scenario runner writes fixture turns
	// there directly.
	stored, err := proj.Chats.LastTurn(ctx, c)
	if err != nil {
		return refreshPlan{}, err
	}
	last = max(last, stored)
	pl := refreshPlan{ch: ch, turn: last, model: "fast"}
	if _, _, err := o.d.Models.Resolve(pl.model); err != nil {
		pl.model = ch.Model
	}
	var st store.ReviewState
	if !full {
		if st, err = proj.Chats.Review(ctx, c); err != nil {
			return pl, err
		}
	}
	if last < 1 {
		return pl, ErrNothingToRefresh
	}
	ms, _, err := proj.Chats.Messages(ctx, c, max(1, st.UpToTurn), last)
	if err != nil {
		return pl, err
	}
	var turns []reviewTurn
	var before reviewTurn    // the turn the last refresh ended in, as the first chunk's context
	var shown []chat.Message // what the map calls see: the context turn and the reviewed turns
	for _, m := range ms {
		lines := reviewLines(m)
		if m.ID <= st.UpTo {
			if m.Turn == st.UpToTurn {
				shown = append(shown, m)
				if len(lines) > 0 {
					before.n = m.Turn
					before.msgs = append(before.msgs, reviewMsg{id: m.ID, lines: lines})
				}
			}
			continue
		}
		pl.upTo = m.ID
		shown = append(shown, m)
		if len(lines) == 0 {
			continue
		}
		if len(turns) == 0 || turns[len(turns)-1].n != m.Turn {
			turns = append(turns, reviewTurn{n: m.Turn})
		}
		t := &turns[len(turns)-1]
		t.msgs = append(t.msgs, reviewMsg{id: m.ID, lines: lines})
	}
	if len(turns) == 0 {
		return pl, ErrNothingToRefresh
	}
	pl.turns = len(turns)
	pl.untrusted = o.untrustedTurn(shown) > 0
	saved := map[id.Message]store.ReviewChunk{}
	for _, k := range st.Chunks {
		saved[k.First] = k
	}
	prev := before
	for _, group := range splitTurns(turns) {
		k := buildChunk(group)
		k.context, prev = contextText(prev), group[len(group)-1]
		if s, ok := saved[k.first]; ok && s.Last == k.last {
			k.saved = &s
		}
		pl.chunks = append(pl.chunks, k)
	}
	return pl, nil
}

// splitTurns groups turns into chunks of at most chunkTurns turns and
// chunkTokens tokens. A turn over chunkTokens is a chunk of its own.
func splitTurns(turns []reviewTurn) [][]reviewTurn {
	var out [][]reviewTurn
	var cur []reviewTurn
	size := 0
	for _, t := range turns {
		n := t.size()
		if len(cur) > 0 && (len(cur) == chunkTurns || size+n > chunkTokens) {
			out = append(out, cur)
			cur, size = nil, 0
		}
		cur = append(cur, t)
		size += n
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// buildChunk labels the messages of turns and writes the chunk's text. A
// turn over chunkTokens is cut.
func buildChunk(turns []reviewTurn) chunk {
	k := chunk{labels: map[string]id.Message{}}
	var b strings.Builder
	for _, t := range turns {
		var tb strings.Builder
		fmt.Fprintf(&tb, "[turn %d]\n", t.n)
		for _, m := range t.msgs {
			if k.first == "" {
				k.first = m.id
			}
			k.last = m.id
			label := fmt.Sprintf("m%d", len(k.labels)+1)
			k.labels[label] = m.id
			for _, l := range m.lines {
				tb.WriteString(label + " " + l + "\n")
			}
		}
		s := tb.String()
		if len(s)/4 > chunkTokens {
			s = cutBytes(s, chunkTokens*4) + "\n[… the rest of this turn is left out]\n"
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(s)
	}
	k.text = b.String()
	return k
}

// contextText is a turn as a chunk's context shows it, without labels,
// cut at contextTokens; "" for no turn.
func contextText(t reviewTurn) string {
	if len(t.msgs) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[turn %d]\n", t.n)
	for _, m := range t.msgs {
		for _, l := range m.lines {
			b.WriteString(l + "\n")
		}
	}
	s := b.String()
	if len(s)/4 > contextTokens {
		s = cutBytes(s, contextTokens*4) + "\n[… the rest of this turn is left out]\n"
	}
	return s
}

// reviewLines are a message as a chunk shows it: the user's and the
// assistant's text, tool calls with their arguments cut, results as their
// stubs or cut, and answered questions. Thinking, cards and notices are
// left out.
func reviewLines(m chat.Message) []string {
	var out []string
	for _, p := range m.Parts {
		switch {
		case p.Text != nil && strings.TrimSpace(p.Text.Text) != "":
			who := "assistant"
			if m.Role == chat.RoleUser {
				who = "user"
			}
			out = append(out, who+": "+p.Text.Text)
		case p.Image != nil:
			s := "user: [image]"
			if p.Image.Alt != "" {
				s = "user: [image: " + p.Image.Alt + "]"
			}
			out = append(out, s)
		case p.ToolCall != nil:
			out = append(out, "assistant calls "+p.ToolCall.Name+" "+cutRunes(string(p.ToolCall.Args), reviewArgsMax))
		case p.ToolResult != nil:
			r := p.ToolResult
			s := tool.Stub(*r)
			if r.Ref == "" {
				s = cutRunes(r.Text, reviewResultMax)
			}
			if r.IsError {
				s = "error: " + s
			}
			out = append(out, "tool result: "+s)
		case p.Question != nil && (len(p.Question.Answers) > 0 || p.Question.Note != ""):
			var qs []string
			for _, it := range p.Question.Questions {
				if a := p.Question.Answers[it.Header]; len(a) > 0 {
					qs = append(qs, it.Question+" "+strings.Join(a, ", "))
				}
			}
			if p.Question.Note != "" {
				qs = append(qs, p.Question.Note)
			}
			out = append(out, "user answers: "+strings.Join(qs, "; "))
		}
	}
	return out
}

// cutRunes is the start of s, at most n characters, with … where it was
// cut.
func cutRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// refresh is one run of Refresh memory.
type refresh struct {
	o     *Orchestrator
	cs    *chatState
	p     *project.Project
	plan  refreshPlan
	full  bool
	calls int // the calls to make
	done  int // the calls made
	// notesFull: the notes left no room for a summary, so none was asked
	// for.
	notesFull bool
}

// reviewItem is a map call's item, with its messages.
type reviewItem struct {
	Kind   string       `json:"kind"`
	Text   string       `json:"text"`
	Scope  string       `json:"scope"`
	Source []id.Message `json:"source"`
}

// mergeOut is the merge call's answer; its items are left unread.
type mergeOut struct {
	Edits   []mergeEdit `json:"edits"`
	Summary string      `json:"summary"`
}

type mergeEdit struct {
	Scope   string `json:"scope"`
	Section string `json:"section"`
	Content string `json:"content"`
}

// plannedEdit is a merge edit checked against memory.
type plannedEdit struct {
	scope     store.MemoryScope
	before    string
	userWrote bool // the user wrote the section last
	next      store.MemorySection
	expected  int
}

func (rf *refresh) source() id.Source { return id.SourceOf("refresh", string(rf.plan.ch.ID)) }

func (rf *refresh) progress() string {
	return fmt.Sprintf("refreshing memory: %d of %d calls", rf.done, rf.calls)
}

func (rf *refresh) setDoing(s string) {
	rf.cs.mu.Lock()
	defer rf.cs.mu.Unlock()
	rf.cs.doing = s
	rf.o.publishStatus(rf.cs)
}

func (rf *refresh) called() {
	rf.cs.mu.Lock()
	rf.done++
	s := rf.progress()
	rf.cs.mu.Unlock()
	rf.setDoing(s)
}

// run is the refresh's goroutine. Stop and the project's close cancel
// it. At its end, a turn queued meanwhile starts.
func (rf *refresh) run(ctx context.Context, cancel context.CancelCauseFunc) {
	o, cs, proj := rf.o, rf.cs, rf.p
	defer o.leave()
	handed := false
	defer func() {
		if !handed {
			proj.Release()
		}
	}()
	done := make(chan struct{})
	defer proj.OnClose(func() {
		cancel(errClosed)
		select {
		case <-done:
		case <-time.After(closeWait):
		}
	})()
	defer close(done)
	defer func() {
		if r := recover(); r != nil {
			o.d.Log.Error("agent: panic in Refresh memory", "project", proj.ID, "chat", cs.key.c, "panic", r)
		}
		closed := errors.Is(context.Cause(ctx), errClosed) || o.ctx.Err() != nil
		cancel(nil)
		cs.mu.Lock()
		defer cs.mu.Unlock()
		cs.refresh.on, cs.refresh.cancel, cs.doing = false, nil, ""
		if cs.queued && !cs.runner {
			if closed {
				cs.queued = false
			} else {
				o.startRunner(cs, proj)
				handed = true
			}
		}
		o.publishStatus(cs)
	}()
	var ee *endError
	if err := rf.refresh(ctx); err != nil && (ctx.Err() == nil || errors.As(err, &ee)) {
		o.d.Log.Warn("agent: Refresh memory", "project", proj.ID, "chat", cs.key.c, "err", err)
		rf.failed(ctx, err)
	}
}

// refresh maps, merges, asks if it must, applies and finishes. A Full
// refresh's progress is replaced only when it finishes, so a cancelled
// one leaves the last.
func (rf *refresh) refresh(ctx context.Context) error {
	proj, c := rf.p, rf.plan.ch.ID
	items, err := rf.mapAll(ctx)
	if err != nil {
		return err
	}
	notes, err := proj.Chats.Notes(ctx, c)
	if err != nil {
		return err
	}
	mem, err := rf.memory(ctx)
	if err != nil {
		return err
	}
	var out mergeOut
	var edits []plannedEdit
	var dropped []string
	if len(items) > 0 {
		if out, edits, dropped, err = rf.merge(ctx, items, notes, mem); err != nil {
			return err
		}
	}
	rf.called()
	if err := context.Cause(ctx); ctx.Err() != nil {
		return err
	}
	approved, asked := true, false
	if len(edits) > 0 {
		// Memory may have changed while the merge ran: the card shows, and
		// the batch writes, what memory has now.
		var stale []string
		if edits, stale, err = rf.replan(ctx, edits); err != nil {
			return err
		}
		dropped = append(dropped, stale...)
	}
	if len(edits) > 0 {
		ask, err := rf.mustAsk(ctx, edits)
		if err != nil {
			return err
		}
		if ask {
			rf.setDoing("refreshing memory: waiting for your answer")
			if approved, err = rf.confirm(ctx, edits); err != nil {
				return err
			}
			asked = true
		}
	}
	// An answered card is kept to, though Stop came with the answer: the
	// card says Saved or Denied.
	if !asked && ctx.Err() != nil {
		return context.Cause(ctx)
	}
	// From here the batch is written whole, even if Stop comes.
	wctx, cancel := writing(ctx)
	defer cancel()
	var applied []chat.MemoryDiff
	if approved {
		var skipped []string
		applied, skipped = rf.apply(wctx, edits)
		dropped = append(dropped, skipped...)
	}
	if err := rf.finish(wctx, out.Summary, notes, applied, dropped, !approved); err != nil {
		return &endError{applied: applied, err: err}
	}
	return nil
}

// endError is a refresh that failed after the card was answered or the
// batch was written. Its notice is written though Stop came, and says
// what changed, with the batch for the undo.
type endError struct {
	applied []chat.MemoryDiff
	err     error
}

func (e *endError) Error() string { return e.err.Error() }
func (e *endError) Unwrap() error { return e.err }

// memory reads both memories, tombstones too.
func (rf *refresh) memory(ctx context.Context) ([]store.MemorySection, error) {
	var all []store.MemorySection
	if rf.p.Registry != nil {
		u, err := rf.p.Registry.UserMemory(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, u...)
	}
	p, err := rf.p.DB.Memory(ctx)
	if err != nil {
		return nil, err
	}
	return append(all, p...), nil
}

// mapAll runs the map calls of the chunks not saved, as the Gate lets
// them, and keeps each result as its call ends. It returns the items in
// chat order.
func (rf *refresh) mapAll(ctx context.Context) ([]reviewItem, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	chunks := rf.plan.chunks
	out := make([][]reviewItem, len(chunks))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	for i, k := range chunks {
		if k.saved != nil {
			var r struct{ Items []reviewItem }
			if json.Unmarshal([]byte(k.saved.Result), &r) == nil {
				out[i] = r.Items
				continue
			}
		}
		wg.Go(func() {
			items, err := rf.mapChunk(ctx, k)
			mapped()
			if err == nil {
				b, _ := json.Marshal(struct {
					Items []reviewItem `json:"items"`
				}{items})
				// A finished chunk is kept, though Stop came meanwhile (D6).
				wctx, wcancel := writing(ctx)
				err = rf.p.Chats.SaveReviewChunk(wctx, rf.plan.ch.ID, store.ReviewChunk{First: k.first, Last: k.last, Result: string(b)})
				wcancel()
			}
			if err != nil {
				mu.Lock()
				if first == nil {
					first = err
					cancel(err)
				}
				mu.Unlock()
				return
			}
			out[i] = items
			rf.called()
		})
	}
	wg.Wait()
	if first != nil {
		return nil, first
	}
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	return slices.Concat(out...), nil
}

// mapped runs as a map call ends, before its chunk is saved; tests stop
// the refresh there.
var mapped = func() {}

func mapSystem() string {
	return strings.NewReplacer("{app}", appinfo.Name, "{schema}", mapSchema.text).Replace(strings.TrimSpace(refreshMapPrompt))
}

// mapChunk asks for a chunk's items.
func (rf *refresh) mapChunk(ctx context.Context, k chunk) ([]reviewItem, error) {
	var out struct {
		Items []struct {
			Kind   string   `json:"kind"`
			Text   string   `json:"text"`
			Scope  string   `json:"scope"`
			Source []string `json:"source"`
		} `json:"items"`
	}
	in := "Conversation part:\n\n" + k.text
	if k.context != "" {
		in = "The turn before the part, for context only:\n\n" + k.context + "\n" + in
	}
	if err := rf.jsonCall(ctx, mapSystem(), in, mapSchema, refreshMapTokens, &out, nil); err != nil {
		return nil, fmt.Errorf("reviewing the turns from message %s: %w", k.first, err)
	}
	items := []reviewItem{}
	for _, it := range out.Items {
		r := reviewItem{Kind: it.Kind, Text: strings.TrimSpace(it.Text), Scope: it.Scope, Source: []id.Message{}}
		for _, l := range it.Source {
			if m, ok := k.labels[strings.TrimSpace(l)]; ok && !slices.Contains(r.Source, m) {
				r.Source = append(r.Source, m)
			}
		}
		if r.Text != "" {
			items = append(items, r)
		}
	}
	return items, nil
}

// mergeSystem is the merge's prompt. With no room for a summary it asks
// for none.
func mergeSystem(summaryCap int) string {
	rule := "The chat's session notes are full, so write no summary: leave summary empty."
	if summaryCap > 0 {
		rule = fmt.Sprintf("Rewrite the chat's summary from the old summary and the items: what this conversation did and decided, and what is still open. At most %d tokens, in short sentences, in the language of the items.", summaryCap)
	}
	return strings.NewReplacer("{app}", appinfo.Name, "{schema}", mergeSchema.text,
		"{user_cap}", fmt.Sprint(store.ScopeUser.Cap()), "{project_cap}", fmt.Sprint(store.ScopeProject.Cap()),
		"{summary_rule}", rule).Replace(strings.TrimSpace(refreshMergePrompt))
}

// summaryRoom is how long the summary may be, in tokens: its own cap, and
// what the notes leave of theirs.
func summaryRoom(n chat.SessionNote) int {
	return max(0, min(store.MaxSummaryTokens, store.MaxNotesTokens-len(n.Content)/4))
}

// merge asks for the merged items, the edits and the summary, and checks
// the edits against memory. Edits still wrong after the retry are
// dropped, and named.
func (rf *refresh) merge(ctx context.Context, items []reviewItem, notes chat.SessionNote, mem []store.MemorySection) (mergeOut, []plannedEdit, []string, error) {
	var out mergeOut
	room := summaryRoom(notes)
	check := func() []string {
		_, problems := rf.planEdits(mem, out.Edits)
		if t := len(out.Summary) / 4; room > 0 && t > room && len(out.Summary) > len(notes.Summary) {
			problems = append(problems, fmt.Sprintf("- summary: %d tokens, over its limit of %d; make it shorter", t, room))
		}
		return problems
	}
	if err := rf.jsonCall(ctx, mergeSystem(room), mergeInput(mem, notes.Summary, items), mergeAnswer, refreshMergeTokens, &out, check); err != nil {
		return out, nil, nil, fmt.Errorf("merging the review: %w", err)
	}
	if room == 0 {
		out.Summary, rf.notesFull = "", true
	}
	edits, problems := rf.planEdits(mem, out.Edits)
	var dropped []string
	for _, p := range problems {
		if strings.HasPrefix(p, "- edit ") {
			dropped = append(dropped, strings.TrimPrefix(p, "- "))
		}
	}
	return out, edits, dropped, nil
}

// mergeInput is the merge call's message: memory now, the summary and
// the items.
func mergeInput(mem []store.MemorySection, summary string, items []reviewItem) string {
	var b strings.Builder
	for _, scope := range []store.MemoryScope{store.ScopeUser, store.ScopeProject} {
		if scope == store.ScopeUser {
			b.WriteString("User memory now:\n")
		} else {
			b.WriteString("Project memory now:\n")
		}
		var ss []store.MemorySection
		for _, m := range mem {
			if m.Scope == scope && m.Content != "" {
				ss = append(ss, m)
			}
		}
		store.SortMemory(ss)
		if len(ss) == 0 {
			b.WriteString("(empty)\n")
		}
		for _, m := range ss {
			by := ""
			if m.Source == id.SourceUser {
				by = ", written by the user"
			}
			fmt.Fprintf(&b, "[%s%s]\n%s\n", m.Section, by, store.QuoteMemory(m.Content))
		}
		b.WriteString("\n")
	}
	b.WriteString("The chat's summary now:\n")
	if strings.TrimSpace(summary) == "" {
		b.WriteString("(none)\n")
	} else {
		b.WriteString(summary + "\n")
	}
	b.WriteString("\nItems:\n")
	for i, it := range items {
		fmt.Fprintf(&b, "i%d (%s, %s): %s\n", i+1, it.Kind, it.Scope, it.Text)
	}
	return b.String()
}

// planEdits checks the merge's edits in order against a copy of memory:
// the last edit of a section wins, and edits that change nothing are left
// out. Each problem is a line for the retry.
func (rf *refresh) planEdits(mem []store.MemorySection, edits []mergeEdit) ([]plannedEdit, []string) {
	work := map[store.MemoryScope][]store.MemorySection{}
	for _, m := range mem {
		work[m.Scope] = append(work[m.Scope], m)
	}
	type key struct {
		scope   store.MemoryScope
		section string
	}
	var problems []string
	var order []key
	last := map[key]mergeEdit{}
	for _, e := range edits {
		scope := store.MemoryScope(e.Scope)
		sec, err := store.NormalizeSection(e.Section)
		if err != nil {
			problems = append(problems, fmt.Sprintf("- edit of section %q: %s", e.Section, strings.TrimPrefix(err.Error(), "store: invalid write: ")))
			continue
		}
		k := key{scope, sec}
		if _, ok := last[k]; !ok {
			order = append(order, k)
		}
		e.Section = sec
		last[k] = e
	}
	var out []plannedEdit
	for _, k := range order {
		e := last[k]
		if k.scope == store.ScopeUser && rf.p.Registry == nil {
			continue
		}
		all := work[k.scope]
		rev, before, userWrote := 0, "", false
		if i := slices.IndexFunc(all, func(m store.MemorySection) bool { return m.Section == k.section }); i >= 0 {
			// A section the user deleted counts too: bringing it back asks.
			rev, before, userWrote = all[i].Revision, all[i].Content, all[i].Source == id.SourceUser
		}
		_, next, op, err := store.PlanMemoryEdit(k.scope, all, store.MemoryEdit{Section: k.section, Content: store.UnquoteMemory(e.Content), Expected: rev, Source: rf.source()})
		var ce *store.MemoryCapError
		switch {
		case errors.As(err, &ce):
			problems = append(problems, fmt.Sprintf("- edit of %s memory, section %s: %s memory would be %d tokens, over its cap of %d; make it shorter", k.scope, k.section, k.scope, ce.Tokens, ce.Cap))
			continue
		case err != nil:
			problems = append(problems, fmt.Sprintf("- edit of %s memory, section %s: %s", k.scope, k.section, strings.TrimPrefix(err.Error(), "store: invalid write: ")))
			continue
		case op == "":
			continue
		}
		if i := slices.IndexFunc(all, func(m store.MemorySection) bool { return m.Section == k.section }); i >= 0 {
			all[i] = next
		} else {
			all = append(all, next)
		}
		work[k.scope] = all
		out = append(out, plannedEdit{scope: k.scope, before: before, userWrote: userWrote, next: next, expected: rev})
	}
	return out, problems
}

// replan checks the edits against memory as it is now: an edit whose
// section changed since the merge read it is dropped, and named, and the
// rest are planned again, so the caps count what memory holds now.
func (rf *refresh) replan(ctx context.Context, edits []plannedEdit) ([]plannedEdit, []string, error) {
	mem, err := rf.memory(ctx)
	if err != nil {
		return nil, nil, err
	}
	work := map[store.MemoryScope][]store.MemorySection{}
	for _, m := range mem {
		work[m.Scope] = append(work[m.Scope], m)
	}
	var out []plannedEdit
	var dropped []string
	for _, e := range edits {
		all := work[e.scope]
		i := slices.IndexFunc(all, func(m store.MemorySection) bool { return m.Section == e.next.Section })
		rev := 0
		if i >= 0 {
			rev = all[i].Revision
		}
		name := fmt.Sprintf("%s memory, section %s", e.scope, e.next.Section)
		if rev != e.expected {
			dropped = append(dropped, name+" (changed meanwhile)")
			continue
		}
		_, next, op, err := store.PlanMemoryEdit(e.scope, all, store.MemoryEdit{Section: e.next.Section, Content: e.next.Content, Expected: rev, Source: rf.source()})
		var ce *store.MemoryCapError
		switch {
		case errors.As(err, &ce):
			dropped = append(dropped, name+" (memory would be over its cap)")
			continue
		case err != nil:
			dropped = append(dropped, name)
			continue
		case op == "":
			continue
		}
		if i >= 0 {
			all[i] = next
		} else {
			all = append(all, next)
		}
		work[e.scope] = all
		e.next = next
		out = append(out, e)
	}
	return out, dropped, nil
}

// jsonCall asks for a JSON answer that matches s and decodes it into v.
// check finds problems the schema can't, such as an edit over a cap. A
// wrong answer gets one retry that lists what was wrong; after it, only an
// answer that can't be read is an error, and check's problems are left to
// the caller.
func (rf *refresh) jsonCall(ctx context.Context, sys, user string, s jsonSchema, maxTokens int, v any, check func() []string) error {
	msgs := []chat.Message{{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: user}}}}}
	text, err := rf.call(ctx, sys, msgs, maxTokens)
	if err != nil {
		return err
	}
	var problems []string
	switch err := readJSON(text, s, v); {
	case err != nil:
		problems = problemLines(err)
	case check != nil:
		problems = check()
	}
	if len(problems) == 0 {
		return nil
	}
	if strings.TrimSpace(text) == "" {
		text = "(no answer)"
	}
	msgs = append(msgs,
		chat.Message{Role: chat.RoleAssistant, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: text}}}},
		chat.Message{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: fixRequest(problems)}}}})
	if text, err = rf.call(ctx, sys, msgs, maxTokens); err != nil {
		return err
	}
	zero(v)
	return readJSON(text, s, v)
}

func problemLines(err error) []string {
	var se *schemaError
	if errors.As(err, &se) {
		return se.lines
	}
	return []string{"- " + err.Error()}
}

// zero clears what v points to, before a second answer is decoded into it.
func zero(v any) { reflect.ValueOf(v).Elem().SetZero() }

// call sends one background request with thinking off, and returns the
// answer's text. A retryable provider error is tried again, up to
// refreshTries in all.
func (rf *refresh) call(ctx context.Context, sys string, msgs []chat.Message, maxTokens int) (string, error) {
	req := provider.Request{Model: rf.plan.model, MaxTokens: maxTokens, System: []provider.Block{{Text: sys}}, Messages: msgs}
	for try := 0; ; try++ {
		var text strings.Builder
		var err error
		for ev, e := range rf.o.d.Models.Stream(ctx, limit.Background, req) {
			if e != nil {
				err = e
				break
			}
			if ev.Kind == provider.EventPart && ev.Part.Text != nil {
				text.WriteString(ev.Part.Text.Text)
			}
		}
		if err == nil {
			return text.String(), nil
		}
		var pe *provider.Error
		if ctx.Err() != nil {
			return "", context.Cause(ctx)
		}
		if !errors.As(err, &pe) || !pe.Retryable() || try+1 >= refreshTries {
			return "", err
		}
		wait := pe.RetryAfter
		if wait <= 0 {
			wait = provider.Backoff(try)
		}
		t := time.NewTimer(wait)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return "", context.Cause(ctx)
		}
	}
}

// mustAsk reports whether the batch needs the user's yes: at Strict,
// when the reviewed turns read untrusted content (at every level), or
// when an edit changes a section the user wrote last (SPEC 3.5).
func (rf *refresh) mustAsk(ctx context.Context, edits []plannedEdit) (bool, error) {
	level, err := rf.p.Level(ctx)
	if err != nil {
		return false, err
	}
	if level == project.Strict || rf.plan.untrusted {
		return true, nil
	}
	return slices.ContainsFunc(edits, func(e plannedEdit) bool { return e.userWrote }), nil
}

func diffs(edits []plannedEdit) []chat.MemoryDiff {
	out := make([]chat.MemoryDiff, 0, len(edits))
	for _, e := range edits {
		out = append(out, chat.MemoryDiff{Target: e.next.Target(), Before: e.before, After: e.next.Content})
	}
	return out
}

// confirm shows the batch's card, Save or Deny, and waits for the answer.
// A message sent meanwhile answers Deny. Stop and the project's close end
// the wait, and the card shows as stopped.
func (rf *refresh) confirm(ctx context.Context, edits []plannedEdit) (bool, error) {
	o, cs := rf.o, rf.cs
	ask := fmt.Sprintf("Save Refresh memory's edits to %d sections?", len(edits))
	if len(edits) == 1 {
		ask = fmt.Sprintf("Save Refresh memory's edit to %s memory, section %s?", edits[0].scope, edits[0].next.Section)
	}
	risk := "Project memory is in the context of every chat in this project."
	if slices.ContainsFunc(edits, func(e plannedEdit) bool { return e.scope == store.ScopeUser }) {
		risk = "User memory is in the context of every chat, in all your projects; project memory in every chat of this project."
	}
	check := " Check that each edit is what you want kept."
	if rf.plan.untrusted {
		check = " This chat read web pages, files or MCP results, so check the changes before they're saved."
	}
	a := chat.Approval{
		ID:      id.Approval(id.New()),
		Kind:    "memory",
		Target:  string(rf.source()),
		Ask:     ask,
		Risk:    risk + check,
		Edits:   diffs(edits),
		Options: []chat.ApprovalOption{{Label: "Save", Grant: chat.GrantOnce}, {Label: "Deny", Grant: chat.GrantDeny}},
	}
	part := chat.Part{Kind: chat.PartApproval, Approval: &a}
	cs.mu.Lock()
	m, err := o.write(ctx, cs, rf.p, chat.Message{Chat: rf.plan.ch.ID, Turn: rf.plan.turn, Role: chat.RoleUser, Parts: []chat.Part{part}})
	if err != nil {
		cs.mu.Unlock()
		return false, err
	}
	pd := &pending{w: chat.Waiting{Message: m.ID, Index: 0, Kind: chat.PartApproval, Text: ask}, part: part, reply: make(chan Answer, 1)}
	cs.waiting = pd
	o.publishStatus(cs)
	cs.mu.Unlock()
	var ans Answer
	select {
	case ans = <-pd.reply:
	case <-ctx.Done():
		cs.mu.Lock()
		if cs.waiting == pd {
			cs.waiting = nil
			o.publishStatus(cs)
		}
		cs.mu.Unlock()
		rf.setPart(ctx, m.ID, stoppedCard(part))
		return false, context.Cause(ctx)
	}
	p := answered(part, ans)
	wctx, cancel := writing(ctx)
	defer cancel()
	c := p.Approval
	if err := recordApproval(wctx, rf.p.DB, store.Approval{ID: c.ID, Kind: c.Kind, Target: c.Target, Answer: c.Answer,
		Note: c.Note, Source: c.By, CreatedAt: *c.AnsweredAt}); err != nil {
		return false, &endError{err: err}
	}
	rf.setPart(ctx, m.ID, p)
	return ans.Grant != chat.GrantDeny, nil
}

// recordApproval records the card's answer; tests make it fail.
var recordApproval = func(ctx context.Context, db *store.ProjectDB, a store.Approval) error {
	return db.RecordApproval(ctx, a)
}

// setPart writes the card's part again and publishes it.
func (rf *refresh) setPart(ctx context.Context, m id.Message, p chat.Part) {
	wctx, cancel := writing(ctx)
	defer cancel()
	cs := rf.cs
	cs.pub.Lock()
	defer cs.pub.Unlock()
	seq, err := rf.p.Chats.SetPart(wctx, m, 0, p)
	if err != nil {
		rf.o.d.Log.Error("agent: Refresh memory's card", "chat", cs.key.c, "err", err)
		return
	}
	cs.bumpSeq(seq)
	rf.o.pub.Part(chat.PartDone{Project: rf.p.ID, Chat: rf.plan.ch.ID, Seq: seq, Message: m, Turn: rf.plan.turn, Index: 0, Part: p})
}

// apply writes the edits as normal memory edits with the refresh's
// source. An edit whose section changed since the merge is skipped, and
// named.
func (rf *refresh) apply(ctx context.Context, edits []plannedEdit) ([]chat.MemoryDiff, []string) {
	var applied []chat.MemoryDiff
	var skipped []string
	for _, e := range edits {
		me := store.MemoryEdit{Section: e.next.Section, Content: e.next.Content, Expected: e.expected, Source: rf.source()}
		var w store.MemoryWrite
		var err error
		if e.scope == store.ScopeUser {
			if w, err = rf.p.Registry.UpdateUserMemory(ctx, me, rf.p.ID); err == nil {
				_ = rf.p.DB.LogUserMemory(ctx, w) // history only, as for update_memory
			}
		} else {
			w, err = rf.p.DB.UpdateMemory(ctx, me)
		}
		var ce *store.MemoryConflictError
		switch {
		case errors.As(err, &ce):
			skipped = append(skipped, fmt.Sprintf("%s memory, section %s (changed meanwhile)", e.scope, e.next.Section))
			continue
		case err != nil:
			rf.o.d.Log.Warn("agent: Refresh memory's edit", "chat", rf.cs.key.c, "target", e.next.Target(), "err", err)
			skipped = append(skipped, fmt.Sprintf("%s memory, section %s", e.scope, e.next.Section))
			continue
		case w.Op == "":
			continue
		}
		applied = append(applied, chat.MemoryDiff{Target: w.Section.Target(), Before: e.before, After: w.Section.Content, Change: w.Change})
	}
	return applied, skipped
}

// finish writes the summary, the progress and the chip, and makes the
// next turn cut.
func (rf *refresh) finish(ctx context.Context, newSummary string, notes chat.SessionNote, applied []chat.MemoryDiff, dropped []string, denied bool) error {
	var summary *string
	if s := strings.TrimSpace(newSummary); s != "" && s != strings.TrimSpace(notes.Summary) {
		summary = &s
	}
	chip := func(serr error) *chat.Message {
		n := chat.Notice{Kind: chat.NoticeMemoryRefreshed, Text: chipText(applied, dropped, denied, rf.notesFull, serr), Edits: applied}
		return &chat.Message{Chat: rf.plan.ch.ID, Turn: rf.plan.turn, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartNotice, Notice: &n}}}
	}
	cs := rf.cs
	cs.pub.Lock()
	f, err := finishReview(ctx, rf.p.Chats, store.FinishReview{Chat: rf.plan.ch.ID, Summary: summary, NotesRevision: notes.Revision, UpTo: rf.plan.upTo, Chip: chip})
	if err == nil {
		cs.bumpSeq(f.Seq)
		if m := f.Message; m != nil {
			for i, p := range m.Parts {
				rf.o.pub.Part(chat.PartDone{Project: rf.p.ID, Chat: m.Chat, Seq: f.Seq, Message: m.ID, Turn: m.Turn, Index: i, Part: p})
			}
		}
	}
	cs.pub.Unlock()
	if err != nil {
		return err
	}
	cs.mu.Lock()
	cs.refresh.cutNext = true
	cs.mu.Unlock()
	return nil
}

// finishReview writes a refresh's end; tests make it fail.
var finishReview = func(ctx context.Context, c *store.ChatsDB, f store.FinishReview) (store.Finished, error) {
	return c.FinishReview(ctx, f)
}

// chipText is the chip's line: "Memory refreshed: 3 added, 1 changed",
// counting the sections the batch wrote, with what wasn't saved.
func chipText(applied []chat.MemoryDiff, dropped []string, denied, notesFull bool, serr error) string {
	var s string
	switch {
	case denied:
		s = "Memory refresh: the edits were not saved"
	case len(applied) == 0:
		s = "Memory refreshed: nothing new"
	default:
		s = "Memory refreshed: " + counts(applied)
	}
	if len(dropped) > 0 {
		s += "; not saved: " + strings.Join(dropped, "; ")
	}
	var ce *store.NotesCapError
	switch {
	case notesFull:
		s += "; no summary was written: the session notes are full"
	case errors.As(serr, &ce):
		s += "; the summary was not saved: it was too long"
	case serr != nil:
		s += "; the summary was not saved: the notes changed meanwhile"
	}
	return s
}

// counts is "3 added, 1 changed, 1 deleted" for the sections of a batch.
func counts(applied []chat.MemoryDiff) string {
	added, changed, deleted := 0, 0, 0
	for _, d := range applied {
		switch {
		case d.Before == "":
			added++
		case d.After == "":
			deleted++
		default:
			changed++
		}
	}
	var parts []string
	for _, c := range []struct {
		n    int
		verb string
	}{{added, "added"}, {changed, "changed"}, {deleted, "deleted"}} {
		if c.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c.n, c.verb))
		}
	}
	return strings.Join(parts, ", ")
}

// failed tells the chat why the refresh ended, also after Stop when the
// card was answered (endError). Before the batch is written, nothing
// changed. After it, when the end couldn't be saved, the
// edits are in: the notice names them and keeps the batch, so its undo
// works, and the next turn cuts. A crash between the batch and the end
// leaves the edits with no chip; the next refresh reviews the turns again.
func (rf *refresh) failed(ctx context.Context, err error) {
	wctx, cancel := writing(ctx)
	defer cancel()
	n := chat.Notice{Kind: chat.NoticeMemoryRefreshed, Text: "Memory refresh failed, nothing was changed: " + err.Error()}
	var ee *endError
	if errors.As(err, &ee) && len(ee.applied) > 0 {
		var names []string
		for _, d := range ee.applied {
			names = append(names, memoryName(d.Target))
		}
		n.Text = fmt.Sprintf("Memory refreshed: %s (%s), but the refresh couldn't finish, so the next one reviews these turns again: %v",
			counts(ee.applied), strings.Join(names, "; "), ee.err)
		n.Edits = ee.applied
	}
	rf.cs.mu.Lock()
	defer rf.cs.mu.Unlock()
	if n.Edits != nil {
		rf.cs.refresh.cutNext = true
	}
	if _, werr := rf.o.write(wctx, rf.cs, rf.p, chat.Message{Chat: rf.plan.ch.ID, Turn: rf.plan.turn, Role: chat.RoleUser,
		Parts: []chat.Part{{Kind: chat.PartNotice, Notice: &n}}}); werr != nil {
		rf.o.d.Log.Error("agent: Refresh memory's notice", "chat", rf.cs.key.c, "err", werr)
	}
}

// memoryName is "user memory, section preferences" for a section's target.
func memoryName(target string) string {
	scope, sec, ok := strings.Cut(strings.TrimPrefix(target, "memory:"), "/")
	if !ok {
		return target
	}
	return fmt.Sprintf("%s memory, section %s", scope, sec)
}
