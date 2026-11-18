package chat_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/chat"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
)

const persian = "سلام، این یک آزمایش است"

// fixture is the chat screen on fake services, showing one chat.
type fixture struct {
	f    *statetest.Fake
	loop *statetest.Loop
	st   *state.Stores
	scr  *chat.Screen
	tt   *kit.Tester
	p    id.Project
	ch   id.Chat
	seq  uint64
}

// open shows chat ch of p with the messages ms.
func open(t *testing.T, dark bool, ms ...jc.Message) *fixture {
	t.Helper()
	return openOn(t, dark, statetest.New(), ms...)
}

// openOn is open on the fake services f.
func openOn(t *testing.T, dark bool, f *statetest.Fake, ms ...jc.Message) *fixture {
	t.Helper()
	return openWith(t, dark, f, f.API(), ms...)
}

// openWith is open on api, with f its chats.
func openWith(t *testing.T, dark bool, f *statetest.Fake, api state.API, ms ...jc.Message) *fixture {
	t.Helper()
	x := newFixture(t, f, api, ms...)
	x.show(t, dark)
	return x
}

// newFixture makes the stores and the chat screen on api, and chat ch of
// p with the messages ms, but doesn't open it: show does.
func newFixture(t *testing.T, f *statetest.Fake, api state.API, ms ...jc.Message) *fixture {
	t.Helper()
	x := &fixture{f: f, loop: &statetest.Loop{}}
	x.p, _ = x.f.AddProject("Demo")
	x.ch = x.f.AddChat(x.p, "Notes")
	x.f.AddMessages(x.ch, ms...)
	x.st = state.NewStores(api, x.loop.Post)
	t.Cleanup(x.st.Close)
	x.scr = chat.New(x.st)
	x.scr.Now = func() time.Time { return time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC) }
	return x
}

// show opens chat ch and draws it.
func (x *fixture) show(t *testing.T, dark bool) {
	t.Helper()
	x.st.OpenChat(x.p, x.ch)
	if !x.loop.Drain(func() bool { return x.st.Threads.Threads[x.ch] != nil }) {
		t.Fatal("the chat did not open")
	}
	tt, err := kit.NewTester(func(c *kit.Context) {
		kit.Root(c, func() {
			if th := x.st.Threads.Threads[x.ch]; th != nil {
				kit.Bar(c, func() { // the chat's header, as the shell's
					kit.Title(c, th.Chat.Title)
					kit.Spacer(c)
					x.scr.HeaderActions(c, th)
				})
				x.scr.View(c, th, false, false)
			}
		})
	}, 1100, 800, dark)
	if err != nil {
		t.Fatal(err)
	}
	tt.UseScreenReader()
	x.tt = tt
	x.draw()
}

// draw runs the work queued, then draws twice: a list lays out its rows
// in the frame after it learns their sizes.
func (x *fixture) draw() {
	x.loop.Drain(nil)
	x.tt.Frame()
	x.tt.Advance(time.Second)
	x.tt.Frame()
}

func (x *fixture) until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	if !x.loop.Drain(func() bool { x.tt.Frame(); return cond() }) {
		t.Fatalf("%s did not happen: %q", what, x.tt.Texts())
	}
	x.draw()
}

func (x *fixture) status(s jc.Status) {
	x.seq++
	s.Project, s.Chat, s.Seq = x.p, x.ch, x.seq
	x.st.Apply(app.EventStatus, s)
}

func (x *fixture) delta(m id.Message, part int, kind jc.PartKind, text string) {
	x.seq++
	x.st.Apply(app.EventDelta, jc.Delta{Project: x.p, Chat: x.ch, Seq: x.seq, Message: m, Part: part, Kind: kind, Text: text})
}

