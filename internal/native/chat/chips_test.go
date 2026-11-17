package chat_test

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
)

func notice(n jc.Notice) jc.Part { return jc.Part{Kind: jc.PartNotice, Notice: &n} }

// toasts are the toasts' texts the stores hold, taken.
func toasts(x *fixture) []string {
	var out []string
	for _, t := range x.st.TakeToasts() {
		out = append(out, t.Text)
	}
	return out
}

func click(t *testing.T, x *fixture, label string) {
	t.Helper()
	if err := x.tt.Click(label); err != nil {
		t.Fatalf("%s: %v in %q", label, err, x.tt.Texts())
	}
	x.draw()
}

func TestAMemoryChipUndoesRedoesAndEdits(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := open(t, dark,
			statetest.Text(1, jc.RoleUser, "Remember that I like short answers"),
			answer(1, notice(jc.Notice{Kind: jc.NoticeMemoryUpdated, Text: "Memory updated: " + persian,
				Target: "memory:project/preferences", Change: "41", Note: "From a chat that read web pages"}),
				text("Noted.")),
		)
		shows(t, x.tt, "Memory updated: "+persian, "Undo", "Edit", "From a chat that read web pages")
		shot(t, x, "memory-chip")
		click(t, x, "Undo")
		x.until(t, "the undo", func() bool { return x.f.IsUndone("memory:41") })
		shows(t, x.tt, "Memory updated: "+persian+" · undone", "Redo")
		if got := toasts(x); len(got) != 0 {
			t.Errorf("an undo that worked said %q", got)
		}
		// The keyboard reaches Redo, and Enter puts the edit back.
		focus(t, x, "Redo")
		x.tt.Key(0, kit.KeyEnter)
		x.until(t, "the redo", func() bool { return !x.f.IsUndone("memory:41") })
		shows(t, x.tt, "Undo")
		if kit.Shows(x.tt, "Memory updated: "+persian+" · undone") {
			t.Error("the chip still says undone")
		}
		click(t, x, "Edit")
		at := x.st.Nav.Current()
		if at == nil || at.View != state.ViewProjectSettings || at.Section != "Memory" || at.Project != x.p {
			t.Fatalf("Edit went to %+v", at)
		}
		if f := x.st.Shell.MemoryFocus; f == nil || f.Target != "memory:project/preferences" {
			t.Errorf("the memory section to edit is %+v", f)
		}
	})
}

func TestAMemoryChipKnowsItIsUndone(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		chip := answer(1, notice(jc.Notice{Kind: jc.NoticeMemoryUpdated, Text: "Memory updated: goals",
			Target: "memory:user/goals", Change: "7"}))
		f := statetest.New()
		f.Undone["memory:7"] = true // undone with its turn, before the chat was opened
		x := openOn(t, dark, f, statetest.Text(1, jc.RoleUser, "Set a goal"), chip)
		x.until(t, "the chip's state", func() bool { return kit.Shows(x.tt, "Redo") })
		shows(t, x.tt, "Memory updated: goals · undone")
		// Undone elsewhere meanwhile: the redo finds it already back.
		x.f.SetChanged("memory:7")
		click(t, x, "Redo")
		x.until(t, "the toast", func() bool { return len(x.st.Toasts) > 0 })
		if got := toasts(x); len(got) != 1 || got[0] != "The section changed since, so it was left as it is. Edit it in the settings." {
			t.Errorf("toasts %q", got)
		}
		click(t, x, "Edit")
		if at := x.st.Nav.Current(); at == nil || at.View != state.ViewSettings || at.Section != "Memory" {
			t.Errorf("Edit of user memory went to %+v", at)
		}
	})
}

func TestADesktopChipUndoes(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := open(t, dark,
			statetest.Text(1, jc.RoleUser, "Search the site"),
			answer(1, notice(jc.Notice{Kind: jc.NoticeDesktopSaved, Text: "Macro saved: site_search (2 steps)",
				Target: "desktop:macro:3", Change: "12", Details: "1. click Search\n2. type " + persian})),
		)
		shows(t, x.tt, "Macro saved: site_search (2 steps)", "View steps", "Undo", "Edit")
		click(t, x, "View steps")
		shows(t, x.tt, "2. type "+persian)
		shot(t, x, "desktop-chip")
		click(t, x, "Undo")
		x.until(t, "the undo", func() bool { return x.f.IsUndone("desktop:12") })
		shows(t, x.tt, "Macro saved: site_search (2 steps) · undone", "Redo")
		click(t, x, "Redo")
		x.until(t, "the redo", func() bool { return !x.f.IsUndone("desktop:12") })
		click(t, x, "Edit")
		if at := x.st.Nav.Current(); at == nil || at.View != state.ViewSettings || at.Section != "Desktop" {
			t.Errorf("Edit went to %+v", at)
		}
	})
}

