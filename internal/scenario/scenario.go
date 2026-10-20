// Package scenario runs scripted chats on the real orchestrator to test
// agent behaviour across models (SPEC 8.4, TASK-001). A set is a folder:
// _set.yaml with what its scenarios share (the fake tools, the fixture
// project, the skills), and one YAML file per scenario.
package scenario

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/appinfo"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/skill"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// SetFile is the name of a set's shared file.
const SetFile = "_set.yaml"

// Set is a folder of scenarios and what they share.
type Set struct {
	Dir      string
	Title    string
	Card     string            // the project card (3.2), block 4
	Today    time.Time         // the fixture's day, for block 1's date (3.1); zero is the real date
	Fixture  string            // a SQL script for the fixture database
	Data     string            // a SQL script run on the project's own database, for the real data tools
	Configs  map[string]string // stored configs by id, for config tools
	Skills   []skill.Skill     // the set's own skills, besides the built-in ones; one with a built-in's name replaces it
	Chats    []ChatSpec        // other chats in the project
	Tools    []*ToolSpec
	Rules    map[string][]*Rule // fake results every scenario starts from
	Settings map[string]any     // the context and llm settings every scenario starts from
	// FalseRunning is text that says work goes on after the answer, for
	// no_false_running; nil is the English default.
	FalseRunning *regexp.Regexp
	Scenarios    []*Scenario
}

// setFile is _set.yaml.
type setFile struct {
	Title        string             `yaml:"title"`
	Card         string             `yaml:"card"`          // a file in the set
	Today        string             `yaml:"today"`         // YYYY-MM-DD, the day the fixture is from
	Fixture      string             `yaml:"fixture"`       // a .sql file in the set
	Data         string             `yaml:"data"`          // a .sql file in the set, run on the project's database
	Configs      string             `yaml:"configs"`       // a folder of <id>.yaml files
	Skills       string             `yaml:"skills"`        // a folder of skill folders
	MotherSkills []string           `yaml:"mother_skills"` // set skills only the Mother chat lists
	Chats        []ChatSpec         `yaml:"chats"`
	Tools        []*ToolSpec        `yaml:"tools"`
	Rules        map[string][]*Rule `yaml:"rules"`
	Settings     map[string]any     `yaml:"settings"`      // context and llm keys of config.yaml
	FalseRunning []string           `yaml:"false_running"` // regexes, for a set in another language
}

// ChatSpec is a chat the fixture project has besides the one under test,
// so Mother's chat list shows it (8.6).
type ChatSpec struct {
	Title string `yaml:"title"`
	Role  string `yaml:"role"`
	Notes string `yaml:"notes"`
}

// Tool kinds: what a fake tool does with a call.
const (
	KindRules    = "rules"    // the first matching rule's result
	KindSQL      = "sql"      // a read-only SELECT on the fixture database
	KindDescribe = "describe" // a table of the fixture database
	KindConfig   = "config"   // a stored config by id
)

// ToolSpec is a fake tool. It replaces a real tool of the same name.
type ToolSpec struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Schema      map[string]any `yaml:"schema"` // JSON Schema of the arguments
	Kind        string         `yaml:"kind"`   // rules (the default), sql, describe or config
	Arg         string         `yaml:"arg"`    // the argument sql, describe and config read
	Mother      bool           `yaml:"mother"` // only the Mother chat gets it
	// Areas are the parts of a project it works on: tables, pipelines or
	// views. Block 1 names only the parts some tool has (TASK-014).
	Areas []string `yaml:"areas"`
	// Background: a call starts background work (8.3). BackgroundArg
	// names a boolean argument that does so instead, such as a
	// subagent's background.
	Background    bool     `yaml:"background"`
	BackgroundArg string   `yaml:"background_arg"`
	Delay         Duration `yaml:"delay"`
	schema        json.RawMessage
	areas         tool.Areas
}

// Rule is a fake result: Match is a regex on the call's arguments as JSON;
// no Match matches every call.
type Rule struct {
	Match  string   `yaml:"match"`
	Result string   `yaml:"result"`
	Delay  Duration `yaml:"delay"` // instead of the tool's
	Error  bool     `yaml:"error"` // the result is an error the model sees
	re     *regexp.Regexp
}

// Duration is a time.Duration written as "1.5s" in YAML.
type Duration time.Duration

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := time.ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: %v", n.Line, err)
	}
	if v < 0 {
		return fmt.Errorf("line %d: a duration can't be negative", n.Line)
	}
	*d = Duration(v)
	return nil
}

