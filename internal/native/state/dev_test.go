package state

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
)

// fakeDev is the developer tools' service: the turns it lists, each
// turn's answers in order (the last repeats), and the runtime.
type fakeDev struct {
	mu      sync.Mutex
	turns   []app.TurnItem
	answers map[string][]turnAnswer
	reads   map[string]int
	rt      []app.Runtime
	rtErr   error
	rtReads int
	block   string
}

type turnAnswer struct {
	view app.TurnView
	err  error
}

func newFakeDev() *fakeDev {
	return &fakeDev{answers: map[string][]turnAnswer{}, reads: map[string]int{}}
}

func (f *fakeDev) Turns(context.Context) ([]app.TurnItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.turns, nil
}

func (f *fakeDev) Turn(_ context.Context, p id.Project, c id.Chat, n int) (app.TurnView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := TurnRef{p, c, n}.Key()
	as := f.answers[key]
	i := min(f.reads[key], len(as)-1)
	f.reads[key]++
	if i < 0 {
		return app.TurnView{}, errors.New("not recorded")
	}
	return as[i].view, as[i].err
}

func (f *fakeDev) Block(_ context.Context, _ id.Project, _ id.Chat, _, request, block int) (string, error) {
	if request > 0 {
		return "", &app.UIError{Message: "This request's texts were dropped to save memory; newer turns keep theirs."}
	}
	return f.block, nil
}

func (f *fakeDev) Runtime(context.Context) (app.Runtime, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rtReads++
	if f.rtErr != nil {
		return app.Runtime{}, f.rtErr
	}
	return f.rt[min(f.rtReads, len(f.rt))-1], nil
}

func (f *fakeDev) count(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads[key]
}

// clock is a clock the test moves, and the timers it set.
type clock struct {
	now    time.Time
	timers []time.Duration
}

func newDev(f *fakeDev) (*Dev, *loop, *clock) {
	l := newLoop()
	d := NewDev(f, l.post)
	k := &clock{now: time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)}
	d.Now = func() time.Time { return k.now }
	d.After = func(by time.Duration, fn func()) { k.timers = append(k.timers, by) }
	return d, l, k
}

func turnItem(c id.Chat, n int, running bool) app.TurnItem {
	return app.TurnItem{Project: "p", ProjectName: "Prices", Chat: c, Turn: n, Running: running}
}

func TestTheInspectorOpensAtTheNewestTurn(t *testing.T) {
	f := newFakeDev()
	f.turns = []app.TurnItem{turnItem("c2", 3, false), turnItem("c1", 2, false)}
	d, l, _ := newDev(f)
	d.OpenInspector(nil)
	until(t, l, func() bool { return d.Pick != nil })
	same(t, *d.Pick, TurnRef{"p", "c2", 3})

	// At a turn, as Inspect opens it, it stays there. Turns is cleared
	// first so until waits for this load, not the first one, which a
	// load landing late could otherwise refill below.
	d.Turns = nil
	d.OpenInspector(&TurnRef{"p", "c1", 2})
	until(t, l, func() bool { return d.Turns != nil })
	same(t, *d.Pick, TurnRef{"p", "c1", 2})

	// With no turns recorded the list is empty, not missing.
	f.turns = nil
	d.Turns = nil
	d.OpenInspector(nil)
	until(t, l, func() bool { return d.Turns != nil })
	same(t, len(d.Turns), 0)
	same(t, d.Pick == nil, true)
}

func TestARunningTurnIsReadEverySecondUntilItEnds(t *testing.T) {
	f := newFakeDev()
	ref := TurnRef{"p", "c", 1}
	f.turns = []app.TurnItem{turnItem("c", 1, true)}
	running := app.TurnView{TurnItem: turnItem("c", 1, true), TookMS: 500}
	ended := app.TurnView{TurnItem: turnItem("c", 1, false), TookMS: 1800}
	f.answers[ref.Key()] = []turnAnswer{{view: running}, {view: ended}}
	d, l, k := newDev(f)
	d.OpenInspector(&ref)
	d.PollTurn()
	d.PollTurn() // busy: no second read
	until(t, l, func() bool { v, _ := d.Shown(); return v != nil })
	v, _ := d.Shown()
	same(t, v.TookMS, int64(500))
	same(t, k.timers, []time.Duration{time.Second})

	d.PollTurn() // not due yet
	l.drain()
	same(t, f.count(ref.Key()), 1)

	k.now = k.now.Add(time.Second)
	f.mu.Lock()
	f.turns = []app.TurnItem{turnItem("c", 1, false)}
	f.mu.Unlock()
	d.PollTurn()
	until(t, l, func() bool { v, _ := d.Shown(); return !v.Running })
	until(t, l, func() bool { return len(d.Turns) == 1 && !d.Turns[0].Running }) // the list says it ended

	k.now = k.now.Add(time.Minute)
	d.PollTurn() // done: never read again
	l.drain()
	same(t, f.count(ref.Key()), 2)
}

