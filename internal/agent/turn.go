package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/tool"
)

const (
	// maxRetrying is how long a turn retries a failing request before it
	// stops with an error card (8.3).
	maxRetrying = 10 * time.Minute
	// writeTimeout bounds a chat write. Writes after Stop still happen,
	// so the history stays valid (8.3).
	writeTimeout = 30 * time.Second
	// closeWait is how long a project's close waits for a turn to write
	// its last messages.
	closeWait = 5 * time.Second
)

// lastRequest goes at the end of a turn's last request, which has no
// tools (8.3). It is not stored.
const lastRequest = "[this is the last request of this turn: answer with what you have, without tools]"

// runner runs a chat's turn. It holds a lease on the project.
type runner struct {
	o  *Orchestrator
	cs *chatState
	p  *project.Project
}

// turn is one turn's state.
type turn struct {
	n        int
	ch       chat.Chat
	set      config.Settings
	model    string // "provider/id"
	provider string
	window   int // the model's context window; 0 if unknown
	requests int
	seen     id.Message // the newest user message in the last request
	bad      int        // requests in a row whose tool calls all failed to run
	trace    *TurnTrace // nil with the developer tools off
}

// turn runs steps until the model answers without tool calls, the request
// cap is reached, or the turn is stopped or fails. A message sent during
// the last answer gets another step. It reports whether the turn ended
// normally.
func (r *runner) turn(ctx context.Context, n int) (*turn, bool) {
	cs := r.cs
	t := &turn{n: n, set: r.o.d.Settings()}
	t.trace = r.o.d.Traces.begin(r.cs.key.p, r.cs.key.c, n)
	defer r.o.d.Traces.end(t.trace)
	if err := r.begin(ctx, t); err != nil {
		r.finish(ctx, t, err)
		return t, false
	}
	for {
		last := t.requests+1 >= t.set.LLM.TurnMaxRequests
		more, err := r.step(ctx, t, last)
		if err != nil {
			r.finish(ctx, t, err)
			return t, false
		}
		if more && !last {
			continue
		}
		if r.newMessages(t) {
			continue // a message came during the answer; after the cap, one more request without tools answers it
		}
		cs.mu.Lock()
		r.idle(false)
		cs.mu.Unlock()
		return t, true
	}
}

// begin reads the chat, resolves its model and cuts the window if due.
func (r *runner) begin(ctx context.Context, t *turn) error {
	ch, err := r.p.Chats.Chat(ctx, r.cs.key.c)
	if err != nil {
		return err
	}
	t.ch = ch
	prov, mid, err := r.o.d.Models.Resolve(ch.Model)
	if err != nil {
		return err
	}
	t.provider, t.model, t.window = prov, prov+"/"+mid, r.o.d.Models.ContextWindow(prov, mid)
	r.o.d.Traces.chat(t.trace, t.model, ch.Title)
	r.cs.mu.Lock()
	r.cs.title = ch.Title
	r.cs.mu.Unlock()
	return r.cutIfDue(ctx, t)
}

// newMessages reports whether the user sent a message to the turn after
// its last request was built.
func (r *runner) newMessages(t *turn) bool {
	r.cs.mu.Lock()
	defer r.cs.mu.Unlock()
	return r.cs.lastUser > t.seen && !r.cs.stopping
}

