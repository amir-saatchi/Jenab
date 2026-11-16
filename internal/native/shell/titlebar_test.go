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

// TASK-063: the window's own title bar. On Windows the window has no
// system title bar; the columns' top bars line up in its place, under
// Windows' buttons, and the right sidebar's tabs are icons at the bar's
// end.

var tabs = []string{"Pages", "Pipelines", "Files", "Links"}

// buttons gives the window Windows' buttons and checks no control is
// under them.
func (x *fixture) buttons(t *testing.T, width int) {
	t.Helper()
	kit.WindowsButtons(x.tt)
	x.settle(nil)
	x.clear(t, width)
}

// clear checks that no control lies under the window's buttons, and that
// the bar's controls are centered with them.
func (x *fixture) clear(t *testing.T, width int) {
	t.Helper()
	if under := kit.UnderWindowButtons(x.tt, width); len(under) > 0 {
		t.Errorf("under the window's buttons: %q", under)
	}
	if off := kit.OffTheWindowButtonsMiddle(x.tt); len(off) > 0 {
		t.Errorf("not on the window buttons' middle: %q", off)
	}
}

// inBar is the box of the text or control named name in the bar's row
// from x0 on.
func (x *fixture) inBar(name string, x0 float32) (kit.AccessInfo, bool) {
	for _, a := range x.tt.AccessAll() {
		if a.Name == name && a.Bounds.Y < 32 && a.Bounds.X >= x0 {
			return a, true
		}
	}
	return kit.AccessInfo{}, false
}

// The bar is one line as high as the window's buttons: the project's
// name with its folder as its tooltip, and the chat's title with all of
// it as its tooltip and no second line, in any script; the rail's top is
// an empty strip as high as the bar.
func TestTheBarIsOneLine(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		x.buttons(t, 1280)
		x.tt.Click("Demo")
		x.settle(func() bool { return x.st.Nav.Current() != nil && x.st.Threads.Threads[x.st.Nav.Current().Chat] != nil })
		x.settle(nil)
		x.clear(t, 1280)
		// The project's folder is the menu's tooltip, not a line.
		if d, _ := kit.Description(x.tt, "Project menu"); d != `C:\Projects\Demo` {
			t.Errorf("the project menu's description is %q, want its folder", d)
		}
		if x.tt.HasText(`C:\Projects\Demo`) {
			t.Error("the project's folder shows as a line")
		}
		// Mother's header: its title, and no second line; the chat list's
		// row still says Project home.
		if _, ok := x.inBar("Mother", 312); !ok {
			t.Errorf("no Mother in the chat's header: %q", x.tt.Texts())
		}
		if _, ok := x.inBar("Project home", 312); ok {
			t.Error("the chat's header has a second line")
		}
		shows(t, x.tt, "Project home")

		// A right-to-left title, whole as its tooltip.
		x.click(t, persian)
		x.settle(func() bool { return x.st.Threads.Threads[x.st.Nav.Current().Chat] != nil })
		x.settle(nil)
		a, ok := x.inBar(persian, 312)
		if !ok {
			t.Fatalf("no %q in the chat's header: %q", persian, x.tt.Texts())
		}
		if a.Description != persian {
			t.Errorf("the title's description is %q, want the title", a.Description)
		}
		if mid := a.Bounds.Y + a.Bounds.H/2; mid < 15.5 || mid > 16.5 {
			t.Errorf("the title is centered at %v, want the window buttons' 16", mid)
		}
		for _, s := range []string{"Last active", "No messages yet"} {
			for _, b := range x.tt.AccessAll() {
				if b.Bounds.Y < 32 && b.Bounds.X >= 312 && len(b.Name) >= len(s) && b.Name[:len(s)] == s {
					t.Errorf("the chat's header has a second line %q", b.Name)
				}
			}
		}
		// The rail's tiles start under the bar.
		if _, y, _, _ := box(t, x.tt, "Demo", false); y < 33 {
			t.Errorf("the rail's first tile is at %v, in the bar", y)
		}
		x.clear(t, 1280)
		shot(t, x, "titlebar-compact")
	})
}

// pressed is the tab whose icon is pressed, "" for none.
func (x *fixture) pressed(t *testing.T) string {
	t.Helper()
	on := ""
	for _, name := range tabs {
		p, ok := kit.Pressed(x.tt, name)
		if !ok {
			t.Fatalf("no toggle %q in %q", name, x.tt.Texts())
		}
		if p {
			if on != "" {
				t.Errorf("%s and %s are both pressed", on, name)
			}
			on = name
		}
	}
	return on
}

// icons is where the tabs' icons start and end.
func (x *fixture) icons(t *testing.T) (start, end float32) {
	t.Helper()
	start, _, _, _ = box(t, x.tt, "Pages", false)
	lx, _, lw, _ := box(t, x.tt, "Links", false)
	return start, lx + lw
}

