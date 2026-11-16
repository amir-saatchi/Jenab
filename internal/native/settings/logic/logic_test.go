package logic

// Ported from the React app's settings/format.test.ts,
// chat/provider-choice.test.ts and memory.test.ts; the rest
// covers what the React pages did inline.

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/update"
	"github.com/amir-saatchi/jenab/internal/version"
)

func line(cost float64, unpriced int) app.UsageLine {
	return app.UsageLine{Cost: cost, Unpriced: unpriced}
}

func TestTokenCountsAndCostsReadShort(t *testing.T) {
	got := []string{Tokens(950), Tokens(12500), Tokens(1_200_000), Tokens(999_999), Tokens(0)}
	if want := []string{"950", "12.5K", "1.2M", "1M", "0"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tokens: %q, want %q", got, want)
	}
	got = []string{Cost(line(0, 0)), Cost(line(4.575, 0)), Cost(line(0.004, 0)), Cost(line(1, 10)), Cost(line(0, 10))}
	if want := []string{"$0.00", "$4.58", "<$0.01", "$1.00+", "—"}; !reflect.DeepEqual(got, want) {
		t.Errorf("costs: %q, want %q", got, want)
	}
	if Total(app.UsageLine{Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4}) != 10 {
		t.Error("total")
	}
	if ContextSize(0) != "" || ContextSize(1_000_000) != "1M" || ContextSize(128_000) != "128K" {
		t.Error("context sizes")
	}
}

func TestALocalOllamaGetsOneBackgroundCall(t *testing.T) {
	cases := []struct {
		kind, url string
		want      int
	}{
		{"ollama", "", 1},
		{"ollama", "http://127.0.0.1:11434/", 1},
		{"ollama", "http://[::1]:11434/", 1},
		{"ollama", "https://ollama.com/", 8},
		{"gemini", "", 8},
	}
	for _, c := range cases {
		if got := LimitDefault(c.kind, c.url, 8); got != c.want {
			t.Errorf("LimitDefault(%q, %q) = %d", c.kind, c.url, got)
		}
	}
}

func TestBaseURLPlaceholdersAreFound(t *testing.T) {
	if got := Placeholders("https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1/"); !reflect.DeepEqual(got, []string{"account_id"}) {
		t.Errorf("got %q", got)
	}
	if got := Placeholders(""); !reflect.DeepEqual(got, []string{}) {
		t.Errorf("got %q", got)
	}
}

func TestTheAboutLineNamesTheVersionAndTheCommit(t *testing.T) {
	v := version.Info{Version: "0.1.0-dev", Commit: "d449b7e", Go: "go1.26.10", OS: "windows/amd64"}
	if got := VersionLine(v); got != "Jenab 0.1.0-dev (d449b7e)" {
		t.Error(got)
	}
	m := v
	m.Modified = true
	if got := VersionLine(m); got != "Jenab 0.1.0-dev (d449b7e, modified)" {
		t.Error(got)
	}
	d := v
	d.Version, d.Commit = "dev", ""
	if got := VersionLine(d); got != "Jenab dev" {
		t.Error(got)
	}
	if got := BuildLine(v); got != "Go 1.26.10, windows/amd64" {
		t.Error(got)
	}
}

