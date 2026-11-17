package state

// TASK-066: reads that fail, overlap or outlive their chat.

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
)

// gate holds each call until the test answers it, so answers can come in
// any order.
type gate[T any] struct {
	mu    sync.Mutex
	calls []chan T
}

func (g *gate[T]) wait() T {
	ch := make(chan T, 1)
	g.mu.Lock()
	g.calls = append(g.calls, ch)
	g.mu.Unlock()
	return <-ch
}

// came waits until n calls have come.
func (g *gate[T]) came(t *testing.T, l *loop, n int) {
	t.Helper()
	until(t, l, func() bool {
		g.mu.Lock()
		defer g.mu.Unlock()
		return len(g.calls) >= n
	})
}

// answer gives call i, counted from 0 in the order they came, v.
func (g *gate[T]) answer(i int, v T) {
	g.mu.Lock()
	ch := g.calls[i]
	g.mu.Unlock()
	ch <- v
}

// failing is a snapshot answer that fails once the test closes release.
func failing(release <-chan struct{}) func() (app.ChatSnapshot, error) {
	return func() (app.ChatSnapshot, error) {
		<-release
		return app.ChatSnapshot{}, errors.New("down")
	}
}

func TestAFailedReReadStillEndsTheTurn(t *testing.T) {
	shortRetries(t)
	s, f, l := newThreads()
	f.push(func() (app.ChatSnapshot, error) {
		return storeSnap("f1", 2, agent.Live{Running: true, Seq: 2, Streaming: "01A1", Kind: chat.PartText, Text: "Hi"}), nil
	})
	await(t, l, s.Open("p1", "f1"))
	same(t, s.Threads["f1"].State, chat.StateWorking)

	// The read fails, and so do its tries again: the service is down.
	release := make(chan struct{})
	f.push(failing(release))
	reload := s.Reload("f1")
	s.Status(chatStatus("f1", 3, chat.StateIdle, "")) // waits for the read
	close(release)
	if err := await(t, l, reload); err == nil {
		t.Fatal("the reload reported no error")
	}
	until(t, l, func() bool { return len(s.reading) == 0 })
	same(t, s.Threads["f1"].State, chat.StateIdle)
	if s.Stream("f1") != nil {
		t.Fatal("the stream outlived its turn")
	}

	// The thread is stale: the next event reads it.
	f.push(func() (app.ChatSnapshot, error) { return storeSnap("f1", 9, agent.Live{}), nil })
	s.Status(chatStatus("f1", 4, chat.StateIdle, ""))
	until(t, l, func() bool { return s.Threads["f1"].Seq == 9 && len(s.reading) == 0 })
	if s.stale["f1"] {
		t.Fatal("still stale after a good read")
	}
}

func TestAFailedReReadIsTriedAgainWithItsEvents(t *testing.T) {
	shortRetries(t)
	s, f, l := newThreads()
	f.push(func() (app.ChatSnapshot, error) {
		return storeSnap("f2", 2, agent.Live{Running: true, Seq: 2, Streaming: "01A1", Kind: chat.PartText, Text: "Hi"}), nil
	})
	await(t, l, s.Open("p1", "f2"))
	release := make(chan struct{})
	stored := storeSnap("f2", 2, agent.Live{Running: true, Seq: 2, Streaming: "01A1", Kind: chat.PartText, Text: "Hi"})
	ended := storeSnap("f2", 4, agent.Live{})
	f.push(failing(release), func() (app.ChatSnapshot, error) { return stored, nil },
		func() (app.ChatSnapshot, error) { return ended, nil })
	reload := s.Reload("f2")
	s.Status(chatStatus("f2", 3, chat.StateIdle, ""))
	close(release)
	// The second try's snapshot predates the status, which was kept and is
	// applied after it; the turn's end reads the chat once more.
	if err := await(t, l, reload); err != nil {
		t.Fatal(err)
	}
	same(t, s.Threads["f2"].State, chat.StateIdle)
	same(t, s.Threads["f2"].Seq, uint64(4))
}

