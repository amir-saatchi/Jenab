package state

import (
	"context"
	"errors"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/version"
)

// SettingsPagesAPI is what Settings' pages read and change besides the
// settings themselves (TASK-055); *app.SettingsService has it.
type SettingsPagesAPI interface {
	Presets(ctx context.Context) ([]app.PresetItem, error)
	ProviderModels(ctx context.Context, name string) (app.ProviderModels, error)
	Usage(ctx context.Context, days int) (app.UsageReport, error)
	UserMemory(ctx context.Context) (app.MemoryView, error)
	SaveUserMemory(ctx context.Context, e app.MemoryEdit) (app.MemorySaved, error)
	Desktop(ctx context.Context) (app.DesktopList, error)
	EditDesktopMacro(ctx context.Context, e app.DesktopMacroEdit) (store.DesktopMacro, error)
	DeleteDesktopMacro(ctx context.Context, id int64) error
	WriteDesktopRecipe(ctx context.Context, w app.DesktopRecipeWrite) (store.DesktopRecipe, error)
	DeleteDesktopRecipe(ctx context.Context, id int64) error
	CheckFolder(ctx context.Context, dir string) (*project.FolderWarning, error)
}

// VersionAPI tells the app's version, for About; *app.SystemService has
// it.
type VersionAPI interface {
	Version(ctx context.Context) (version.Info, error)
}

// errNoService is what a page gets when the app gave no service for it.
var errNoService = errors.New("this part of the settings is not available")

// Pages is what Settings' pages read (TASK-055): the usage, the desktop
// macros and recipes, and the version. The memory, the presets, a
// provider's models and the folder check belong to a dialog or an editor
// that is open a while, so they come back through a then callback, on the
// UI thread, rather than staying here.
type Pages struct {
	// Usage is the report for UsageDays, nil until read; UsageBusy while a
	// read is under way, which keeps the last report on screen.
	Usage     *app.UsageReport
	UsageDays int
	UsageBusy bool
	// Desktop is Settings › Desktop, nil until read.
	Desktop *app.DesktopList
	// Version is the build for About, nil until read.
	Version *version.Info
	// PickFolder asks the user for a folder in the system's dialog,
	// starting at start; "" when cancelled. The app sets it; nil hides
	// Browse.
	PickFolder func(title, start string) string

	api     SettingsPagesAPI
	ver     VersionAPI
	post    Post
	onError func(error)
	usage   int // counts the usage reads, so only the latest one shows
}

// NewPages makes the store. api and ver may be nil: their calls then
// fail.
func NewPages(api SettingsPagesAPI, ver VersionAPI, post Post, onError func(error)) *Pages {
	if onError == nil {
		onError = func(error) {}
	}
	return &Pages{api: api, ver: ver, post: post, onError: onError, UsageDays: 30}
}

// pageCall runs fn in a goroutine and gives its result to then on the UI
// thread.
func pageCall[T any](post Post, fn func() (T, error), then func(T, error)) {
	go func() {
		v, err := fn()
		post(func() { then(v, err) })
	}()
}

// LoadUsage reads the usage of the last days.
func (p *Pages) LoadUsage(days int) {
	p.usage++
	seq := p.usage
	p.UsageDays, p.UsageBusy = days, true
	pageCall(p.post, func() (app.UsageReport, error) {
		if p.api == nil {
			return app.UsageReport{}, errNoService
		}
		return p.api.Usage(background, days)
	}, func(r app.UsageReport, err error) {
		if seq != p.usage {
			return
		}
		p.UsageBusy = false
		if err != nil {
			p.onError(err)
			return
		}
		p.Usage = &r
	})
}

// LoadDesktop reads the desktop macros and recipes.
func (p *Pages) LoadDesktop() {
	pageCall(p.post, func() (app.DesktopList, error) {
		if p.api == nil {
			return app.DesktopList{}, errNoService
		}
		return p.api.Desktop(background)
	}, func(l app.DesktopList, err error) {
		if err != nil {
			p.onError(err)
			return
		}
		p.Desktop = &l
	})
}

