package provider

import "testing"

func TestToolCall(t *testing.T) {
	tests := []struct{ args, want, invalid string }{
		{"", "{}", ""},
		{"  ", "{}", ""},
		{`{"sql":"SELECT 1"}`, `{"sql":"SELECT 1"}`, ""},
		{`{"sql":`, "{}", `{"sql":`},
		{`{"a":1}{"b":2}`, "{}", `{"a":1}{"b":2}`},
	}
	for _, tt := range tests {
		c := ToolCall("c1", "query", tt.args, "")
		if string(c.Args) != tt.want || c.Invalid != tt.invalid || c.ID != "c1" || c.Name != "query" {
			t.Errorf("ToolCall(%q) = %+v, want Args %s, Invalid %q", tt.args, c, tt.want, tt.invalid)
		}
	}
}