// In a wide window the icons open, switch and hide the right sidebar, at
// the bar's end before the window's buttons, where they stay as it hides;
// the sidebar names its tab, and a link to a file shows it again.
func TestTheTabIconsOpenSwitchAndHideTheSidebar(t *testing.T) {
	bothFiles(t, func(t *testing.T, x *fixture) {
		x.buttons(t, 1280)
		if x.pressed(t) != "Pages" || !kit.HasRole(x.tt, "heading", "Pages") {
			t.Errorf("the sidebar opens on %q, its title Pages: %v", x.pressed(t), kit.HasRole(x.tt, "heading", "Pages"))
		}
		if !kit.HasRole(x.tt, "group", "Project sidebar") {
			t.Error("the icons are not a group")
		}
		start, end := x.icons(t)
		if end > 1280-138-6 || end < 1280-138-10 || start < 1280-288 {
			t.Errorf("the icons run %v to %v, want inside the sidebar and 8 before the buttons at %v", start, end, 1280-138)
		}
		// The sidebar's title is right under the bar, with no tabs between.
		for _, a := range x.tt.AccessAll() {
			if a.Name == "Pages" && a.Bounds.Y > 56 && a.Bounds.Y > 56+24 {
				t.Errorf("the sidebar's title is at %v, not under the bar", a.Bounds.Y)
			}
		}
		// The chat list's head and the chat's header line up.
		_, hy, _, hh := box(t, x.tt, "Project menu", false)
		_, my, _, mh := box(t, x.tt, "Chat menu", false)
		if d := (hy + hh/2) - (my + mh/2); d > 1 || d < -1 {
			t.Errorf("the project's name is centered at %v, the chat menu at %v", hy+hh/2, my+mh/2)
		}
		shot(t, x, "titlebar-wide")

		// Another tab's icon switches the tab.
		x.click(t, "Links")
		if x.pressed(t) != "Links" || !kit.HasRole(x.tt, "heading", "Links") || !x.tt.HasText("No links yet") {
			t.Errorf("Links: %q pressed, %q", x.pressed(t), x.tt.Texts())
		}
		// The open tab's icon hides the sidebar; the icons stay put.
		x.click(t, "Links")
		if kit.HasRole(x.tt, "complementary", "Project") || !x.st.Shell.RightHidden {
			t.Fatal("the open tab's icon did not hide the sidebar")
		}
		if on := x.pressed(t); on != "" {
			t.Errorf("%s is pressed with the sidebar hidden", on)
		}
		if s2, e2 := x.icons(t); s2 != start || e2 != end {
			t.Errorf("the icons moved from %v–%v to %v–%v", start, end, s2, e2)
		}
		cx, _, cw, _ := box(t, x.tt, "Chat menu", false)
		if cx+cw > start {
			t.Errorf("the chat menu ends at %v, past the icons at %v", cx+cw, start)
		}
		x.clear(t, 1280)
		shot(t, x, "titlebar-hidden")

		// A tab's icon shows it again on that tab.
		x.click(t, "Pipelines")
		if x.pressed(t) != "Pipelines" || !kit.HasRole(x.tt, "complementary", "Project") || !x.tt.HasText("No pipelines yet") {
			t.Errorf("Pipelines did not show the sidebar: %q", x.tt.Texts())
		}

		// Hidden, a file opened from a jenab:// link shows it on Files.
		x.click(t, "Pipelines")
		x.click(t, "the reading notes")
		if !x.settle(func() bool { _, ok := x.docked(); return ok }) {
			t.Fatal("the link opened no file")
		}
		x.settle(nil)
		if x.pressed(t) != "Files" || x.st.Shell.RightHidden || !kit.HasRole(x.tt, "complementary", "Project") {
			t.Errorf("the link did not show Files: %q pressed, hidden %v", x.pressed(t), x.st.Shell.RightHidden)
		}
		x.clear(t, 1280)
	})
}

// With a file docked and the sidebar hidden, the icons end the bar at the
// window's end edge: the chat's on the end side, else the file's.
func TestTheTabIconsEndTheDockedBars(t *testing.T) {
	bothFiles(t, func(t *testing.T, x *fixture) {
		x.buttons(t, 1280)
		start, end := x.icons(t)
		x.st.Shell.SetWidth(p, 420)
		x.click(t, "the reading notes")
		if !x.settle(func() bool { _, ok := x.docked(); return ok }) {
			t.Fatal("the file did not open")
		}
		x.settle(func() bool { return x.st.Files.View(p, notesKey, x.st.Shell.Rev[p]).Loaded })
		x.click(t, "Files")
		if x.st.Shell.RightHidden != true {
			t.Fatal("Files did not hide the sidebar")
		}
		x.tt.Frame()
		for _, side := range []string{"the chat on the end side", "the chat on the start side", "the chat hidden"} {
			switch side {
			case "the chat on the start side":
				x.click(t, "Move the chat to the left")
			case "the chat hidden":
				x.click(t, "Hide the chat")
			}
			x.tt.Frame()
			if s2, e2 := x.icons(t); s2 != start || e2 != end {
				t.Errorf("%s: the icons run %v–%v, not %v–%v", side, s2, e2, start, end)
			}
			cx, _, cw, _ := box(t, x.tt, "Close the file", false)
			if cx+cw > start {
				t.Errorf("%s: Close the file ends at %v, past the icons", side, cx+cw)
			}
			x.clear(t, 1280)
		}
		shot(t, x, "titlebar-file")
	})
}

