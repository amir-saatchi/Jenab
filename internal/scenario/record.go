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
	evUser     = "user"     // a user message's text
	evNotice   = "notice"   // a notice part, such as turn_failed
	evSkill    = "skill"    // the skill_loaded chip: a skill came into the chat
	evText     = "text"     // the agent's text
	evCall     = "call"     // a tool call
	evResult   = "result"   // a tool result
	evApproval = "approval" // an approval card, with its answer
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
	agent  string // a subagent's name; "" for the chat
	parent string // a subagent's event: the chat's call that ran it
}

// record is what a run did: the chat's parts and the requests' traces.
type record struct {
	events   []event
	requests []agent.RequestTrace
	reqTurn  []int // each request's turn
	checks   []groundCheck
	// sub are the subagents' parts, read from their transcripts (8.6), in
	// the order of the calls that ran them.
	sub []event
	// subRequests and subUsage are the subagents' requests.
	subRequests int
	subUsage    chat.Usage
	// db are the answers of the db_query checks, by query and table regex.
	db map[string][]dbAnswer
	sc *Scenario
	// grounding is the run's number check (SPIKE-032); nil is off.
	grounding *GroundingSpec
}

// groundCheck is a verdict of the number check, with its turn and the
// index of the checked answer's request in the record.
type groundCheck struct {
	turn, request int
	agent.GroundingCheck
}

// record reads the chat and its traces once the run is over.
func (r *run) record(ctx context.Context, traces *agent.Traces, sent []sent) (*record, error) {
	proj, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return nil, err
	}
	defer proj.Release()
	// The chats under test in order; a chat's turns count on from the last
	// one's (new_chat).
	var ms []chat.Message
	base := map[id.Chat]int{}
	for _, c := range r.chats {
		cms, _, err := proj.Chats.Messages(ctx, c, 1, 0)
		if err != nil {
			return nil, err
		}
		base[c] = turnsIn(ms)
		for _, m := range cms {
			m.Turn += base[c]
			ms = append(ms, m)
		}
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
				if strings.HasPrefix(p.ToolResult.Ref, subagentDir) {
					if err := rec.readSub(ctx, proj, m.Turn, e.callID, p.ToolResult.Ref); err != nil {
						return nil, err
					}
				}
			case p.Approval != nil:
				e.kind, e.text = evApproval, approvalText(p.Approval)
			case p.Notice != nil && p.Notice.Kind == chat.NoticeSkillText:
				continue // the skill's text, which the chip stands for
			case p.Notice != nil && p.Notice.Kind == chat.NoticeSkillLoaded:
				e.kind, e.text, e.skill = evSkill, p.Notice.Text, p.Notice.Skill
			case p.Notice != nil:
				e.kind, e.text, e.notice = evNotice, p.Notice.Text, p.Notice.Kind
			default:
				continue
			}
			rec.events = append(rec.events, e)
		}
	}
	if rec.db, err = r.dbQueries(ctx, proj); err != nil {
		return nil, err
	}
	list := traces.List()
	for _, t := range slices.Backward(list) {
		if b, ok := base[t.Chat]; ok {
			for _, c := range t.Checks {
				rec.checks = append(rec.checks, groundCheck{turn: b + t.Turn, request: len(rec.requests) + c.Request, GroundingCheck: c})
			}
			rec.requests = append(rec.requests, t.Requests...)
			for range t.Requests {
				rec.reqTurn = append(rec.reqTurn, b+t.Turn)
			}
		}
	}
	return rec, nil
}

// turnsIn is the last turn of ms.
func turnsIn(ms []chat.Message) int {
	n := 0
	for _, m := range ms {
		n = max(n, m.Turn)
	}
	return n
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

// usage is the requests' usage added up.
func (rec *record) usage() chat.Usage {
	var u chat.Usage
	for _, q := range rec.requests {
		u.Input += q.Usage.Input
		u.CacheRead += q.Usage.CacheRead
		u.CacheWrite += q.Usage.CacheWrite
		u.Output += q.Usage.Output
	}
	return u
}

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
			rec.subTranscript(&b, e.callID)
		case evApproval:
			fmt.Fprintf(&b, "\n> card %s\n", strings.ReplaceAll(clip(e.text, 1200), "\n", "\n> "))
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
	rec.groundTranscript(&b)
	if len(rec.db) > 0 {
		b.WriteString("\n## Database at the end\n")
		for _, a := range rec.sc.Asserts {
			if a.Type != "db_query" {
				continue
			}
			fmt.Fprintf(&b, "\n> %s\n", strings.ReplaceAll(strings.TrimSpace(a.Query), "\n", "\n> "))
			for _, d := range rec.db[dbKey(a)] {
				fmt.Fprintf(&b, "\n%s\n", d)
			}
		}
	}
	if rec.subRequests > 0 {
		u := rec.subUsage
		fmt.Fprintf(&b, "\nSubagent requests: %d (input %d, cache read %d, output %d)\n", rec.subRequests, u.Input, u.CacheRead, u.Output)
	}
	if len(rec.requests) > 0 {
		fmt.Fprintf(&b, "\n## Requests\n\n| # | input | cache read | cache write | output | stop | took | error |\n|---|---|---|---|---|---|---|---|\n")
		for i, q := range rec.requests {
			u := q.Usage
			fmt.Fprintf(&b, "| %d | %d | %d | %d | %d | %s | %.1f s | %s |\n", i+1, u.Input, u.CacheRead, u.CacheWrite, u.Output, q.Stop, q.Took.Seconds(), clip(q.Err, 120))
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
