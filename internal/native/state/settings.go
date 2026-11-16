package state

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/config"
)

// SettingsAPI is what Settings reads and saves; *app.SettingsService has
// it.
type SettingsAPI interface {
	Get(ctx context.Context) (app.SettingsView, error)
	Save(ctx context.Context, next config.Settings) (app.SettingsView, error)
	Models(ctx context.Context) ([]app.ModelGroup, error)
	Started(ctx context.Context) (app.Started, error)
	Connect(ctx context.Context, req app.ConnectRequest) (app.ConnectResult, error)
	Remove(ctx context.Context, name string) (app.SettingsView, error)
}

// Settings is the settings, the models that are on and what this start
// used (state/settings.ts).
//
// Save replaces all the settings, so the calls that read or replace them
// run one at a time, in a queue of their own, each starting from the
// settings the one before it saved. An edit shows at once: View is the
// settings as last saved with the edits not saved yet made to a copy.
type Settings struct {
	View    *app.SettingsView
	Models  []app.ModelGroup // nil until read
	Started *app.Started
	// OnTheme gets the theme of every view shown: "light", "dark" or
	// "system". The native app sets mygo's theme source from it.
	OnTheme func(theme string)

	api     SettingsAPI
	post    Post
	onError func(error)
	pending []*pendingEdit // UI thread
	// models counts the model reads started, from any goroutine;
	// modelsShown is the one whose answer shows (UI thread), so an older
	// answer that comes last doesn't replace a newer one.
	models      atomic.Int64
	modelsShown int64

	mu      sync.Mutex
	saved   *app.SettingsView // as Go last sent it; written by the queue
	queue   []func()
	running bool
}

type pendingEdit struct{ change func(*config.Settings) }

// NewSettings makes the store. onError gets the errors of work no caller
// waits for, as reading the models after a save.
func NewSettings(api SettingsAPI, post Post, onError func(error)) *Settings {
	if onError == nil {
		onError = func(error) {}
	}
	return &Settings{api: api, post: post, onError: onError}
}

// enqueue runs job in the queue's goroutine once the jobs before it are
// done, failed or not.
func (s *Settings) enqueue(job func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = append(s.queue, job)
	if !s.running {
		s.running = true
		go s.work()
	}
}

func (s *Settings) work() {
	for {
		s.mu.Lock()
		if len(s.queue) == 0 {
			s.running = false
			s.mu.Unlock()
			return
		}
		job := s.queue[0]
		s.queue = s.queue[1:]
		s.mu.Unlock()
		job()
	}
}

func cloneSettings(c config.Settings) config.Settings {
	b, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	var out config.Settings
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	return out
}

// withPending is the saved view with the pending edits made to a copy.
func (s *Settings) withPending() *app.SettingsView {
	s.mu.Lock()
	saved := s.saved
	s.mu.Unlock()
	if saved == nil || len(s.pending) == 0 {
		return saved
	}
	v := *saved
	v.Settings = cloneSettings(saved.Settings)
	for _, p := range s.pending {
		p.change(&v.Settings)
	}
	return &v
}

// show puts the saved view with the pending edits on screen, and applies
// its theme. UI thread.
func (s *Settings) show() {
	s.View = s.withPending()
	if s.View != nil && s.OnTheme != nil {
		s.OnTheme(ReadTheme(s.View.Settings.UI.Theme))
	}
}

// keep makes v the saved view and shows it. Queue goroutine.
func (s *Settings) keep(v app.SettingsView) {
	s.mu.Lock()
	s.saved = &v
	s.mu.Unlock()
	s.post(s.show)
}

// ReadTheme is a theme setting as the app takes it: anything but light and
// dark is System.
func ReadTheme(v string) string {
	if v == "light" || v == "dark" {
		return v
	}
	return "system"
}

// Load reads the settings.
func (s *Settings) Load() <-chan error {
	done := make(chan error, 1)
	s.enqueue(func() {
		v, err := s.api.Get(background)
		if err == nil {
			s.keep(v)
		}
		s.post(func() { done <- err })
	})
	return done
}

// LoadModels reads the models that are on, for the model picker.
func (s *Settings) LoadModels() <-chan error {
	done := make(chan error, 1)
	go func() { done <- s.loadModels() }()
	return done
}

func (s *Settings) loadModels() error {
	seq := s.models.Add(1)
	gs, err := s.api.Models(background)
	if err != nil {
		return err
	}
	if gs == nil {
		gs = []app.ModelGroup{}
	}
	s.post(func() {
		if seq > s.modelsShown {
			s.modelsShown, s.Models = seq, gs
		}
	})
	return nil
}

// LoadStarted reads what this start of the app used: the data folder and
// the developer tools apply at the next start.
func (s *Settings) LoadStarted() <-chan error {
	done := make(chan error, 1)
	go func() {
		st, err := s.api.Started(background)
		s.post(func() {
			if err == nil {
				s.Started = &st
			}
			done <- err
		})
	}()
	return done
}

// Edit makes a change to the settings. The view shows it at once; it is
// saved after the edits before it, on top of the settings as last saved,
// since Save replaces all of them. A failed save takes it out again.
// change only sets values: it runs on a copy each time the view is shown
// and once more for the save. An edit made before the settings are read
// reads them first, and shows once they are.
func (s *Settings) Edit(change func(*config.Settings)) <-chan error {
	done := make(chan error, 1)
	p := &pendingEdit{change}
	s.pending = append(s.pending, p)
	s.show()
	s.enqueue(func() {
		end := func(err error) {
			s.post(func() {
				s.pending = slices.DeleteFunc(s.pending, func(q *pendingEdit) bool { return q == p })
				s.show()
				done <- err
			})
		}
		s.mu.Lock()
		saved := s.saved
		s.mu.Unlock()
		if saved == nil {
			v, err := s.api.Get(background)
			if err != nil {
				end(err)
				return
			}
			s.keep(v)
			saved = &v
		}
		next := cloneSettings(saved.Settings)
		change(&next)
		v, err := s.api.Save(background, next)
		if err == nil {
			s.mu.Lock()
			s.saved = &v
			s.mu.Unlock()
		}
		end(err)
		if err == nil {
			if err := s.loadModels(); err != nil {
				s.onError(err)
			}
		}
	})
	return done
}

// Connect connects a provider, or replaces its key, in turn with the
// edits, so no save in flight drops it.
func (s *Settings) Connect(req app.ConnectRequest, then func(app.ConnectResult, error)) {
	s.enqueue(func() {
		r, err := s.api.Connect(background, req)
		if err == nil {
			s.keep(r.View)
			if err := s.loadModels(); err != nil {
				s.onError(err)
			}
		}
		s.post(func() { then(r, err) })
	})
}

// RemoveProvider disconnects a provider and deletes its key.
func (s *Settings) RemoveProvider(name string) <-chan error {
	done := make(chan error, 1)
	s.enqueue(func() {
		v, err := s.api.Remove(background, name)
		if err == nil {
			s.keep(v)
		}
		// Read again either way: a failed remove may have done part of it.
		if v, err := s.api.Get(background); err == nil {
			s.keep(v)
		} else {
			s.onError(err)
		}
		if err := s.loadModels(); err != nil {
			s.onError(err)
		}
		s.post(func() { done <- err })
	})
	return done
}

// SetTheme is an edit: the theme applies at once, and a failed save puts
// the saved theme back.
func (s *Settings) SetTheme(theme string) <-chan error {
	return s.Edit(func(c *config.Settings) { c.UI.Theme = theme })
}
