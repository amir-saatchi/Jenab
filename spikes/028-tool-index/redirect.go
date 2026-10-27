package main

import (
	"fmt"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
)

// noTool starts the app's result for a call of a name the chat lacks.
const noTool = "there is no tool named "

// redirect points a native call of an index tool to call_tool. The app
// answers such a call with "there is no tool named X"; an index of
// outside tools (SPEC 8.7) would answer with the way to call it. The app
// has no hook for that result, so the spike rewrites it in each request it
// sends, the same way every time, so the cached prefix holds. The store
// and the transcript keep the app's text; the call counts as Native
// (metrics.go). Requests without call_tool, or names not in the index,
// pass as they are.
func redirect(req provider.Request) provider.Request {
	indexed := false
	for _, t := range req.Tools {
		indexed = indexed || t.Name == callName
	}
	listed := ""
	for _, b := range req.System {
		if i := strings.Index(b.Text, indexHead); i >= 0 {
			listed = b.Text[i:]
		}
	}
	if !indexed || listed == "" {
		return req
	}
	var out []chat.Message
	for i, m := range req.Messages {
		if m.Role != chat.RoleTool {
			continue
		}
		for j, p := range m.Parts {
			r := p.ToolResult
			if r == nil {
				continue
			}
			name, ok := strings.CutPrefix(r.Text, noTool)
			if !ok || !strings.Contains(listed, "\n- "+name+": ") {
				continue
			}
			if out == nil {
				out = append([]chat.Message(nil), req.Messages...)
			}
			if &out[i].Parts[0] == &req.Messages[i].Parts[0] {
				out[i].Parts = append([]chat.Part(nil), m.Parts...)
			}
			nr := *r
			nr.Text = redirectText(name)
			out[i].Parts[j].ToolResult = &nr
		}
	}
	if out != nil {
		req.Messages = out
	}
	return req
}

func redirectText(name string) string {
	return fmt.Sprintf("%s is in the More tools list, not in your tool list. Call it with call_tool(name: %q, arguments); describe_tool(%q) gives its arguments.", name, name, name)
}