// Scenario is one scripted chat with its checks.
type Scenario struct {
	ID      string   `yaml:"id"`
	Title   string   `yaml:"title"`
	Tests   []string `yaml:"tests"`   // the test groups it belongs to, as labels
	Context string   `yaml:"context"` // main or mother
	Why     string   `yaml:"why"`     // the good behaviour, in words
	// Needs are app features the scenario needs and Phase 1 doesn't have,
	// such as background work; the runner skips it until they come.
	Needs    []string            `yaml:"needs"`
	Role     string              `yaml:"role"`   // the chat's role
	Skills   []string            `yaml:"skills"` // skills loaded in the chat at the start
	Messages []Message           `yaml:"messages"`
	Delays   map[string]Duration `yaml:"delays"`
	Rules    map[string][]*Rule  `yaml:"rules"`
	// Settings are context and llm keys of config.yaml over the set's,
	// such as llm.turn_max_requests.
	Settings map[string]any `yaml:"settings"`
	Asserts  []Assert       `yaml:"asserts"`
	File     string         `yaml:"-"`
	set      *Set
	settings config.Settings // the app settings of its runs, before the models
}

// Message is a scripted user message. Without At it is sent once the chat
// is idle; with At, that long after the scenario's start, also during a
// turn, where it joins the running turn (8.3). With Restart the app
// restarts before it: the chat is idle, the app closes its projects and
// opens them again from disk.
type Message struct {
	Text    string    `yaml:"text"`
	At      *Duration `yaml:"at"`
	Restart bool      `yaml:"restart"`
}

// Assert is one check on a finished run. Each type uses some fields.
type Assert struct {
	Type string `yaml:"type"`
	// Test groups checks in the report; "info" is reported but never
	// fails a scenario.
	Test        string   `yaml:"test"`
	Match       string   `yaml:"match"`
	Tool        string   `yaml:"tool"`
	Tools       []string `yaml:"tools"`
	ArgsMatch   string   `yaml:"args_match"`
	ArgsNot     string   `yaml:"args_not_match"`
	AnswerMatch string   `yaml:"answer_match"`
	Turn        string   `yaml:"turn"` // first, last, finish, any (the default) or a turn number
	N           int      `yaml:"n"`
	MaxOverlap  float64  `yaml:"max_overlap"`
	Markers     []string `yaml:"markers"`
	Skill       string   `yaml:"skill"`
	Skills      []string `yaml:"skills"`
	Allow       []string `yaml:"allow"`
	Min         float64  `yaml:"min"`  // cache_read_share: the least share, from 0 to 1
	When        string   `yaml:"when"` // after_message: during or between (turns)
	// Other holds fields only planned checks use.
	Other map[string]any `yaml:",inline"`
}

// Needs the runner knows. Each is skipped until the app has it.
var knownNeeds = []string{"background"}

// plannedAsserts are checks that come with an app feature, by the need
// they come with. A scenario that uses one needs that feature, so it is
// skipped; their fields aren't checked yet.
var plannedAsserts = map[string]string{
	"background_started":                "background",
	"slow_started":                      "background",
	"mentions_running":                  "background",
	"quick_before_finish":               "background",
	"no_resend_after_notice":            "background",
	"no_restart":                        "background",
	"created_chat_only_for_lasting_job": "background",
	"right_target":                      "background",
	"used_subagent_for_one_off":         "background",
}

// after_message's places for the message.
const (
	whenDuring  = "during"
	whenBetween = "between"
)

var turnNames = []string{"", "any", "first", "last", "finish"}

