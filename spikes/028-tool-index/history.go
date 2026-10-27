package main

// Long chats: the earlier turns the harness writes into the chat before
// the scenario's message, through the runner's Setup hook.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
)

// historyDir has _filler.yaml and one file per long chat, by scenario ID.
var historyDir = filepath.Join("scenarios", "long-history")

type seedCall struct {
	Name   string         `yaml:"name"`
	Args   map[string]any `yaml:"args"`
	Result string         `yaml:"result"`
}

type seedTurn struct {
	User   string     `yaml:"user"`
	Calls  []seedCall `yaml:"calls"`
	Answer string     `yaml:"answer"`
}

// history is a long chat's earlier turns and the turn its follow-up needs.
type history struct {
	Turns []seedTurn
	At    int // the needed turn, 1-based
}

// loadHistory reads the filler and the chat's own turn, which goes in at
// its place.
func loadHistory(dir, scenario string) (history, error) {
	var filler struct {
		Turns []seedTurn `yaml:"turns"`
	}
	var own struct {
		At   int      `yaml:"at"`
		Turn seedTurn `yaml:"turn"`
	}
	if err := readYAML(filepath.Join(dir, "_filler.yaml"), &filler); err != nil {
		return history{}, err
	}
	if err := readYAML(filepath.Join(dir, scenario+".yaml"), &own); err != nil {
		return history{}, err
	}
	if own.At < 1 || own.At > len(filler.Turns)+1 {
		return history{}, fmt.Errorf("%s: at %d is outside 1..%d", scenario, own.At, len(filler.Turns)+1)
	}
	h := history{At: own.At}
	h.Turns = append(h.Turns, filler.Turns[:own.At-1]...)
	h.Turns = append(h.Turns, own.Turn)
	h.Turns = append(h.Turns, filler.Turns[own.At-1:]...)
	return h, nil
}

func readYAML(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// seed writes the turns into the chat as the app would have: the user's
// message, the calls, their results and the answer. With index, a call of
// a tool outside the core is written as call_tool, as condition 2 makes it.
func seed(ctx context.Context, p *project.Project, c id.Chat, turns []seedTurn, index bool) error {
	text := func(s string) []chat.Part { return []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: s}}} }
	for i, t := range turns {
		n := i + 1
		ms := []chat.Message{{Chat: c, Turn: n, Role: chat.RoleUser, Parts: text(t.User)}}
		if len(t.Calls) > 0 {
			var calls, results []chat.Part
			for k, sc := range t.Calls {
				name, args := sc.Name, any(sc.Args)
				if index && !isCore(name) {
					name, args = callName, map[string]any{"name": sc.Name, "arguments": sc.Args}
				}
				raw, err := json.Marshal(args)
				if err != nil {
					return err
				}
				cid := fmt.Sprintf("seed-%d-%d", n, k+1)
				calls = append(calls, chat.Part{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: cid, Name: name, Args: raw}})
				results = append(results, chat.Part{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: cid, Text: sc.Result}})
			}
			ms = append(ms,
				chat.Message{Chat: c, Turn: n, Role: chat.RoleAssistant, Parts: calls},
				chat.Message{Chat: c, Turn: n, Role: chat.RoleTool, Parts: results})
		}
		ms = append(ms, chat.Message{Chat: c, Turn: n, Role: chat.RoleAssistant, Parts: text(t.Answer)})
		for _, m := range ms {
			if _, _, err := p.Chats.AppendMessage(ctx, m); err != nil {
				return fmt.Errorf("turn %d: %w", n, err)
			}
		}
	}
	return nil
}
