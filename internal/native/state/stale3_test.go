package state

// TASK-078: a failed host change reads the list again, and a project's
// host changes run one after another.

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
)

// storedHosts keeps a project's list as the service would. A read takes
// the list when it is made and answers when the test says; a save stores
// the list when the test answers it with no error.
type storedHosts struct {
	mu           sync.Mutex
	list         []string
	reads, saves gate[error]
}

func (g *storedHosts) set(hs ...string) {
	g.mu.Lock()
	g.list = hs
	g.mu.Unlock()
}

func (g *storedHosts) get() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.list)
}

func (g *storedHosts) PrivateHosts(context.Context, id.Project) ([]string, error) {
	hs := g.get()
	if err := g.reads.wait(); err != nil {
		return nil, err
	}
	return hs, nil
}

func (g *storedHosts) SetPrivateHosts(_ context.Context, _ id.Project, hs []string) ([]string, error) {
	if err := g.saves.wait(); err != nil {
		return nil, err
	}
	g.set(slices.Clone(hs)...)
	return hs, nil
}

func TestAFailedHostChangeReadsTheListAgain(t *testing.T) {
	g, l := &storedHosts{}, newLoop()
	g.set("nas.local")
	h := NewHosts(g, l.post, func(err error) { t.Error(err) })
	h.ByProject["p1"] = []string{"nas.local"}

	saved := make(chan error, 1)
	h.Change("p1", func(hs []string) []string { return WithoutHost(hs, "nas.local") }, func(err error) { saved <- err })
	g.reads.came(t, l, 1)
	g.reads.answer(0, nil)
	g.saves.came(t, l, 1)
	g.set("nas.local", "printer.local") // a card adds a host
	h.Load("p1")                        // and the page reads the list during the save
	g.reads.came(t, l, 2)
	g.saves.answer(0, errors.New("the disk is full"))
	if err := await(t, l, saved); err == nil {
		t.Fatal("the change didn't fail")
	}
	g.reads.answer(1, nil) // read during the save: dropped
	g.reads.came(t, l, 3)  // so the list is read again
	g.reads.answer(2, nil)
	until(t, l, func() bool { return len(h.ByProject["p1"]) == 2 })
	same(t, h.ByProject["p1"], []string{"nas.local", "printer.local"})
	same(t, h.Busy["p1"], false)
}

func TestHostChangesRunOneAfterAnother(t *testing.T) {
	g, l := &storedHosts{}, newLoop()
	g.set("nas.local")
	h := NewHosts(g, l.post, func(err error) { t.Error(err) })
	h.ByProject["p1"] = []string{"nas.local"}

	first, second := make(chan error, 1), make(chan error, 1)
	h.Change("p1", func(hs []string) []string { return WithHost(hs, "printer.local") }, func(err error) { first <- err })
	h.Change("p1", func(hs []string) []string { return WithoutHost(hs, "nas.local") }, func(err error) { second <- err })
	g.reads.came(t, l, 1)
	g.reads.answer(0, nil)
	g.saves.came(t, l, 1)
	g.saves.answer(0, nil)
	if err := await(t, l, first); err != nil {
		t.Fatal(err)
	}
	same(t, h.Busy["p1"], true) // the second still runs
	g.reads.came(t, l, 2)
	g.reads.answer(1, nil)
	g.saves.came(t, l, 2)
	g.saves.answer(1, nil)
	if err := await(t, l, second); err != nil {
		t.Fatal(err)
	}
	same(t, g.get(), []string{"printer.local"})
	same(t, h.ByProject["p1"], []string{"printer.local"})
	same(t, h.Busy["p1"], false)
}
