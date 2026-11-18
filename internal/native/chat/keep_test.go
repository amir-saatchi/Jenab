package chat_test

// TASK-081: a chat made and left while the chat list was being read keeps
// what its screen kept.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/native/state/statetest"
)

// heldList holds each chat list read while on: it tells came it started,
// then answers with the list the test sends to answer, as read before.
type heldList struct {
	state.ChatAPI
	on     atomic.Bool
	came   chan struct{}
	answer chan []app.ChatItem
}

func (h *heldList) List(ctx context.Context, p id.Project) ([]app.ChatItem, error) {
	if !h.on.Load() {
		return h.ChatAPI.List(ctx, p)
	}
	h.came <- struct{}{}
	return <-h.answer, nil
}

func TestAChatMadeSinceTheListWasReadKeepsItsState(t *testing.T) {
	f := statetest.New()
	api := f.API()
	held := &heldList{ChatAPI: api.Chats, came: make(chan struct{}, 1), answer: make(chan []app.ChatItem, 1)}
	api.Actions, _ = api.Chats.(state.ActionsAPI)
	api.ChatExport, _ = api.Chats.(state.ChatExportAPI)
	api.Chats = held
	x := newFixture(t, f, api,
		statetest.Text(1, jc.RoleUser, "Search the site"),
		answer(1, notice(jc.Notice{Kind: jc.NoticeDesktopSaved, Text: "Macro saved: site_search (2 steps)",
			Target: "desktop:macro:3", Change: "12", Details: "1. click Search"})),
	)
	other := f.AddChat(x.p, "Other")
	listed := func(chats ...id.Chat) []app.ChatItem {
		var items []app.ChatItem
		for _, c := range chats {
			items = append(items, app.ChatItem{Chat: jc.Chat{ID: c}})
		}
		return items
	}
	read := func() <-chan error {
		loaded := x.st.Chats.Load(x.p)
		select {
		case <-held.came:
		case <-time.After(5 * time.Second):
			t.Fatal("the list was not read")
		}
		return loaded
	}

	// The list is read before x.ch is made: it has only other.
	held.on.Store(true)
	loaded := read()
	x.show(t, false) // x.ch made and opened
	click(t, x, "View steps")
	click(t, x, "Undo")
	x.until(t, "the undo", func() bool { return x.f.IsUndone("desktop:12") })
	var ms []id.Message
	for _, m := range x.st.Threads.Threads[x.ch].Messages {
		ms = append(ms, m.ID)
	}
	before := x.scr.Keeps(x.ch, ms)
	if before < 4 {
		t.Fatalf("the chat keeps %d things before", before)
	}
	x.st.OpenChat(x.p, other) // and left
	x.until(t, "the other chat", func() bool { return x.st.Threads.Threads[other] != nil })
	held.answer <- listed(other)
	x.until(t, "the list", func() bool { return len(loaded) == 1 })
	if got := x.scr.Keeps(x.ch, ms); got != before {
		t.Errorf("the chat made since the list was read keeps %d things, was %d", got, before)
	}

	// A list read after x.ch was deleted drops what it keeps.
	loaded = read()
	held.answer <- listed(other)
	x.until(t, "the next list", func() bool { return len(loaded) == 1 })
	if got := x.scr.Keeps(x.ch, ms); got != 0 {
		t.Errorf("the deleted chat still keeps %d things", got)
	}
}
