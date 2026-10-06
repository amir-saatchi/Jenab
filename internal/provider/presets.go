package provider

// Preset fills in a well-known provider's kind and base URL in the
// Connect form (SPEC 3.9). It holds no models: the user brings the key and
// picks the models, and can edit the base URL. Any provider can also be
// added by hand.
type Preset struct {
	ID      string // the suggested connection name
	Name    string
	Kind    Kind
	BaseURL string // "" means the kind's own; may hold placeholders such as {account_id}
}

// Presets lists the well-known providers, in the order the form shows them.
var Presets = []Preset{
	{ID: "anthropic", Name: "Anthropic", Kind: KindAnthropic},
	{ID: "openai", Name: "OpenAI", Kind: KindOpenAI},
	{ID: "gemini", Name: "Google Gemini", Kind: KindGemini},
	{ID: "ollama", Name: "Ollama on this computer", Kind: KindOllama},
	{ID: "ollama-cloud", Name: "Ollama Cloud", Kind: KindOllama, BaseURL: "https://ollama.com/"},
	{ID: "cloudflare", Name: "Cloudflare Workers AI", Kind: KindCompatible, BaseURL: "https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1/"},
	{ID: "groq", Name: "Groq", Kind: KindCompatible, BaseURL: "https://api.groq.com/openai/v1/"},
	{ID: "openrouter", Name: "OpenRouter", Kind: KindCompatible, BaseURL: "https://openrouter.ai/api/v1/"},
	{ID: "zai", Name: "Z.ai", Kind: KindCompatible, BaseURL: "https://api.z.ai/api/paas/v4/"},
}
