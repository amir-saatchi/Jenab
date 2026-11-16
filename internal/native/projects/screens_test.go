package projects_test

// The project screens in the shell on fake services (TASK-057): new
// project, the welcome page, project settings, the exports and the chat
// menu, each in a light and a dark window. With JENAB_SHOTS set to a
// folder, the tests save what they show there as PNG files.

import (
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/chatexport"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/chat"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/projects"
	"github.com/amir-saatchi/jenab/internal/native/shell"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
	"github.com/amir-saatchi/jenab/internal/project"
)

const persian = "یادداشت‌های قیمت"

type fixture struct {
	t      *testing.T
	f      *statetest.Fake
	loop   *statetest.Loop
	st     *state.Stores
	sh     *shell.Shell
	tt     *kit.Tester
	dark   bool
	p      id.Project
	mother id.Chat
	notes  id.Chat
	saves  []state.SaveRequest
	picked string
}

func start(t *testing.T, dark bool, admin statetest.ProjectAdmin) *fixture {
	t.Helper()
	x := &fixture{t: t, f: statetest.New(), loop: &statetest.Loop{}, dark: dark}
	x.p, x.mother = x.f.AddProject("Demo")
	x.notes = x.f.AddChat(x.p, persian)
	x.f.AddMessages(x.mother, statetest.Text(1, jc.RoleUser, "Track my reading"), statetest.Text(1, jc.RoleAssistant, "I made a **books** table."))
	x.f.SetAdmin(admin)
	x.st = state.NewStores(x.f.API(), x.loop.Post)
	t.Cleanup(x.st.Close)
	x.st.Exports.Save = func(r state.SaveRequest) (string, error) {
		x.saves = append(x.saves, r) // read once the loop has run the call's end
		return x.picked, nil
	}
	x.sh = shell.New(x.st, chat.New(x.st), x.loop.Post)
	x.sh.Now = func() time.Time { return time.Date(2026, 11, 16, 15, 0, 0, 0, time.UTC) }
	x.sh.Start()
	x.settle(func() bool { return x.st.Projects.Loaded })
	tt, err := kit.NewTester(x.sh.View, 1280, 800, dark)
	if err != nil {
		t.Fatal(err)
	}
	x.tt = tt
	x.settle(nil)
	return x
}

// until runs the stores' work, drawing, until the window shows text.
func (x *fixture) until(text string) bool {
	return x.settle(func() bool {
		x.tt.Frame()
		return kit.Shows(x.tt, text)
	})
}

// settle runs the stores' work until cond holds, then draws.
func (x *fixture) settle(cond func() bool) bool {
	ok := x.loop.Drain(cond)
	if x.tt != nil {
		x.tt.Frame()
		x.tt.Advance(time.Second)
		x.tt.Frame()
	}
	return ok
}

func (x *fixture) click(label string) {
	x.t.Helper()
	if err := x.tt.Click(label); err != nil {
		x.t.Fatalf("click %q: %v in %q", label, err, x.tt.Texts())
	}
	x.settle(nil)
}

func (x *fixture) choose(button, item string) {
	x.t.Helper()
	x.click(button)
	if err := x.tt.ChooseMenuItem(item); err != nil {
		x.t.Fatalf("choose %q in %q: %v (menu %q)", item, button, err, x.tt.Menu())
	}
	x.settle(nil)
}

func (x *fixture) shows(texts ...string) {
	x.t.Helper()
	for _, s := range texts {
		if !kit.Shows(x.tt, s) {
			x.t.Errorf("no %q in %q", s, x.tt.Texts())
		}
	}
}

func (x *fixture) hides(texts ...string) {
	x.t.Helper()
	for _, s := range texts {
		if kit.Shows(x.tt, s) {
			x.t.Errorf("%q still shows", s)
		}
	}
}

// openDemo opens the Demo project at its Mother chat.
func (x *fixture) openDemo() {
	x.t.Helper()
	x.click("Demo")
	if !x.settle(func() bool {
		pl := x.st.Nav.Current()
		_, listed := x.st.Chats.ByProject[x.p]
		return pl != nil && x.st.Threads.Threads[pl.Chat] != nil && listed
	}) {
		x.t.Fatalf("the project did not open: %q", x.tt.Texts())
	}
	x.settle(nil)
}

// shot saves the window as name in JENAB_SHOTS, if set.
func (x *fixture) shot(name string) {
	dir := os.Getenv("JENAB_SHOTS")
	if dir == "" {
		return
	}
	theme := "light"
	if x.dark {
		theme = "dark"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		x.t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%s.png", name, theme)))
	if err != nil {
		x.t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, x.tt.Image()); err != nil {
		x.t.Fatal(err)
	}
}

func both(t *testing.T, admin statetest.ProjectAdmin, test func(t *testing.T, x *fixture)) {
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) { test(t, start(t, dark, admin)) })
	}
}

