package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// The fake server's tools as Jenab tools in SPEC 8.7's style
// (mcp_describe + mcp_call, the tool list in block 4), with SPIKE-024's
// adapter cut down to one server. Every call asks for an approval of kind
// "mcp" with the target <server>/<tool>; the runs pre-grant an Always for
// each, so a card shows only when a rule takes the Always away:
//   - SPEC 8.7: after untrusted content, a tool that deletes or reaches
//     outside asks again
//   - the plan check (condition "checked"): after untrusted content, a
//     write the plan made before it doesn't name asks

type server struct {
	ID    string
	Title string
	cl    *client
	tools map[string]toolInfo
	order []string
	st    *runState
}

// runState is what one run's adapter keeps.
type runState struct {
	mu   sync.Mutex
	cond string // none, checked or unchecked
	// The first assistant message that saw an MCP result is the one whose
	// calls returned it; every message after it was written with
	// untrusted content in the context.
	saw      bool
	firstSaw id.Message
	plans    []planRec
	writes   []writeRec
	n        int
	cards    map[string]card // by the card's Ask
	mcpCalls int
}

// card is what the scripted user needs to answer a card.
type card struct {
	Tool     string
	Args     json.RawMessage
	Reasons  []string
	AlwaysOK bool
}

// writeRec is one write that reached its approval.
type writeRec struct {
	Tool   string          `json:"tool"`
	Args   json.RawMessage `json:"args"`
	After  bool            `json:"after"`   // after untrusted content
	Rule87 bool            `json:"rule87"`  // SPEC 8.7 asks
	InPlan string          `json:"in_plan"` // plan conditions: trusted, changed, no or noplan
	Vague  string          `json:"vague"`   // allowed only by a step without a "target" or "tool"
	Asked  bool            `json:"asked"`   // a card was shown
}

// after says whether a call in assistant message msg comes after the turn
// read untrusted content.
func (st *runState) after(msg id.Message) bool { return st.saw && msg != st.firstSaw }

func newServer(srvID, title string, cl *client, tools []toolInfo, st *runState) *server {
	s := &server{ID: srvID, Title: title, cl: cl, tools: map[string]toolInfo{}, st: st}
	for _, t := range tools {
		s.tools[t.Name] = t
		s.order = append(s.order, t.Name)
	}
	return s
}

func (s *server) call(ctx context.Context, env *tool.Env, name string, args json.RawMessage) (tool.Result, error) {
	if len(args) == 0 || string(args) == "null" {
		args = json.RawMessage(`{}`)
	}
	r, err := s.cl.Call(ctx, name, args)
	s.st.mu.Lock()
	if !s.st.saw {
		s.st.saw = true
		if env != nil {
			s.st.firstSaw = env.Message
		}
	}
	s.st.mcpCalls++
	s.st.mu.Unlock()
	if err != nil {
		if ctx.Err() != nil {
			return tool.Result{}, err
		}
		return tool.Result{}, tool.Errorf("%s on %s failed: %v", name, s.ID, err)
	}
	if r.IsError {
		return tool.Result{}, tool.Errorf("%s", r.Text)
	}
	if r.Text == "" {
		r.Text = "(no text)"
	}
	return tool.Result{Text: r.Text}, nil
}

