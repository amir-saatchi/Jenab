package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/skill"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// The tool index (SPIKE-028): the core tools stay native with their full
// schemas; every other tool is a line in block 1, its schema comes from
// describe_tool as a tool result, and call_tool runs it. This is SPEC 8.7's
// mcp_describe / mcp_call (SPIKE-024) for Jenab's own tools.

// core are the tools that stay native: data and schema, memory,
// load_skill, and the history tools, which block 1's "Earlier turns" line
// names. apply_migration is a fake of the skill-loading set that stands for
// a schema change.
var core = []string{
	"query", "describe_table", "insert", "request_schema_change", "apply_migration",
	"update_memory", "update_session_notes", "load_skill",
	"search_history", "read_messages",
}

func isCore(name string) bool { return slices.Contains(core, name) }

// Names of the two tools of the index.
const (
	describeName = "describe_tool"
	callName     = "call_tool"
)

// indexHead starts the index in block 1. Its wording follows SPIKE-024's
// card block and is the same for every model (SPEC 1).
const indexHead = "## More tools\n" +
	"These tools are not in your tool list. To use one, call describe_tool(name) for its arguments, then call_tool(name, arguments). " +
	"Each line is a tool's name and what it does:"

// index is a run's tools with the index: the registry the orchestrator
// gets, and the block 1 text for each kind of chat.
type index struct {
	all    *tool.Registry // every tool, as condition 1 has them
	skills *skill.Set
}

// newIndex wraps a run's registry: the core tools and describe_tool and
// call_tool are native, the rest are in the index.
func newIndex(all *tool.Registry, skills *skill.Set) (*tool.Registry, func(chat.Kind) string) {
	x := &index{all: all, skills: skills}
	reg := tool.NewRegistry()
	var timeout time.Duration
	var bg bool
	var areas tool.Areas
	for _, t := range all.For(chat.KindMother) { // every tool, Mother's too
		s := t.Spec()
		if slices.Contains(core, s.Name) {
			reg.Add(t)
			continue
		}
		timeout = max(timeout, s.Timeout)
		bg = bg || s.Background
		areas |= s.Areas
	}
	if timeout == 0 {
		timeout = tool.DefaultTimeout
	}
	reg.Add(x.describe(areas), &preflighted{Tool: x.call(timeout, bg), pre: x.preflight})
	return reg, x.text
}

// rare are the tools in the index for a chat of kind k, by name.
func (x *index) rare(k chat.Kind) []tool.Tool {
	var out []tool.Tool
	for _, t := range x.all.For(k) {
		if !slices.Contains(core, t.Spec().Name) {
			out = append(out, t)
		}
	}
	return out
}

func (x *index) names(k chat.Kind) []string {
	var out []string
	for _, t := range x.rare(k) {
		out = append(out, t.Spec().Name)
	}
	return out
}

