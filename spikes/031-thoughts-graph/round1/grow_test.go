package main

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/secret"
)

type th = map[string]any

func TestGrow(t *testing.T) {
	logOut = io.Discard
	fp := fake.New(
		fake.ToolCall("c1", toolName, th{"thoughts": []th{{"kind": "problem", "title": "Fast report page", "content": "Goal: under 1 s.", "weight": 1, "rationale": "-", "status": "open"}}, "next": "#1"}),
		// Weight 2 is outside the schema: one retry with the error.
		fake.ToolCall("c2", toolName, th{"thoughts": []th{{"kind": "solution", "title": "Cache", "content": "x", "weight": 2, "rationale": "-", "status": "open"}}, "next": "#2"}),
		fake.ToolCall("c3", toolName, th{"thoughts": []th{
			{"kind": "solution", "title": "Cache query results", "content": "Redis, 5 min TTL.", "weight": 0.6, "rationale": "-", "status": "open"},
			{"kind": "solution", "title": "Fix N+1 queries", "content": "One join.", "weight": 0.9, "rationale": "-", "status": "open"},
			{"kind": "solution", "title": "Rewrite in Rust", "content": "No.", "weight": 0.1, "rationale": "-", "status": "dead_end"},
		}, "next": "#3"}),
		// Thoughts split over two calls, JSON-in-text isn't needed here.
		fake.Reply{Events: append(
			fake.ToolCall("c4", toolName, th{"thoughts": []th{{"kind": "step", "title": "Profile the slow queries", "content": "EXPLAIN ANALYZE.", "weight": 0.8, "rationale": "-", "status": "done"}}, "next": "#2"}).Events[:1],
			fake.ToolCall("c5", toolName, th{"thoughts": []th{{"kind": "merge", "title": "Fix queries, then cache", "content": "Both.", "weight": 0.95, "rationale": "-", "status": "done", "merge_with": []int{2}}},
				"reweight": []th{{"id": 2, "weight": 0.5}}, "next": "finish"}).Events...)},
		fake.Text("Fix the queries first, then cache.\nUsed: #3, #6"),
		fake.Text("Add an index."),
	)
	reg := provider.NewRegistry(provider.Deps{
		Settings: config.LLMSettings{MaxParallelCalls: 1, Providers: map[string]config.ProviderSettings{
			"fake": {Kind: "openai_compatible", BaseURL: "https://fake.invalid/v1/", Models: []config.ModelSettings{{ID: "m", Context: 32000}}},
		}},
		Secrets: secret.New(&keyring{m: map[string]string{provider.KeyName("fake"): "k-0123456789"}}),
		Gate:    limit.NewGate(1), Backends: map[provider.Kind]provider.Factory{provider.KindCompatible: fp.Factory()},
		Catalog: provider.MustCatalog(),
	})
	dir := t.TempDir()
	st, err := openStore(filepath.Join(dir, "g.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.db.Close()
	ctx := context.Background()
	g, err := grow(ctx, st, &client{reg: reg, model: "fake/m"}, problems[0], options{Mode: "siblings", K: 3, MaxThoughts: 16, MaxDepth: 4, Baseline: true})
	if err != nil {
		t.Fatal(err)
	}
	if g.Finish != "model" || len(g.nodes) != 6 || g.Answer != "Fix the queries first, then cache." || g.Used != "#3, #6" || g.Baseline != "Add an index." {
		t.Fatalf("finish %s, %d thoughts, answer %q, used %q, baseline %q", g.Finish, len(g.nodes), g.Answer, g.Used, g.Baseline)
	}
	want := `#1 [problem] Fast report page · expanded
  #2 [solution] Cache query results · w 0.50 · open
  #3 [solution] Fix N+1 queries · w 0.90 · expanded
    #5 [step] Profile the slow queries · w 0.80 · done
    #6 [merge] Fix queries, then cache · w 0.95 · done · also from #2
  #4 [solution] Rewrite in Rust · w 0.10 · dead end`
	if got := g.index(); got != want {
		t.Fatalf("index:\n%s\nwant:\n%s", got, want)
	}

	// The retry sent the schema error back as the tool result.
	calls := fp.Calls()
	if len(calls) != 6 {
		t.Fatalf("%d requests", len(calls))
	}
	last := calls[2].Messages[len(calls[2].Messages)-1]
	if last.Role != "tool" || !strings.Contains(last.Parts[0].ToolResult.Text, "/thoughts/0/weight") {
		t.Fatalf("retry message: %+v", last)
	}
	// The expand of #3 saw the index and #3's path in full.
	p := calls[3].Messages[0].Parts[0].Text.Text
	if !strings.Contains(p, "#4 [solution] Rewrite in Rust · w 0.10 · dead end") || !strings.Contains(p, "Path to #3, in full:") || !strings.Contains(p, "One join.") {
		t.Fatalf("expand prompt:\n%s", p)
	}

	var thoughts, edges, merges, reweights int
	db := st.db
	db.QueryRow(`SELECT COUNT(*) FROM thoughts WHERE graph_id = ?`, g.ID).Scan(&thoughts)
	db.QueryRow(`SELECT COUNT(*) FROM edges`).Scan(&edges)
	db.QueryRow(`SELECT COUNT(*) FROM edges WHERE kind = 'merges'`).Scan(&merges)
	db.QueryRow(`SELECT COUNT(*) FROM reweights`).Scan(&reweights)
	if thoughts != 6 || edges != 6 || merges != 2 || reweights != 1 {
		t.Fatalf("thoughts %d, edges %d, merges %d, reweights %d", thoughts, edges, merges, reweights)
	}
	s, err := statsOf(ctx, st, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s.Calls != 5 || s.FirstTry != 2 || s.Retried != 1 || s.Failures["schema"] != 1 || s.Depth != 2 {
		t.Fatalf("stats: %+v", s)
	}
	if _, err := writeGraph(ctx, st, g, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := writeSummary(ctx, st, dir); err != nil {
		t.Fatal(err)
	}
}

func TestJSONIn(t *testing.T) {
	got := jsonIn("Sure! {not json} here:\n```json\n{\"next\": \"#2\"}\n```")
	if string(got) != `{"next": "#2"}` {
		t.Fatalf("got %s", got)
	}
}

func TestUsedLine(t *testing.T) {
	a, u := usedLine("Do this.\n\n**Used: #2, #5**\n")
	if a != "Do this." || u != "#2, #5" {
		t.Fatalf("%q %q", a, u)
	}
}
