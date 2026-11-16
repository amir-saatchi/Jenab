package settings_test

import (
	"testing"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/settings"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
	"github.com/amir-saatchi/jenab/internal/store"
)

// TASK-062: project settings in Settings' frame, with the project's
// memory, as settings-view.tsx and project-settings.tsx share
// settings-layout.tsx.

// withProjectMemory gives the project a memory and its settings the
// memory section, as cmd/desktop does.
func withProjectMemory(x *fixture) {
	x.f.SetAdmin(statetest.ProjectAdmin{Hosts: map[id.Project][]string{}, Memory: map[id.Project]app.MemoryView{
		x.project: {Scope: "project", Cap: 4000, Tokens: 40, Sections: []store.MemorySection{
			{Section: "goals", Content: "Track the gold price.", Revision: 1},
			{Section: "قیمت", Content: persian, Revision: 4},
		}},
	}})
	x.sh.Projects.ProjectMemory = settings.ProjectMemory(x.st)
}

// pageList is the frame's list of pages, as assistive technology gets it.
func pageList(t *testing.T, tt *kit.Tester) (width float32) {
	t.Helper()
	for _, a := range tt.AccessAll() {
		if a.Name == "Settings pages" {
			return a.Bounds.W
		}
	}
	t.Fatalf("no page list in %q", tt.Texts())
	return 0
}

func TestProjectSettingsShareSettingsFrame(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		withProjectMemory(x)
		settingsList := pageList(t, x.tt)
		x.click(t, "Project settings")
		x.settle(func() bool { _, ok := x.st.Hosts.ByProject[x.project]; return ok })
		// The same frame: Back to chats, the list as wide, a title and
		// a heading per section.
		if w := pageList(t, x.tt); w != settingsList {
			t.Errorf("the page list is %v wide, %v in Settings", w, settingsList)
		}
		shows(t, x.tt, "Back to chats", "Project: Demo", "Project settings · created", "Name", "Folder", "Approval level",
			"What every chat in this project knows: its goals, decisions and conventions. Keep values that change, such as prices, in tables.",
			"Track the gold price.", "قیمت", persian, "About 40 of 4,000 tokens")
		for _, h := range []string{"Demo", "General", "Memory", "Approvals", "Private-network exceptions", "Export"} {
			if !kit.HasRole(x.tt, "heading", h) {
				t.Errorf("no heading %q", h)
			}
		}
		// The list moves the page as Settings' does.
		x.click(t, "Export")
		if !kit.InView(x.tt, "group", "Export") {
			t.Error("Export is not in view")
		}
		x.click(t, "Memory")
		if !kit.InView(x.tt, "group", "Memory") {
			t.Error("Memory is not in view")
		}
		// A section of the project's memory saves to the project.
		x.click(t, "Edit goals")
		x.focus(t, "Text")
		x.tt.Key(0, kit.KeyEnd)
		x.tt.Type(" Weekly.")
		x.settle(nil)
		x.click(t, "Save")
		saves := x.f.Admin().MemSaves
		if len(saves) != 1 || saves[0].Section != "goals" || saves[0].Content != "Track the gold price. Weekly." || saves[0].Revision != 1 {
			t.Fatalf("saves = %+v", saves)
		}
		shows(t, x.tt, "Track the gold price. Weekly.", "revision 2")

		x.click(t, "Back to chats")
		if p := x.st.Nav.Current(); p == nil || p.View != state.ViewChat {
			t.Errorf("Back to chats went to %+v", p)
		}
	})
}

// A chat chip's [edit] on a project memory section opens the section in
// the project's settings, ready to edit.
func TestAProjectMemoryChipOpensItsSection(t *testing.T) {
	x := start(t, true, seed())
	withProjectMemory(x)
	x.st.OpenMemory(x.project, "memory:project/قیمت")
	x.settle(nil)
	x.settle(nil)
	if p := x.st.Nav.Current(); p.View != state.ViewProjectSettings || p.Section != "Memory" {
		t.Fatalf("the chip went to %+v", p)
	}
	if !kit.InView(x.tt, "group", "Memory") {
		t.Error("Memory is not in view")
	}
	shows(t, x.tt, "Delete section", "Text")
	if x.st.Shell.MemoryFocus != nil {
		t.Error("the chip's ask was not taken")
	}
	editing := false
	for _, a := range x.tt.AccessAll() {
		editing = editing || a.Name == "Text" && a.Value == persian
	}
	if !editing {
		t.Error("the editor is not on the section's text")
	}
}

// In a narrow window the page list is narrower, as settings-layout.tsx's
// w-44 below 900 px, in Settings and in a project's settings. The list
// is inside the panel's 1 px border.
func TestTheSettingsListNarrows(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		if w := pageList(t, x.tt); w != 224-1 {
			t.Errorf("the wide list is %v wide", w)
		}
		x.tt.SetSize(800, 800)
		x.settle(nil)
		x.tt.Frame()
		if w := pageList(t, x.tt); w != 176-1 {
			t.Errorf("the narrow list is %v wide", w)
		}
		x.click(t, "Project settings")
		if w := pageList(t, x.tt); w != 176-1 {
			t.Errorf("the project's narrow list is %v wide", w)
		}
	})
}
