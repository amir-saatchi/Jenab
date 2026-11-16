package shell_test

import (
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/native/chat"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/shell"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
)

const persian = "سلام، این یک آزمایش است"

// app is the shell on fake services, with one project of two chats.
type fixture struct {
	f    *statetest.Fake
	loop *statetest.Loop
	st   *state.Stores
	sh   *shell.Shell
	tt   *kit.Tester
	quit int
}

func start(t *testing.T, dark bool) *fixture {
	t.Helper()
	x := &fixture{f: statetest.New(), loop: &statetest.Loop{}}
	p, mother := x.f.AddProject("Demo")
	notes := x.f.AddChat(p, persian)
	x.f.AddMessages(mother, statetest.Text(1, jc.RoleUser, "Track my reading"), statetest.Text(1, jc.RoleAssistant, "I made a **books** table."))
	x.f.AddMessages(notes, statetest.Text(1, jc.RoleUser, "یادداشت اول"))
	x.st = state.NewStores(x.f.API(), x.loop.Post)
	t.Cleanup(x.st.Close)
	x.sh = shell.New(x.st, chat.New(x.st), x.loop.Post)
	x.sh.Quit = func() { x.quit++ }
	x.sh.Start()
	x.settle(func() bool { return x.st.Projects.Loaded })
	tt, err := kit.NewTester(x.sh.View, 1280, 800, dark)
	if err != nil {
		t.Fatal(err)
	}
	x.tt = tt
	return x
}

// settle runs the stores' work until cond holds, then draws.
func (x *fixture) settle(cond func() bool) bool {
	ok := x.loop.Drain(cond)
	if x.tt != nil {
		x.tt.Frame()
		x.tt.Advance(time.Second)
	}
	return ok
}

func shows(t *testing.T, tt *kit.Tester, texts ...string) {
	t.Helper()
	for _, s := range texts {
		if !kit.Shows(tt, s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
}

func both(t *testing.T, test func(t *testing.T, x *fixture)) {
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) { test(t, start(t, dark)) })
	}
}

func TestTheFrameOpensAProjectAndItsChats(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		shows(t, x.tt, "Choose a project", "Demo", "New project", "Settings", "Quit Jenab")
		if err := x.tt.Click("Demo"); err != nil {
			t.Fatal(err)
		}
		if !x.settle(func() bool { return x.st.Nav.Current() != nil && x.st.Threads.Threads[x.st.Nav.Current().Chat] != nil }) {
			t.Fatalf("the project did not open: %q", x.tt.Texts())
		}
		x.settle(nil)
		shows(t, x.tt, "Chats", "Mother", persian, "Project home", "Track my reading", "books")
		if !kit.HasRole(x.tt, "navigation", "Chats") || !kit.HasRole(x.tt, "list", "Messages") {
			t.Error("the chat list or the thread has no role")
		}
		if err := x.tt.Click(persian); err != nil {
			t.Fatal(err)
		}
		x.settle(func() bool { return x.st.Threads.Threads[x.st.Nav.Current().Chat] != nil })
		x.settle(nil)
		shows(t, x.tt, "یادداشت اول")
		if x.tt.HasText("Track my reading") {
			t.Error("the Mother chat's thread is still shown")
		}
		// The list hides and shows again.
		if err := x.tt.Click("Hide the chat list"); err != nil {
			t.Fatal(err)
		}
		x.settle(nil)
		if kit.HasRole(x.tt, "navigation", "Chats") {
			t.Error("the chat list did not hide")
		}
		if err := x.tt.Click("Show the chat list"); err != nil {
			t.Fatal(err)
		}
		x.settle(nil)
		if !kit.HasRole(x.tt, "navigation", "Chats") {
			t.Error("the chat list did not come back")
		}
		if err := x.tt.Click("Quit Jenab"); err != nil || x.quit != 1 {
			t.Errorf("Quit: %v, %d calls", err, x.quit)
		}
	})
}

func TestTheComposerSendsWithEnter(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		x.tt.Click("Demo")
		x.settle(func() bool { return x.st.Nav.Current() != nil && x.st.Threads.Threads[x.st.Nav.Current().Chat] != nil })
		x.settle(nil)
		if !x.tt.Focused("Message") {
			t.Error("the composer has no focus once the chat opens")
		}
		x.tt.Type(persian + " and English")
		x.tt.Key(0, kit.KeyEnter)
		x.settle(func() bool { return len(x.f.Sent()) == 1 })
		sent := x.f.Sent()
		if len(sent) != 1 || sent[0].Text != persian+" and English" {
			t.Fatalf("sent %+v", sent)
		}
		if x.tt.HasText(persian + " and English") {
			t.Error("the composer kept the text")
		}
	})
}

func TestTheModelPicker(t *testing.T) {
	x := start(t, false)
	x.f.Groups = []app.ModelGroup{{Provider: "Gemini", Models: []app.ModelItem{
		{Ref: "gemini/flash", Name: "Gemini Flash", Aliases: []string{"default"}},
		{Ref: "gemini/pro", Name: "Gemini Pro"},
	}}}
	x.st.Settings.LoadModels()
	x.tt.Click("Demo")
	x.settle(func() bool {
		return len(x.st.Settings.Models) == 1 && x.st.Nav.Current() != nil && x.st.Threads.Threads[x.st.Nav.Current().Chat] != nil
	})
	x.settle(nil)
	shows(t, x.tt, "Gemini Flash")
	if err := x.tt.Click("Model: Gemini Flash"); err != nil {
		t.Fatal(err)
	}
	menu := strings.Join(x.tt.Menu(), "|")
	if !strings.Contains(menu, "Gemini Pro") {
		t.Fatalf("the menu is %q", menu)
	}
	if err := x.tt.ChooseMenuItem("Gemini Pro"); err != nil {
		t.Fatal(err)
	}
	ch := x.st.Nav.Current().Chat
	x.settle(func() bool { return x.f.SetTo[ch] == "gemini/pro" && x.st.Threads.Threads[ch].Chat.Model == "gemini/pro" })
	x.settle(nil)
	shows(t, x.tt, "Gemini Pro")
}

func TestSubline(t *testing.T) {
	now := time.Date(2026, 11, 16, 15, 0, 0, 0, time.UTC)
	today := time.Date(2026, 11, 16, 10, 42, 0, 0, time.UTC)
	before := time.Date(2026, 3, 2, 10, 42, 0, 0, time.UTC)
	for _, c := range []struct {
		it   app.ChatItem
		want string
	}{
		{app.ChatItem{State: jc.StateWaiting}, "Waiting for you"},
		{app.ChatItem{State: jc.StateWorking}, "Working…"},
		{app.ChatItem{Chat: jc.Chat{Kind: jc.KindMother}}, "Project home"},
		{app.ChatItem{Chat: jc.Chat{Role: "Reads papers\nand more"}}, "Reads papers"},
		{app.ChatItem{}, "No messages yet"},
		{app.ChatItem{LastActivity: &today}, "Last active 10:42"},
		{app.ChatItem{LastActivity: &before}, "Last active Mar 2, 2026"},
	} {
		if got := shell.Subline(c.it, now, func(t time.Time) string { return t.Format("15:04") }, func(t time.Time) string { return t.Format("Jan 2, 2006") }); got != c.want {
			t.Errorf("Subline(%+v) = %q, want %q", c.it, got, c.want)
		}
	}
	if got := shell.StatusLabel("Notes", jc.StateWaiting); got != "Notes · waiting for you" {
		t.Errorf("StatusLabel = %q", got)
	}
}