func shows(t *testing.T, tt *kit.Tester, texts ...string) {
	t.Helper()
	for _, s := range texts {
		if !kit.Shows(tt, s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
}

func both(t *testing.T, test func(t *testing.T, dark bool)) {
	t.Run("light", func(t *testing.T) { test(t, false) })
	t.Run("dark", func(t *testing.T) { test(t, true) })
}

func text(s string) jc.Part { return jc.Part{Kind: jc.PartText, Text: &jc.Text{Text: s}} }

func answer(turn int, parts ...jc.Part) jc.Message {
	m := statetest.Text(turn, jc.RoleAssistant, "")
	m.Parts = parts
	return m
}

func TestTheThreadShowsEachKindOfPart(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		args, _ := json.Marshal(map[string]string{"path": "notes.md"})
		x := open(t, dark,
			statetest.Text(1, jc.RoleUser, persian),
			answer(1,
				jc.Part{Kind: jc.PartThinking, Thinking: &jc.Thinking{Text: "the user wants a table"}},
				jc.Part{Kind: jc.PartToolCall, ToolCall: &jc.ToolCall{ID: "call1", Name: "read_file", Args: args}},
			),
			jc.Message{Turn: 1, Role: jc.RoleTool, Parts: []jc.Part{{Kind: jc.PartToolResult, ToolResult: &jc.ToolResult{CallID: "call1", Text: "# Notes"}}}},
			answer(1, text("Here is **the table** and a [link](https://example.com).")),
		)
		shows(t, x.tt, "You said", persian, "Jenab said", "Thought", "read_file", "the table", "Copy", "Reply", "Edit and resend")
		if x.tt.HasText("the user wants a table") {
			t.Error("the thinking shows before it is unfolded")
		}
		if !kit.HasRole(x.tt, "list", "Messages") || !kit.HasRole(x.tt, "article", "Jenab said") {
			t.Error("the thread or a message has no role")
		}
		if err := x.tt.Click("Thought"); err != nil {
			t.Fatal(err)
		}
		x.draw()
		shows(t, x.tt, "the user wants a table")
		if err := x.tt.Click("Copy"); err != nil {
			t.Fatal(err)
		}
		x.draw()
		if got := x.tt.Clipboard(); !strings.Contains(got, "the table") {
			t.Errorf("Copy put %q on the clipboard", got)
		}
		shows(t, x.tt, "Copied")
	})
}

func TestAnAnswerStreamsAndStops(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := open(t, dark, statetest.Text(1, jc.RoleUser, "Write a poem"))
		x.status(jc.Status{State: jc.StateWorking})
		x.draw()
		shows(t, x.tt, "Working", "Stop")
		x.status(jc.Status{State: jc.StateWorking, Streaming: "a1"})
		x.delta("a1", 0, jc.PartText, "First line ")
		x.delta("a1", 0, jc.PartText, "and "+persian)
		x.draw()
		shows(t, x.tt, "Jenab is answering", "First line and "+persian)
		if err := x.tt.Click("Stop"); err != nil {
			t.Fatal(err)
		}
		x.until(t, "Stop", func() bool { return x.f.Stops() == 1 })
		x.f.AddMessages(x.ch, jc.Message{ID: "a1", Turn: 1, Role: jc.RoleAssistant, CreatedAt: time.Now(), Parts: []jc.Part{text("First line and " + persian)}})
		x.status(jc.Status{State: jc.StateIdle})
		x.until(t, "the read after the turn", func() bool { return len(x.st.Threads.Threads[x.ch].Messages) == 2 })
		shows(t, x.tt, "Jenab said", "First line and "+persian)
		if !strings.Contains(strings.Join(x.tt.Announcements(), "|"), "Answer finished") {
			t.Errorf("no announcement, got %q", x.tt.Announcements())
		}
		if x.tt.HasText("Stop") {
			t.Error("Stop still shows after the turn")
		}
	})
}

