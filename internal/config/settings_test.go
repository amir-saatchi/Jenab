package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func writeConfig(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaults(t *testing.T) {
	d := Defaults()
	if probs := d.check(nil); len(probs) != 0 {
		t.Errorf("default.yaml has problems: %v", probs)
	}
	if d.LLM.Models == nil || d.Updates.Mode != "notify" || d.LLM.MaxParallelCalls != 8 ||
		d.LLM.MaxSubagentsPerChat != 5 || len(d.LLM.ProviderMaxParallelCalls) != 0 {
		t.Errorf("Defaults = %+v", d)
	}
	// Every field of Settings is in default.yaml, so a new file documents all of them.
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := SaveSettings(path, d); err != nil {
		t.Fatal(err)
	}
	got, probs, err := LoadSettings(path)
	if err != nil || len(probs) != 0 {
		t.Fatalf("LoadSettings = %v, %v", probs, err)
	}
	if diff := cmp.Diff(d, got); diff != "" {
		t.Errorf("round trip (-want +got):\n%s", diff)
	}
}

func TestLoadMissing(t *testing.T) {
	s, probs, err := LoadSettings(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil || len(probs) != 0 {
		t.Fatalf("LoadSettings = %v, %v", probs, err)
	}
	if diff := cmp.Diff(Defaults(), s); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
}

func TestLoadEmpty(t *testing.T) {
	s, probs, err := LoadSettings(writeConfig(t, "# nothing yet\n"))
	if err != nil || len(probs) != 0 {
		t.Fatalf("LoadSettings = %v, %v", probs, err)
	}
	if diff := cmp.Diff(Defaults(), s); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
}

func TestLoadValues(t *testing.T) {
	s, probs, err := LoadSettings(writeConfig(t, `
data_folder: `+filepath.Join(t.TempDir(), "Data")+`
llm:
  max_parallel_calls: 2
  models:
    default: gemini-flash
ui:
  theme: dark
`))
	if err != nil || len(probs) != 0 {
		t.Fatalf("LoadSettings = %v, %v", probs, err)
	}
	want := Defaults()
	want.DataFolder = s.DataFolder
	want.LLM.MaxParallelCalls = 2
	want.LLM.Models = map[string]string{"default": "gemini-flash"}
	want.UI.Theme = "dark"
	if diff := cmp.Diff(want, s); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
}

func TestLoadUnknownKeys(t *testing.T) {
	s, probs, err := LoadSettings(writeConfig(t, `ui:
  theme: dark
  font_size: 14
telemetry: true
`))
	if err != nil {
		t.Fatal(err)
	}
	want := []Problem{
		{Path: "ui.font_size", Line: 3, Msg: "unknown setting, ignored"},
		{Path: "telemetry", Line: 4, Msg: "unknown setting, ignored"},
	}
	if diff := cmp.Diff(want, probs); diff != "" {
		t.Errorf("problems (-want +got):\n%s", diff)
	}
	if s.UI.Theme != "dark" {
		t.Errorf("known keys next to unknown ones are still read: theme = %q", s.UI.Theme)
	}
}

func TestLoadBadValues(t *testing.T) {
	s, probs, err := LoadSettings(writeConfig(t, `data_folder: relative/path
context:
  history_min_turns: lots
  history_max_turns: 0
  turn_trim_ratio: 1.5
llm:
  turn_max_requests: 1
approvals:
  default_level: yolo
updates:
  channel: nightly
`))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(Defaults(), s); diff != "" {
		t.Errorf("bad values should fall back to the defaults (-want +got):\n%s", diff)
	}
	lines := map[int]bool{}
	for _, p := range probs {
		lines[p.Line] = true
		if !strings.HasSuffix(p.Msg, "the default is used") {
			t.Errorf("problem without the fallback note: %v", p)
		}
	}
	for _, l := range []int{1, 3, 4, 5, 7, 9, 11} {
		if !lines[l] {
			t.Errorf("no problem for line %d; got %v", l, probs)
		}
	}
	if len(probs) != 7 {
		t.Errorf("got %d problems, want 7: %v", len(probs), probs)
	}
}

func TestLoadProviderLimits(t *testing.T) {
	s, probs, err := LoadSettings(writeConfig(t, `llm:
  provider_max_parallel_calls:
    gemini: 2
    groq: 0
`))
	if err != nil {
		t.Fatal(err)
	}
	// A bad entry is dropped, so that provider gets the global limit.
	want := map[string]int{"gemini": 2}
	if diff := cmp.Diff(want, s.LLM.ProviderMaxParallelCalls); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
	if len(probs) != 1 || probs[0].Path != "llm.provider_max_parallel_calls.groq" || probs[0].Line != 4 {
		t.Errorf("problems = %v", probs)
	}
}

func TestLoadSyntaxError(t *testing.T) {
	for _, text := range []string{"ui: [dark\n", "- a list\n"} {
		s, _, err := LoadSettings(writeConfig(t, text))
		if err == nil {
			t.Errorf("%q: no error", text)
		}
		if diff := cmp.Diff(Defaults(), s); diff != "" {
			t.Errorf("%q: the defaults should come with the error (-want +got):\n%s", text, diff)
		}
	}
}

func TestSaveNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Jenab", "config.yaml") // the folder doesn't exist yet
	s := Defaults()
	s.UI.Theme = "light"
	if err := SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{
		"# Jenab settings.",                    // the template's header
		"theme: light # light, dark or system", // value changed, comment kept
		"models: {}",
		"provider_max_parallel_calls: {} # background calls per provider", // an empty map keeps its comment
	} {
		if !strings.Contains(text, want) {
			t.Errorf("new file is missing %q:\n%s", want, text)
		}
	}
	tmp, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".config-*"))
	if len(tmp) != 0 {
		t.Errorf("temp files left: %v", tmp)
	}
}

