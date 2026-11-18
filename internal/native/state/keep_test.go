package state

// TASK-081: what Shell and the OnPrune hooks keep for a chat made since
// the chat list was read.

import (
	"slices"
	"testing"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

func TestPruneKeepsAChatMadeSinceTheListWasRead(t *testing.T) {
	g, l := &gatedChats{}, newLoop()
	s, _, _, _ := newStores(t)
	s.Chats = NewChats(g, l.post, s.Threads)
	s.Chats.OnList, s.Chats.Stamp = s.listed, s.Nav.Stamp
	var hooked [][]id.Chat
	s.OnPrune = append(s.OnPrune, func(p id.Project, keep []id.Chat) {
		hooked = append(hooked, slices.Clone(keep))
	})
	c1, c2, c3 := ChatRef{"p1", "c1"}, ChatRef{"p1", "c2"}, ChatRef{"p1", "c3"}
	s.Nav.Go(ChatPlace("p1", "c1"))
	s.Nav.Go(ChatPlace("p1", "c2"))
	s.Shell.Dock(c2, Docked{"file", "old.csv"}) // c2 is deleted below
	loaded := s.Chats.Load("p1")                // reads the list before c3 is made
	g.lists.came(t, l, 1)
	s.Nav.Go(ChatPlace("p1", "c3")) // made
	s.Shell.Dock(c3, Docked{"file", "new.csv"})
	s.Shell.ChatHidden[c3] = true
	s.Nav.Go(ChatPlace("p1", "c1")) // and left
	g.lists.answer(0, []app.ChatItem{item("c1", nil, "")})
	await(t, l, loaded)

	same(t, s.Shell.Open[c3], Docked{"file", "new.csv"})
	if !s.Shell.ChatHidden[c3] {
		t.Error("c3's chat was shown again")
	}
	if _, ok := s.Shell.Open[c2]; ok {
		t.Error("the deleted chat's dock is still kept")
	}
	if len(hooked) != 1 || !slices.Contains(hooked[0], "c3") || slices.Contains(hooked[0], "c2") {
		t.Fatalf("the hooks were told to keep %v", hooked)
	}

	// A list read after c3 was deleted drops it.
	loaded = s.Chats.Load("p1")
	g.lists.came(t, l, 2)
	g.lists.answer(1, []app.ChatItem{item("c1", nil, "")})
	await(t, l, loaded)
	if _, ok := s.Shell.Open[c3]; ok {
		t.Error("the deleted chat's dock is still kept at the next list")
	}
	if _, ok := s.Shell.ChatHidden[c3]; ok {
		t.Error("the deleted chat's ChatHidden is still kept at the next list")
	}
	if len(hooked) != 2 || slices.Contains(hooked[1], "c3") {
		t.Errorf("the hooks were told to keep %v", hooked)
	}
	same(t, s.Nav.MRU, []ChatRef{c1})
}

func TestAChatMadeSinceTheListWasReadKeepsItsDraft(t *testing.T) {
	g, l := &gatedChats{}, newLoop()
	s, _, _, _ := newStores(t)
	s.Chats = NewChats(g, l.post, s.Threads)
	s.Chats.OnList, s.Chats.Stamp = s.listed, s.Nav.Stamp
	draft := func(c id.Chat, text string) {
		s.Threads.Threads[c] = &Thread{Project: "p1", Chat: chat.Chat{ID: c}}
		s.Threads.Drafts[c] = text
	}
	s.Nav.Go(ChatPlace("p1", "c1"))
	s.Nav.Go(ChatPlace("p1", "c2"))
	draft("c2", "deleted below")
	loaded := s.Chats.Load("p1") // reads the list before c3 is made
	g.lists.came(t, l, 1)
	s.Nav.Go(ChatPlace("p1", "c3")) // made
	draft("c3", "half a thought")
	s.Nav.Go(ChatPlace("p1", "c1")) // and left
	g.lists.answer(0, []app.ChatItem{item("c1", nil, "")})
	await(t, l, loaded)

	same(t, s.Threads.Drafts["c3"], "half a thought")
	if s.Threads.Threads["c3"] == nil {
		t.Error("c3's thread was forgotten")
	}
	if _, ok := s.Threads.Drafts["c2"]; ok {
		t.Error("the deleted chat's draft is still kept")
	}

	// A list read after c3 was deleted drops its thread and draft.
	loaded = s.Chats.Load("p1")
	g.lists.came(t, l, 2)
	g.lists.answer(1, []app.ChatItem{item("c1", nil, "")})
	await(t, l, loaded)
	if _, ok := s.Threads.Drafts["c3"]; ok {
		t.Error("the deleted chat's draft is still kept at the next list")
	}
	if s.Threads.Threads["c3"] != nil {
		t.Error("the deleted chat's thread is still kept at the next list")
	}
}