func TestReloadAnswersAfterTheSnapshotIsApplied(t *testing.T) {
	s, f, l := newThreads()
	f.push(func() (app.ChatSnapshot, error) { return storeSnap("r1", 1, agent.Live{}), nil })
	await(t, l, s.Open("p1", "r1"))
	first, give := later()
	f.push(first, func() (app.ChatSnapshot, error) { return storeSnap("r1", 7, agent.Live{}), nil })
	one := s.Reload("r1")
	two := s.Reload("r1") // a read is running: this one waits for the next
	give <- storeSnap("r1", 5, agent.Live{})
	if err := await(t, l, two); err != nil {
		t.Fatal(err)
	}
	same(t, s.Threads["r1"].Seq, uint64(7))
	if err := await(t, l, one); err != nil {
		t.Fatal(err)
	}
}

func TestAForgottenChatsReadDoesNotBringItBack(t *testing.T) {
	s, f, l := newThreads()
	f.push(func() (app.ChatSnapshot, error) { return storeSnap("d1", 1, agent.Live{}), nil })
	await(t, l, s.Open("p1", "d1"))
	answer, give := later()
	f.push(answer)
	reload := s.Reload("d1")
	s.Forget("d1", true) // deleted while the read runs
	give <- storeSnap("d1", 2, agent.Live{})
	await(t, l, reload)
	if s.Threads["d1"] != nil {
		t.Fatal("the read brought the deleted chat back")
	}

	// A first read too, as the chat list drops a chat being opened.
	answer, give = later()
	f.push(answer)
	opening := s.Open("p1", "d2")
	s.Forget("d2", true)
	give <- storeSnap("d2", 1, agent.Live{})
	await(t, l, opening)
	if s.Threads["d2"] != nil {
		t.Fatal("the first read brought the deleted chat back")
	}
	same(t, len(s.reading), 0)
}

// gatedChats answers List and Waiting when the test says.
type gatedChats struct {
	lists   gate[[]app.ChatItem]
	waiting gate[[]app.WaitingItem]
}

func (g *gatedChats) List(context.Context, id.Project) ([]app.ChatItem, error) {
	return g.lists.wait(), nil
}

func (g *gatedChats) Waiting(context.Context) ([]app.WaitingItem, error) {
	return g.waiting.wait(), nil
}

func (g *gatedChats) Create(context.Context, id.Project, string, string) (chat.Chat, error) {
	return chat.Chat{}, errors.New("not here")
}

func TestOverlappingChatListsKeepTheNewer(t *testing.T) {
	g, l := &gatedChats{}, newLoop()
	s := NewChats(g, l.post, nil)
	var listed [][]id.Chat
	s.OnList = func(_ id.Project, chats []id.Chat, _ int) { listed = append(listed, chats) }
	older := s.Load("p1")
	g.lists.came(t, l, 1)
	newer := s.Load("p1")
	g.lists.came(t, l, 2)
	g.lists.answer(1, []app.ChatItem{item("c1", nil, ""), item("c2", nil, "")})
	await(t, l, newer)
	g.lists.answer(0, []app.ChatItem{item("c1", nil, "")}) // from before c2 was made
	await(t, l, older)
	same(t, chatIDs(s.ByProject["p1"]), []string{"c1", "c2"})
	same(t, listed, [][]id.Chat{{"c1", "c2"}})

	olderW := s.LoadWaiting()
	g.waiting.came(t, l, 1)
	newerW := s.LoadWaiting()
	g.waiting.came(t, l, 2)
	g.waiting.answer(1, []app.WaitingItem{})
	await(t, l, newerW)
	g.waiting.answer(0, []app.WaitingItem{{Project: "p1", Chat: "c1"}})
	await(t, l, olderW)
	same(t, len(s.Waiting), 0)
}

// gatedHosts answers PrivateHosts when the test says.
type gatedHosts struct{ reads gate[[]string] }

func (g *gatedHosts) PrivateHosts(context.Context, id.Project) ([]string, error) {
	return g.reads.wait(), nil
}

func (g *gatedHosts) SetPrivateHosts(context.Context, id.Project, []string) ([]string, error) {
	return nil, errors.New("not here")
}