// step sends one request and runs the tool calls of its answer in order.
// It reports whether the model wants another step.
func (r *runner) step(ctx context.Context, t *turn, last bool) (bool, error) {
	req, err := r.request(ctx, t, last)
	if err != nil {
		return false, err
	}
	t.requests++
	r.cs.mu.Lock()
	r.cs.requests = t.requests
	r.cs.mu.Unlock()

	resp, err := r.ask(ctx, t, req, last)
	if err != nil {
		if resp != nil && ctx.Err() != nil {
			// Stopped while streaming: keep the text so far (8.3).
			if parts := resp.stopped(); len(parts) > 0 {
				r.writeMessage(ctx, chat.Message{ID: resp.id, Chat: t.ch.ID, Turn: t.n, Role: chat.RoleAssistant, Model: t.model, Usage: resp.usage, Parts: parts})
			}
		}
		return false, err
	}
	if resp.stop == provider.StopMaxTokens {
		resp.dropCutCalls()
	}
	if len(resp.parts) > 0 {
		m := chat.Message{ID: resp.id, Chat: t.ch.ID, Turn: t.n, Role: chat.RoleAssistant, Model: t.model, Usage: resp.usage, Parts: resp.parts}
		if err := r.writeMessage(ctx, m); err != nil {
			return false, err
		}
	}
	var calls []chat.ToolCall
	for _, p := range resp.parts {
		if p.ToolCall != nil {
			calls = append(calls, *p.ToolCall)
		}
	}
	if len(calls) == 0 {
		if s := cut(resp); s != "" {
			n := chat.Notice{Kind: chat.NoticeAnswerCut, Text: s}
			if err := r.writeMessage(ctx, chat.Message{Chat: t.ch.ID, Turn: t.n, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartNotice, Notice: &n}}}); err != nil {
				return false, err
			}
		}
		return false, nil
	}
	if err := r.runTools(ctx, t, resp, calls, last); err != nil {
		return false, err
	}
	if ctx.Err() != nil {
		return false, context.Cause(ctx)
	}
	return true, nil
}

// response is one model answer, kept in memory until it is complete.
type response struct {
	id     id.Message // the assistant message
	toolID id.Message // the message for its tool results
	usage  chat.Usage
	stop   provider.StopReason

	mu    sync.Mutex // parts, kind and text, which Live reads
	parts []chat.Part
	kind  chat.PartKind   // the part being streamed
	text  strings.Builder // its text so far
	u16   int             // its length in UTF-16 units
}

// try starts a try: its IDs, and Status.Streaming. The IDs are made under
// cs.mu, like those of messages from Send: messages sort by ID within a
// turn, so a message the user sends during the try comes after the answer
// and its tool results, where the protocols allow it (8.3).
func (r *runner) try() *response {
	cs := r.cs
	cs.mu.Lock()
	defer cs.mu.Unlock()
	resp := &response{id: id.Message(id.New())}
	resp.toolID = id.Message(id.New())
	cs.resp = resp
	r.o.publishStatus(cs)
	return resp
}

// tried ends a try. The next status has no Streaming, so after a failed
// try the frontend drops its text: the retry wait or the turn's end sends
// one.
func (r *runner) tried() {
	r.cs.mu.Lock()
	r.cs.resp = nil
	r.cs.mu.Unlock()
}

// delta adds streamed text and returns where it starts, in UTF-16 units.
func (resp *response) delta(k chat.PartKind, s string) int {
	resp.mu.Lock()
	defer resp.mu.Unlock()
	if k != resp.kind {
		resp.kind = k
		resp.text.Reset()
		resp.u16 = 0
	}
	at := resp.u16
	resp.text.WriteString(s)
	for _, r := range s {
		resp.u16 += utf16.RuneLen(r)
	}
	return at
}

func (resp *response) part(p chat.Part) {
	resp.mu.Lock()
	defer resp.mu.Unlock()
	resp.parts = append(resp.parts, p)
	resp.kind = ""
	resp.text.Reset()
	resp.u16 = 0
}

// dropCutCalls drops tool calls whose arguments are not valid JSON from an
// answer that reached the output limit: they were cut off, not mistaken.
// Without other calls, the turn shows its cut notice (SPEC 3.8).
func (resp *response) dropCutCalls() {
	resp.mu.Lock()
	defer resp.mu.Unlock()
	resp.parts = slices.DeleteFunc(resp.parts, func(p chat.Part) bool {
		return p.ToolCall != nil && p.ToolCall.Invalid != ""
	})
}

