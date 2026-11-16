package settings_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/chat"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/settings"
	"github.com/amir-saatchi/jenab/internal/native/shell"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/version"
)

const persian = "سلام، این یک آزمایش است"

// fixture is the shell on fake services, with Settings open on a project
// whose chat was used last.
type fixture struct {
	f       *statetest.Fake
	set     *statetest.Settings
	loop    *statetest.Loop
	st      *state.Stores
	sh      *shell.Shell
	scr     *settings.Screen
	tt      *kit.Tester
	project id.Project
}

// seed is the settings services as a user with one provider has them.
func seed() *statetest.Settings {
	s := statetest.NewSettings()
	set := &s.View.Settings
	set.LLM.Providers = map[string]config.ProviderSettings{
		"gemini": {Kind: "gemini", Models: []config.ModelSettings{{ID: "gemini-3.5-flash-lite"}, {ID: "gemini-3.5-pro"}}},
		"local":  {Kind: "ollama", BaseURL: "http://localhost:11434/v1/", Models: []config.ModelSettings{{ID: "qwen3:8b", Context: 32000}}},
	}
	set.LLM.Models = map[string]string{"default": "gemini/gemini-3.5-flash-lite", "fast": "gemini/gemini-3.5-flash-lite"}
	s.Groups = []app.ModelGroup{
		{Provider: "gemini", Kind: "gemini", Models: []app.ModelItem{
			{Ref: "gemini/gemini-3.5-flash-lite", Name: "Gemini 3.5 Flash-Lite", Aliases: []string{"default", "fast"}},
			{Ref: "gemini/gemini-3.5-pro", Name: "Gemini 3.5 Pro"}}},
		{Provider: "local", Kind: "ollama", Models: []app.ModelItem{{Ref: "local/qwen3:8b", Name: "qwen3:8b"}}},
	}
	s.Start = app.Started{DataFolder: `D:\Jenab`, DefaultDataFolder: `D:\Jenab`}
	s.Build = version.Info{Version: "0.9.0", Commit: "abc1234", Go: "go1.26.0"}
	s.PresetList = []app.PresetItem{
		{ID: "anthropic", Name: "Anthropic", Kind: "anthropic", Group: "cloud", Host: "api.anthropic.com"},
		{ID: "gemini", Name: "Google Gemini", Kind: "gemini", Group: "cloud", Host: "generativelanguage.googleapis.com"},
	}
	s.Listed["gemini"] = app.ProviderModels{Provider: "gemini", Kind: "gemini",
		Models: []app.ModelOption{
			{ID: "gemini-3.5-flash-lite", Name: "Gemini 3.5 Flash-Lite", On: true, Context: 1048576, Known: true},
			{ID: "gemini-3.5-pro", Name: "Gemini 3.5 Pro", On: true, Context: 1048576, Known: true},
			{ID: "gemini-3.5-flash", Name: "Gemini 3.5 Flash", Context: 1048576, Known: true},
		},
		Other: []app.ModelOption{{ID: "gemma-4-27b", Name: "Gemma 4 27B"}},
	}
	s.Memory.Sections = []store.MemorySection{
		{Section: "preferences", Content: "Short answers, please.", Revision: 2},
		{Section: "goals", Content: persian, Revision: 1},
	}
	s.Memory.Tokens = 120
	var days []app.UsageLine
	start := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	for i := range 90 {
		days = append(days, app.UsageLine{Day: start.AddDate(0, 0, i).Format("2006-01-02"), Input: 1000 * (i % 7), Output: 300 * (i % 5)})
	}
	s.Report = app.UsageReport{Days: days,
		Models: []app.UsageLine{{Model: "Gemini 3.5 Flash-Lite", Provider: "gemini", Input: 52000, CacheRead: 8000, Output: 12000, Cost: 0.0123}},
		Chats:  []app.UsageLine{{Chat: persian, Project: "Demo", Input: 52000, Output: 12000, Cost: 0.0123}, {Project: "Demo", Input: 10}},
		Total:  app.UsageLine{Input: 52010, CacheRead: 8000, Output: 12000, Cost: 0.0123},
	}
	s.Desk = app.DesktopList{Apps: []app.DesktopAppItem{{App: "calculatorapp",
		Macros: []app.DesktopMacroItem{{Macro: store.DesktopMacro{ID: 7, App: "calculatorapp", Name: "add", Params: []string{"a", "b"},
			Description: "Adds two numbers", Version: 2, Runs: 5, Failures: 1}, Listing: "1. click {a}\n2. type +", Steps: "[]",
			Runs: []store.DesktopRun{{OK: true, Seconds: 2.4, At: time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)}}}},
		Recipes: []store.DesktopRecipe{{ID: 9, App: "calculatorapp", Name: "memory_keys", Text: "Use MS and MR.", Source: "agent", Uses: 3}},
	}}}
	return s
}

