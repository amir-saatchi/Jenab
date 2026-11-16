package dev_test

import (
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/dev"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
)

const persian = "سلام، این یک آزمایش است"

var started = time.Date(2026, 11, 16, 14, 5, 0, 0, time.UTC)

// fixture is the developer tools on fake services: a project with a
// Persian chat of two recorded turns.
type fixture struct {
	f    *statetest.Fake
	d    *statetest.Dev
	loop *statetest.Loop
	st   *state.Stores
	dev  *state.Dev
	scr  *dev.Screen
	tt   *kit.Tester
	p    id.Project
	ch   id.Chat
}

type options struct {
	off   bool // the developer tools off at this start
	empty bool // no turns recorded
}

func open(t *testing.T, dark bool, o options, at state.Place) *fixture {
	t.Helper()
	x := &fixture{f: statetest.New(), d: statetest.NewDev(), loop: &statetest.Loop{}}
	x.p, _ = x.f.AddProject("Prices")
	x.ch = x.f.AddChat(x.p, persian)
	if !o.empty {
		for n := 1; n <= 2; n++ {
			x.d.Add(statetest.SampleTurn(x.p, "Prices", x.ch, persian, n, started.Add(time.Duration(n)*time.Minute)))
		}
		ref := state.TurnRef{Project: x.p, Chat: x.ch, Turn: 2}
		x.d.Texts[statetest.BlockKey(ref, 0, 1)] = "── system\nYou are Jenab.\n\n" + persian + "\n"
		x.d.Texts[statetest.BlockKey(ref, 0, 3)] = ""
	}
	api := x.f.WithDevTools()
	if o.off {
		api = x.f.API()
	}
	x.st = state.NewStores(api, x.loop.Post)
	t.Cleanup(x.st.Close)
	x.dev = state.NewDev(x.d, x.loop.Post)
	x.dev.After = func(time.Duration, func()) {} // the frames below read again
	x.scr = dev.New(x.st, x.dev)
	x.scr.Now = func() time.Time { return started.Add(10 * time.Minute) }
	x.st.Nav.Go(at)
	tt, err := kit.NewTester(func(c *kit.Context) {
		kit.Root(c, func() {
			kit.Main(c, func() { x.scr.View(c, x.st.Nav.Current()) })
		})
	}, 1280, 900, dark)
	if err != nil {
		t.Fatal(err)
	}
	tt.UseScreenReader()
	x.tt = tt
	x.draw()
	return x
}

// draw draws and runs the reads each frame starts, a few times over, then
// draws twice more: a list lays out its rows in the frame after it learns
// their sizes.
func (x *fixture) draw() {
	for range 4 {
		x.tt.Frame()
		x.loop.Drain(nil)
	}
	x.tt.Advance(time.Second)
	x.tt.Frame()
}

// until draws until cond holds.
func (x *fixture) until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	if !x.loop.Drain(func() bool { x.tt.Frame(); return cond() }) {
		t.Fatalf("%s did not happen: %q", what, x.tt.Texts())
	}
	x.draw()
}

func (x *fixture) click(t *testing.T, name string) {
	t.Helper()
	if err := x.tt.Click(name); err != nil {
		t.Fatalf("click %q: %v in %q", name, err, x.tt.Texts())
	}
	x.draw()
}

