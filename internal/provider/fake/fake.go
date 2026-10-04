// Package fake is a scripted Provider for tests: each call plays the next
// Reply. It never touches the network.
package fake

import (
	"context"
	"encoding/json"
	"iter"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
)

// Reply is one scripted answer.
type Reply struct {
	// Wait is how long each event takes to arrive.
	Wait   time.Duration
	Events []provider.Event
	// Err, if set, follows the events.
	Err error
	// Hang blocks after the events until the request is cancelled.
	Hang bool
}

// Provider plays its replies in order. When they run out it answers "ok".
type Provider struct {
	mu      sync.Mutex
	replies []Reply
	calls   []provider.Request
	Listed  []provider.ModelInfo // what Models returns
	ListErr error
}

// New returns a Provider with the given replies.
func New(replies ...Reply) *Provider { return &Provider{replies: replies} }

// Push adds replies.
func (p *Provider) Push(r ...Reply) {
	p.mu.Lock()
	p.replies = append(p.replies, r...)
	p.mu.Unlock()
}

// Calls returns the requests received so far.
func (p *Provider) Calls() []provider.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]provider.Request(nil), p.calls...)
}

// Factory returns a factory that always gives p.
func (p *Provider) Factory() provider.Factory {
	return func(provider.Connection) (provider.Provider, error) { return p, nil }
}

func (p *Provider) next(req provider.Request) Reply {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, req)
	if len(p.replies) == 0 {
		return Text("ok")
	}
	r := p.replies[0]
	p.replies = p.replies[1:]
	return r
}

// Stream plays the next reply.
func (p *Provider) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	r := p.next(req)
	return func(yield func(provider.Event, error) bool) {
		for _, ev := range r.Events {
			if err := sleep(ctx, r.Wait); err != nil {
				yield(provider.Event{}, provider.TransportError("fake", "cancelled", err))
				return
			}
			if !yield(ev, nil) {
				return
			}
		}
		if r.Hang {
			<-ctx.Done()
			yield(provider.Event{}, provider.TransportError("fake", "cancelled", ctx.Err()))
			return
		}
		if r.Err != nil {
			yield(provider.Event{}, r.Err)
		}
	}
}

// Models returns Listed.
func (p *Provider) Models(context.Context) ([]provider.ModelInfo, error) {
	return p.Listed, p.ListErr
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Text is a complete answer of one text part.
func Text(s string) Reply {
	return Reply{Events: []provider.Event{
		{Kind: provider.EventDelta, PartKind: chat.PartText, Text: s},
		{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: s}}},
		Done(provider.StopEnd, chat.Usage{Input: 10, Output: 1}),
	}}
}

// ToolCall is a complete answer of one tool call.
func ToolCall(id, name string, args any) Reply {
	b, _ := json.Marshal(args)
	return Reply{Events: []provider.Event{
		{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: id, Name: name, Args: b}}},
		Done(provider.StopToolUse, chat.Usage{Input: 10, Output: 5}),
	}}
}

// Done is the end event.
func Done(stop provider.StopReason, u chat.Usage) provider.Event {
	return provider.Event{Kind: provider.EventDone, Stop: stop, Usage: &u}
}

// Fail is a reply that fails at once with err.
func Fail(err error) Reply { return Reply{Err: err} }
