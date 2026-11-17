package state

import (
	"slices"

	"github.com/amir-saatchi/jenab/internal/id"
)

// View is the kind of place the main area shows.
type View string

const (
	ViewChat            View = "chat"
	ViewSettings        View = "settings"
	ViewProjectSettings View = "project-settings"
	ViewInspector       View = "inspector"
	ViewRuntime         View = "runtime"
)

// Place is what the main area shows. There are no tabs (SPEC 5.12): one
// chat, Settings, a project's settings or a developer tool (8.4). Settings
// may open at a section; the turn inspector at a turn. The fields a view
// doesn't use stay empty.
type Place struct {
	View    View
	Project id.Project
	Chat    id.Chat
	Section string
	Turn    int
}

// ChatPlace is the place of a chat.
func ChatPlace(p id.Project, c id.Chat) Place { return Place{View: ViewChat, Project: p, Chat: c} }

// ChatRef names a chat.
type ChatRef struct {
	Project id.Project
	Chat    id.Chat
}

// SamePlace tells whether a and b are the same place; a nil place is
// none.
func SamePlace(a, b *Place) bool {
	if a == nil || b == nil || a.View != b.View {
		return false
	}
	switch a.View {
	case ViewChat:
		return a.Project == b.Project && a.Chat == b.Chat
	case ViewProjectSettings:
		return a.Project == b.Project && a.Section == b.Section
	case ViewSettings:
		return a.Section == b.Section
	case ViewInspector:
		return a.Project == b.Project && a.Chat == b.Chat && a.Turn == b.Turn
	}
	return true
}

const maxHistory = 100

// History is back and forward through the places opened (Alt+Left and
// Alt+Right).
type History struct {
	Places []Place
	Index  int
}

// NoHistory is a history with no places.
func NoHistory() History { return History{Index: -1} }

// At is the place at Index, or nil.
func (h History) At() *Place {
	if h.Index < 0 || h.Index >= len(h.Places) {
		return nil
	}
	return &h.Places[h.Index]
}

// Push opens p after the current place, dropping the places forward of it.
// The same place again is no step.
func Push(h History, p Place) History {
	if SamePlace(h.At(), &p) {
		return h
	}
	places := append(append([]Place(nil), h.Places[:h.Index+1]...), p)
	if len(places) > maxHistory {
		places = places[len(places)-maxHistory:]
	}
	return History{Places: places, Index: len(places) - 1}
}

// Step goes back (-1) or forward (1), or stays at the ends.
func Step(h History, by int) History {
	i := h.Index + by
	if i < 0 || i >= len(h.Places) {
		return h
	}
	h.Index = i
	return h
}

// Used moves the chat to the front of the most-recently-used list that
// Ctrl+Tab walks.
func Used(mru []ChatRef, c ChatRef) []ChatRef {
	out := []ChatRef{c}
	for _, m := range mru {
		if m != c {
			out = append(out, m)
		}
	}
	if len(out) > 50 {
		out = out[:50]
	}
	return out
}

// Nav is the place in the main area, its history, and the chats in the
// order they were used (5.12).
type Nav struct {
	History History
	MRU     []ChatRef
	// Cycle is how far back Ctrl+Tab has gone while Ctrl is held; -1 when
	// not cycling. cycleFrom is the history when the cycle began: the chats
	// passed on the way replace each other, so a cycle adds one place.
	Cycle     int
	cycleFrom *History
	// SidebarHidden hides the left sidebar and puts its chats into the
	// rail, as while a page is open (5.12).
	SidebarHidden bool

	// ticks counts the chats that came into MRU; entered is the tick each
	// one in it came at. Prune keeps a chat that came after the list it
	// gets was read: that list may predate the chat.
	ticks   int
	entered map[ChatRef]int
}

// NewNav is a Nav at no place.
func NewNav() *Nav { return &Nav{History: NoHistory(), Cycle: -1} }

// Current is the place shown, or nil.
func (n *Nav) Current() *Place { return n.History.At() }

func (n *Nav) noCycle() { n.Cycle, n.cycleFrom = -1, nil }

// Go opens a place. Opening a place, back and forward end a Ctrl+Tab cycle.
func (n *Nav) Go(p Place) {
	n.History = Push(n.History, p)
	if p.View == ViewChat {
		n.use(ChatRef{p.Project, p.Chat})
	}
	n.noCycle()
}

// use moves c to the front of MRU, noting when it came in if it is new
// there.
func (n *Nav) use(c ChatRef) {
	if n.entered == nil {
		n.entered = map[ChatRef]int{}
	}
	if _, ok := n.entered[c]; !ok || !slices.Contains(n.MRU, c) {
		n.ticks++
		n.entered[c] = n.ticks
	}
	n.MRU = Used(n.MRU, c)
	if len(n.entered) > len(n.MRU) {
		for r := range n.entered {
			if !slices.Contains(n.MRU, r) {
				delete(n.entered, r)
			}
		}
	}
}

// Stamp is the time, in Nav's ticks, of a chat list's read as it starts:
// give it to Prune with the list.
func (n *Nav) Stamp() int { return n.ticks }

// Back goes back a place; the chat landed on counts as used.
func (n *Nav) Back() { n.moved(Step(n.History, -1)) }

// Forward goes forward a place; the chat landed on counts as used.
func (n *Nav) Forward() { n.moved(Step(n.History, 1)) }

func (n *Nav) moved(h History) {
	n.History = h
	if p := h.At(); p != nil && p.View == ViewChat {
		n.use(ChatRef{p.Project, p.Chat})
	}
	n.noCycle()
}

// NextChat is Ctrl+Tab (by 1) and Ctrl+Shift+Tab (by -1). It starts from
// the chat shown when it is the one used last, and else from before the
// list, so the first Ctrl+Tab opens that chat.
func (n *Nav) NextChat(by int) {
	start := n.Cycle
	if start < 0 {
		start = -1
		if p := n.Current(); len(n.MRU) > 0 && p != nil && p.View == ViewChat && p.Project == n.MRU[0].Project && p.Chat == n.MRU[0].Chat {
			start = 0
		}
	}
	if len(n.MRU) == 0 || (start >= 0 && len(n.MRU) < 2) {
		return
	}
	var at int
	switch {
	case start < 0 && by == 1:
		at = 0
	case start < 0:
		at = len(n.MRU) - 1
	default:
		at = (start + by + len(n.MRU)) % len(n.MRU)
	}
	from := n.History
	if n.cycleFrom != nil {
		from = *n.cycleFrom
	}
	n.Cycle, n.cycleFrom = at, &from
	n.History = Push(from, ChatPlace(n.MRU[at].Project, n.MRU[at].Chat))
}

// Prune drops from the MRU list the chats of project p that its new
// chat list doesn't have. It keeps keep, the chat shown, and the chats
// that came into MRU after the list's read started, at stamp (Stamp):
// the list may predate them. During a Ctrl+Tab cycle it waits for the
// next list, so the cycle's place in the list holds.
func (n *Nav) Prune(p id.Project, listed []id.Chat, keep id.Chat, stamp int) {
	if n.Cycle >= 0 {
		return
	}
	n.MRU = slices.DeleteFunc(n.MRU, func(r ChatRef) bool {
		gone := r.Project == p && r.Chat != keep && !slices.Contains(listed, r.Chat) && n.entered[r] <= stamp
		if gone {
			delete(n.entered, r)
		}
		return gone
	})
}

// EndCycle is Ctrl let go: the chat landed on counts as used.
func (n *Nav) EndCycle() {
	if n.Cycle < 0 {
		return
	}
	n.use(n.MRU[n.Cycle])
	n.noCycle()
}