func TestAnApprovalCardAnswers(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		card := answer(1, jc.Part{Kind: jc.PartApproval, Approval: &jc.Approval{ID: "ap1", Kind: "host", Target: "example.com",
			Ask: "Fetch pages from example.com?", Risk: "low", Options: []jc.ApprovalOption{
				{Label: "Allow once", Grant: jc.Grant("once")}, {Label: "Deny", Grant: jc.Grant("deny")}}}})
		card.ID = "card1"
		x := open(t, dark, statetest.Text(1, jc.RoleUser, "Read example.com"), card)
		x.status(jc.Status{State: jc.StateWaiting, Waiting: &jc.Waiting{Message: "card1", Index: 0, Kind: jc.PartApproval, Text: "Fetch pages?"}})
		x.draw()
		shows(t, x.tt, "Fetch pages from example.com?", "Allow once", "Deny")
		if err := x.tt.Click("Allow once"); err != nil {
			t.Fatal(err)
		}
		x.until(t, "the answer", func() bool { return len(x.f.Answers()) == 1 })
		if a := x.f.Answers()[0]; a.Message != "card1" || a.Grant != "once" {
			t.Errorf("answered %+v", a)
		}
	})
}

func TestAQuestionFormAnswers(t *testing.T) {
	q := answer(1, jc.Part{Kind: jc.PartQuestion, Question: &jc.Question{Questions: []jc.QuestionItem{{
		Header: "Color", Question: "Which color?", Options: []jc.Option{{Label: "Blue"}, {Label: "سبز", Description: "Green"}},
	}}}})
	q.ID = "q1"
	x := open(t, false, statetest.Text(1, jc.RoleUser, "Make a page"), q)
	x.status(jc.Status{State: jc.StateWaiting, Waiting: &jc.Waiting{Message: "q1", Index: 0, Kind: jc.PartQuestion, Text: "Which color?"}})
	x.draw()
	shows(t, x.tt, "Which color?", "Blue", "سبز", "Green", "Submit")
	if err := x.tt.Click("سبز"); err != nil {
		t.Fatal(err)
	}
	x.draw()
	if err := x.tt.Click("Submit"); err != nil {
		t.Fatal(err)
	}
	x.until(t, "the answer", func() bool { return len(x.f.Answers()) == 1 })
	if got := x.f.Answers()[0].Answers["Color"]; len(got) != 1 || got[0] != "سبز" {
		t.Errorf("answered %v", x.f.Answers()[0].Answers)
	}
}

func TestReplyQuotesAndEscapeTakesItOff(t *testing.T) {
	x := open(t, false, statetest.Text(1, jc.RoleUser, "Hi"), answer(1, text("An answer to quote")))
	clickLast(t, x.tt, "Reply") // the answer's
	x.draw()
	if q, ok := x.st.Quotes[x.ch]; !ok || q.Role != jc.RoleAssistant {
		t.Fatalf("Reply set no quote of the answer: %+v %q", q, x.tt.Texts())
	}
	shows(t, x.tt, "An answer to quote")
	if !x.tt.Focused("Message") {
		t.Error("Reply did not focus the composer")
	}
	x.tt.Key(0, kit.KeyEscape)
	x.draw()
	if _, ok := x.st.Quotes[x.ch]; ok {
		t.Error("Escape left the quote")
	}
}

func TestEditAndResendRewinds(t *testing.T) {
	x := open(t, false,
		statetest.Text(1, jc.RoleUser, "First"), answer(1, text("One")),
		statetest.Text(2, jc.RoleUser, "Second "+persian), answer(2, text("Two")))
	clickLast(t, x.tt, "Edit and resend") // the second message's
	x.until(t, "the dialog", func() bool { return x.tt.HasText("Edit and resend?") })
	clickLast(t, x.tt, "Edit and resend") // the dialog's
	x.until(t, "the rewind", func() bool { return len(x.f.Rewinds()) == 1 })
	x.until(t, "the reload", func() bool { return len(x.st.Threads.Threads[x.ch].Messages) == 2 })
	if got := x.f.Rewinds()[0]; got != 1 {
		t.Errorf("rewound to turn %d, want 1", got)
	}
	x.draw()
	if x.tt.HasText("Two") {
		t.Error("the rewound answer still shows")
	}
	if a, _ := x.tt.Access("Message"); a.Value != "Second "+persian {
		t.Errorf("the composer holds %q, want the message taken out", a.Value)
	}
}

