package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
	"github.com/amir-saatchi/jenab/internal/store"
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
// and their models (connect.go), and usage (usage.go).
type SettingsService struct {
	base
	settings *Settings
	models   *provider.Registry
	secrets  *secret.Store // nil in tests that keep no keys
	registry *store.Registry
	started  Started
	ollamaUp func(context.Context) bool // whether Ollama runs on this machine
	now      func() time.Time
}

// Started is what this start of the app used, for the settings that apply
// at the next start: the data folder and the developer tools.
type Started struct {
	DataFolder        string `json:"data_folder"`         // in use
	DefaultDataFolder string `json:"default_data_folder"` // used when data_folder is ""
	DevTools          bool   `json:"dev_tools"`
	// Moved is what moving the projects to a changed data folder did at
	// this start (SPEC 2.1); nil if the folder didn't change.
	Moved *project.MoveResult `json:"moved"`
}

// Started returns what this start used.
func (s *SettingsService) Started(ctx context.Context) (st Started, err error) {
	defer s.guard("settings.started", &err)
	return s.started, nil
}

// CheckFolder checks a folder chosen as the data folder: it must be a full
// path, and gets the warning of SPEC 2.1 if it is on a network drive or in
// a synced folder.
func (s *SettingsService) CheckFolder(ctx context.Context, dir string) (w *project.FolderWarning, err error) {
	defer s.guard("settings.check_folder", &err)
	if !filepath.IsAbs(dir) {
		return nil, &UIError{Kind: KindInvalid, Message: fmt.Sprintf("%q is not a full path. Choose a folder.", dir)}
	}
	return project.CheckFolder(dir), nil
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
