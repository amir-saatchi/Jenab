package config

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// defaultYAML holds every setting with its default and a comment. A new
// config.yaml is written from it, so the defaults live in one place.
//
//go:embed default.yaml
var defaultYAML []byte

// Settings are the user settings in config.yaml (SPEC 2.1).
type Settings struct {
	DataFolder string            `yaml:"data_folder" json:"data_folder"` // "" means ~/Jenab
	Context    ContextSettings   `yaml:"context" json:"context"`
	LLM        LLMSettings       `yaml:"llm" json:"llm"`
	Scheduler  SchedulerSettings `yaml:"scheduler" json:"scheduler"`
	Approvals  ApprovalSettings  `yaml:"approvals" json:"approvals"`
	UI         UISettings        `yaml:"ui" json:"ui"`
	Updates    UpdateSettings    `yaml:"updates" json:"updates"`
	DevTools   bool              `yaml:"dev_tools" json:"dev_tools"`
}

// ContextSettings size the history window (SPEC 3.6).
type ContextSettings struct {
	HistoryMinTurns   int     `yaml:"history_min_turns" json:"history_min_turns"`
	HistoryMaxTurns   int     `yaml:"history_max_turns" json:"history_max_turns"`
	HistoryMaxTokens  int     `yaml:"history_max_tokens" json:"history_max_tokens"`
	ToolPreviewTokens int     `yaml:"tool_preview_tokens" json:"tool_preview_tokens"`
	TurnTrimRatio     float64 `yaml:"turn_trim_ratio" json:"turn_trim_ratio"`
}

// LLMSettings are the limits for model calls (SPEC 7.6, 8.3) and the model
// aliases (SPEC 3.9).
type LLMSettings struct {
	MaxParallelCalls          int               `yaml:"max_parallel_calls" json:"max_parallel_calls"`                   // background calls; chats never wait
	ProviderMaxParallelCalls  map[string]int    `yaml:"provider_max_parallel_calls" json:"provider_max_parallel_calls"` // provider name → background calls; a local Ollama defaults to 1
	MaxBackgroundTasksPerChat int               `yaml:"max_background_tasks_per_chat" json:"max_background_tasks_per_chat"`
	MaxSubagentsPerChat       int               `yaml:"max_subagents_per_chat" json:"max_subagents_per_chat"`
	TurnMaxRequests           int               `yaml:"turn_max_requests" json:"turn_max_requests"`
	SystemTurnMaxRequests     int               `yaml:"system_turn_max_requests" json:"system_turn_max_requests"`
	SystemTurnMaxTokens       int               `yaml:"system_turn_max_tokens" json:"system_turn_max_tokens"`
	Models                    map[string]string `yaml:"models" json:"models"` // alias → model, e.g. "default"
	// Providers are the connected providers by name, e.g. "gemini" or
	// "zai". Their keys are in the OS keychain as "provider:<name>".
	Providers map[string]ProviderSettings `yaml:"providers" json:"providers"`
}

