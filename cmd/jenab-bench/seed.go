package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Chats the seed makes, by title.
const (
	historyChat = "History 200" // 200 messages, for N-02 and N-01
	busyChat    = "Busy"        // the second busy chat of N-54
	otherChats  = 8             // "Call 1" to "Call 8": the 8 other calls of N-54
)

// home is a fresh user folder for the app: LOCALAPPDATA and USERPROFILE
// point into it, so the app has its own settings, projects and WebView2
// profile, and never sees the user's.
type home struct {
	dir string
}

func (h home) env() []string {
	return []string{
		"LOCALAPPDATA=" + filepath.Join(h.dir, "local"),
		"USERPROFILE=" + filepath.Join(h.dir, "user"),
	}
}

func (h home) paths() config.Paths {
	root := filepath.Join(h.dir, "local", appName)
	return config.Paths{Root: root, Settings: filepath.Join(root, "config.yaml"), Registry: filepath.Join(root, "registry.db"),
		Logs: filepath.Join(root, "logs")}.WithDataFolder(filepath.Join(h.dir, "user", appName))
}

// appName is the app's folder name, as cmd/desktop sets it.
const appName = "Jenab"

// writeSettings connects the fake model as a local Ollama and makes it the
// default and fast model. Background calls to it may run 8 at a time.
func (h home) writeSettings(modelURL string) error {
	p := h.paths()
	if err := os.MkdirAll(p.Root, 0o755); err != nil {
		return err
	}
	s := map[string]any{
		"llm": map[string]any{
			"providers": map[string]any{"bench": map[string]any{"kind": "ollama", "base_url": modelURL,
				"models": []map[string]any{{"id": modelName, "context": 262144}}}},
			"models":                      map[string]string{"default": "bench/" + modelName, "fast": "bench/" + modelName},
			"provider_max_parallel_calls": map[string]int{"bench": 8},
		},
	}
	b, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(p.Settings, b, 0o644)
}

// seed makes the project "Bench" with its chats, before the app starts.
// kind is what the 200-message chat holds (historyKinds).
func (h home) seed(ctx context.Context, kind string) error {
	p := h.paths()
	if err := os.MkdirAll(p.Root, 0o755); err != nil {
		return err
	}
	reg, err := store.OpenRegistry(ctx, p.Registry)
	if err != nil {
		return err
	}
	defer reg.Close()
	pm := project.NewManager(project.Deps{Paths: p, Registry: reg})
	defer pm.CloseAll(context.WithoutCancel(ctx))
	pid, err := pm.Create(ctx, "Bench")
	if err != nil {
		return err
	}
	pr, err := pm.Open(ctx, pid)
	if err != nil {
		return err
	}
	defer pr.Release()
	titles := []string{busyChat}
	for i := range otherChats {
		titles = append(titles, fmt.Sprint("Call ", i+1))
	}
	for _, t := range titles {
		if _, err := pr.Chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: t}); err != nil {
			return err
		}
	}
	// The history chat last, so it is the newest.
	ch, err := pr.Chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: historyChat})
	if err != nil {
		return err
	}
	r := rand.New(rand.NewPCG(7, 7))
	at := time.Now().Add(-200 * time.Minute)
	for i := range 200 {
		role := chat.RoleUser
		m := chat.Message{Chat: ch.ID, Turn: i/2 + 1, Role: role, CreatedAt: at.Add(time.Duration(i) * time.Minute),
			Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: historyMessage(r, i, kind)}}}}
		if i%2 == 1 {
			m.Role, m.Model = chat.RoleAssistant, "bench/"+modelName
		}
		if _, _, err := pr.Chats.AppendMessage(ctx, m); err != nil {
			return fmt.Errorf("message %d: %w", i, err)
		}
	}
	return nil
}
