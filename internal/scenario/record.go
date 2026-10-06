package scenario

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

// Event kinds of a record, in the chat's order.
const (
	evUser   = "user"   // a user message's text
	evNotice = "notice" // a notice part, such as turn_failed
	evSkill  = "skill"  // the skill_loaded chip: a skill came into the chat
	evText   = "text"   // the agent's text
	evCall   = "call"   // a tool call
	evResult = "result" // a tool result
)

// event is one part of the chat, as the checks read it.
type event struct {
	kind   string
	turn   int
	msg    int // for a user event: the scripted message's index, or -1
	text   string
	tool   string // calls and results
	args   string // calls: the arguments as JSON
	callID string
	err    bool // results: an error
	final  bool // text of an assistant message without tool calls
	notice chat.NoticeKind
	skill  string
}

// record is what a run did: the chat's parts and the requests' traces.
type record struct {
	events   []event
	requests []agent.RequestTrace
	sc       *Scenario
}

// record reads the chat and its traces once the run is over.
func (r *run) record(ctx context.Context, traces *agent.Traces, sent []sent) (*record, error) {
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return nil, err
	}
	defer p.Release()
	ms, _, err := p.Chats.Messages(ctx, r.cid, 1, 0)
	if err != nil {
		return nil, err
	}
	index := map[id.Message]int{}
	for _, s := range sent {
		index[s.id] = s.index
	}
	rec := &record{sc: r.sc}
	for _, m := range ms {
		calls := slices.ContainsFunc(m.Parts, func(p chat.Part) bool { return p.ToolCall != nil })
		msg, scripted := index[m.ID]
		if !scripted {
			msg = -1
		}
		for _, p := range m.Parts {
			e := event{turn: m.Turn, msg: -1}
			switch {
			case p.Text != nil && m.Role == chat.RoleUser:
				e.kind, e.text, e.msg = evUser, p.Text.Text, msg
			case p.Text != nil && m.Role == chat.RoleAssistant:
				e.kind, e.text, e.final = evText, p.Text.Text, !calls
			case p.ToolCall != nil:
				e.kind, e.tool, e.args, e.callID = evCall, p.ToolCall.Name, string(p.ToolCall.Args), p.ToolCall.ID
			case p.ToolResult != nil:
				e.kind, e.text, e.callID, e.err = evResult, p.ToolResult.Text, p.ToolResult.CallID, p.ToolResult.IsError
				e.tool = rec.callTool(e.callID)
			case p.Notice != nil && p.Notice.Kind == chat.NoticeSkillLoaded:
				e.kind, e.text, e.skill = evSkill, p.Notice.Text, strings.TrimSpace(strings.TrimPrefix(p.Notice.Text, "Skill loaded:"))
			case p.Notice != nil:
				e.kind, e.text, e.notice = evNotice, p.Notice.Text, p.Notice.Kind
			default:
				continue
			}
			rec.events = append(rec.events, e)
		}
	}
	list := traces.List()
	for _, t := range slices.Backward(list) {
		if t.Chat == r.cid {
			rec.requests = append(rec.requests, t.Requests...)
		}
	}
	return rec, nil
}

func (rec *record) callTool(callID string) string {
	for _, e := range slices.Backward(rec.events) {
		if e.kind == evCall && e.callID == callID {
			return e.tool
		}
	}
	return ""
}

func (rec *record) turns() int {
	n := 0
	for _, e := range rec.events {
		n = max(n, e.turn)
	}
	return n
}

// promptOf is a request's prompt size: the input, cached or not.
func promptOf(u chat.Usage) int { return u.Input + u.CacheRead + u.CacheWrite }

func (rec *record) prompt() int {
	n := 0
	for _, q := range rec.requests {
		n += promptOf(q.Usage)
	}
	return n
}

func (rec *record) peak() int {
	n := 0
	for _, q := range rec.requests {
		n = max(n, promptOf(q.Usage))
	}
	return n
}

func (rec *record) output() int {
	n := 0
	for _, q := range rec.requests {
		n += q.Usage.Output
	}
	return n
}

// failure is the last request's error when the run ended on one.
func (rec *record) failure() string {
	if len(rec.requests) == 0 {
		return ""
	}
	return rec.requests[len(rec.requests)-1].Err
}

// transcript is the chat as Markdown, for reading a run.
func (rec *record) transcript() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s\n", rec.sc.ID, rec.sc.Title)
	if rec.sc.Why != "" {
		fmt.Fprintf(&b, "\nGood behaviour: %s\n", rec.sc.Why)
	}
	turn := 0
	for _, e := range rec.events {
		if e.turn != turn {
			turn = e.turn
			fmt.Fprintf(&b, "\n## Turn %d\n", turn)
		}
		switch e.kind {
		case evUser:
			fmt.Fprintf(&b, "\n**User:** %s\n", e.text)
		case evText:
			if t := strings.TrimSpace(e.text); t != "" {
				fmt.Fprintf(&b, "\n**Agent:** %s\n", t)
			}
		case evCall:
			fmt.Fprintf(&b, "\n> call %s %s\n", e.tool, clip(e.args, 600))
		case evResult:
			mark := ""
			if e.err {
				mark = " (error)"
			}
			fmt.Fprintf(&b, "\n> result %s%s:\n> %s\n", e.tool, mark, strings.ReplaceAll(clip(e.text, 600), "\n", "\n> "))
		case evSkill, evNotice:
			fmt.Fprintf(&b, "\n*%s*\n", e.text)
		}
	}
	if len(rec.requests) > 0 {
		fmt.Fprintf(&b, "\n## Requests\n\n| # | prompt | output | took | error |\n|---|---|---|---|---|\n")
		for i, q := range rec.requests {
			fmt.Fprintf(&b, "| %d | %d | %d | %.1f s | %s |\n", i+1, promptOf(q.Usage), q.Usage.Output, q.Took.Seconds(), clip(q.Err, 120))
		}
	}
	return b.String()
}

// clip shortens s to n bytes at a rune boundary.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8Start(s[n]) {
		n--
	}
	return s[:n] + "…"
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
