package provider

import (
	"regexp"
	"strings"
	"testing"
)

func TestToolCall(t *testing.T) {
	tests := []struct{ args, want, invalid string }{
		{"", "{}", ""},
		{"  ", "{}", ""},
		{`{"sql":"SELECT 1"}`, `{"sql":"SELECT 1"}`, ""},
		{`{"sql":`, "{}", `{"sql":`},
		{`{"a":1}{"b":2}`, "{}", `{"a":1}{"b":2}`},
		// Valid JSON that is not an object (TASK-007).
		{"null", "{}", "null"},
		{"[]", "{}", "[]"},
		{`"x"`, "{}", `"x"`},
		{" 7 ", "{}", " 7 "},
	}
	for _, tt := range tests {
		c := ToolCall("c1", "query", tt.args, "")
		if string(c.Args) != tt.want || c.Invalid != tt.invalid || c.ID != "c1" || c.Name != "query" {
			t.Errorf("ToolCall(%q) = %+v, want Args %s, Invalid %q", tt.args, c, tt.want, tt.invalid)
		}
	}
}

func TestNewCallIDIsNew(t *testing.T) {
	a, b := NewCallID(), NewCallID()
	if a == b || !strings.HasPrefix(a, "call_") {
		t.Errorf("NewCallID gave %q and %q", a, b)
	}
}

func TestCallIDRule(t *testing.T) {
	anthropic := CallIDRule{Allowed: IDChar}
	chat := CallIDRule{Max: 40}
	valid := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	long := "call_" + strings.Repeat("9", 60)

	// An ID that fits is unchanged.
	for _, id := range []string{"toolu_01A", "call_ab12", "c1"} {
		if got := anthropic.Map(id); got != id {
			t.Errorf("anthropic.Map(%q) = %q, want it unchanged", id, got)
		}
	}
	if got := chat.Map("functions.read_file:0"); got != "functions.read_file:0" {
		t.Errorf("chat.Map changed Kimi's ID to %q", got)
	}

	// Others map to a valid ID, the same every time, and different IDs
	// stay different.
	for _, id := range []string{"functions.read_file:0", "functions.read_file:1", long, ""} {
		got := anthropic.Map(id)
		if !valid.MatchString(got) || got != anthropic.Map(id) {
			t.Errorf("anthropic.Map(%q) = %q, want a stable valid ID", id, got)
		}
	}
	if anthropic.Map("functions.read_file:0") == anthropic.Map("functions.read_file:1") {
		t.Error("two IDs map to the same one")
	}
	if got := chat.Map(long); len(got) > 40 || got != chat.Map(long) {
		t.Errorf("chat.Map(long) = %q, want at most 40 characters", got)
	}
}

func TestObjectArgs(t *testing.T) {
	for in, want := range map[string]string{`{"a":1}`: `{"a":1}`, " {} ": " {} ", "null": "{}", "[]": "{}", `"x"`: "{}", "": "{}"} {
		if got := string(ObjectArgs([]byte(in))); got != want {
			t.Errorf("ObjectArgs(%q) = %q, want %q", in, got, want)
		}
	}
}
