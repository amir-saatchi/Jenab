//go:build smoke

// The smoke test calls real providers with the development keys in the
// repo-root .env (git-ignored), with synthetic prompts only:
//
//	go test -tags smoke -run Smoke -v ./internal/provider/backends/
//
// Key values stay in memory: never printed, logged or put in a URL.
package backends

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
)

type memKeyring struct {
	mu sync.Mutex
	m  map[string]string
}

func (k *memKeyring) Get(name string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[name]
	if !ok {
		return "", secret.ErrNotFound
	}
	return v, nil
}

func (k *memKeyring) Set(name, v string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[name] = v
	return nil
}

func (k *memKeyring) Delete(name string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, name)
	return nil
}

// envKeys reads the named keys from ../../../.env.
func envKeys(t *testing.T, names ...string) map[string]string {
	f, err := os.Open("../../../.env")
	if err != nil {
		t.Skip("no .env")
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		for _, n := range names {
			if k == n && v != "" {
				out[n] = v
			}
		}
	}
	return out
}

var addTool = provider.ToolDef{Name: "add", Description: "Adds two integers and returns the sum.",
	Schema: json.RawMessage(`{"type":"object","properties":{"a":{"type":"integer"},"b":{"type":"integer"}},"required":["a","b"]}`)}

func TestSmoke(t *testing.T) {
	cases := []struct {
		name     string
		kind     provider.Kind
		base     string
		env      string
		model    string
		thinking bool
		connect  bool // list the models through Connect
	}{
		{"gemini", provider.KindGemini, "", "GEMINI_API_KEY", "gemini-3.5-flash-lite", true, true},
		{"groq", provider.KindCompatible, "https://api.groq.com/openai/v1/", "GROQ_API_KEY", "openai/gpt-oss-20b", false, true},
		{"ollamacloud", provider.KindCompatible, "https://ollama.com/v1/", "OLLAMA_API_KEY", "gpt-oss:20b", false, true},
		{"ollamanative", provider.KindOllama, "https://ollama.com/", "OLLAMA_API_KEY", "gpt-oss:20b", false, true},
		{"zai", provider.KindCompatible, "https://api.z.ai/api/paas/v4/", "Z_API_KEY", "glm-4.7-flash", false, true},
		// Workers AI's OpenAI-compatible endpoint: the account ID fills a
		// placeholder, and there is no model list.
		{"cloudflare", provider.KindCompatible, "https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1/", "CLOUDFLARE_TOKEN", "@cf/meta/llama-3.3-70b-instruct-fp8-fast", false, true},
	}
	keys := envKeys(t, "GEMINI_API_KEY", "GROQ_API_KEY", "OLLAMA_API_KEY", "Z_API_KEY", "CLOUDFLARE_TOKEN", "CLOUDFLARE_ID")

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			key := keys[c.env]
			if key == "" {
				t.Skipf("%s is not set", c.env)
			}
			kr := &memKeyring{m: map[string]string{}}
			sec := secret.New(kr)
			var fields map[string]string
			if len(provider.Placeholders(c.base)) > 0 {
				if keys["CLOUDFLARE_ID"] == "" {
					t.Skip("CLOUDFLARE_ID is not set")
				}
				fields = map[string]string{"account_id": keys["CLOUDFLARE_ID"]}
			}
			settings := config.LLMSettings{MaxParallelCalls: 2, Providers: map[string]config.ProviderSettings{}}
			reg := provider.NewRegistry(provider.Deps{Settings: settings, Secrets: sec, Gate: limit.NewGate(2), Backends: All()})
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			ps := config.ProviderSettings{Kind: string(c.kind), BaseURL: c.base, Models: []config.ModelSettings{{ID: c.model, Context: 128000}}}
			if c.connect {
				got, err := reg.Connect(ctx, c.name, c.kind, c.base, key, fields)
				if err != nil {
					t.Fatalf("connect: %s", sec.Redact(err.Error()))
				}
				found := false
				for _, m := range append(got.Settings.Models, modelSettings(got.Other)...) {
					found = found || m.ID == c.model
				}
				t.Logf("connect: %d models on, %d other, no list %v; %s listed: %v", len(got.Settings.Models), len(got.Other), got.NoModelList, c.model, found)
			} else {
				kr.Set(provider.KeyName(c.name), key)
			}
			settings.Providers[c.name] = ps
			reg.Apply(settings)

			ref := c.name + "/" + c.model
			req := provider.Request{Model: ref, Thinking: c.thinking, MaxTokens: 2000, Tools: []provider.ToolDef{addTool},
				System:   []provider.Block{{Text: "You are a test assistant. Use the tools you are given.", Cache: true}},
				Messages: []chat.Message{{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "Use the add tool to add 17 and 25. Then reply with the sum only."}}}}}}

			parts, done := turn(t, ctx, reg, sec, req)
			var calls []chat.Part
			for _, p := range parts {
				if p.ToolCall != nil {
					calls = append(calls, p)
				}
			}
			if done.Stop != provider.StopToolUse || len(calls) == 0 {
				t.Fatalf("turn 1: stop %s, %d calls, parts %s", done.Stop, len(calls), summary(parts))
			}
			t.Logf("turn 1: %s; stop %s; usage %+v", summary(parts), done.Stop, *done.Usage)

			results := chat.Message{Role: chat.RoleTool}
			for _, p := range calls {
				results.Parts = append(results.Parts, chat.Part{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: p.ToolCall.ID, Text: "42"}})
			}
			req.Messages = append(req.Messages, chat.Message{Role: chat.RoleAssistant, Parts: parts}, results)
			parts, done = turn(t, ctx, reg, sec, req)
			t.Logf("turn 2: %s; stop %s; usage %+v", summary(parts), done.Stop, *done.Usage)
			text := ""
			for _, p := range parts {
				if p.Text != nil {
					text += p.Text.Text
				}
			}
			if done.Stop != provider.StopEnd || !strings.Contains(text, "42") {
				t.Fatalf("turn 2: stop %s, text %q", done.Stop, text)
			}
		})
	}
}

