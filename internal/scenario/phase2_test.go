package scenario

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// requestText is everything a request sends as text: the system blocks,
// the messages, the tool results and the notices.
func requestText(r provider.Request) string {
	var b strings.Builder
	for _, s := range r.System {
		b.WriteString(s.Text + "\n")
	}
	b.WriteString(textOfAll(r.Messages))
	for _, m := range r.Messages {
		for _, p := range m.Parts {
			if p.Notice != nil {
				b.WriteString(p.Notice.Text + "\n")
			}
		}
	}
	return b.String()
}

// TestPhase2Acceptance runs P2-14's scenario on its scripted model: the
// schema made and widened in one chat through the schema agent, rows by
// hand, the goal in project memory, a migration card the runner approves,
// a restart, a new chat that answers from query, the card and memory, and
// Undo turn on one of two turns of inserts. The set's checks read the
// database at the end; this test also reads what the model was sent.
func TestPhase2Acceptance(t *testing.T) {
	dir := filepath.Join("..", "..", "scenarios", "phase2-acceptance")
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	sc, err := ReadScript(filepath.Join(dir, "_offline.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	const id = "P2_schema_memory"
	fp := fake.New(sc.replies(id)...)
	rep, err := RunSet(context.Background(), s, Options{
		Models: ScriptedModels(sc), Backends: map[provider.Kind]provider.Factory{scriptedKind: fp.Factory()},
		Scale: 0.01, Timeout: 2 * time.Minute, Temp: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	r := rep.Runs[0]
	if r.Error != "" {
		t.Fatalf("error: %s", r.Error)
	}
	for _, a := range r.Asserts {
		if a.Status != Pass {
			t.Errorf("%s (%s): %s, %s", a.Type, a.Test, a.Status, a.Why)
		}
	}
	if r.Turns != 9 {
		t.Errorf("%d turns, want 9", r.Turns)
	}
	// Turn 7 is the second chat's turn 4.
	if !strings.Contains(r.Transcript, "Undo turn 7: done, 3 rows in prices") {
		t.Errorf("the undo isn't in the transcript:\n%s", r.Transcript)
	}

	calls := fp.Calls()
	asking := func(s string) provider.Request {
		t.Helper()
		for _, c := range calls {
			if len(c.Messages) > 0 && strings.Contains(textOf(c.Messages[len(c.Messages)-1]), s) {
				return c
			}
		}
		t.Fatalf("no request asks %q", s)
		return provider.Request{}
	}

	// The new chat after the restart starts with no history: memory holds
	// the goal and the card the widened table, and nothing of the first
	// chat's turns is in it.
	first := requestText(asking("Bitcoin close on 2026-11-12"))
	for _, want := range []string{"every Monday beats buying only after a 5 % drop", "prices"} {
		if !strings.Contains(first, want) {
			t.Errorf("turn 4's request lacks %q:\n%s", want, first)
		}
	}
	for _, old := range []string{"Here are the first closes", "Also track Ethereum", "btc_prices"} {
		if strings.Contains(first, old) {
			t.Errorf("turn 4's request has %q from the first chat:\n%s", old, first)
		}
	}
	if !strings.Contains(requestText(asking("Remind me, what is the goal")), "every Monday") {
		t.Error("turn 6's request has no goal in memory")
	}
	// After the undo the chat is told what was undone.
	if after := requestText(asking("Which Ethereum days")); !strings.Contains(after, "was undone: 3 rows in prices") {
		t.Errorf("turn 9's request has no undo notice:\n%s", after)
	}
}

// An undo message on a turn that changed nothing is recorded, not a run
// error; the run goes on.
func TestUndoNothing(t *testing.T) {
	dir := writeSet(t, map[string]string{
		"_set.yaml": "title: P\n",
		"a.yaml":    "id: a\ncontext: main\nmessages: [{text: hi}, {undo: 1}, {text: ho}]\n",
	})
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	sc := &Script{Name: "s", Scenarios: map[string][]ScriptReply{"a": {{Text: "hello"}, {Text: "bye"}}}}
	rep, err := RunSet(context.Background(), s, Options{Models: ScriptedModels(sc), Script: sc, Scale: 0.01, Timeout: time.Minute, Temp: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	r := rep.Runs[0]
	if r.Error != "" || r.Turns != 2 {
		t.Fatalf("error %q, %d turns", r.Error, r.Turns)
	}
	if !strings.Contains(r.Transcript, "Undo turn 1: nothing to undo") {
		t.Errorf("transcript:\n%s", r.Transcript)
	}
}