func TestOverlappingHostReadsKeepTheNewer(t *testing.T) {
	g, l := &gatedHosts{}, newLoop()
	var answered atomic.Int32
	post := func(fn func()) { l.post(func() { fn(); answered.Add(1) }) }
	h := NewHosts(g, post, func(err error) { t.Error(err) })
	h.Load("p1")
	g.reads.came(t, l, 1)
	h.Load("p1")
	g.reads.came(t, l, 2)
	g.reads.answer(1, []string{"nas.local", "127.0.0.1"})
	until(t, l, func() bool { return answered.Load() == 1 })
	g.reads.answer(0, []string{"nas.local"}) // read before the change
	until(t, l, func() bool { return answered.Load() == 2 })
	same(t, h.ByProject["p1"], []string{"nas.local", "127.0.0.1"})
}

// gatedModels answers Models when the test says.
type gatedModels struct {
	*fakeSettings
	models gate[[]app.ModelGroup]
}

func (g *gatedModels) Models(context.Context) ([]app.ModelGroup, error) {
	return g.models.wait(), nil
}

func TestOverlappingModelReadsKeepTheNewer(t *testing.T) {
	g, l := &gatedModels{fakeSettings: &fakeSettings{}}, newLoop()
	s := NewSettings(g, l.post, nil)
	older := s.LoadModels()
	g.models.came(t, l, 1)
	newer := s.LoadModels()
	g.models.came(t, l, 2)
	g.models.answer(1, []app.ModelGroup{{Provider: "after"}})
	await(t, l, newer)
	g.models.answer(0, []app.ModelGroup{{Provider: "before"}})
	await(t, l, older)
	l.drain()
	same(t, s.Models, []app.ModelGroup{{Provider: "after"}})
}

func TestAnEditBeforeTheSettingsLoadIsKept(t *testing.T) {
	f, l := &fakeSettings{}, newLoop()
	f.disk.UI.Theme = "dark"
	s := NewSettings(f, l.post, nil)
	done := s.Edit(func(x *config.Settings) { x.DevTools = true })
	f.release(t, l)
	if err := await(t, l, done); err != nil {
		t.Fatal(err)
	}
	disk := f.onDisk()
	same(t, disk.DevTools, true)
	same(t, disk.UI.Theme, "dark") // saved on top of the settings read
	same(t, s.View.Settings.DevTools, true)
}

func TestStoresCloseTwice(t *testing.T) {
	s, _, _, _ := newStores(t)
	s.Close()
	s.Close() // and the cleanup's
}

func TestADeletedChatLeavesNavAndShell(t *testing.T) {
	s, f, _, l := newStores(t)
	f.items = []app.ChatItem{item("c1", nil, ""), item("c2", nil, "")}
	for _, c := range []id.Chat{"c1", "c2", "c3"} {
		s.Nav.Go(ChatPlace("p1", c))
		s.Shell.Dock(ChatRef{"p1", c}, Docked{Kind: "file", Key: "notes.md"})
		s.Shell.ChatHidden[ChatRef{"p1", c}] = true
	}
	s.Nav.Go(ChatPlace("p2", "x1"))
	s.Shell.Drawer[ChatRef{"p2", "x1"}] = true
	s.Nav.Go(ChatPlace("p1", "c2"))
	// c3 is gone from p1's list; p2's chats are another project's.
	await(t, l, s.Chats.Load("p1"))
	same(t, s.Nav.MRU, []ChatRef{{"p1", "c2"}, {"p2", "x1"}, {"p1", "c1"}})
	if _, ok := s.Shell.Open[ChatRef{"p1", "c3"}]; ok {
		t.Error("the deleted chat's dock is kept")
	}
	if _, ok := s.Shell.ChatHidden[ChatRef{"p1", "c3"}]; ok {
		t.Error("the deleted chat's hidden flag is kept")
	}
	if !s.Shell.Drawer[ChatRef{"p2", "x1"}] || s.Shell.Open[ChatRef{"p1", "c1"}].Key != "notes.md" {
		t.Error("a chat still listed lost its shell state")
	}

	// The chat shown stays, though the list predates it.
	s.Nav.Go(ChatPlace("p1", "new"))
	await(t, l, s.Chats.Load("p1"))
	same(t, s.Nav.MRU[0], ChatRef{"p1", "new"})
}