func TestPathsCompareAsWindowsDoes(t *testing.T) {
	if !SamePath(`D:\Data\Jenab\`, `d:\data\jenab`) || SamePath(`D:\Data`, `D:\Data2`) {
		t.Error("SamePath")
	}
	if Thousands(1234567) != "1,234,567" || Thousands(12) != "12" || Thousands(1000) != "1,000" {
		t.Error("Thousands")
	}
}

// The list as SettingsService.Presets sends it (internal/provider/presets.go).
func preset(id, name, kind, baseURL, group, host string) app.PresetItem {
	return app.PresetItem{ID: id, Name: name, Kind: kind, BaseURL: baseURL, Group: group, Host: host,
		Fields: Placeholders(baseURL), NoKey: group == "local"}
}

var presets = []app.PresetItem{
	preset("anthropic", "Anthropic", "anthropic", "", "cloud", "api.anthropic.com"),
	preset("openai", "OpenAI", "openai", "", "cloud", "api.openai.com"),
	preset("gemini", "Google Gemini", "gemini", "", "cloud", "generativelanguage.googleapis.com"),
	preset("groq", "Groq", "openai_compatible", "https://api.groq.com/openai/v1/", "cloud", "api.groq.com"),
	preset("openrouter", "OpenRouter", "openai_compatible", "https://openrouter.ai/api/v1/", "cloud", "openrouter.ai"),
	preset("zai", "Z.ai", "openai_compatible", "https://api.z.ai/api/paas/v4/", "cloud", "api.z.ai"),
	preset("cloudflare", "Cloudflare Workers AI", "openai_compatible", "https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1/", "cloud", "api.cloudflare.com"),
	preset("ollama-cloud", "Ollama Cloud", "ollama", "https://ollama.com/", "cloud", "ollama.com"),
	preset("ollama", "Ollama", "ollama", "", "local", "localhost:11434"),
	preset("lmstudio", "LM Studio", "openai_compatible", "http://localhost:1234/v1/", "local", "localhost:1234"),
}

func TestTheProvidersComeInThreeGroupsEachWithASecondLine(t *testing.T) {
	groups := ProviderGroups(presets)
	var labels, cloud []string
	for _, g := range groups {
		labels = append(labels, g.Label)
	}
	for _, c := range groups[0].Items {
		cloud = append(cloud, c.Name)
	}
	if want := []string{"Cloud", "On this computer", "Other"}; !reflect.DeepEqual(labels, want) {
		t.Errorf("labels %q", labels)
	}
	if want := []string{"Anthropic", "OpenAI", "Google Gemini", "Groq", "OpenRouter", "Z.ai", "Cloudflare Workers AI", "Ollama Cloud"}; !reflect.DeepEqual(cloud, want) {
		t.Errorf("cloud %q", cloud)
	}
	if groups[0].Items[0].Hint != "api.anthropic.com" || groups[0].Items[6].Hint != "api.cloudflare.com" {
		t.Error("hints")
	}
	if want := []Choice{{"ollama", "Ollama", "no key needed"}, {"lmstudio", "LM Studio", "no key needed"}}; !reflect.DeepEqual(groups[1].Items, want) {
		t.Errorf("local %v", groups[1].Items)
	}
	if o := groups[2].Items; len(o) != 1 || o[0].ID != Other || o[0].Name != "OpenAI-compatible…" {
		t.Errorf("other %v", o)
	}
	for _, g := range groups {
		for _, c := range g.Items {
			if c.Hint == "" {
				t.Errorf("%s has no second line", c.Name)
			}
		}
	}
}

func TestARunningOllamaPutsItsGroupFirstAndIsPicked(t *testing.T) {
	var running []app.PresetItem
	for _, p := range presets {
		if p.ID == "ollama" {
			p.Running = true
			running = append([]app.PresetItem{p}, running...) // as the Go list orders it
		} else {
			running = append(running, p)
		}
	}
	groups := ProviderGroups(running)
	if groups[0].Label != "On this computer" || groups[1].Label != "Cloud" || groups[2].Label != "Other" {
		t.Errorf("groups %v", groups)
	}
	if groups[0].Items[0].Hint != "running · no key needed" {
		t.Error(groups[0].Items[0].Hint)
	}
	if FirstPick(running) != "ollama" || FirstPick(presets) != "" {
		t.Error("first pick")
	}
}

func TestTheFormAsksForWhatTheProviderNeeds(t *testing.T) {
	if f := FormFor("", presets, nil); f.Chosen || f.Other || f.Key || len(f.Fields) != 0 {
		t.Errorf("nothing chosen: %+v", f)
	}
	if f := FormFor("anthropic", presets, nil); !f.Chosen || f.Other || !f.Key || len(f.Fields) != 0 || f.Preset.BaseURL != "" {
		t.Errorf("a cloud provider asks for its key: %+v", f)
	}
	for _, id := range []string{"ollama", "lmstudio"} {
		if f := FormFor(id, presets, nil); !f.Chosen || f.Key || f.Other || len(f.Fields) != 0 {
			t.Errorf("%s hides the key field: %+v", id, f)
		}
	}
	if FormFor("lmstudio", presets, nil).Preset.BaseURL != "http://localhost:1234/v1/" {
		t.Error("LM Studio's base URL")
	}
	if f := FormFor("cloudflare", presets, nil); !f.Key || !reflect.DeepEqual(f.Fields, []string{"account_id"}) {
		t.Errorf("Cloudflare: %+v", f)
	}
	if FieldLabel("account_id") != "Account ID" || FieldLabel("region") != "Region" {
		t.Error("field labels")
	}
	if f := FormFor(Other, presets, nil); !f.Chosen || !f.Other || !f.Key || len(f.Fields) != 0 {
		t.Errorf("Other: %+v", f)
	}
	r := &Replace{BaseURL: "https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1/"}
	if f := FormFor("", nil, r); !f.Chosen || f.Other || !f.Key || !reflect.DeepEqual(f.Fields, []string{"account_id"}) {
		t.Errorf("a new key keeps the placeholders: %+v", f)
	}
}

func TestFreeNameAndTheToasts(t *testing.T) {
	taken := map[string]int{"gemini": 1, "gemini-2": 1, "openai": 1}
	for in, want := range map[string]string{"groq": "groq", "openai": "openai-2", "gemini": "gemini-3"} {
		if got := FreeName(in, taken); got != want {
			t.Errorf("FreeName(%q) = %q", in, got)
		}
	}
	cases := map[string]string{
		ConnectedText("Gemini", app.ConnectResult{Models: 3}, false):     "Gemini connected · 3 models on",
		ConnectedText("Gemini", app.ConnectResult{Models: 1}, false):     "Gemini connected · 1 model on",
		ConnectedText("x", app.ConnectResult{NoModelList: true}, false):  "x connected. It lists no models; add them in Settings → Models.",
		ConnectedText("gemini", app.ConnectResult{}, true):               "gemini: the new key works and is saved",
		KindName("openai_compatible") + "|" + KindName("something-else"): "OpenAI-compatible|something-else",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("%q, want %q", got, want)
		}
	}
}

func TestProviderRowsAndModelToggles(t *testing.T) {
	s := config.Defaults()
	s.LLM.Providers = map[string]config.ProviderSettings{
		"zai":    {Kind: "openai_compatible", BaseURL: "https://api.z.ai/api/paas/v4/", Models: []config.ModelSettings{{ID: "glm"}}},
		"ollama": {Kind: "ollama", Models: []config.ModelSettings{{ID: "a"}, {ID: "b"}}},
	}
	ps := Providers(s)
	if len(ps) != 2 || ps[0].Name != "ollama" || !ps[0].Local() || ps[1].Local() {
		t.Fatalf("providers %+v", ps)
	}
	if ps[0].Line() != "On this computer · no key · 2 models on" || ps[1].Line() != "Key in the keychain · 1 model on" {
		t.Errorf("lines %q, %q", ps[0].Line(), ps[1].Line())
	}

	aliases := map[string]string{"default": "zai/glm", "fast": "zai/glm"}
	if got := Aliased(aliases, "zai", "glm"); !reflect.DeepEqual(got, []string{"default", "fast"}) {
		t.Errorf("aliased %q", got)
	}
	if SwitchHint([]string{"default", "fast"}, true, false) != "Used as default and fast; pick another model for it first" {
		t.Error("hint")
	}
	if SwitchHint(nil, false, true) != "Enter the context window first" || SwitchHint(nil, true, false) != "" {
		t.Error("hints")
	}

	Toggle("zai", app.ModelOption{ID: "other-1"}, true, 32000)(&s)
	Toggle("zai", app.ModelOption{ID: "glm", Known: true}, false, 0)(&s)
	Toggle("zai", app.ModelOption{ID: "glm-2", Known: true}, true, 9)(&s)
	Toggle("gone", app.ModelOption{ID: "x"}, true, 0)(&s)
	want := []config.ModelSettings{{ID: "other-1", Context: 32000}, {ID: "glm-2"}}
	if got := s.LLM.Providers["zai"].Models; !reflect.DeepEqual(got, want) {
		t.Errorf("models %+v", got)
	}

	n := 3
	ProviderLimit("zai", &n)(&s)
	if s.LLM.ProviderMaxParallelCalls["zai"] != 3 {
		t.Error("limit not set")
	}
	ProviderLimit("zai", nil)(&s)
	if _, ok := s.LLM.ProviderMaxParallelCalls["zai"]; ok {
		t.Error("an empty limit kept its value")
	}
}

func TestTheProviderDialogListsTypedModelsAfterTheListedOnes(t *testing.T) {
	pm := app.ProviderModels{Kind: "openai_compatible",
		Models: []app.ModelOption{{ID: "a", Name: "A", Context: 128000}},
		Other:  []app.ModelOption{{ID: "b", Name: "b"}}}
	rows := ProviderModelRows(pm, []config.ModelSettings{{ID: "a"}, {ID: "b"}, {ID: "typed", Context: 8000}})
	if len(rows) != 2 || rows[1].ID != "typed" || !rows[1].On || rows[1].Context != 8000 {
		t.Errorf("rows %+v", rows)
	}
	if ModelLine(rows[0], false) != "a · 128K context" || ModelLine(rows[0], true) != "a" {
		t.Error("model lines")
	}
	if !CanAddModels(pm) || CanAddModels(app.ProviderModels{Kind: "gemini"}) || !CanAddModels(app.ProviderModels{Kind: "gemini", NoModelList: true}) {
		t.Error("CanAddModels")
	}
}

func TestAliasPickersOfferTheModelsThatAreOn(t *testing.T) {
	groups := []app.ModelGroup{{Provider: "gemini", Models: []app.ModelItem{{Ref: "gemini/flash", Name: "Flash"}}}}
	if c := AliasChoices(groups, "gemini/flash"); len(c) != 1 || c[0].Group != "gemini" {
		t.Errorf("choices %+v", c)
	}
	c := AliasChoices(groups, "zai/glm")
	if len(c) != 2 || !c[1].NotOn || c[1].Group != "Not on" {
		t.Errorf("a model not on: %+v", c)
	}
	if AliasName(groups, "") != "No model" || AliasName(groups, "gemini/flash") != "Flash" || AliasName(groups, "zai/glm") != "zai/glm" {
		t.Error("names")
	}
}

func TestPagesProblemsAndMoves(t *testing.T) {
	if got := strings.Join(Pages(true), ","); got != "General,Memory,Appearance,Models,Usage,Desktop,Developer" {
		t.Error(got)
	}
	if got := strings.Join(Pages(false), ","); got != "General,Memory,Appearance,Models,Usage,Developer" {
		t.Error(got)
	}
	if got := ProblemLine(config.Problem{Path: "llm.turn_max_requests", Line: 12, Msg: "below 2"}); got != "Line 12: llm.turn_max_requests: below 2" {
		t.Error(got)
	}
	if got := ProblemLine(config.Problem{Msg: "not YAML"}); got != "not YAML" {
		t.Error(got)
	}
	if MovedText(0, nil) != "" || MovedText(1, nil) != "1 project was moved to the new data folder at this start." {
		t.Error("moved")
	}
	if got := MovedText(2, []string{"قیمت‌ها"}); got != "2 projects were moved to the new data folder at this start. Not moved, still opened from the old folder: قیمت‌ها. Jenab tries again at the next start; the log has the reason." {
		t.Error(got)
	}
	s := config.Defaults()
	for _, l := range Limits {
		l.Set(&s, l.Min+5)
		if l.Get(&s) != l.Min+5 {
			t.Errorf("%s does not round-trip", l.Label)
		}
	}
}

func TestDesktopLines(t *testing.T) {
	format := func(t time.Time) string { return t.Format("2006-01-02 15:04") }
	at := time.Date(2026, 11, 16, 9, 30, 0, 0, time.UTC)
	if When(time.Time{}, format) != "never" || When(at, format) != "2026-11-16 09:30" {
		t.Error("when")
	}
	if SourceLabel("app") != "comes with the app" || SourceLabel("user") != "yours" || SourceLabel("agent") != "saved by an agent" {
		t.Error("source")
	}
	ok := store.DesktopRun{At: at, OK: true, Seconds: 4.4, Skipped: 1}
	bad := store.DesktopRun{At: at, FailedAt: 3, Reason: "no window"}
	if RunLine(ok, format) != "2026-11-16 09:30: worked in 4 s, 1 optional skipped" || RunLine(bad, format) != "2026-11-16 09:30: failed at step 3: no window" {
		t.Errorf("%q / %q", RunLine(ok, format), RunLine(bad, format))
	}
	r := RemovedItems([]store.DesktopMacro{{Name: "m", App: "calc", RemovedWhy: "3 failures"}}, []store.DesktopRecipe{{Name: "r", App: "chrome"}})
	if len(r) != 2 || r[0].Kind != "Macro" || r[1].Kind != "Recipe" {
		t.Errorf("removed %+v", r)
	}
}

// memory.tsx's editor, which the React app tests by hand.

func sec(name, content string, rev int) store.MemorySection {
	return store.MemorySection{Section: name, Content: content, Revision: rev}
}

func TestTheMemoryEditorOpensTheSectionAChipAsks(t *testing.T) {
	var m Memory
	m.Loaded(app.MemoryView{Sections: []store.MemorySection{sec("goals", "save", 2)}, Tokens: 1234, Cap: 4000}, "goals")
	if !m.Editing("goals") || m.Draft.Content != "save" || m.Draft.Revision != 2 {
		t.Fatalf("draft %+v", m.Draft)
	}
	m.Draft.Content = "unsaved"
	m.Loaded(*m.View, "goals")
	if m.Draft.Content != "unsaved" {
		t.Error("reading again dropped the unsaved text")
	}
	line, f := Used(*m.View)
	if line != "About 1,234 of 4,000 tokens" || f < 0.30 || f > 0.31 {
		t.Errorf("%q %v", line, f)
	}
	if section, ok := FocusSection("memory:user/goals", "", "user", ""); !ok || section != "goals" {
		t.Error("user focus")
	}
	if _, ok := FocusSection("memory:project/goals", "p1", "project", "p2"); ok {
		t.Error("another project's section opened")
	}
	if _, ok := FocusSection("memory:project/goals", "p1", "user", ""); ok {
		t.Error("a project section opened in user memory")
	}
}

func TestAConflictKeepsTheDraftToMerge(t *testing.T) {
	var m Memory
	m.Loaded(app.MemoryView{Sections: []store.MemorySection{sec("goals", "old", 1)}}, "")
	m.Edit(m.Sections()[0])
	d := *m.Draft
	theirs := sec("goals", "theirs", 2)
	m.Saved(d, "mine", app.MemorySaved{Memory: app.MemoryView{Sections: []store.MemorySection{theirs}}, Conflict: &theirs})
	if m.Conflict == nil || m.Draft.Content != "mine" || m.Draft.Revision != 2 {
		t.Fatalf("after the conflict: %+v %+v", m.Draft, m.Conflict)
	}
	if ConflictTitle(*m.Conflict) != "This section changed since you opened it" || ResolveLabel(*m.Conflict) != "Use theirs" {
		t.Error("conflict words")
	}
	m.Resolve()
	if m.Conflict != nil || m.Draft.Content != "theirs" {
		t.Errorf("Use theirs: %+v", m.Draft)
	}
	m.Saved(*m.Draft, "theirs", app.MemorySaved{Memory: *m.View})
	if m.Draft != nil {
		t.Error("a save did not close the editor")
	}
}

func TestASectionDeletedElsewhereStaysAsAnOrphan(t *testing.T) {
	var m Memory
	m.Loaded(app.MemoryView{Sections: []store.MemorySection{sec("goals", "old", 1)}}, "goals")
	gone := store.MemorySection{Section: "goals", Revision: 2}
	m.Saved(*m.Draft, "mine", app.MemorySaved{Memory: app.MemoryView{}, Conflict: &gone})
	if !m.Orphan() || m.Empty() {
		t.Errorf("orphan %v, empty %v", m.Orphan(), m.Empty())
	}
	if ConflictText(gone) != "Save to keep your text, or let it go." || ResolveLabel(gone) != "Let it go" {
		t.Error("deleted words")
	}
	m.Resolve()
	if m.Draft != nil || !m.Empty() {
		t.Error("Let it go kept the draft")
	}
	m.Add()
	if m.CanSave() || m.Empty() {
		t.Error("a new section with no name can be saved")
	}
	m.Draft.Section = "prefs"
	if !m.CanSave() {
		t.Error("a named section can't be saved")
	}
	m.Busy = true
	if m.CanSave() {
		t.Error("saved twice at once")
	}
	if e := m.Draft.Edit(""); e.Section != "prefs" || e.Content != "" {
		t.Errorf("delete edit %+v", e)
	}
}

func TestTheScopeLineWords(t *testing.T) {
	// memory.test.ts: the line says the memory is for every project.
	if !regexp.MustCompile(`^This memory is for every project\.$`).MatchString(ScopeLine) ||
		ScopeLine+" "+ScopeNoProject != "This memory is for every project. Each project has its own memory too, in its Project settings." {
		t.Error("scope line")
	}
	if ScopeLinkLabel != "Project settings › Memory" {
		t.Error(ScopeLinkLabel)
	}
}

func TestUpdateLines(t *testing.T) {
	at := time.Date(2026, 11, 16, 9, 30, 0, 0, time.UTC)
	format := func(t time.Time) string { return t.Format("Jan 2") }
	for _, c := range []struct {
		st     update.Status
		text   string
		action UpdateAction
	}{
		{update.Status{State: update.StateUnavailable}, "This build can't update itself.", UpdateNone},
		{update.Status{State: update.StateOff}, "Updates are off.", UpdateCheck},
		{update.Status{State: update.StateUpToDate, Checked: at}, "Jenab is up to date. Checked Nov 16.", UpdateCheck},
		{update.Status{State: update.StateAvailable, Version: "0.2.0"}, "Jenab 0.2.0 is available.", UpdateInstall},
		{update.Status{State: update.StateDownloading, Version: "0.2.0", Progress: 0.42}, "Downloading Jenab 0.2.0… 42%", UpdateNone},
		{update.Status{State: update.StateReady, Version: "0.2.0"}, "Jenab 0.2.0 is installed.", UpdateRestart},
		{update.Status{State: update.StateFailed, Version: "0.2.0", Err: "bad signature"}, "Jenab 0.2.0 was not installed: bad signature. This version keeps running.", UpdateCheck},
		{update.Status{State: update.StateFailed, Err: "offline"}, "The check failed: offline", UpdateCheck},
		{update.Status{}, "Not checked yet.", UpdateCheck},
	} {
		text, action := UpdateLine(c.st, format)
		if !strings.HasPrefix(text, c.text) || action != c.action {
			t.Errorf("%s: %q, %d; want %q…, %d", c.st.State, text, action, c.text, c.action)
		}
	}
	if UpdateButton(UpdateRestart) != "Restart now" || UpdateButton(UpdateNone) != "" {
		t.Error("buttons")
	}
}