// shot saves what the window shows as name, light or dark, when
// JENAB_DEV_SHOTS names a folder: the screenshots of the report.
func (x *fixture) shot(t *testing.T, name string) {
	dir := os.Getenv("JENAB_DEV_SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+"-"+strings.ToLower(strings.TrimPrefix(t.Name()[strings.LastIndex(t.Name(), "/")+1:], "/"))+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, x.tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func shows(t *testing.T, tt *kit.Tester, texts ...string) {
	t.Helper()
	for _, s := range texts {
		if !kit.Shows(tt, s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
}

func hides(t *testing.T, tt *kit.Tester, texts ...string) {
	t.Helper()
	for _, s := range texts {
		if kit.Shows(tt, s) {
			t.Errorf("%q still shows", s)
		}
	}
}

func both(t *testing.T, test func(t *testing.T, dark bool)) {
	t.Run("light", func(t *testing.T) { test(t, false) })
	t.Run("dark", func(t *testing.T) { test(t, true) })
}

// text is the text shown that contains s.
func text(t *testing.T, tt *kit.Tester, s string) string {
	t.Helper()
	for _, x := range tt.Texts() {
		if strings.Contains(x, s) {
			return x
		}
	}
	t.Fatalf("no text has %q: %q", s, tt.Texts())
	return ""
}

func TestTheInspectorShowsTheNewestTurn(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := open(t, dark, options{}, state.Place{View: state.ViewInspector})
		if p := x.dev.Pick; p == nil || p.Turn != 2 {
			t.Fatalf("the pick is %+v, want turn 2", p)
		}
		shows(t, x.tt, "Developer tools", "Turn inspector", "Runtime", "Read-only · secrets redacted",
			"What the context builder sent, and what came back.",
			"Prices · "+persian, "· turn 2 · 2 requests",
			// The timeline.
			"Timeline", "fake/scripted · 3.6 s", "0 s", "4 s", "Model", "req 1", "req 2", "Tools", "Parts",
			"thinking · answer · tool call", "Tool calls: ", "640 ms,", "38 ms.",
			// The requests, the tool calls and the first request's blocks.
			"Requests", "5,420", "7,412", "6,120", "412", "1.2 s", "fetch_url, query",
			"Tool calls", "20.5 KB", "64 B · error", "cache/fetch/1.md",
			"Context blocks, request 1", "System prompt", "· cache point", "2,400", "5,420", "Total", "0 cached (0 %)")
		if !kit.HasRole(x.tt, "heading", "Turn inspector") {
			t.Error("the page has no heading")
		}
		x.shot(t, "inspector")

		// Request 2 shows its blocks, read mostly from the cache.
		x.click(t, "Request 2")
		shows(t, x.tt, "Context blocks, request 2", "6,120 cached (83 %)", "hit", "part", "miss")
		x.shot(t, "request-2")
		hides(t, x.tt, "Context blocks, request 1")
	})
}

func TestTheTurnPickerAndTheKeyboard(t *testing.T) {
	x := open(t, false, options{}, state.Place{View: state.ViewInspector})
	// The turn picker lists the chat's turns; choosing turn 1 shows it.
	x.click(t, "Turn")
	x.click(t, text(t, x.tt, "· turn 1 · "))
	if p := x.dev.Pick; p == nil || p.Turn != 1 {
		t.Fatalf("the pick is %+v, want turn 1", p)
	}
	x.until(t, "turn 1 showing", func() bool { v, _ := x.dev.Shown(); return v != nil && v.Turn == 1 })
	shows(t, x.tt, "· turn 1 · 2 requests")
	// The keyboard reaches a request and Enter chooses it.
	for i := 0; i < 40 && !x.tt.Focused("Request 2"); i++ {
		x.tt.Key(0, kit.KeyTab)
		x.tt.Frame()
	}
	if !x.tt.Focused("Request 2") {
		t.Fatal("Tab never reached Request 2")
	}
	x.tt.Key(0, kit.KeyEnter)
	x.draw()
	shows(t, x.tt, "Context blocks, request 2")
}

func TestABlocksTextOpensInADialog(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := open(t, dark, options{}, state.Place{View: state.ViewInspector})
		x.click(t, "System prompt")
		x.until(t, "the text", func() bool { return x.dev.Block != nil && x.dev.Block.Lines != nil })
		shows(t, x.tt, "Request 1 · about 1,800 tokens · secrets redacted", "── system", "You are Jenab.", persian)
		if !kit.HasRole(x.tt, "list", "System prompt") {
			t.Error("the text is no list named for its block")
		}
		x.shot(t, "block")
		x.tt.Key(0, kit.KeyEscape)
		x.draw()
		if x.dev.Block != nil || kit.Shows(x.tt, "You are Jenab.") {
			t.Error("Escape did not close the text")
		}
		// An empty block says so; a block not recorded says why.
		x.click(t, "This turn")
		x.until(t, "the empty text", func() bool { return x.dev.Block != nil && x.dev.Block.Lines != nil })
		shows(t, x.tt, "(empty)")
		x.tt.Key(0, kit.KeyEscape)
		x.draw()
		x.click(t, "History")
		x.until(t, "the error", func() bool { return x.dev.Block != nil && x.dev.Block.Err != "" })
		shows(t, x.tt, "This block isn't recorded.")
	})
}

