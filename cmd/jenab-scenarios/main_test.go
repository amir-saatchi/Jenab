package main

import (
	"bytes"
	"context"
	"encoding/json"
	"iter"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/scenario"
)

func TestCheckFindsTheRepoSets(t *testing.T) {
	root := filepath.Join("..", "..", "scenarios")
	var b strings.Builder
	if err := check([]string{root}, &b); err != nil {
		t.Fatal(err)
	}
	for _, set := range []string{"bench-bitcoin", "multi-part", "role-skills", "skill-loading"} {
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
		{scenario.Run{Seconds: 5, Requests: 3}, "pass in 5s, 3 requests"},
		{scenario.Run{Seconds: 5, Requests: 3, SubRequests: 2}, "pass in 5s, 3+2 requests"},
	} {
		if got := line(c.r); !strings.HasSuffix(got, c.want) {
			t.Errorf("%q, want suffix %q", got, c.want)
		}
	}
}

// scripted:<file> needs no models file and no keys, and runs alone.
func TestPickScripted(t *testing.T) {
	script := "scripted:" + filepath.Join("..", "..", "scenarios", "skill-loading", "_offline.yaml")
	none := filepath.Join(t.TempDir(), "models.yaml")
	models, s, err := pick([]string{script}, none, "")
	if err != nil || s == nil || s.Name != "offline" || len(models.Run) != 1 || models.Run[0] != "scripted/offline" {
		t.Fatalf("pick: %v %v %v", models, s, err)
	}
	if _, _, err := pick([]string{script, "gemini/x"}, none, ""); err == nil {
		t.Error("picked a script with another model")
	}
	if _, _, err := pick([]string{"gemini/x"}, none, ""); err == nil || !strings.Contains(err.Error(), "no models file") {
		t.Errorf("no models file: %v", err)
	}
}

// -json-mode off: the backend gets no schema, and isn't a JSONMode, so
// the Registry doesn't send twice for it. -schema-log: a line per request
// with a schema, none for one without.
func TestSchemaWatch(t *testing.T) {
	for _, strip := range []bool{false, true} {
		var got []provider.Request
		var log bytes.Buffer
		f := func(provider.Connection) (provider.Provider, error) { return &fake{seen: &got}, nil }
		all := schemaWatch(map[provider.Kind]provider.Factory{provider.KindOllama: f}, strip, &log)
		p, err := all[provider.KindOllama](provider.Connection{})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := p.(provider.JSONMode); ok {
			t.Error("the wrapper is a JSONMode")
		}
		msg := chat.Message{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "hi"}}}}
		for range p.Stream(context.Background(), provider.Request{Model: "m", Messages: []chat.Message{msg}, Schema: json.RawMessage(`{"type":"object","properties":{"items":{},"edits":{}}}`)}) {
		}
		for range p.Stream(context.Background(), provider.Request{Model: "m"}) {
		}
		if len(got) != 2 || (got[0].Schema == nil) != strip || got[0].Model != "m" {
			t.Errorf("strip %v: the backend got %+v", strip, got)
		}
		var c schemaCall
		if err := json.Unmarshal(log.Bytes(), &c); err != nil {
			t.Fatalf("strip %v: the log %q: %v", strip, log.String(), err)
		}
		if c.Schema != "edits,items" || c.Messages != 1 || c.Sent == strip || c.Answer != `{"items":[]}` || !c.Valid || c.Input == "" {
			t.Errorf("strip %v: the log line %+v", strip, c)
		}
	}
}

// -usage-log: a line per request with its tokens, totals that start again
// after a take, and the backend's JSON mode kept.
func TestUsageWatch(t *testing.T) {
	var got []provider.Request
	var log bytes.Buffer
	tally := &usageTally{log: &log}
	f := func(provider.Connection) (provider.Provider, error) { return &fake{seen: &got}, nil }
	all := usageWatch(map[provider.Kind]provider.Factory{provider.KindOllama: f}, tally)
	p, err := all[provider.KindOllama](provider.Connection{})
	if err != nil {
		t.Fatal(err)
	}
	if j, ok := p.(provider.JSONMode); !ok || !j.SendsSchema() {
		t.Error("the wrapper lost the backend's JSON mode")
	}
	// Two requests of one conversation drop the same block: one break. A
	// third, of another conversation, is a second; a fourth, of the first
	// with a new system block, a third.
	ask := func(text, sys string) provider.Request {
		return provider.Request{Model: "m", System: []provider.Block{{Text: sys}}, Messages: []chat.Message{{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: text}}}}}}
	}
	for _, req := range []provider.Request{ask("a", "s"), ask("a", "s"), ask("b", "s"), ask("a", "t")} {
		for range p.Stream(context.Background(), req) {
		}
	}
	lines := strings.Split(strings.TrimSpace(log.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("the log %q", log.String())
	}
	var c usageCall
	if err := json.Unmarshal([]byte(lines[0]), &c); err != nil {
		t.Fatal(err)
	}
	if c.Model != "m" || c.Input != 10 || c.CacheRead != 900 || c.CacheWrite != 90 || c.Output != 50 || c.Thought != 20 || c.Dropped != 3 || c.DropPath != "messages.1.content.0" || c.Conv == "" || c.Prefix == "" || c.Stop != "end" {
		t.Errorf("the log line %+v", c)
	}
	if s := tally.take(); s != "4 requests, input 40, cache read 3600, cache write 360, output 200 (thinking 80), thinking blocks dropped 12 in 4 requests, 3 breaks" {
		t.Errorf("take: %q", s)
	}
	if s := tally.take(); s != "0 requests, input 0, cache read 0, cache write 0, output 0" {
		t.Errorf("take after take: %q", s)
	}
}

type fake struct{ seen *[]provider.Request }

func (f *fake) SendsSchema() bool { return true }

func (f *fake) Stream(_ context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	*f.seen = append(*f.seen, req)
	return func(yield func(provider.Event, error) bool) {
		if len(req.Messages) > 0 {
			if !yield(provider.Event{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: `{"items":[]}`}}}, nil) {
				return
			}
		}
		yield(provider.Event{Kind: provider.EventDone, Stop: provider.StopEnd, Usage: &chat.Usage{Input: 10, CacheRead: 900, CacheWrite: 90, Output: 50, Thought: 20, ThinkingDropped: 3, ThinkingDropPath: "messages.1.content.0"}}, nil)
	}
}

func (f *fake) Models(context.Context) ([]provider.ModelInfo, error) { return nil, nil }

func TestContextSettings(t *testing.T) {
	set, err := contextSettings("history_max_turns=12, history_min_turns=4")
	if err != nil {
		t.Fatal(err)
	}
	c := config.Defaults()
	c.Context.HistoryMaxTokens = 9000
	set(&c)
	if c.Context.HistoryMaxTurns != 12 || c.Context.HistoryMinTurns != 4 || c.Context.HistoryMaxTokens != 9000 {
		t.Errorf("context %+v", c.Context)
	}
	if set, err := contextSettings(""); set != nil || err != nil {
		t.Errorf("empty: %v", err)
	}
	for _, bad := range []string{"history_max_turns", "history_max_turn=3", "history_max_turns=-1"} {
		if _, err := contextSettings(bad); err == nil {
			t.Errorf("took %q", bad)
		}
	}
}
