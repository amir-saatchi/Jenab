package state

// TASK-079: a host a card adds while a page change is on its way isn't
// saved over.

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
)

// lockedHosts is the service: mu is the project's lock, which a card's
// add takes too. Each call waits on calls as it comes, as if on its way;
// a read takes the list before that, as its answer is what was stored.
type lockedHosts struct {
	mu    sync.Mutex
	list  []string
	calls gate[error]
}

func (g *lockedHosts) get() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.list)
}

// add is a card's "Allow in this project".
func (g *lockedHosts) add(h string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.list = WithHost(g.list, h)
}

func (g *lockedHosts) PrivateHosts(context.Context, id.Project) ([]string, error) {
	hs := g.get()
	return hs, g.calls.wait()
}

func (g *lockedHosts) SetPrivateHosts(_ context.Context, _ id.Project, hs []string) ([]string, error) {
	if err := g.calls.wait(); err != nil {
		return nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.list = slices.Clone(hs)
	return slices.Clone(hs), nil
}

func (g *lockedHosts) EditPrivateHosts(_ context.Context, _ id.Project, edit func([]string) []string) ([]string, error) {
	if err := g.calls.wait(); err != nil {
		return nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.list = slices.Clone(edit(slices.Clone(g.list)))
	return slices.Clone(g.list), nil
}

func TestACardsHostDuringAPageChangeIsKept(t *testing.T) {
	g, l := &lockedHosts{list: []string{"nas.local"}}, newLoop()
	h := NewHosts(g, l.post, func(err error) { t.Error(err) })
	h.ByProject["p1"] = []string{"nas.local"}

	done := make(chan error, 1)
	h.Change("p1", func(hs []string) []string { return WithHost(hs, "printer.local") }, func(err error) { done <- err })
	g.calls.came(t, l, 1)
	g.add("card.local") // the card's host, while the change is on its way
	g.calls.answer(0, nil)
	answered := 1
	until(t, l, func() bool { // and any call after it
		g.calls.mu.Lock()
		n := len(g.calls.calls)
		g.calls.mu.Unlock()
		for ; answered < n; answered++ {
			g.calls.answer(answered, nil)
		}
		return len(done) == 1
	})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	want := []string{"nas.local", "card.local", "printer.local"}
	same(t, g.get(), want)
	same(t, h.ByProject["p1"], want)
}
