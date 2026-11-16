package shell_test

import (
	"testing"
	"time"

	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/native/chat"
	"github.com/amir-saatchi/jenab/internal/native/dev"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/shell"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
)

// The developer tools open from the rail, and a turn's *Inspect* opens
// the inspector at that turn, while they are on at this start.
func TestTheDeveloperToolsOpenFromTheRailAndATurn(t *testing.T) {
	for _, dark := range []bool{false, true} {
		x := &fixture{f: statetest.New(), loop: &statetest.Loop{}}
		p, mother := x.f.AddProject("Demo")
		x.f.AddMessages(mother, statetest.Text(1, jc.RoleUser, "Track my reading"), statetest.Text(1, jc.RoleAssistant, "I made a **books** table."))
		d := statetest.NewDev()
		d.Add(statetest.SampleTurn(p, "Demo", mother, "Mother", 1, time.Date(2026, 11, 16, 14, 5, 0, 0, time.UTC)))
		x.st = state.NewStores(x.f.WithDevTools(), x.loop.Post)
		t.Cleanup(x.st.Close)
		store := state.NewDev(d, x.loop.Post)
		store.After = func(time.Duration, func()) {}
		x.sh = shell.New(x.st, chat.New(x.st), x.loop.Post)
		x.sh.Dev = dev.New(x.st, store)
		x.sh.Start()
		x.settle(func() bool { return x.st.Projects.Loaded && x.st.Settings.Started != nil })
		tt, err := kit.NewTester(x.sh.View, 1280, 800, dark)
		if err != nil {
			t.Fatal(err)
		}
		x.tt = tt
		x.settle(nil)

		// The rail's entry opens the inspector at the newest turn.
		if err := tt.Click("Developer tools"); err != nil {
			t.Fatalf("no Developer tools in the rail: %v", err)
		}
		x.settle(func() bool { tt.Frame(); v, _ := store.Shown(); return v != nil })
		x.settle(nil)
		shows(t, tt, "Turn inspector", "Timeline", "Demo · Mother")

		// A turn's Inspect opens it.
		x.st.OpenProject(p, mother)
		x.settle(func() bool { return x.st.Threads.Threads[mother] != nil })
		x.settle(nil)
		shows(t, tt, "I made a")
		store.Choose(state.TurnRef{Project: p, Chat: mother, Turn: 9}) // as if another turn were shown
		if err := tt.Click("Inspect"); err != nil {
			t.Fatalf("no Inspect in the turn's footer: %v in %q", err, tt.Texts())
		}
		x.settle(func() bool { tt.Frame(); v, _ := store.Shown(); return v != nil })
		x.settle(nil)
		if pl := x.st.Nav.Current(); pl == nil || pl.View != state.ViewInspector || pl.Turn != 1 || pl.Chat != mother {
			t.Fatalf("Inspect went to %+v", pl)
		}
		if pk := store.Pick; pk == nil || pk.Turn != 1 {
			t.Errorf("the inspector shows %+v", pk)
		}
		shows(t, tt, "· turn 1 · 2 requests")
	}
}

// With the developer tools off, neither the rail nor a turn offers them.
func TestTheDeveloperToolsHideWhileOff(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		x.sh.Dev = dev.New(x.st, state.NewDev(statetest.NewDev(), x.loop.Post))
		x.settle(func() bool { return x.st.Settings.Started != nil })
		if err := x.tt.Click("Demo"); err != nil {
			t.Fatal(err)
		}
		x.settle(func() bool { return x.st.Nav.Current() != nil && x.st.Threads.Threads[x.st.Nav.Current().Chat] != nil })
		x.settle(nil)
		shows(t, x.tt, "books")
		if kit.Shows(x.tt, "Developer tools") || kit.Shows(x.tt, "Inspect") {
			t.Error("the developer tools show while off")
		}
	})
}
