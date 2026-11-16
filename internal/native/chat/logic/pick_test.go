package logic

// Ported from the React app's chat/pick.test.ts.

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
)

func opt(n int, group string) chat.ApprovalOption {
	return chat.ApprovalOption{Label: fmt.Sprintf("W%d", n), Grant: chat.Grant(fmt.Sprintf("pick:w-%d", n)), Group: group}
}

var deny = chat.ApprovalOption{Label: "Deny", Grant: chat.GrantDeny}

// grants lists options by grant; nil and empty read the same.
func grants(os []chat.ApprovalOption) []string {
	out := []string{}
	for _, o := range os {
		out = append(out, string(o.Grant)+"/"+o.Group)
	}
	return out
}

func checkList(t *testing.T, v *PickView, first, other []chat.ApprovalOption, folded bool) {
	t.Helper()
	if v == nil || v.Kind != PickList {
		t.Fatalf("view = %+v, want a list", v)
	}
	if !reflect.DeepEqual(grants(v.First), grants(first)) || !reflect.DeepEqual(grants(v.Other), grants(other)) || v.Folded != folded {
		t.Errorf("view = %+v, want first %v, other %v, folded %v", v, grants(first), grants(other), folded)
	}
}

func TestPickViewNoAppNamedIsOnePlainList(t *testing.T) {
	checkList(t, PickViewOf([]chat.ApprovalOption{opt(1, ""), opt(2, ""), deny}, false), []chat.ApprovalOption{opt(1, ""), opt(2, "")}, nil, false)
}

func TestPickViewOneWindowOfTheAppThenShowAll(t *testing.T) {
	options := []chat.ApprovalOption{opt(1, "match"), opt(2, "other"), deny}
	if v := PickViewOf(options, false); v == nil || v.Kind != PickOne || v.Window != opt(1, "match") {
		t.Errorf("view = %+v, want one W1", v)
	}
	// Show all windows opens the full list in the same card, unfolded.
	checkList(t, PickViewOf(options, true), []chat.ApprovalOption{opt(1, "match")}, []chat.ApprovalOption{opt(2, "other")}, false)
}

func TestPickViewSeveralFoldTheRestNoneShowItOpen(t *testing.T) {
	several := PickViewOf([]chat.ApprovalOption{opt(1, "match"), opt(2, "match"), opt(3, "other")}, false)
	if several == nil || several.Kind != PickList || !several.Folded {
		t.Errorf("several = %+v, want a folded list", several)
	}
	none := PickViewOf([]chat.ApprovalOption{opt(1, "other"), opt(2, "other")}, false)
	if none == nil || none.Kind != PickList || len(none.First) != 0 || none.Folded {
		t.Errorf("none = %+v, want an open list with no first", none)
	}
	if v := PickViewOf([]chat.ApprovalOption{deny}, false); v != nil {
		t.Errorf("deny only = %+v, want nil", v)
	}
}

func pickMsg(a chat.Approval) []chat.Message {
	if a.Kind == "" {
		a.Kind = "desktop_window"
	}
	return []chat.Message{{ID: "m", Parts: []chat.Part{{Kind: chat.PartApproval, Approval: &a}}}}
}

func TestPickWaitingOnlyAnOpenWindowCardStopsAutoScroll(t *testing.T) {
	wait := &chat.Waiting{Message: "m", Index: 0}
	cases := []struct {
		ms      []chat.Message
		waiting *chat.Waiting
		want    bool
	}{
		{pickMsg(chat.Approval{}), wait, true},
		{pickMsg(chat.Approval{}), nil, false},
		{pickMsg(chat.Approval{Answer: "pick:w-1"}), wait, false},
		{pickMsg(chat.Approval{Stopped: true}), wait, false},
		{pickMsg(chat.Approval{Kind: "desktop_key"}), wait, false},
	}
	for n, c := range cases {
		if got := PickWaiting(c.ms, c.waiting); got != c.want {
			t.Errorf("case %d: got %v, want %v", n, got, c.want)
		}
	}
}

func TestMoveGuardAClickRightAfterTheCardMovesIsIgnored(t *testing.T) {
	t0 := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	ms := func(n int) time.Time { return t0.Add(time.Duration(n) * time.Millisecond) }
	g := NewMoveGuard(500 * time.Millisecond)
	steady := func(at int, want bool) {
		t.Helper()
		if got := g.Steady(ms(at)); got != want {
			t.Errorf("Steady(%d) = %v, want %v", at, got, want)
		}
	}
	g.Seen(10, 100, ms(0)) // the card appears
	steady(200, false)
	steady(500, true)
	g.Seen(10, 100, ms(900)) // same place: still steady
	steady(900, true)
	g.Seen(10, 40, ms(1000)) // content above pushed it up
	steady(1300, false)
	steady(1500, true)
	g.Moved(ms(2000)) // it resized in place
	steady(2100, false)
}
