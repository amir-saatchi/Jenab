package logic

// Ported from the React app's chat/undo-text.test.ts.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

var i = Isolate

func conflict(c store.UndoConflict, chatTitle string) agent.TurnConflict {
	if c.Item == (store.UndoItem{}) {
		c.Item = store.UndoItem{Table: "prices", Key: "[2]"}
	}
	if c.Reason == "" {
		c.Reason = store.ConflictChanged
	}
	return agent.TurnConflict{Conflict: c, ChatTitle: chatTitle}
}

func TestItemName(t *testing.T) {
	cases := []struct {
		it   store.UndoItem
		want string
	}{
		{store.UndoItem{Table: "prices", Key: "[2]"}, "row " + i("2") + " of " + i("prices")},
		{store.UndoItem{Table: "pairs", Key: `[1,"BTC"]`}, "row " + i("1") + ", " + i("BTC") + " of " + i("pairs")},
		{store.UndoItem{Table: "_jenab_objects", Key: "object:reports/a.txt"}, "the file " + i("reports/a.txt")},
		{store.UndoItem{Table: "_jenab_chat_role", Key: "01CHAT"}, "the chat's role"},
		{store.UndoItem{Table: "prices", Key: ""}, "the table " + i("prices")},
		{store.UndoItem{Table: "_jenab_memory", Key: "memory:project/goals"}, "the project memory section " + i("goals")},
		{store.UndoItem{Table: "_jenab_views", Key: "view:btc_prices"}, "the view " + i("btc_prices")},
		{store.UndoItem{Table: "_jenab_pages", Key: "page:bitcoin"}, "the page " + i("bitcoin")},
		{store.UndoItem{Table: "_jenab_user_memory", Key: "ترجیحات"}, "the user memory section " + i("ترجیحات")},
	}
	for _, c := range cases {
		if got := ItemName(c.it); got != c.want {
			t.Errorf("ItemName(%v) = %q, want %q", c.it, got, c.want)
		}
	}
}

func TestConflictTextNamesWhoChangedMemory(t *testing.T) {
	user := store.UndoItem{Table: "_jenab_user_memory", Key: "preferences"}
	cases := []struct {
		src  id.Source
		want string
	}{
		{"message:01J", "the user memory section " + i("preferences") + " changed since by a chat in another project"},
		{"undo:user_memory:4", "the user memory section " + i("preferences") + " changed since by an undo"},
	}
	for _, c := range cases {
		if got := ConflictText(conflict(store.UndoConflict{Item: user, Source: c.src}, "")); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}

// A Persian table name and key stay whole, each in its own isolate, so
// the English sentence around them keeps its order.
func TestItemNameIsolatesRightToLeftNames(t *testing.T) {
	got := ItemName(store.UndoItem{Table: "قیمت‌ها", Key: `["تهران",2]`})
	want := strings.NewReplacer("<", fsi, ">", pdi).Replace("row <تهران>, <2> of <قیمت‌ها>")
	if want != "row ⁨تهران⁩, ⁨2⁩ of ⁨قیمت‌ها⁩" || got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConflictText(t *testing.T) {
	row := "row " + i("2") + " of " + i("prices")
	order := "row " + i("7") + " of " + i("orders")
	reached := &store.UndoItem{Table: "orders", Key: "[7]"}
	cases := []struct {
		c    agent.TurnConflict
		want string
	}{
		{conflict(store.UndoConflict{Source: "message:01M"}, "Other"), row + " changed since in the chat “" + i("Other") + "”"},
		{conflict(store.UndoConflict{Source: "user"}, ""), row + " changed since by you"},
		{conflict(store.UndoConflict{Reason: store.ConflictReferenced, Reached: reached}, ""), order + " refers to " + row},
		{conflict(store.UndoConflict{Reason: store.ConflictCascade, Reached: reached}, ""), "undoing " + row + " would also change " + order},
		{conflict(store.UndoConflict{Reason: store.ConflictSchema, Item: store.UndoItem{Table: "prices", Key: ""}}, ""), "a later migration changed the table " + i("prices")},
	}
	for _, c := range cases {
		if got := ConflictText(c.c); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}

func TestSkipAllAddsEachItemOnce(t *testing.T) {
	cs := []agent.TurnConflict{
		conflict(store.UndoConflict{}, ""),
		conflict(store.UndoConflict{}, ""),
		conflict(store.UndoConflict{Item: store.UndoItem{Table: "prices", Key: "[3]"}}, ""),
	}
	got := SkipAll([]store.UndoItem{{Table: "prices", Key: "[3]"}}, cs)
	want := []store.UndoItem{{Table: "prices", Key: "[3]"}, {Table: "prices", Key: "[2]"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestPartlyFollowsTheSideTheLastUndoOrRedoLeft(t *testing.T) {
	plan := func(changes int, rows []store.TableRows, left int) agent.TurnPlan {
		return agent.TurnPlan{Changes: changes, Rows: rows, Left: left}
	}
	state := func(undo, redo agent.TurnPlan) *agent.TurnUndoState {
		return &agent.TurnUndoState{Turn: 1, Undo: undo, Redo: redo}
	}
	two := []store.TableRows{{Table: "prices", Rows: 2}}
	one := []store.TableRows{{Table: "prices", Rows: 1}}
	rests := func(p *Partly) [2]bool { return [2]bool{IsRest(p, false), IsRest(p, true)} }

	undone := PartlyOf(state(plan(1, two, 2), plan(2, one, 0)))
	if want := (Partly{"partly undone: 2 items not undone", RestUndo}); undone == nil || *undone != want {
		t.Errorf("undone = %v", undone)
	}
	if got := rests(undone); got != [2]bool{true, false} {
		t.Errorf("undone rests = %v", got)
	}
	redone := PartlyOf(state(plan(2, two, 0), plan(1, one, 1)))
	if want := (Partly{"partly redone: 1 item not redone", RestRedo}); redone == nil || *redone != want {
		t.Errorf("redone = %v", redone)
	}
	if got := rests(redone); got != [2]bool{false, true} {
		t.Errorf("redone rests = %v", got)
	}
	// No side known: both ways are the rest.
	crash := PartlyOf(state(plan(1, one, 0), plan(1, one, 0)))
	if want := (Partly{"partly undone", RestBoth}); crash == nil || *crash != want {
		t.Errorf("crash = %v", crash)
	}
	if got := rests(crash); got != [2]bool{true, true} {
		t.Errorf("crash rests = %v", got)
	}
	// Past the undo window the turn is history: no partly state.
	past, pastRedo := plan(1, one, 0), plan(1, one, 1)
	past.Past, pastRedo.Past = true, true
	for _, st := range []*agent.TurnUndoState{
		state(past, pastRedo),
		state(plan(1, two, 0), plan(0, nil, 0)),
		state(plan(0, nil, 0), plan(1, two, 0)),
		nil,
	} {
		if got := PartlyOf(st); got != nil {
			t.Errorf("PartlyOf(%v) = %v, want nil", st, got)
		}
	}
	if IsRest(nil, false) {
		t.Error("IsRest(nil, false) = true")
	}
}
