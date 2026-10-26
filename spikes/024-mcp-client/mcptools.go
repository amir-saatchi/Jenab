package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// An MCP server's tools as Jenab tools (SPEC 8.7), in the two styles the
// ticket compares:
//   - native: each MCP tool is a tool of the agent, named <server>_<tool>
//   - describe: mcp_describe and mcp_call, with the tool list in block 4
//
// Every call asks for an approval of kind "mcp" with the target
// <server>/<tool>; the app's orchestrator drops it once the project has an
// Always for that target (8.8). With the 8.7 rule on, a tool that isn't
// read-only asks again after the turn has read an MCP result, even with an
// Always.

type server struct {
	ID    string
	Title string // one line about the app
	cl    mcpClient
	tools map[string]toolInfo
	order []string
	run   map[string]tool.Tool // a checked tool per MCP tool, named <server>_<tool>
	st    *runState
}

// runState is what one run's adapter keeps.
type runState struct {
	mu        sync.Mutex
	rule87    bool // ask again for tools that aren't read-only after an MCP result
	saw       bool // the turn has read an MCP result
	n         int
	cards     map[string]card // by the card's Ask
	guard     func(srv, name string, args json.RawMessage) string
	mcpCalls  int // calls that reached the server
	mcpErrors int
}

// card is what the harness user needs to answer a card.
type card struct {
	Server, Tool string
	Args         json.RawMessage
	ReadOnly     bool
	AlwaysOK     bool
}

func newServer(srvID, title string, cl mcpClient, tools []toolInfo, offer func(toolInfo) bool, st *runState) (*server, error) {
	s := &server{ID: srvID, Title: title, cl: cl, tools: map[string]toolInfo{}, run: map[string]tool.Tool{}, st: st}
	for _, t := range tools {
		if offer != nil && !offer(t) {
			continue
		}
		s.tools[t.Name] = t
		s.order = append(s.order, t.Name)
		rt, err := s.checked(t)
		if err != nil {
			return nil, err
		}
		s.run[t.Name] = rt
	}
	return s, nil
}

// checked makes the tool that checks the arguments against the MCP
// schema (errors as in SPEC 10) and calls the server.
func (s *server) checked(t toolInfo) (rt tool.Tool, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("tool %s: %v", t.Name, r)
		}
	}()
	name := t.Name
	return tool.Func(tool.Spec{
		Name:        s.ID + "_" + name,
		Description: t.Description,
		Schema:      t.InputSchema,
		Effects:     tool.Untrusted | tool.NoUndo,
	}, func(ctx context.Context, env *tool.Env, args json.RawMessage) (tool.Result, error) {
		return s.call(ctx, env, name, args)
	}), nil
}

func (s *server) call(ctx context.Context, env *tool.Env, name string, args json.RawMessage) (tool.Result, error) {
	if s.st.guard != nil {
		if why := s.st.guard(s.ID, name, args); why != "" {
			return tool.Result{}, tool.Errorf("%s", why)
		}
	}
	if len(args) == 0 || string(args) == "null" {
		args = json.RawMessage(`{}`)
	}
	var progress progressFunc
	if env != nil && env.Progress != nil {
		progress = func(p, total float64, msg string) { env.Progress(msg) }
	}
	r, err := s.cl.Call(ctx, name, args, progress)
	s.st.mu.Lock()
	s.st.saw = true
	s.st.mcpCalls++
	if err != nil || r.IsError {
		s.st.mcpErrors++
	}
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
	res := tool.Result{Text: r.Text}
	for i, img := range r.Images {
		if env == nil || env.Project == nil {
			break
		}
		b, err := base64.StdEncoding.DecodeString(img.Data)
		if err != nil {
			continue
		}
		key := fmt.Sprintf("mcp/%s/%s-%s-%d.%s", s.ID, name, id.New(), i, ext(img.MIME))
		if _, err := env.Project.DB.PutObject(ctx, env.Priority, env.Source, key, strings.NewReader(string(b)), store.PutOptions{Source: "tool"}); err != nil {
			return res, err
		}
		res.Images = append(res.Images, chat.Image{Ref: key, MIME: img.MIME})
		res.Text += fmt.Sprintf("\n[image %s]", key)
	}
	if res.Text == "" {
		res.Text = "(no text)"
	}
	return res, nil
}

func ext(mime string) string {
	switch mime {
	case "image/png":
		return "png"
	case "image/webp":
		return "webp"
	}
	return "jpg"
}

// approval is the card for one call (8.8: server, tool and arguments).
func (s *server) approval(name string, args json.RawMessage, callID string) chat.Approval {
	t := s.tools[name]
	st := s.st
	st.mu.Lock()
	defer st.mu.Unlock()
	st.n++
	target := s.ID + "/" + name
	alwaysOK := true
	if st.rule87 && st.saw && !t.readOnly() {
		// 8.7: after an MCP result, a tool that isn't read-only asks
		// again; a target of its own keeps an Always from counting.
		target += "#" + callID
		alwaysOK = false
	}
	risk := "It only reads, by the server's own account."
	switch {
	case t.destructive():
		risk = "It can change or delete things in " + s.Title + ". Jenab can't undo it."
	case !t.readOnly():
		risk = "It changes things in " + s.Title + ". Jenab can't undo it."
	}
	ask := fmt.Sprintf("Run %s on %s? [#%d]", name, s.ID, st.n)
	if st.cards == nil {
		st.cards = map[string]card{}
	}
	st.cards[ask] = card{Server: s.ID, Tool: name, Args: args, ReadOnly: t.readOnly(), AlwaysOK: alwaysOK}
	opts := []chat.ApprovalOption{{Label: "Once", Grant: chat.GrantOnce}}
	if alwaysOK {
		opts = append(opts, chat.ApprovalOption{Label: "Always in this project", Grant: chat.GrantAlways})
	}
	opts = append(opts, chat.ApprovalOption{Label: "Deny", Grant: chat.GrantDeny})
	return chat.Approval{ID: id.Approval(id.New()), Kind: "mcp", Target: target, Ask: ask, Risk: risk,
		Details: string(args), Options: opts}
}