// stopped is what is kept after Stop: the finished text parts and the
// text streamed so far, the last one marked as stopped. Blank text,
// thinking and tool calls of an unfinished answer are dropped.
func (resp *response) stopped() []chat.Part {
	var out []chat.Part
	for _, p := range resp.parts {
		if p.Text != nil && strings.TrimSpace(p.Text.Text) != "" {
			out = append(out, p)
		}
	}
	if s := resp.text.String(); resp.kind == chat.PartText && strings.TrimSpace(s) != "" {
		out = append(out, chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: s, Stopped: true}})
	} else if n := len(out); n > 0 {
		t := *out[n-1].Text
		t.Stopped = true
		out[n-1].Text = &t
	}
	return out
}

// cut is the notice for an answer without tool calls that ended early or
// had no text, or "" (8.3).
func cut(resp *response) string {
	switch resp.stop {
	case provider.StopMaxTokens:
		return "[the answer was cut off: it reached the model's output limit]"
	case provider.StopRefused:
		return "[the answer stopped: the model or the provider's content filter refused it]"
	}
	for _, p := range resp.parts {
		if p.Text != nil && strings.TrimSpace(p.Text.Text) != "" {
			return ""
		}
	}
	return "[the model sent an empty answer]"
}

// ask streams the request, retrying rate limits, overloads and transport
// errors with the wait shown in the chat, for up to maxRetrying (8.3). A
// message sent during the wait goes into the next try. A stopped turn
// returns the answer so far with the turn's cause.
func (r *runner) ask(ctx context.Context, t *turn, req provider.Request, last bool) (*response, error) {
	var since time.Time
	for attempt := 0; ; attempt++ {
		resp := r.try()
		r.moving(r.o.d.Models.FirstEvent(t.provider)) // each try, so one after a wait isn't flagged
		i := r.o.d.Traces.request(t.trace, req, resp.id, last)
		err := r.stream(ctx, t, req, resp)
		r.o.d.Traces.answered(t.trace, i, resp, err)
		if err == nil {
			r.tried()
			return resp, nil
		}
		if ctx.Err() != nil {
			r.setRetry(nil, nil)
			return resp, context.Cause(ctx)
		}
		r.tried()
		var pe *provider.Error
		if !errors.As(err, &pe) || !pe.Retryable() {
			r.setRetry(nil, nil)
			return nil, err
		}
		if since.IsZero() {
			since = time.Now()
		}
		left := maxRetrying - time.Since(since)
		if left <= 0 || pe.RetryAfter > left {
			r.setRetry(nil, nil)
			return nil, &gaveUp{pe, time.Since(since)}
		}
		wait := pe.RetryAfter
		if wait <= 0 {
			wait = min(provider.Backoff(attempt), left)
		}
		wake := make(chan struct{})
		r.setRetry(&chat.Retry{Provider: pe.Provider, Kind: string(pe.Kind), At: retryAt(wait)}, func() { close(wake) })
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-wake:
			timer.Stop()
			r.o.d.Models.Resume(pe.Provider)
		case <-ctx.Done():
			timer.Stop()
			r.setRetry(nil, nil)
			return nil, context.Cause(ctx)
		}
		r.setRetry(nil, nil)
		if r.newMessages(t) {
			if req, err = r.request(ctx, t, last); err != nil {
				return nil, err
			}
		}
	}
}

// retryAt is when a wait of d ends, as Retry.At.
func retryAt(d time.Duration) time.Time {
	return time.Now().Add(d).UTC().Truncate(time.Millisecond)
}

// gaveUp is a request that failed for maxRetrying, or whose provider asks
// to wait past it.
type gaveUp struct {
	last  *provider.Error
	after time.Duration // since the first failure
}

func (g *gaveUp) Error() string { return "retried for " + minutes(g.after) + ": " + g.last.Error() }
func (g *gaveUp) Unwrap() error { return g.last }

// minutes is d in whole minutes, at least 1.
func minutes(d time.Duration) string {
	if n := max(1, int(d.Round(time.Minute)/time.Minute)); n > 1 {
		return fmt.Sprintf("%d minutes", n)
	}
	return "1 minute"
}

