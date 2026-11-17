package dev_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/native/dev"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
)

// failingStarted is the settings whose Started fails until ok is set.
type failingStarted struct {
	state.SettingsAPI
	calls atomic.Int32
	ok    atomic.Bool
}

func (f *failingStarted) Started(context.Context) (app.Started, error) {
	f.calls.Add(1)
	if !f.ok.Load() {
		return app.Started{}, errors.New("the service is gone")
	}
	return app.Started{DevTools: true}, nil
}

// A failed read of what this start used shows with Try again (TASK-072),
// and isn't read again each frame.
func TestAFailedStartReadShowsTryAgain(t *testing.T) {
	f, loop := statetest.New(), &statetest.Loop{}
	f.AddProject("Prices")
	api := f.API()
	settings := &failingStarted{SettingsAPI: api.Settings}
	api.Settings = settings
	st := state.NewStores(api, loop.Post)
	t.Cleanup(st.Close)
	d := state.NewDev(statetest.NewDev(), loop.Post)
	d.After = func(time.Duration, func()) {}
	scr := dev.New(st, d)
	st.Nav.Go(state.Place{View: state.ViewRuntime})
	tt, err := kit.NewTester(func(c *kit.Context) {
		kit.Root(c, func() { kit.Main(c, func() { scr.View(c, st.Nav.Current()) }) })
	}, 1280, 900, false)
	if err != nil {
		t.Fatal(err)
	}
	tt.UseScreenReader()
	draw := func() {
		for range 10 {
			tt.Frame()
			loop.Drain(nil)
		}
	}
	draw()
	if !tt.HasText("the service is gone") || !kit.Shows(tt, "Try again") {
		t.Fatalf("no error with Try again in %q", tt.Texts())
	}
	if tt.HasText("Loading") {
		t.Error("the spinner shows with the error")
	}
	if n := settings.calls.Load(); n != 1 {
		t.Errorf("read %d times in 10 frames, want 1", n)
	}

	settings.ok.Store(true)
	if err := tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	draw()
	if tt.HasText("the service is gone") {
		t.Errorf("the error still shows: %q", tt.Texts())
	}
	if n := settings.calls.Load(); n != 2 {
		t.Errorf("read %d times after Try again, want 2", n)
	}
	if st.Settings.Started == nil || !st.Settings.Started.DevTools {
		t.Error("what this start used wasn't read")
	}
}