// focus tabs to name, failing when Tab doesn't reach it.
func focus(t *testing.T, x *fixture, name string) {
	t.Helper()
	for range 60 {
		if x.tt.Focused(name) {
			return
		}
		x.tt.Key(0, kit.KeyTab)
	}
	t.Fatalf("Tab did not reach %q", name)
}

// shot saves the window as name-light.png or name-dark.png in the folder
// JENAB_SHOTS names, for a look outside the repo; without it, nothing.
func shot(t *testing.T, x *fixture, name string) {
	t.Helper()
	dir := os.Getenv("JENAB_SHOTS")
	if dir == "" {
		return
	}
	theme := "light"
	if filepath.Base(t.Name()) == "dark" {
		theme = "dark"
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, x.tt.Image()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "chat-"+name+"-"+theme+".png"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// off tells whether an element named name takes no input.
func off(t *testing.T, x *fixture, name string) bool {
	t.Helper()
	found := false
	for _, a := range x.tt.AccessAll() {
		if a.Name == name {
			if a.Disabled {
				return true
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("no %q in %q", name, x.tt.Texts())
	}
	return false
}

// A chip offers Undo or Redo only once it knows which applies: clicked
// before, it does nothing (TASK-071).
func TestAChipWaitsForItsState(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		chip := answer(1, notice(jc.Notice{Kind: jc.NoticeMemoryUpdated, Text: "Memory updated: " + persian,
			Target: "memory:user/goals", Change: "7"}))
		f := statetest.New()
		f.Undone["memory:7"] = true // undone with its turn, before the chat was opened
		hold := make(chan struct{})
		f.HoldUndone = hold
		defer func() {
			select {
			case <-hold:
			default:
				close(hold)
			}
		}()
		x := openOn(t, dark, f, statetest.Text(1, jc.RoleUser, "Set a goal"), chip)
		shows(t, x.tt, "Memory updated: "+persian, "Undo")
		if !off(t, x, "Undo") {
			t.Fatal("Undo is on before the chip knows it is undone")
		}
		_ = x.tt.Click("Undo")
		x.draw()
		if !x.f.IsUndone("memory:7") {
			t.Fatal("a click before the state was read undid the redo")
		}
		close(hold)
		x.until(t, "the chip's state", func() bool { return kit.Shows(x.tt, "Redo") })
		shows(t, x.tt, "Memory updated: "+persian+" · undone")
		if off(t, x, "Redo") {
			t.Error("Redo is off once the state is read")
		}
	})
}

// turnUndone is the notice an Undo turn or Redo turn leaves in the chat.
func turnUndone(msg id.Message, text string) jc.Message {
	m := statetest.Text(1, jc.RoleUser, "")
	m.ID = msg
	m.Parts = []jc.Part{notice(jc.Notice{Kind: jc.NoticeTurnUndone, Text: text})}
	return m
}

// Undo turn reverts the chips' changes with the rest, and Redo turn puts
// them back: the chips read their state again after each (TASK-071).
func TestTheChipsFollowAnUndoOfTheirTurn(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := open(t, dark,
			statetest.Text(1, jc.RoleUser, "Remember my goal and save a macro"),
			answer(1, notice(jc.Notice{Kind: jc.NoticeMemoryUpdated, Text: "Memory updated: goals",
				Target: "memory:project/goals", Change: "41"}),
				notice(jc.Notice{Kind: jc.NoticeDesktopSaved, Text: "Macro saved: " + persian,
					Target: "desktop:macro:3", Change: "12"})),
		)
		x.until(t, "the chips' state", func() bool { return !off(t, x, "Undo") })
		shows(t, x.tt, "Memory updated: goals", "Macro saved: "+persian)
		// Undo turn 1.
		x.f.SetUndone("memory:41", true)
		x.f.SetUndone("desktop:12", true)
		x.f.AddMessages(x.ch, turnUndone("undo1", "Turn 1 undone"))
		x.st.Threads.Reload(x.ch)
		x.until(t, "the chips undone", func() bool {
			return kit.Shows(x.tt, "Memory updated: goals · undone") && kit.Shows(x.tt, "Macro saved: "+persian+" · undone")
		})
		if x.tt.HasText("Undo") {
			t.Errorf("a chip still offers Undo: %q", x.tt.Texts())
		}
		// Redo turn 1.
		x.f.SetUndone("memory:41", false)
		x.f.SetUndone("desktop:12", false)
		x.f.AddMessages(x.ch, turnUndone("undo2", "Turn 1 back"))
		x.st.Threads.Reload(x.ch)
		x.until(t, "the chips back", func() bool {
			return !kit.Shows(x.tt, "Memory updated: goals · undone") && !kit.Shows(x.tt, "Macro saved: "+persian+" · undone")
		})
		if x.tt.HasText("Redo") {
			t.Errorf("a chip still offers Redo: %q", x.tt.Texts())
		}
		// The chip's own Undo goes on working.
		click(t, x, "Undo")
		x.until(t, "the undo", func() bool { return x.f.IsUndone("memory:41") || x.f.IsUndone("desktop:12") })
	})
}
