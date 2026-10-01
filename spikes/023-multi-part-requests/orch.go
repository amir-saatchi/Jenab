package main

// The orchestrator loop (SPEC 8.3 draft) on a virtual clock:
//  - A turn is a list of parts: text, tool calls, tool results, user messages, notices.
//  - Each model request costs stepSeconds of virtual time; each tool call costs its delay.
//  - Background tools (run_pipeline, subagent background:true, create_chat, send_to_chat) return
//    an ID at once and finish at start + delay. At most 3 run at once per chat.
//  - A finished background task starts a new system-started turn with a notice part. If a turn
//    is running, the notice waits until it ends. Notices that are due together go into one turn.
//    A task whose final status the agent already read with run_status/task_status gets no notice.
//  - A scripted user message with an `at` time that falls inside a turn is added at the next
//    step boundary: after the current tool calls (OpenAI message order does not allow a user
//    message between a tool call and its result), or, if the model response has no tool calls,
//    the turn ends and the message starts a new user turn.
//  - A message without `at` is sent when the chat is idle: no turn, no background work, no
//    pending notice.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
)

const (
	stepSeconds      = 4.0  // virtual time per model request
	maxStepsPerTurn  = 8    // model requests per turn
	maxTurns         = 8    // turns per scenario
	maxBg            = 3    // background tasks per chat (SPEC 8.3)
	userTurnMaxOut   = 8192 // max output tokens per request in a user turn
	systemTurnMaxOut = 4096 // lower cap for system-started turns (system_turn_max_tokens)
)

var clockBase = time.Date(2026, 9, 29, 12, 0, 0, 0, time.FixedZone("CEST", 2*3600))

func clock(t float64) string {
	return clockBase.Add(time.Duration(t * float64(time.Second))).Format("15:04:05")
}

// LLM is what the orchestrator calls; the real one wraps stream(), the tests use a script.
type LLM interface {
	Call(ctx context.Context, r Req) Result
}

type modelLLM struct{ m *Model }

func (l modelLLM) Call(ctx context.Context, r Req) Result { return stream(ctx, l.m, r) }

type Part struct {
	T      float64 `json:"t"`    // virtual seconds since the scenario start
	Kind   string  `json:"kind"` // user, mid_user, notice, text, call, result
	Text   string  `json:"text,omitempty"`
	Tool   string  `json:"tool,omitempty"`
	Args   string  `json:"args,omitempty"`
	CallID string  `json:"call_id,omitempty"`
	BgID   string  `json:"bg_id,omitempty"` // background task started by this call / reported by this notice
	Resp   int     `json:"resp,omitempty"`  // model response number (groups text and calls of one response)
	Msg    int     `json:"msg,omitempty"`   // index of the scripted user message (user, mid_user)
}

type ReqRec struct {
	Prompt, Output, Cached int
	Seconds                float64
	Finish                 string
	ReasoningChars         int      `json:",omitempty"`
	Err                    string   `json:",omitempty"`
	Notes                  []string `json:",omitempty"`
}

type Turn struct {
	N        int
	Kind     string // user or system
	Start    float64
	End      float64
	Parts    []Part
	Requests []ReqRec
	Stop     string // done, step cap, error, budget
}

type BgTask struct {
	ID, Tool, Args string
	Target         string `json:",omitempty"` // pipeline id or chat id
	Start, Finish  float64
	Result         string
	Delivered      string  `json:",omitempty"` // notice, poll
	DeliveredT     float64 `json:",omitempty"`
	StartTurn      int
}

type pendingMsg struct {
	idx  int
	text string
	at   float64 // -1: when idle
	done bool
	how  string // mid_turn, new_turn
}

type Orch struct {
	sc       *Scenario
	llm      LLM
	sys      string
	tools    []openai.ChatCompletionToolUnionParam
	db       *sql.DB
	now      float64
	turns    []*Turn
	tasks    []*BgTask
	msgs     []*pendingMsg
	resp     int
	nRun     int
	nTask    int
	nChat    int
	polls    int
	err      string
	label    string
	onCall   func(r Result) // token accounting
	newChats map[string]string
}