// Load reads a set: _set.yaml, its files and every other .yaml file as a
// scenario. Unknown fields are errors, so a typo doesn't pass silently.
func Load(dir string) (*Set, error) {
	b, err := os.ReadFile(filepath.Join(dir, SetFile))
	if err != nil {
		return nil, err
	}
	var f setFile
	if err := strict(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", SetFile, err)
	}
	s := &Set{Dir: dir, Title: f.Title, Chats: f.Chats, Tools: f.Tools, Rules: f.Rules, Configs: map[string]string{}}
	if s.Title == "" {
		s.Title = filepath.Base(dir)
	}
	read := func(name string) (string, error) {
		if name == "" {
			return "", nil
		}
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		return strings.ReplaceAll(string(b), "\r\n", "\n"), err
	}
	if s.Card, err = read(f.Card); err != nil {
		return nil, err
	}
	if f.Today != "" {
		if s.Today, err = time.ParseInLocation(time.DateOnly, f.Today, time.Local); err != nil {
			return nil, fmt.Errorf("%s: today %q is not a date as YYYY-MM-DD", SetFile, f.Today)
		}
	}
	if s.Fixture, err = read(f.Fixture); err != nil {
		return nil, err
	}
	if s.Data, err = read(f.Data); err != nil {
		return nil, err
	}
	if f.Configs != "" {
		files, err := filepath.Glob(filepath.Join(dir, filepath.FromSlash(f.Configs), "*.yaml"))
		if err != nil {
			return nil, err
		}
		for _, p := range files {
			c, err := os.ReadFile(p)
			if err != nil {
				return nil, err
			}
			s.Configs[strings.TrimSuffix(filepath.Base(p), ".yaml")] = strings.ReplaceAll(string(c), "\r\n", "\n")
		}
	}
	if f.Skills != "" {
		if s.Skills, err = readSkills(filepath.Join(dir, filepath.FromSlash(f.Skills)), f.MotherSkills); err != nil {
			return nil, err
		}
	} else if len(f.MotherSkills) > 0 {
		return nil, fmt.Errorf("%s: mother_skills needs a skills folder", SetFile)
	}
	if err := s.checkTools(); err != nil {
		return nil, fmt.Errorf("%s: %w", SetFile, err)
	}
	if err := compileRules(s.Rules, s.tool); err != nil {
		return nil, fmt.Errorf("%s: %w", SetFile, err)
	}
	s.Settings = f.Settings
	if _, err := parseSettings(s.Settings); err != nil {
		return nil, fmt.Errorf("%s: settings: %w", SetFile, err)
	}
	if len(f.FalseRunning) > 0 {
		re, err := regexp.Compile(`(?i)(` + strings.Join(f.FalseRunning, "|") + `)`)
		if err != nil {
			return nil, fmt.Errorf("%s: false_running: %w", SetFile, err)
		}
		s.FalseRunning = re
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	seen := map[string]bool{}
	for _, p := range files {
		name := filepath.Base(p)
		if strings.HasPrefix(name, "_") {
			continue
		}
		sc, err := s.loadScenario(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if seen[sc.ID] {
			return nil, fmt.Errorf("%s: the id %s is used twice", name, sc.ID)
		}
		seen[sc.ID] = true
		s.Scenarios = append(s.Scenarios, sc)
	}
	if len(s.Scenarios) == 0 {
		return nil, errors.New("the set has no scenarios")
	}
	return s, nil
}

func strict(b []byte, v any) error {
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	return dec.Decode(v)
}

// readSkills reads each folder of dir as a skill. Names in mother are
// only for the Mother chat.
func readSkills(dir string, mother []string) ([]skill.Skill, error) {
	fsys := os.DirFS(dir)
	ents, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var out []skill.Skill
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		sk, err := skill.Read(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		sk.Mother = slices.Contains(mother, sk.Name)
		out = append(out, sk)
	}
	for _, n := range mother {
		if !slices.ContainsFunc(out, func(sk skill.Skill) bool { return sk.Name == n }) {
			return nil, fmt.Errorf("mother_skills: no skill %s in the set", n)
		}
	}
	return out, nil
}

var toolName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func (s *Set) checkTools() error {
	seen := map[string]bool{}
	for i, t := range s.Tools {
		where := fmt.Sprintf("tools[%d]", i)
		if !toolName.MatchString(t.Name) {
			return fmt.Errorf("%s: bad name %q", where, t.Name)
		}
		if seen[t.Name] {
			return fmt.Errorf("%s: %s is listed twice", where, t.Name)
		}
		seen[t.Name] = true
		if real, ok := realSpec(t.Name); ok && t.Description == "" && t.Schema == nil {
			// A fake of a real tool without its own text takes the
			// real one's, so the model sees what the app sends.
			t.Description, t.schema = real.Description, real.Schema
		} else {
			if t.Description == "" {
				return fmt.Errorf("%s (%s): no description", where, t.Name)
			}
			if t.Schema == nil {
				t.Schema = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			b, err := json.Marshal(t.Schema)
			if err != nil {
				return fmt.Errorf("%s (%s): schema: %v", where, t.Name, err)
			}
			t.schema = b
		}
		for _, a := range t.Areas {
			i := slices.Index(tool.AreaNames, a)
			if i < 0 {
				return fmt.Errorf("%s (%s): unknown area %q; the areas are %s", where, t.Name, a, strings.Join(tool.AreaNames, ", "))
			}
			t.areas |= 1 << i
		}
		switch t.Kind {
		case "":
			t.Kind = KindRules
		case KindRules:
		case KindSQL, KindDescribe, KindConfig:
			if t.Arg == "" {
				return fmt.Errorf("%s (%s): kind %s needs arg, the argument it reads", where, t.Name, t.Kind)
			}
			if t.Kind != KindConfig && s.Fixture == "" {
				return fmt.Errorf("%s (%s): kind %s needs a fixture", where, t.Name, t.Kind)
			}
		default:
			return fmt.Errorf("%s (%s): unknown kind %q; the kinds are rules, sql, describe and config", where, t.Name, t.Kind)
		}
		if t.Background && t.BackgroundArg != "" {
			return fmt.Errorf("%s (%s): background and background_arg exclude each other", where, t.Name)
		}
	}
	return nil
}

// realSpec is the spec of the app's tool of that name.
func realSpec(name string) (tool.Spec, bool) {
	for _, t := range append(tool.Builtin(tool.Deps{}), agent.Tools()...) {
		if s := t.Spec(); s.Name == name {
			return s, true
		}
	}
	return tool.Spec{}, false
}

func (s *Set) tool(name string) *ToolSpec {
	for _, t := range s.Tools {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// compileRules checks rules: each names a fake tool of kind rules, and
// its Match compiles.
func compileRules(rs map[string][]*Rule, tool func(string) *ToolSpec) error {
	for name, list := range rs {
		t := tool(name)
		if t == nil {
			return fmt.Errorf("rules.%s: no such fake tool", name)
		}
		if t.Kind != KindRules {
			return fmt.Errorf("rules.%s: the tool is of kind %s, which has no rules", name, t.Kind)
		}
		for i, r := range list {
			if r.Match == "" {
				continue
			}
			re, err := regexp.Compile(r.Match)
			if err != nil {
				return fmt.Errorf("rules.%s[%d]: %v", name, i, err)
			}
			r.re = re
		}
	}
	return nil
}

func (s *Set) loadScenario(path string) (*Scenario, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sc Scenario
	if err := strict(b, &sc); err != nil {
		return nil, err
	}
	sc.File, sc.set = filepath.Base(path), s
	if sc.ID == "" {
		return nil, errors.New("no id")
	}
	if sc.Context != "main" && sc.Context != "mother" {
		return nil, errors.New("context must be main or mother")
	}
	if sc.Context == "mother" && sc.Role != "" {
		return nil, errors.New("the " + appinfo.Mother + " chat has no role")
	}
	for _, n := range sc.Needs {
		if !slices.Contains(knownNeeds, n) {
			return nil, fmt.Errorf("needs: unknown %q; the runner knows %s", n, strings.Join(knownNeeds, ", "))
		}
	}
	if len(sc.Messages) == 0 {
		return nil, errors.New("no messages")
	}
	for i, m := range sc.Messages {
		if strings.TrimSpace(m.Text) == "" {
			return nil, fmt.Errorf("messages[%d]: no text", i)
		}
		if m.Restart && m.At != nil {
			return nil, fmt.Errorf("messages[%d]: a restart waits until the chat is idle, so it has no at", i)
		}
	}
	if sc.Messages[0].At != nil || sc.Messages[0].Restart {
		return nil, errors.New("messages[0]: the first message starts the scenario, so it has no at or restart")
	}
	for name := range sc.Delays {
		if s.tool(name) == nil {
			return nil, fmt.Errorf("delays.%s: no such fake tool", name)
		}
	}
	if err := compileRules(sc.Rules, s.tool); err != nil {
		return nil, err
	}
	if sc.settings, err = parseSettings(mergeMaps(s.Settings, sc.Settings)); err != nil {
		return nil, fmt.Errorf("settings: %w", err)
	}
	for i, a := range sc.Asserts {
		if need, ok := plannedAsserts[a.Type]; ok {
			if !slices.Contains(sc.Needs, need) {
				return nil, fmt.Errorf("asserts[%d] (%s): the check comes with %s, so the scenario needs [%s]", i, a.Type, need, need)
			}
			continue
		}
		if err := s.checkAssert(a); err != nil {
			return nil, fmt.Errorf("asserts[%d] (%s): %w", i, a.Type, err)
		}
		if a.Type == "cache_read_share" && a.Test == "" {
			sc.Asserts[i].Test = "info" // a model's cache use is no pass or fail by default
		}
	}
	return &sc, nil
}

// parseSettings checks settings as config.yaml's context and llm keys and
// returns the app settings with them. The runner sets the providers and
// models itself.
func parseSettings(m map[string]any) (config.Settings, error) {
	for k, v := range m {
		if k != "context" && k != "llm" {
			return config.Settings{}, fmt.Errorf("%s: only context and llm can be set", k)
		}
		if l, ok := v.(map[string]any); ok && k == "llm" {
			for _, own := range []string{"providers", "models"} {
				if _, ok := l[own]; ok {
					return config.Settings{}, fmt.Errorf("llm.%s: the runner sets it", own)
				}
			}
		}
	}
	b, err := yaml.Marshal(m)
	if err != nil {
		return config.Settings{}, err
	}
	s, probs, err := config.ParseSettings(b)
	if err != nil {
		return config.Settings{}, err
	}
	if len(probs) > 0 {
		msg := strings.TrimSuffix(strings.TrimSuffix(probs[0].Msg, "; the default is used"), ", ignored")
		return config.Settings{}, fmt.Errorf("%s: %s", probs[0].Path, msg)
	}
	return s, nil
}

// mergeMaps is a with b's keys over it, nested maps merged.
func mergeMaps(a, b map[string]any) map[string]any {
	out := maps.Clone(a)
	if out == nil {
		out = map[string]any{}
	}
	for k, v := range b {
		am, ok1 := out[k].(map[string]any)
		bm, ok2 := v.(map[string]any)
		if ok1 && ok2 {
			out[k] = mergeMaps(am, bm)
		} else {
			out[k] = v
		}
	}
	return out
}

func (s *Set) checkAssert(a Assert) error {
	need, ok := assertFields[a.Type]
	if !ok {
		return fmt.Errorf("unknown type; the types are %s", strings.Join(assertTypes(), ", "))
	}
	if len(a.Other) > 0 {
		return fmt.Errorf("unknown field %s", slices.Sorted(maps.Keys(a.Other))[0])
	}
	if n, err := strconv.Atoi(a.Turn); !slices.Contains(turnNames, a.Turn) && (err != nil || n < 1) {
		return fmt.Errorf("turn must be first, last, finish, any or a turn number")
	}
	for _, re := range append([]string{a.Match, a.ArgsMatch, a.ArgsNot, a.AnswerMatch}, a.Markers...) {
		if _, err := regexp.Compile(re); err != nil {
			return err
		}
	}
	if a.When != "" && (a.Type != "after_message" || a.When != whenDuring && a.When != whenBetween) {
		return fmt.Errorf("when is for after_message, and is during or between")
	}
	if a.Min < 0 || a.Min > 1 {
		return fmt.Errorf("min is a share from 0 to 1")
	}
	has := map[string]bool{
		"match": a.Match != "", "tools": a.Tool != "" || len(a.Tools) > 0, "n": a.N > 0,
		"skills": a.Skill != "" || len(a.Skills) > 0, "min": a.Min > 0,
	}
	for _, f := range need {
		if !has[f] {
			return fmt.Errorf("needs %s", f)
		}
	}
	return nil
}

// skip reports why the scenario can't run yet, or "".
func (sc *Scenario) skip() string {
	if len(sc.Needs) > 0 {
		return "needs " + strings.Join(sc.Needs, ", ")
	}
	return ""
}

// delay is how long a call of a fake tool takes: the rule's delay, the
// scenario's, then the tool's.
func (sc *Scenario) delay(t *ToolSpec, r *Rule) time.Duration {
	if r != nil && r.Delay > 0 {
		return time.Duration(r.Delay)
	}
	if d, ok := sc.Delays[t.Name]; ok {
		return time.Duration(d)
	}
	return time.Duration(t.Delay)
}

// rule finds the first rule that matches a call: the scenario's, then the
// set's.
func (sc *Scenario) rule(tool, args string) *Rule {
	for _, list := range [][]*Rule{sc.Rules[tool], sc.set.Rules[tool]} {
		for _, r := range list {
			if r.re == nil || r.re.MatchString(args) {
				return r
			}
		}
	}
	return nil
}