func start(t *testing.T, dark bool, s *statetest.Settings) *fixture {
	t.Helper()
	x := &fixture{f: statetest.New(), set: s, loop: &statetest.Loop{}}
	p, mother := x.f.AddProject("Demo")
	x.project = p
	x.st = state.NewStores(s.Use(x.f.API()), x.loop.Post)
	t.Cleanup(x.st.Close)
	x.sh = shell.New(x.st, chat.New(x.st), x.loop.Post)
	x.scr = settings.New(x.st, x.loop.Post)
	x.scr.Windows = true
	x.sh.Settings = x.scr
	x.sh.Start()
	x.settle(func() bool { return x.st.Projects.Loaded })
	x.st.Nav.Go(state.Place{View: state.ViewChat, Project: p, Chat: mother})
	tt, err := kit.NewTester(x.sh.View, 1280, 800, dark)
	if err != nil {
		t.Fatal(err)
	}
	x.tt = tt
	x.open(state.Place{View: state.ViewSettings})
	return x
}

// open goes to place and waits for the pages to be read.
func (x *fixture) open(place state.Place) {
	x.st.Nav.Go(place)
	x.tt.Frame()
	x.settle(func() bool {
		p := x.st.Pages
		return x.st.Settings.View != nil && x.st.Settings.Started != nil && p.Usage != nil && p.Desktop != nil && p.Version != nil
	})
	x.settle(nil)
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

// focus tabs to the element named name.
func (x *fixture) focus(t *testing.T, name string) {
	t.Helper()
	for range 200 {
		if focused(x.tt) == name {
			return
		}
		x.tt.Key(0, kit.KeyTab)
	}
	t.Fatalf("Tab never reached %q", name)
}

// enter replaces the focused field's text and presses Enter.
func (x *fixture) enter(text string) {
	x.tt.Key(kit.Ctrl, kit.KeyA)
	x.tt.Type(text)
	x.tt.Key(0, kit.KeyEnter)
	x.settle(nil)
}

// reveal scrolls the settings page until label is in the window.
func (x *fixture) reveal(label string) {
	for range 60 {
		r, ok := x.tt.Find(label)
		if !ok || (r.Y >= 40 && r.Y+r.H <= 760) {
			return
		}
		dy := float32(240)
		if r.Y < 40 {
			dy = -240
		}
		x.tt.Wheel(800, 400, 0, dy)
		x.tt.Frame()
		x.tt.Advance(time.Second)
		x.tt.Frame()
	}
}

// focused is the name of the element with the focus.
func focused(tt *kit.Tester) string {
	for _, a := range tt.AccessAll() {
		if a.Focused {
			return a.Name
		}
	}
	return ""
}

// invalid tells whether a field named name is marked invalid.
func invalid(tt *kit.Tester, name string) bool {
	for _, a := range tt.AccessAll() {
		if a.Name == name && a.Invalid {
			return true
		}
	}
	return false
}

func (x *fixture) click(t *testing.T, label string) {
	t.Helper()
	x.reveal(label)
	if err := x.tt.Click(label); err != nil {
		t.Fatal(err)
	}
	x.settle(nil)
}

func (x *fixture) saved() config.Settings {
	var s config.Settings
	x.set.Lock(func() { s = x.set.View.Settings })
	return s
}

func shows(t *testing.T, tt *kit.Tester, texts ...string) {
	t.Helper()
	for _, s := range texts {
		if !kit.Shows(tt, s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
}

func hides(t *testing.T, tt *kit.Tester, texts ...string) {
	t.Helper()
	for _, s := range texts {
		if kit.Shows(tt, s) {
			t.Errorf("%q shows", s)
		}
	}
}

func both(t *testing.T, test func(t *testing.T, x *fixture)) {
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) { test(t, start(t, dark, seed())) })
	}
}