// ProviderSettings is one connected provider (SPEC 3.9).
type ProviderSettings struct {
	Kind string `yaml:"kind" json:"kind"` // anthropic, openai, gemini, openai_compatible or ollama
	// BaseURL is needed for openai_compatible; "" means the kind's own. It
	// may hold placeholders such as {account_id}, whose values are kept in
	// the keychain like the key, not in this file.
	BaseURL string `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	// Models are the models that are on, as the provider names them.
	Models []ModelSettings `yaml:"models" json:"models"`
}

// ModelSettings is a model that is on.
type ModelSettings struct {
	ID      string `yaml:"id" json:"id"`
	Context int    `yaml:"context,omitempty" json:"context,omitempty"` // tokens; for models the catalog doesn't know
}

// ProviderKinds are the kinds a provider can have.
var ProviderKinds = []string{"anthropic", "openai", "gemini", "openai_compatible", "ollama"}

type SchedulerSettings struct {
	MaxParallelRuns int `yaml:"max_parallel_runs" json:"max_parallel_runs"`
}

type ApprovalSettings struct {
	DefaultLevel string `yaml:"default_level" json:"default_level"` // strict, standard, auto (SPEC 8.8)
}

type UISettings struct {
	Theme string `yaml:"theme" json:"theme"` // light, dark, system (SPEC 5.11)
}

type UpdateSettings struct {
	Mode    string `yaml:"mode" json:"mode"`       // automatic, notify, off (SPEC 2.8)
	Channel string `yaml:"channel" json:"channel"` // stable, beta
}

// Defaults returns the settings of a new install.
func Defaults() Settings {
	var s Settings
	if err := yaml.Unmarshal(defaultYAML, &s); err != nil {
		panic("config: default.yaml: " + err.Error()) // caught by the tests
	}
	return s
}

// Problem is a setting that was ignored: unknown, of the wrong type or out
// of range. The default is used instead.
type Problem struct {
	Path string `json:"path"` // e.g. "context.history_min_turns"
	Line int    `json:"line"`
	Msg  string `json:"msg"`
}

func (p Problem) String() string {
	if p.Path == "" && p.Line == 0 {
		return "config.yaml: " + p.Msg
	}
	if p.Path == "" {
		return fmt.Sprintf("config.yaml:%d: %s", p.Line, p.Msg)
	}
	return fmt.Sprintf("config.yaml:%d: %s: %s", p.Line, p.Path, p.Msg)
}

// LoadSettings reads config.yaml. A missing file gives the defaults. Unknown
// keys (for example from a newer version), wrong types and values out of
// range are returned as problems and replaced by their defaults, so the app
// still starts. Only a file that can't be read or parsed is an error.
func LoadSettings(path string) (Settings, []Problem, error) {
	src, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Defaults(), nil, nil
	}
	if err != nil {
		return Defaults(), nil, fmt.Errorf("config: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return Defaults(), nil, fmt.Errorf("config: %s: %w", filepath.Base(path), err)
	}
	root := docRoot(&doc)
	if root == nil {
		return Defaults(), nil, nil
	}
	if root.Kind != yaml.MappingNode {
		return Defaults(), nil, fmt.Errorf("config: %s: line %d: expected settings, found %s", filepath.Base(path), root.Line, kindName(root))
	}
	return decode(root)
}

// LoadSettingsOrDefaults is LoadSettings for the start of the app: a file
// that can't be read or parsed gives the defaults and a problem saying so,
// so the app still starts (SPEC 2.1). A save keeps a copy of the file.
func LoadSettingsOrDefaults(path string) (Settings, []Problem) {
	s, probs, err := LoadSettings(path)
	if err == nil {
		return s, probs
	}
	msg := strings.TrimPrefix(err.Error(), "config: ")
	msg = strings.TrimPrefix(msg, filepath.Base(path)+": ")
	msg = strings.TrimPrefix(msg, "yaml: ")
	var line int
	if _, err := fmt.Sscanf(msg, "line %d:", &line); err == nil {
		_, msg, _ = strings.Cut(msg, ": ")
	}
	return Defaults(), []Problem{{Line: line, Msg: msg + "; the file can't be used, so every setting has its default." +
		" Fix it, or save in Settings, which keeps a copy of it as " + filepath.Base(path) + ".broken"}}
}

// decode reads the settings in root, a mapping.
func decode(root *yaml.Node) (Settings, []Problem, error) {
	s := Defaults()
	var probs []Problem
	if err := root.Decode(&s); err != nil {
		var te *yaml.TypeError
		if !errors.As(err, &te) {
			return Defaults(), nil, fmt.Errorf("config: %w", err)
		}
		for _, msg := range te.Errors { // "line 3: cannot unmarshal …"; the field keeps its default
			probs = append(probs, typeProblem(msg))
		}
	}
	probs = append(probs, unknownKeys(root, reflect.TypeFor[Settings](), "")...)
	probs = append(probs, s.check(root)...)
	return s, probs, nil
}

func typeProblem(msg string) Problem {
	var line int
	if _, err := fmt.Sscanf(msg, "line %d:", &line); err == nil {
		_, msg, _ = strings.Cut(msg, ": ")
	}
	return Problem{Line: line, Msg: msg + "; the default is used"}
}

// unknownKeys lists keys in a mapping that t has no field for.
func unknownKeys(n *yaml.Node, t reflect.Type, prefix string) []Problem {
	if n.Kind != yaml.MappingNode || t.Kind() != reflect.Struct {
		return nil
	}
	var probs []Problem
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		path := prefix + k.Value
		f, ok := fieldByKey(t, k.Value)
		if !ok {
			probs = append(probs, Problem{Path: path, Line: k.Line, Msg: "unknown setting, ignored"})
			continue
		}
		probs = append(probs, unknownKeys(v, f.Type, path+".")...)
	}
	return probs
}

func fieldByKey(t reflect.Type, key string) (reflect.StructField, bool) {
	for i := range t.NumField() {
		f := t.Field(i)
		if name, _, _ := strings.Cut(f.Tag.Get("yaml"), ","); name == key {
			return f, true
		}
	}
	return reflect.StructField{}, false
}

// check replaces values out of range with their defaults.
func (s *Settings) check(root *yaml.Node) []Problem {
	d := Defaults()
	var probs []Problem
	bad := func(path, msg string) {
		probs = append(probs, Problem{Path: path, Line: lineOf(root, path), Msg: msg + "; the default is used"})
	}
	atLeast := func(path string, v *int, min, def int) {
		if *v < min {
			bad(path, fmt.Sprintf("%d is below %d", *v, min))
			*v = def
		}
	}
	oneOf := func(path string, v *string, def string, allowed ...string) {
		if !slices.Contains(allowed, *v) {
			bad(path, fmt.Sprintf("%q is not one of %s", *v, strings.Join(allowed, ", ")))
			*v = def
		}
	}

	if s.DataFolder != "" && !filepath.IsAbs(s.DataFolder) {
		bad("data_folder", fmt.Sprintf("%q is not a full path", s.DataFolder))
		s.DataFolder = d.DataFolder
	}

	c, dc := &s.Context, d.Context
	atLeast("context.history_min_turns", &c.HistoryMinTurns, 1, dc.HistoryMinTurns)
	// Max at min would cut at every turn, so the prompt cache never holds.
	atLeast("context.history_max_turns", &c.HistoryMaxTurns, c.HistoryMinTurns+1, max(dc.HistoryMaxTurns, c.HistoryMinTurns+1))
	atLeast("context.history_max_tokens", &c.HistoryMaxTokens, 1000, dc.HistoryMaxTokens)
	atLeast("context.tool_preview_tokens", &c.ToolPreviewTokens, 100, dc.ToolPreviewTokens)
	if c.TurnTrimRatio <= 0 || c.TurnTrimRatio > 1 {
		bad("context.turn_trim_ratio", fmt.Sprintf("%g is not between 0 and 1", c.TurnTrimRatio))
		c.TurnTrimRatio = dc.TurnTrimRatio
	}

	l, dl := &s.LLM, d.LLM
	atLeast("llm.max_parallel_calls", &l.MaxParallelCalls, 1, dl.MaxParallelCalls)
	for p, n := range l.ProviderMaxParallelCalls {
		if n < 1 {
			bad("llm.provider_max_parallel_calls."+p, fmt.Sprintf("%d is below 1", n))
			delete(l.ProviderMaxParallelCalls, p) // the provider gets the global limit
		}
	}
	atLeast("llm.max_background_tasks_per_chat", &l.MaxBackgroundTasksPerChat, 0, dl.MaxBackgroundTasksPerChat)
	atLeast("llm.max_subagents_per_chat", &l.MaxSubagentsPerChat, 1, dl.MaxSubagentsPerChat)
	atLeast("llm.turn_max_requests", &l.TurnMaxRequests, 2, dl.TurnMaxRequests) // the last request has no tools (8.3)
	atLeast("llm.system_turn_max_requests", &l.SystemTurnMaxRequests, 2, dl.SystemTurnMaxRequests)
	atLeast("llm.system_turn_max_tokens", &l.SystemTurnMaxTokens, 1000, dl.SystemTurnMaxTokens)
	if l.Models == nil {
		l.Models = map[string]string{}
	}
	if l.ProviderMaxParallelCalls == nil {
		l.ProviderMaxParallelCalls = map[string]int{}
	}
	if l.Providers == nil {
		l.Providers = map[string]ProviderSettings{}
	}
	for name, p := range l.Providers {
		path := "llm.providers." + name
		switch {
		case name == "" || strings.ContainsAny(name, "/: "):
			bad(path, fmt.Sprintf("%q is not a usable name: no slash, colon or space", name))
		case !slices.Contains(ProviderKinds, p.Kind):
			bad(path+".kind", fmt.Sprintf("%q is not one of %s", p.Kind, strings.Join(ProviderKinds, ", ")))
		case p.Kind == "openai_compatible" && p.BaseURL == "":
			bad(path+".base_url", "an OpenAI-compatible provider needs a base URL")
		default:
			continue
		}
		delete(l.Providers, name) // the provider is left out until it is fixed
	}

	atLeast("scheduler.max_parallel_runs", &s.Scheduler.MaxParallelRuns, 1, d.Scheduler.MaxParallelRuns)
	oneOf("approvals.default_level", &s.Approvals.DefaultLevel, d.Approvals.DefaultLevel, "strict", "standard", "auto")
	oneOf("ui.theme", &s.UI.Theme, d.UI.Theme, "light", "dark", "system")
	oneOf("updates.mode", &s.Updates.Mode, d.Updates.Mode, "automatic", "notify", "off")
	oneOf("updates.channel", &s.Updates.Channel, d.Updates.Channel, "stable", "beta")
	return probs
}

// lineOf returns the line of a dotted key, or 0 if the file doesn't have it.
func lineOf(root *yaml.Node, path string) int {
	n := root
	line := 0
	for key := range strings.SplitSeq(path, ".") {
		k, v := lookup(n, key)
		if k == nil {
			return line
		}
		line, n = k.Line, v
	}
	return line
}

func lookup(n *yaml.Node, key string) (k, v *yaml.Node) {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i], n.Content[i+1]
		}
	}
	return nil, nil
}

// SaveSettings writes s to path. Values in the existing file are updated in
// place, so the user's comments and keys this version doesn't know stay. A
// new file starts from default.yaml, with its comments. Map entries that
// were left out at load (a provider with an unknown kind, say) stay in the
// file until they are fixed. A file that can't be parsed is copied to
// config.yaml.broken first. The file is replaced in one step, so a crash
// leaves the old or the new file.
func SaveSettings(path string, s Settings) error {
	base, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		base = defaultYAML
	} else if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(base, &doc); err != nil || docRoot(&doc) == nil || docRoot(&doc).Kind != yaml.MappingNode {
		// A broken file isn't merged: keep a copy, then start again from
		// the defaults.
		if len(bytes.TrimSpace(base)) > 0 {
			if err := backup(path, base); err != nil {
				return err
			}
		}
		doc = yaml.Node{}
		if err := yaml.Unmarshal(defaultYAML, &doc); err != nil {
			return fmt.Errorf("config: default.yaml: %w", err)
		}
	}
	root := docRoot(&doc)
	// loaded is what LoadSettings makes of the file. An entry the file has
	// but loaded doesn't was rejected at load, not removed by the user.
	cur, _, err := decode(root)
	if err != nil {
		cur = s // can't tell: an entry s doesn't have was removed
	}
	var loaded, next yaml.Node
	if err := loaded.Encode(cur); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := next.Encode(s); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	merge(root, &next, &loaded, reflect.TypeFor[Settings]())

	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return writeFile(path, []byte(b.String()))
}

// backup copies a broken config.yaml to config.yaml.broken, or to a name
// with the time if that is taken, so a save never loses the user's file.
func backup(path string, data []byte) error {
	name := path + ".broken"
	if _, err := os.Stat(name); err == nil {
		name += "-" + time.Now().Format("20060102-150405")
	}
	return writeFile(name, data)
}

// merge copies the values of next into old. Struct fields are merged key by
// key; a known field next leaves out (omitempty) is removed, keys this
// version doesn't know stay. A map such as llm.models gets the keys of
// next, plus the keys of old that loaded doesn't have: those were rejected
// at load, and stay until they are fixed. Entries it already had keep
// their node, so their comments stay, and struct values are merged too. A
// list of items with an id is merged item by item. A scalar is replaced.
// Comments on the old nodes stay. loaded may be nil.
func merge(old, next, loaded *yaml.Node, t reflect.Type) {
	for i := 0; i+1 < len(next.Content); i += 2 {
		nk, nv := next.Content[i], next.Content[i+1]
		f, ok := fieldByKey(t, nk.Value)
		if !ok {
			continue
		}
		ok2, ov := lookup(old, nk.Value)
		_, lv := lookup(loaded, nk.Value)
		switch {
		case ok2 == nil:
			old.Content = append(old.Content, nk, nv)
		case f.Type.Kind() == reflect.Struct && ov.Kind == yaml.MappingNode:
			merge(ov, nv, lv, f.Type)
		case f.Type.Kind() == reflect.Map && ov.Kind == yaml.MappingNode:
			mergeMap(ov, nv, lv, f.Type.Elem())
			lineComment(ok2, ov)
		case f.Type.Kind() == reflect.Slice && ov.Kind == yaml.SequenceNode && hasID(f.Type.Elem()):
			mergeList(ov, nv, f.Type.Elem())
		default:
			setValue(ov, nv)
		}
	}
	content := old.Content[:0]
	for i := 0; i+1 < len(old.Content); i += 2 {
		k := old.Content[i]
		if _, known := fieldByKey(t, k.Value); known {
			if nk, _ := lookup(next, k.Value); nk == nil {
				continue // empty, so next omits it
			}
		}
		content = append(content, k, old.Content[i+1])
	}
	old.Content = content
}

func mergeMap(old, next, loaded *yaml.Node, elem reflect.Type) {
	content := make([]*yaml.Node, 0, len(old.Content)+len(next.Content))
	for i := 0; i+1 < len(old.Content); i += 2 {
		ok, ov := old.Content[i], old.Content[i+1]
		nk, nv := lookup(next, ok.Value)
		if nk == nil {
			if lk, _ := lookup(loaded, ok.Value); lk == nil {
				content = append(content, ok, ov) // rejected at load: kept
			}
			continue // removed
		}
		if elem.Kind() == reflect.Struct && ov.Kind == yaml.MappingNode && nv.Kind == yaml.MappingNode {
			_, lv := lookup(loaded, ok.Value)
			merge(ov, nv, lv, elem)
		} else {
			setValue(ov, nv)
		}
		content = append(content, ok, ov)
	}
	for i := 0; i+1 < len(next.Content); i += 2 {
		if k, _ := lookup(old, next.Content[i].Value); k == nil {
			content = append(content, next.Content[i], next.Content[i+1])
		}
	}
	old.Content = content
	if len(content) == 0 {
		old.Style = yaml.FlowStyle // an empty map as {}
	} else if old.Style == yaml.FlowStyle {
		old.Style = 0 // a map that was {} gets one entry per line
	}
}

// mergeList merges a list of structs with an id, such as a provider's
// models: an item whose id was there keeps its node and comments. The
// items and their order are next's.
func mergeList(old, next *yaml.Node, elem reflect.Type) {
	byID := map[string]*yaml.Node{}
	for _, item := range old.Content {
		if _, v := lookup(item, "id"); v != nil && v.Kind == yaml.ScalarNode {
			if _, dup := byID[v.Value]; !dup {
				byID[v.Value] = item
			}
		}
	}
	content := make([]*yaml.Node, 0, len(next.Content))
	for _, nv := range next.Content {
		var ov *yaml.Node
		if _, id := lookup(nv, "id"); id != nil {
			ov = byID[id.Value]
			delete(byID, id.Value) // a second item with this id is new
		}
		if ov == nil {
			content = append(content, nv)
			continue
		}
		merge(ov, nv, nil, elem)
		content = append(content, ov)
	}
	old.Content = content
	if len(content) == 0 {
		old.Style = yaml.FlowStyle // an empty list as []
	} else if old.Style == yaml.FlowStyle {
		old.Style = 0
	}
}

// hasID reports whether t is a struct with an id field.
func hasID(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}
	_, ok := fieldByKey(t, "id")
	return ok
}

// lineComment keeps a map's line comment on its key's line. The encoder
// writes a {} value's comment after it, on the same line, but a block
// map's after its last entry, where it would look like the next key's.
func lineComment(k, v *yaml.Node) {
	if v.Style == yaml.FlowStyle {
		if v.LineComment == "" {
			k.LineComment, v.LineComment = "", k.LineComment
		}
		return
	}
	if k.LineComment == "" {
		k.LineComment, v.LineComment = v.LineComment, ""
	}
}

func setValue(old, next *yaml.Node) {
	old.Kind, old.Tag, old.Value, old.Content, old.Style = next.Kind, next.Tag, next.Value, next.Content, next.Style
	if next.Kind == yaml.MappingNode && len(next.Content) == 0 {
		old.Style = yaml.FlowStyle
	}
}

func docRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 {
		return doc.Content[0]
	}
	return nil
}

func kindName(n *yaml.Node) string {
	switch n.Kind {
	case yaml.SequenceNode:
		return "a list"
	case yaml.ScalarNode:
		return "a single value"
	default:
		return "something else"
	}
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op after the rename
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("config: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("config: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return nil
}