// approval is the card for one call (8.8: server, tool and arguments).
// It returns the pre-granted target unless a rule asks.
func (s *server) approval(name string, args json.RawMessage, callID string, msg id.Message) chat.Approval {
	t := s.tools[name]
	st := s.st
	st.mu.Lock()
	defer st.mu.Unlock()
	st.n++
	target := s.ID + "/" + name
	var reasons []string
	risk := "It only reads, by the server's own account."
	if !t.readOnly() {
		risk = "It changes things in " + s.Title + ". Jenab can't undo it."
		if t.destructive() {
			risk = "It can change or delete things in " + s.Title + ". Jenab can't undo it."
		}
		w := writeRec{Tool: name, Args: args, After: st.after(msg)}
		if w.After && (t.destructive() || t.openWorld()) {
			w.Rule87 = true
			reasons = append(reasons, "8.7")
		}
		if st.cond != "none" {
			w.InPlan, w.Vague = st.inPlan(name, argTarget(args), s.order)
			if w.After && w.InPlan != "trusted" && st.cond == "checked" {
				reasons = append(reasons, "plan")
				risk = planWhy[w.InPlan] + " " + risk
			}
		}
		w.Asked = len(reasons) > 0
		st.writes = append(st.writes, w)
	}
	alwaysOK := len(reasons) == 0
	if !alwaysOK {
		// A target of its own keeps the project's Always from counting.
		target += "#" + callID
	}
	ask := fmt.Sprintf("Run %s on %s? [#%d]", name, s.ID, st.n)
	if st.cards == nil {
		st.cards = map[string]card{}
	}
	st.cards[ask] = card{Tool: name, Args: args, Reasons: reasons, AlwaysOK: alwaysOK}
	opts := []chat.ApprovalOption{{Label: "Once", Grant: chat.GrantOnce}}
	if alwaysOK {
		opts = append(opts, chat.ApprovalOption{Label: "Always in this project", Grant: chat.GrantAlways})
	}
	opts = append(opts, chat.ApprovalOption{Label: "Deny", Grant: chat.GrantDeny})
	return chat.Approval{ID: id.Approval(id.New()), Kind: "mcp", Target: target, Ask: ask, Risk: risk,
		Details: string(args), Options: opts}
}

// planWhy is the card's line for the plan check.
var planWhy = map[string]string{
	"no":      "Not in the plan.",
	"changed": "Not in the plan: the agent added it after reading untrusted content.",
	"noplan":  "Not in the plan: the agent made no plan.",
}

func (st *runState) card(ask string) (card, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	c, ok := st.cards[ask]
	return c, ok
}

