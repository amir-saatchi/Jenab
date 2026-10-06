package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
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
		// Only the local Ollama, at the kind's own URL, needs no key.
		it := PresetItem{ID: p.ID, Name: p.Name, Kind: string(p.Kind), BaseURL: p.BaseURL,
			Fields: provider.Placeholders(p.BaseURL), NoKey: p.Kind == provider.KindOllama && p.BaseURL == ""}
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
	req.Name, req.BaseURL = strings.TrimSpace(req.Name), strings.TrimSpace(req.BaseURL)
	kind := provider.Kind(req.Kind)
	// Checked before the key is stored, and again when saving.
	if old, ok := s.settings.Get().LLM.Providers[req.Name]; ok && !sameConnection(old, kind, req.BaseURL) {
		return r, nameTaken(req.Name)
	}
	c, err := s.models.Connect(ctx, req.Name, kind, req.BaseURL, strings.TrimSpace(req.Key), req.Fields)
	if err != nil {
		return r, err
	}
	loaded, problems, err := s.settings.update(func(next *config.Settings) error {
		// A new key for a connected provider keeps the models the user chose.
		if old, ok := next.LLM.Providers[req.Name]; ok {
			if !sameConnection(old, kind, req.BaseURL) {
				return nameTaken(req.Name)
			}
			c.Settings.Models = old.Models
		}
		next.LLM.Providers[req.Name] = c.Settings
		setAliases(&next.LLM, s.models.Catalog(), req.Name, c.Settings)
		return nil
	})
	if err != nil {
		return r, err
	}
	s.models.Forget(req.Name) // the next call reads the new key
	r = ConnectResult{Provider: req.Name, Models: len(c.Settings.Models), Other: []string{}, NoModelList: c.NoModelList,
		View: settingsView(loaded, problems)}
	for _, m := range c.Other {
		r.Other = append(r.Other, m.ID)
	}
	return r, nil
}

// sameConnection tells whether a connected provider has this kind and base
// URL ("" being the kind's own).
func sameConnection(old config.ProviderSettings, kind provider.Kind, baseURL string) bool {
	or := func(u string) string { return cmp.Or(u, provider.DefaultBaseURL(kind)) }
	return old.Kind == string(kind) && or(old.BaseURL) == or(baseURL)
}

func nameTaken(name string) error {
	return &UIError{Kind: KindInvalid, Message: fmt.Sprintf("A provider named %q is already connected with another kind or base URL. Choose another name, or remove it first.", name)}
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

// Remove disconnects a provider (SPEC 3.9): it leaves the settings with its
// limit, the aliases that pointed at it move to another provider's models,
// and its key and base-URL values are deleted from the keychain.
func (s *SettingsService) Remove(ctx context.Context, name string) (v SettingsView, err error) {
	defer s.guard("settings.remove", &err)
	var ps config.ProviderSettings
	loaded, problems, err := s.settings.update(func(next *config.Settings) error {
		var ok bool
		if ps, ok = next.LLM.Providers[name]; !ok {
			return fmt.Errorf("%w: %q", provider.ErrUnknownProvider, name)
		}
		delete(next.LLM.Providers, name)
		delete(next.LLM.ProviderMaxParallelCalls, name)
		for alias, ref := range next.LLM.Models {
			if p, _, _ := strings.Cut(ref, "/"); p == name {
				delete(next.LLM.Models, alias)
			}
		}
		for _, p := range slices.Sorted(maps.Keys(next.LLM.Providers)) {
			setAliases(&next.LLM, s.models.Catalog(), p, next.LLM.Providers[p])
		}
		return nil
	})
	if err != nil {
		return v, err
	}
	v = settingsView(loaded, problems)
	if s.secrets == nil {
		return v, nil
	}
	base := ps.BaseURL
	if base == "" {
		base = provider.DefaultBaseURL(provider.Kind(ps.Kind))
	}
	names := []string{provider.KeyName(name)}
	for _, f := range provider.Placeholders(base) {
		names = append(names, provider.FieldName(name, f))
	}
	for _, n := range names {
		if err := s.secrets.Delete(n); err != nil {
			return v, fmt.Errorf("the provider was removed, but its key is still in the keychain: %w", err)
		}
	}
	return v, nil
}

// ProviderModels is one provider's models for *Settings → Models* (SPEC
// 3.9).
type ProviderModels struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	// Models are the catalog models the provider lists (Anthropic, OpenAI,
	// Gemini), or every listed model (OpenAI-compatible, Ollama), then the
	// models that are on but not listed.
	Models []ModelOption `json:"models"`
	// Other are listed models the catalog doesn't know (*Other models*).
	// Turning one on needs its context window.
	Other []ModelOption `json:"other"`
	// NoModelList: the provider has no model list, so models are typed in.
	NoModelList bool `json:"no_model_list"`
	// Problem is why the list couldn't be read; the models that are on are
	// still shown.
	Problem string `json:"problem,omitempty"`
}

// ModelOption is a model that can be turned on or off.
type ModelOption struct {
	ID      string `json:"id"`
	Name    string `json:"name"`    // the catalog's or the provider's name, else the ID
	On      bool   `json:"on"`      // in llm.providers.<name>.models
	Context int    `json:"context"` // tokens: the setting, the catalog or the provider's; 0 if unknown
	Known   bool   `json:"known"`   // in the catalog
}

// ProviderModels lists a provider's models, asking the provider for its
// list.
func (s *SettingsService) ProviderModels(ctx context.Context, name string) (pm ProviderModels, err error) {
	defer s.guard("settings.provider_models", &err)
	ps, ok := s.settings.Get().LLM.Providers[name]
	if !ok {
		return pm, fmt.Errorf("%w: %q", provider.ErrUnknownProvider, name)
	}
	kind := provider.Kind(ps.Kind)
	cat := s.models.Catalog()
	pm = ProviderModels{Provider: name, Kind: ps.Kind, Models: []ModelOption{}, Other: []ModelOption{}}
	on := map[string]config.ModelSettings{}
	for _, m := range ps.Models {
		on[m.ID] = m
	}
	listed, err := s.models.Models(ctx, name)
	var pe *provider.Error
	switch {
	case err == nil:
	case errors.As(err, &pe) && (pe.Status == 404 || pe.Status == 405):
		pm.NoModelList = true
	default:
		pm.Problem = s.redact(err.Error())
	}
	seen := map[string]bool{}
	option := func(id, name string, context int) ModelOption {
		seen[id] = true
		o := ModelOption{ID: id, Name: id, Context: context}
		if name != "" {
			o.Name = name
		}
		if c, ok := cat.Find(kind, id); ok {
			o.Name, o.Context, o.Known = c.Name, c.Context, true
		}
		if m, ok := on[id]; ok {
			o.On = true
			if m.Context > 0 {
				o.Context = m.Context
			}
		}
		return o
	}
	known := cat.Models(kind)
	have := map[string]provider.ModelInfo{}
	for _, m := range listed {
		have[m.ID] = m
	}
	if len(known) > 0 {
		for _, c := range known {
			if _, ok := have[c.ID]; ok {
				pm.Models = append(pm.Models, option(c.ID, "", 0))
			}
		}
	}
	for _, m := range listed {
		if seen[m.ID] {
			continue
		}
		o := option(m.ID, m.Name, m.Context)
		if len(known) > 0 && !o.Known {
			pm.Other = append(pm.Other, o)
		} else {
			pm.Models = append(pm.Models, o)
		}
	}
	for _, m := range ps.Models {
		if !seen[m.ID] {
			pm.Models = append(pm.Models, option(m.ID, "", m.Context))
		}
	}
	return pm, nil
}
