package agent

import (
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/provider"
)

// Traces keeps what recent turns sent and got back, for the turn inspector
// (SPEC 8.4). It lives in memory only, from the start of the app while the
// developer tools are on. When the requests' texts pass the budget, the
// oldest lose their texts first; their numbers stay.
type Traces struct {
	mu     sync.Mutex
	turns  []*TurnTrace // oldest first
	max    int
	budget int // bytes of request text kept
	size   int
}

// Trace limits: the turns kept, and the request text kept across them.
const (
	traceTurns  = 100
	traceBudget = 32 << 20
)

// NewTraces returns an empty store with the default limits.
func NewTraces() *Traces { return &Traces{max: traceTurns, budget: traceBudget} }

// TurnTrace is one turn: its requests and its tool calls.
type TurnTrace struct {
	Project  id.Project
	Chat     id.Chat
	Turn     int
	Title    string // the chat's title at the turn's start
	Model    string // "provider/id"
	Started  time.Time
	Ended    time.Time // zero while the turn runs
	Requests []RequestTrace
	Tools    []ToolTrace
}

// RequestTrace is one try of a request, as the provider got it.
type RequestTrace struct {
	Started time.Time
	Took    time.Duration
	Done    bool             // the try has ended
	Last    bool             // the turn's last request, without tools
	Req     provider.Request // System, Tools and Messages as sent; empty once Dropped
	Dropped bool             // the texts went over the budget
	Blocks  []TraceBlock
	Message id.Message // the assistant message it would write
	Usage   chat.Usage
	Stop    provider.StopReason
	Parts   []chat.PartKind // what came back
	Err     string
}

// TraceBlock is one context block of a request (3.1), in the order
// providers cache them: tools, the system blocks, then the messages.
type TraceBlock struct {
	Name   string
	Tokens int  // estimated, 4 bytes a token
	Cache  bool // a cache point after it
}

// ToolTrace is one tool call's run.
type ToolTrace struct {
	Request int // the index of the request that asked for it
	CallID  string
	Name    string
	Started time.Time
	Took    time.Duration
	Bytes   int    // the full result's size
	Ref     string // where a long result is kept in the bucket
	Error   bool
}

// TraceKey finds a turn.
type TraceKey struct {
	Project id.Project
	Chat    id.Chat
	Turn    int
}

func (t *TurnTrace) key() TraceKey { return TraceKey{t.Project, t.Chat, t.Turn} }

// Blocks splits a request into its context blocks: tools, each system
// block, the history window and this turn.
func Blocks(req provider.Request, turn int) []TraceBlock {
	var out []TraceBlock
	if len(req.Tools) > 0 {
		out = append(out, TraceBlock{Name: "Tools", Tokens: provider.EstimateTokens(provider.Request{Tools: req.Tools})})
	}
	for i, b := range req.System {
		name := b.Name
		if name == "" {
			name = "System block " + strconv.Itoa(i+1)
		}
		out = append(out, TraceBlock{Name: name, Tokens: provider.EstimateTokens(provider.Request{System: []provider.Block{b}}), Cache: b.Cache})
	}
	before, now := SplitTurn(req.Messages, turn)
	if len(before) > 0 {
		out = append(out, TraceBlock{Name: "History window", Tokens: provider.EstimateTokens(provider.Request{Messages: before})})
	}
	out = append(out, TraceBlock{Name: "This turn", Tokens: provider.EstimateTokens(provider.Request{Messages: now})})
	return out
}

// SplitTurn splits messages into those of earlier turns and those of turn.
func SplitTurn(ms []chat.Message, turn int) (before, now []chat.Message) {
	i := slices.IndexFunc(ms, func(m chat.Message) bool { return m.Turn >= turn })
	if i < 0 {
		return ms, nil
	}
	return ms[:i], ms[i:]
}

// begin starts a turn's trace, or continues it after Retry.
func (s *Traces) begin(p id.Project, c id.Chat, n int) *TurnTrace {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := TraceKey{p, c, n}
	for _, t := range s.turns {
		if t.key() == k {
			t.Ended = time.Time{}
			return t
		}
	}
	t := &TurnTrace{Project: p, Chat: c, Turn: n, Started: time.Now()}
	s.turns = append(s.turns, t)
	if len(s.turns) > s.max {
		// The oldest ended turn goes; a running one still adds to it. With
		// every turn running, the store goes over max for a while.
		if i := slices.IndexFunc(s.turns, func(old *TurnTrace) bool { return !old.Ended.IsZero() }); i >= 0 {
			s.remove(i)
		}
	}
	return t
}

