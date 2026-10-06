package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/scenario"
)

func TestCheckFindsTheRepoSets(t *testing.T) {
	root := filepath.Join("..", "..", "scenarios")
	var b strings.Builder
	if err := check([]string{root}, &b); err != nil {
		t.Fatal(err)
	}
	for _, set := range []string{"multi-part", "role-skills", "skill-loading"} {
		if !strings.Contains(b.String(), set) {
			t.Errorf("check missed %s:\n%s", set, b.String())
		}
	}
	one, err := sets([]string{filepath.Join(root, "role-skills")})
	if err != nil || len(one) != 1 {
		t.Errorf("one set: %v %v", one, err)
	}
	if _, err := sets([]string{t.TempDir()}); err == nil {
		t.Error("a folder without sets was accepted")
	}
}

func TestLine(t *testing.T) {
	for _, c := range []struct {
		r    scenario.Run
		want string
	}{
		{scenario.Run{Skipped: "needs background"}, "skip (needs background)"},
		{scenario.Run{Error: "p: 429\ntoo many", Seconds: 1.25}, "broke after 1.2s: p: 429 too many"},
		{scenario.Run{Seconds: 3, Asserts: []scenario.Result{{Type: "tool_called", Status: scenario.Fail}, {Type: "no_repeat", Test: "info", Status: scenario.Fail}}}, "fail in 3s: tool_called"},
		{scenario.Run{Seconds: 61}, "pass in 1m1s"},
	} {
		if got := line(c.r); !strings.HasSuffix(got, c.want) {
			t.Errorf("%q, want suffix %q", got, c.want)
		}
	}
}