// stream sends one try. Deltas go to the coalescer; parts are kept in
// resp.
func (r *runner) stream(ctx context.Context, t *turn, req provider.Request, resp *response) error {
	co := newCoalescer(deltaEvery, r.o.pub.Delta)
	defer co.flush()
	for ev, err := range r.o.d.Models.Stream(ctx, limit.Interactive, req) {
		if err != nil {
			return err
		}
		r.cs.moved.Store(time.Now().UnixNano())
		switch ev.Kind {
		case provider.EventWait:
			// The provider is paused after a rate limit or an overload;
			// the Registry waits it out. *Retry now* ends the wait.
			if ev.Wait <= 0 {
				r.setRetry(nil, nil)
				break
			}
			kind := ev.Paused
			if kind == "" {
				kind = provider.RateLimited
			}
			r.setRetry(&chat.Retry{Provider: t.provider, Kind: string(kind), At: retryAt(ev.Wait)},
				func() { r.o.d.Models.Resume(t.provider) })
		case provider.EventDelta:
			at := resp.delta(ev.PartKind, ev.Text)
			co.add(chat.Delta{Project: r.p.ID, Chat: t.ch.ID, Seq: r.cs.seq.Load(), Message: resp.id, Part: len(resp.parts), Kind: ev.PartKind, Text: ev.Text, Offset: at})
		case provider.EventPart:
			co.flush()
			resp.part(*ev.Part)
		case provider.EventDone:
			if ev.Usage != nil {
				resp.usage = *ev.Usage
			}
			resp.stop = ev.Stop
		}
	}
	return nil
}

// setRetry shows or clears the wait before the next try. now ends the wait
// early (*Retry now*).
func (r *runner) setRetry(rt *chat.Retry, now func()) {
	cs := r.cs
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if rt == nil && cs.retry == nil {
		return
	}
	cs.retry, cs.retryNow = rt, nil
	if now != nil {
		cs.retryNow = sync.OnceFunc(now)
	}
	r.o.publishStatus(cs)
}

// runTools runs the calls in order and writes each result as it is done,
// in one tool message. After Stop the remaining calls get "cancelled by
// user" (8.3). Calls in the answer to the last request, which had no
// tools, are closed without running.
func (r *runner) runTools(ctx context.Context, t *turn, resp *response, calls []chat.ToolCall, last bool) error {
	tm := &toolMsg{r: r, t: t, id: resp.toolID}
	allBad := !last
	for i, c := range calls {
		parts := []chat.Part{{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: c.ID, Text: "not run: the turn reached its request limit", IsError: true}}}
		if !last {
			var bad bool
			parts, bad = r.runTool(ctx, t, tm, resp.id, i, c)
			allBad = allBad && bad
		}
		for _, part := range parts {
			if _, err := tm.add(ctx, part); err != nil {
				return err
			}
		}
	}
	if !allBad {
		t.bad = 0
		return nil
	}
	if t.bad++; t.bad >= maxBadRequests {
		return errBadCalls
	}
	return nil
}

// maxBadRequests is how many requests in a row may have only tool calls
// that couldn't run (bad JSON, an unknown tool, wrong arguments) before
// the turn stops (8.3). The model gets each error and may fix the call.
const maxBadRequests = 3

var errBadCalls = fmt.Errorf("the model sent tool calls that couldn't run in %d requests in a row", maxBadRequests)

// toolMsg is an answer's tool message: the results of its calls, and the
// cards and forms they wait for (8.8). It is written with its first part.
type toolMsg struct {
	r     *runner
	t     *turn
	id    id.Message
	wrote bool
}

// add writes a part, publishes it and returns its index. It writes after
// Stop too.
func (m *toolMsg) add(ctx context.Context, p chat.Part) (int, error) {
	wctx, cancel := writing(ctx)
	defer cancel()
	m.r.cs.pub.Lock()
	defer m.r.cs.pub.Unlock()
	i, seq, err := m.store(wctx, p)
	if err != nil {
		return 0, err
	}
	m.publish(seq, i, p)
	return i, nil
}