// clickLast clicks the last element named label, as a screen reader
// finds them.
func clickLast(t *testing.T, tt *kit.Tester, label string) {
	t.Helper()
	found := false
	var x, y float32
	for _, a := range tt.AccessAll() {
		if a.Name == label {
			found, x, y = true, a.Bounds.X+a.Bounds.W/2, a.Bounds.Y+a.Bounds.H/2
		}
	}
	if !found {
		t.Fatalf("no %q", label)
	}
	tt.ClickAt(x, y)
}

func TestALongChatBuildsOnlyWhatShows(t *testing.T) {
	var ms []jc.Message
	for i := 1; i <= 100; i++ {
		ms = append(ms, statetest.Text(i, jc.RoleUser, fmt.Sprintf("question %d", i)), answer(i, text(fmt.Sprintf("answer %d", i))))
	}
	x := open(t, false, ms...)
	shows(t, x.tt, "answer 100")
	for _, s := range x.tt.Texts() {
		if s == "question 1" || s == "answer 50" {
			t.Errorf("the list built %q, far above the end", s)
		}
	}
	n := 0
	for _, s := range x.tt.Texts() {
		if strings.HasPrefix(s, "answer ") {
			n++
		}
	}
	if n == 0 || n > 20 {
		t.Errorf("%d answers built of 100", n)
	}
}

// A selection runs across messages and copies as plain text, as a
// browser copies one, with the items of a list the kit builds again for
// their direction; an answer's Copy copies its Markdown.
func TestASelectionCopiesAcrossMessages(t *testing.T) {
	x := open(t, false,
		statetest.Text(1, jc.RoleUser, "first question"),
		answer(1, text("the **first** answer\n\n- خرید نان\n- an English item")),
		statetest.Text(2, jc.RoleUser, "second question"),
		answer(2, text("the "+persian+" answer")))
	all := x.tt.AccessAll()
	from, to := -1, -1
	for i, a := range all {
		switch {
		case a.Name == "first question":
			from = i
		case strings.Contains(a.Name, persian):
			to = i
		}
	}
	if from < 0 || to < 0 {
		t.Fatalf("no first question or Persian answer in %q", x.tt.Texts())
	}
	// From the start of the first question to the end of the last answer,
	// a left-to-right paragraph, as it starts with "the".
	f, e := all[from].Bounds, all[to].Bounds
	x.tt.Press(f.X+1, f.Y+f.H/2)
	x.tt.Move(e.X+e.W-1, e.Y+e.H/2)
	x.tt.Release(e.X+e.W-1, e.Y+e.H/2)
	x.draw()
	x.tt.Key(kit.Ctrl, kit.KeyC)
	x.tt.Frame()
	got := x.tt.Clipboard()
	t.Logf("the selection copied %q", got)
	for _, want := range []string{"first question", "the first answer", "خرید نان", "an English item", "second question", persian} {
		if !strings.Contains(got, want) {
			t.Errorf("the copy %q has no %q", got, want)
		}
	}
	if strings.Contains(got, "**") {
		t.Errorf("the selection copied Markdown: %q", got)
	}
	clickLast(t, x.tt, "Copy")
	x.draw()
	if got := x.tt.Clipboard(); got != "the "+persian+" answer" {
		t.Errorf("Copy copied %q", got)
	}
}

