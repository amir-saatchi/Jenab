// Package chat is the chat screen on MyGo (SPIKE-048 Part B): the thread
// of one chat and its composer, as the React app's chat/thread-view,
// parts, cards, rewind, undo and composer. It reads the stores and acts
// through state.Actions; what it shows is built with the kit. The pure
// rules it shares with the React app are in package logic.
package chat

import (
	"maps"
	"slices"
	"strings"
	"time"

	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/chat/logic"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Screen shows the open chat. It keeps what each chat's view needs
// between frames: the list's place, what is unfolded, the cards' input,
// the composer's files.
type Screen struct {
	st *state.Stores
	// Now is the clock; nil is time.Now.
	Now func() time.Time
	// OnLink opens a link in an answer; nil does what MyGo does.
	OnLink func(url string)

	md     kit.Markdown // the thread's, sharing its selection
	hello  kit.Markdown // the Mother chat's greeting, outside the thread
	views  map[id.Chat]*view
	open   map[string]*bool
	cards  map[string]*card
	undos  map[string]*undo
	chips  map[id.Chat]map[string]*chip
	ask    *rewindAsk
	dialog bool // the rewind dialog

	refreshes map[id.Chat]*refresh
	long      *refreshAsk // a long refresh asking first
	longOpen  bool        // its dialog
	connect   *connecting // the Connect card's form
}

// New makes the chat screen on the stores.
func New(st *state.Stores) *Screen {
	s := &Screen{st: st, md: kit.Markdown{ShareSelection: true},
		views: map[id.Chat]*view{}, open: map[string]*bool{}, cards: map[string]*card{}, undos: map[string]*undo{},
		chips: map[id.Chat]map[string]*chip{}, refreshes: map[id.Chat]*refresh{}}
	s.md.Own, s.md.CodeLink = s.own, state.CodeLink
	st.OnPrune = append(st.OnPrune, s.prune)
	return s
}

// prune drops what the screen keeps for project p's chats that were
// deleted: those its fresh list doesn't have, but the chat shown. What
// it keeps by key, a card's input or what is unfolded, is under a key
// with the chat's ID or one of its messages' IDs between its colons.
func (s *Screen) prune(p id.Project, listed []id.Chat, shown id.Chat) {
	gone := map[string]bool{}
	for ch, v := range s.views {
		if v.thread == nil || v.thread.Project != p || ch == shown || slices.Contains(listed, ch) {
			continue
		}
		gone[string(ch)] = true
		for _, m := range v.msgs {
			gone[string(m.ID)] = true
		}
		delete(s.views, ch)
		delete(s.chips, ch)
		delete(s.refreshes, ch)
	}
	if len(gone) == 0 {
		return
	}
	hit := func(key string) bool {
		return slices.ContainsFunc(strings.Split(key, ":"), func(seg string) bool { return gone[seg] })
	}
	maps.DeleteFunc(s.open, func(k string, _ *bool) bool { return hit(k) })
	maps.DeleteFunc(s.cards, func(k string, _ *card) bool { return hit(k) })
	maps.DeleteFunc(s.undos, func(k string, _ *undo) bool { return hit(k) })
	if s.connect != nil && gone[string(s.connect.chat)] {
		s.connect.form.Clear()
		s.connect = nil
	}
	if s.long != nil && gone[string(s.long.chat)] {
		s.long, s.longOpen = nil, false
	}
}

func (s *Screen) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// view is one chat's view: its list, and what it derived from the thread
// it showed last, made again only when the thread changes.
type view struct {
	list    kit.ListState
	thread  *state.Thread
	msgs    []jc.Message
	rows    []logic.Row
	starts  map[id.Message]int
	marks   []logic.Mark
	notice  *logic.Notice
	undo    id.Message // the newest Undo turn or Redo turn, for the chips
	results map[string]jc.ToolResult
	running bool // as the last frame saw it, for "Answer finished"
	older   bool // earlier turns are loading
	ticking bool // a redraw is due in a second, for the retry's countdown
	composer
	items []item
}

// item is one entry of the thread's list.
type item struct {
	key  string
	kind itemKind
	row  int // rows[row], for itemRow
	mark logic.Mark
}

type itemKind int

const (
	itemOlder itemKind = iota
	itemEmpty
	itemRow
	itemMark
	itemStream // the answer streaming with no agent row to hold it
	itemWorking
)

func (s *Screen) viewOf(t *state.Thread) *view {
	v := s.views[t.Chat.ID]
	if v == nil {
		v = &view{}
		v.list.FollowEnd = true
		v.list.FollowAnimation = kit.Glide
		v.list.Key = func(i int) any {
			if i < len(v.items) {
				return v.items[i].key
			}
			return i
		}
		s.views[t.Chat.ID] = v
	}
	if v.thread != t {
		v.thread = t
		v.msgs = make([]jc.Message, len(t.Messages))
		for i, m := range t.Messages {
			v.msgs[i] = *m
		}
		v.rows = logic.Rows(v.msgs)
		v.starts = logic.TurnStarts(v.msgs)
		v.marks = logic.Marks(t.Rewinds)
		v.notice = logic.LastNotice(v.msgs)
		v.undo = logic.LastUndo(v.msgs)
		v.results = logic.Results(v.msgs)
	}
	return v
}

// View shows the chat t: the thread, the bars above the composer, and the
// composer. mother is a Mother chat; readOnly a project open read-only.
func (s *Screen) View(c *kit.Context, t *state.Thread, mother, readOnly bool) {
	v := s.viewOf(t)
	running := t.State != jc.StateIdle
	if v.running && !running {
		kit.Announce(c, "Answer finished")
	}
	v.running = running
	s.keepConnect(t.Chat.ID, v.notice != nil && v.notice.Kind == jc.NoticeTurnFailed && !running && s.noProvider())
	// A window card holds still: the chat stops following new content
	// until it is answered (SPEC 8.10).
	v.list.FollowEnd = !logic.PickWaiting(v.msgs, t.Waiting)
	s.items(v, t)
	kit.Column(c).Grow(1).Shrink(1).MinHeight(0).AlignItems(kit.Stretch).Children(func() {
		kit.Selectable(kit.List(c, &v.list, len(v.items), func(i int) { s.item(c, v, t, v.items[i], mother) }).
			Grow(1).Label("Messages"))
		kit.Column(c).Width(768).MaxWidthPercent(100).AlignSelf(kit.Center).PaddingX(24).Gap(8).Children(func() {
			s.bars(c, v, t)
		})
		s.composerView(c, v, t, readOnly)
	})
	s.rewindDialog(c, t)
	s.refreshDialog(c, t)
}

// items lays the thread out as thread-view.tsx does: earlier turns, the
// rows with each rewind's mark after its turn, the answer streaming, the
// messages that join the turn, and a spinner while the turn starts.
func (s *Screen) items(v *view, t *state.Thread) {
	running := t.State != jc.StateIdle
	streaming := t.Stream != "" && !slices.ContainsFunc(v.msgs, func(m jc.Message) bool { return m.ID == t.Stream })
	joining := make([]string, len(t.Joining))
	for i, m := range t.Joining {
		joining[i] = string(m)
	}
	shown, joined := logic.SplitJoining(v.rows, joining)
	v.items = v.items[:0]
	add := func(it item) { v.items = append(v.items, it) }
	if t.From > 1 {
		add(item{key: "older", kind: itemOlder})
	}
	markItem := func(m logic.Mark) item { return item{key: "rewind:" + itoa(int(m.Rewind.ID)), kind: itemMark, mark: m} }
	if t.From <= 1 {
		for _, m := range v.marks {
			if m.At == 0 || len(shown) == 0 {
				add(markItem(m))
			}
		}
	}
	if len(v.rows) == 0 && !streaming {
		add(item{key: "empty", kind: itemEmpty})
	}
	for i := range shown {
		add(item{key: shown[i].ID, kind: itemRow, row: i})
		turn := rowTurn(shown[i])
		next := int(^uint(0) >> 1)
		if i+1 < len(shown) {
			next = rowTurn(shown[i+1])
		}
		for _, m := range v.marks {
			if m.At >= turn && m.At < next && !(t.From <= 1 && (m.At == 0 || len(shown) == 0)) {
				add(markItem(m))
			}
		}
	}
	var last *logic.Row
	if len(shown) > 0 {
		last = &shown[len(shown)-1]
	}
	if streaming && (last == nil || last.Kind != logic.RowAgent) {
		add(item{key: string(t.Stream), kind: itemStream})
	}
	for i := range joined {
		add(item{key: joined[i].ID, kind: itemRow, row: len(shown) + i})
	}
	if running && !streaming && (last == nil || last.Kind != logic.RowAgent) && t.Waiting == nil {
		add(item{key: "working", kind: itemWorking})
	}
}

func rowTurn(r logic.Row) int {
	if r.Kind == logic.RowAgent {
		return r.Messages[len(r.Messages)-1].Turn
	}
	return r.Message.Turn
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// item builds one entry of the list, in the thread's column.
func (s *Screen) item(c *kit.Context, v *view, t *state.Thread, it item, mother bool) {
	kit.Column(c).Width(768).MaxWidthPercent(100).AlignSelf(kit.Center).Padding(10, 24).AlignItems(kit.Stretch).Children(func() {
		switch it.kind {
		case itemOlder:
			s.older(c, v, t)
		case itemEmpty:
			s.empty(c, t, mother)
		case itemMark:
			s.rewindMark(c, t, it.mark.Rewind)
		case itemStream:
			kit.Message(c, "Jenab is answering", false, func() { s.stream(c, t) })
		case itemWorking:
			kit.Spinner(c, "Working")
		case itemRow:
			s.row(c, v, t, it.row)
		}
	})
}

// older loads earlier turns, as it comes into view or is clicked.
func (s *Screen) older(c *kit.Context, v *view, t *state.Thread) {
	kit.Row(c).Justify(kit.Center).Children(func() {
		b := kit.SmallButton(c, nil, "Earlier messages", kit.Ghost)
		if v.older {
			kit.Disable(b, true)
		}
		if first, last := v.list.Visible(); (b.Clicked() || (first == 0 && last >= 0)) && !v.older {
			v.older = true
			ch := t.Chat.ID
			done := s.st.Threads.Older(ch)
			go func() {
				err := <-done
				s.post(func() {
					v.older = false
					s.st.Error(err)
				})
			}()
		}
	})
}

// post runs fn on the UI thread, through the stores' actions.
func (s *Screen) post(fn func()) { s.st.Actions.Post(fn) }

func hello(title string) string {
	return "Hi, I'm " + title + ", the home of this project. Tell me what to track, and I'll set up the tables, a pipeline and a page."
}

func (s *Screen) empty(c *kit.Context, t *state.Thread, mother bool) {
	if !mother {
		kit.Muted(c, "Write the first message below.").AlignSelf(kit.Center)
		return
	}
	title := t.Chat.Title
	if title == "" {
		title = "Mother"
	}
	kit.Message(c, "Jenab said", false, func() {
		s.hello.View(c, hello(title))
		kit.Row(c).Gap(8).Wrap().Children(func() {
			for _, st := range logic.Starters {
				if kit.Button(c, st, kit.Outline).Key(st).Clicked() {
					s.st.Actions.Send(t.Project, t.Chat.ID, st, nil, func(_ id.Message, err error) { s.st.Error(err) })
				}
			}
		})
	})
}

// rewindMark marks where a rewind took turns out (rewind.tsx), with Undo
// rewind and the undo of the changes of the turns it took out.
func (s *Screen) rewindMark(c *kit.Context, t *state.Thread, r store.RewindInfo) {
	kit.Column(c).Gap(4).AlignItems(kit.Stretch).Children(func() {
		kit.Marker(c, logic.MarkText(r)+" · "+kit.Clock(c, r.At.Local()), false)
		if t.State != jc.StateIdle {
			return
		}
		kit.Row(c).Gap(4).Justify(kit.Center).Children(func() {
			key := "rewind:" + string(t.Chat.ID) + ":" + itoa(int(r.ID))
			if r.Undoable {
				busy := s.flag(key + ":busy")
				b := kit.SmallButton(c, nil, "Undo rewind", kit.Ghost)
				kit.Disable(b, *busy)
				if b.Clicked() && !*busy {
					*busy = true
					p, ch := t.Project, t.Chat.ID
					s.st.Actions.UndoRewind(p, ch, r.ID, func(err error) {
						*busy = false
						if err != nil {
							s.st.Error(err)
							return
						}
						s.st.Threads.Reload(ch)
					})
				}
			}
			s.undoTurn(c, t, 0, r.ID, logic.TurnsText(r.Turn+1, r.LastTurn))
		})
	})
}

// flag is a bool kept between frames under key.
func (s *Screen) flag(key string) *bool {
	b := s.open[key]
	if b == nil {
		b = new(bool)
		s.open[key] = b
	}
	return b
}