func (st *runState) card(ask string) (card, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	c, ok := st.cards[ask]
	return c, ok
}

// newTurn clears the mark at the user's message.
func (st *runState) newTurn() {
	st.mu.Lock()
	st.saw = false
	st.mu.Unlock()
}

// preflighted adds a Preflight to a tool.
type preflighted struct {
	tool.Tool
	pre func(ctx context.Context, call tool.Call) (tool.Needs, error)
}

func (p preflighted) Preflight(ctx context.Context, call tool.Call) (tool.Needs, error) {
	return p.pre(ctx, call)
}

// nativeTools: each offered MCP tool as a tool of the agent.
func (s *server) nativeTools() []tool.Tool {
	var out []tool.Tool
	for _, n := range s.order {
		name := n
		out = append(out, preflighted{Tool: s.run[name], pre: func(_ context.Context, call tool.Call) (tool.Needs, error) {
			return tool.Needs{Effects: tool.Untrusted | tool.NoUndo, Approvals: []chat.Approval{s.approval(name, call.Args, call.ID)}}, nil
		}})
	}
	return out
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
	if len(line) > 220 {
		line = clip(line, 217)
	}
	return line
}

// cardBlock is the server's part of block 4.
func (s *server) cardBlock(describe bool, instructions string) string {
	var b strings.Builder
	b.WriteString("MCP servers (tools of other apps the user connected):\n")
	if describe {
		fmt.Fprintf(&b, "- %s: %s. Call mcp_describe for a tool's arguments, then mcp_call. Its tools:\n", s.ID, s.Title)
		for _, n := range s.order {
			fmt.Fprintf(&b, "  - %s: %s\n", n, oneLine(s.tools[n].Description))
		}
	} else {
		fmt.Fprintf(&b, "- %s: %s. Its tools are named %s_<tool>.\n", s.ID, s.Title, s.ID)
	}
	if instructions != "" {
		fmt.Fprintf(&b, "\nThe %s server's instructions:\n%s\n", s.ID, strings.TrimSpace(instructions))
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

// describeTools: mcp_describe and mcp_call for the servers (SPEC 8.7).
func describeTools(servers []*server) []tool.Tool {
	ids := []string{}
	byID := map[string]*server{}
	for _, s := range servers {
		ids = append(ids, s.ID)
		byID[s.ID] = s
	}
	enum, _ := json.Marshal(ids)
	find := func(srv, name string) (*server, toolInfo, error) {
		s, ok := byID[srv]
		if !ok {
			return nil, toolInfo{}, tool.Errorf("no MCP server %q; servers: %s", srv, strings.Join(ids, ", "))
		}
		t, ok := s.tools[name]
		if !ok {
			return nil, toolInfo{}, tool.Errorf("%s has no tool %q; its tools: %s", srv, name, strings.Join(s.order, ", "))
		}
		return s, t, nil
	}
	describe := tool.Func(tool.Spec{
		Name:        "mcp_describe",
		Description: "Get an MCP tool's description and the JSON Schema of its arguments, before calling it with mcp_call.",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"server":{"type":"string","enum":` + string(enum) + `},
			"tool":{"type":"string","minLength":1}},
			"required":["server","tool"],"additionalProperties":false}`),
	}, func(_ context.Context, _ *tool.Env, a describeArgs) (tool.Result, error) {
		_, t, err := find(a.Server, a.Tool)
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
			"server":{"type":"string","enum":` + string(enum) + `},
			"tool":{"type":"string","minLength":1},
			"arguments":{"type":"object","description":"The tool's arguments."}},
			"required":["server","tool","arguments"],"additionalProperties":false}`),
		Effects: tool.Untrusted | tool.NoUndo,
	}, func(ctx context.Context, env *tool.Env, a callArgs) (tool.Result, error) {
		s, t, err := find(a.Server, a.Tool)
		if err != nil {
			return tool.Result{}, err
		}
		return s.run[t.Name].Run(ctx, tool.Call{Args: a.Arguments, Env: env})
	})
	return []tool.Tool{describe, preflighted{Tool: call, pre: func(_ context.Context, c tool.Call) (tool.Needs, error) {
		var a callArgs
		if err := json.Unmarshal(c.Args, &a); err != nil {
			return tool.Needs{}, nil // Run reports the bad arguments
		}
		s, _, err := find(a.Server, a.Tool)
		if err != nil {
			return tool.Needs{}, nil
		}
		return tool.Needs{Effects: tool.Untrusted | tool.NoUndo, Approvals: []chat.Approval{s.approval(a.Tool, a.Arguments, c.ID)}}, nil
	}}}
}

func compact(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// toolNames are a server's offered tools, sorted.
func (s *server) toolNames() []string { return slices.Sorted(slices.Values(s.order)) }
