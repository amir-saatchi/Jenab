package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// Subagents (SPEC 8.6): a run of its own inside a tool call of a chat's
// turn, such as the schema agent (8.2).
//
//   - Its messages stay in memory; nothing is written to chats.db, so
//     search_history doesn't find them.
//   - One level: its Env has no Subagent, and its tools are its own.
//   - Its writes use the parent call's source, so Undo turn on the parent
//     turn reverts them.
//   - When it ends, its transcript is stored in the bucket as JSON, and the
//     parent's result points to it. The transcript is the chat's record,
//     not a change of the turn: it is written with the chat as its source,
//     so Undo turn leaves it, and it goes with the chat (Delete, Clear).
//
// The loop is the chat loop's core without the chat: a request, its tool
// calls in order, the limits and what the model is told. It stays apart
// from the chat code so Phase 5's subagent tool and a later loop core
// (HARNESS 5) can share it.

const (
	// subMaxRequests is a subagent's request cap; the last request asks
	// for an answer without tools, like a turn's (8.3).
	subMaxRequests = 15
	// subTries and subRetrying: a subagent tries a failing request up to
	// 3 times within 2 minutes, then fails, and the parent gets the error
	// with its kind and wait (8.3).
	subTries    = 3
	subRetrying = 2 * time.Minute
)

// subRun is one subagent run.
type subRun struct {
	r      *runner
	t      *turn
	parent *tool.Env
	s      tool.Subagent
	system []provider.Block
	tools  map[string]tool.Tool
	defs   []provider.ToolDef
	msgs   []chat.Message
	bad    int // requests in a row whose calls all failed to run
	usage  chat.Usage
}

// transcript is a subagent's run as the bucket keeps it (8.6).
type transcript struct {
	Name     string         `json:"name"`
	Model    string         `json:"model"`
	Parent   id.Source      `json:"parent"` // the call that started it, as its writes name it
	Started  time.Time      `json:"started"`
	Ended    time.Time      `json:"ended"`
	Requests int            `json:"requests"`
	Usage    chat.Usage     `json:"usage"`
	Messages []chat.Message `json:"messages"`
	Error    string         `json:"error,omitempty"`
}

// subagent runs s on the chat's model, inside the call whose Env is
// parent. A stopped run stores nothing.
func (r *runner) subagent(ctx context.Context, t *turn, parent *tool.Env, s tool.Subagent) (tool.SubagentResult, error) {
	sr := &subRun{r: r, t: t, parent: parent, s: s, tools: map[string]tool.Tool{}}
	if err := sr.build(); err != nil {
		r.o.d.Log.Error("agent: starting a subagent", "subagent", s.Name, "chat", t.ch.ID, "err", err)
		return tool.SubagentResult{}, err
	}
	started := time.Now()
	res, err := sr.loop(ctx)
	if ctx.Err() != nil {
		return res, context.Cause(ctx)
	}
	ref, serr := sr.store(ctx, started, res.Requests, err)
	if serr != nil {
		r.o.d.Log.Error("agent: storing a subagent's transcript", "chat", t.ch.ID, "err", serr)
	}
	res.Ref = ref
	var te *tool.Error
	if err != nil && !errors.As(err, &te) {
		r.o.d.Log.Error("agent: a subagent failed", "subagent", s.Name, "chat", t.ch.ID, "err", err)
	}
	return res, err
}