func TestSettingsListsItsPagesAndGoesBack(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		for _, p := range []string{"General", "Memory", "Appearance", "Models", "Usage", "Desktop", "Developer"} {
			if !kit.HasRole(x.tt, "heading", p) {
				t.Errorf("no heading %q", p)
			}
		}
		shows(t, x.tt, "Back to chats", "Data folder", `D:\Jenab`, "Approval level", "0.9.0")
		if !kit.HasRole(x.tt, "main", "") && !kit.HasRole(x.tt, "heading", "Settings") {
			t.Error("no Settings title")
		}
		x.click(t, "Usage")
		if r, ok := x.tt.Find("Tokens across all projects. Costs come from the built-in prices; your provider's bill is what counts."); !ok || r.Y > 400 {
			t.Errorf("Usage is not scrolled up: %v %v", r, ok)
		}
		x.click(t, "Back to chats")
		if p := x.st.Nav.Current(); p == nil || p.View != state.ViewChat {
			t.Errorf("Back to chats went to %+v", p)
		}
	})
}

func TestTheOpenProjectIsInThePageList(t *testing.T) {
	// settings-view.test.ts: the open project is the one of the chat used
	// last; its entry opens its settings.
	x := start(t, false, seed())
	if p := settings.OpenProject(x.st); p == nil || p.Name != "Demo" {
		t.Fatalf("OpenProject = %+v", p)
	}
	shows(t, x.tt, "Demo", "Project settings")
	x.click(t, "Project settings")
	if p := x.st.Nav.Current(); p.View != state.ViewProjectSettings || p.Project != x.project || p.Section != "" {
		t.Errorf("the entry went to %+v", p)
	}

	// With no project open, the list has no project entry.
	y := start(t, false, seed())
	y.st.Nav.MRU = nil
	y.settle(nil)
	if settings.OpenProject(y.st) != nil {
		t.Error("a project is open with no chat used")
	}
	if _, ok := y.tt.Find("Project settings"); ok {
		t.Error("the page list has a project entry")
	}
}

func TestTheMemoryScopeLinksTheProjectsMemory(t *testing.T) {
	// memory.test.ts: with a project open, the line links to its memory in
	// its settings; with none, it has no link.
	x := start(t, false, seed())
	shows(t, x.tt, "This memory is for every project.", "Project settings › Memory")
	x.click(t, "Project settings › Memory")
	if p := x.st.Nav.Current(); p.View != state.ViewProjectSettings || p.Section != "Memory" {
		t.Errorf("the link went to %+v", p)
	}

	y := start(t, false, seed())
	y.st.Nav.MRU = nil
	y.settle(nil)
	shows(t, y.tt, "Each project has its own memory too, in its Project settings.")
	hides(t, y.tt, "Project settings › Memory")
}

func TestTheMemoryEditorSavesASection(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		shows(t, x.tt, "preferences", "revision 2", "Short answers, please.", persian, "About 120 of 4,000 tokens")
		x.click(t, "Edit preferences")
		x.focus(t, "Text")
		x.tt.Key(0, kit.KeyEnd)
		x.tt.Type(" In Persian too.")
		x.settle(nil)
		x.click(t, "Save")
		var saves []app.MemoryEdit
		x.set.Lock(func() { saves = slices.Clone(x.set.MemSaves) })
		if len(saves) != 1 || saves[0].Section != "preferences" || saves[0].Revision != 2 || !strings.HasSuffix(saves[0].Content, "In Persian too.") {
			t.Fatalf("saves = %+v", saves)
		}
		shows(t, x.tt, "revision 3")
	})
}

func TestAMemoryConflictShowsTheirsToMerge(t *testing.T) {
	x := start(t, false, seed())
	x.set.Lock(func() {
		x.set.Conflict = &store.MemorySection{Section: "preferences", Content: "Long answers.", Revision: 3}
	})
	x.click(t, "Edit preferences")
	x.click(t, "Save")
	shows(t, x.tt, "This section changed since you opened it", "Long answers.", "Use theirs")
	x.click(t, "Use theirs")
	hides(t, x.tt, "Use theirs")
}

