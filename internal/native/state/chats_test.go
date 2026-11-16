package state

// Ported from the React app's state/chats.test.ts.

import (
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

func item(cid string, edit func(*chat.Chat), last string) app.ChatItem {
	c := chat.Chat{ID: id.Chat(cid), Kind: chat.KindChat, Title: cid, Skills: []string{}, Model: "default", CreatedBy: "user", CreatedAt: testTime}
	if edit != nil {
		edit(&c)
	}
	it := app.ChatItem{Chat: c, State: chat.StateIdle}
	if last != "" {
		at, err := time.Parse(time.RFC3339, last)
		if err != nil {
			panic(err)
		}
		it.LastActivity = &at
	}
	return it
}

func waitStatus(c string, state chat.State, w *chat.Waiting) chat.Status {
	return chat.Status{Project: "p1", Chat: id.Chat(c), Seq: 1, State: state, Waiting: w}
}

var wait = chat.Waiting{Message: "m1", Index: 0, Kind: chat.PartQuestion, Text: "Which currency?"}

func chatIDs(items []app.ChatItem) (out []string) {
	for _, it := range items {
		out = append(out, string(it.Chat.ID))
	}
	return out
}

func TestVisible(t *testing.T) {
	list := Visible([]app.ChatItem{
		item("old", nil, "2026-10-02T10:00:00Z"),
		item("gone", func(c *chat.Chat) { c.Archived = true }, ""),
		item("new", nil, "2026-10-04T10:00:00Z"),
		item("mother", func(c *chat.Chat) { c.Kind = chat.KindMother }, ""),
	})
	same(t, chatIDs(list), []string{"mother", "new", "old"})
}

// sameSlice tells whether a and b share their array: the function gave
// back what it got.
func sameSlice[T any](a, b []T) bool { return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0]) }

func TestListWithStatus(t *testing.T) {
	items := []app.ChatItem{item("a", nil, ""), item("b", nil, "")}
	next := ListWithStatus(items, waitStatus("b", chat.StateWorking, nil))
	same(t, next[1].State, chat.StateWorking)
	same(t, items[1].State, chat.StateIdle) // the old list is unchanged
	if !sameSlice(ListWithStatus(next, waitStatus("b", chat.StateWorking, nil)), next) {
		t.Fatal("no change made a new list")
	}
	if !sameSlice(ListWithStatus(items, waitStatus("x", chat.StateWorking, nil)), items) {
		t.Fatal("an unknown chat made a new list")
	}
}

func TestWithWaiting(t *testing.T) {
	ws := WithWaiting(nil, waitStatus("a", chat.StateWaiting, &wait), "Prices")
	same(t, ws, []app.WaitingItem{{Project: "p1", Chat: "a", Title: "Prices", Waiting: wait}})
	nextCard := wait
	nextCard.Index = 1
	ws = WithWaiting(ws, waitStatus("a", chat.StateWaiting, &nextCard), "Prices") // the next card
	same(t, len(ws), 1)
	same(t, ws[0].Waiting.Index, 1)
	if !sameSlice(WithWaiting(ws, waitStatus("b", chat.StateIdle, nil), ""), ws) {
		t.Fatal("no change made a new list")
	}
	same(t, len(WithWaiting(ws, waitStatus("a", chat.StateWorking, nil), "Prices")), 0)
}

func TestStatusKeepsTheTitleForWaiting(t *testing.T) {
	s := NewChats(nil, nil, nil)
	s.ByProject["p1"] = []app.ChatItem{item("a", func(c *chat.Chat) { c.Title = "Prices" }, "")}
	s.Status(waitStatus("a", chat.StateWaiting, &wait))
	same(t, s.Waiting[0].Title, "Prices")
	same(t, s.ByProject["p1"][0].State, chat.StateWaiting)
}

func TestInitials(t *testing.T) {
	same(t, Initial("  jenab"), "J")
	same(t, Initial(""), "?")
	same(t, Initial("پروژه"), "پ")
	same(t, Initials("coin prices"), "CP")
	same(t, Initials("coins"), "co")
	same(t, Initials("قیمت سکه"), "قس")
	same(t, Initials(" "), "?")
}