// store writes a part without publishing it, and returns its index and
// the chat's new sequence number. cs.pub is held.
func (m *toolMsg) store(ctx context.Context, p chat.Part) (int, uint64, error) {
	var (
		i   int
		seq uint64
		err error
	)
	if m.wrote {
		i, seq, err = m.r.p.Chats.AppendPart(ctx, m.id, p)
	} else {
		_, seq, err = m.r.p.Chats.AppendMessage(ctx, chat.Message{ID: m.id, Chat: m.t.ch.ID, Turn: m.t.n, Role: chat.RoleTool, Parts: []chat.Part{p}})
	}
	if err != nil {
		return 0, 0, err
	}
	m.wrote = true
	m.r.cs.bumpSeq(seq)
	return i, seq, nil
}

// set replaces an answered or closed card; its PartDone has the same
// index, so the frontend replaces it too.
func (m *toolMsg) set(ctx context.Context, i int, p chat.Part) error {
	wctx, cancel := writing(ctx)
	defer cancel()
	m.r.cs.pub.Lock()
	defer m.r.cs.pub.Unlock()
	seq, err := m.r.p.Chats.SetPart(wctx, m.id, i, p)
	if err != nil {
		return err
	}
	m.r.cs.bumpSeq(seq)
	m.publish(seq, i, p)
	return nil
}

// publish sends part i's PartDone. cs.pub is held.
func (m *toolMsg) publish(seq uint64, i int, p chat.Part) {
	m.r.o.pub.Part(chat.PartDone{Project: m.r.p.ID, Chat: m.t.ch.ID, Seq: seq, Message: m.id, Turn: m.t.n, Index: i, Part: p})
}

