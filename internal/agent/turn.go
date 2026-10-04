package agent

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

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
}

// turn runs steps until the model answers without tool calls, the request
// cap is reached, or the turn is stopped or fails. A message sent during
// the last answer gets another step. It reports whether the turn ended
// normally.
func (r *runner) turn(ctx context.Context, n int) (*turn, bool) {
	cs := r.cs
	t := &turn{n: n, set: r.o.d.Settings()}
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
	r.cs.requests, r.cs.active = t.requests, time.Now()
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

func (resp *response) delta(k chat.PartKind, s string) {
	resp.mu.Lock()
	defer resp.mu.Unlock()
	if k != resp.kind {
		resp.kind = k
		resp.text.Reset()
	}
	resp.text.WriteString(s)
}

func (resp *response) part(p chat.Part) {
	resp.mu.Lock()
	defer resp.mu.Unlock()
	resp.parts = append(resp.parts, p)
	resp.kind = ""
	resp.text.Reset()
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
		err := r.stream(ctx, t, req, resp)
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
			resp.delta(ev.PartKind, ev.Text)
			co.add(chat.Delta{Project: r.p.ID, Chat: t.ch.ID, Seq: r.cs.seq.Load(), Message: resp.id, Part: len(resp.parts), Kind: ev.PartKind, Text: ev.Text})
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
	wrote := false
	for i, c := range calls {
		parts := []chat.Part{{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: c.ID, Text: "not run: the turn reached its request limit", IsError: true}}}
		if !last {
			parts = r.runTool(ctx, t, resp.id, i, c)
		}
		for _, part := range parts {
			var err error
			if !wrote {
				err = r.writeMessage(ctx, chat.Message{ID: resp.toolID, Chat: t.ch.ID, Turn: t.n, Role: chat.RoleTool, Parts: []chat.Part{part}})
				wrote = err == nil
			} else {
				err = r.writePart(ctx, resp.toolID, part)
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// runTool runs one call. Every outcome is a result the model can read; a
// panic or a system failure is logged too.
func (r *runner) runTool(ctx context.Context, t *turn, msg id.Message, n int, c chat.ToolCall) []chat.Part {
	result := func(text string) []chat.Part {
		return []chat.Part{{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: c.ID, Text: text, IsError: true}}}
	}
	if ctx.Err() != nil {
		return result(cancelled(ctx))
	}
	tl, ok := r.o.d.Tools.Get(c.Name)
	if !ok || tl.Spec().Mother && t.ch.Kind != chat.KindMother {
		return result("there is no tool named " + c.Name)
	}
	call := tool.Call{ID: c.ID, Args: c.Args, Env: &tool.Env{
		Project: r.p, Chat: t.ch.ID, Message: msg, Source: id.SourceOf("message", string(msg)),
		Priority: limit.Interactive, PreviewTokens: t.set.Context.ToolPreviewTokens,
	}}
	res, err := r.safeRun(ctx, tl, call)
	if err != nil && ctx.Err() != nil {
		return result(cancelled(ctx))
	}
	out, err := tool.Output(context.WithoutCancel(ctx), call, c.Name, n, res, err)
	if err != nil {
		r.o.d.Log.Error("agent: tool failed", "tool", c.Name, "chat", t.ch.ID, "err", err)
		return result(fmt.Sprintf("%s failed: %v", c.Name, err))
	}
	parts := []chat.Part{{Kind: chat.PartToolResult, ToolResult: &out}}
	if !out.IsError {
		for _, img := range res.Images {
			parts = append(parts, chat.Part{Kind: chat.PartImage, Image: &img})
		}
	}
	return parts
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
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
	defer cancel()
	_, err := r.o.write(wctx, r.cs, r.p, m)
	return err
}

func (r *runner) writePart(ctx context.Context, m id.Message, p chat.Part) error {
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
	defer cancel()
	r.cs.pub.Lock()
	defer r.cs.pub.Unlock()
	i, seq, err := r.p.Chats.AppendPart(wctx, m, p)
	if err != nil {
		return err
	}
	r.cs.bumpSeq(seq)
	r.o.pub.Part(chat.PartDone{Project: r.p.ID, Chat: r.cs.key.c, Seq: seq, Message: m, Index: i, Part: p})
	return nil
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