func TestAddingAMemorySection(t *testing.T) {
	x := start(t, false, seed())
	x.click(t, "Add section")
	if focused(x.tt) != "Section name" {
		t.Error("the new section's name has no focus")
	}
	x.tt.Type("دفتر")
	x.focus(t, "Text")
	x.tt.Type(persian)
	x.settle(nil)
	x.click(t, "Save")
	var saves []app.MemoryEdit
	x.set.Lock(func() { saves = slices.Clone(x.set.MemSaves) })
	if len(saves) != 1 || saves[0].Section != "دفتر" || saves[0].Content != persian || saves[0].Revision != 0 {
		t.Fatalf("saves = %+v", saves)
	}
}

func TestAppearanceAndDeveloperSaveAtOnce(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		x.click(t, "Dark")
		x.settle(func() bool { return x.saved().UI.Theme == "dark" })
		if got := x.saved().UI.Theme; got != "dark" {
			t.Errorf("theme = %q", got)
		}
		x.focus(t, "Developer tools")
		x.tt.Key(0, kit.KeySpace)
		x.settle(func() bool { return x.saved().DevTools })
		if !x.saved().DevTools {
			t.Error("developer tools are not on")
		}
		shows(t, x.tt, "Applies at the next start.")
		x.focus(t, "Let agents use other apps' windows")
		x.tt.Key(0, kit.KeySpace)
		x.settle(func() bool { return x.saved().DesktopControl })
		if !x.saved().DesktopControl {
			t.Error("desktop control is not on")
		}
	})
}

func TestTheApprovalLevelForNewProjects(t *testing.T) {
	x := start(t, false, seed())
	x.click(t, "Strict")
	x.settle(func() bool { return x.saved().Approvals.DefaultLevel == "strict" })
	if got := x.saved().Approvals.DefaultLevel; got != "strict" {
		t.Errorf("level = %q", got)
	}
}

func TestTheDataFolderDialog(t *testing.T) {
	x := start(t, false, seed())
	x.set.Lock(func() { x.set.Warn[`E:\Sync`] = &project.FolderWarning{Text: "This folder is synced."} })
	picked := ""
	x.st.Pages.PickFolder = func(title, start string) string { picked = start; return `E:\Sync` }
	x.click(t, "Change…")
	shows(t, x.tt, "Where projects are kept. The change applies at the next start, when the projects move there.", "Browse…")
	x.click(t, "Browse…")
	if picked != `D:\Jenab` {
		t.Errorf("the folder dialog started at %q", picked)
	}
	x.tt.Advance(time.Second)
	x.settle(func() bool { return kit.Shows(x.tt, "This folder is synced.") })
	shows(t, x.tt, "This folder is synced.")
	x.click(t, "Save")
	x.settle(func() bool { return x.saved().DataFolder != "" })
	if got := x.saved().DataFolder; got != `E:\Sync` {
		t.Errorf("data folder = %q", got)
	}
	shows(t, x.tt, `At the next start, the projects move to E:\Sync. Settings and logs stay where they are.`)
}

func TestTheAboutRowCopiesTheVersion(t *testing.T) {
	x := start(t, false, seed())
	x.click(t, "Copy")
	if got := x.tt.Clipboard(); !strings.Contains(got, "0.9.0") || !strings.Contains(got, "abc1234") {
		t.Errorf("clipboard = %q", got)
	}
	shows(t, x.tt, "Copied the version")
}

func TestTheProviderRows(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		shows(t, x.tt, "gemini", "Gemini", "Key in the keychain · 2 models on", "local", "Ollama", "On this computer · no key · 1 model on",
			"http://localhost:11434/v1/", "Replace the key of gemini")
		hides(t, x.tt, "Replace the key of local")
		x.focus(t, "Background calls at once for gemini")
		x.enter("3")
		x.settle(func() bool { return x.saved().LLM.ProviderMaxParallelCalls["gemini"] == 3 })
		if got := x.saved().LLM.ProviderMaxParallelCalls["gemini"]; got != 3 {
			t.Errorf("gemini's limit = %d", got)
		}
		x.enter("")
		x.settle(func() bool { _, ok := x.saved().LLM.ProviderMaxParallelCalls["gemini"]; return !ok })
		if _, ok := x.saved().LLM.ProviderMaxParallelCalls["gemini"]; ok {
			t.Error("an empty limit stayed")
		}
	})
}

