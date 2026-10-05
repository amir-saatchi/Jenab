package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
	"github.com/amir-saatchi/jenab/internal/web"
)

// needy is a test tool that needs an approval of its kind for its target
// argument, like fetch_page for a host. runs counts its runs.
type needy struct {
	tool.Tool
	runs atomic.Int32
}

type needyArgs struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

func newNeedy() *needy {
	n := &needy{}
	n.Tool = tool.Func(tool.Spec{Name: "needy", Description: "Needs an approval.",
		Schema: json.RawMessage(`{"type": "object", "properties": {"kind": {"type": "string"}, "target": {"type": "string"}}}`)},
		func(_ context.Context, _ *tool.Env, a needyArgs) (tool.Result, error) {
			n.runs.Add(1)
			return tool.Result{Text: "ran for " + a.Target}, nil
		})
	return n
}

func (n *needy) Preflight(_ context.Context, call tool.Call) (tool.Needs, error) {
	var a needyArgs
	if err := json.Unmarshal(call.Args, &a); err != nil {
		return tool.Needs{}, err
	}
	if a.Target == "" {
		return tool.Needs{}, tool.Errorf("needy needs a target")
	}
	card := tool.HostApproval(a.Target, "needy")
	if a.Kind != "" {
		card.Kind = a.Kind
	}
	if a.Kind == "starlark" { // a card with Once
		card.Options = []chat.ApprovalOption{{Label: "Approve", Grant: chat.GrantOnce}, {Label: "Deny", Grant: chat.GrantDeny}}
	}
	return tool.Needs{Effects: tool.Network, Approvals: []chat.Approval{card}}, nil
}

// outside returns outside data, like a web page.
func outside() tool.Tool {
	return tool.Func(tool.Spec{Name: "outside", Description: "Reads a page.", Effects: tool.Untrusted,
		Schema: json.RawMessage(`{"type": "object"}`)},
		func(context.Context, *tool.Env, struct{}) (tool.Result, error) {
			return tool.Result{Text: "page text"}, nil
		})
}

func needs(kind, target string) map[string]string {
	return map[string]string{"kind": kind, "target": target}
}

// waiting waits until the chat's turn waits for a card or form, and
// returns it.
func (h *harness) waiting(c id.Chat) chat.Waiting {
	h.t.Helper()
	synctest.Wait()
	l := h.o.Live(h.pid, c)
	if l.Waiting == nil {
		h.t.Fatalf("the chat doesn't wait: %+v", l)
	}
	return *l.Waiting
}

func (h *harness) answer(c id.Chat, a Answer) {
	h.t.Helper()
	if err := h.o.Answer(h.pid, c, a); err != nil {
		h.t.Fatal(err)
	}
}

// cards are the chat's approval cards.
func cards(ms []chat.Message) []chat.Approval {
	var out []chat.Approval
	for _, m := range ms {
		for _, p := range m.Parts {
			if p.Approval != nil {
				out = append(out, *p.Approval)
			}
		}
	}
	return out
}

func (h *harness) approvals() []store.Approval {
	h.t.Helper()
	var as []store.Approval
	h.open(func(p *project.Project) {
		var err error
		if as, err = p.DB.Approvals(context.Background()); err != nil {
			h.t.Fatal(err)
		}
	})
	return as
}

func (h *harness) setLevel(l project.Level) {
	h.t.Helper()
	h.open(func(p *project.Project) {
		if err := p.SetLevel(context.Background(), l); err != nil {
			h.t.Fatal(err)
		}
	})
}

