package state

// Ported from the React app's state/settings.test.ts.

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/config"
)

// fakeSettings saves like Go: Save replaces all the settings. Each Save
// waits until release is called, so edits can overlap.
type fakeSettings struct {
	mu       sync.Mutex
	disk     config.Settings
	saves    []config.Settings
	waiting  []chan struct{}
	failNext bool
}

func (f *fakeSettings) view() app.SettingsView {
	return app.SettingsView{Settings: cloneSettings(f.disk), Problems: []config.Problem{}}
}

func (f *fakeSettings) Get(context.Context) (app.SettingsView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.view(), nil
}

func (f *fakeSettings) Models(context.Context) ([]app.ModelGroup, error) { return nil, nil }

func (f *fakeSettings) Started(context.Context) (app.Started, error) { return app.Started{}, nil }

func (f *fakeSettings) Remove(context.Context, string) (app.SettingsView, error) { return f.Get(nil) }

func (f *fakeSettings) Connect(_ context.Context, req app.ConnectRequest) (app.ConnectResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disk = cloneSettings(f.disk)
	f.disk.LLM.Providers = map[string]config.ProviderSettings{req.Name: {}}
	return app.ConnectResult{View: f.view(), Provider: req.Name}, nil
}

func (f *fakeSettings) Save(_ context.Context, next config.Settings) (app.SettingsView, error) {
	ch := make(chan struct{})
	f.mu.Lock()
	f.saves = append(f.saves, cloneSettings(next))
	f.waiting = append(f.waiting, ch)
	f.mu.Unlock()
	<-ch
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext {
		f.failNext = false
		return app.SettingsView{}, errors.New("disk full")
	}
	f.disk = cloneSettings(next)
	return f.view(), nil
}

// release lets the next Save finish.
func (f *fakeSettings) release(t *testing.T, l *loop) {
	t.Helper()
	until(t, l, func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return len(f.waiting) > 0
	})
	f.mu.Lock()
	ch := f.waiting[0]
	f.waiting = f.waiting[1:]
	f.mu.Unlock()
	close(ch)
}

func (f *fakeSettings) onDisk() config.Settings {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.disk
}

func newSettings(t *testing.T) (*Settings, *fakeSettings, *loop, *[]string) {
	f := &fakeSettings{}
	f.disk.Approvals.DefaultLevel = "standard"
	f.disk.UI.Theme = "system"
	l := newLoop()
	s := NewSettings(f, l.post, nil)
	themes := &[]string{}
	s.OnTheme = func(c string) { *themes = append(*themes, c) }
	if err := await(t, l, s.Load()); err != nil {
		t.Fatal(err)
	}
	return s, f, l, themes
}

func lastOf(xs []string) string { return xs[len(xs)-1] }

func TestTwoQuickEditsBothSurvive(t *testing.T) {
	s, f, l, _ := newSettings(t)
	a := s.Edit(func(x *config.Settings) { x.DevTools = true })
	b := s.Edit(func(x *config.Settings) { x.Approvals.DefaultLevel = "auto" })
	// Both show at once, before any save is back.
	same(t, s.View.Settings.DevTools, true)
	same(t, s.View.Settings.Approvals.DefaultLevel, "auto")
	f.release(t, l)
	f.release(t, l)
	for _, ch := range []<-chan error{a, b} {
		if err := await(t, l, ch); err != nil {
			t.Fatal(err)
		}
	}
	same(t, len(f.saves), 2)
	same(t, f.saves[1].DevTools, true) // the second save starts from the first
	disk := f.onDisk()
	same(t, disk.DevTools, true)
	same(t, disk.Approvals.DefaultLevel, "auto")
	same(t, s.View.Settings, disk)
}

func TestAFailedSaveTakesItsEditOut(t *testing.T) {
	s, f, l, _ := newSettings(t)
	f.failNext = true
	a := s.Edit(func(x *config.Settings) { x.DevTools = true })
	b := s.Edit(func(x *config.Settings) { x.Approvals.DefaultLevel = "strict" })
	f.release(t, l)
	if err := await(t, l, a); err == nil || err.Error() != "disk full" {
		t.Fatalf("err %v", err)
	}
	same(t, s.View.Settings.DevTools, false)
	same(t, s.View.Settings.Approvals.DefaultLevel, "strict") // still pending
	f.release(t, l)
	await(t, l, b)
	same(t, f.onDisk().DevTools, false)
	same(t, f.onDisk().Approvals.DefaultLevel, "strict")
}

func TestSetThemeAppliesAtOnceAndGoesBackIfItsSaveFails(t *testing.T) {
	s, f, l, themes := newSettings(t)
	a := s.Edit(func(x *config.Settings) { x.DevTools = true })
	th := s.SetTheme("dark")
	same(t, lastOf(*themes), "dark")
	f.release(t, l)
	await(t, l, a)
	same(t, f.saves[0].UI.Theme, "system") // the theme waits for the edit before it
	f.release(t, l)
	await(t, l, th)
	same(t, f.onDisk().UI.Theme, "dark")
	same(t, f.onDisk().DevTools, true)

	f.mu.Lock()
	f.failNext = true
	f.mu.Unlock()
	u := s.SetTheme("light")
	same(t, lastOf(*themes), "light")
	f.release(t, l)
	if err := await(t, l, u); err == nil {
		t.Fatal("no error")
	}
	same(t, lastOf(*themes), "dark")
	same(t, s.View.Settings.UI.Theme, "dark")
}

func TestAConnectWaitsForTheSaveBeforeIt(t *testing.T) {
	s, f, l, _ := newSettings(t)
	a := s.Edit(func(x *config.Settings) { x.DevTools = true })
	c := make(chan error, 1)
	s.Connect(app.ConnectRequest{Name: "groq"}, func(_ app.ConnectResult, err error) { c <- err })
	f.release(t, l)
	await(t, l, a)
	if err := await(t, l, c); err != nil {
		t.Fatal(err)
	}
	same(t, f.onDisk().DevTools, true) // the connect started from the saved edit
	same(t, f.onDisk().LLM.Providers, map[string]config.ProviderSettings{"groq": {}})
	b := s.Edit(func(x *config.Settings) { x.UI.Theme = "dark" })
	f.release(t, l)
	await(t, l, b)
	same(t, f.saves[len(f.saves)-1].LLM.Providers, map[string]config.ProviderSettings{"groq": {}})
}