// build makes the system blocks, with the skills after the instructions,
// and the tool list.
func (sr *subRun) build() error {
	text := strings.TrimSpace(sr.s.System)
	set := sr.r.o.skills()
	for _, name := range sr.s.Skills {
		sk, ok := set.Get(chat.KindChat, name)
		if !ok {
			return fmt.Errorf("agent: the %s needs the skill %s, which isn't there", sr.s.Name, name)
		}
		text += "\n\n" + sk.Text()
	}
	sr.system = []provider.Block{{Text: text, Cache: true, Name: sr.s.Name}}
	for _, tl := range sr.s.Tools {
		spec := tl.Spec()
		if spec.Effects&tool.Memory != 0 {
			continue // subagents can't change memory (SPEC 8.1)
		}
		sr.tools[spec.Name] = tl
		sr.defs = append(sr.defs, provider.ToolDef{Name: spec.Name, Description: spec.Description, Schema: spec.Schema})
	}
	sr.msgs = []chat.Message{{ID: id.Message(id.New()), Chat: sr.t.ch.ID, Turn: sr.t.n, Role: chat.RoleUser,
		Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: sr.s.Task}}}}}
	return nil
}

// loop sends requests until the model answers without tool calls, Done
// says the work is done, or the request cap is reached.
func (sr *subRun) loop(ctx context.Context) (tool.SubagentResult, error) {
	var res tool.SubagentResult
	for {
		last := res.Requests+1 >= subMaxRequests
		req := provider.Request{Model: sr.t.model, System: sr.system, Messages: sr.msgs, Tools: sr.defs, Thinking: true, Image: sr.r.image}
		if last {
			req.ToolChoice = provider.ToolsNone
			req.Messages = append(req.Messages[:len(req.Messages):len(req.Messages)], chat.Message{Chat: sr.t.ch.ID, Turn: sr.t.n, Role: chat.RoleUser,
				Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: lastRequest}}}})
		}
		res.Requests++
		m, stop, err := sr.ask(ctx, req)
		if err != nil {
			return res, err
		}
		res.Cut = stop == provider.StopMaxTokens
		if res.Cut {
			m.Parts = dropCut(m.Parts)
		}
		if len(m.Parts) > 0 {
			sr.msgs = append(sr.msgs, m)
		}
		var calls []chat.ToolCall
		var text []string
		for _, p := range m.Parts {
			switch {
			case p.ToolCall != nil:
				calls = append(calls, *p.ToolCall)
			case p.Text != nil && strings.TrimSpace(p.Text.Text) != "":
				text = append(text, strings.TrimSpace(p.Text.Text))
			}
		}
		res.Text = strings.Join(text, "\n\n")
		if len(calls) == 0 {
			res.Capped = last && res.Text == ""
			return res, nil
		}
		if err := sr.runTools(ctx, m.ID, calls, last); err != nil {
			return res, err
		}
		if ctx.Err() != nil {
			return res, context.Cause(ctx)
		}
		if sr.s.Done != nil && sr.s.Done() {
			res.Text = ""
			return res, nil
		}
		if last {
			res.Capped = res.Text == ""
			return res, nil
		}
	}
}

// dropCut drops tool calls cut off by the output limit (3.8).
func dropCut(ps []chat.Part) []chat.Part {
	out := ps[:0:0]
	for _, p := range ps {
		if p.ToolCall == nil || p.ToolCall.Invalid == "" {
			out = append(out, p)
		}
	}
	return out
}

// ask streams one request into a message, retrying rate limits,
// overloads and transport errors up to subTries times within subRetrying.
func (sr *subRun) ask(ctx context.Context, req provider.Request) (chat.Message, provider.StopReason, error) {
	var since time.Time
	for attempt := 0; ; attempt++ {
		sr.r.moving(sr.r.o.d.Models.FirstEvent(sr.t.provider))
		m, stop, err := sr.stream(ctx, req)
		if err == nil {
			return m, stop, nil
		}
		if ctx.Err() != nil {
			return m, stop, context.Cause(ctx)
		}
		var pe *provider.Error
		if !errors.As(err, &pe) {
			return m, stop, err
		}
		if since.IsZero() {
			since = time.Now()
		}
		wait := pe.RetryAfter
		if wait <= 0 {
			wait = provider.Backoff(attempt)
		}
		if !pe.Retryable() || attempt+1 >= subTries || time.Since(since)+wait > subRetrying {
			return m, stop, sr.failed(pe)
		}
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return m, stop, context.Cause(ctx)
		}
	}
}

