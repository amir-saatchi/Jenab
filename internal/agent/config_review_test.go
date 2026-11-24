package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// saveTool saves the view close_now.
func saveTool() tool.Tool {
	return tool.Func(tool.Spec{Name: "save", Description: "Saves a view.", Schema: json.RawMessage(`{"type": "object"}`)},
		func(ctx context.Context, env *tool.Env, _ struct{}) (tool.Result, error) {
			_, err := env.Project.DB.SaveConfig(ctx, store.ConfigSave{Kind: store.KindView, ID: "close_now", Type: "stat", Title: "Close",
				YAML: "version: 1\nid: close_now\n", Source: env.Source})
			return tool.Result{Text: "saved"}, err
		})
}

// A config change reaches the card at a turn's first request, also from
// another chat; within the turn it waits, but a schema change brings the
// card at once, with the config too (P3-03, 3.1).
func TestCardConfigTiming(t *testing.T) {
	h := newHarness(t, t.TempDir(), "", testSettings(), append(cardTools(), saveTool())...)
	defer h.stop()
	h.o.d.Card = realCard
	a := h.newChat(chat.Chat{Title: "A"})
	b := h.newChat(chat.Chat{Title: "B"})
	h.fp.Push(
		fake.Text("hi"),                            // A 1
		fake.ToolCall("c1", "save", struct{}{}),    // B 1
		fake.Text("saved"),                         // B 1
		fake.Text("hi again"),                      // A 2
		fake.ToolCall("c2", "migrate", struct{}{}), // B 2
		fake.Text("made"),                          // B 2
	)
	h.turn(a.ID, "hello")
	h.turn(b.ID, "save a view")
	h.turn(a.ID, "hello again")
	calls := h.fp.Calls()
	if len(calls) != 4 {
		t.Fatalf("%d requests, want 4", len(calls))
	}
	// B's save, in the turn: no note at its next request.
	if notes := cardNotes(calls[2]); len(notes) != 0 {
		t.Errorf("mid-turn config change: %q", notes)
	}
	// A's next turn, first request: the note, with the view.
	if notes := cardNotes(calls[3]); len(notes) != 1 || !strings.Contains(notes[0], "- close_now (stat)") {
		t.Errorf("A's next turn: %q", notes)
	}

	// B's next turn starts with the note of its own save; the migration
	// in the turn brings another at once.
	h.turn(b.ID, "make a table")
	calls = h.fp.Calls()[4:]
	if notes := cardNotes(calls[0]); len(notes) != 1 || !strings.Contains(notes[0], "close_now") {
		t.Fatalf("B's next turn, first request: %q", notes)
	}
	if notes := cardNotes(calls[1]); len(notes) != 2 || !strings.Contains(notes[1], "- prices (0 rows)") || !strings.Contains(notes[1], "close_now") {
		t.Errorf("after the migration: %q", notes)
	}
}

// A subagent run is a turn of its own for its tools: Turn and a Run of
// its own, its answer checks before it answers, its end functions when
// it ends (P3-03: save_view's repair limit in subagents).
func TestSubagentTurnHooks(t *testing.T) {
	var (
		runs   []string
		turns  []int
		ended  int
		asked  int
		before = 0
	)
	probe := tool.Func(tool.Spec{Name: "probe", Description: "Probes.", Schema: json.RawMessage(`{"type": "object"}`)},
		func(_ context.Context, env *tool.Env, _ struct{}) (tool.Result, error) {
			runs, turns = append(runs, env.Run), append(turns, env.Turn)
			if env.AtTurnEnd == nil || env.BeforeAnswer == nil {
				t.Error("no turn hooks in a subagent")
				return tool.Result{}, nil
			}
			env.AtTurnEnd(func() { ended++ })
			env.BeforeAnswer(chat.NoticeEarlyStop, func() string {
				if asked++; asked > 1 {
					return ""
				}
				before = ended
				return "save it again"
			})
			return tool.Result{Text: "probed"}, nil
		})
	delegate := tool.Func(tool.Spec{Name: "delegate", Description: "Runs a helper.", Schema: json.RawMessage(`{"type": "object"}`)},
		func(ctx context.Context, env *tool.Env, _ struct{}) (tool.Result, error) {
			res, err := env.Subagent(ctx, tool.Subagent{Name: "helper", System: "Help.", Task: "go", Tools: []tool.Tool{probe}})
			return tool.Result{Text: res.Text}, err
		})
	h := newHarness(t, t.TempDir(), "", testSettings(), delegate)
	defer h.stop()
	c := h.newChat(chat.Chat{Title: "A"})
	h.fp.Push(
		fake.ToolCall("c1", "delegate", struct{}{}), // the chat
		fake.ToolCall("s1", "probe", struct{}{}),    // the helper
		fake.Text("done"),                           // the helper, reminded
		fake.Text("saved"),                          // the helper
		fake.ToolCall("c2", "delegate", struct{}{}), // the chat
		fake.ToolCall("s2", "probe", struct{}{}),    // a second helper
		fake.Text("done"),                           // the helper, no reminder: once
		fake.Text("ok"),                             // the chat
	)
	h.turn(c.ID, "go")
	calls := h.fp.Calls()
	if len(calls) != 8 {
		t.Fatalf("%d requests, want 8", len(calls))
	}
	// The helper's third request has the reminder, as a notice at the end.
	last := calls[3].Messages[len(calls[3].Messages)-1]
	if n := last.Parts[0].Notice; last.Role != chat.RoleUser || n == nil || n.Kind != chat.NoticeEarlyStop || n.Text != "save it again" {
		t.Errorf("reminder: %+v", last)
	}
	if before != 0 || ended != 2 {
		t.Errorf("ended %d before the reminder, %d in all; want 0 and 2", before, ended)
	}
	if len(runs) != 2 || runs[0] == "" || runs[0] == runs[1] || !strings.HasPrefix(runs[0], "helper:") || turns[0] != 1 || turns[1] != 1 {
		t.Errorf("runs %q, turns %v", runs, turns)
	}
	// The chat's own turn didn't get the helper's check.
	for _, m := range calls[7].Messages {
		for _, p := range m.Parts {
			if p.Notice != nil && p.Notice.Kind == chat.NoticeEarlyStop {
				t.Errorf("the chat got the helper's notice")
			}
		}
	}
}
