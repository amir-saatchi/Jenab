package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/secret"
)

type th = map[string]any

func js(v any) fake.Reply {
	b, _ := json.Marshal(v)
	return fake.Text(string(b))
}

func setup(t *testing.T, replies ...fake.Reply) (*fake.Provider, *client, *store, string) {
	t.Helper()
	logOut = io.Discard
	fp := fake.New(replies...)
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
	t.Cleanup(func() { st.db.Close() })
	return fp, &client{reg: reg, model: "fake/m"}, st, dir
}

func sol(title string, w float64, status string) th {
	return th{"kind": "solution", "title": title, "content": title + ".", "weight": w, "rationale": "-", "status": status}
}

func TestGrowIndex(t *testing.T) {
	fp, c, st, dir := setup(t,
		js(th{"thoughts": []th{{"kind": "problem", "title": "Fast report page", "content": "Goal: under 1 s.", "weight": 1, "rationale": "-", "status": "open"}}}),
		// Weight 2 is outside the schema: one retry with the error.
		js(th{"thoughts": []th{sol("Cache", 2, "open")}, "next": "#2"}),
		js(th{"thoughts": []th{sol("Cache query results", 0.6, "open"), sol("Fix N+1 queries", 0.9, "open"), sol("Rewrite in Rust", 0.1, "dead_end")}, "next": "#3"}),
		// JSON after a sentence, in a code fence.
		fake.Text("Here it is:\n```json\n"+string(must(json.Marshal(th{"thoughts": []th{
			{"kind": "step", "title": "Profile the slow queries", "content": "EXPLAIN ANALYZE.", "weight": 0.8, "rationale": "-", "status": "done"},
			{"kind": "merge", "title": "Fix queries, then cache", "content": "Both.", "weight": 0.95, "rationale": "-", "status": "done", "merge_with": []int{2}},
		}, "reweight": []th{{"id": 2, "weight": 0.5}}, "next": "finish"})))+"\n```"),
		fake.Text("Fix the queries first, then cache.\nUsed: #3, #6"),
	)
	ctx := context.Background()
	g, err := grow(ctx, st, c, problems[0], options{Mode: "index", K: 3, MaxThoughts: 16, MaxDepth: 4, MaxTokens: 4000})
	if err != nil {
		t.Fatal(err)
	}
	if g.Finish != "model" || len(g.nodes) != 6 || g.Answer != "Fix the queries first, then cache." || g.Used != "#3, #6" {
		t.Fatalf("finish %s, %d thoughts, answer %q, used %q", g.Finish, len(g.nodes), g.Answer, g.Used)
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

	calls := fp.Calls()
	if len(calls) != 5 {
		t.Fatalf("%d requests", len(calls))
	}
	// No tools; the schema is in the system prompt; the retry lists the error.
	if len(calls[1].Tools) != 0 || !strings.Contains(calls[1].System[0].Text, `"next"`) || strings.Contains(calls[0].System[0].Text, `"next"`) {
		t.Fatalf("system: %s", calls[1].System[0].Text)
	}
	last := calls[2].Messages[len(calls[2].Messages)-1]
	if last.Role != chat.RoleUser || !strings.Contains(last.Parts[0].Text.Text, "/thoughts/0/weight") {
		t.Fatalf("retry message: %+v", last)
	}
	// The expand of #3 saw the index and #3's path in full.
	p := calls[3].Messages[0].Parts[0].Text.Text
	if !strings.Contains(p, "#4 [solution] Rewrite in Rust · w 0.10 · dead end") || !strings.Contains(p, "Path to #3, in full:") || !strings.Contains(p, "Fix N+1 queries.") {
		t.Fatalf("expand prompt:\n%s", p)
	}

	var thoughts, edges, merges, reweights, answers int
	db := st.db
	db.QueryRow(`SELECT COUNT(*) FROM thoughts WHERE graph_id = ?`, g.ID).Scan(&thoughts)
	db.QueryRow(`SELECT COUNT(*) FROM edges`).Scan(&edges)
	db.QueryRow(`SELECT COUNT(*) FROM edges WHERE kind = 'merges'`).Scan(&merges)
	db.QueryRow(`SELECT COUNT(*) FROM reweights`).Scan(&reweights)
	db.QueryRow(`SELECT COUNT(*) FROM answers WHERE kind = 'index' AND graph_id = ?`, g.ID).Scan(&answers)
	if thoughts != 6 || edges != 6 || merges != 2 || reweights != 1 || answers != 1 {
		t.Fatalf("thoughts %d, edges %d, merges %d, reweights %d, answers %d", thoughts, edges, merges, reweights, answers)
	}
	s, err := statsOf(ctx, st, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s.Calls != 4 || s.FirstTry != 2 || s.Retried != 1 || s.Failures["schema"] != 1 || s.Depth != 2 {
		t.Fatalf("stats: %+v", s)
	}
	if _, err := writeGraph(ctx, st, g, dir); err != nil {
		t.Fatal(err)
	}
	if err := reports(ctx, st, dir); err != nil {
		t.Fatal(err)
	}
}

// Path mode: no index, no next; the controller expands the open solutions
// first, the highest weight first.
func TestGrowPath(t *testing.T) {
	step := func(title string) fake.Reply {
		return js(th{"thoughts": []th{{"kind": "step", "title": title, "content": title + ".", "weight": 0.7, "rationale": "-", "status": "open"}}})
	}
	fp, c, st, _ := setup(t,
		js(th{"thoughts": []th{{"kind": "problem", "title": "P", "content": "P.", "weight": 1, "rationale": "-", "status": "open"}}}),
		js(th{"thoughts": []th{sol("Low", 0.4, "open"), sol("High", 0.9, "open"), sol("Dead", 0.1, "dead_end")}}),
		step("Under high"),
		step("Under low"),
		fake.Text("Answer."),
	)
	g, err := grow(context.Background(), st, c, problems[0], options{Mode: "path", K: 3, MaxThoughts: 6, MaxDepth: 4, MaxTokens: 4000})
	if err != nil {
		t.Fatal(err)
	}
	if g.Finish != "budget" || len(g.nodes) != 6 {
		t.Fatalf("finish %s, %d thoughts", g.Finish, len(g.nodes))
	}
	calls := fp.Calls()
	for i, want := range []string{"Expand #1", "Expand #3", "Expand #2"} {
		p := calls[i+1].Messages[0].Parts[0].Text.Text
		if !strings.Contains(p, want) || strings.Contains(p, "Index of the graph") || strings.Contains(p, "In next") {
			t.Fatalf("call %d, want %s:\n%s", i+1, want, p)
		}
	}
	if strings.Contains(calls[1].System[0].Text, `"next"`) || strings.Contains(calls[1].System[0].Text, "index") {
		t.Fatalf("system: %s", calls[1].System[0].Text)
	}
}

// With fewer than 2 live solutions the controller goes back to #1.
func TestPickBreadth(t *testing.T) {
	g := &graph{MaxDepth: 4, nodes: []*thought{
		{Num: 1, Kind: "problem", State: "expanded"},
		{Num: 2, Kind: "solution", State: "expanded", Weight: 0.9, Depth: 1, Parents: []int{1}},
		{Num: 3, Kind: "step", State: "open", Weight: 0.8, Depth: 2, Parents: []int{2}},
	}}
	if n := g.pick(); n != 1 {
		t.Fatalf("pick %d", n)
	}
	g.nodes = append(g.nodes, &thought{Num: 4, Kind: "solution", State: "open", Weight: 0.3, Depth: 1, Parents: []int{1}})
	if n := g.pick(); n != 4 {
		t.Fatalf("pick %d", n)
	}
	g.nodes[3].State = "expanded"
	if n := g.pick(); n != 3 {
		t.Fatalf("pick %d", n)
	}
}

func TestBaselineAndJudge(t *testing.T) {
	thinking := fake.Reply{Events: []provider.Event{
		{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: "hmm"}}},
		{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: "Think, then index."}}},
		fake.Done(provider.StopEnd, chat.Usage{Input: 10, Output: 5}),
	}}
	verdicts := []fake.Reply{
		// index vs plain, both orders: index wins both.
		js(th{"reason": "r", "winner": "A", "score_a": 8, "score_b": 5}),
		// The winner's score is lower: one retry.
		js(th{"reason": "r", "winner": "A", "score_a": 4, "score_b": 7}),
		js(th{"reason": "r", "winner": "B", "score_a": 4, "score_b": 7}),
		// index vs thinking: the order changes the verdict.
		js(th{"reason": "r", "winner": "A", "score_a": 7, "score_b": 6}),
		js(th{"reason": "r", "winner": "A", "score_a": 7, "score_b": 6}),
	}
	fp, c, st, dir := setup(t, append([]fake.Reply{fake.Text("Add an index."), fake.Text("No thinking here."), thinking}, verdicts...)...)
	ctx := context.Background()
	p := problems[0]
	if a, err := baseline(ctx, st, c, p, false, 4000); err != nil || !a.Valid {
		t.Fatalf("plain: %+v %v", a, err)
	}
	// Thinking asked for, none came back: kept, not valid.
	a, err := baseline(ctx, st, c, p, true, 12000)
	if err != nil || a.Valid || a.Note == "" {
		t.Fatalf("thinking: %+v %v", a, err)
	}
	if !fp.Calls()[1].Thinking || fp.Calls()[0].Thinking {
		t.Fatal("thinking not sent as asked")
	}
	st.db.Exec(`DELETE FROM answers WHERE kind = 'thinking'`)
	if a, err := baseline(ctx, st, c, p, true, 12000); err != nil || !a.Valid {
		t.Fatalf("thinking: %+v %v", a, err)
	}
	if _, err := st.addAnswer(ctx, &answerRow{Problem: p.ID, Model: c.model, Kind: "index", Text: "From the graph.", Valid: true}); err != nil {
		t.Fatal(err)
	}

	if err := judge(ctx, st, c, 12000); err != nil {
		t.Fatal(err)
	}
	if n := len(fp.Calls()); n != 3+5 {
		t.Fatalf("%d calls", n)
	}
	if !fp.Calls()[3].Thinking || !strings.Contains(fp.Calls()[3].Messages[0].Parts[0].Text.Text, "=== Answer A ===\nFrom the graph.") {
		t.Fatalf("judge request: %+v", fp.Calls()[3])
	}
	outs, err := outcomes(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, o := range outs {
		got[o.X+" vs "+o.Y] = o.Result
	}
	if got["index vs plain"] != "x" || got["index vs thinking"] != "split" {
		t.Fatalf("outcomes: %v", got)
	}
	// A second run judges nothing again.
	if err := judge(ctx, st, c, 12000); err != nil || len(fp.Calls()) != 8 {
		t.Fatalf("rerun: %v, %d calls", err, len(fp.Calls()))
	}
	if err := reports(ctx, st, dir); err != nil {
		t.Fatal(err)
	}
	bdir, err := writeBlind(ctx, st, dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(bdir, "pair-01.md"))
	if !strings.Contains(string(b), "From the graph.") || !strings.Contains(string(b), "Think, then index.") || strings.Contains(string(b), "Add an index.") {
		t.Fatalf("blind pair:\n%s", b)
	}
}

// Some models think without sending thinking text; the thought tokens in
// the usage are enough.
func TestBaselineThoughtTokens(t *testing.T) {
	_, c, st, _ := setup(t, fake.Reply{Events: []provider.Event{
		{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: "Thought, silently."}}},
		fake.Done(provider.StopEnd, chat.Usage{Input: 10, Output: 900, Thought: 800}),
	}})
	a, err := baseline(context.Background(), st, c, problems[0], true, 12000)
	if err != nil || !a.Valid {
		t.Fatalf("%+v %v", a, err)
	}
	var thought int
	st.db.QueryRow(`SELECT thought_tokens FROM calls WHERE id = ?`, a.CallID).Scan(&thought)
	if thought != 800 {
		t.Fatalf("thought tokens %d", thought)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
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
