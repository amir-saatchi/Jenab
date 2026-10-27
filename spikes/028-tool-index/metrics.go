package main

import (
	"encoding/json"
	"slices"
	"strings"
)

// calls are a run's tool calls as its transcript shows them, the same way
// for every condition: a native call is "> call X {args}", one through the
// index "> call call_tool→X {args}", and a result "> result X (error):".
type calls struct {
	Rare      int `json:"rare"`       // calls of tools outside the core, natively or through call_tool
	Indexed   int `json:"indexed"`    // of those, through call_tool
	Describes int `json:"describes"`  // describe_tool calls
	ArgErrors int `json:"arg_errors"` // rare calls refused for their arguments
	Unknown   int `json:"unknown"`    // calls of a name the chat doesn't have: Native, Nested and others
	Native    int `json:"native"`     // of those, a tool of the index called natively (redirected to call_tool)
	Nested    int `json:"nested"`     // of those, call_tool naming call_tool or describe_tool
	Blind     int `json:"blind"`      // call_tool of a tool not described before in the run
	Failed    int `json:"failed"`     // distinct tools whose call failed for arguments or a name
	Recovered int `json:"recovered"`  // of those, a later call of the same tool worked
	History   int `json:"history"`    // search_history and read_messages calls
}

type callLine struct {
	tool, via string
	args      string // as the transcript shows it, clipped
	err       string // "", "args", "unknown", "other"
}

// parseCalls reads the chat's calls and results from a transcript, in
// order. A result belongs to the oldest open call of its tool.
func parseCalls(tr string) []callLine {
	var out []callLine
	open := map[string][]int{}
	lines := strings.Split(tr, "\n")
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "> call "):
			rest := strings.TrimPrefix(l, "> call ")
			name, args, _ := strings.Cut(rest, " ")
			via := ""
			if v, n, ok := strings.Cut(name, "→"); ok {
				via, name = v, n
			}
			open[name] = append(open[name], len(out))
			out = append(out, callLine{tool: name, via: via, args: args})
		case strings.HasPrefix(l, "> result "):
			rest := strings.TrimSuffix(strings.TrimPrefix(l, "> result "), ":")
			name, mark, _ := strings.Cut(rest, " ")
			q := open[name]
			if len(q) == 0 {
				continue
			}
			j := q[0]
			open[name] = q[1:]
			if mark != "(error)" {
				continue
			}
			text := ""
			if i+1 < len(lines) {
				text = strings.ToLower(lines[i+1])
			}
			switch {
			case strings.Contains(text, "there is no tool named"):
				out[j].err = "unknown"
			case strings.Contains(text, "wrong arguments") || strings.Contains(text, "arguments must be") ||
				strings.Contains(text, "not valid json"):
				out[j].err = "args"
			default:
				out[j].err = "other"
			}
		}
	}
	return out
}

// countCalls sums a transcript's calls.
func countCalls(tr string) calls {
	var c calls
	described := map[string]bool{}
	failed := map[string]bool{}
	recovered := map[string]bool{}
	for _, l := range parseCalls(tr) {
		switch {
		case l.tool == describeName:
			c.Describes++
			var a describeArgs
			if json.Unmarshal([]byte(l.args), &a) == nil {
				described[a.Name] = true
			}
			continue
		case l.tool == "search_history" || l.tool == "read_messages":
			c.History++
		}
		if l.via == callName && l.err != "unknown" && !described[l.tool] {
			c.Blind++
		}
		if l.err == "unknown" {
			c.Unknown++
			switch {
			case l.tool == callName || l.tool == describeName:
				c.Nested++
			case l.via == "" && !isCore(l.tool):
				c.Native++
			}
		}
		if !slices.Contains(core, l.tool) && l.tool != callName && l.err != "unknown" {
			c.Rare++
			if l.via != "" {
				c.Indexed++
			}
			if l.err == "args" {
				c.ArgErrors++
			}
		}
		switch {
		case l.err == "args" || l.err == "unknown":
			failed[l.tool] = true
		case l.err == "" && failed[l.tool]:
			recovered[l.tool] = true
		}
	}
	c.Failed, c.Recovered = len(failed), len(recovered)
	return c
}