func TestTheWelcomePage(t *testing.T) {
	both(t, statetest.ProjectAdmin{}, func(t *testing.T, x *fixture) {
		x.shows("Welcome to Jenab", "Each project is one folder with its data, chats and files.", "Create project", "Open the example", "Saved in", "~/Jenab", "change in Settings")
		x.shot("welcome")
		if a, ok := x.tt.Access("Open the example"); !ok || !a.Disabled {
			t.Errorf("Open the example is not disabled: %+v", a)
		}
		x.click("change in Settings")
		if pl := x.st.Nav.Current(); pl == nil || pl.View != state.ViewSettings {
			t.Errorf("the place is %+v", pl)
		}
	})
}

// folder-warning.tsx: the 2.1 warning shows on the welcome page and in
// project settings' General, and not when the folder is fine.
func TestTheFolderWarning(t *testing.T) {
	const text = "This folder is synced by OneDrive. SQLite files can be damaged there."
	both(t, statetest.ProjectAdmin{Hosts: map[id.Project][]string{}}, func(t *testing.T, x *fixture) {
		x.settle(nil)
		if kit.Shows(x.tt, text) {
			t.Error("a warning with no warning")
		}
		x.st.Projects.FolderWarning = &project.FolderWarning{Folder: `C:\OneDrive\Jenab`, Service: "OneDrive", Text: text}
		x.until(text)
		x.shows("Welcome to Jenab", text)
		x.openDemo()
		x.choose("Project menu", "Project settings")
		x.until("Project settings · created")
		x.shows(text)
	})
}

func TestNewProjectOpensItsMotherChat(t *testing.T) {
	both(t, statetest.ProjectAdmin{}, func(t *testing.T, x *fixture) {
		x.click("Create project")
		x.shows("New project", "A project is one folder with its data, chats and files.", "Name", "Cancel", "Create")
		if !x.tt.Focused("Name") {
			t.Error("the name has no focus")
		}
		if a, ok := x.tt.Access("Create"); !ok || !a.Disabled {
			t.Error("Create is enabled with no name")
		}
		x.tt.Type("قیمت طلا")
		x.settle(nil)
		x.shot("new-project")
		x.tt.Key(0, kit.KeyEnter)
		if !x.settle(func() bool {
			pl := x.st.Nav.Current()
			return pl != nil && pl.View == state.ViewChat && pl.Project != x.p
		}) {
			t.Fatalf("the new project did not open: %q", x.tt.Texts())
		}
		x.settle(nil)
		pl := x.st.Nav.Current()
		if pl.Chat != x.st.Projects.Opened[pl.Project].Mother {
			t.Errorf("opened %s, not the Mother chat", pl.Chat)
		}
		x.hides("A project is one folder with its data, chats and files.")
		x.shows("قیمت طلا", "Mother")
		if x.st.Shell.NewProject {
			t.Error("the dialog is still open")
		}
	})
}

func TestNewProjectShowsARefusedName(t *testing.T) {
	refused := &app.UIError{Kind: app.KindInvalid, Message: "A project with that name exists."}
	both(t, statetest.ProjectAdmin{CreateErr: refused}, func(t *testing.T, x *fixture) {
		x.click("New project")
		x.tt.Type("Demo")
		x.settle(nil)
		x.click("Create")
		x.until("A project with that name exists.")
		x.shows("A project with that name exists.")
		x.shot("new-project-refused")
		if !x.st.Shell.NewProject {
			t.Fatal("the dialog closed")
		}
		// Typing clears the problem; Cancel closes.
		x.click("Name")
		x.tt.Type("2")
		x.settle(nil)
		x.hides("A project with that name exists.")
		x.click("Cancel")
		x.hides("A project is one folder with its data, chats and files.")
	})
	// Other errors go to a toast.
	x := start(t, false, statetest.ProjectAdmin{CreateErr: errors.New("disk on fire")})
	x.st.Shell.NewProject = true
	x.settle(nil)
	x.tt.Type("Demo")
	x.tt.Key(0, kit.KeyEnter)
	x.until("disk on fire")
	x.shows("disk on fire")
}

