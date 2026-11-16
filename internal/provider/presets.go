package provider

import (
	"cmp"
	"net/url"
)

// Preset fills in a well-known provider's kind and base URL in the
// Connect form (SPEC 3.9). It holds no models: the user brings the key and
// picks the models, and can edit the base URL. Any provider can also be
// added by hand.
type Preset struct {
	ID      string // the suggested connection name
	Name    string
	Kind    Kind
	BaseURL string // "" means the kind's own; may hold placeholders such as {account_id}
	// Local: it runs on this computer and needs no key. The form lists it
	// under "On this computer" and asks for no key.
	Local bool
}

// Presets lists the well-known providers, in the order the form shows them.
var Presets = []Preset{
	{ID: "anthropic", Name: "Anthropic", Kind: KindAnthropic},
	{ID: "openai", Name: "OpenAI", Kind: KindOpenAI},
	{ID: "gemini", Name: "Google Gemini", Kind: KindGemini},
	{ID: "groq", Name: "Groq", Kind: KindCompatible, BaseURL: "https://api.groq.com/openai/v1/"},
	{ID: "openrouter", Name: "OpenRouter", Kind: KindCompatible, BaseURL: "https://openrouter.ai/api/v1/"},
	{ID: "zai", Name: "Z.ai", Kind: KindCompatible, BaseURL: "https://api.z.ai/api/paas/v4/"},
	{ID: "cloudflare", Name: "Cloudflare Workers AI", Kind: KindCompatible, BaseURL: "https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1/"},
	{ID: "ollama-cloud", Name: "Ollama Cloud", Kind: KindOllama, BaseURL: "https://ollama.com/"},
	{ID: "ollama", Name: "Ollama", Kind: KindOllama, Local: true},
	{ID: "lmstudio", Name: "LM Studio", Kind: KindCompatible, BaseURL: "http://localhost:1234/v1/", Local: true},
}

// Host is the host the preset talks to, such as api.anthropic.com, for
// the form's second line.
func (p Preset) Host() string {
	u, err := url.Parse(cmp.Or(p.BaseURL, DefaultBaseURL(p.Kind)))
	if err != nil {
		return ""
	}
	return u.Host
}
