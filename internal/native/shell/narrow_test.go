package shell_test

import (
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/shell"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// TASK-062: the narrow places, as the React app's shell under 900 px and
// the composer's @max-sm/composer.

// box is where the element a screen reader gets as name, or whose name
// starts with it when prefix, lies.
func box(t *testing.T, tt *kit.Tester, name string, prefix bool) (x, y, w, h float32) {
	t.Helper()
	for _, a := range tt.AccessAll() {
		if a.Name == name || prefix && strings.HasPrefix(a.Name, name) {
			return a.Bounds.X, a.Bounds.Y, a.Bounds.W, a.Bounds.H
		}
	}
	t.Fatalf("no %q in %q", name, tt.Texts())
	return
}

// In a chat docked about 370 px wide, the composer's row keeps its
// buttons: the model's name is cut shorter and the approval level is its
// icon, still named for what it shows, as composer.tsx does in a narrow
// composer.
func TestTheComposerFitsANarrowDock(t *testing.T) {
	bothFiles(t, func(t *testing.T, x *fixture) {
		if _, _, w, _ := box(t, x.tt, "Approvals: Standard", false); w < 100 {
			t.Errorf("the wide composer's level is %v wide, without its name", w)
		}
		x.st.Shell.SetWidth(p, 370)
		x.click(t, "the reading notes")
		if !x.settle(func() bool { _, ok := x.docked(); return ok }) {
			t.Fatal("the file did not open")
		}
		x.settle(nil)
		x.tt.Frame()
		mx, _, mw, _ := box(t, x.tt, "Model: ", true)
		ax, _, aw, _ := box(t, x.tt, "Approvals: Standard", false)
		sx, _, sw, _ := box(t, x.tt, "Send", false)
		if aw > 64 {
			t.Errorf("the level is %v wide, not its icon", aw)
		}
		if mx+mw > ax || ax+aw > sx {
			t.Errorf("the row's buttons overlap: model %v+%v, level %v+%v, send %v", mx, mw, ax, aw, sx)
		}
		if right := float32(1280 - 24); sx+sw > right {
			t.Errorf("Send ends at %v, past the dock's padding at %v", sx+sw, right)
		}
		shot(t, x, "composer-narrow-dock")
	})
}

// narrow makes the window w wide, the window's height kept.
func (x *fixture) resize(w int) {
	x.tt.SetSize(w, 800)
	x.settle(nil)
	x.tt.Frame()
}

// escape presses Escape, closing an overlay.
func (x *fixture) escape() {
	x.tt.Key(0, kit.KeyEscape)
	x.settle(nil)
}

// Below 900 px the rail and both sidebars are overlays from the window's
// edges and the bottom bar is hidden; a file docked next to the chat
// fills the area, the chat in a drawer over it (app-shell.tsx, dock.tsx).
func TestTheNarrowWindow(t *testing.T) {
	bothFiles(t, func(t *testing.T, x *fixture) {
		x.resize(800)
		for role, name := range map[string]string{"navigation": "Projects", "complementary": "Project", "contentinfo": "Status"} {
			if kit.HasRole(x.tt, role, name) {
				t.Errorf("the %s %q shows in a narrow window", role, name)
			}
		}
		if kit.Shows(x.tt, "Hide the chat list") {
			t.Error("the chat list's button shows in a narrow window")
		}
		shows(t, x.tt, "Open the sidebar", "Pages", "Where are my notes?")
		shot(t, x, "narrow-chat")

		// ☰ opens the rail with the chat list from the start edge.
		x.click(t, "Open the sidebar")
		if !kit.HasRole(x.tt, "dialog", "Projects and chats") || !kit.HasRole(x.tt, "navigation", "Chats") {
			t.Fatalf("the left overlay did not open: %q", x.tt.Texts())
		}
		if rx, _, _, rh := box(t, x.tt, "Projects", false); rx != 0 || rh < 790 {
			t.Errorf("the rail is at %v, %v high", rx, rh)
		}
		shot(t, x, "narrow-left-overlay")
		x.escape()
		if x.st.Shell.LeftOverlay || kit.HasRole(x.tt, "navigation", "Projects") {
			t.Error("Escape did not close the left overlay")
		}

		// *Pages* opens the right sidebar from the end edge.
		x.click(t, "Pages")
		if !kit.HasRole(x.tt, "complementary", "Project") {
			t.Fatal("the right overlay did not open")
		}
		if px, _, pw, _ := box(t, x.tt, "Project", false); px+pw < 799 {
			t.Errorf("the right sidebar ends at %v", px+pw)
		}
		x.filesTab(t)
		shot(t, x, "narrow-right-overlay")
		// A file chosen there opens, the overlay closing, and fills the
		// area; the chat is a drawer the file's *Chat* button opens.
		x.click(t, "notes")
		x.settle(func() bool { return x.st.Files.Folder(p, "notes/", x.st.Shell.Rev[p]).Folder != nil })
		x.settle(nil)
		x.click(t, "reading-notes.md")
		x.settle(func() bool { return x.st.Files.View(p, notesKey, x.st.Shell.Rev[p]).Loaded })
		x.settle(nil)
		if x.st.Shell.RightOverlay {
			t.Error("the right overlay stayed open")
		}
		shows(t, x.tt, "بوف کور", "Chat", "Close the file")
		if x.tt.HasText("Where are my notes?") || kit.HasRole(x.tt, "group", "The file and the chat") {
			t.Error("the chat shows next to the file in a narrow window")
		}
		if kit.Shows(x.tt, "Hide the chat") || kit.Shows(x.tt, "Move the chat to the left") {
			t.Error("the wide dock's buttons show")
		}
		shot(t, x, "narrow-file")
		x.click(t, "Chat")
		if !kit.HasRole(x.tt, "dialog", "Chat") {
			t.Fatal("the drawer did not open")
		}
		shows(t, x.tt, "Where are my notes?")
		if dx, _, dw, _ := box(t, x.tt, "Chat", false); dw != 448 || dx+dw < 799 {
			t.Errorf("the drawer is at %v, %v wide", dx, dw)
		}
		shot(t, x, "narrow-drawer")
		x.escape()
		if ref := (state.ChatRef{Project: p, Chat: x.st.Nav.Current().Chat}); x.st.Shell.Drawer[ref] {
			t.Error("Escape did not close the drawer")
		}

		// Wide again, the frame and the dock come back.
		x.resize(1280)
		if !kit.HasRole(x.tt, "navigation", "Projects") || !kit.HasRole(x.tt, "group", "The file and the chat") {
			t.Errorf("the wide frame did not come back: %q", x.tt.Texts())
		}
	})
}

// A place other than a chat gets a header with ☰ and its title; a choice
// in the rail closes the overlay.
func TestANarrowPageHasAHeader(t *testing.T) {
	bothFiles(t, func(t *testing.T, x *fixture) {
		x.resize(800)
		x.st.Nav.Go(state.Place{View: state.ViewProjectSettings, Project: p})
		x.settle(nil)
		x.tt.Frame()
		if !kit.HasRole(x.tt, "heading", "Project settings") && !x.tt.HasText("Project settings") {
			t.Errorf("no title: %q", x.tt.Texts())
		}
		if _, y, _, _ := box(t, x.tt, "Open the sidebar", false); y > 20 {
			t.Errorf("☰ is at %v, not in the header", y)
		}
		shot(t, x, "narrow-project-settings")
		x.click(t, "Open the sidebar")
		x.click(t, "Settings")
		if x.st.Shell.LeftOverlay {
			t.Error("Settings left the overlay open")
		}
		if pl := x.st.Nav.Current(); pl.View != state.ViewSettings {
			t.Errorf("Settings went to %+v", pl)
		}
		shows(t, x.tt, "Open the sidebar")
	})
}

func TestPageTitle(t *testing.T) {
	for v, want := range map[state.View]string{state.ViewSettings: "Settings", state.ViewProjectSettings: "Project settings",
		state.ViewInspector: "Turn inspector", state.ViewRuntime: "Runtime", state.ViewChat: "Chat"} {
		if got := shell.PageTitle(&state.Place{View: v}); got != want {
			t.Errorf("%v: %q, want %q", v, got, want)
		}
	}
	if got := shell.PageTitle(nil); got != "Jenab" {
		t.Errorf("the welcome page: %q", got)
	}
}