// The turn waits for the card; Allow runs the tool and is remembered,
// Deny with a note doesn't run it and the note reaches the model.
func TestApprovalCard(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n := newNeedy()
		h := newHarness(t, t.TempDir(), "", testSettings(), n)
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(fake.ToolCall("c1", "needy", needs("", "api.example.com")), fake.Text("done"))
		h.send(c.ID, "prices?")
		w := h.waiting(c.ID)
		if w.Kind != chat.PartApproval || w.Text != "Allow requests to api.example.com for this project?" {
			t.Errorf("waiting %+v", w)
		}
		st := h.pub.allStatuses()
		if last := st[len(st)-1]; last.State != chat.StateWaiting || last.Waiting == nil || *last.Waiting != w {
			t.Errorf("last status %+v", last)
		}
		if s := h.o.ChatStatus(c.ID); s != "waiting for the user" {
			t.Errorf("ChatStatus = %q", s)
		}
		if s := h.o.State(h.pid, c.ID); s != chat.StateWaiting {
			t.Errorf("State = %q", s)
		}
		if ws := h.o.Waits(); len(ws) != 1 || ws[0] != (Wait{Project: h.pid, Chat: c.ID, Waiting: w}) {
			t.Errorf("Waits %+v", ws)
		}
		if s := h.o.State(h.pid, id.Chat(id.New())); s != chat.StateIdle {
			t.Errorf("unknown chat State = %q", s)
		}
		if st := h.o.find(h.pid, c.ID).Status(); len(st) != 1 || st[0].State != "waiting" || !strings.HasSuffix(st[0].Progress, ", waiting for the user") {
			t.Errorf("activity %+v", st)
		}
		if n.runs.Load() != 0 {
			t.Fatal("the tool ran before the answer")
		}
		for name, a := range map[string]Answer{
			"another card":   {Message: w.Message, Index: w.Index + 1, Grant: chat.GrantAlways},
			"another chat":   {Message: id.Message(id.New()), Index: w.Index, Grant: chat.GrantAlways},
			"not an option":  {Message: w.Message, Index: w.Index, Grant: chat.GrantOnce},
			"no grant":       {Message: w.Message, Index: w.Index},
			"a form answer":  {Message: w.Message, Index: w.Index, Grant: chat.GrantAlways, Answers: map[string][]string{"x": {"y"}}},
			"unknown answer": {Message: w.Message, Index: w.Index, Grant: "forever"},
		} {
			want := ErrBadAnswer
			if strings.HasPrefix(name, "another") {
				want = ErrNotWaiting
			}
			if err := h.o.Answer(h.pid, c.ID, a); !errors.Is(err, want) {
				t.Errorf("%s: %v, want %v", name, err, want)
			}
		}
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantAlways})
		st = h.pub.allStatuses()
		if last := st[len(st)-1]; last.State != chat.StateWorking || last.Waiting != nil {
			t.Errorf("status after the answer %+v", last)
		}
		h.wait()
		ms := h.messages(c.ID)
		validHistory(t, ms)
		if got := texts(ms); !slices.Contains(got, "tool: result ran for api.example.com") {
			t.Errorf("history %q", got)
		}
		cs := cards(ms)
		if len(cs) != 1 || cs[0].Answer != chat.GrantAlways || cs[0].By != id.SourceUser || cs[0].AnsweredAt == nil {
			t.Errorf("cards %+v", cs)
		}
		if as := h.approvals(); len(as) != 1 || as[0].Target != "api.example.com" || as[0].Answer != chat.GrantAlways || as[0].Source != id.SourceUser {
			t.Errorf("records %+v", as)
		}
		if st := h.pub.allStatuses(); st[len(st)-1].State != chat.StateIdle {
			t.Errorf("last status %+v", st[len(st)-1])
		}

		// Approved for the project: no card the next time.
		h.fp.Push(fake.ToolCall("c2", "needy", needs("", "api.example.com")), fake.Text("done"))
		h.turn(c.ID, "again")
		if n.runs.Load() != 2 || len(cards(h.messages(c.ID))) != 1 {
			t.Errorf("%d runs, %d cards", n.runs.Load(), len(cards(h.messages(c.ID))))
		}

		// Deny with a note.
		h.fp.Push(fake.ToolCall("c3", "needy", needs("", "evil.example")), fake.Text("ok, not that one"))
		h.send(c.ID, "and evil?")
		w = h.waiting(c.ID)
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantDeny, Note: " use the other API "})
		h.wait()
		if n.runs.Load() != 2 {
			t.Error("a denied tool ran")
		}
		want := "not run: the user denied it (Allow requests to evil.example for this project?); their note: use the other API"
		calls := h.fp.Calls()
		if got := texts(calls[len(calls)-1].Messages); !slices.Contains(got, "tool: result "+want) {
			t.Errorf("last request %q", got)
		}
		cs = cards(h.messages(c.ID))
		if d := cs[len(cs)-1]; d.Answer != chat.GrantDeny || d.Note != "use the other API" {
			t.Errorf("denied card %+v", d)
		}
		if as := h.approvals(); len(as) != 2 || as[0].Answer != chat.GrantDeny || as[0].Note != "use the other API" {
			t.Errorf("records %+v", as)
		}
	})
}