// Below 900 px the icons stay in the bar and open the right overlay on
// their tab; the overlay has them at the same place, and its open tab's
// icon closes it.
func TestTheTabIconsOpenTheNarrowOverlay(t *testing.T) {
	bothFiles(t, func(t *testing.T, x *fixture) {
		x.resize(800)
		x.buttons(t, 800)
		if on := x.pressed(t); on != "" {
			t.Errorf("%s is pressed with no overlay", on)
		}
		start, end := x.icons(t)
		if end > 800-138-6 || end < 800-138-10 {
			t.Errorf("the icons end at %v, want 8 before the buttons at %v", end, 800-138)
		}
		if _, y, _, _ := box(t, x.tt, "Open the sidebar", false); y > 20 {
			t.Errorf("☰ is at %v, not in the bar", y)
		}
		shot(t, x, "titlebar-narrow")
		x.click(t, "Files")
		if !kit.HasRole(x.tt, "dialog", "Pages, pipelines, files and links") || x.pressed(t) != "Files" || !x.tt.HasText("reading") && !kit.Shows(x.tt, "notes") {
			t.Fatalf("Files did not open the overlay on Files: %q", x.tt.Texts())
		}
		if s2, e2 := x.icons(t); s2 != start || e2 != end {
			t.Errorf("the overlay's icons run %v–%v, not %v–%v", s2, e2, start, end)
		}
		x.clear(t, 800)
		shot(t, x, "titlebar-narrow-overlay")
		x.click(t, "Pages")
		if !x.st.Shell.RightOverlay || x.pressed(t) != "Pages" {
			t.Error("Pages in the overlay did not switch the tab")
		}
		x.click(t, "Pages")
		if x.st.Shell.RightOverlay || kit.HasRole(x.tt, "dialog", "Pages, pipelines, files and links") {
			t.Error("the open tab's icon did not close the overlay")
		}
		// Wide again, the sidebar shows as it was.
		x.resize(1280)
		if !kit.HasRole(x.tt, "complementary", "Project") || x.pressed(t) != "Pages" {
			t.Error("the wide sidebar did not come back on Pages")
		}
	})
}

// Settings, project settings and the developer tools have the bar, its
// title at the start and nothing under the window's buttons.
func TestOtherPlacesHaveTheBar(t *testing.T) {
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) { otherPlaces(t, dark) })
	}
}

func otherPlaces(t *testing.T, dark bool) {
	{
		x := &fixture{f: statetest.New(), loop: &statetest.Loop{}}
		p, mother := x.f.AddProject("Demo")
		x.f.AddMessages(mother, statetest.Text(1, jc.RoleUser, "Track my reading"))
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
		x.buttons(t, 1280)
		for _, place := range []struct {
			place state.Place
			title string
		}{
			{state.Place{View: state.ViewSettings}, "Settings"},
			{state.Place{View: state.ViewProjectSettings, Project: p}, "Project settings"},
			{state.Place{View: state.ViewInspector}, "Developer tools"},
		} {
			x.st.Nav.Go(place.place)
			x.settle(nil)
			x.tt.Frame()
			inBar := false
			for _, a := range tt.AccessAll() {
				inBar = inBar || a.Name == place.title && a.Bounds.Y+a.Bounds.H < 56
			}
			if !inBar {
				t.Errorf("%s: no title in the bar", place.title)
			}
			x.clear(t, 1280)
			if kit.Shows(tt, "Pages") {
				t.Errorf("%s: the tabs' icons show", place.title)
			}
			shot(t, x, "titlebar-"+map[string]string{"Settings": "settings", "Project settings": "project-settings", "Developer tools": "dev"}[place.title])
		}
	}
}

// The shell's state: a tab's icon opens, switches and hides, wide and
// narrow.
func TestToggleRight(t *testing.T) {
	sh := state.NewShell()
	if !sh.RightOpen(state.TabPages, false) || sh.RightOpen(state.TabPages, true) {
		t.Fatal("the wide sidebar starts on Pages, the overlay closed")
	}
	sh.ToggleRight(state.TabFiles, false)
	if !sh.RightOpen(state.TabFiles, false) || sh.RightHidden {
		t.Error("Files did not switch")
	}
	sh.ToggleRight(state.TabFiles, false)
	if sh.RightOpen(state.TabFiles, false) || !sh.RightHidden || sh.RightTab != state.TabFiles {
		t.Error("Files again did not hide the sidebar")
	}
	sh.ToggleRight(state.TabLinks, true)
	if !sh.RightOverlay || sh.RightTab != state.TabLinks || !sh.RightHidden {
		t.Error("narrow, Links did not open the overlay alone")
	}
	sh.ToggleRight(state.TabLinks, true)
	if sh.RightOverlay {
		t.Error("narrow, Links again did not close the overlay")
	}
}
