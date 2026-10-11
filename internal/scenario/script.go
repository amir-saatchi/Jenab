package scenario

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// Script is scripted answers for runs without a model: per scenario, the
// replies the model gives, in order, played through the real orchestrator
// (-model scripted:<file>). A regression run then needs no keys. When a
// scenario's replies run out, the model answers "ok".
type Script struct {
	Name      string                   `yaml:"-"` // the model's ID in reports
	Scenarios map[string][]ScriptReply `yaml:"scenarios"`
}

// ScriptReply is one answer: text, tool calls, or both.
type ScriptReply struct {
	Text  string       `yaml:"text"`
	Calls []ScriptCall `yaml:"calls"`
	Usage ScriptUsage  `yaml:"usage"`
}

// ScriptCall is a tool call.
type ScriptCall struct {
	Name string         `yaml:"name"`
	Args map[string]any `yaml:"args"`
}

// ScriptUsage is what the provider reports for the reply, so the token and
// cache numbers have something to show.
type ScriptUsage struct {
	Input      int `yaml:"input"`
	CacheRead  int `yaml:"cache_read"`
	CacheWrite int `yaml:"cache_write"`
	Output     int `yaml:"output"`
}

// ReadScript reads a script file. The model is named after the file,
// without a leading _ and the extension.
func ReadScript(path string) (*Script, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Script
	if err := strict(b, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(s.Scenarios) == 0 {
		return nil, fmt.Errorf("%s: no scenarios", path)
	}
	for id, rs := range s.Scenarios {
		for i, r := range rs {
			if strings.TrimSpace(r.Text) == "" && len(r.Calls) == 0 {
				return nil, fmt.Errorf("%s: %s[%d]: a reply needs text or calls", path, id, i)
			}
			for j, c := range r.Calls {
				if !toolName.MatchString(c.Name) {
					return nil, fmt.Errorf("%s: %s[%d].calls[%d]: bad tool name %q", path, id, i, j, c.Name)
				}
			}
		}
	}
	s.Name = strings.TrimPrefix(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), "_")
	if s.Name == "" {
		return nil, errors.New("scenario: a script file needs a name")
	}
	return &s, nil
}

// scriptedProvider is the provider name of a scripted run, and
// scriptedKind its API kind. Nothing is sent anywhere: the run's fake
// provider answers.
const (
	scriptedProvider = "scripted"
	scriptedKind     = provider.KindCompatible
)

// ScriptedModels are the Models of a scripted run: one model, named after
// the script, with a placeholder key.
func ScriptedModels(s *Script) *Models {
	return Static([]ProviderSpec{{Name: scriptedProvider, Kind: string(scriptedKind), BaseURL: "https://scripted.invalid/v1/",
		Models: []config.ModelSettings{{ID: s.Name, Context: 128000}}}},
		map[string]string{scriptedProvider: "no-key"}, scriptedProvider+"/"+s.Name)
}

// replies are a scenario's answers for the fake provider.
func (s *Script) replies(scenario string) []fake.Reply {
	var out []fake.Reply
	for i, r := range s.Scenarios[scenario] {
		var ev []provider.Event
		if r.Text != "" {
			ev = append(ev, provider.Event{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: r.Text}}})
		}
		for j, c := range r.Calls {
			args, err := json.Marshal(c.Args)
			if err != nil || c.Args == nil {
				args = []byte(`{}`)
			}
			call := provider.ToolCall(fmt.Sprintf("call_%d_%d", i+1, j+1), c.Name, string(args), "")
			ev = append(ev, provider.Event{Kind: provider.EventPart, Part: &chat.Part{Kind: chat.PartToolCall, ToolCall: call}})
		}
		stop := provider.StopEnd
		if len(r.Calls) > 0 {
			stop = provider.StopToolUse
		}
		u := chat.Usage{Input: r.Usage.Input, CacheRead: r.Usage.CacheRead, CacheWrite: r.Usage.CacheWrite, Output: r.Usage.Output}
		out = append(out, fake.Reply{Events: append(ev, fake.Done(stop, u))})
	}
	return out
}
