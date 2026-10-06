package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/logfile"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/store"
)

// DevService is the developer tools' read-only view (SPEC 8.4): the turn
// inspector and the runtime panel. It is bound only when the developer
// tools are on. Every text it returns has its secrets redacted (6.7).
type DevService struct {
	base
	projects *project.Manager
	models   *provider.Registry
	traces   *agent.Traces
	logs     string
	now      func() time.Time
}

// writerStuck is how long a database write may run before the runtime
// panel flags it. Writes take milliseconds; a chat write gives up at 30 s.
const writerStuck = 30 * time.Second

// Providers lists each provider's calls, limits and pause.
func (s *DevService) Providers(ctx context.Context) (ps []ProviderStatus, err error) {
	defer s.guard("dev.providers", &err)
	return providerStatus(s.models, s.redact), nil
}

// Log returns the last log lines that contain match; "" returns the last
// lines.
func (s *DevService) Log(ctx context.Context, match string) (lines []string, err error) {
	defer s.guard("dev.log", &err)
	lines, err = logfile.Tail(s.logs, match)
	for i, l := range lines {
		lines[i] = s.redact(l)
	}
	if lines == nil {
		lines = []string{}
	}
	return lines, err
}

// TurnItem is a recorded turn, for the inspector's pickers.
type TurnItem struct {
	Project     id.Project `json:"project"`
	ProjectName string     `json:"project_name"`
	Chat        id.Chat    `json:"chat"`
	ChatTitle   string     `json:"chat_title"`
	Turn        int        `json:"turn"`
	Model       string     `json:"model"`
	Started     time.Time  `json:"started"`
	Running     bool       `json:"running"`
	Requests    int        `json:"requests"`
}

// TurnView is one turn in the inspector. Times are in milliseconds from
// the turn's start.
type TurnView struct {
	TurnItem
	TookMS   int64         `json:"took_ms"` // so far, while it runs
	Requests []RequestView `json:"requests"`
	Tools    []ToolView    `json:"tools"`
}

// RequestView is one try of a request.
type RequestView struct {
	StartMS int64       `json:"start_ms"`
	TookMS  int64       `json:"took_ms"`
	Last    bool        `json:"last"`    // the turn's last request, without tools
	Dropped bool        `json:"dropped"` // its texts were dropped to save memory
	Usage   chat.Usage  `json:"usage"`
	Stop    string      `json:"stop,omitempty"`
	Parts   []string    `json:"parts"` // the kinds of the parts that came back
	Err     string      `json:"err,omitempty"`
	Blocks  []BlockView `json:"blocks"`
}

// BlockView is one context block (3.1). Tokens are estimated. Cache says
// how much of it the provider read from its cache: hit, part or miss; ""
// when the request has no usage.
type BlockView struct {
	Name   string `json:"name"`
	Tokens int    `json:"tokens"`
	Point  bool   `json:"point"` // a cache point after it
	Cache  string `json:"cache"`
}

// ToolView is one tool call's run.
type ToolView struct {
	Request int    `json:"request"`
	CallID  string `json:"call_id"`
	Name    string `json:"name"`
	StartMS int64  `json:"start_ms"`
	TookMS  int64  `json:"took_ms"`
	Bytes   int    `json:"bytes"`
	Ref     string `json:"ref,omitempty"`
	Error   bool   `json:"error"`
}

// Turns lists the recorded turns, newest first. They are kept in memory
// from the start of the app: the last 100.
func (s *DevService) Turns(ctx context.Context) (out []TurnItem, err error) {
	defer s.guard("dev.turns", &err)
	names := s.projectNames(ctx)
	out = []TurnItem{}
	for _, t := range s.traces.List() {
		out = append(out, s.item(t, names))
	}
	return out, nil
}

// Turn is one recorded turn.
func (s *DevService) Turn(ctx context.Context, p id.Project, c id.Chat, n int) (v TurnView, err error) {
	defer s.guard("dev.turn", &err)
	t, ok := s.traces.Turn(agent.TraceKey{Project: p, Chat: c, Turn: n})
	if !ok {
		return v, &UIError{Kind: KindNotFound, Message: "This turn isn't recorded. Turns are kept from the start of the app: the last 100."}
	}
	end := t.Ended
	if end.IsZero() {
		end = s.now()
	}
	v = TurnView{TurnItem: s.item(t, s.projectNames(ctx)), TookMS: end.Sub(t.Started).Milliseconds(), Requests: []RequestView{}, Tools: []ToolView{}}
	for _, r := range t.Requests {
		rv := RequestView{StartMS: r.Started.Sub(t.Started).Milliseconds(), TookMS: r.Took.Milliseconds(), Last: r.Last,
			Dropped: r.Dropped, Usage: r.Usage, Stop: string(r.Stop), Parts: []string{}, Err: s.redact(r.Err)}
		if !r.Done {
			rv.TookMS = s.now().Sub(r.Started).Milliseconds() // still streaming
		}
		for _, k := range r.Parts {
			rv.Parts = append(rv.Parts, string(k))
		}
		hits := cacheHits(r.Blocks, r.Usage)
		for i, b := range r.Blocks {
			rv.Blocks = append(rv.Blocks, BlockView{Name: b.Name, Tokens: b.Tokens, Point: b.Cache, Cache: hits[i]})
		}
		v.Requests = append(v.Requests, rv)
	}
	for _, tl := range t.Tools {
		v.Tools = append(v.Tools, ToolView{Request: tl.Request, CallID: tl.CallID, Name: tl.Name, StartMS: tl.Started.Sub(t.Started).Milliseconds(),
			TookMS: tl.Took.Milliseconds(), Bytes: tl.Bytes, Ref: tl.Ref, Error: tl.Error})
	}
	return v, nil
}