// failed is a provider error the parent can read and decide on: wait, try
// again, or do the work itself (8.3).
func (sr *subRun) failed(pe *provider.Error) error {
	msg := fmt.Sprintf("the %s stopped on an error from %s (%s)", sr.s.Name, pe.Provider, kindText(pe.Kind))
	if pe.RetryAfter > 0 {
		msg += fmt.Sprintf(", retry after %d s", int(pe.RetryAfter.Round(time.Second)/time.Second))
	}
	if pe.Message != "" {
		msg += ": " + short(pe.Message)
	}
	return &tool.Error{Msg: msg, Err: pe}
}

// stream sends one try and collects its parts. Nothing is published: the
// parent's call shows as running meanwhile.
func (sr *subRun) stream(ctx context.Context, req provider.Request) (chat.Message, provider.StopReason, error) {
	m := chat.Message{ID: id.Message(id.New()), Chat: sr.t.ch.ID, Turn: sr.t.n, Role: chat.RoleAssistant, Model: sr.t.model}
	var stop provider.StopReason
	for ev, err := range sr.r.o.d.Models.Stream(ctx, limit.Interactive, req) {
		if err != nil {
			return m, stop, err
		}
		sr.r.cs.moved.Store(time.Now().UnixNano())
		switch ev.Kind {
		case provider.EventPart:
			m.Parts = append(m.Parts, *ev.Part)
		case provider.EventDone:
			if ev.Usage != nil {
				m.Usage = *ev.Usage
				sr.usage.Input += ev.Usage.Input
				sr.usage.Output += ev.Usage.Output
				sr.usage.CacheRead += ev.Usage.CacheRead
				sr.usage.CacheWrite += ev.Usage.CacheWrite
			}
			stop = ev.Stop
		}
	}
	return m, stop, nil
}

// runTools runs an answer's calls in order and adds their results as one
// tool message. Calls of the last request, which had no tools, are closed
// without running.
func (sr *subRun) runTools(ctx context.Context, msg id.Message, calls []chat.ToolCall, last bool) error {
	tm := chat.Message{ID: id.Message(id.New()), Chat: sr.t.ch.ID, Turn: sr.t.n, Role: chat.RoleTool}
	sr.r.moving(0) // its tools run inside the parent's call, which has no time limit
	allBad := !last
	for i, c := range calls {
		out, bad := chat.ToolResult{CallID: c.ID, Text: "not run: the run reached its request limit", IsError: true}, false
		if !last {
			out, bad = sr.runTool(ctx, msg, i, c)
		}
		allBad = allBad && bad
		tm.Parts = append(tm.Parts, chat.Part{Kind: chat.PartToolResult, ToolResult: &out})
	}
	sr.msgs = append(sr.msgs, tm)
	if !allBad {
		sr.bad = 0
		return nil
	}
	if sr.bad++; sr.bad >= maxBadRequests {
		return &tool.Error{Msg: fmt.Sprintf("the %s stopped: its model sent tool calls that couldn't run, %d times in a row", sr.s.Name, maxBadRequests)}
	}
	return nil
}

