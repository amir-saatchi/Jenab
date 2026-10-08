// Package backends maps each provider kind to its backend. It sits apart
// from package provider because the backends import that package.
package backends

import (
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/anthropic"
	"github.com/amir-saatchi/jenab/internal/provider/gemini"
	"github.com/amir-saatchi/jenab/internal/provider/ollama"
	"github.com/amir-saatchi/jenab/internal/provider/openai"
)

// All returns a factory for every kind, for provider.Deps.Backends.
func All() map[provider.Kind]provider.Factory {
	return map[provider.Kind]provider.Factory{
		provider.KindAnthropic:  anthropic.New,
		provider.KindOpenAI:     openai.New,
		provider.KindGemini:     gemini.New,
		provider.KindCompatible: openai.New,
		provider.KindOllama:     ollama.New,
	}
}
