package logic

import (
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
)

// The window card of desktop control (SPEC 8.10): the agent names an app
// and the user picks one of its windows.

// IsPick tells whether a grant is a pick option's, which carries what it
// picks: "pick:<value>".
func IsPick(g chat.Grant) bool {
	return strings.HasPrefix(string(g), chat.GrantPick)
}

// PickKind is how a pick card lays out its options.
type PickKind string

const (
	// PickOne is the one window of the app the agent named: Allow, Deny,
	// Show all.
	PickOne PickKind = "one"
	// PickList is a list: the app's windows first; the rest folded when
	// there are some.
	PickList PickKind = "list"
)

// PickView is a pick card's layout. Window is PickOne's; First, Other and
// Folded are PickList's.
type PickView struct {
	Kind   PickKind
	Window chat.ApprovalOption
	First  []chat.ApprovalOption
	Other  []chat.ApprovalOption
	Folded bool
}

func filterOptions(os []chat.ApprovalOption, keep func(chat.ApprovalOption) bool) []chat.ApprovalOption {
	var out []chat.ApprovalOption
	for _, o := range os {
		if keep(o) {
			out = append(out, o)
		}
	}
	return out
}

// PickViewOf lays out a pick card's options; all is "Show all windows".
// Options without a group (the agent named no app) are one plain list.
// It is nil when there is nothing to pick.
func PickViewOf(options []chat.ApprovalOption, all bool) *PickView {
	picks := filterOptions(options, func(o chat.ApprovalOption) bool { return IsPick(o.Grant) })
	if len(picks) == 0 {
		return nil
	}
	if !slices.ContainsFunc(picks, func(o chat.ApprovalOption) bool { return o.Group != "" }) {
		return &PickView{Kind: PickList, First: picks}
	}
	first := filterOptions(picks, func(o chat.ApprovalOption) bool { return o.Group == chat.OptionMatch })
	other := filterOptions(picks, func(o chat.ApprovalOption) bool { return o.Group != chat.OptionMatch })
	if !all && len(first) == 1 {
		return &PickView{Kind: PickOne, Window: first[0]}
	}
	// With no window of the app, the others are all there is: unfolded.
	return &PickView{Kind: PickList, First: first, Other: other, Folded: !all && len(first) > 0}
}

// PickWaiting tells whether the turn waits for a window card. The chat
// then stops following new content, so the card holds still under the
// pointer.
func PickWaiting(messages []chat.Message, waiting *chat.Waiting) bool {
	if waiting == nil {
		return false
	}
	i := slices.IndexFunc(messages, func(m chat.Message) bool { return m.ID == waiting.Message })
	if i < 0 || waiting.Index < 0 || waiting.Index >= len(messages[i].Parts) {
		return false
	}
	a := messages[i].Parts[waiting.Index].Approval
	return a != nil && a.Kind == "desktop_window" && a.Answer == "" && !a.Stopped
}

// MoveGuard tells whether a card held still long enough for a click to be
// meant for what is under the pointer now: it moved when its position
// changed, its size changed, or it appeared. Its zero value has a hold of
// 0; NewMoveGuard gives the React app's 500 ms.
type MoveGuard struct {
	Hold    time.Duration
	seen    bool
	x, y    float64
	moved   bool
	movedAt time.Time
}

// NewMoveGuard is a guard that wants hold of stillness; 0 is 500 ms.
func NewMoveGuard(hold time.Duration) *MoveGuard {
	if hold == 0 {
		hold = 500 * time.Millisecond
	}
	return &MoveGuard{Hold: hold}
}

// Seen records the card's position at now.
func (g *MoveGuard) Seen(x, y float64, now time.Time) {
	if !g.seen || g.x != x || g.y != y {
		g.seen, g.x, g.y = true, x, y
		g.Moved(now)
	}
}

// Moved records a change that keeps the position, such as a resize.
func (g *MoveGuard) Moved(now time.Time) {
	g.moved, g.movedAt = true, now
}

// Steady tells whether the card has held still for Hold at now.
func (g *MoveGuard) Steady(now time.Time) bool {
	return !g.moved || now.Sub(g.movedAt) >= g.Hold
}
