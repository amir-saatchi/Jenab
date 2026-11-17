package logic

import (
	"testing"

	"github.com/amir-saatchi/jenab/internal/config"
)

// A provider is on this computer when its base URL is, by the registry's
// own rule, whatever its kind: LM Studio too, not only Ollama (TASK-068).
func TestAProviderOnThisComputerNeedsNoKey(t *testing.T) {
	for _, c := range []struct {
		kind, url string
		local     bool
	}{
		{"ollama", "", true},
		{"ollama", "http://localhost:11434/", true},
		{"ollama", "http://gpu-box.lan:11434/", false},
		{"openai_compatible", "http://localhost:1234/v1/", true},
		{"openai_compatible", "http://127.0.0.1:8080/v1/", true},
		{"openai_compatible", "http://[::1]:8080/v1/", true},
		{"openai_compatible", "https://api.z.ai/api/paas/v4/", false},
		{"openai_compatible", "", false},
		{"gemini", "", false},
	} {
		p := Provider{Name: "p", ProviderSettings: config.ProviderSettings{Kind: c.kind, BaseURL: c.url}}
		if p.Local() != c.local {
			t.Errorf("%s at %q: local = %v", c.kind, c.url, p.Local())
		}
	}
	lm := Provider{Name: "lmstudio", ProviderSettings: config.ProviderSettings{Kind: "openai_compatible", BaseURL: "http://localhost:1234/v1/",
		Models: []config.ModelSettings{{ID: "qwen"}}}}
	if got := lm.Line(); got != "On this computer · no key needed · 1 model on" {
		t.Errorf("LM Studio's line = %q", got)
	}
}

// A provider on this computer offers an optional key field: a preset with
// no key, or an OpenAI-compatible URL on localhost (TASK-068).
func TestAProviderOnThisComputerMayStillHaveAKey(t *testing.T) {
	for _, id := range []string{"ollama", "lmstudio"} {
		if f := FormFor(id, presets, nil); f.Key || !f.OptionalKey {
			t.Errorf("%s: %+v", id, f)
		}
	}
	if f := FormFor("anthropic", presets, nil); !f.Key || f.OptionalKey {
		t.Errorf("anthropic: %+v", f)
	}
	other := FormFor(Other, presets, nil)
	for url, local := range map[string]bool{
		"":                           false,
		"https://example.com/v1/":    false,
		"http://localhost:8080/v1/":  true,
		" http://127.0.0.1:5000/v1 ": true,
	} {
		f := other.WithURL(url)
		if f.Key == local || f.OptionalKey != local {
			t.Errorf("Other at %q: %+v", url, f)
		}
	}
	if f := FormFor("anthropic", presets, nil).WithURL("http://localhost:1/"); !f.Key {
		t.Errorf("a preset's shape doesn't follow a typed URL: %+v", f)
	}
}