// The composer's Approvals picker sets the project's level (SPEC 8.8), by
// the mouse and by the keyboard, and shows the level it set.
func TestTheApprovalsPickerSetsTheLevel(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := open(t, dark, statetest.Text(1, jc.RoleUser, persian))
		x.st.Projects.Open(x.p, func(app.OpenedProject, error) {})
		x.until(t, "the project", func() bool { _, ok := x.st.Projects.Opened[x.p]; return ok })
		shows(t, x.tt, "Approvals: Standard")
		if err := x.tt.Click("Approvals: Standard"); err != nil {
			t.Fatal(err)
		}
		x.draw()
		shows(t, x.tt, "Approvals for this project", "Strict", "Auto", "Asks less, but always for destructive changes")
		if err := x.tt.Click("Auto"); err != nil {
			t.Fatal(err)
		}
		x.until(t, "the level", func() bool { return x.st.Projects.Opened[x.p].Level == "auto" })
		if got := x.f.Levels; len(got) != 1 || got[0] != "auto" {
			t.Errorf("set the levels %q", got)
		}
		shows(t, x.tt, "Approvals: Auto")

		// By the keyboard: Tab to the picker, Enter opens it, Home goes to
		// the first level and Enter picks it.
		for i := 0; i < 20 && !x.tt.Focused("Approvals: Auto"); i++ {
			x.tt.Key(0, kit.KeyTab)
			x.tt.Frame()
		}
		if !x.tt.Focused("Approvals: Auto") {
			t.Fatal("Tab never reached the picker")
		}
		x.tt.Key(0, kit.KeyEnter)
		x.draw()
		x.tt.Key(0, kit.KeyHome)
		x.tt.Frame()
		x.tt.Key(0, kit.KeyEnter)
		x.until(t, "the level by the keyboard", func() bool { return x.st.Projects.Opened[x.p].Level == "strict" })
		if got := x.f.Levels; len(got) != 2 || got[1] != "strict" {
			t.Errorf("set the levels %q", got)
		}
	})
}

// A message's time is in the user's regional formats, as Windows is set:
// "01:11 PM" with a 12-hour clock, "13:11" with a 24-hour one.
func TestTimesFollowTheRegion(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		m := statetest.Text(1, jc.RoleUser, "Hi")
		a := answer(1, text(persian))
		a.CreatedAt = time.Date(2026, 11, 16, 13, 11, 0, 0, time.UTC)
		x := open(t, dark, m, a)
		at := a.CreatedAt.Local()
		x.tt.SetRegion(kit.Region{Locale: "en-US", HourCycle: 12})
		x.draw()
		shows(t, x.tt, persian, at.Format("03:04 PM"))
		x.tt.SetRegion(kit.Region{Locale: "en-GB", HourCycle: 24})
		x.draw()
		shows(t, x.tt, at.Format("15:04"))
		if x.tt.HasText(at.Format("03:04 PM")) {
			t.Error("the 12-hour time still shows with a 24-hour clock")
		}
	})
}

// A deleted chat's views, chips, cards' input and what was unfolded go
// once a fresh list no longer has it; the chat shown stays, since the
// list may predate it.
func TestADeletedChatsStateGoes(t *testing.T) {
	x := open(t, false,
		statetest.Text(1, jc.RoleUser, "Search the site"),
		answer(1, notice(jc.Notice{Kind: jc.NoticeDesktopSaved, Text: "Macro saved: site_search (2 steps)",
			Target: "desktop:macro:3", Change: "12", Details: "1. click Search"})),
	)
	click(t, x, "View steps")
	click(t, x, "Undo")
	x.until(t, "the undo", func() bool { return x.f.IsUndone("desktop:12") })
	var ms []id.Message
	for _, m := range x.st.Threads.Threads[x.ch].Messages {
		ms = append(ms, m.ID)
	}
	before := x.scr.Keeps(x.ch, ms)
	if before < 4 {
		t.Fatalf("the chat keeps %d things before", before)
	}
	// Deleted while shown: it stays.
	x.f.RemoveChat(x.p, x.ch)
	x.st.Chats.Load(x.p)
	x.draw()
	if got := x.scr.Keeps(x.ch, ms); got != before {
		t.Errorf("the chat shown keeps %d things, was %d", got, before)
	}
	// Another chat shows: the next list drops it.
	other := x.f.AddChat(x.p, "Other")
	x.st.OpenChat(x.p, other)
	x.until(t, "the other chat", func() bool { return x.st.Threads.Threads[other] != nil })
	x.st.Chats.Load(x.p)
	x.draw()
	if got := x.scr.Keeps(x.ch, ms); got != 0 {
		t.Errorf("the deleted chat still keeps %d things", got)
	}
}