func newOrch(sc *Scenario, llm LLM, sys string, db *sql.DB) *Orch {
	o := &Orch{sc: sc, llm: llm, sys: sys, tools: toolSets[sc.Context], db: db, newChats: map[string]string{}}
	for i, m := range sc.Msgs {
		at := -1.0
		if i == 0 {
			at = 0
		}
		if m.At != "" {
			at = secs(m.At)
		}
		o.msgs = append(o.msgs, &pendingMsg{idx: i, text: m.Text, at: at})
	}
	return o
}

func (o *Orch) nextMsg() *pendingMsg {
	for _, m := range o.msgs {
		if !m.done {
			return m
		}
	}
	return nil
}

func (o *Orch) undelivered() []*BgTask {
	var out []*BgTask
	for _, t := range o.tasks {
		if t.Delivered == "" {
			out = append(out, t)
		}
	}
	return out
}

func (o *Orch) running() int {
	n := 0
	for _, t := range o.tasks {
		if t.Delivered == "" && o.now < t.Finish {
			n++
		}
	}
	return n
}

// Run plays the scenario until nothing is left to happen.
func (o *Orch) Run(ctx context.Context) {
	for len(o.turns) < maxTurns && o.err == "" {
		m := o.nextMsg()
		pend := o.undelivered()
		// earliest notice
		nt := -1.0
		for _, t := range pend {
			if nt < 0 || t.Finish < nt {
				nt = t.Finish
			}
		}
		mt := -1.0
		if m != nil {
			if m.at >= 0 {
				mt = m.at
			} else if len(pend) == 0 {
				mt = o.now
			}
		}
		switch {
		case mt >= 0 && (nt < 0 || mt <= nt):
			if mt > o.now {
				o.now = mt
			}
			m.done, m.how = true, "new_turn"
			kind := "user"
			pk := "user"
			o.turn(ctx, kind, []Part{{T: o.now, Kind: pk, Text: m.text, Msg: m.idx}})
		case nt >= 0:
			if nt > o.now {
				o.now = nt
			}
			var parts []Part
			for _, t := range pend {
				if t.Finish <= o.now {
					t.Delivered, t.DeliveredT = "notice", o.now
					parts = append(parts, Part{T: o.now, Kind: "notice", Text: o.notice(t), BgID: t.ID})
				}
			}
			o.turn(ctx, "system", parts)
		default:
			return
		}
	}
}

func (o *Orch) notice(t *BgTask) string {
	switch t.Tool {
	case "run_pipeline":
		return fmt.Sprintf("[run %s finished %s: %s: %s]", t.ID, clock(t.Finish), t.Target, t.Result)
	case "subagent":
		return fmt.Sprintf("[task %s finished %s: subagent result: %s]", t.ID, clock(t.Finish), t.Result)
	default:
		title := o.chatTitle(t.Target)
		return fmt.Sprintf("[task %s finished %s: reply from %s (%s): %s]", t.ID, clock(t.Finish), title, t.Target, t.Result)
	}
}

func (o *Orch) messages() []openai.ChatCompletionMessageParamUnion {
	msgs := []openai.ChatCompletionMessageParamUnion{openai.SystemMessage(o.sys)}
	for _, t := range o.turns {
		var notices []string
		flushNotices := func() {
			if len(notices) > 0 {
				msgs = append(msgs, openai.UserMessage(strings.Join(notices, "\n")))
				notices = nil
			}
		}
		for i := 0; i < len(t.Parts); i++ {
			p := t.Parts[i]
			switch p.Kind {
			case "notice":
				notices = append(notices, p.Text)
				continue
			case "user", "mid_user":
				flushNotices()
				msgs = append(msgs, openai.UserMessage(p.Text))
			case "text", "call":
				flushNotices()
				am := openai.ChatCompletionAssistantMessageParam{}
				text := ""
				j := i
				for ; j < len(t.Parts) && (t.Parts[j].Kind == "text" || t.Parts[j].Kind == "call") && t.Parts[j].Resp == p.Resp; j++ {
					q := t.Parts[j]
					if q.Kind == "text" {
						text += q.Text
					} else {
						args := q.Args
						if args == "" || !json.Valid([]byte(args)) {
							args = "{}"
						}
						am.ToolCalls = append(am.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: q.CallID, Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{Name: q.Tool, Arguments: args}}})
					}
				}
				i = j - 1
				if text == "" && len(am.ToolCalls) == 0 {
					continue // empty reply: not sent back
				}
				if text != "" {
					am.Content.OfString = openai.String(text)
				}
				msgs = append(msgs, openai.ChatCompletionMessageParamUnion{OfAssistant: &am})
			case "result":
				msgs = append(msgs, openai.ToolMessage(p.Text, p.CallID))
			}
		}
		flushNotices()
	}
	return msgs
}