func TestTheModelsDialogTurnsModelsOnAndOff(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		x.click(t, "Models of gemini")
		x.settle(func() bool { return kit.Shows(x.tt, "Gemini 3.5 Flash") })
		shows(t, x.tt, "Models that are on appear in the model picker at once.", "Gemini 3.5 Pro", "Other models", "Gemma 4 27B",
			"gemini-3.5-pro · 1M context")
		// The default and fast model can't be turned off.
		x.focus(t, "Gemini 3.5 Flash")
		x.tt.Key(0, kit.KeySpace)
		x.settle(func() bool { return len(x.saved().LLM.Providers["gemini"].Models) == 3 })
		if ms := x.saved().LLM.Providers["gemini"].Models; len(ms) != 3 || ms[2].ID != "gemini-3.5-flash" || ms[2].Context != 0 {
			t.Errorf("models = %+v", ms)
		}
		// An Other model needs its context window first.
		x.focus(t, "Context window of Gemma 4 27B")
		x.enter("128000")
		x.focus(t, "Gemma 4 27B")
		x.tt.Key(0, kit.KeySpace)
		x.settle(func() bool { return len(x.saved().LLM.Providers["gemini"].Models) == 4 })
		if ms := x.saved().LLM.Providers["gemini"].Models; len(ms) != 4 || ms[3].Context != 128000 {
			t.Errorf("models = %+v", ms)
		}
		x.tt.Key(0, kit.KeyEscape)
		x.settle(nil)
		hides(t, x.tt, "Other models")
	})
}

func TestRemovingAProvider(t *testing.T) {
	x := start(t, false, seed())
	x.click(t, "Remove local")
	shows(t, x.tt, "Remove local?")
	x.click(t, "Remove")
	x.settle(func() bool { _, ok := x.saved().LLM.Providers["local"]; return !ok })
	var removed []string
	x.set.Lock(func() { removed = x.set.Removed })
	if len(removed) != 1 || removed[0] != "local" {
		t.Errorf("removed = %v", removed)
	}
}

func TestAddingAProvider(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		x.set.Lock(func() {
			x.set.ConnectOK = func(req app.ConnectRequest) (app.ConnectResult, error) {
				v := x.set.View
				v.Settings.LLM.Providers = map[string]config.ProviderSettings{}
				for k, p := range x.set.View.Settings.LLM.Providers {
					v.Settings.LLM.Providers[k] = p
				}
				v.Settings.LLM.Providers[req.Name] = config.ProviderSettings{Kind: req.Kind, Models: []config.ModelSettings{{ID: "claude-x"}}}
				return app.ConnectResult{Provider: req.Name, Models: 1, View: v}, nil
			}
		})
		x.click(t, "Add provider")
		x.settle(func() bool { return kit.Shows(x.tt, "Choose a provider") })
		shows(t, x.tt, "The key is checked with the provider and stored in the OS keychain.")
		if err := x.tt.Click("Choose a provider"); err != nil {
			t.Fatal(err)
		}
		x.tt.Frame()
		if err := x.tt.ChooseMenuItem("Anthropic"); err != nil {
			t.Fatalf("%v; menu %q", err, x.tt.Menu())
		}
		x.settle(nil)
		x.focus(t, "API key")
		x.tt.Type("sk-test")
		x.settle(nil)
		x.click(t, "Connect")
		x.settle(func() bool { _, ok := x.saved().LLM.Providers["anthropic"]; return ok })
		var reqs []app.ConnectRequest
		x.set.Lock(func() { reqs = slices.Clone(x.set.Connects) })
		if len(reqs) != 1 || reqs[0].Name != "anthropic" || reqs[0].Kind != "anthropic" || reqs[0].Key != "sk-test" {
			t.Fatalf("connects = %+v", reqs)
		}
		x.settle(nil)
		shows(t, x.tt, "Anthropic connected · 1 model on")
		hides(t, x.tt, "The key is checked with the provider and stored in the OS keychain.")
	})
}