func TestProjectSettings(t *testing.T) {
	both(t, statetest.ProjectAdmin{Hosts: map[id.Project][]string{}}, func(t *testing.T, x *fixture) {
		x.openDemo()
		x.choose("Project menu", "Project settings")
		x.settle(func() bool { _, ok := x.st.Hosts.ByProject[x.p]; return ok })
		x.shows("Back to chats", "Project: Demo", "General", "Approvals", "Private-network exceptions", "Export",
			"Project settings · created", "Name", "Folder", `C:\Projects\Demo`,
			"How much the agent asks before it acts. A change applies from the next tool call.",
			"Approval level", "Strict", "Standard", "Auto", "Add a host", state.HostAdvice)
		if !kit.HasRole(x.tt, "heading", "Demo") {
			t.Error("the page has no title")
		}
		x.shot("project-settings")

		// The approval level.
		x.click("Strict")
		x.settle(func() bool { return x.st.Projects.Opened[x.p].Level == "strict" })
		if !slices.Equal(x.f.Levels, []string{"strict"}) {
			t.Errorf("levels set %q", x.f.Levels)
		}

		// A host with a port is caught before the call; a good one adds.
		x.click("Add a host")
		x.tt.Type("127.0.0.1:8099")
		x.tt.Key(0, kit.KeyEnter)
		x.settle(nil)
		x.shows("Give the host without a port.")
		x.hides(state.HostAdvice)
		for range len("127.0.0.1:8099") {
			x.tt.Key(0, kit.KeyBackspace)
		}
		x.tt.Type("nas.local")
		x.settle(nil)
		x.hides("Give the host without a port.")
		x.click("Add")
		x.settle(func() bool { return slices.Contains(x.st.Hosts.ByProject[x.p], "nas.local") })
		x.shows("nas.local", "Remove nas.local")
		if got := x.f.Admin().Hosts[x.p]; !slices.Equal(got, []string{"nas.local"}) {
			t.Errorf("stored %q", got)
		}
		x.shot("project-settings-hosts")
		x.click("Remove nas.local")
		x.settle(func() bool { return len(x.st.Hosts.ByProject[x.p]) == 0 })
		x.hides("Remove nas.local")

		x.click("Back to chats")
		if pl := x.st.Nav.Current(); pl == nil || pl.View != state.ViewChat || pl.Chat != x.mother {
			t.Errorf("back at %+v", pl)
		}
	})
}

func TestARefusedHostShowsUnderTheField(t *testing.T) {
	x := start(t, false, statetest.ProjectAdmin{HostsErr: &app.UIError{Kind: app.KindInvalid, Message: `"example.com" is not on this computer or your network.`}})
	x.openDemo()
	x.choose("Project menu", "Project settings")
	x.settle(func() bool { _, ok := x.st.Hosts.ByProject[x.p]; return ok })
	x.click("Add a host")
	x.tt.Type("example.com")
	x.tt.Key(0, kit.KeyEnter)
	x.until("is not on this computer")
	x.shows(`"example.com" is not on this computer or your network.`)
}

func TestTheSettingsPageListRevealsASection(t *testing.T) {
	x := start(t, false, statetest.ProjectAdmin{})
	x.openDemo()
	x.st.Nav.Go(state.Place{View: state.ViewProjectSettings, Project: x.p, Section: "Export"})
	x.settle(nil)
	if !kit.InView(x.tt, "group", "Export") {
		t.Error("Export is not in view")
	}
	x.click("General")
	x.tt.Advance(time.Second)
	x.settle(nil)
	if !kit.InView(x.tt, "group", "General") {
		t.Error("General is not in view")
	}
}

func TestExportAProject(t *testing.T) {
	both(t, statetest.ProjectAdmin{Estimate: project.ExportEstimate{Bytes: 2_500_000, Objects: 3}, Gate: make(chan struct{})}, func(t *testing.T, x *fixture) {
		x.openDemo()
		x.picked = `C:\Backups\Demo`
		x.choose("Project menu", "Export…")
		x.until("About 2.5 MB")
		x.shows("Export project", "A copy of Demo as it is now: its data, chats and files. You can keep working while it runs.",
			"Zip file", "Folder", "Include approvals (for my own backup)", "About 2.5 MB before compression, with 3 files.", "Cancel", "Export…")
		x.shot("export-project")
		x.click("Folder")
		x.click("Include approvals (for my own backup)")
		x.click("Export…")
		x.settle(func() bool { return len(x.f.Admin().Exports) == 1 })
		x.shows("Starting…", "Stop")
		if a, ok := x.tt.Access("Export progress"); !ok {
			t.Errorf("no progress bar: %+v", a)
		}
		// Escape leaves the dialog open while it runs.
		x.tt.Key(0, kit.KeyEscape)
		x.settle(nil)
		x.shows("Stop")
		x.st.Apply(app.EventExport, project.ExportProgress{Project: x.p, Step: project.StepChats, Done: 500_000, Total: 2_000_000})
		x.settle(nil)
		x.shows("Copying the chats…", "500 KB / 2 MB")
		x.shot("export-project-running")
		x.f.Admin().Gate <- struct{}{}
		x.until("Exported")
		x.shows("Exported 2.5 MB in 1.5 s.", `C:\Backups\Demo`, "Copied into the projects folder, the export opens as a new project next to this one.", "Done")
		x.shot("export-project-done")
		got := x.f.Admin().Exports[0]
		if got != (app.ExportRequest{Target: `C:\Backups\Demo`, Format: project.ExportFolder, IncludeApprovals: true}) {
			t.Errorf("asked %+v", got)
		}
		if len(x.saves) != 1 || x.saves[0] != (state.SaveRequest{Title: "Export project", Name: "Demo"}) {
			t.Errorf("the save dialog showed %+v", x.saves)
		}
		x.click("Done")
		x.hides("Export project")
		if len(x.st.Exports.Projects) != 0 {
			t.Error("the closed dialog still takes events")
		}
	})
}

