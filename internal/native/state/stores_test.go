package state

// The event handling of the React app's state/events.ts and the opening of
// shell/open.ts, which have no tests in the React app.

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
)

type fakeServices struct {
	fakeChats
	lists    atomic.Int32
	mu       sync.Mutex
	notified []app.Notification
	items    []app.ChatItem
}

func (f *fakeServices) List(context.Context, id.Project) ([]app.ChatItem, error) {
	f.lists.Add(1)
	return f.items, nil
}

func (f *fakeServices) Waiting(context.Context) ([]app.WaitingItem, error) { return nil, nil }

func (f *fakeServices) Create(context.Context, id.Project, string, string) (chat.Chat, error) {
	return chat.Chat{ID: "new"}, nil
}

func (f *fakeServices) Notify(_ context.Context, n app.Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notified = append(f.notified, n)
	return nil
}

func (f *fakeServices) notifications() []app.Notification {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]app.Notification(nil), f.notified...)
}

type fakeProjects struct{ opens atomic.Int32 }

func (f *fakeProjects) List(context.Context) ([]app.ProjectItem, error) { return nil, nil }

func (f *fakeProjects) FolderWarning(context.Context) (*project.FolderWarning, error) {
	return nil, nil
}

func (f *fakeProjects) Create(_ context.Context, name string) (app.OpenedProject, error) {
	return app.OpenedProject{ID: "p9", Name: name}, nil
}

func (f *fakeProjects) Open(_ context.Context, p id.Project) (app.OpenedProject, error) {
	f.opens.Add(1)
	return app.OpenedProject{ID: p, Name: "Coins", Mother: "mother"}, nil
}

func newStores(t *testing.T) (*Stores, *fakeServices, *fakeProjects, *loop) {
	f, fp, l := &fakeServices{}, &fakeProjects{}, newLoop()
	s := NewStores(API{Projects: fp, Chats: f, Settings: &fakeSettings{}, System: f}, l.post)
	t.Cleanup(s.Close)
	old := reloadWait
	reloadWait = 5 * time.Millisecond
	t.Cleanup(func() { reloadWait = old })
	return s, f, fp, l
}

func TestAChatThatStartsToWaitNotifiesUnlessShown(t *testing.T) {
	s, f, _, l := newStores(t)
	s.Chats.ByProject["p1"] = []app.ChatItem{item("a", func(c *chat.Chat) { c.Title = "Prices" }, "")}
	s.Projects.Opened["p1"] = app.OpenedProject{ID: "p1", Name: "Coins"}
	q := wait
	s.Apply(app.EventStatus, waitStatus("a", chat.StateWaiting, &q))
	until(t, l, func() bool { return len(f.notifications()) == 1 })
	same(t, f.notifications()[0], app.Notification{Project: "p1", Chat: "a", Title: "Prices is waiting for your answer", Body: "Coins · Which currency?"})
	// Still waiting, the next card: no second notification.
	s.Apply(app.EventStatus, waitStatus("a", chat.StateWaiting, &q))
	// Shown in an active window: none either.
	s.Apply(app.EventStatus, waitStatus("a", chat.StateWorking, nil))
	s.Nav.Go(ChatPlace("p1", "a"))
	s.WindowActive = func() bool { return true }
	s.Apply(app.EventStatus, waitStatus("a", chat.StateWaiting, &q))
	l.drain()
	same(t, len(f.notifications()), 1)
	same(t, len(s.Chats.Waiting), 1)
}

func TestABurstOfStatusesReadsTheListOnce(t *testing.T) {
	s, f, _, l := newStores(t)
	f.items = []app.ChatItem{item("a", nil, ""), item("b", nil, "")}
	s.Chats.ByProject["p1"] = f.items[:1]
	for range 5 {
		s.Apply(app.EventStatus, waitStatus("b", chat.StateWorking, nil)) // b is not listed yet
	}
	until(t, l, func() bool { return len(s.Chats.ByProject["p1"]) == 2 })
	time.Sleep(20 * time.Millisecond)
	l.drain()
	same(t, f.lists.Load(), int32(1))
}

func TestAppOpenOpensTheChat(t *testing.T) {
	s, f, fp, l := newStores(t)
	f.push(func() (app.ChatSnapshot, error) { return storeSnap("a", 1, agent.Live{}), nil })
	s.Apply(app.EventOpen, app.Open{Project: "p1", Chat: "a"})
	until(t, l, func() bool { return s.Threads.Threads["a"] != nil })
	wantAt(t, s.Nav, ChatPlace("p1", "a"))
	_, listed := s.Chats.ByProject["p1"]
	until(t, l, func() bool { _, listed = s.Chats.ByProject["p1"]; return listed })
	// Opened once a session.
	f.push(func() (app.ChatSnapshot, error) { return storeSnap("mother", 1, agent.Live{}), nil })
	s.Nav.Go(Place{View: ViewSettings})
	s.OpenProject("p1", "")
	l.drain()
	wantAt(t, s.Nav, ChatPlace("p1", "a")) // the chat used last in it
	same(t, fp.opens.Load(), int32(1))
}

func TestOpeningStaysPutWhenThePlaceChanged(t *testing.T) {
	s, _, _, l := newStores(t)
	s.OpenProject("p1", "a")
	s.Nav.Go(Place{View: ViewSettings}) // the user moved on while it opened
	until(t, l, func() bool { _, ok := s.Projects.Opened["p1"]; return ok })
	wantAt(t, s.Nav, Place{View: ViewSettings})
}

func TestNoticesBecomeToasts(t *testing.T) {
	s, _, _, _ := newStores(t)
	s.Apply(app.EventNotice, project.Notice{Project: "p1", Kind: project.NoticeDamaged, Text: "damaged"})
	s.Apply(app.EventNotice, project.Notice{Project: "p1", Kind: project.NoticeRecovered, Text: "recovered"})
	same(t, s.TakeToasts(), []Toast{{Text: "damaged", Warning: true, Sticky: true}, {Text: "recovered"}})
	same(t, len(s.TakeToasts()), 0)
}
