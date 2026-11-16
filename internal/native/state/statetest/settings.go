package statetest

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/version"
)

// Settings is the settings services in memory, for Settings' pages
// (TASK-055): it keeps what is saved, and records the connects, removes
// and memory saves. Its fields are set up before the stores read them.
type Settings struct {
	mu         sync.Mutex
	View       app.SettingsView
	Groups     []app.ModelGroup
	Start      app.Started
	PresetList []app.PresetItem
	Listed     map[string]app.ProviderModels // ProviderModels by provider
	Report     app.UsageReport
	Days       []int // the days Usage was asked for, in order
	Memory     app.MemoryView
	Desk       app.DesktopList
	Build      version.Info
	Warn       map[string]*project.FolderWarning // CheckFolder's warning by folder
	Refuse     map[string]string                 // CheckFolder's refusal by folder, as the backend's
	PresetsBad int                               // how many Presets calls fail before one works
	Connects   []app.ConnectRequest
	Removed    []string
	MemSaves   []app.MemoryEdit
	Conflict   *store.MemorySection // the next memory save's conflict, once
	Deleted    []int64              // the macros and recipes deleted
	Edits      []app.DesktopMacroEdit
	Writes     []app.DesktopRecipeWrite
	ConnectOK  func(app.ConnectRequest) (app.ConnectResult, error)
}

// NewSettings makes the settings services with the default settings.
func NewSettings() *Settings {
	return &Settings{View: app.SettingsView{Settings: config.Defaults()}, Listed: map[string]app.ProviderModels{},
		Warn: map[string]*project.FolderWarning{}, Refuse: map[string]string{}, Memory: app.MemoryView{Scope: "user", Cap: 4000}}
}

// Use puts s in api as the settings, the pages and the version.
func (s *Settings) Use(api state.API) state.API {
	api.Settings, api.Pages, api.Version = s, s, s
	return api
}

func (s *Settings) Get(context.Context) (app.SettingsView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.View, nil
}

func (s *Settings) Save(_ context.Context, next config.Settings) (app.SettingsView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.View = app.SettingsView{Settings: next}
	return s.View, nil
}

func (s *Settings) Models(context.Context) ([]app.ModelGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.Groups), nil
}

func (s *Settings) Started(context.Context) (app.Started, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Start, nil
}

func (s *Settings) Connect(_ context.Context, req app.ConnectRequest) (app.ConnectResult, error) {
	s.mu.Lock()
	s.Connects = append(s.Connects, req)
	ok := s.ConnectOK
	s.mu.Unlock()
	if ok == nil {
		return app.ConnectResult{}, errors.New("not in the fake")
	}
	r, err := ok(req)
	if err == nil {
		s.mu.Lock()
		s.View = r.View
		s.mu.Unlock()
	}
	return r, err
}

func (s *Settings) Remove(_ context.Context, name string) (app.SettingsView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Removed = append(s.Removed, name)
	delete(s.View.Settings.LLM.Providers, name)
	return s.View, nil
}

func (s *Settings) Presets(context.Context) ([]app.PresetItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.PresetsBad > 0 {
		s.PresetsBad--
		return nil, errors.New("the presets couldn't be read")
	}
	return slices.Clone(s.PresetList), nil
}

func (s *Settings) ProviderModels(_ context.Context, name string) (app.ProviderModels, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pm, ok := s.Listed[name]
	if !ok {
		return app.ProviderModels{}, errors.New("no such provider")
	}
	return pm, nil
}

func (s *Settings) Usage(_ context.Context, days int) (app.UsageReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Days = append(s.Days, days)
	r := s.Report
	if n := len(r.Days); n > days {
		r.Days = r.Days[n-days:]
	}
	return r, nil
}

func (s *Settings) UserMemory(context.Context) (app.MemoryView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Memory, nil
}

func (s *Settings) SaveUserMemory(_ context.Context, e app.MemoryEdit) (app.MemorySaved, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.MemSaves = append(s.MemSaves, e)
	if c := s.Conflict; c != nil {
		s.Conflict = nil
		return app.MemorySaved{Memory: s.Memory, Conflict: c}, nil
	}
	secs := slices.DeleteFunc(slices.Clone(s.Memory.Sections), func(m store.MemorySection) bool { return m.Section == e.Section })
	if strings.TrimSpace(e.Content) != "" {
		secs = append(secs, store.MemorySection{Section: e.Section, Content: e.Content, Revision: e.Revision + 1})
	}
	s.Memory.Sections = secs
	return app.MemorySaved{Memory: s.Memory}, nil
}

func (s *Settings) Desktop(context.Context) (app.DesktopList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Desk, nil
}

func (s *Settings) EditDesktopMacro(_ context.Context, e app.DesktopMacroEdit) (store.DesktopMacro, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Edits = append(s.Edits, e)
	return store.DesktopMacro{ID: e.ID}, nil
}

func (s *Settings) DeleteDesktopMacro(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Deleted = append(s.Deleted, id)
	return nil
}

func (s *Settings) WriteDesktopRecipe(_ context.Context, w app.DesktopRecipeWrite) (store.DesktopRecipe, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Writes = append(s.Writes, w)
	return store.DesktopRecipe{ID: w.ID, App: w.App, Text: w.Text}, nil
}

func (s *Settings) DeleteDesktopRecipe(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Deleted = append(s.Deleted, id)
	return nil
}

func (s *Settings) CheckFolder(_ context.Context, dir string) (*project.FolderWarning, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msg := s.Refuse[dir]
	if msg == "" {
		msg = project.DataFolderProblem(dir, "", nil)
	}
	if msg != "" {
		return nil, &app.UIError{Kind: app.KindInvalid, Message: msg}
	}
	return s.Warn[dir], nil
}

func (s *Settings) Version(context.Context) (version.Info, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Build, nil
}

// Lock runs fn under the fake's lock, to read what was recorded.
func (s *Settings) Lock(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

var (
	_ state.SettingsAPI      = (*Settings)(nil)
	_ state.SettingsPagesAPI = (*Settings)(nil)
	_ state.VersionAPI       = (*Settings)(nil)
)