// remove drops turn i. s.mu is held.
func (s *Traces) remove(i int) {
	for _, r := range s.turns[i].Requests {
		s.size -= textSize(r)
	}
	s.turns = slices.Delete(s.turns, i, i+1)
}

// drop forgets a chat's turns, after Clear starts its turns again from 1.
func (s *Traces) drop(p id.Project, c id.Chat) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.turns) - 1; i >= 0; i-- {
		if t := s.turns[i]; t.Project == p && t.Chat == c {
			s.remove(i)
		}
	}
}

func textSize(r RequestTrace) int {
	if r.Dropped {
		return 0
	}
	return 4 * provider.EstimateTokens(r.Req)
}

// chat sets the turn's model and the chat's title once they are read.
func (s *Traces) chat(t *TurnTrace, model, title string) {
	if t == nil {
		return
	}
	s.mu.Lock()
	t.Model, t.Title = model, title
	s.mu.Unlock()
}

// request records a try as it starts and returns its index.
func (s *Traces) request(t *TurnTrace, req provider.Request, msg id.Message, last bool) int {
	if t == nil {
		return -1
	}
	r := RequestTrace{Started: time.Now(), Last: last, Message: msg, Blocks: Blocks(req, t.Turn),
		Req: provider.Request{System: req.System, Tools: req.Tools, Messages: req.Messages}}
	s.mu.Lock()
	defer s.mu.Unlock()
	t.Requests = append(t.Requests, r)
	s.size += textSize(r)
	// Over the budget, the oldest texts go first; the newest stays.
	for _, old := range s.turns {
		for i := range old.Requests {
			if s.size <= s.budget || old == t && i == len(t.Requests)-1 {
				return len(t.Requests) - 1
			}
			if !old.Requests[i].Dropped {
				s.size -= textSize(old.Requests[i])
				old.Requests[i].Req, old.Requests[i].Dropped = provider.Request{}, true
			}
		}
	}
	return len(t.Requests) - 1
}

// answered records how a try ended.
func (s *Traces) answered(t *TurnTrace, i int, resp *response, err error) {
	if t == nil || i < 0 {
		return
	}
	resp.mu.Lock()
	kinds := make([]chat.PartKind, len(resp.parts))
	for j, p := range resp.parts {
		kinds[j] = p.Kind
	}
	resp.mu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	r := &t.Requests[i]
	r.Took, r.Done, r.Usage, r.Stop, r.Parts = time.Since(r.Started), true, resp.usage, resp.stop, kinds
	if err != nil {
		r.Err = err.Error()
	}
}

// tool records a tool call's run.
func (s *Traces) tool(t *TurnTrace, tt ToolTrace) {
	if t == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tt.Request = len(t.Requests) - 1
	t.Tools = append(t.Tools, tt)
}

// end marks the turn as ended.
func (s *Traces) end(t *TurnTrace) {
	if t == nil {
		return
	}
	s.mu.Lock()
	t.Ended = time.Now()
	s.mu.Unlock()
}

// List returns the turns kept, newest first, without their requests'
// texts.
func (s *Traces) List() []TurnTrace {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TurnTrace, 0, len(s.turns))
	for _, t := range slices.Backward(s.turns) {
		c := *t
		c.Requests = make([]RequestTrace, len(t.Requests))
		for i, r := range t.Requests {
			r.Req = provider.Request{}
			c.Requests[i] = r
		}
		c.Tools = slices.Clone(t.Tools)
		out = append(out, c)
	}
	return out
}

// Turn returns a copy of one turn. The requests share their texts, which
// are never changed after they are recorded.
func (s *Traces) Turn(k TraceKey) (TurnTrace, bool) {
	if s == nil {
		return TurnTrace{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.turns {
		if t.key() == k {
			c := *t
			c.Requests, c.Tools = slices.Clone(t.Requests), slices.Clone(t.Tools)
			return c, true
		}
	}
	return TurnTrace{}, false
}