func TestTheAliasPickers(t *testing.T) {
	x := start(t, false, seed())
	shows(t, x.tt, "Default model", "Fast model", "New chats start with it.")
	x.focus(t, "Fast model")
	x.tt.Key(0, kit.KeyEnter)
	x.tt.Frame()
	if err := x.tt.ChooseMenuItem("Gemini 3.5 Pro"); err != nil {
		t.Fatalf("%v; menu %q", err, x.tt.Menu())
	}
	x.settle(func() bool { return x.saved().LLM.Models["fast"] == "gemini/gemini-3.5-pro" })
	if got := x.saved().LLM.Models["fast"]; got != "gemini/gemini-3.5-pro" {
		t.Errorf("fast = %q", got)
	}
}

func TestTheLimits(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		shows(t, x.tt, "Parallel background LLM calls", "Chats count but never wait", "Tokens per app-started turn", "config.yaml")
		x.focus(t, "Requests per turn")
		x.enter("40")
		x.settle(func() bool { return x.saved().LLM.TurnMaxRequests == 40 })
		if got := x.saved().LLM.TurnMaxRequests; got != 40 {
			t.Errorf("requests per turn = %d", got)
		}
		x.enter("1")
		if got := x.saved().LLM.TurnMaxRequests; got != 40 {
			t.Errorf("a value below the least was saved: %d", got)
		}
		if !invalid(x.tt, "Requests per turn") {
			t.Error("the field is not marked")
		}
	})
}

func TestTheUsagePage(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		shows(t, x.tt, "72K", "$0.01", "By model", "Gemini 3.5 Flash-Lite · gemini", "By chat", persian+" · Demo",
			"Untitled chat · Demo", "Input", "Output", "Cached")
		if !kit.HasRole(x.tt, "table", "By model") {
			t.Error("By model is no table")
		}
		x.click(t, "7 days")
		x.settle(func() bool { return x.st.Pages.UsageDays == 7 && !x.st.Pages.UsageBusy })
		var days []int
		x.set.Lock(func() { days = slices.Clone(x.set.Days) })
		if !slices.Equal(days, []int{30, 7}) {
			t.Errorf("days asked = %v", days)
		}
		if n := len(x.st.Pages.Usage.Days); n != 7 {
			t.Errorf("%d days shown", n)
		}
	})
}

func TestTheDesktopPage(t *testing.T) {
	both(t, func(t *testing.T, x *fixture) {
		shows(t, x.tt, "calculatorapp", "add(a, b)", "Adds two numbers", "1. click {a}", "memory_keys", "saved by an agent · used 3 times",
			"worked in 2 s")
		x.click(t, "Delete memory_keys")
		x.settle(func() bool {
			var n int
			x.set.Lock(func() { n = len(x.set.Deleted) })
			return n == 1
		})
		x.click(t, "Add recipe")
		x.focus(t, "Name")
		x.tt.Type("site_search")
		x.focus(t, "Text")
		x.tt.Type(persian)
		x.settle(nil)
		shows(t, x.tt, fmt.Sprintf("%d of 1,200 characters.", len([]rune(persian))))
		x.click(t, "Save")
		x.settle(func() bool {
			var n int
			x.set.Lock(func() { n = len(x.set.Writes) })
			return n == 1
		})
		var w []app.DesktopRecipeWrite
		var del []int64
		x.set.Lock(func() { w, del = slices.Clone(x.set.Writes), slices.Clone(x.set.Deleted) })
		if len(del) != 1 || del[0] != 9 {
			t.Errorf("deleted = %v", del)
		}
		if len(w) != 1 || w[0].App != "calculatorapp" || w[0].Name != "site_search" || w[0].Text != persian {
			t.Errorf("writes = %+v", w)
		}
	})
}

func TestAMacroEdit(t *testing.T) {
	x := start(t, false, seed())
	x.click(t, "Edit add(a, b)")
	x.focus(t, "Params")
	x.enter("a, b, c")
	x.click(t, "Save")
	x.settle(func() bool {
		var n int
		x.set.Lock(func() { n = len(x.set.Edits) })
		return n == 1
	})
	var e []app.DesktopMacroEdit
	x.set.Lock(func() { e = slices.Clone(x.set.Edits) })
	if len(e) != 1 || e[0].ID != 7 || !slices.Equal(e[0].Params, []string{"a", "b", "c"}) || e[0].Description != "Adds two numbers" {
		t.Errorf("edits = %+v", e)
	}
}
