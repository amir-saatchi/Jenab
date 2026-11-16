package state

// Ported from frontend/src/state/nav.test.ts.

import (
	"reflect"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
)

func chatAt(c string) Place { return ChatPlace("p1", id.Chat(c)) }

func places(ps ...Place) []Place { return ps }

var settingsPlace = Place{View: ViewSettings}

func TestPushAndStep(t *testing.T) {
	h := Push(NoHistory(), chatAt("a"))
	h = Push(h, chatAt("a")) // the same place again is no step
	h = Push(h, chatAt("b"))
	if len(h.Places) != 2 {
		t.Fatalf("places %v", h.Places)
	}
	h = Step(h, -1)
	if *h.At() != chatAt("a") {
		t.Fatalf("at %v", h.At())
	}
	if again := Step(h, -1); !reflect.DeepEqual(again, h) { // nothing further back
		t.Fatalf("stepped to %v", again)
	}
	h = Push(h, settingsPlace) // drops the forward places
	if !reflect.DeepEqual(h.Places, places(chatAt("a"), settingsPlace)) {
		t.Fatalf("places %v", h.Places)
	}
	a, b := Place{View: ViewProjectSettings, Project: "p1"}, Place{View: ViewProjectSettings, Project: "p2"}
	if SamePlace(&a, &b) {
		t.Fatal("two projects' settings are the same place")
	}
}

func TestUsed(t *testing.T) {
	m := Used(Used(Used(nil, ChatRef{"p1", "a"}), ChatRef{"p1", "b"}), ChatRef{"p1", "a"})
	if !reflect.DeepEqual(m, []ChatRef{{"p1", "a"}, {"p1", "b"}}) {
		t.Fatalf("mru %v", m)
	}
}

func mruChats(n *Nav) (out []string) {
	for _, m := range n.MRU {
		out = append(out, string(m.Chat))
	}
	return out
}

func wantAt(t *testing.T, n *Nav, p Place) {
	t.Helper()
	if c := n.Current(); c == nil || *c != p {
		t.Fatalf("at %v, want %v", c, p)
	}
}

func TestCtrlTabWalksBackThroughTheChatsUsedLast(t *testing.T) {
	n := NewNav()
	for _, c := range []string{"a", "b", "c"} {
		n.Go(chatAt(c))
	}
	n.NextChat(1) // Ctrl+Tab
	wantAt(t, n, chatAt("b"))
	n.NextChat(1) // Tab again while holding Ctrl
	wantAt(t, n, chatAt("a"))
	n.EndCycle() // Ctrl let go
	if got := mruChats(n); !reflect.DeepEqual(got, []string{"a", "c", "b"}) {
		t.Fatalf("mru %v", got)
	}
	// One cycle is one step in the history, whatever it passed on the way.
	if !reflect.DeepEqual(n.History.Places, places(chatAt("a"), chatAt("b"), chatAt("c"), chatAt("a"))) {
		t.Fatalf("history %v", n.History.Places)
	}
	n.Back()
	wantAt(t, n, chatAt("c"))
	n.Forward()
	n.NextChat(1)
	wantAt(t, n, chatAt("c"))
	n.EndCycle()
	n.NextChat(-1) // Ctrl+Shift+Tab goes the other way round
	wantAt(t, n, chatAt("b"))
}

func TestBackAndForwardCountAsUse(t *testing.T) {
	n := NewNav()
	n.Go(chatAt("a"))
	n.Go(chatAt("b"))
	n.Go(settingsPlace)
	n.Back()
	n.Back()
	wantAt(t, n, chatAt("a"))
	if n.MRU[0].Chat != "a" {
		t.Fatalf("mru %v", n.MRU)
	}
	n.Forward()
	wantAt(t, n, chatAt("b"))
}

func TestPlacesOfTheirOwn(t *testing.T) {
	at := func(turn int) *Place { return &Place{View: ViewInspector, Project: "p", Chat: "c", Turn: turn} }
	if !SamePlace(at(1), at(1)) || SamePlace(at(1), at(2)) {
		t.Fatal("a turn in the inspector is not a place of its own")
	}
	if SamePlace(&Place{View: ViewSettings}, &Place{View: ViewSettings, Section: "Models"}) {
		t.Fatal("a section of Settings is not a place of its own")
	}
	if !SamePlace(&Place{View: ViewRuntime}, &Place{View: ViewRuntime}) {
		t.Fatal("runtime differs from itself")
	}
}

func TestCtrlTabAwayFromTheChatUsedLastOpensThatChatFirst(t *testing.T) {
	n := NewNav()
	for _, c := range []string{"a", "b"} {
		n.Go(chatAt(c))
	}
	n.Go(settingsPlace)
	n.NextChat(1)
	wantAt(t, n, chatAt("b"))
	n.NextChat(1)
	wantAt(t, n, chatAt("a"))
	n.EndCycle()
	if !reflect.DeepEqual(n.History.Places, places(chatAt("a"), chatAt("b"), settingsPlace, chatAt("a"))) {
		t.Fatalf("history %v", n.History.Places)
	}
	n.Go(Place{View: ViewRuntime})
	n.NextChat(-1) // the other way round starts at the far end
	wantAt(t, n, chatAt("b"))
	n.EndCycle()
}

func TestCtrlTabRoundToWhereItStartedAddsNoStep(t *testing.T) {
	n := NewNav()
	for _, c := range []string{"a", "b"} {
		n.Go(chatAt(c))
	}
	n.NextChat(1)
	n.NextChat(1)
	wantAt(t, n, chatAt("b"))
	n.EndCycle()
	if !reflect.DeepEqual(n.History.Places, places(chatAt("a"), chatAt("b"))) {
		t.Fatalf("history %v", n.History.Places)
	}
}

func TestOneChatCtrlTab(t *testing.T) {
	n := NewNav()
	n.Go(chatAt("a"))
	n.NextChat(1)
	if n.Cycle != -1 {
		t.Fatal("cycled on the only chat")
	}
	n.Go(settingsPlace)
	n.NextChat(1)
	wantAt(t, n, chatAt("a"))
	n.EndCycle()
}
