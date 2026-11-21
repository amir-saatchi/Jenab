package links_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/links"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
)

// The Links tab alone, on fake services, as the shell draws it: Panel in
// a frame, then EndFrame.

const faTitle = "می‌خواهم یک کتاب بخرم"

// flaky is a links service whose List fails until ok is set.
type flaky struct {
	state.LinksAPI
	mu sync.Mutex
	ok bool
}

func (f *flaky) List(ctx context.Context, p id.Project) ([]app.Link, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.ok {
		return nil, errors.New("the project can't be read")
	}
	return f.LinksAPI.List(ctx, p)
}

type fixture struct {
	f    *statetest.Fake
	loop *statetest.Loop
	st   *state.Stores
	scr  *links.Screen
	tt   *kit.Tester
	p, q id.Project
	// shown is the project whose tab the frame draws; "" draws none.
	shown id.Project
}

func start(t *testing.T, dark bool, wrap func(state.LinksAPI) state.LinksAPI) *fixture {
	t.Helper()
	x := &fixture{f: statetest.New(), loop: &statetest.Loop{}}
	x.p, _ = x.f.AddProject("Prices")
	x.q, _ = x.f.AddProject("Books")
	x.f.AddLink(x.p, id.SourceUser, "https://www.example.com/btc", "Bitcoin news", "Daily prices", "btc")
	x.f.AddLink(x.p, id.SourceUser, "https://news.example.ir/x", faTitle, "", "خبر")
	api := x.f.API()
	if wrap != nil {
		api.Links = wrap(api.Links)
	}
	x.st = state.NewStores(api, x.loop.Post)
	t.Cleanup(x.st.Close)
	x.scr = links.New(x.st)
	x.shown = x.p
	tt, err := kit.NewTester(func(c *kit.Context) {
		kit.Root(c, func() {
			if x.shown != "" {
				kit.Column(c).Grow(1).AlignItems(kit.Stretch).Children(func() { x.scr.Panel(c, x.shown) })
			}
		})
		x.scr.EndFrame()
	}, 420, 640, dark)
	if err != nil {
		t.Fatal(err)
	}
	tt.UseScreenReader()
	x.tt = tt
	x.draw()
	return x
}

// draw draws and runs what the frames started, then once more: a list
// lays out its rows in the frame after it learns their sizes.
func (x *fixture) draw() {
	for range 3 {
		x.tt.Frame()
		x.loop.Drain(nil)
	}
	x.tt.Advance(time.Second)
	x.tt.Frame()
}

func both(t *testing.T, test func(t *testing.T, dark bool)) {
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) { test(t, dark) })
	}
}

// The tab lists the links in a tree and names its search and its Add
// button; a Persian title starts on the right; the keyboard reaches the
// search and Add, whose dialog Escape closes.
func TestPanel(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := start(t, dark, nil)
		if !kit.HasRole(x.tt, "tree", "Links") || !kit.Shows(x.tt, "Bitcoin news") || !kit.Shows(x.tt, faTitle) {
			t.Fatalf("the list: %q", x.tt.Texts())
		}
		if _, ok := x.tt.Find("Search links"); !ok {
			t.Error("no search named Search links")
		}
		if _, ok := x.tt.Find("Add link"); !ok {
			t.Error("no Add link button")
		}
		// A row is named by its title, the Persian one as it is: its text
		// is drawn isolated, so it can't reorder the row.
		if !kit.HasRole(x.tt, "treeitem", faTitle) || !x.tt.HasText("⁨"+faTitle+"⁩") {
			t.Errorf("the Persian row: %q", x.tt.Texts())
		}

		x.tt.Key(0, kit.KeyTab)
		x.draw()
		x.tt.Type("nothing-like-it")
		x.draw()
		if !kit.Shows(x.tt, "No links match") {
			t.Fatalf("the search typed from the keyboard: %q", x.tt.Texts())
		}
		x.tt.Key(0, kit.KeyTab)
		x.draw()
		x.tt.Key(0, kit.KeyEnter)
		x.draw()
		if !kit.HasRole(x.tt, "dialog", "Add link") {
			t.Fatalf("Enter on Add link: %q", x.tt.Texts())
		}
		x.tt.Key(0, kit.KeyEscape)
		x.draw()
		if x.st.Links.Editor != nil {
			t.Fatal("Escape left the dialog open")
		}
	})
}

// "Links can't be shown" offers Try again, which reads the list again.
func TestPanelTryAgain(t *testing.T) {
	var fl *flaky
	x := start(t, false, func(a state.LinksAPI) state.LinksAPI { fl = &flaky{LinksAPI: a}; return fl })
	if !kit.Shows(x.tt, "Links can't be shown") || !kit.Shows(x.tt, "the project can't be read") {
		t.Fatalf("the error: %q", x.tt.Texts())
	}
	fl.mu.Lock()
	fl.ok = true
	fl.mu.Unlock()
	if err := x.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	x.draw()
	if !kit.Shows(x.tt, "Bitcoin news") || kit.Shows(x.tt, "Links can't be shown") {
		t.Fatalf("after Try again: %q", x.tt.Texts())
	}
}

// The dialog is drawn only in its project's tab, and EndFrame closes it
// when that tab isn't drawn.
func TestPanelDialogFollowsItsTab(t *testing.T) {
	x := start(t, false, nil)
	x.st.Links.StartAdd(x.p)
	x.draw()
	if !kit.HasRole(x.tt, "dialog", "Add link") {
		t.Fatal("no dialog")
	}
	x.shown = x.q
	x.draw()
	if x.st.Links.Editor != nil || kit.HasRole(x.tt, "dialog", "Add link") {
		t.Fatal("the first project's dialog shows in the second's tab")
	}
	x.st.Links.StartAdd(x.q)
	x.draw()
	x.shown = ""
	x.draw()
	if x.st.Links.Editor != nil {
		t.Fatal("the dialog stayed with no tab drawn")
	}
}