// A message written instead of answering closes the card as Deny, with
// the message as its note; the model gets both.
func TestDenyByMessage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n := newNeedy()
		h := newHarness(t, t.TempDir(), "", testSettings(), n)
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(fake.ToolCall("c1", "needy", needs("", "api.example.com")), fake.Text("ok"))
		h.send(c.ID, "prices?")
		h.waiting(c.ID)
		h.send(c.ID, "no, use binance")
		h.wait()
		if n.runs.Load() != 0 {
			t.Error("the tool ran")
		}
		ms := h.messages(c.ID)
		validHistory(t, ms)
		if cs := cards(ms); len(cs) != 1 || cs[0].Answer != chat.GrantDeny || cs[0].Note != "no, use binance" || cs[0].By != id.SourceUser {
			t.Errorf("cards %+v", cs)
		}
		calls := h.fp.Calls()
		got := texts(calls[len(calls)-1].Messages)
		want := []string{"user: prices?", "assistant: call needy",
			"tool: result not run: the user denied it (Allow requests to api.example.com for this project?); their note: no, use binance",
			"user: no, use binance"}
		if !slices.Equal(got, want) {
			t.Errorf("last request\n got %q\nwant %q", got, want)
		}
		if turnsOf(ms)[len(turnsOf(ms))-1] != 1 {
			t.Errorf("turns %v: the message should join turn 1", turnsOf(ms))
		}
	})
}

// Stop while waiting closes the card without an answer and records
// nothing.
func TestStopWhileWaiting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n := newNeedy()
		h := newHarness(t, t.TempDir(), "", testSettings(), n)
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		h.fp.Push(fake.ToolCall("c1", "needy", needs("", "api.example.com")))
		h.send(c.ID, "prices?")
		w := h.waiting(c.ID)
		h.o.Stop(h.pid, c.ID)
		h.wait()
		ms := h.messages(c.ID)
		validHistory(t, ms)
		if cs := cards(ms); len(cs) != 1 || !cs[0].Stopped || cs[0].Answer != "" {
			t.Errorf("cards %+v", cs)
		}
		if got := texts(ms); got[len(got)-1] != "tool: result cancelled by user" {
			t.Errorf("history %q", got)
		}
		if as := h.approvals(); len(as) != 0 {
			t.Errorf("records %+v", as)
		}
		if err := h.o.Answer(h.pid, c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantAlways}); !errors.Is(err, ErrNotWaiting) {
			t.Errorf("Answer after Stop: %v", err)
		}
		if l := h.o.Live(h.pid, c.ID); l.Running || l.Waiting != nil {
			t.Errorf("Live %+v", l)
		}
	})
}

