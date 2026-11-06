package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// Desktop control's hooks into the turn (SPEC 8.10), with stand-in tools:
// a switch that hides the tools, a hook at the turn's end, and older
// snapshots sent as stubs.

// switched is a tool behind a switch, for user turns; each call registers
// a turn-end hook and returns a snapshot of a series.
func switched(on *atomic.Bool, ended *atomic.Int32, turns *[]int) tool.Tool {
	n := 0
	return tool.Func(tool.Spec{Name: "look", Description: "Looks.", Schema: json.RawMessage(`{"type": "object"}`),
		On: on.Load, UserTurns: true, Effects: tool.Untrusted},
		func(_ context.Context, env *tool.Env, _ struct{}) (tool.Result, error) {
			n++
			*turns = append(*turns, env.Turn)
			env.AtTurnEnd(func() { ended.Add(1) })
			return tool.Result{Text: fmt.Sprintf("snapshot %d: a long tree", n), Whole: true, Series: "look",
				Stub: fmt.Sprintf("[snapshot %d — replaced]", n)}, nil
		})
}

func TestSwitchHidesTools(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var on atomic.Bool
		var ended atomic.Int32
		var turns []int
		h := newHarness(t, t.TempDir(), "", testSettings(), append(cardTools(), switched(&on, &ended, &turns))...)
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Desk"})

		// Off: not offered, and a call is refused.
		h.fp.Push(fake.ToolCall("c1", "look", struct{}{}), fake.Text("ok"))
		h.turn(c.ID, "look")
		if got := toolNames(h.fp.Calls()[0]); slices.Contains(got, "look") {
			t.Errorf("an off tool is offered: %q", got)
		}
		if tx := texts(h.messages(c.ID)); !slices.Contains(tx, "tool: result look is turned off in Settings") || len(turns) != 0 {
			t.Fatalf("an off tool ran: %q", tx)
		}

		// On: offered from the next request.
		on.Store(true)
		h.fp.Push(fake.ToolCall("c2", "look", struct{}{}), fake.ToolCall("c3", "look", struct{}{}), fake.Text("done"))
		h.turn(c.ID, "look twice")
		calls := h.fp.Calls()
		if got := toolNames(calls[2]); !slices.Contains(got, "look") {
			t.Errorf("an on tool isn't offered: %q", got)
		}
		if ended.Load() != 2 || !slices.Equal(turns, []int{2, 2}) {
			t.Fatalf("hooks ran %d times; turns %v", ended.Load(), turns)
		}

		// The last request has the first snapshot as its stub and the
		// second whole.
		var results []string
		for _, m := range calls[len(calls)-1].Messages {
			for _, p := range m.Parts {
				if r := p.ToolResult; r != nil && strings.HasPrefix(r.CallID, "c") && r.CallID != "c1" {
					results = append(results, r.Text)
				}
			}
		}
		if !slices.Equal(results, []string{"[snapshot 1 — replaced]", "snapshot 2: a long tree"}) {
			t.Fatalf("results sent: %q", results)
		}
		// The chat keeps both whole.
		if tx := texts(h.messages(c.ID)); !slices.Contains(tx, "tool: result snapshot 1: a long tree") {
			t.Errorf("the stub replaced the stored result: %q", tx)
		}
	})
}

// A subagent never gets a tool for user turns.
func TestSubagentHasNoUserTurnTools(t *testing.T) {
	var on atomic.Bool
	on.Store(true)
	var ended atomic.Int32
	var turns []int
	tl := switched(&on, &ended, &turns)
	if !tl.Spec().UserTurns {
		t.Fatal("the stand-in must be for user turns")
	}
	for _, name := range subagentToolNames(t, tl) {
		if name == "look" {
			t.Fatal("a subagent got a user-turn tool")
		}
	}
}

func subagentToolNames(t *testing.T, tools ...tool.Tool) []string {
	t.Helper()
	sr := &subRun{r: &runner{o: &Orchestrator{}}, t: &turn{}, s: tool.Subagent{Name: "helper", Tools: tools}, tools: map[string]tool.Tool{}}
	if err := sr.build(); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, d := range sr.defs {
		out = append(out, d.Name)
	}
	return out
}

// The approval rows (8.8): the window and the first key ask at every
// level, an action at Strict only, and no earlier answer counts.
func TestDesktopApprovalRows(t *testing.T) {
	for kind, want := range map[string][3]decision{
		"desktop_window": {ask, ask, ask},
		"desktop_key":    {ask, ask, ask},
		"desktop_action": {ask, noAsk, noAsk},
		"desktop_macro":  {ask, noAsk, noAsk},
	} {
		for i, l := range []project.Level{project.Strict, project.Standard, project.Auto} {
			for _, untrusted := range []bool{false, true} {
				if got := levelDecision(l, kind, untrusted); got != want[i] {
					t.Errorf("%s at %v (untrusted %v): %v, want %v", kind, l, untrusted, got, want[i])
				}
			}
		}
		if !fresh[kind] {
			t.Errorf("%s: an earlier answer would count", kind)
		}
	}
}

// A tool's check before the answer (8.11): the first answer without calls
// gets the reminder as a notice the model reads, and one more request;
// the check answers once, so the turn then ends. The tool sees the model.
func TestReminderBeforeAnswer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var model string
		var asked int
		act := tool.Func(tool.Spec{Name: "act", Description: "Acts.", Schema: json.RawMessage(`{"type": "object"}`), UserTurns: true},
			func(_ context.Context, env *tool.Env, _ struct{}) (tool.Result, error) {
				model = env.Model
				env.BeforeAnswer(func() string {
					asked++
					if asked == 1 {
						return "Before you answer: save it."
					}
					return ""
				})
				return tool.Result{Text: "done"}, nil
			})
		h := newHarness(t, t.TempDir(), "", testSettings(), append(cardTools(), act)...)
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Desk"})
		h.fp.Push(fake.ToolCall("c1", "act", struct{}{}), fake.Text("It is done."), fake.Text("Saved, and done."))
		h.turn(c.ID, "do it")
		tx := texts(h.messages(c.ID))
		i := slices.Index(tx, "user: Before you answer: save it.")
		if i < 0 || i == 0 || tx[i-1] != "assistant: It is done." || tx[len(tx)-1] != "assistant: Saved, and done." {
			t.Fatalf("messages: %q", tx)
		}
		calls := h.fp.Calls()
		if len(calls) != 3 || asked != 2 {
			t.Fatalf("%d requests, %d checks; want 3 and 2", len(calls), asked)
		}
		var sent bool
		for _, m := range calls[2].Messages {
			for _, p := range m.Parts {
				sent = sent || p.Text != nil && strings.Contains(p.Text.Text, "save it") || p.Notice != nil && strings.Contains(p.Notice.Text, "save it")
			}
		}
		if !sent {
			t.Fatal("the model didn't get the reminder")
		}
		if model == "" {
			t.Fatal("the tool didn't see the turn's model")
		}
		// The next turn starts without the check.
		h.fp.Push(fake.Text("Hi."))
		h.turn(c.ID, "hello")
		if asked != 2 {
			t.Fatalf("a check of the last turn ran again: %d", asked)
		}
	})
}
