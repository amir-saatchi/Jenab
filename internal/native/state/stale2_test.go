package state

// TASK-072: a read made during a save, and Ctrl+Tab over a chat made
// since the list was read.

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
)

// savingHosts answers PrivateHosts and SetPrivateHosts when the test says.
type savingHosts struct {
	reads, saves gate[[]string]
}

func (g *savingHosts) PrivateHosts(context.Context, id.Project) ([]string, error) {
	return g.reads.wait(), nil
}

func (g *savingHosts) SetPrivateHosts(context.Context, id.Project, []string) ([]string, error) {
	return g.saves.wait(), nil
}

func TestAReadDuringASaveDoesNotBringARemovedHostBack(t *testing.T) {
	g, l := &savingHosts{}, newLoop()
	var answered atomic.Int32
	post := func(fn func()) { l.post(func() { fn(); answered.Add(1) }) }
	h := NewHosts(g, post, func(err error) { t.Error(err) })
	h.ByProject["p1"] = []string{"nas.local", "127.0.0.1"}

	saved := make(chan error, 1)
	h.Change("p1", func(hs []string) []string { return WithoutHost(hs, "nas.local") }, func(err error) { saved <- err })
	g.reads.came(t, l, 1)
	g.reads.answer(0, []string{"nas.local", "127.0.0.1"}) // the change's own read
	g.saves.came(t, l, 1)
	h.Load("p1") // the list read again while the save runs
	g.reads.came(t, l, 2)
	g.saves.answer(0, []string{"127.0.0.1"})
	if err := await(t, l, saved); err != nil {
		t.Fatal(err)
	}
	same(t, h.ByProject["p1"], []string{"127.0.0.1"})
	same(t, h.Busy["p1"], false)
	g.reads.answer(1, []string{"nas.local", "127.0.0.1"}) // read before the save landed
	until(t, l, func() bool { return answered.Load() == 2 })
	same(t, h.ByProject["p1"], []string{"127.0.0.1"})

	// A read started after the save shows.
	h.Load("p1")
	g.reads.came(t, l, 3)
	g.reads.answer(2, []string{"127.0.0.1", "printer.local"})
	until(t, l, func() bool { return answered.Load() == 3 })
	same(t, h.ByProject["p1"], []string{"127.0.0.1", "printer.local"})
}

func TestCtrlTabKeepsAChatMadeSinceTheListWasRead(t *testing.T) {
	g, l := &gatedChats{}, newLoop()
	s, _, _, _ := newStores(t)
	s.Chats = NewChats(g, l.post, s.Threads)
	s.Chats.OnList, s.Chats.Stamp = s.listed, s.Nav.Stamp
	s.Nav.Go(ChatPlace("p1", "c1"))
	s.Nav.Go(ChatPlace("p1", "c2"))
	loaded := s.Chats.Load("p1") // reads the list before c3 is made
	g.lists.came(t, l, 1)
	s.Nav.Go(ChatPlace("p1", "c3")) // made and opened
	s.Nav.Go(ChatPlace("p1", "c1")) // and left
	g.lists.answer(0, []app.ChatItem{item("c1", nil, ""), item("c2", nil, "")})
	await(t, l, loaded)
	same(t, s.Nav.MRU, []ChatRef{{"p1", "c1"}, {"p1", "c3"}, {"p1", "c2"}})
	s.Nav.NextChat(1)
	s.Nav.EndCycle()
	same(t, s.Nav.Current().Chat, id.Chat("c3"))

	// A list read after c2 was deleted drops it; one read after c3 was
	// made and before it was deleted would have it.
	loaded = s.Chats.Load("p1")
	g.lists.came(t, l, 2)
	g.lists.answer(1, []app.ChatItem{item("c1", nil, ""), item("c3", nil, "")})
	await(t, l, loaded)
	if slices.Contains(s.Nav.MRU, ChatRef{"p1", "c2"}) {
		t.Errorf("the deleted chat is still in %v", s.Nav.MRU)
	}
}