// Each level on the test tool (8.8): Strict and Standard ask for a new
// host; Auto approves it unless the turn read outside data since the
// user's last message; Starlark code is approved at Auto even then; a kind
// without a rule asks at every level.
func TestApprovalLevels(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		n := newNeedy()
		h := newHarness(t, t.TempDir(), "", testSettings(), n, outside())
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Levels"})
		i := 0
		try := func(level project.Level, kind string, untrusted, wantAsk bool) {
			t.Helper()
			i++
			target := fmt.Sprintf("t%d.example", i)
			h.setLevel(level)
			if untrusted {
				h.fp.Push(fake.ToolCall("o", "outside", map[string]any{}))
			}
			h.fp.Push(fake.ToolCall("c", "needy", needs(kind, target)), fake.Text("done"))
			h.send(c.ID, "go")
			synctest.Wait()
			name := fmt.Sprintf("%s %s untrusted=%v", level, kind, untrusted)
			l := h.o.Live(h.pid, c.ID)
			if asked := l.Waiting != nil; asked != wantAsk {
				t.Fatalf("%s: asked %v, want %v", name, asked, wantAsk)
			}
			if wantAsk {
				h.answer(c.ID, Answer{Message: l.Waiting.Message, Index: l.Waiting.Index, Grant: chat.GrantDeny})
				h.wait()
				return
			}
			h.wait()
			cs := cards(h.messages(c.ID))
			last := cs[len(cs)-1]
			wantGrant := chat.GrantAlways
			if kind == "starlark" {
				wantGrant = chat.GrantOnce
			}
			if last.Target != target || last.By != id.SourceAuto || last.Answer != wantGrant || last.AnsweredAt == nil {
				t.Errorf("%s: chip %+v", name, last)
			}
			if as := h.approvals(); as[0].Target != target || as[0].Source != id.SourceAuto || as[0].Answer != wantGrant {
				t.Errorf("%s: record %+v", name, as[0])
			}
		}
		try(project.Strict, "host", false, true)
		try(project.Standard, "host", false, true)
		try(project.Auto, "host", false, false)
		try(project.Auto, "host", true, true)
		try(project.Auto, "host", false, false) // the user's next message clears the mark
		try(project.Auto, "starlark", true, false)
		try(project.Standard, "starlark", false, true)
		try(project.Auto, "migration", false, true)
		if runs := n.runs.Load(); runs != 3 {
			t.Errorf("%d runs, want 3 (the auto-approved ones)", runs)
		}
	})
}

