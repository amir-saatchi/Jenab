package app

import (
	"context"
	"sync"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/provider"
)

// Settings holds the user settings while the app runs. Readers get the
// settings now, and a save applies at once (SPEC 7.6), except the data
// folder, which takes a restart.
type Settings struct {
	path string

	mu       sync.Mutex
	s        config.Settings
	problems []config.Problem
	apply    []func(config.Settings)
}

// NewSettings holds s, as loaded from path with problems.
func NewSettings(path string, s config.Settings, problems []config.Problem) *Settings {
	return &Settings{path: path, s: s, problems: problems}
}

// Get returns the settings now. Callers must not change its maps.
func (s *Settings) Get() config.Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.s
}

// OnChange adds fn, called with the new settings after each save, e.g. to
// resize the call gates and apply provider changes.
func (s *Settings) OnChange(fn func(config.Settings)) {
	s.mu.Lock()
	s.apply = append(s.apply, fn)
	s.mu.Unlock()
}

// save writes next to config.yaml, keeping its comments, and reads it back,
// so values out of range come back as problems with their defaults used.
func (s *Settings) save(next config.Settings) (config.Settings, []config.Problem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := config.SaveSettings(s.path, next); err != nil {
		return s.s, s.problems, err
	}
	loaded, problems, err := config.LoadSettings(s.path)
	if err != nil {
		return s.s, s.problems, err
	}
	s.s, s.problems = loaded, problems
	for _, fn := range s.apply {
		fn(loaded)
	}
	return loaded, problems, nil
}

func (s *Settings) view() SettingsView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return settingsView(s.s, s.problems)
}

func settingsView(s config.Settings, ps []config.Problem) SettingsView {
	if ps == nil {
		ps = []config.Problem{}
	}
	return SettingsView{Settings: s, Problems: ps}
}

// SettingsView is the settings with the problems found reading them.
type SettingsView struct {
	Settings config.Settings  `json:"settings"`
	Problems []config.Problem `json:"problems"`
}

// SettingsService is *Settings* (SPEC 5.12): the settings, the providers
// and their models (connect.go). P1-16 adds usage.
type SettingsService struct {
	base
	settings *Settings
	models   *provider.Registry
	ollamaUp func(context.Context) bool // whether Ollama runs on this machine
}

// Get returns the settings and the problems found in config.yaml.
func (s *SettingsService) Get(ctx context.Context) (v SettingsView, err error) {
	defer s.guard("settings.get", &err)
	return s.settings.view(), nil
}

// Save stores the settings and applies them.
func (s *SettingsService) Save(ctx context.Context, next config.Settings) (v SettingsView, err error) {
	defer s.guard("settings.save", &err)
	loaded, problems, err := s.settings.save(next)
	return settingsView(loaded, problems), err
}

// ProviderStatus is a connected provider's state, for the bottom bar
// (SPEC 5.12).
type ProviderStatus struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	PausedMS    int64  `json:"paused_ms"` // how long it stays paused; 0 if not
	Limit       int    `json:"limit"`     // background calls at once
	Current     int    `json:"current"`   // the limit now, halved after a rate limit
	Running     int    `json:"running"`   // calls running now
	Waiting     int    `json:"waiting"`   // calls waiting for a slot
	LastProblem string `json:"last_problem,omitempty"`
}

// Providers lists the connected providers.
func (s *SettingsService) Providers(ctx context.Context) (ps []ProviderStatus, err error) {
	defer s.guard("settings.providers", &err)
	return providerStatus(s.models, s.redact), nil
}

func providerStatus(r *provider.Registry, redact func(string) string) []ProviderStatus {
	out := []ProviderStatus{}
	for _, st := range r.Status() {
		out = append(out, ProviderStatus{
			Name: st.Name, Kind: string(st.Kind), PausedMS: st.PausedFor.Milliseconds(),
			Limit: st.Limit, Current: st.Current, Running: st.Calls.InUse, Waiting: st.Calls.Waiting,
			LastProblem: redact(st.LastProblem),
		})
	}
	return out
}