// LoadVersion reads the app's version for About.
func (p *Pages) LoadVersion() {
	pageCall(p.post, func() (version.Info, error) {
		if p.ver == nil {
			return version.Info{}, errNoService
		}
		return p.ver.Version(background)
	}, func(v version.Info, err error) {
		if err != nil {
			p.onError(err)
			return
		}
		p.Version = &v
	})
}

// desktop runs a change to the macros or recipes, then reads them again;
// then gets the change's error.
func (p *Pages) desktop(fn func(SettingsPagesAPI) error, then func(error)) {
	pageCall(p.post, func() (struct{}, error) {
		if p.api == nil {
			return struct{}{}, errNoService
		}
		return struct{}{}, fn(p.api)
	}, func(_ struct{}, err error) {
		if err == nil {
			p.LoadDesktop()
		}
		then(err)
	})
}

// EditMacro saves the user's edit of a desktop macro.
func (p *Pages) EditMacro(e app.DesktopMacroEdit, then func(error)) {
	p.desktop(func(a SettingsPagesAPI) error { _, err := a.EditDesktopMacro(background, e); return err }, then)
}

// DeleteMacro deletes a desktop macro.
func (p *Pages) DeleteMacro(id int64, then func(error)) {
	p.desktop(func(a SettingsPagesAPI) error { return a.DeleteDesktopMacro(background, id) }, then)
}

// WriteRecipe saves a new recipe, or the user's edit of one.
func (p *Pages) WriteRecipe(w app.DesktopRecipeWrite, then func(error)) {
	p.desktop(func(a SettingsPagesAPI) error { _, err := a.WriteDesktopRecipe(background, w); return err }, then)
}

// DeleteRecipe deletes a desktop recipe.
func (p *Pages) DeleteRecipe(id int64, then func(error)) {
	p.desktop(func(a SettingsPagesAPI) error { return a.DeleteDesktopRecipe(background, id) }, then)
}

// Presets reads the well-known providers for the Connect form.
func (p *Pages) Presets(then func([]app.PresetItem, error)) {
	pageCall(p.post, func() ([]app.PresetItem, error) {
		if p.api == nil {
			return nil, errNoService
		}
		ps, err := p.api.Presets(background)
		if ps == nil {
			ps = []app.PresetItem{}
		}
		return ps, err
	}, then)
}

// ProviderModels reads a provider's models, asking the provider for its
// list.
func (p *Pages) ProviderModels(name string, then func(app.ProviderModels, error)) {
	pageCall(p.post, func() (app.ProviderModels, error) {
		if p.api == nil {
			return app.ProviderModels{}, errNoService
		}
		return p.api.ProviderModels(background, name)
	}, then)
}

// UserMemory reads the user's memory.
func (p *Pages) UserMemory(then func(app.MemoryView, error)) {
	pageCall(p.post, func() (app.MemoryView, error) {
		if p.api == nil {
			return app.MemoryView{}, errNoService
		}
		return p.api.UserMemory(background)
	}, then)
}

// SaveUserMemory saves an edit of a section of the user's memory.
func (p *Pages) SaveUserMemory(e app.MemoryEdit, then func(app.MemorySaved, error)) {
	pageCall(p.post, func() (app.MemorySaved, error) {
		if p.api == nil {
			return app.MemorySaved{}, errNoService
		}
		return p.api.SaveUserMemory(background, e)
	}, then)
}

// CheckFolder checks a folder chosen as the data folder: the warning of
// SPEC 2.1, or nil, or why it can't be one.
func (p *Pages) CheckFolder(dir string, then func(*project.FolderWarning, error)) {
	pageCall(p.post, func() (*project.FolderWarning, error) {
		if p.api == nil {
			return nil, errNoService
		}
		return p.api.CheckFolder(background, dir)
	}, then)
}

// ErrorText is what the user sees of err: a service's message, or the
// error itself.
func ErrorText(err error) string { return errorText(err) }

// The services have what Pages reads.
var (
	_ SettingsPagesAPI = (*app.SettingsService)(nil)
	_ VersionAPI       = (*app.SystemService)(nil)
)