// runTool runs one call, with the parent's approvals (8.8). Every outcome
// is a result the model can read; a system failure is logged too.
func (sr *subRun) runTool(ctx context.Context, msg id.Message, n int, c chat.ToolCall) (chat.ToolResult, bool) {
	r := sr.r
	result := func(text string) chat.ToolResult { return chat.ToolResult{CallID: c.ID, Text: text, IsError: true} }
	failed := func(err error) (chat.ToolResult, bool) {
		var te *tool.Error
		switch {
		case ctx.Err() != nil:
			return result(cancelled(ctx)), false
		case errors.As(err, &te):
			return result(te.Msg), errors.Is(err, tool.ErrArgs)
		}
		r.o.d.Log.Error("agent: a subagent's tool failed", "subagent", sr.s.Name, "tool", c.Name, "chat", sr.t.ch.ID, "err", err)
		return result(fmt.Sprintf("%s failed: %v", c.Name, err)), false
	}
	if ctx.Err() != nil {
		return result(cancelled(ctx)), false
	}
	if c.Invalid != "" {
		return result(invalidArgs(c.Invalid)), true
	}
	tl, ok := sr.tools[c.Name]
	if !ok {
		return result("there is no tool named " + c.Name), true
	}
	p := sr.parent
	env := &tool.Env{Project: p.Project, Chat: p.Chat, Message: msg, Source: p.Source, Priority: p.Priority,
		PreviewTokens: p.PreviewTokens, PrivateHosts: p.PrivateHosts, ChatStatus: p.ChatStatus, Approve: p.Approve, Progress: p.Progress}
	call := tool.Call{ID: c.ID, Args: c.Args, Env: env}
	needs := tool.Needs{Effects: tl.Spec().Effects}
	if pf, ok := tl.(tool.Preflighter); ok {
		var err error
		if needs, err = pf.Preflight(ctx, call); err != nil {
			return failed(err)
		}
	}
	for _, a := range needs.Approvals {
		got, err := p.Approve(ctx, a)
		if err != nil {
			return failed(err)
		}
		if got.Answer == chat.GrantDeny {
			return result(denied(&got)), false
		}
	}
	res, err := r.safeRun(ctx, tl, call)
	if needs.Effects&tool.Untrusted != 0 {
		r.readOutside(sr.t.n) // outside data reached the turn through it (8.8)
	}
	if err != nil && ctx.Err() != nil {
		return result(cancelled(ctx)), false
	}
	bad := errors.Is(err, tool.ErrArgs)
	out, err := tool.Output(context.WithoutCancel(ctx), call, c.Name, n, res, err)
	if err != nil {
		return failed(err)
	}
	return out, bad
}

// transcriptDir is where a chat's subagent transcripts are.
func transcriptDir(c id.Chat) string { return "subagents/" + string(c) + "/" }

// dropTranscripts deletes the chat's subagent transcripts with its
// messages. A failure is logged: the messages are gone either way.
func (o *Orchestrator) dropTranscripts(ctx context.Context, db *store.ProjectDB, c id.Chat) {
	for {
		l, err := db.ListObjects(ctx, transcriptDir(c), "", 1000)
		if err == nil && len(l.Objects) == 0 {
			return
		}
		for _, ob := range l.Objects {
			if err != nil {
				break
			}
			err = db.DeleteObject(ctx, limit.Interactive, id.SourceUser, ob.Key)
		}
		if err != nil {
			o.d.Log.Error("agent: deleting a chat's subagent transcripts", "chat", c, "err", err)
			return
		}
	}
}

// store writes the transcript to subagents/<chat>/<ID>.json and returns
// its key. It writes after Stop too, as chat writes do, but is bounded.
func (sr *subRun) store(ctx context.Context, started time.Time, requests int, runErr error) (string, error) {
	tr := transcript{Name: sr.s.Name, Model: sr.t.model, Parent: sr.parent.Source, Started: started.UTC(), Ended: time.Now().UTC(),
		Requests: requests, Usage: sr.usage, Messages: sr.msgs}
	if runErr != nil {
		tr.Error = runErr.Error()
	}
	b, err := json.MarshalIndent(tr, "", "  ")
	if err != nil {
		return "", err
	}
	wctx, cancel := writing(ctx)
	defer cancel()
	key := transcriptDir(sr.t.ch.ID) + id.New() + ".json"
	src := id.SourceOf("chat", string(sr.t.ch.ID))
	if _, err := sr.parent.Project.DB.PutObject(wctx, sr.parent.Priority, src, key, bytes.NewReader(b), store.PutOptions{Source: "agent"}); err != nil {
		return "", err
	}
	return key, nil
}
