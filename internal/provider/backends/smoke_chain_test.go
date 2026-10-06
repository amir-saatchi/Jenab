//go:build smoke

package backends

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
)

var noteTool = provider.ToolDef{Name: "note", Description: "Saves one code.",
	Schema: json.RawMessage(`{"type":"object","properties":{"code":{"type":"string"}},"required":["code"]}`)}

// TestSmokeChain: the model saves three codes it made up, one call per
// reply, then lists them. The codes are only in its own tool calls, so it
// can list them only if every tool-call message reaches it. With content
// [] Ollama dropped all but the first call (2026-10-06):
//
//	go test -tags smoke -run TestSmokeChain -v ./internal/provider/backends/
func TestSmokeChain(t *testing.T) {
	cases := []struct {
		name, kind, base, env, model string
		thinking                     bool
	}{
		{"gemini", string(provider.KindGemini), "https://generativelanguage.googleapis.com/v1beta/openai/", "GEMINI_API_KEY", "gemini-3.5-flash-lite", true},
		{"groq", string(provider.KindCompatible), "https://api.groq.com/openai/v1/", "GROQ_API_KEY", "openai/gpt-oss-20b", false},
		{"ollamacloud", string(provider.KindCompatible), "https://ollama.com/v1/", "OLLAMA_API_KEY", "gemma4:31b", false},
		{"zai", string(provider.KindCompatible), "https://api.z.ai/api/paas/v4/", "Z_API_KEY", "glm-4.5-flash", false},
		{"cloudflare", string(provider.KindCompatible), "https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1/", "CLOUDFLARE_TOKEN", "@cf/meta/llama-3.3-70b-instruct-fp8-fast", false},
	}
	keys := envKeys(t, "GEMINI_API_KEY", "GROQ_API_KEY", "OLLAMA_API_KEY", "Z_API_KEY", "CLOUDFLARE_TOKEN", "CLOUDFLARE_ID")
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			key := keys[c.env]
			if key == "" {
				t.Skipf("%s is not set", c.env)
			}
			kr := &memKeyring{m: map[string]string{provider.KeyName(c.name): key}}
			sec := secret.New(kr)
			base := c.base
			if strings.Contains(base, "{account_id}") {
				if keys["CLOUDFLARE_ID"] == "" {
					t.Skip("CLOUDFLARE_ID is not set")
				}
				base = strings.ReplaceAll(base, "{account_id}", keys["CLOUDFLARE_ID"])
			}
			settings := config.LLMSettings{MaxParallelCalls: 1, Providers: map[string]config.ProviderSettings{
				c.name: {Kind: c.kind, BaseURL: base, Models: []config.ModelSettings{{ID: c.model, Context: 128000}}},
			}}
			reg := provider.NewRegistry(provider.Deps{Settings: settings, Secrets: sec, Gate: limit.NewGate(1), Backends: All()})
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			redact := func(s string) string {
				if id := keys["CLOUDFLARE_ID"]; id != "" {
					s = strings.ReplaceAll(s, id, "[account]")
				}
				return sec.Redact(s)
			}

			req := provider.Request{Model: c.name + "/" + c.model, Thinking: c.thinking, MaxTokens: 2000, Tools: []provider.ToolDef{noteTool},
				System:   []provider.Block{{Text: "You are a test assistant. Use the tools you are given."}},
				Messages: []chat.Message{{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "Make up three different random codes of 6 letters and digits. Save each with the note tool: one call per reply, and wait for its result before the next call. Don't write the codes in your text. When all three are saved, reply with the word done."}}}}}}
			var codes []string
			var prompts []int
			for step := 0; step < 6; step++ {
				parts, done, err := chainTurn(ctx, reg, req)
				if err != nil {
					t.Fatalf("step %d: %s", step+1, redact(err.Error()))
				}
				prompts = append(prompts, done.Usage.Input+done.Usage.CacheRead+done.Usage.CacheWrite)
				res := chat.Message{Role: chat.RoleTool}
				for _, p := range parts {
					if p.ToolCall != nil {
						var a struct{ Code string }
						json.Unmarshal(p.ToolCall.Args, &a)
						codes = append(codes, a.Code)
						res.Parts = append(res.Parts, chat.Part{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: p.ToolCall.ID, Text: "saved"}})
					}
				}
				// Text the model wrote around a call stays; only the calls'
				// own messages matter here.
				req.Messages = append(req.Messages, chat.Message{Role: chat.RoleAssistant, Parts: parts})
				if len(res.Parts) == 0 {
					break
				}
				if len(res.Parts) > 1 {
					t.Logf("step %d: %d calls in one reply", step+1, len(res.Parts))
				}
				req.Messages = append(req.Messages, res)
			}
			req.Messages = append(req.Messages, chat.Message{Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "List the codes you passed to the note tool earlier in this conversation. Reply with the codes only, comma-separated, in order."}}}})
			parts, done, err := chainTurn(ctx, reg, req)
			if err != nil {
				t.Fatalf("recall: %s", redact(err.Error()))
			}
			prompts = append(prompts, done.Usage.Input+done.Usage.CacheRead+done.Usage.CacheWrite)
			answer := ""
			for _, p := range parts {
				if p.Text != nil {
					answer += p.Text.Text
				}
			}
			recalled := 0
			for _, code := range codes {
				if code != "" && strings.Contains(strings.ToUpper(answer), strings.ToUpper(code)) {
					recalled++
				}
			}
			t.Logf("%d calls, %d codes recalled; prompt tokens %v; answer %q", len(codes), recalled, prompts, strings.TrimSpace(answer))
			if len(codes) < 2 || recalled < len(codes) {
				t.Errorf("recalled %d of %d codes", recalled, len(codes))
			}
		})
	}
}

func chainTurn(ctx context.Context, reg *provider.Registry, req provider.Request) ([]chat.Part, provider.Event, error) {
	var parts []chat.Part
	for ev, err := range reg.Stream(ctx, limit.Interactive, req) {
		if err != nil {
			return nil, provider.Event{}, err
		}
		switch ev.Kind {
		case provider.EventPart:
			parts = append(parts, *ev.Part)
		case provider.EventDone:
			if ev.Usage == nil {
				ev.Usage = &chat.Usage{}
			}
			return parts, ev, nil
		}
	}
	return nil, provider.Event{}, context.Canceled
}