func TestTheInspectorOpensAtTheTurnInspected(t *testing.T) {
	x := open(t, false, options{}, state.Place{})
	x.st.Nav.Go(state.Place{View: state.ViewInspector, Project: x.p, Chat: x.ch, Turn: 1})
	x.draw()
	if p := x.dev.Pick; p == nil || p.Turn != 1 {
		t.Fatalf("the pick is %+v, want turn 1", p)
	}
	shows(t, x.tt, "· turn 1 · 2 requests", "Timeline")
	// A turn not recorded says why.
	x.st.Nav.Go(state.Place{View: state.ViewInspector, Project: x.p, Chat: x.ch, Turn: 9})
	x.draw()
	shows(t, x.tt, "Can't show this turn", "This turn isn't recorded.")
}

func TestOffAndNothingRecorded(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := open(t, dark, options{off: true}, state.Place{View: state.ViewInspector})
		shows(t, x.tt, "The developer tools are off", "Turn them on in Settings → Developer. They apply at the next start of Jenab.")
		hides(t, x.tt, "Timeline", "Chat")
		x = open(t, dark, options{empty: true}, state.Place{View: state.ViewInspector})
		shows(t, x.tt, "No turns recorded yet")
	})
}

func runtime(now time.Time) app.Runtime {
	return app.Runtime{
		Calls: app.Slots{Size: 4, InUse: 1},
		Providers: []app.ProviderStatus{
			{Name: "Gemini", Kind: "gemini", Limit: 4, Current: 2, Running: 1, PausedMS: 30_000, LastProblem: "Rate limited"},
			{Name: "Fake", Kind: "fake", Limit: 4, Current: 4},
		},
		Projects: []app.ProjectRuntime{{
			Project: "p1", Name: persian, Leases: 1,
			Work: []app.WorkView{{ID: "w1", Kind: "chat", Title: "Notes", State: "running", Progress: "step 2",
				Started: now.Add(-65 * time.Second), IdleMS: 3000, LimitMS: 120_000, Stuck: true}},
			Writers: []app.WriterView{
				{File: "project.db", Interactive: 1, Background: 2, Busy: true, RunningMS: 40, Done: 1234},
				{File: "chats.db", Done: 7, LastError: "disk full"},
			},
		}},
		Stuck: 1,
	}
}

func TestTheRuntime(t *testing.T) {
	both(t, func(t *testing.T, dark bool) {
		x := open(t, dark, options{}, state.Place{View: state.ViewRuntime})
		x.d.RT = runtime(x.scr.Now())
		x.until(t, "the runtime", func() bool { return x.dev.Runtime != nil && x.dev.Runtime.Stuck == 1 })
		shows(t, x.tt, "What runs now. Reads again every second.",
			"1 thing hasn't moved for longer than its limit", "They are marked Stuck below.",
			"LLM calls", "1 of 4 slots in use, 0 waiting.", "Gemini", "2 of 4", "Paused for 30 s · Rate limited", "OK",
			persian, "1 running. Held open by 1 lease.", "Notes", "chat", "· step 2", "1 min 5 s", "3 s ago", "2 min", "Stuck",
			"project.db", "1 · 2", "Writing for 40 ms", "1,234", "chats.db", "Idle", "disk full",
			"Queue: interactive · background requests waiting.")
		x.shot(t, "runtime")

		// A failed read keeps the last runtime under the error.
		x.d.RTError = errors.New("the runtime is gone")
		x.tt.Advance(2 * time.Second)
		x.dev.Now = func() time.Time { return time.Now().Add(time.Hour) }
		x.until(t, "the error", func() bool { return x.dev.RuntimeError != "" })
		shows(t, x.tt, "Can't read the runtime", "the runtime is gone", "LLM calls")

		// The bar goes to the inspector.
		x.click(t, "Turn inspector")
		if p := x.st.Nav.Current(); p == nil || p.View != state.ViewInspector {
			t.Fatalf("the place is %+v", p)
		}
		shows(t, x.tt, "Timeline")
	})
}

func TestNoProjectIsOpen(t *testing.T) {
	x := open(t, false, options{}, state.Place{View: state.ViewRuntime})
	shows(t, x.tt, "0 of 0 slots in use, 0 waiting.", "No project is open.")
}