func (o *Orch) turn(ctx context.Context, kind string, first []Part) {
	t := &Turn{N: len(o.turns) + 1, Kind: kind, Start: o.now, Parts: first}
	o.turns = append(o.turns, t)
	maxOut := userTurnMaxOut
	if kind == "system" {
		maxOut = systemTurnMaxOut
	}
	defer func() { t.End = o.now }()
	for step := 0; ; step++ {
		if step >= maxStepsPerTurn {
			t.Stop = "step cap"
			return
		}
		if overBudget() {
			t.Stop = "budget"
			o.err = "budget reached"
			return
		}
		msgs := o.messages()
		r := o.llm.Call(ctx, Req{Label: fmt.Sprintf("%s/t%d/s%d", o.label, t.N, step+1), Messages: msgs, Tools: o.tools, MaxTokens: maxOut})
		if o.onCall != nil {
			o.onCall(r)
		}
		rec := ReqRec{Prompt: r.Prompt, Output: r.Output, Cached: r.Cached, Seconds: r.Total.Seconds(), Finish: r.Finish, ReasoningChars: len(r.Reasoning), Notes: r.Notes}
		if r.Err != nil {
			rec.Err = short(r.ErrText, 600)
			t.Requests = append(t.Requests, rec)
			t.Stop = "error"
			o.err = rec.Err
			return
		}
		t.Requests = append(t.Requests, rec)
		o.now += stepSeconds
		o.resp++
		if strings.TrimSpace(r.Text) != "" || len(r.Calls) == 0 {
			t.Parts = append(t.Parts, Part{T: o.now, Kind: "text", Text: r.Text, Resp: o.resp})
		}
		for i, c := range r.Calls {
			id := c.ID
			if id == "" {
				id = fmt.Sprintf("call_%d_%d", o.resp, i)
			}
			t.Parts = append(t.Parts, Part{T: o.now, Kind: "call", Tool: c.Name, Args: c.Args, CallID: id, Resp: o.resp})
		}
		if len(r.Calls) == 0 {
			t.Stop = "done"
			return
		}
		for i := range t.Parts {
			p := &t.Parts[i]
			if p.Kind != "call" || p.Resp != o.resp {
				continue
			}
			res, delay, bg := o.exec(p.Tool, p.Args, t.N)
			o.now += delay
			if bg != nil {
				p.BgID = bg.ID
			}
			t.Parts = append(t.Parts, Part{T: o.now, Kind: "result", Tool: p.Tool, Text: res, CallID: p.CallID, BgID: p.BgID})
		}
		// step boundary: user messages that arrived meanwhile join this turn
		for _, m := range o.msgs {
			if !m.done && m.at >= 0 && m.at <= o.now && m == o.nextMsg() {
				m.done, m.how = true, "mid_turn"
				t.Parts = append(t.Parts, Part{T: o.now, Kind: "mid_user", Text: m.text, Msg: m.idx})
			}
		}
	}
}