func TestStopAProjectExport(t *testing.T) {
	x := start(t, false, statetest.ProjectAdmin{Estimate: project.ExportEstimate{Bytes: 10, Objects: 1}, Gate: make(chan struct{})})
	x.openDemo()
	x.picked = `C:\Backups\Demo`
	x.choose("Project menu", "Export…")
	x.until("About 10 B")
	x.shows("with 1 file.")
	x.click("Export…")
	x.until("Stop")
	x.click("Stop")
	x.until(state.StoppedText)
	x.shows(state.StoppedText, "Zip file", "Export…")
	if x.saves[0].Ext != "zip" || x.f.Admin().Exports[0].Target != `C:\Backups\Demo.zip` {
		t.Errorf("saved %+v, exported %+v", x.saves[0], x.f.Admin().Exports[0])
	}
}

func TestExportAChat(t *testing.T) {
	both(t, statetest.ProjectAdmin{}, func(t *testing.T, x *fixture) {
		x.openDemo()
		x.picked = `C:\Backups\mother`
		x.choose("Chat menu", "Export chat…")
		x.shows("Export chat", "Save Mother as a file. Keys are replaced with their names.", "Markdown", "JSON",
			projects.FormatText(chatexport.FormatMarkdown))
		x.shot("export-chat")
		x.click("JSON")
		x.shows(projects.FormatText(chatexport.FormatJSON))
		x.click("Export…")
		x.until("Exported")
		x.shows("Exported 2 messages, 1.2 KB.", `C:\Backups\mother.json`, "Done")
		x.shot("export-chat-done")
		if got := x.f.Admin().ChatExports; len(got) != 1 || got[0] != (app.ChatExportRequest{Target: `C:\Backups\mother.json`, Format: chatexport.FormatJSON}) {
			t.Errorf("asked %+v", got)
		}
		x.click("Done")
		x.hides("Export chat")
	})
}

func TestTheChatRowMenuExportsItsChat(t *testing.T) {
	both(t, statetest.ProjectAdmin{}, func(t *testing.T, x *fixture) {
		x.openDemo()
		x.picked = `C:\Backups\notes`
		x.choose("Chat menu: "+persian, "Export chat…")
		x.shows("Save " + persian + " as a file. Keys are replaced with their names.")
		x.shot("chat-row-menu")
		x.click("Export…")
		x.until("Exported")
		x.shows("Exported 0 messages, 1.2 KB.", `C:\Backups\notes.md`)
		if x.saves[0].Name != persian+".md" {
			t.Errorf("the save dialog suggested %q", x.saves[0].Name)
		}
		// Cancel closes it before it runs.
		x.click("Done")
		x.choose("Chat menu: "+persian, "Export chat…")
		x.click("Cancel")
		x.hides("Export chat")
	})
}

func TestTheRailShowsTheChatsWhileTheListIsHidden(t *testing.T) {
	both(t, statetest.ProjectAdmin{}, func(t *testing.T, x *fixture) {
		x.openDemo()
		if a, ok := x.tt.Access("Search"); !ok || !a.Disabled {
			t.Errorf("Search is not there disabled: %+v", a)
		}
		x.click("Hide the chat list")
		x.shows("Mother", persian, "New chat")
		x.shot("rail-chats")
		x.click(persian)
		x.settle(func() bool { return x.st.Nav.Current().Chat == x.notes })
		if x.st.Nav.Current().Chat != x.notes {
			t.Errorf("opened %s", x.st.Nav.Current().Chat)
		}
		// The list is hidden per chat: the notes chat has it.
		if !kit.HasRole(x.tt, "navigation", "Chats") {
			t.Error("the notes chat has no chat list")
		}
	})
}
