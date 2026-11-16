package shell_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/native/chat"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/shell"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
	"github.com/amir-saatchi/jenab/internal/sysmem"
)

// The Files tab, a file in the main area with the chat docked, and the
// bottom bar (TASK-056): frontend/src/shell's files-panel, file-view,
// dock and bottom-bar tests, ported.

const (
	notesKey  = "notes/reading-notes.md"
	notesName = "یادداشت‌های کتاب.md" // the original name: keys are ASCII (SPEC 4.1)
	notesText = "# یادداشت‌ها\n\nفهرست کتاب‌هایی که امسال خواندم.\n\n- *Dune*\n- بوف کور"
	wordKey   = "report.docx"
	photoKey  = "photo.png"
)

var p = statetest.ProjectID

// startFiles is the shell on a project whose bucket has a Markdown note
// with a Persian name, a Word file and a picture. Its Mother chat is open,
// its answer linking to the note.
func startFiles(t *testing.T, dark bool) *fixture {
	t.Helper()
	x := &fixture{f: statetest.New(), loop: &statetest.Loop{}}
	mother := x.f.AddProjectAs(p, "Demo")
	x.f.AddFile(p, notesKey, statetest.File{Data: []byte(notesText), MIME: "text/markdown", Title: notesName, Versions: []int64{12}})
	x.f.AddFile(p, wordKey, statetest.File{Data: make([]byte, 24_000), Title: "گزارش سالانه",
		MIME: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", Opens: true})
	x.f.AddFile(p, photoKey, statetest.File{Data: picture(t), MIME: "image/png", Opens: true})
	x.f.AddMessages(mother, statetest.Text(1, jc.RoleUser, "Where are my notes?"),
		statetest.Text(1, jc.RoleAssistant, "They are in [the reading notes]("+state.LinkOf(p, notesKey)+")."),
		statetest.Text(2, jc.RoleUser, "And this one: "+state.LinkOf(p, wordKey)))
	g := uint64(1 << 30)
	x.f.SetMemory(sysmem.Reading{Free: 5 * g, Total: 29 * g, RAM: 16 * g, Level: sysmem.OK})
	x.f.Bucket.Paused = []app.ProviderStatus{{Name: "Z.ai", PausedMS: 29_100, LastProblem: "Too many requests"}}
	x.st = state.NewStores(x.f.API(), x.loop.Post)
	t.Cleanup(x.st.Close)
	x.sh = shell.New(x.st, chat.New(x.st), x.loop.Post)
	x.sh.Start()
	x.settle(func() bool { return x.st.Projects.Loaded })
	tt, err := kit.NewTester(x.sh.View, 1280, 800, dark)
	if err != nil {
		t.Fatal(err)
	}
	x.tt = tt
	if err := tt.Click("Demo"); err != nil {
		t.Fatal(err)
	}
	if !x.settle(func() bool { pl := x.st.Nav.Current(); return pl != nil && x.st.Threads.Threads[pl.Chat] != nil }) {
		t.Fatalf("the project did not open: %q", tt.Texts())
	}
	x.settle(nil)
	return x
}

func picture(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 320, 200))
	for y := range 200 {
		for x := range 320 {
			img.Set(x, y, color.RGBA{uint8(x * 255 / 320), 120, uint8(y * 255 / 200), 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func bothFiles(t *testing.T, test func(t *testing.T, x *fixture)) {
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) { test(t, startFiles(t, dark)) })
	}
}

// shot saves the window as name-light.png or name-dark.png in the folder
// JENAB_SHOTS names, for the ticket's report.
func shot(t *testing.T, x *fixture, name string) {
	dir := os.Getenv("JENAB_SHOTS")
	if dir == "" {
		return
	}
	// The pointer leaves for the bottom bar, its tooltips closing.
	x.tt.Move(600, 790)
	x.tt.Frame()
	x.tt.Advance(time.Second)
	theme := "light"
	if filepath.Base(t.Name()) == "dark" {
		theme = "dark"
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, x.tt.Image()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+"-"+theme+".png"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (x *fixture) click(t *testing.T, s string) {
	t.Helper()
	if err := x.tt.Click(s); err != nil {
		t.Fatalf("%v; shown: %q", err, x.tt.Texts())
	}
	x.settle(nil)
}

func (x *fixture) docked() (state.Docked, bool) {
	pl := x.st.Nav.Current()
	d, ok := x.st.Shell.Open[state.ChatRef{Project: pl.Project, Chat: pl.Chat}]
	return d, ok
}

func (x *fixture) filesTab(t *testing.T) {
	t.Helper()
	x.click(t, "Files")
	if !x.settle(func() bool { v := x.st.Files.Folder(p, "", x.st.Shell.Rev[p]); return v.Folder != nil }) {
		t.Fatal("the files did not load")
	}
	x.settle(nil)
}

func TestTheFilesTabOpensAFileNextToTheChat(t *testing.T) {
	bothFiles(t, func(t *testing.T, x *fixture) {
		if !kit.HasRole(x.tt, "complementary", "Project") {
			t.Fatal("no right sidebar beside the open chat")
		}
		shows(t, x.tt, "No pages yet")
		x.filesTab(t)
		shows(t, x.tt, "notes", wordKey, "گزارش سالانه", photoKey)
		if !kit.HasRole(x.tt, "tree", "Files") {
			t.Error("the files are not a tree")
		}
		shot(t, x, "files-tab")
		// A folder opens in place; the path bar goes back.
		x.click(t, "notes")
		x.settle(func() bool { return x.st.Files.Folder(p, "notes/", x.st.Shell.Rev[p]).Folder != nil })
		x.settle(nil)
		shows(t, x.tt, "reading-notes.md", notesName)
		if x.tt.HasText("گزارش سالانه") {
			t.Error("the top's files still show inside notes/")
		}
		// A file opens in the main area, the chat docked beside it.
		x.click(t, "reading-notes.md")
		if d, ok := x.docked(); !ok || d.Key != notesKey {
			t.Fatalf("docked %+v %v", d, ok)
		}
		x.settle(func() bool { v := x.st.Files.View(p, notesKey, x.st.Shell.Rev[p]); return v.Loaded })
		x.settle(nil)
		shows(t, x.tt, "یادداشت‌ها", "بوف کور", "Versions", "Version 2", "Current", "Where are my notes?")
		if !kit.HasRole(x.tt, "group", "The file and the chat") {
			t.Error("no split between the file and the chat")
		}
		if x.st.ListHidden() && kit.HasRole(x.tt, "navigation", "Chats") {
			t.Error("the chat list shows while hidden")
		}
		shot(t, x, "file-docked")
		// Copy link, swap, hide and show the chat, close.
		x.click(t, "Copy link")
		if got, want := x.tt.Clipboard(), state.LinkOf(p, notesKey); got != want {
			t.Errorf("copied %q, want %q", got, want)
		}
		x.click(t, "Move the chat to the left")
		if x.st.Shell.Side[p] != state.DockLeft {
			t.Error("the chat did not move left")
		}
		x.click(t, "Move the chat to the right")
		x.click(t, "Hide the chat")
		if x.tt.HasText("Where are my notes?") {
			t.Error("the chat still shows")
		}
		x.click(t, "Show chat")
		shows(t, x.tt, "Where are my notes?")
		x.click(t, "Close the file")
		if _, ok := x.docked(); ok || x.tt.HasText("بوف کور") {
			t.Error("the file did not close")
		}
	})
}

func TestALinkInAnAnswerOpensTheFile(t *testing.T) {
	bothFiles(t, func(t *testing.T, x *fixture) {
		x.click(t, "the reading notes")
		if !x.settle(func() bool { _, ok := x.docked(); return ok }) {
			t.Fatalf("the link opened nothing; opened URLs %q", x.tt.OpenedURLs())
		}
		if d, _ := x.docked(); d.Key != notesKey {
			t.Errorf("docked %q", d.Key)
		}
		if len(x.tt.OpenedURLs()) != 0 {
			t.Errorf("the link went to the browser: %q", x.tt.OpenedURLs())
		}
		if x.st.Shell.RightTab != state.TabFiles {
			t.Error("the right sidebar did not switch to Files")
		}
	})
}

func TestAFileIsDeletedAndPutBack(t *testing.T) {
	bothFiles(t, func(t *testing.T, x *fixture) {
		x.filesTab(t)
		if err := x.tt.RightClick(photoKey); err != nil {
			t.Fatal(err)
		}
		x.settle(nil)
		if err := x.tt.ChooseMenuItem("Delete"); err != nil {
			t.Fatalf("%v; menu %q", err, x.tt.Menu())
		}
		// No time passes until Undo: the toast closes after a few seconds.
		x.loop.Drain(func() bool { return len(x.f.Bucket.Deleted) == 1 })
		x.tt.Frame()
		if !x.loop.Drain(func() bool { return x.st.Files.Folder(p, "", x.st.Shell.Rev[p]).Folder != nil }) {
			t.Fatal("the folder did not reload")
		}
		x.tt.Frame()
		shows(t, x.tt, "Deleted "+photoKey)
		listed := x.st.Files.Folder(p, "", x.st.Shell.Rev[p]).Folder.Objects
		if slices.ContainsFunc(listed, func(o app.Object) bool { return o.Key == photoKey }) {
			t.Error("the deleted file is still listed")
		}
		shot(t, x, "deleted")
		x.click(t, "Undo")
		x.settle(func() bool { return len(x.f.Bucket.Deleted) == 0 })
		x.settle(func() bool { return x.st.Files.Folder(p, "", x.st.Shell.Rev[p]).Folder != nil })
		x.settle(nil)
		shows(t, x.tt, photoKey+" is back")
	})
}

func TestTheWordFileAndThePicture(t *testing.T) {
	bothFiles(t, func(t *testing.T, x *fixture) {
		x.filesTab(t)
		x.click(t, wordKey)
		x.settle(func() bool { return x.st.Files.View(p, wordKey, x.st.Shell.Rev[p]).Object != nil })
		x.settle(nil)
		shows(t, x.tt, "application/vnd.openxmlformats-officedocument.wordprocessingml.document · 23 KB")
		shot(t, x, "word-file")
		x.click(t, "Open with the system app")
		if !x.settle(func() bool { return slices.Contains(x.f.OpenedWithSystem(), wordKey) }) {
			t.Error("the Word file did not open with the system app")
		}
		x.click(t, photoKey)
		x.settle(func() bool { return x.st.Files.View(p, photoKey, x.st.Shell.Rev[p]).Loaded })
		x.settle(nil)
		if !kit.HasRole(x.tt, "image", photoKey) {
			t.Errorf("the picture is not shown: %q", x.tt.Texts())
		}
		shot(t, x, "picture")
	})
}

func TestTheBottomBar(t *testing.T) {
	bothFiles(t, func(t *testing.T, x *fixture) {
		x.settle(func() bool { return x.st.Bar.Memory != nil && len(x.st.Bar.Paused()) == 1 })
		x.settle(nil)
		shows(t, x.tt, "Memory 5.0 GB free", "Z.ai paused, retry in 30 s")
		if !kit.HasRole(x.tt, "contentinfo", "Status") {
			t.Error("the bar has no role")
		}
	})
}

func TestABareLinkInTheUsersMessageOpensTheFile(t *testing.T) {
	bothFiles(t, func(t *testing.T, x *fixture) {
		x.click(t, state.LinkOf(p, wordKey))
		if !x.settle(func() bool { d, ok := x.docked(); return ok && d.Key == wordKey }) {
			t.Fatalf("the link opened nothing: %q", x.tt.Texts())
		}
		if len(x.tt.OpenedURLs()) != 0 {
			t.Errorf("the link went to the browser: %q", x.tt.OpenedURLs())
		}
	})
}