func (o *Orch) exec(name, raw string, turnN int) (string, float64, *BgTask) {
	var a map[string]any
	if raw == "" {
		raw = "{}"
	}
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		return "error: tool arguments are not valid JSON", 0.2, nil
	}
	if a == nil {
		a = map[string]any{}
	}
	d := o.sc.delay(name)
	switch name {
	case "query":
		return runQuery(o.db, argStr(a, "sql")), d, nil
	case "describe_table":
		return describeTable(o.db, argStr(a, "name")), d, nil
	case "read_config":
		if c, ok := configs[argStr(a, "id")]; ok {
			return c, d, nil
		}
		return fmt.Sprintf("error: config %q not found (daily_prices, backfill_prices, fear_greed, price_chart, news_table)", argStr(a, "id")), d, nil
	case "call_api":
		return callAPI(a, raw), d, nil
	case "web_search", "fetch_page":
		if r := o.sc.rule(name, raw); r != nil {
			if r.Delay != "" {
				d = secs(r.Delay)
			}
			return r.Result, d, nil
		}
		return "no results", d, nil
	case "run_status", "task_status":
		o.polls++
		id := argStr(a, "run_id")
		if id == "" {
			id = argStr(a, "id")
		}
		for _, t := range o.tasks {
			if t.ID == id {
				if o.now+d >= t.Finish {
					if t.Delivered == "" {
						t.Delivered, t.DeliveredT = "poll", o.now+d
					}
					return fmt.Sprintf("finished %s: %s", clock(t.Finish), t.Result), d, nil
				}
				return fmt.Sprintf("running for %.0f s (typical length %.0f s)", o.now+d-t.Start, t.Finish-t.Start), d, nil
			}
		}
		return fmt.Sprintf("error: %q not found", id), d, nil
	case "list_chats":
		if o.sc.Context != "mother" {
			return "error: unknown tool", 0.2, nil
		}
		return chatListText, d, nil
	case "run_pipeline", "subagent", "create_chat", "send_to_chat":
		if (name == "create_chat" || name == "send_to_chat") && o.sc.Context != "mother" {
			return "error: only the Mother chat can use " + name, 0.2, nil
		}
		bg := isBgCall(name, a)
		var result, target string
		switch name {
		case "run_pipeline":
			target = argStr(a, "id")
			fin, pd, e := pipelineRun(target, argMap(a, "inputs"))
			if e != "" {
				return "error: " + e, 0.5, nil
			}
			result, d = fin, pd
			if dd, ok := o.sc.Delays["run_pipeline"]; ok {
				d = secs(dd)
			}
		case "send_to_chat":
			target = argStr(a, "chat_id")
			if o.chatTitle(target) == "" {
				return fmt.Sprintf("error: chat %q not found; use list_chats", target), 0.5, nil
			}
			if r := o.sc.rule(name, raw); r != nil {
				result = r.Result
				if r.Delay != "" {
					d = secs(r.Delay)
				}
			} else {
				result = "Done."
			}
		case "create_chat":
			if argStr(a, "title") == "" || argStr(a, "message") == "" {
				return "error: title and message are required", 0.5, nil
			}
			o.nChat++
			target = fmt.Sprintf("c_new%d", o.nChat)
			o.newChats[target] = argStr(a, "title")
			if r := o.sc.rule(name, raw); r != nil {
				result = r.Result
				if r.Delay != "" {
					d = secs(r.Delay)
				}
			} else {
				result = "Set up and ready."
			}
		case "subagent":
			if r := o.sc.rule(name, raw); r != nil {
				result = r.Result
				if r.Delay != "" {
					d = secs(r.Delay)
				}
			} else {
				result = "Done; nothing notable found."
			}
		}
		if !bg {
			return result, d, nil // foreground subagent: blocks for its whole length
		}
		if o.running() >= maxBg {
			return "error: 3 background tasks are already running in this chat; wait for one to finish", 0.2, nil
		}
		t := &BgTask{Tool: name, Args: raw, Target: target, Start: o.now, Finish: o.now + d, Result: result, StartTurn: turnN}
		if name == "run_pipeline" {
			o.nRun++
			t.ID = fmt.Sprintf("r_%d", o.nRun)
		} else {
			o.nTask++
			t.ID = fmt.Sprintf("t_%d", o.nTask)
		}
		o.tasks = append(o.tasks, t)
		ack := map[string]any{"status": "started"}
		switch name {
		case "run_pipeline":
			ack["run_id"] = t.ID
		case "create_chat":
			ack["chat_id"] = target
			ack["task_id"] = t.ID
		default:
			ack["task_id"] = t.ID
		}
		b, _ := json.Marshal(ack)
		return string(b), 0.5, t
	}
	return "error: unknown tool " + name, 0.2, nil
}

// chats of the Mother fixture (prompts/chats.md) plus the ones created in a run
var knownChats = map[string]string{"c_reviewer": "Reviewer", "c_eth": "ETH tracker", "c_setup": "Project setup"}

func (o *Orch) chatTitle(id string) string {
	if t, ok := knownChats[id]; ok {
		return t
	}
	return o.newChats[id]
}
