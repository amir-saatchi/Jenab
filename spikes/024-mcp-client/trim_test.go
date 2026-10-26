package main

import (
	"strings"
	"testing"
)

// TestTrimViso: the committed bench row keeps scene entries and errors, but
// none of the server's own text. The texts here are made up.
func TestTrimViso(t *testing.T) {
	scene := "## Current Scene State: test\nSECRET-INTRO\n\n## Scene Index\n" +
		"- Box (box) pos [0, 0.5, 0] size [1, 1, 1]  id:b1\n    scale [1, 1, 1] · #ff0000 metal 0 rough 0.6\n\n" +
		"## Notes\nSECRET-NOTES\n"
	r := benchResult{runOut: runOut{Calls: []call{
		{Tool: "mcp_describe", Args: `{"server":"viso","tool":"add_object"}`, Result: "SECRET-SCHEMA"},
		{Tool: "mcp_call", Args: `{"server":"viso","tool":"get_scene","arguments":{}}`, Result: scene},
		{Tool: "viso_get_scene", Args: `{}`, Result: scene},
		{Tool: "viso_add_object", Args: `{"colour":"red"}`, Error: true, Result: "unknown argument colour"},
	}}}
	tr := trimViso(r)
	for i, c := range tr.Calls {
		if strings.Contains(c.Result, "SECRET") {
			t.Errorf("call %d keeps the server's text: %q", i, c.Result)
		}
	}
	for _, i := range []int{1, 2} {
		if !strings.Contains(tr.Calls[i].Result, "id:b1") || !strings.Contains(tr.Calls[i].Result, "#ff0000") {
			t.Errorf("call %d lost the scene entry: %q", i, tr.Calls[i].Result)
		}
	}
	if tr.Calls[3].Result != "unknown argument colour" {
		t.Errorf("the error changed: %q", tr.Calls[3].Result)
	}
	if !strings.Contains(r.Calls[0].Result, "SECRET") {
		t.Error("trimViso changed the full row")
	}
}