func modelSettings(ms []provider.ModelInfo) []config.ModelSettings {
	var out []config.ModelSettings
	for _, m := range ms {
		out = append(out, config.ModelSettings{ID: m.ID})
	}
	return out
}

func turn(t *testing.T, ctx context.Context, reg *provider.Registry, sec *secret.Store, req provider.Request) ([]chat.Part, provider.Event) {
	t.Helper()
	var parts []chat.Part
	for ev, err := range reg.Stream(ctx, limit.Interactive, req) {
		if err != nil {
			var pe *provider.Error
			if errors.As(err, &pe) {
				t.Fatalf("%s error (status %d): %s", pe.Kind, pe.Status, sec.Redact(pe.Message))
			}
			t.Fatal(sec.Redact(err.Error()))
		}
		switch ev.Kind {
		case provider.EventPart:
			parts = append(parts, *ev.Part)
		case provider.EventWait:
			t.Logf("paused for %s", ev.Wait)
		case provider.EventDone:
			return parts, ev
		}
	}
	t.Fatal("no done event")
	return nil, provider.Event{}
}

func summary(ps []chat.Part) string {
	var s []string
	for _, p := range ps {
		switch {
		case p.Thinking != nil:
			x := "thinking"
			if p.Thinking.Signature != "" {
				x += "+signature"
			}
			s = append(s, x)
		case p.Text != nil:
			s = append(s, "text "+strings.TrimSpace(p.Text.Text))
		case p.ToolCall != nil:
			x := "call " + p.ToolCall.Name + string(p.ToolCall.Args)
			if len(p.ToolCall.Extra) > 0 {
				x += "+extra"
			}
			s = append(s, x)
		}
	}
	return strings.Join(s, ", ")
}