func TestSaveKeepsComments(t *testing.T) {
	path := writeConfig(t, `# my settings
ui:
  theme: dark # I like it dark
llm:
  # set by hand
  max_parallel_calls: 2
  models:
    default: old-model # my pick
    gone: removed-model
  future_key: keep me
plugins: [a, b]
`)
	s, _, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	s.UI.Theme = "system"
	s.LLM.MaxParallelCalls = 6
	s.LLM.Models = map[string]string{"default": "new-model", "fast": "small"}
	if err := SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	text := string(b)
	for _, want := range []string{
		"# my settings",
		"theme: system # I like it dark",
		"# set by hand",
		"max_parallel_calls: 6",
		"default: new-model # my pick",
		"fast: small",
		"future_key: keep me",
		"plugins: [a, b]",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("saved file is missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "old-model") || strings.Contains(text, "gone:") {
		t.Errorf("the models map should have exactly the new entries:\n%s", text)
	}

	got, _, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(s, got); diff != "" {
		t.Errorf("reload (-want +got):\n%s", diff)
	}
}

func TestSaveOverBrokenFile(t *testing.T) {
	path := writeConfig(t, "ui: [dark\n")
	if err := SaveSettings(path, Defaults()); err != nil {
		t.Fatal(err)
	}
	if _, probs, err := LoadSettings(path); err != nil || len(probs) != 0 {
		t.Errorf("after a save the file loads: %v, %v", probs, err)
	}
	// The broken file is kept next to it; a second one doesn't replace it.
	if b, err := os.ReadFile(path + ".broken"); err != nil || string(b) != "ui: [dark\n" {
		t.Errorf("backup = %q, %v", b, err)
	}
	if err := os.WriteFile(path, []byte("- a list\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SaveSettings(path, Defaults()); err != nil {
		t.Fatal(err)
	}
	copies, _ := filepath.Glob(path + ".broken*")
	if len(copies) != 2 {
		t.Errorf("backups = %v, want 2", copies)
	}
}

func TestLoadSettingsOrDefaults(t *testing.T) {
	for _, c := range []struct {
		text string
		line int
	}{{"ui:\n  theme: dark\n  font: \"x\n", 3}, {"- a list\n", 1}} {
		s, probs := LoadSettingsOrDefaults(writeConfig(t, c.text))
		if diff := cmp.Diff(Defaults(), s); diff != "" {
			t.Errorf("%q: (-want +got):\n%s", c.text, diff)
		}
		if len(probs) != 1 || probs[0].Line != c.line || !strings.Contains(probs[0].Msg, "config.yaml.broken") ||
			strings.HasPrefix(probs[0].Msg, "config") {
			t.Errorf("%q: problems = %v", c.text, probs)
		}
	}
	s, probs := LoadSettingsOrDefaults(writeConfig(t, "ui:\n  theme: dark\n  size: 3\n"))
	if s.UI.Theme != "dark" || len(probs) != 1 || probs[0].Path != "ui.size" {
		t.Errorf("a good file loads as usual: %v, %v", s.UI, probs)
	}
}

// Entries rejected at load stay in the file at an unrelated save; one the
// user removes is removed.
func TestSaveKeepsRejectedEntries(t *testing.T) {
	path := writeConfig(t, `llm:
  provider_max_parallel_calls:
    gemini: 2
    groq: 0
  providers:
    good:
      kind: ollama
      models: []
    odd:
      kind: bogus # from a newer version
      models: []
    compat:
      kind: openai_compatible
      models: []
`)
	s, probs, err := LoadSettings(path)
	if err != nil || len(probs) != 3 {
		t.Fatalf("LoadSettings = %v, %v", probs, err)
	}
	s.UI.Theme = "dark"
	if err := SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	for _, want := range []string{"groq: 0", "gemini: 2", "good:", "odd:", "kind: bogus # from a newer version", "compat:", "theme: dark"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("saved file is missing %q:\n%s", want, b)
		}
	}
	got, probs2, err := LoadSettings(path)
	if err != nil || len(probs2) != 3 {
		t.Fatalf("reload = %v, %v", probs2, err)
	}
	if diff := cmp.Diff(s, got); diff != "" {
		t.Errorf("reload (-want +got):\n%s", diff)
	}

	delete(s.LLM.Providers, "good")
	delete(s.LLM.ProviderMaxParallelCalls, "gemini")
	if err := SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	if strings.Contains(string(b), "good:") || strings.Contains(string(b), "gemini:") {
		t.Errorf("removed entries are still there:\n%s", b)
	}
	if !strings.Contains(string(b), "odd:") || !strings.Contains(string(b), "groq: 0") {
		t.Errorf("rejected entries are gone:\n%s", b)
	}
}

// Comments and unknown keys inside a provider and its models survive a save.
func TestSaveKeepsCommentsInEntries(t *testing.T) {
	path := writeConfig(t, `llm:
  providers:
    p:
      kind: openai_compatible # my proxy
      # where it runs
      base_url: http://localhost:8080/v1
      region: eu # not known to this version
      models:
        # the main one
        - id: big
          context: 1000 # set by hand
        - id: old
`)
	s, probs, err := LoadSettings(path)
	if err != nil || len(probs) != 0 { // unknown keys in a map entry aren't checked
		t.Fatalf("LoadSettings = %v, %v", probs, err)
	}
	p := s.LLM.Providers["p"]
	p.BaseURL = "http://localhost:9090/v1"
	p.Models = []ModelSettings{{ID: "big", Context: 2000}, {ID: "new"}}
	s.LLM.Providers = map[string]ProviderSettings{"p": p}
	if err := SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	text := string(b)
	for _, want := range []string{
		"kind: openai_compatible # my proxy",
		"# where it runs",
		"base_url: http://localhost:9090/v1",
		"region: eu # not known to this version",
		"# the main one",
		"- id: big",
		"context: 2000 # set by hand",
		"- id: new",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("saved file is missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "id: old") {
		t.Errorf("a removed model is still there:\n%s", text)
	}
	got, _, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(s, got); diff != "" {
		t.Errorf("reload (-want +got):\n%s", diff)
	}

	// An emptied omitempty field is removed, not left with its old value.
	p.Kind, p.BaseURL = "ollama", ""
	p.Models[0].Context = 0
	s.LLM.Providers["p"] = p
	if err := SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := LoadSettings(path); !cmp.Equal(s, got) {
		t.Errorf("reload after clearing (-want +got):\n%s", cmp.Diff(s, got))
	}
	b, _ = os.ReadFile(path)
	if strings.Contains(string(b), "base_url") || strings.Contains(string(b), "context: 2000") {
		t.Errorf("cleared fields are still there:\n%s", b)
	}
}

func TestPaths(t *testing.T) {
	p, err := DefaultPaths("Jenab")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{p.Root, p.Settings, p.Registry, p.Logs, p.DataFolder, p.Projects} {
		if !filepath.IsAbs(path) {
			t.Errorf("%q is not absolute", path)
		}
	}
	if filepath.Dir(p.Settings) != p.Root || filepath.Dir(p.Projects) != p.DataFolder {
		t.Errorf("paths = %+v", p)
	}
	other := filepath.Join(t.TempDir(), "Data")
	q := p.WithDataFolder(other)
	if q.Projects != filepath.Join(other, "projects") || q.Settings != p.Settings {
		t.Errorf("WithDataFolder = %+v", q)
	}
	if r := p.WithDataFolder(""); r != p {
		t.Errorf("an empty folder changed the paths: %+v", r)
	}
}

func TestHistoryMaxAboveMin(t *testing.T) {
	s, probs, err := LoadSettings(writeConfig(t, `context:
  history_min_turns: 5
  history_max_turns: 5
`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Context.HistoryMinTurns != 5 || s.Context.HistoryMaxTurns != Defaults().Context.HistoryMaxTurns || len(probs) != 1 {
		t.Errorf("min %d, max %d, problems %v; want max back at the default", s.Context.HistoryMinTurns, s.Context.HistoryMaxTurns, probs)
	}
}

// A {} map that gets entries keeps its comment on its own line, not on the
// next key's.
func TestSaveFilledMapKeepsItsComment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	s := Defaults()
	s.LLM.Models = map[string]string{"default": "p/a"}
	s.LLM.ProviderMaxParallelCalls = map[string]int{"p": 2}
	s.LLM.Providers = map[string]ProviderSettings{"p": {Kind: "ollama", Models: []ModelSettings{{ID: "a"}}}}
	for range 2 {
		if err := SaveSettings(path, s); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(path)
	lines := strings.Split(string(b), "\n")
	for key, comment := range map[string]string{
		"  models:":                      "# aliases such as default",
		"  providers:":                   "# connected providers",
		"  provider_max_parallel_calls:": "# background calls per provider",
		"scheduler:":                     "",
	} {
		i := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, key) })
		if i < 0 {
			t.Fatalf("no %q in:\n%s", key, b)
		}
		if comment == "" && strings.Contains(lines[i], "#") || comment != "" && !strings.Contains(lines[i], comment) {
			t.Errorf("line %q, want comment %q:\n%s", lines[i], comment, b)
		}
	}
	// Emptied again, it is {} with its comment.
	s.LLM.ProviderMaxParallelCalls = map[string]int{}
	if err := SaveSettings(path, s); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	if !strings.Contains(string(b), "provider_max_parallel_calls: {} # background calls per provider") {
		t.Errorf("emptied map:\n%s", b)
	}
}
