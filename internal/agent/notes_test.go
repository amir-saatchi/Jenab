package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// note is an update_session_notes call.
func note(callID, content string, rev int) fake.Reply {
	return fake.ToolCall(callID, "update_session_notes", map[string]any{"content": content, "expected_revision": rev})
}

// Block 5 is the notes as they were at the last cut (3.1, 3.4): the
// chat's own edit is in its turn as the call, with no notice and no new
// block until the window is cut, by its size or an expired cache. It
// comes after the project card and before the line on earlier turns.
func TestNotesBlock(t *testing.T) {
	set := testSettings()
	set.Context.HistoryMaxTurns, set.Context.HistoryMinTurns = 2, 1
	h := newHarness(t, t.TempDir(), "", set, tool.UpdateSessionNotes())
	defer h.stop()
	h.o.d.Card = func(context.Context, *project.Project) (string, error) { return "the card", nil }
	c := h.newChat(chat.Chat{Title: "Plan"})

	h.fp.Push(note("c1", "Plan:\n1. import prices\n2. chart them", 0), fake.Text("planned"))
	h.turn(c.ID, "plan the work")
	for _, req := range h.fp.Calls() {
		if got := blockOf(req, notesBlockName); got != "" {
			t.Errorf("a block before the first cut after the edit: %q", got)
		}
	}
	h.fp.Push(fake.Text("ok"))
	h.turn(c.ID, "go on")
	req := lastCall(h)
	if got := blockOf(req, notesBlockName); got != "" {
		t.Errorf("the block changed between cuts: %q", got)
	}
	if got := userTexts(req); slices.ContainsFunc(got, func(s string) bool { return strings.HasPrefix(s, "[") }) {
		t.Errorf("a notice of the chat's own edit: %q", got)
	}

	// Turns 3 and 4: the window passes history_max_turns and is cut.
	h.fp.Push(note("c2", "Plan:\n1. import prices (done)\n2. chart them", 1), fake.Text("imported"))
	h.turn(c.ID, "import")
	h.fp.Push(fake.Text("charting"))
	h.turn(c.ID, "next step?")
	req = lastCall(h)
	want := "Session notes of this chat, revision 2:\n\n[notes]\nPlan:\n1. import prices (done)\n2. chart them"
	if got := blockOf(req, notesBlockName); got != want {
		t.Fatalf("block 5 after the cut:\n%q\nwant\n%q", got, want)
	}
	var names []string
	for _, b := range req.System {
		names = append(names, b.Name)
	}
	if want := []string{"System prompt, role and skills", cardBlock, notesBlockName, "Earlier turns"}; !slices.Equal(names, want) {
		t.Errorf("blocks %q, want %q", names, want)
	}
	cardOf(t, req) // checks the cache point

	// Clear empties the notes; the next turn has no block.
	if err := h.o.Clear(context.Background(), h.pid, c.ID); err != nil {
		t.Fatal(err)
	}
	h.fp.Push(fake.Text("hi"))
	h.turn(c.ID, "hello")
	if got := blockOf(lastCall(h), notesBlockName); got != "" {
		t.Errorf("block after Clear: %q", got)
	}

	// The summary, written by Refresh memory, joins the block at a cut.
	h.open(func(p *project.Project) {
		if _, err := p.Chats.SaveSummary(context.Background(), c.ID, "Imported prices.", 0); err != nil {
			t.Fatal(err)
		}
	})
	expire(h, c.ID)
	h.fp.Push(fake.Text("hi"))
	h.turn(c.ID, "again")
	if got := blockOf(lastCall(h), notesBlockName); got != "Session notes of this chat, revision 1:\n\n[summary, written by Refresh memory]\nImported prices." {
		t.Errorf("block with the summary: %q", got)
	}
}

// A subagent doesn't get update_session_notes, even when its tool list
// has it: like update_memory, it has the Memory effect (8.1).
func TestSubagentNoNotes(t *testing.T) {
	empty := json.RawMessage(`{"type": "object"}`)
	delegate := tool.Func(tool.Spec{Name: "delegate", Description: "Runs a helper.", Schema: empty},
		func(ctx context.Context, env *tool.Env, _ struct{}) (tool.Result, error) {
			res, err := env.Subagent(ctx, tool.Subagent{Name: "helper", System: "Help.", Task: "go", Tools: []tool.Tool{tool.UpdateSessionNotes(), echo()}})
			return tool.Result{Text: res.Text}, err
		})
	h := newHarness(t, t.TempDir(), "", testSettings(), delegate, tool.UpdateSessionNotes())
	defer h.stop()
	a := h.newChat(chat.Chat{Title: "A"})
	h.fp.Push(fake.ToolCall("c1", "delegate", struct{}{}), fake.Text("helped"), fake.Text("done"))
	h.turn(a.ID, "go")
	var helper, main bool
	for _, req := range h.fp.Calls() {
		has := slices.ContainsFunc(req.Tools, func(d provider.ToolDef) bool { return d.Name == "update_session_notes" })
		if len(req.System) > 0 && req.System[0].Name == "helper" {
			helper = true
			if has || len(req.Tools) != 1 {
				t.Errorf("subagent tools %+v", req.Tools)
			}
		} else if !main {
			main = true
			if !has {
				t.Error("the chat's agent lacks update_session_notes")
			}
		}
	}
	if !helper {
		t.Fatal("no subagent request")
	}
}
