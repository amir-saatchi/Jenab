package main

// The set's long chats: chats/<chat>.yaml, each a list of earlier turns
// with their calls, as the app would have stored them.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

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

// loadChat reads one chat's earlier turns.
func loadChat(dir, name string) ([]seedTurn, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "chats", name+".yaml"))
	if err != nil {
		return nil, err
	}
	var c struct {
		Turns []seedTurn `yaml:"turns"`
	}
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	for i, t := range c.Turns {
		if strings.TrimSpace(t.User) == "" || strings.TrimSpace(t.Answer) == "" {
			return nil, fmt.Errorf("%s: turn %d has no user text or no answer", name, i+1)
		}
		for _, sc := range t.Calls {
			if sc.Name == "" || sc.Result == "" {
				return nil, fmt.Errorf("%s: turn %d has a call without a name or a result", name, i+1)
			}
		}
	}
	return c.Turns, nil
}

// chatOf is the chat a scenario follows up: its ID up to the first "_".
func chatOf(scenario string) string {
	c, _, _ := strings.Cut(scenario, "_")
	return c
}