// text is the index in block 1.
func (x *index) text(k chat.Kind) string {
	ts := x.rare(k)
	if len(ts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(indexHead)
	for _, t := range ts {
		fmt.Fprintf(&b, "\n- %s: %s", t.Spec().Name, oneLine(t.Spec().Description))
	}
	return b.String()
}

// find is the tool a describe_tool or call_tool names, for a chat of kind
// k. A core tool is found too: it runs as natively.
func (x *index) find(k chat.Kind, name string) (tool.Tool, error) {
	t, ok := x.all.Get(name)
	if !ok || t.Spec().Mother && k != chat.KindMother {
		return nil, tool.Errorf("there is no tool named %q. The tools in the index: %s", name, strings.Join(x.names(k), ", "))
	}
	return t, nil
}

type describeArgs struct {
	Name string `json:"name"`
}

func (x *index) describe(areas tool.Areas) tool.Tool {
	return tool.Func(tool.Spec{
		Name:        describeName,
		Description: "Get a tool's description and the JSON Schema of its arguments, for a tool in the More tools list. Then call it with call_tool.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","minLength":1,"description":"The tool's name."}},"required":["name"],"additionalProperties":false}`),
		Areas:       areas, // so block 1 names the same parts of a project as with every tool native
	}, func(ctx context.Context, env *tool.Env, a describeArgs) (tool.Result, error) {
		k := kindOf(ctx, env)
		t, err := x.find(k, a.Name)
		if err != nil {
			return tool.Result{}, err
		}
		s := t.Spec()
		return tool.Result{Text: fmt.Sprintf("%s\n%s\n\nArguments (JSON Schema):\n%s", s.Name, strings.TrimSpace(s.Description), compact(s.Schema))}, nil
	})
}

type callArgs struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (x *index) call(timeout time.Duration, bg bool) tool.Tool {
	return tool.Func(tool.Spec{
		Name: callName,
		Description: "Call a tool from the More tools list. arguments must match the tool's schema from describe_tool. " +
			"The user may be asked to approve the call.",
		Schema: json.RawMessage(`{"type":"object","properties":{
			"name":{"type":"string","minLength":1,"description":"The tool's name."},
			"arguments":{"type":"object","description":"The tool's arguments."}},
			"required":["name","arguments"],"additionalProperties":false}`),
		Effects:    tool.ReadsDB,
		Timeout:    timeout,
		Background: bg, // so block 1 has the prompt rule as with every tool native
	}, func(ctx context.Context, env *tool.Env, a callArgs) (tool.Result, error) {
		k := kindOf(ctx, env)
		t, err := x.find(k, a.Name)
		if err != nil {
			return tool.Result{}, err
		}
		if err := checkNames(t.Spec(), a.Arguments); err != nil {
			return tool.Result{}, err
		}
		args := a.Arguments
		if len(args) == 0 || string(args) == "null" {
			args = json.RawMessage(`{}`)
		}
		res, err := tool.Run(ctx, t, tool.Call{Args: args, Env: env})
		if err == nil {
			x.loadWith(ctx, env, k, a.Name)
		}
		return res, err
	})
}

// loadWith loads the skills whose load_with names the tool, as the
// orchestrator does after a native call (8.9): it sees only call_tool.
func (x *index) loadWith(ctx context.Context, env *tool.Env, k chat.Kind, name string) {
	if x.skills == nil || env == nil || env.Skill == nil {
		return
	}
	for _, s := range x.skills.With(k, name) {
		env.Skill(ctx, s.Name, "") // a loaded skill answers "already loaded", which is dropped
	}
}

// keepSkills is the skill set the orchestrator gets with the index. It
// hides a skill whose load_with or needs tools are all missing from its
// registry (Skills.Usable), and the index's tools are not in it. So each
// skill condition 1 would keep loses the names of the index's tools from
// load_with and needs: it stays listed and loadable, and the index loads
// it with its tool (loadWith), from the full set.
func keepSkills(s *skill.Set, all, native *tool.Registry) *skill.Set {
	if s == nil {
		return nil
	}
	isNative := func(n string) bool { _, ok := native.Get(n); return ok }
	usable := s.Usable(func(n string) bool { _, ok := all.Get(n); return ok })
	var out []skill.Skill
	for _, sk := range usable.For(chat.KindMother) { // every skill, Mother's too
		sk.LoadWith = slices.DeleteFunc(slices.Clone(sk.LoadWith), func(n string) bool { return !isNative(n) })
		sk.Needs = slices.DeleteFunc(slices.Clone(sk.Needs), func(n string) bool { return !isNative(n) })
		out = append(out, sk)
	}
	kept, err := skill.New(out...)
	if err != nil {
		return s
	}
	return kept
}

// preflight gives call_tool the approvals and effects of the tool it
// runs, so a call through the index asks as a native call does (8.8).
func (x *index) preflight(ctx context.Context, c tool.Call) (tool.Needs, error) {
	var a callArgs
	if json.Unmarshal(c.Args, &a) != nil {
		return tool.Needs{}, nil // Run reports the bad arguments
	}
	t, err := x.find(kindOf(ctx, c.Env), a.Name)
	if err != nil {
		return tool.Needs{}, nil
	}
	if pf, ok := t.(tool.Preflighter); ok {
		return pf.Preflight(ctx, tool.Call{ID: c.ID, Args: a.Arguments, Env: c.Env})
	}
	return tool.Needs{Effects: t.Spec().Effects}, nil
}

// checkNames refuses argument names the schema doesn't list and names
// that are missing, and names the arguments the tool takes (8.7).
// Types and values are left to the tool's own check.
func checkNames(s tool.Spec, raw json.RawMessage) error {
	var sch struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
		Additional json.RawMessage            `json:"additionalProperties"`
	}
	if json.Unmarshal(s.Schema, &sch) != nil {
		return nil
	}
	var got map[string]json.RawMessage
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &got); err != nil {
			return &tool.Error{Msg: "arguments must be a JSON object. " + takes(s.Name, sch.Properties, sch.Required), Err: tool.ErrArgs}
		}
	}
	var unknown, missing []string
	for n := range got {
		// Other names are refused only where the tool's own schema refuses
		// them, as in a native call.
		if _, ok := sch.Properties[n]; !ok && sch.Properties != nil && string(sch.Additional) == "false" {
			unknown = append(unknown, n)
		}
	}
	for _, n := range sch.Required {
		if _, ok := got[n]; !ok {
			missing = append(missing, n)
		}
	}
	if len(unknown) == 0 && len(missing) == 0 {
		return nil
	}
	slices.Sort(unknown)
	var why []string
	if len(unknown) > 0 {
		why = append(why, fmt.Sprintf("%s takes no argument %s.", s.Name, strings.Join(unknown, ", ")))
	}
	if len(missing) > 0 {
		why = append(why, fmt.Sprintf("%s needs %s.", s.Name, strings.Join(missing, ", ")))
	}
	return &tool.Error{Msg: "wrong arguments: " + strings.Join(why, " ") + " " + takes(s.Name, sch.Properties, sch.Required), Err: tool.ErrArgs}
}

func takes(name string, props map[string]json.RawMessage, req []string) string {
	var ns []string
	for n := range props {
		if slices.Contains(req, n) {
			ns = append(ns, n+" (required)")
		} else {
			ns = append(ns, n)
		}
	}
	slices.Sort(ns)
	if len(ns) == 0 {
		return name + " takes no arguments."
	}
	return "Its arguments: " + strings.Join(ns, ", ") + ". describe_tool gives their schema."
}

// kindOf is the chat's kind; a lookup that fails is a plain chat.
func kindOf(ctx context.Context, env *tool.Env) chat.Kind {
	if env == nil || env.Project == nil {
		return chat.KindChat
	}
	c, err := env.Project.Chats.Chat(ctx, env.Chat)
	if err != nil {
		return chat.KindChat
	}
	return c.Kind
}

// preflighted is a tool with a Preflight.
type preflighted struct {
	tool.Tool
	pre func(context.Context, tool.Call) (tool.Needs, error)
}

func (p *preflighted) Preflight(ctx context.Context, c tool.Call) (tool.Needs, error) {
	return p.pre(ctx, c)
}

// unwrap is the call the scenario checks read: call_tool(X, a) is a call
// of X with a (scenario.Options.Call).
func unwrap(name, args string) (string, string) {
	if name != callName {
		return name, args
	}
	var a callArgs
	if json.Unmarshal([]byte(args), &a) != nil || a.Name == "" {
		return name, args
	}
	if len(a.Arguments) == 0 {
		return a.Name, "{}"
	}
	return a.Name, string(a.Arguments)
}

// oneLine is a description's first sentence, clipped (SPIKE-024).
func oneLine(desc string) string {
	desc = strings.Join(strings.Fields(desc), " ")
	end := func(s string) int {
		for i := 0; i+1 < len(s); i++ {
			if s[i] == '.' && s[i+1] == ' ' {
				return i + 1
			}
		}
		return len(s)
	}
	return clip(desc[:end(desc)], 160)
}

func compact(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	b, _ := json.Marshal(v)
	return string(b)
}