// ask_user shows the form, and the answer comes back as the tool result.
func TestAskUser(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings(), Tools()...)
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Prices"})
		form := map[string]any{"questions": []map[string]any{
			{"header": "Currency", "question": "Which currency?", "options": []map[string]any{{"label": "EUR", "recommended": true}, {"label": "USD"}}},
			{"header": "Coins", "question": "Which coins?", "options": []map[string]any{{"label": "BTC"}, {"label": "ETH"}}, "multi": true},
			{"header": "Period", "question": "Which period?", "options": []map[string]any{{"label": "Day"}, {"label": "Week"}}},
		}}
		h.fp.Push(fake.ToolCall("c1", "ask_user", form), fake.Text("ok"))
		h.send(c.ID, "set up prices")
		w := h.waiting(c.ID)
		if w.Kind != chat.PartQuestion || w.Text != "Which currency? (and 2 more)" {
			t.Errorf("waiting %+v", w)
		}
		at := func(a Answer) Answer { a.Message, a.Index = w.Message, w.Index; return a }
		for name, a := range map[string]Answer{
			"a grant":         at(Answer{Grant: chat.GrantAlways, Answers: map[string][]string{"Currency": {"EUR"}}}),
			"unknown header":  at(Answer{Answers: map[string][]string{"Colour": {"red"}}}),
			"two for one":     at(Answer{Answers: map[string][]string{"Currency": {"EUR", "USD"}}}),
			"nothing":         at(Answer{Note: "hm"}),
			"only blank text": at(Answer{Answers: map[string][]string{"Currency": {" "}}}),
		} {
			if err := h.o.Answer(h.pid, c.ID, a); !errors.Is(err, ErrBadAnswer) {
				t.Errorf("%s: %v", name, err)
			}
		}
		h.answer(c.ID, at(Answer{Answers: map[string][]string{"Currency": {"<EUR>"}, "Coins": {"BTC", " ", "ETH"}, "Period": {}}, Note: "weekly please"}))
		h.wait()
		want := `{"Coins":["BTC","ETH"],"Currency":"<EUR>"}` + "\nNote: weekly please"
		calls := h.fp.Calls()
		if got := texts(calls[len(calls)-1].Messages); !slices.Contains(got, "tool: result "+want) {
			t.Errorf("last request %q", got)
		}
		ms := h.messages(c.ID)
		validHistory(t, ms)
		var q *chat.Question
		for _, m := range ms {
			for _, p := range m.Parts {
				if p.Question != nil {
					q = p.Question
				}
			}
		}
		if q == nil || q.AnsweredAt == nil || len(q.Answers) != 2 || q.Note != "weekly please" {
			t.Errorf("form %+v", q)
		}

		// A message written instead is the answer.
		h.fp.Push(fake.ToolCall("c2", "ask_user", form), fake.Text("ok"))
		h.send(c.ID, "again")
		h.waiting(c.ID)
		h.send(c.ID, "EUR, BTC, a week")
		h.wait()
		calls = h.fp.Calls()
		if got := texts(calls[len(calls)-1].Messages); !slices.Contains(got, "tool: result The user wrote a message instead of answering the form: EUR, BTC, a week") {
			t.Errorf("last request %q", got)
		}

		// A form that breaks the rules goes back to the model.
		bad := map[string]any{"questions": []map[string]any{
			{"header": "A", "question": "x?", "options": []map[string]any{{"label": "1"}, {"label": "2"}}},
			{"header": "A", "question": "y?", "options": []map[string]any{{"label": "1"}, {"label": "2"}}},
		}}
		h.fp.Push(fake.ToolCall("c3", "ask_user", bad), fake.Text("ok"))
		h.turn(c.ID, "once more")
		calls = h.fp.Calls()
		if got := texts(calls[len(calls)-1].Messages); !slices.Contains(got, `tool: result header "A" is used twice`) {
			t.Errorf("last request %q", got)
		}
	})
}

// Where no one can answer, ask_user says so.
func TestAskUserWithoutAsk(t *testing.T) {
	args := `{"questions": [{"header": "A", "question": "x?", "options": [{"label": "1"}, {"label": "2"}]}]}`
	_, err := tool.Run(context.Background(), askUser(), tool.Call{Args: json.RawMessage(args), Env: &tool.Env{}})
	var te *tool.Error
	if !errors.As(err, &te) || !strings.Contains(te.Msg, "no one can answer") {
		t.Errorf("err = %v", err)
	}
}

// A level change adds a notice, and a failed turn can still be retried
// after it, across a restart.
func TestSetLevel(t *testing.T) {
	root := t.TempDir()
	h := newHarness(t, root, "", testSettings())
	c := h.newChat(chat.Chat{Title: "Prices"})
	ctx := context.Background()
	if err := h.o.SetLevel(ctx, h.pid, c.ID, "yolo"); err == nil {
		t.Error("an unknown level was set")
	}
	if err := h.o.SetLevel(ctx, h.pid, c.ID, project.Standard); err != nil {
		t.Fatal(err)
	}
	if ms := h.messages(c.ID); len(ms) != 0 {
		t.Errorf("the same level added %q", texts(ms))
	}
	if err := h.o.SetLevel(ctx, h.pid, c.ID, project.Strict); err != nil {
		t.Fatal(err)
	}
	ms := h.messages(c.ID)
	if got := texts(ms); len(got) != 1 || got[0] != "user: [the approval level changed from standard to strict]" || ms[0].Turn != 1 {
		t.Errorf("history %q, turn %d", got, ms[0].Turn)
	}
	h.open(func(p *project.Project) {
		if l, _ := p.Level(ctx); l != project.Strict {
			t.Errorf("level %q", l)
		}
	})
	h.fp.Push(fake.Fail(&provider.Error{Kind: provider.BadRequest, Provider: "p", Status: 401, Message: "bad key"}))
	h.turn(c.ID, "price?")
	if ms := h.messages(c.ID); turnsOf(ms)[len(turnsOf(ms))-1] != 1 {
		t.Errorf("turns %v: the first message joins the notice's turn", turnsOf(ms))
	}
	if err := h.o.SetLevel(ctx, h.pid, c.ID, project.Auto); err != nil {
		t.Fatal(err)
	}
	h.stop()

	h = newHarness(t, root, h.pid, testSettings())
	defer h.stop()
	if err := h.o.Retry(ctx, h.pid, c.ID); err != nil {
		t.Fatalf("Retry after a level change and a restart: %v", err)
	}
	h.wait()
}

