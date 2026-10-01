package main

// Scenario files (YAML). One file per scenario, plus scenarios/_defaults.yaml with the fake-tool
// delays and rules that every scenario starts from. A scenario's own delays and rules are checked
// first, then the defaults.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type Msg struct {
	Text string `yaml:"text" json:"text"`
	// At is the virtual time since the scenario start ("6s"). Empty: the message is sent once the
	// chat is idle (no turn running, no background work left, no notice pending).
	At string `yaml:"at" json:"at,omitempty"`
}

type Rule struct {
	Match  string `yaml:"match" json:"match,omitempty"` // regex on the raw tool arguments (JSON)
	Result string `yaml:"result" json:"result"`         // tool result, or finish text for background tools
	Delay  string `yaml:"delay" json:"delay,omitempty"` // overrides the tool delay
	re     *regexp.Regexp
}

type Assert struct {
	Type string `yaml:"type" json:"type"`
	Test string `yaml:"test" json:"test"` // "1".."6", or "info" (reported, not part of a test's pass)
	// generic fields; each assertion type uses some of them
	Match      string   `yaml:"match" json:"match,omitempty"`
	MatchAll   []string `yaml:"match_all" json:"match_all,omitempty"`
	Tool       string   `yaml:"tool" json:"tool,omitempty"`
	Tools      []string `yaml:"tools" json:"tools,omitempty"`
	ArgsMatch  string   `yaml:"args_match" json:"args_match,omitempty"`
	ArgsNot    string   `yaml:"args_not_match" json:"args_not_match,omitempty"`
	Turn       string   `yaml:"turn" json:"turn,omitempty"` // first, finish, last, any, after_message
	N          int      `yaml:"n" json:"n,omitempty"`
	MaxOverlap float64  `yaml:"max_overlap" json:"max_overlap,omitempty"`
	Markers    []string `yaml:"markers" json:"markers,omitempty"`
	ChatID     string   `yaml:"chat_id" json:"chat_id,omitempty"`
	Expect     *bool    `yaml:"expect" json:"expect,omitempty"`
	RoleMatch  string   `yaml:"role_match" json:"role_match,omitempty"`
	AnswerRe   string   `yaml:"answer_match" json:"answer_match,omitempty"`
}

type Scenario struct {
	ID      string             `yaml:"id"`
	Title   string             `yaml:"title"`
	Tests   []string           `yaml:"tests"`
	Context string             `yaml:"context"` // main or mother
	Why     string             `yaml:"why"`     // the expected good behaviour, in words
	Msgs    []Msg              `yaml:"messages"`
	Delays  map[string]string  `yaml:"delays"`
	Rules   map[string][]*Rule `yaml:"rules"`
	Asserts []Assert           `yaml:"asserts"`
	File    string             `yaml:"-"`
}

type Defaults struct {
	Delays map[string]string  `yaml:"delays"`
	Rules  map[string][]*Rule `yaml:"rules"`
}

var defaults Defaults

func strictUnmarshal(b []byte, v any) error {
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	return dec.Decode(v)
}

func compileRules(rs map[string][]*Rule, where string) error {
	for tool, list := range rs {
		for i, r := range list {
			if r.Match == "" {
				continue
			}
			re, err := regexp.Compile(r.Match)
			if err != nil {
				return fmt.Errorf("%s: rules.%s[%d]: %v", where, tool, i, err)
			}
			r.re = re
		}
	}
	return nil
}

func loadScenarios(dir string) ([]*Scenario, error) {
	b, err := os.ReadFile(filepath.Join(dir, "_defaults.yaml"))
	if err != nil {
		return nil, err
	}
	if err := strictUnmarshal(b, &defaults); err != nil {
		return nil, fmt.Errorf("_defaults.yaml: %v", err)
	}
	if err := compileRules(defaults.Rules, "_defaults.yaml"); err != nil {
		return nil, err
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	sort.Strings(files)
	var out []*Scenario
	seen := map[string]bool{}
	for _, f := range files {
		if strings.HasPrefix(filepath.Base(f), "_") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var s Scenario
		if err := strictUnmarshal(b, &s); err != nil {
			return nil, fmt.Errorf("%s: %v", filepath.Base(f), err)
		}
		s.File = filepath.Base(f)
		if s.ID == "" || seen[s.ID] {
			return nil, fmt.Errorf("%s: missing or duplicate id", s.File)
		}
		seen[s.ID] = true
		if s.Context != "main" && s.Context != "mother" {
			return nil, fmt.Errorf("%s: context must be main or mother", s.File)
		}
		if len(s.Msgs) == 0 {
			return nil, fmt.Errorf("%s: no messages", s.File)
		}
		if err := compileRules(s.Rules, s.File); err != nil {
			return nil, err
		}
		for k, v := range s.Delays {
			if _, err := time.ParseDuration(v); err != nil {
				return nil, fmt.Errorf("%s: delays.%s: %v", s.File, k, err)
			}
		}
		for i, m := range s.Msgs {
			if m.At != "" {
				if _, err := time.ParseDuration(m.At); err != nil {
					return nil, fmt.Errorf("%s: messages[%d].at: %v", s.File, i, err)
				}
			}
		}
		for i, a := range s.Asserts {
			if !knownAssert[a.Type] {
				return nil, fmt.Errorf("%s: asserts[%d]: unknown type %q", s.File, i, a.Type)
			}
			for _, re := range append(append([]string{a.Match, a.ArgsMatch, a.ArgsNot, a.RoleMatch, a.AnswerRe}, a.MatchAll...), a.Markers...) {
				if re == "" {
					continue
				}
				if _, err := regexp.Compile(re); err != nil {
					return nil, fmt.Errorf("%s: asserts[%d]: %v", s.File, i, err)
				}
			}
		}
		out = append(out, &s)
	}
	return out, nil
}

func secs(d string) float64 {
	v, err := time.ParseDuration(d)
	if err != nil {
		return 0
	}
	return v.Seconds()
}

// delay of a tool: scenario, then defaults, then 1 s.
func (s *Scenario) delay(tool string) float64 {
	if d, ok := s.Delays[tool]; ok {
		return secs(d)
	}
	if d, ok := defaults.Delays[tool]; ok {
		return secs(d)
	}
	return 1
}

// rule finds the first matching rule for a tool call: scenario rules, then defaults.
func (s *Scenario) rule(tool, args string) *Rule {
	for _, list := range [][]*Rule{s.Rules[tool], defaults.Rules[tool]} {
		for _, r := range list {
			if r.re == nil || r.re.MatchString(args) {
				return r
			}
		}
	}
	return nil
}