// runTool runs one call: its approvals first (8.8), then the tool. Every
// outcome is a result the model can read; a panic or a system failure is
// logged too.
func (r *runner) runTool(ctx context.Context, t *turn, tm *toolMsg, msg id.Message, n int, c chat.ToolCall) ([]chat.Part, bool) {
	result := func(text string) []chat.Part {
		return []chat.Part{{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: c.ID, Text: text, IsError: true}}}
	}
	failed := func(err error) ([]chat.Part, bool) {
		var te *tool.Error
		switch {
		case ctx.Err() != nil:
			return result(cancelled(ctx)), false
		case errors.As(err, &te):
			return result(te.Msg), errors.Is(err, tool.ErrArgs)
		}
		r.o.d.Log.Error("agent: tool failed", "tool", c.Name, "chat", t.ch.ID, "err", err)
		return result(fmt.Sprintf("%s failed: %v", c.Name, err)), false
	}
	if ctx.Err() != nil {
		return result(cancelled(ctx)), false
	}
	if c.Invalid != "" {
		return result(invalidArgs(c.Invalid)), true
	}
	tl, ok := r.o.d.Tools.Get(c.Name)
	if !ok || tl.Spec().Mother && t.ch.Kind != chat.KindMother {
		return result("there is no tool named " + c.Name), true
	}
	env := &tool.Env{
		Project: r.p, Chat: t.ch.ID, Message: msg, Source: id.SourceOf("message", string(msg)),
		Priority: limit.Interactive, PreviewTokens: t.set.Context.ToolPreviewTokens, ChatStatus: r.o.ChatStatus,
		Ask: func(ctx context.Context, q chat.Question) (chat.Question, error) {
			p, err := r.await(ctx, tm, chat.Part{Kind: chat.PartQuestion, Question: &q})
			return *p.Question, err
		},
		Skill: func(ctx context.Context, name, file string) (tool.Result, error) {
			return r.loadSkill(ctx, t, tm, name, file)
		},
	}
	call := tool.Call{ID: c.ID, Args: c.Args, Env: env}
	needs := tool.Needs{Effects: tl.Spec().Effects}
	if pf, ok := tl.(tool.Preflighter); ok {
		var err error
		if needs, err = pf.Preflight(ctx, call); err != nil {
			return failed(err)
		}
	}
	asks, autos, err := r.approve(ctx, needs)
	if err != nil {
		return failed(err)
	}
	for _, a := range asks {
		p, err := r.await(ctx, tm, chat.Part{Kind: chat.PartApproval, Approval: &a})
		if err != nil {
			return failed(err)
		}
		if p.Approval.Answer == chat.GrantDeny {
			return result(denied(p.Approval)), false
		}
	}
	// The level's approvals are recorded once the cards are approved, so a
	// denied call grants nothing.
	for _, a := range autos {
		if err := r.autoApprove(ctx, tm, a); err != nil {
			return failed(err)
		}
	}
	if needs.Effects&tool.Network != 0 {
		hosts, err := r.p.PrivateHosts(ctx)
		if err != nil {
			return failed(err)
		}
		env.PrivateHosts = func(h string) bool { return slices.Contains(hosts, strings.ToLower(h)) }
	}
	timeout := tl.Spec().Timeout
	if timeout <= 0 {
		timeout = tool.DefaultTimeout
	}
	start := r.moving(timeout)
	res, err := r.safeRun(ctx, tl, call)
	if needs.Effects&tool.Untrusted != 0 {
		// Outside data entered the turn: Auto asks again until the user's
		// next message (8.8).
		r.cs.mu.Lock()
		r.cs.untrusted = true
		r.cs.mu.Unlock()
	}
	if err != nil && ctx.Err() != nil {
		r.o.d.Traces.tool(t.trace, ToolTrace{CallID: c.ID, Name: c.Name, Started: start, Took: time.Since(start), Bytes: len(res.Text), Error: true})
		return result(cancelled(ctx)), false
	}
	bad := errors.Is(err, tool.ErrArgs)
	out, err := tool.Output(context.WithoutCancel(ctx), call, c.Name, n, res, err)
	r.o.d.Traces.tool(t.trace, ToolTrace{CallID: c.ID, Name: c.Name, Started: start, Took: time.Since(start),
		Bytes: max(len(res.Text), len(out.Text)), Ref: out.Ref, Error: err != nil || out.IsError})
	if err != nil {
		r.o.d.Log.Error("agent: tool failed", "tool", c.Name, "chat", t.ch.ID, "err", err)
		return result(fmt.Sprintf("%s failed: %v", c.Name, err)), false
	}
	out.Text += r.loadWith(ctx, t, tm, c.Name)
	parts := []chat.Part{{Kind: chat.PartToolResult, ToolResult: &out}}
	if !out.IsError {
		for _, img := range res.Images {
			parts = append(parts, chat.Part{Kind: chat.PartImage, Image: &img})
		}
	}
	return parts, bad
}

// invalidArgs is the result for a call whose arguments are not valid JSON,
// with the parser's error and where it is, so the model can fix the call.
func invalidArgs(raw string) string {
	var v any
	err := json.Unmarshal([]byte(raw), &v)
	var se *json.SyntaxError
	if errors.As(err, &se) {
		return fmt.Sprintf("the arguments are not valid JSON: %v, at byte %d of %d. Send the call again with valid JSON.", se, se.Offset, len(raw))
	}
	return fmt.Sprintf("the arguments are not valid JSON: %v. Send the call again with valid JSON.", err)
}

// moving starts a request or a tool call that may go limit without
// moving before the runtime panel flags it (8.4). It returns the start.
func (r *runner) moving(limit time.Duration) time.Time {
	now := time.Now()
	r.cs.mu.Lock()
	r.cs.active, r.cs.limit = now, limit
	r.cs.mu.Unlock()
	return now
}

func (r *runner) safeRun(ctx context.Context, tl tool.Tool, call tool.Call) (res tool.Result, err error) {
	defer func() {
		if p := recover(); p != nil {
			r.o.d.Log.Error("agent: tool panicked", "tool", tl.Spec().Name, "panic", p, "stack", string(debug.Stack()))
			err = fmt.Errorf("the tool crashed: %v", p)
		}
	}()
	return tool.Run(ctx, tl, call)
}