// Block is the text of one context block of a request, as the provider
// got it.
func (s *DevService) Block(ctx context.Context, p id.Project, c id.Chat, n, request, block int) (text string, err error) {
	defer s.guard("dev.block", &err)
	t, ok := s.traces.Turn(agent.TraceKey{Project: p, Chat: c, Turn: n})
	if !ok || request < 0 || request >= len(t.Requests) {
		return "", &UIError{Kind: KindNotFound, Message: "This request isn't recorded."}
	}
	r := t.Requests[request]
	if r.Dropped {
		return "", &UIError{Kind: KindNotFound, Message: "This request's texts were dropped to save memory; newer turns keep theirs."}
	}
	text, ok = BlockText(r.Req, t.Turn, block)
	if !ok {
		return "", &UIError{Kind: KindNotFound, Message: "This block isn't recorded."}
	}
	return s.redact(text), nil
}

// BlockText is the text of block i of agent.Blocks(req, turn).
func BlockText(req provider.Request, turn, i int) (string, bool) {
	if i < 0 {
		return "", false
	}
	if len(req.Tools) > 0 {
		if i == 0 {
			return toolsText(req.Tools), true
		}
		i--
	}
	if i < len(req.System) {
		return req.System[i].Text, true
	}
	i -= len(req.System)
	before, now := agent.SplitTurn(req.Messages, turn)
	if len(before) > 0 {
		if i == 0 {
			return messagesText(before), true
		}
		i--
	}
	if i == 0 {
		return messagesText(now), true
	}
	return "", false
}

func toolsText(ts []provider.ToolDef) string {
	var b strings.Builder
	for _, t := range ts {
		fmt.Fprintf(&b, "── %s\n%s\n%s\n\n", t.Name, t.Description, t.Schema)
	}
	return b.String()
}

