package main

// Part B's conditions: earlier turns as answers only, or answers with a
// trace, given to the orchestrator through Deps.Earlier. Today's window is
// the hook left nil.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
)

// earlierTurns sets what a request keeps of earlier turns.
type earlierTurns struct {
	// trace starts each answer with a trace line per tool, written by Go.
	trace bool
	// line gives a call's trace entry: its main argument and what it
	// made, such as a ref or a task ID. Nil, or "" for main, takes the
	// call's first string argument; "" for made takes the result's ref.
	line func(c chat.ToolCall, r *chat.ToolResult) (main, made string)
}

// earlier is ms with the turns before cur as answers only: the user's
// messages as they are, and the turn's answer, its last text part, in
// place of everything the agent did. Thinking, tool calls, results and
// text between calls go together, so every provider accepts the history.
// With trace, the answer starts with a line per tool. The current turn
// stays as it is. Nothing is changed in the store.
func (et earlierTurns) earlier(ms []chat.Message, cur int) []chat.Message {
	out := make([]chat.Message, 0, len(ms))
	for i := 0; i < len(ms); {
		n := ms[i].Turn
		if n >= cur {
			out = append(out, ms[i])
			i++
			continue
		}
		j := i
		for j < len(ms) && ms[j].Turn == n {
			j++
		}
		out = append(out, et.answerOnly(ms[i:j])...)
		i = j
	}
	return out
}

// answerOnly is one earlier turn as answers only. The answer goes where
// the message that holds it was, so a notice after it stays after it.
func (et earlierTurns) answerOnly(turn []chat.Message) []chat.Message {
	at, text := -1, ""
	for i, m := range turn {
		if m.Role != chat.RoleAssistant {
			continue
		}
		for _, p := range m.Parts {
			if p.Text != nil && strings.TrimSpace(p.Text.Text) != "" {
				at, text = i, p.Text.Text
			}
		}
	}
	trace := ""
	if et.trace {
		trace = traceLines(turn, et.line)
	}
	if trace != "" && at < 0 {
		// No answer: the trace goes after the last of the agent's messages.
		for i, m := range turn {
			if m.Role != chat.RoleUser {
				at = i
			}
		}
	}
	var out []chat.Message
	for i, m := range turn {
		if m.Role == chat.RoleUser {
			out = append(out, m)
		}
		if i != at {
			continue
		}
		s := strings.TrimSpace(strings.Join([]string{trace, text}, "\n\n"))
		a := chat.Message{ID: m.ID, Chat: m.Chat, Turn: m.Turn, Role: chat.RoleAssistant, Model: m.Model, CreatedAt: m.CreatedAt,
			Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: s}}}}
		out = append(out, a)
	}
	return out
}

// traceLines is a turn's trace: one line per tool, its calls merged, in
// the order each tool was first called, then where the full steps are.
//
//	[fetch_page jobs.example.com/123 → ref cache/pages/jobs.example.com/ab12.txt]
//	[create_table applications]
//	[subagent "search LinkedIn" → task t_4]
//	[full steps: read_messages(from: 12, to: 12)]
func traceLines(turn []chat.Message, line func(chat.ToolCall, *chat.ToolResult) (string, string)) string {
	results := map[string]*chat.ToolResult{}
	for _, m := range turn {
		for _, p := range m.Parts {
			if p.ToolResult != nil {
				results[p.ToolResult.CallID] = p.ToolResult
			}
		}
	}
	type entry struct{ main, made string }
	var order []string
	calls := map[string][]entry{}
	for _, m := range turn {
		if m.Role != chat.RoleAssistant {
			continue
		}
		for _, p := range m.Parts {
			c := p.ToolCall
			if c == nil {
				continue
			}
			r := results[c.ID]
			var e entry
			if line != nil {
				e.main, e.made = line(*c, r)
			}
			if e.main == "" {
				e.main = firstString(c.Args)
			}
			if e.made == "" && r != nil && r.Ref != "" {
				e.made = "ref " + r.Ref
			}
			if r != nil && r.IsError {
				e.made = "failed"
			}
			if _, ok := calls[c.Name]; !ok {
				order = append(order, c.Name)
			}
			calls[c.Name] = append(calls[c.Name], e)
		}
	}
	if len(order) == 0 {
		return ""
	}
	var b strings.Builder
	for _, name := range order {
		es := calls[name]
		b.WriteString("[" + name)
		if len(es) > 1 {
			fmt.Fprintf(&b, " ×%d:", len(es))
		}
		for i, e := range es {
			if i > 0 {
				b.WriteString(";")
			}
			if e.main != "" {
				b.WriteString(" " + e.main)
			}
			if e.made != "" {
				b.WriteString(" → " + e.made)
			}
		}
		b.WriteString("]\n")
	}
	n := turn[0].Turn
	fmt.Fprintf(&b, "[full steps: read_messages(from: %d, to: %d)]", n, n)
	return b.String()
}

// traceArgMax bounds a main argument in a trace line.
const traceArgMax = 80

// firstString is the first string value of a call's arguments, in their
// order, quoted when it has spaces, and shortened.
func firstString(args json.RawMessage) string {
	d := json.NewDecoder(bytes.NewReader(args))
	if t, err := d.Token(); err != nil || t != json.Delim('{') {
		return ""
	}
	for d.More() {
		if _, err := d.Token(); err != nil { // the key
			return ""
		}
		var v any
		if err := d.Decode(&v); err != nil {
			return ""
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return traceArg(s)
		}
	}
	return ""
}

// traceArg is an argument as a trace line shows it: on one line, at most
// traceArgMax characters, quoted when it has spaces.
func traceArg(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > traceArgMax {
		s = string(r[:traceArgMax-1]) + "…"
	}
	if strings.Contains(s, " ") {
		return `"` + s + `"`
	}
	return s
}