func cancelled(ctx context.Context) string {
	switch cause := context.Cause(ctx); {
	case errors.Is(cause, errStopped):
		return "cancelled by user"
	case errors.Is(cause, errClosed):
		return "cancelled: the project closed"
	}
	return "cancelled: the app closed"
}

// writeMessage stores a message and publishes its parts. It writes after
// Stop too.
func (r *runner) writeMessage(ctx context.Context, m chat.Message) error {
	wctx, cancel := writing(ctx)
	defer cancel()
	_, err := r.o.write(wctx, r.cs, r.p, m)
	return err
}

// writing is the context for a write: Stop doesn't cancel it, so the
// history stays valid, but it is bounded.
func writing(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
}

// finish ends a turn that did not end normally. Stop and a closing app end
// it quietly; an error adds a notice that says why, and Retry continues
// the turn (8.3).
func (r *runner) finish(ctx context.Context, t *turn, err error) {
	failed := !errors.Is(err, errStopped) && !errors.Is(err, errClosed) && !errors.Is(err, context.Canceled)
	if failed {
		r.o.d.Log.Warn("agent: turn stopped", "project", r.p.ID, "chat", r.cs.key.c, "turn", t.n, "err", err)
		n := chat.Notice{Kind: chat.NoticeTurnFailed, Text: failure(err, t)}
		if werr := r.writeMessage(ctx, chat.Message{Chat: r.cs.key.c, Turn: t.n, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartNotice, Notice: &n}}}); werr != nil {
			r.o.d.Log.Error("agent: writing the failure notice", "chat", r.cs.key.c, "err", werr)
		}
	}
	r.cs.mu.Lock()
	r.idle(failed)
	r.cs.mu.Unlock()
}

// idle marks the turn as ended. cs.mu is held.
func (r *runner) idle(failed bool) {
	cs := r.cs
	cs.running, cs.stopping, cs.cancel, cs.retryNow, cs.retry, cs.resp = false, false, nil, nil, nil, nil
	cs.failed = failed
	r.o.publishStatus(cs)
}

// failure is the notice for an error that stopped a turn: what happened
// and what the user can do (8.3).
func failure(err error, t *turn) string {
	var g *gaveUp
	if errors.As(err, &g) {
		return fmt.Sprintf("[the turn stopped: %s is %s; retried for %s. Use Retry to try again.]", g.last.Provider, kindText(g.last.Kind), minutes(g.after))
	}
	var pe *provider.Error
	if errors.As(err, &pe) {
		switch pe.Kind {
		case provider.Quota:
			return fmt.Sprintf("[the turn stopped: %s says the quota or credit is used up: %s. Add credit, or pick another model for this chat.]", pe.Provider, short(pe.Message))
		case provider.TooLarge:
			return "[the turn stopped: the request is too large for this model. Pick a model with a larger context window, or start a new chat.]"
		default:
			return fmt.Sprintf("[the turn stopped: %s refused the request: %s. Check the provider's settings, or pick another model.]", pe.Provider, short(pe.Message))
		}
	}
	if errors.Is(err, errBadCalls) {
		return fmt.Sprintf("[the turn stopped: the model sent tool calls that couldn't run, %d times in a row. Use Retry to try again, or pick another model.]", maxBadRequests)
	}
	if errors.Is(err, provider.ErrUnknownModel) || errors.Is(err, provider.ErrUnknownProvider) {
		return fmt.Sprintf("[the turn stopped: the model %q is not set up. Connect a provider in Settings, or pick another model.]", t.ch.Model)
	}
	return fmt.Sprintf("[the turn stopped: %s]", short(err.Error()))
}

func kindText(k provider.ErrorKind) string {
	switch k {
	case provider.RateLimited:
		return "rate limited"
	case provider.Overloaded:
		return "overloaded"
	case provider.Transport:
		return "not reachable"
	}
	return string(k)
}

// short cuts s to 300 characters.
func short(s string) string {
	if r := []rune(s); len(r) > 300 {
		return string(r[:299]) + "…"
	}
	return s
}