func messagesText(ms []chat.Message) string {
	var b strings.Builder
	for _, m := range ms {
		fmt.Fprintf(&b, "── %s · turn %d\n", m.Role, m.Turn)
		for _, p := range m.Parts {
			b.WriteString(partText(p))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func partText(p chat.Part) string {
	switch {
	case p.Text != nil:
		return p.Text.Text
	case p.Thinking != nil:
		if p.Thinking.Text == "" && p.Thinking.Redacted != "" {
			return "[thinking, redacted by the provider]"
		}
		return "[thinking]\n" + p.Thinking.Text
	case p.ToolCall != nil:
		return fmt.Sprintf("[tool call %s · %s]\n%s", p.ToolCall.Name, p.ToolCall.ID, p.ToolCall.Args)
	case p.ToolResult != nil:
		head := "[tool result · " + p.ToolResult.CallID
		if p.ToolResult.IsError {
			head += " · error"
		}
		if p.ToolResult.Ref != "" {
			head += " · full output at " + p.ToolResult.Ref
		}
		return head + "]\n" + p.ToolResult.Text
	case p.Image != nil:
		return "[image " + p.Image.Ref + "]"
	case p.Notice != nil:
		return "[notice · " + string(p.Notice.Kind) + "]\n" + p.Notice.Text
	}
	b, _ := json.Marshal(p)
	return "[" + string(p.Kind) + "]\n" + string(b)
}

// cacheHits says, per block, how much the provider read from its cache.
// The estimates are scaled to the prompt the provider counted; the cache
// covers the blocks from the start.
func cacheHits(bs []agent.TraceBlock, u chat.Usage) []string {
	out := make([]string, len(bs))
	prompt, est := u.Input+u.CacheRead+u.CacheWrite, 0
	for _, b := range bs {
		est += b.Tokens
	}
	if prompt == 0 || est == 0 {
		return out
	}
	scale := float64(prompt) / float64(est)
	from := 0.0
	for i, b := range bs {
		to := from + float64(b.Tokens)*scale
		switch read := float64(u.CacheRead); {
		case read >= to-0.5:
			out[i] = "hit"
		case read > from:
			out[i] = "part"
		default:
			out[i] = "miss"
		}
		from = to
	}
	return out
}

func (s *DevService) item(t agent.TurnTrace, names map[id.Project]string) TurnItem {
	return TurnItem{Project: t.Project, ProjectName: names[t.Project], Chat: t.Chat, ChatTitle: t.Title, Turn: t.Turn,
		Model: t.Model, Started: t.Started, Running: t.Ended.IsZero(), Requests: len(t.Requests)}
}

// projectNames maps the registry's projects to their names; a failure
// leaves them unnamed.
func (s *DevService) projectNames(ctx context.Context) map[id.Project]string {
	names := map[id.Project]string{}
	es, err := s.projects.List(ctx)
	if err != nil {
		s.log.Warn("dev: listing the projects failed", "err", err)
	}
	for _, e := range es {
		names[e.ID] = e.Name
	}
	return names
}

// Runtime is everything running now (SPEC 8.4). Stuck counts what hasn't
// moved for longer than its limit.
type Runtime struct {
	Calls     Slots            `json:"calls"` // the max_parallel_calls slots
	Providers []ProviderStatus `json:"providers"`
	Projects  []ProjectRuntime `json:"projects"`
	Stuck     int              `json:"stuck"`
}

// Slots is a limit's slots in use and the callers waiting (7.6).
type Slots struct {
	Size    int `json:"size"`
	InUse   int `json:"in_use"`
	Waiting int `json:"waiting"`
}

// ProjectRuntime is an open project's work and its database writers.
type ProjectRuntime struct {
	Project id.Project   `json:"project"`
	Name    string       `json:"name"`
	Leases  int          `json:"leases"`
	Work    []WorkView   `json:"work"`
	Writers []WriterView `json:"writers"`
}

// WorkView is a chat in a turn, a background task or a run.
type WorkView struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	Title    string    `json:"title"`
	State    string    `json:"state"`
	Progress string    `json:"progress"`
	Err      string    `json:"err,omitempty"`
	Started  time.Time `json:"started"`
	IdleMS   int64     `json:"idle_ms"`  // since it last moved
	LimitMS  int64     `json:"limit_ms"` // 0: no limit, e.g. while it waits for the user
	Stuck    bool      `json:"stuck"`
}

// WriterView is one database file's writer (7.3).
type WriterView struct {
	File        string `json:"file"`
	Interactive int    `json:"interactive"` // requests waiting
	Background  int    `json:"background"`
	Busy        bool   `json:"busy"`
	RunningMS   int64  `json:"running_ms"` // how long the current request has run
	Done        int64  `json:"done"`
	LastError   string `json:"last_error,omitempty"`
	Stuck       bool   `json:"stuck"`
}

// Runtime is a snapshot of the open projects, the LLM-call slots and the
// providers. It opens nothing.
func (s *DevService) Runtime(ctx context.Context) (rt Runtime, err error) {
	defer s.guard("dev.runtime", &err)
	c := s.models.Calls()
	rt = Runtime{Calls: Slots{Size: c.Size, InUse: c.InUse, Waiting: c.Waiting}, Providers: providerStatus(s.models, s.redact), Projects: []ProjectRuntime{}}
	now := s.now()
	for _, a := range s.projects.Activities() {
		pr := ProjectRuntime{Project: a.Project, Name: a.Name, Leases: a.Leases, Work: []WorkView{}}
		for _, w := range a.Work {
			idle := now.Sub(w.LastActivity)
			wv := WorkView{ID: w.ID, Kind: w.Kind, Title: w.Title, State: w.State, Progress: s.redact(w.Progress), Err: s.redact(w.Err),
				Started: w.Started, IdleMS: idle.Milliseconds(), LimitMS: w.Limit.Milliseconds(), Stuck: w.Limit > 0 && idle > w.Limit}
			pr.Work = append(pr.Work, wv)
		}
		pr.Writers = []WriterView{s.writer("project.db", a.Writer), s.writer("chats.db", a.Chats)}
		for _, w := range pr.Work {
			if w.Stuck {
				rt.Stuck++
			}
		}
		for _, w := range pr.Writers {
			if w.Stuck {
				rt.Stuck++
			}
		}
		rt.Projects = append(rt.Projects, pr)
	}
	return rt, nil
}

func (s *DevService) writer(file string, w store.WriterStats) WriterView {
	return WriterView{File: file, Interactive: w.Interactive, Background: w.Background, Busy: w.Busy, RunningMS: w.Age.Milliseconds(),
		Done: w.Done, LastError: s.redact(w.LastError), Stuck: w.Busy && w.Age > writerStuck}
}
