package app

import (
	"context"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/provider"
)

// ModelGroup is one connected provider with its models that are on, for
// the model picker (SPEC 3.9).
type ModelGroup struct {
	Provider string      `json:"provider"`
	Kind     string      `json:"kind"`
	Models   []ModelItem `json:"models"`
}

// ModelItem is a model that is on.
type ModelItem struct {
	Ref     string   `json:"ref"`     // "provider/model", as chats.model stores it
	Name    string   `json:"name"`    // the catalog's name, else the model ID
	Aliases []string `json:"aliases"` // "default" or "fast" when they point at it
}

// Models lists the models that are on, by provider name.
func (s *SettingsService) Models(ctx context.Context) (gs []ModelGroup, err error) {
	defer s.guard("settings.models", &err)
	llm := s.settings.Get().LLM
	cat := s.models.Catalog()
	gs = []ModelGroup{}
	for _, name := range slices.Sorted(maps.Keys(llm.Providers)) {
		ps := llm.Providers[name]
		g := ModelGroup{Provider: name, Kind: ps.Kind, Models: []ModelItem{}}
		for _, m := range ps.Models {
			it := ModelItem{Ref: name + "/" + m.ID, Name: m.ID, Aliases: []string{}}
			if c, ok := cat.Find(provider.Kind(ps.Kind), m.ID); ok {
				it.Name = c.Name
			}
			for _, a := range []string{"default", "fast"} {
				if llm.Models[a] == it.Ref {
					it.Aliases = append(it.Aliases, a)
				}
			}
			g.Models = append(g.Models, it)
		}
		gs = append(gs, g)
	}
	return gs, nil
}

// PresetItem is a well-known provider for the *Connect* form (SPEC 3.9).
type PresetItem struct {
	ID      string   `json:"id"` // the suggested connection name
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	BaseURL string   `json:"base_url"` // "" means the kind's own
	Fields  []string `json:"fields"`   // the base URL's placeholders, e.g. account_id
	NoKey   bool     `json:"no_key"`   // a local Ollama needs no key
	Running bool     `json:"running"`  // Ollama answers on this machine
}

// Presets lists the well-known providers. When Ollama runs on this
// machine it comes first, with no key needed.
func (s *SettingsService) Presets(ctx context.Context) (ps []PresetItem, err error) {
	defer s.guard("settings.presets", &err)
	up := s.ollamaUp(ctx)
	for _, p := range provider.Presets {
		it := PresetItem{ID: p.ID, Name: p.Name, Kind: string(p.Kind), BaseURL: p.BaseURL,
			Fields: provider.Placeholders(p.BaseURL), NoKey: p.Kind == provider.KindOllama}
		if it.Fields == nil {
			it.Fields = []string{}
		}
		if it.NoKey && up {
			it.Running = true
			ps = append([]PresetItem{it}, ps...)
			continue
		}
		ps = append(ps, it)
	}
	return ps, nil
}

// ollamaRunning tells whether Ollama answers on this machine.
func ollamaRunning(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.DefaultBaseURL(provider.KindOllama)+"api/version", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// ConnectRequest is the *Connect* form.
type ConnectRequest struct {
	Name    string            `json:"name"`
	Kind    string            `json:"kind"`
	BaseURL string            `json:"base_url"`
	Key     string            `json:"key"`
	Fields  map[string]string `json:"fields"` // values for the base URL's placeholders
}

// ConnectResult is what *Connect* found and saved.
type ConnectResult struct {
	Provider string `json:"provider"`
	Models   int    `json:"models"` // the models turned on
	// Other are listed models the catalog doesn't know, off by default.
	Other []string `json:"other"`
	// NoModelList: the provider lists no models, so they are added by hand.
	NoModelList bool         `json:"no_model_list"`
	View        SettingsView `json:"view"`
}

// Connect checks the key by listing the provider's models, stores it in
// the keychain and saves the provider with its models turned on. The
// aliases default and fast are set when they point at no connected
// provider, so a first provider makes the app usable at once (SPEC 3.9).
func (s *SettingsService) Connect(ctx context.Context, req ConnectRequest) (r ConnectResult, err error) {
	defer s.guard("settings.connect", &err)
	req.Name = strings.TrimSpace(req.Name)
	c, err := s.models.Connect(ctx, req.Name, provider.Kind(req.Kind), strings.TrimSpace(req.BaseURL), strings.TrimSpace(req.Key), req.Fields)
	if err != nil {
		return r, err
	}
	next := s.settings.Get()
	next.LLM.Providers = maps.Clone(next.LLM.Providers)
	next.LLM.Models = maps.Clone(next.LLM.Models)
	if next.LLM.Providers == nil {
		next.LLM.Providers = map[string]config.ProviderSettings{}
	}
	if next.LLM.Models == nil {
		next.LLM.Models = map[string]string{}
	}
	next.LLM.Providers[req.Name] = c.Settings
	setAliases(&next.LLM, s.models.Catalog(), req.Name, c.Settings)
	loaded, problems, err := s.settings.save(next)
	if err != nil {
		return r, err
	}
	r = ConnectResult{Provider: req.Name, Models: len(c.Settings.Models), Other: []string{}, NoModelList: c.NoModelList,
		View: settingsView(loaded, problems)}
	for _, m := range c.Other {
		r.Other = append(r.Other, m.ID)
	}
	return r, nil
}

// setAliases points default and fast at the provider name's models when
// they point at no connected provider: the catalog's suggestions if they
// are on, else its first model. fast falls back to default's pick.
func setAliases(llm *config.LLMSettings, cat *provider.Catalog, name string, ps config.ProviderSettings) {
	if len(ps.Models) == 0 {
		return
	}
	on := func(id string) bool {
		return id != "" && slices.ContainsFunc(ps.Models, func(m config.ModelSettings) bool { return m.ID == id })
	}
	def, fast := cat.Suggested(provider.Kind(ps.Kind))
	if !on(def) {
		def = ps.Models[0].ID
	}
	if !on(fast) {
		fast = def
	}
	for alias, id := range map[string]string{"default": def, "fast": fast} {
		p, _, _ := strings.Cut(llm.Models[alias], "/")
		if _, ok := llm.Providers[p]; ok && p != "" {
			continue
		}
		llm.Models[alias] = name + "/" + id
	}
}