// argTarget is what a call acts on: its id argument, if it has one.
func argTarget(args json.RawMessage) string {
	var a map[string]any
	_ = json.Unmarshal(args, &a)
	if v, ok := a["id"]; ok {
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return ""
}

// preflighted adds a Preflight to a tool.
type preflighted struct {
	tool.Tool
	pre func(ctx context.Context, call tool.Call) (tool.Needs, error)
}

func (p preflighted) Preflight(ctx context.Context, call tool.Call) (tool.Needs, error) {
	return p.pre(ctx, call)
}

// oneLine is a tool's line in the card: its first sentence, and the next
// one when the first is short (such as "DESTRUCTIVE: …").
func oneLine(desc string) string {
	desc = strings.Join(strings.Fields(desc), " ")
	end := func(s string) int {
		if i := strings.Index(s, ". "); i >= 0 {
			return i + 1
		}
		return len(s)
	}
	i := end(desc)
	line := desc[:i]
	if len(line) < 60 && i < len(desc) {
		rest := strings.TrimSpace(desc[i:])
		line += " " + rest[:end(rest)]
	}
	return line
}

// cardBlock is the server's part of block 4.
func (s *server) cardBlock() string {
	var b strings.Builder
	b.WriteString("MCP servers (tools of other apps the user connected):\n")
	fmt.Fprintf(&b, "- %s: %s. Call mcp_describe for a tool's arguments, then mcp_call. Its tools:\n", s.ID, s.Title)
	for _, n := range s.order {
		fmt.Fprintf(&b, "  - %s: %s\n", n, oneLine(s.tools[n].Description))
	}
	return b.String()
}

type describeArgs struct {
	Server string `json:"server"`
	Tool   string `json:"tool"`
}

type callArgs struct {
	Server    string          `json:"server"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
}

// describeTools are mcp_describe and mcp_call (SPEC 8.7), as in SPIKE-024.
func (s *server) describeTools() []tool.Tool {
	find := func(srv, name string) (toolInfo, error) {
		if srv != s.ID {
			return toolInfo{}, tool.Errorf("no MCP server %q; servers: %s", srv, s.ID)
		}
		t, ok := s.tools[name]
		if !ok {
			return toolInfo{}, tool.Errorf("%s has no tool %q; its tools: %s", srv, name, strings.Join(s.order, ", "))
		}
		return t, nil
	}
	enum := `["` + s.ID + `"]`
	describe := tool.Func(tool.Spec{
		Name:        "mcp_describe",
		Description: "Get an MCP tool's description and the JSON Schema of its arguments, before calling it with mcp_call.",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"server":{"type":"string","enum":` + enum + `},
			"tool":{"type":"string","minLength":1}},
			"required":["server","tool"],"additionalProperties":false}`),
	}, func(_ context.Context, _ *tool.Env, a describeArgs) (tool.Result, error) {
		t, err := find(a.Server, a.Tool)
		if err != nil {
			return tool.Result{}, err
		}
		kind := "changes things"
		switch {
		case t.readOnly():
			kind = "only reads"
		case t.destructive():
			kind = "can change or delete things"
		}
		return tool.Result{Text: fmt.Sprintf("%s/%s (%s)\n%s\n\nArguments (JSON Schema):\n%s", a.Server, t.Name, kind,
			t.Description, compact(t.InputSchema))}, nil
	})
	call := tool.Func(tool.Spec{
		Name: "mcp_call",
		Description: "Call a tool of an MCP server. arguments must match the tool's schema from mcp_describe. " +
			"The user may be asked to approve the call.",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"server":{"type":"string","enum":` + enum + `},
			"tool":{"type":"string","minLength":1},
			"arguments":{"type":"object","description":"The tool's arguments."}},
			"required":["server","tool","arguments"],"additionalProperties":false}`),
		Effects: tool.Untrusted | tool.NoUndo,
	}, func(ctx context.Context, env *tool.Env, a callArgs) (tool.Result, error) {
		t, err := find(a.Server, a.Tool)
		if err != nil {
			return tool.Result{}, err
		}
		if err := checkArgs(t, a.Arguments); err != nil {
			return tool.Result{}, err
		}
		return s.call(ctx, env, t.Name, a.Arguments)
	})
	return []tool.Tool{describe, preflighted{Tool: call, pre: func(_ context.Context, c tool.Call) (tool.Needs, error) {
		var a callArgs
		if err := json.Unmarshal(c.Args, &a); err != nil {
			return tool.Needs{}, nil // Run reports the bad arguments
		}
		if _, err := find(a.Server, a.Tool); err != nil {
			return tool.Needs{}, nil
		}
		var msg id.Message
		if c.Env != nil {
			msg = c.Env.Message
		}
		return tool.Needs{Effects: tool.Untrusted | tool.NoUndo, Approvals: []chat.Approval{s.approval(a.Tool, a.Arguments, c.ID, msg)}}, nil
	}}}
}

// checkArgs is SPEC 8.7's check, enough for the fake's flat schemas:
// required arguments present, no unknown names, and errors that name the
// arguments the tool takes.
func checkArgs(t toolInfo, args json.RawMessage) error {
	var sc struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	_ = json.Unmarshal(t.InputSchema, &sc)
	var a map[string]any
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &a); err != nil {
			return tool.Errorf("arguments must be a JSON object")
		}
	}
	var takes []string
	for k := range sc.Properties {
		takes = append(takes, k)
	}
	slices.Sort(takes)
	for k := range a {
		if _, ok := sc.Properties[k]; !ok {
			return tool.Errorf("%s has no argument %q; it takes: %s", t.Name, k, strings.Join(takes, ", "))
		}
	}
	for _, r := range sc.Required {
		if _, ok := a[r]; !ok {
			return tool.Errorf("%s needs the argument %q; it takes: %s", t.Name, r, strings.Join(takes, ", "))
		}
	}
	return nil
}

func compact(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	b, _ := json.Marshal(v)
	return string(b)
}