func TestAFailedReadWaitsLongerEachTime(t *testing.T) {
	f := newFakeDev()
	ref := TurnRef{"p", "c", 1}
	fail := errors.New("the project is closed")
	f.answers[ref.Key()] = []turnAnswer{{err: fail}, {err: fail}, {view: app.TurnView{TurnItem: turnItem("c", 1, true)}}, {err: fail}}
	d, l, k := newDev(f)
	d.Choose(ref)
	read := func() {
		t.Helper()
		n := f.count(ref.Key())
		d.PollTurn()
		until(t, l, func() bool { return f.count(ref.Key()) == n+1 && !d.turn.busy })
	}
	read()
	_, err := d.Shown()
	same(t, err, "the project is closed")
	// Not again before the wait is over.
	d.PollTurn()
	same(t, d.turn.busy, false)
	k.now = k.now.Add(2 * time.Second)
	read()
	same(t, k.timers, []time.Duration{2 * time.Second, 4 * time.Second})

	k.now = k.now.Add(4 * time.Second)
	read()
	k.now = k.now.Add(time.Second)
	read()
	// A turn on screen stays there while the read is tried again.
	v, err := d.Shown()
	same(t, v != nil, true)
	same(t, err, "")
	same(t, k.timers[len(k.timers)-1], 2*time.Second)
}

func TestANewPickNeverShowsTheOldTurn(t *testing.T) {
	f := newFakeDev()
	a, b := TurnRef{"p", "c", 1}, TurnRef{"p", "c", 2}
	f.answers[a.Key()] = []turnAnswer{{view: app.TurnView{TurnItem: turnItem("c", 1, false)}}}
	f.answers[b.Key()] = []turnAnswer{{view: app.TurnView{TurnItem: turnItem("c", 2, false)}}}
	d, l, _ := newDev(f)
	d.Choose(a)
	d.PollTurn()
	d.Choose(b) // before a's read is back
	v, err := d.Shown()
	same(t, v == nil && err == "", true)
	d.PollTurn()
	until(t, l, func() bool { v, _ := d.Shown(); return v != nil })
	l.drain()
	v, _ = d.Shown()
	same(t, v.Turn, 2)
}

func TestTheRuntimeIsReadASecondAfterEachRead(t *testing.T) {
	f := newFakeDev()
	f.rt = []app.Runtime{{Stuck: 0}, {Stuck: 2}}
	d, l, k := newDev(f)
	d.OpenRuntime()
	d.PollRuntime()
	until(t, l, func() bool { return d.Runtime != nil })
	d.PollRuntime()
	l.drain()
	same(t, f.rtReads, 1)
	k.now = k.now.Add(time.Second)
	d.PollRuntime()
	until(t, l, func() bool { return d.Runtime.Stuck == 2 })

	// A failure keeps the last runtime and says why.
	f.mu.Lock()
	f.rtErr = errors.New("the registry is closed")
	f.mu.Unlock()
	k.now = k.now.Add(time.Second)
	d.PollRuntime()
	until(t, l, func() bool { return d.RuntimeError != "" })
	same(t, d.Runtime.Stuck, 2)
	same(t, len(k.timers), 3)

	d.OpenRuntime()
	same(t, d.Runtime == nil && d.RuntimeError == "", true)
}

func TestABlockShowsItsTextOrWhyNot(t *testing.T) {
	f := newFakeDev()
	f.block = "── system\r\nYou are Jenab.\r\n"
	v := &app.TurnView{TurnItem: turnItem("c", 1, false), Requests: []app.RequestView{
		{Blocks: []app.BlockView{{Name: "System prompt", Tokens: 1200}}},
		{Dropped: true, Blocks: []app.BlockView{{Name: "System prompt", Tokens: 1200}}},
	}}
	d, l, _ := newDev(f)
	d.OpenBlock(v, 0, 0)
	same(t, d.Block.Name, "System prompt")
	same(t, d.Block.Tokens, 1200)
	until(t, l, func() bool { return d.Block.Lines != nil })
	same(t, d.Block.Lines, []string{"── system", "You are Jenab."})

	d.OpenBlock(v, 1, 0)
	until(t, l, func() bool { return d.Block.Err != "" })
	same(t, d.Block.Err, "This request's texts were dropped to save memory; newer turns keep theirs.")
	d.CloseBlock()
	same(t, d.Block == nil, true)
	same(t, Lines(""), []string{""})
}