// A network tool gets the project's private host exceptions; others get
// none.
func TestPrivateHostsReachTheTool(t *testing.T) {
	report := func(name string, e tool.Effects) tool.Tool {
		return tool.Func(tool.Spec{Name: name, Description: "Reports.", Effects: e, Schema: json.RawMessage(`{"type": "object"}`)},
			func(_ context.Context, env *tool.Env, _ struct{}) (tool.Result, error) {
				if env.PrivateHosts == nil {
					return tool.Result{Text: "none"}, nil
				}
				return tool.Result{Text: fmt.Sprint(env.PrivateHosts("NAS.local"), env.PrivateHosts("other.local"), env.ChatStatus != nil)}, nil
			})
	}
	h := newHarness(t, t.TempDir(), "", testSettings(), report("net", tool.Network), report("local", tool.ReadsDB))
	defer h.stop()
	h.open(func(p *project.Project) {
		if err := p.SetPrivateHosts(context.Background(), []string{"nas.local"}); err != nil {
			t.Fatal(err)
		}
	})
	c := h.newChat(chat.Chat{Title: "Hosts"})
	h.fp.Push(fake.ToolCall("c1", "net", map[string]any{}), fake.ToolCall("c2", "local", map[string]any{}), fake.Text("ok"))
	h.turn(c.ID, "go")
	got := texts(h.messages(c.ID))
	if !slices.Contains(got, "tool: result true false true") || !slices.Contains(got, "tool: result none") {
		t.Errorf("history %q", got)
	}
}

// fetch_page asks for a new host through its preflight; denied, nothing
// is fetched.
func TestFetchPageAsksForTheHost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newHarness(t, t.TempDir(), "", testSettings(), tool.Builtin(tool.Deps{Web: web.NewClient(web.Options{})})...)
		defer h.stop()
		c := h.newChat(chat.Chat{Title: "Pages"})
		h.fp.Push(fake.ToolCall("c1", "fetch_page", map[string]string{"url": "https://Example.COM/a"}), fake.Text("ok"))
		h.send(c.ID, "read it")
		w := h.waiting(c.ID)
		if w.Text != "Allow requests to example.com for this project?" {
			t.Errorf("waiting %+v", w)
		}
		h.answer(c.ID, Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantDeny})
		h.wait()
		if got := texts(h.messages(c.ID)); !slices.Contains(got, "tool: result not run: the user denied it (Allow requests to example.com for this project?)") {
			t.Errorf("history %q", got)
		}

		// A URL the preflight refuses is an error for the model, with no card.
		h.fp.Push(fake.ToolCall("c2", "fetch_page", map[string]string{"url": "ftp://example.com/a"}), fake.Text("ok"))
		h.turn(c.ID, "and this")
		ms := h.messages(c.ID)
		if n := len(cards(ms)); n != 1 {
			t.Errorf("%d cards", n)
		}
		if got := texts(ms); !slices.ContainsFunc(got, func(s string) bool {
			return strings.HasPrefix(s, "tool: result ") && strings.Contains(s, "only http and https URLs are allowed") && !strings.Contains(s, "failed")
		}) {
			t.Errorf("history %q", got)
		}
	})
}
