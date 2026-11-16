package logic

// Ported from the React app's chat/cards.test.ts: what its assertions check
// about text. Which buttons a pick card shows and in what order is
// PickViewOf's (pick_test.go); "Other windows (1)", "Allow", "Show all
// windows" and "overflow-y-auto" are the screen's markup.

import (
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
)

// The window card of desktop control (SPEC 8.10): one button per window
// that can be picked, then Deny; an answered one says what was picked.
func pickCard(answer chat.Grant) chat.Approval {
	a := chat.Approval{
		ID: "a1", Kind: "desktop_window", Target: "desktop",
		Ask: "Pick a window for the agent to use", Why: "to write your sentence", Risk: "The agent can read the window",
		Options: []chat.ApprovalOption{
			{Label: "Notes — notepad", Grant: "pick:w-1"},
			{Label: "Search — chrome", Grant: "pick:w-2"},
			{Label: "Deny", Grant: chat.GrantDeny},
		},
		Answer: answer,
	}
	if answer != "" {
		at := time.Date(2026, 11, 3, 10, 14, 0, 0, time.UTC)
		a.AnsweredAt = &at
	}
	return a
}

func TestAPickCardListsTheWindowsAndDeny(t *testing.T) {
	a := pickCard("")
	if got := CardKind(a); got != "Other app's window" {
		t.Errorf("CardKind = %q", got)
	}
	v := PickViewOf(a.Options, false)
	if v == nil || v.Kind != PickList || len(v.First) != 2 || v.First[0].Label != "Notes — notepad" || v.First[1].Label != "Search — chrome" {
		t.Errorf("view = %+v", v)
	}
}

func TestAnAnsweredPickCardSaysWhichWindow(t *testing.T) {
	a := pickCard("pick:w-2")
	if got := RecordText(a, CardTime(a.AnsweredAt, clock24)); got != "Picked Search — chrome 10:14" {
		t.Errorf("RecordText = %q", got)
	}
}

func TestRecordText(t *testing.T) {
	cases := []struct {
		a    chat.Approval
		at   string
		want string
	}{
		{chat.Approval{Answer: chat.GrantDeny}, "10:14", "Denied 10:14"},
		{chat.Approval{Answer: chat.GrantDeny, Note: "not now"}, "10:14", "Denied 10:14 · “not now”"},
		{chat.Approval{Answer: chat.GrantOnce}, "10:14", "Approved 10:14"},
		{chat.Approval{Answer: chat.GrantOnce, By: "auto"}, "10:14", "Approved by the Auto level 10:14"},
		{chat.Approval{Answer: chat.GrantOnce}, "", "Approved"},
		{chat.Approval{Stopped: true}, "", "Stopped without an answer"},
	}
	for _, c := range cases {
		if got := RecordText(c.a, c.at); got != c.want {
			t.Errorf("RecordText(%+v) = %q, want %q", c.a, got, c.want)
		}
	}
	if got := CardTime(nil, clock24); got != "" {
		t.Errorf("CardTime(nil) = %q", got)
	}
}

func TestCardKind(t *testing.T) {
	cases := []struct {
		a    chat.Approval
		want string
	}{
		{chat.Approval{Kind: "host"}, "New host"},
		{chat.Approval{Kind: "memory", Target: "refresh:c1"}, "Refresh memory"},
		{chat.Approval{Kind: "new_kind"}, "new_kind"},
	}
	for _, c := range cases {
		if got := CardKind(c.a); got != c.want {
			t.Errorf("CardKind(%+v) = %q, want %q", c.a, got, c.want)
		}
	}
}

func TestCardKey(t *testing.T) {
	if got := CardKey("01M", 2); got != "01M:2" {
		t.Errorf("got %q", got)
	}
}

// clock24 writes times as kit.Clock does with a 24-hour clock.
func clock24(t time.Time) string { return t.Format("15:04") }
