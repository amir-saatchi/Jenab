package chat

import (
	"fmt"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/chat/logic"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
	"github.com/amir-saatchi/jenab/internal/store"
)

// card is a card's input between frames.
type card struct {
	busy    bool
	note    string
	all     bool
	step    int
	choice  map[string]*string // single choice by header
	checked map[string]map[string]*bool
	other   map[string]*string
}

func (s *Screen) cardOf(key string) *card {
	k := s.cards[key]
	if k == nil {
		k = &card{choice: map[string]*string{}, checked: map[string]map[string]*bool{}, other: map[string]*string{}}
		s.cards[key] = k
	}
	return k
}

// approvalCard is an approval (SPEC 8.8): what, why, the risk, the
// details and the choice; answered or stopped, its one-line record.
func (s *Screen) approvalCard(c *kit.Context, t *state.Thread, msg id.Message, index int, a jc.Approval, live bool) {
	if a.Answer != "" || a.Stopped || !live {
		icon, tone := kit.IconCheck, kit.Green
		switch {
		case a.Answer == jc.GrantDeny:
			icon, tone = kit.IconX, kit.Red
		case a.Answer == "":
			icon, tone = kit.IconCircleSlash, kit.Neutral
		}
		kit.Record(c, icon, tone, func() {
			kit.Muted(c, a.Ask+" ·")
			kit.Text(c, logic.RecordText(a, logic.CardTime(localTime(a.AnsweredAt), func(t time.Time) string { return kit.Clock(c, t) })))
		})
		return
	}
	key := logic.CardKey(string(msg), index)
	k := s.cardOf(key)
	answer := func(g jc.Grant) {
		k.busy = true
		ans := agent.Answer{Message: msg, Index: index, Grant: g, Note: strings.TrimSpace(k.note)}
		s.st.Actions.Answer(t.Project, t.Chat.ID, ans, func(err error) {
			k.busy = false
			s.st.Error(err)
		})
	}
	refresh := strings.HasPrefix(a.Target, "refresh:")
	deny := false
	for _, o := range a.Options {
		deny = deny || (!refresh && o.Grant == jc.GrantDeny)
	}
	view := logic.PickViewOf(a.Options, k.all)
	ask := a.Ask
	if k.all {
		ask = "Pick a window for the agent to use"
	}
	kit.Card(c, "Approval: "+ask, func() {
		kit.Column(c).Gap(4).Children(func() {
			kit.Kicker(c, strings.ToUpper("Approval · "+logic.CardKind(a)))
			kit.CardTitle(c, ask)
			if a.Why != "" {
				kit.Text(c, a.Why).FontSize(14)
			}
			if a.Risk != "" {
				kit.Muted(c, a.Risk).MaxLines(4)
			}
		})
		if a.Kind == "memory" {
			if len(a.Edits) > 0 {
				for _, e := range a.Edits {
					s.memoryEdit(c, key+e.Target, logic.MemoryTitle(e.Target), e.Before, e.After)
				}
			} else {
				s.memoryEdit(c, key, "", a.Before, a.After)
			}
		}
		if view != nil && view.Kind == logic.PickList {
			kit.Column(c).Gap(4).AlignItems(kit.Stretch).Children(func() {
				for _, o := range view.First {
					if windowButton(c, o.Label, k.busy) {
						answer(o.Grant)
					}
				}
				other := func() {
					for _, o := range view.Other {
						if windowButton(c, o.Label, k.busy) {
							answer(o.Grant)
						}
					}
				}
				if len(view.Other) > 0 && view.Folded {
					label := fmt.Sprintf("Other windows (%d)", len(view.Other))
					kit.Disclosure(c, label, s.flag(key+":other"), false, func() { kit.Text(c, label) }, other)
				} else {
					other()
				}
			})
		}
		if a.Details != "" {
			kit.Disclosure(c, "Details", s.flag(key+":details"), false, func() { kit.Text(c, "Details") }, func() {
				kit.Panel(c, func() { kit.Text(c, a.Details).PreserveSpaces() })
			})
		}
		kit.Row(c).Gap(8).Wrap().AlignItems(kit.Center).Children(func() {
			if view != nil && view.Kind == logic.PickOne {
				if kit.Disable(kit.Button(c, "Allow", kit.Default), k.busy).Clicked() {
					answer(view.Window.Grant)
				}
			}
			n := 0
			for _, o := range a.Options {
				if logic.IsPick(o.Grant) {
					continue
				}
				v := kit.Default
				if o.Grant == jc.GrantDeny || n > 0 {
					v = kit.Outline
				}
				n++
				if kit.Disable(kit.Button(c, o.Label, v).Key(string(o.Grant)), k.busy).Clicked() {
					answer(o.Grant)
				}
			}
			if deny {
				kit.TextInput(c, &k.note, "Note for the agent").Placeholder("Note for the agent (optional)").MinWidth(160).Grow(1)
			}
			if view != nil && view.Kind == logic.PickOne {
				if kit.Disable(kit.Button(c, "Show all windows", kit.Ghost), k.busy).Clicked() {
					k.all = true
				}
			}
		})
	})
}

func localTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	l := t.Local()
	return &l
}

func windowButton(c *kit.Context, label string, busy bool) bool {
	b := kit.SmallButton(c, kit.IconAppWindow, label, kit.Outline).Height(32).Key(label)
	return kit.Disable(b, busy).Clicked()
}

// memoryEdit is a memory edit's section, before and after (SPEC 3.3): the
// changed lines first, then the whole section.
func (s *Screen) memoryEdit(c *kit.Context, key, title, before, after string) {
	ops := logic.LineDiff(before, after)
	changes := logic.Changes(ops)
	if title == "" {
		title = "Changes"
	}
	kit.Column(c).Gap(6).AlignItems(kit.Stretch).Children(func() {
		kit.Kicker(c, strings.ToUpper(title))
		kit.Column(c).AlignItems(kit.Stretch).Children(func() {
			for _, ch := range changes {
				diffLine(c, ch.Kind, ch.Text, ch.Blanks)
			}
		})
		same := false
		for _, o := range ops {
			same = same || o.Kind == logic.OpSame
		}
		if same {
			kit.Disclosure(c, "Whole section", s.flag(key+":whole"), false, func() { kit.Text(c, "Whole section") }, func() {
				for _, o := range ops {
					diffLine(c, o.Kind, o.Text, 0)
				}
			})
		}
	})
}

func diffLine(c *kit.Context, kind logic.OpKind, text string, blanks int) {
	k := kit.TokensOf(c)
	row := kit.Row(c).Gap(8).Padding(2, 12)
	sign, label := "", ""
	switch kind {
	case logic.OpAdd:
		sign, label = "+", "Added: "
		row.Background(k.Tone(kit.Green).Soft)
	case logic.OpDel:
		sign, label = "−", "Removed: "
		row.Background(k.Tone(kit.Red).Soft)
	}
	row.Label(label + text).Children(func() {
		kit.Mono(c, sign).Width(12).Shrink(0).TextColor(k.MutedForeground)
		switch {
		case blanks == 1:
			kit.Muted(c, "a blank line")
		case blanks > 1:
			kit.Muted(c, fmt.Sprintf("%d blank lines", blanks))
		default:
			t := kit.Text(c, text+" ").Grow(1).Shrink(1).PreserveSpaces()
			if kind == logic.OpDel {
				t.Strikethrough()
			}
			if kind == logic.OpSame {
				t.TextColor(k.MutedForeground)
			}
		}
	})
}

// questionForm is ask_user's form (SPEC 8.8): its questions one at a
// time, with Other always there; answered or stopped, a record of each.
func (s *Screen) questionForm(c *kit.Context, t *state.Thread, msg id.Message, index int, q jc.Question, live bool) {
	if q.AnsweredAt != nil || q.Stopped || !live {
		answered := len(q.Answers) > 0
		kit.Column(c).Gap(6).Children(func() {
			for _, it := range q.Questions {
				icon, tone := kit.IconCheck, kit.Green
				what := strings.Join(q.Answers[it.Header], ", ")
				switch {
				case answered && what == "":
					what = "no answer"
				case !answered && q.Note != "":
					icon, tone, what = kit.IconCircleSlash, kit.Neutral, "you wrote instead"
				case !answered:
					icon, tone, what = kit.IconCircleSlash, kit.Neutral, "not answered"
				}
				kit.Record(c, icon, tone, func() {
					kit.Muted(c, it.Question+" ·")
					kit.Text(c, what)
				}).Key(it.Header)
			}
		})
		return
	}
	k := s.cardOf(logic.CardKey(string(msg), index))
	if len(q.Questions) == 0 {
		return
	}
	k.step = min(k.step, len(q.Questions)-1)
	it := q.Questions[k.step]
	choice := k.choice[it.Header]
	if choice == nil {
		choice = new(string)
		for _, o := range it.Options {
			if o.Recommended && !it.Multi {
				*choice = o.Label
			}
		}
		k.choice[it.Header] = choice
	}
	if k.checked[it.Header] == nil {
		k.checked[it.Header] = map[string]*bool{}
	}
	other := k.other[it.Header]
	if other == nil {
		other = new(string)
		k.other[it.Header] = other
	}
	kit.Card(c, "Question: "+it.Question, func() {
		kit.Column(c).Gap(4).Children(func() {
			kit.Kicker(c, strings.ToUpper("Question · "+it.Header))
			kit.CardTitle(c, it.Question)
		})
		kit.Column(c).Gap(8).AlignItems(kit.Stretch).Key(it.Header).Children(func() {
			if it.Multi {
				for _, o := range it.Options {
					b := k.checked[it.Header][o.Label]
					if b == nil {
						b = new(bool)
						k.checked[it.Header][o.Label] = b
					}
					kit.Checkbox(c, b, optionLabel(o))
				}
			} else {
				kit.RadioGroup(c, it.Header, func() {
					for _, o := range it.Options {
						kit.Radio(c, choice, o.Label, optionLabel(o), o.Description)
					}
				})
			}
			kit.TextInput(c, other, "Other answer").Placeholder("Other…")
		})
		kit.Row(c).Gap(8).AlignItems(kit.Center).Children(func() {
			many := len(q.Questions) > 1
			if many {
				if kit.Disable(kit.Button(c, "Previous", kit.Outline), k.step == 0).Clicked() && k.step > 0 {
					k.step--
				}
				kit.Muted(c, fmt.Sprintf("%d of %d", k.step+1, len(q.Questions)))
			}
			if k.step < len(q.Questions)-1 {
				if kit.Button(c, "Next", kit.Default).Clicked() {
					k.step++
				}
				return
			}
			if kit.Disable(kit.Button(c, "Submit", kit.Default), k.busy).Clicked() && !k.busy {
				answers := map[string][]string{}
				for _, it := range q.Questions {
					var picked []string
					if it.Multi {
						for _, o := range it.Options {
							if b := k.checked[it.Header][o.Label]; b != nil && *b {
								picked = append(picked, o.Label)
							}
						}
					} else if ch := k.choice[it.Header]; ch != nil && strings.TrimSpace(*ch) != "" {
						picked = append(picked, *ch)
					}
					if o := k.other[it.Header]; o != nil && strings.TrimSpace(*o) != "" {
						picked = append(picked, *o)
					}
					answers[it.Header] = picked
				}
				k.busy = true
				s.st.Actions.Answer(t.Project, t.Chat.ID, agent.Answer{Message: msg, Index: index, Answers: answers}, func(err error) {
					k.busy = false
					s.st.Error(err)
				})
			}
		})
		kit.Muted(c, "Or type your answer in the chat.")
	})
}

func optionLabel(o jc.Option) string {
	if o.Recommended {
		return o.Label + " (Recommended)"
	}
	return o.Label
}

// failedCard is a turn a provider error stopped; Retry continues it
// (8.3). The Connect card for a chat with no provider set up is not
// built on MyGo yet.
func (s *Screen) failedCard(c *kit.Context, t *state.Thread, text string) {
	k := s.cardOf("failed:" + string(t.Chat.ID))
	kit.Card(c, "The turn stopped", func() {
		kit.CardTitle(c, "The turn stopped")
		kit.Muted(c, logic.NoticeText(text)).MaxLines(6)
		kit.Row(c).Children(func() {
			b := kit.SmallButton(c, kit.IconRefresh, "Retry", kit.Default).Height(32)
			if kit.Disable(b, k.busy).Clicked() && !k.busy {
				k.busy = true
				s.st.Actions.Retry(t.Project, t.Chat.ID, func(err error) {
					k.busy = false
					s.st.Error(err)
				})
			}
		})
	})
}

var retryKinds = map[string]string{
	"rate_limited": "is rate limited",
	"overloaded":   "is overloaded",
	"transport":    "can't be reached",
}

// bars are the strips above the composer: what a tool call is doing, a
// request waiting to be tried again, and the card the turn waits for
// while it is out of view.
func (s *Screen) bars(c *kit.Context, v *view, t *state.Thread) {
	if t.Doing != "" {
		d := []rune(t.Doing)
		kit.Strip(c, nil, kit.Neutral, strings.ToUpper(string(d[:1]))+string(d[1:]), nil)
	}
	if r := t.Retry; r != nil {
		left := int(time.Until(r.At).Seconds() + 0.999)
		when := "now"
		if left > 0 {
			when = fmt.Sprintf("in %d s", left)
			if !v.ticking {
				v.ticking = true
				time.AfterFunc(time.Second, func() { s.post(func() { v.ticking = false }) })
			}
		}
		kind, ok := retryKinds[r.Kind]
		if !ok {
			kind = "failed"
		}
		kit.Strip(c, kit.IconRefresh, kit.Amber, fmt.Sprintf("%s %s, retrying %s", r.Provider, kind, when), func() {
			if kit.SmallButton(c, nil, "Retry now", kit.Outline).Clicked() {
				s.st.Actions.Retry(t.Project, t.Chat.ID, nil)
			}
			if kit.SmallButton(c, nil, "Cancel", kit.Ghost).Clicked() {
				s.st.Actions.Stop(t.Project, t.Chat.ID)
			}
		})
	}
	if w := t.Waiting; w != nil {
		at := -1
		for i, it := range v.items {
			if it.kind == itemRow {
				r := v.rows[it.row]
				for _, m := range r.Messages {
					if m.ID == w.Message {
						at = i
					}
				}
			}
		}
		first, last := v.list.Visible()
		if at >= first && at <= last {
			return
		}
		what := "approval"
		if w.Kind == jc.PartQuestion {
			what = "answer"
		}
		kit.Strip(c, kit.IconBell, kit.Amber, "Waiting for your "+what+": "+w.Text, func() {
			b := kit.SmallButton(c, nil, "Show", kit.Outline)
			if kit.Disable(b, at < 0).Clicked() && at >= 0 {
				v.list.HoldPlace()
				v.list.ScrollTo(at, kit.Center)
			}
		})
	}
}

// rewindAsk is the rewind dialog's question: Rewind to here, or Edit and
// resend, with what the rewind would do.
type rewindAsk struct {
	edit    bool
	state   agent.RewindState
	message jc.Message
	busy    bool
}

// rewindActions are Rewind to here and Edit and resend on a message that
// starts a turn (TASK-018).
func (s *Screen) rewindActions(c *kit.Context, t *state.Thread, m jc.Message, later bool) {
	busy := s.flag("rewinding:" + string(t.Chat.ID))
	confirm := func(edit bool) {
		*busy = true
		turn := m.Turn
		if edit {
			turn--
		}
		s.st.Actions.RewindState(t.Project, t.Chat.ID, turn, func(st agent.RewindState, err error) {
			*busy = false
			if err != nil {
				s.st.Error(err)
				return
			}
			s.ask = &rewindAsk{edit: edit, state: st, message: m}
			s.dialog = true
		})
	}
	if later && kit.Disable(kit.SmallButton(c, kit.IconUndo, "Rewind to here", kit.Ghost), *busy).Clicked() && !*busy {
		confirm(false)
	}
	if kit.Disable(kit.SmallButton(c, kit.IconPencil, "Edit and resend", kit.Ghost), *busy).Clicked() && !*busy {
		confirm(true)
	}
}

// rewindDialog asks before a rewind, saying what stays.
func (s *Screen) rewindDialog(c *kit.Context, t *state.Thread) {
	a := s.ask
	if a == nil {
		return
	}
	title := "Edit and resend?"
	if !a.edit {
		title = "Take " + logic.TurnsText(a.state.Turn+1, a.state.LastTurn) + " out?"
	}
	kit.Dialog(c, &s.dialog, title, func() {
		kit.Column(c).Gap(8).Children(func() {
			for _, l := range logic.RewindLines(a.state, a.edit) {
				kit.Text(c, l).FontSize(14).TextColor(kit.TokensOf(c).MutedForeground)
			}
		})
		kit.Row(c).Gap(8).Justify(kit.End).Children(func() {
			if kit.Disable(kit.Button(c, "Cancel", kit.Outline), a.busy).Clicked() && !a.busy {
				s.dialog = false
			}
			label := "Rewind"
			if a.edit {
				label = "Edit and resend"
			}
			if kit.Disable(kit.Button(c, label, kit.Default), a.busy).Clicked() && !a.busy {
				a.busy = true
				p, ch := t.Project, t.Chat.ID
				s.st.Actions.Rewind(p, ch, a.state.Turn, func(_ agent.RewindResult, err error) {
					a.busy = false
					s.dialog = false
					if err != nil {
						s.st.Error(err)
						return
					}
					if a.edit {
						s.st.Threads.Fill(ch, state.MessageText(&a.message))
						var q *jc.Quote
						for _, p := range a.message.Parts {
							if p.Quote != nil && p.Quote.Turn <= a.state.Turn {
								q = p.Quote
							}
						}
						s.st.Quotes.Set(ch, q)
					}
					s.st.Threads.Reload(ch)
				})
			}
		})
	})
	if !s.dialog && !a.busy {
		s.ask = nil
	}
}

// undo is Undo turn's state for a turn or a rewind's turns.
type undo struct {
	state  *agent.TurnUndoState
	seen   int
	read   bool
	busy   bool
	open   bool
	redo   bool
	rest   bool
	partly bool
	plan   agent.TurnPlan
	migr   int
	confl  []agent.TurnConflict
	skip   []store.UndoItem
}

// undoTurn is Undo turn (SPEC 2.6), or for rewind > 0 the undo of the
// changes of the turns that rewind took out, named label (undo.tsx).
func (s *Screen) undoTurn(c *kit.Context, t *state.Thread, turn int, rewind int64, label string) {
	key := fmt.Sprintf("undo:%s:%d:%d", t.Chat.ID, turn, rewind)
	u := s.undos[key]
	if u == nil {
		u = &undo{}
		s.undos[key] = u
	}
	p, ch := t.Project, t.Chat.ID
	read := func(then func(agent.TurnUndoState)) {
		s.st.Actions.UndoState(p, ch, turn, rewind, func(st agent.TurnUndoState, err error) {
			if err != nil {
				if then != nil {
					u.busy = false
					s.st.Error(err)
				}
				return
			}
			u.state = &st
			if then != nil {
				then(st)
			}
		})
	}
	if !u.read || u.seen != len(t.Messages) {
		u.read, u.seen = true, len(t.Messages)
		read(nil)
	}
	it, noun := "This turn", "turn"
	name := fmt.Sprintf("turn %d", turn)
	if rewind > 0 {
		it, noun, name = "These turns", "changes", "the changes of "+label
	}
	open := func(redo *bool) {
		u.busy = true
		read(func(st agent.TurnUndoState) {
			u.busy = false
			pt := logic.PartlyOf(&st)
			confirm := func(r bool, plan agent.TurnPlan, m int) {
				u.open, u.redo, u.partly, u.rest, u.plan, u.migr, u.confl = true, r, pt != nil, logic.IsRest(pt, r), plan, m, nil
			}
			switch {
			case (redo == nil || !*redo) && st.Undo.Changes > 0:
				confirm(false, st.Undo, st.Migrations)
			case (redo == nil || *redo) && st.Redo.Changes > 0:
				confirm(true, st.Redo, 0)
			case redo == nil:
				s.st.Toasts = append(s.st.Toasts, state.Toast{Text: it + " changed nothing that can be undone."})
			default:
				way := "in effect"
				if *redo {
					way = "undone"
				}
				s.st.Toasts = append(s.st.Toasts, state.Toast{Text: "Nothing of " + strings.ToLower(it) + " is " + way + " now."})
			}
		})
	}
	partly := logic.PartlyOf(u.state)
	if rewind > 0 && !u.open && (u.state == nil || (u.state.Undo.Changes == 0 && u.state.Redo.Changes == 0)) {
		return
	}
	if partly != nil {
		kit.Text(c, partly.Text)
		ways := []bool{false, true}
		if partly.Rest == logic.RestRedo {
			ways = []bool{true, false}
		}
		for _, redo := range ways {
			verb := "Undo"
			if redo {
				verb = "Redo"
			}
			what := noun
			if logic.IsRest(partly, redo) {
				what = "the rest"
			}
			if kit.Disable(kit.SmallButton(c, nil, verb+" "+what, kit.Ghost).Key(verb), u.busy).Clicked() && !u.busy {
				open(&redo)
			}
		}
	} else {
		verb := "Undo "
		if u.state != nil && u.state.Undo.Changes == 0 && u.state.Redo.Changes > 0 {
			verb = "Redo "
		}
		if kit.Disable(kit.SmallButton(c, kit.IconUndo, verb+noun, kit.Ghost), u.busy).Clicked() && !u.busy {
			open(nil)
		}
	}
	run := func(skip []store.UndoItem) {
		u.busy = true
		req := agent.UndoRequest{Turn: turn, Redo: u.redo, Skip: skip}
		if rewind > 0 {
			req = agent.UndoRequest{Rewind: rewind, Redo: u.redo, Skip: skip}
		}
		s.st.Actions.UndoTurn(p, ch, req, func(r agent.TurnUndo, err error) {
			u.busy = false
			switch {
			case err != nil:
				u.open = false
				s.st.Error(err)
			case len(r.Conflicts) > 0:
				u.confl, u.skip = r.Conflicts, skip
			default:
				u.open = false
				if !r.Done && r.Skipped > 0 {
					what := "undone"
					if u.redo {
						what = "redone"
					}
					s.st.Toasts = append(s.st.Toasts, state.Toast{Text: "Nothing was " + what + ": the skipped items stay as they are."})
				}
			}
			read(nil)
		})
	}
	verb := "Undo"
	if u.redo {
		verb = "Redo"
	}
	title := verb + " " + name + "?"
	if u.rest {
		title = verb + " the rest of " + name + "?"
	}
	if u.confl != nil {
		title = "Later changes are in the way"
	}
	kit.Dialog(c, &u.open, title, func() {
		if u.confl != nil {
			kit.Text(c, "Nothing was changed. Leave these as they are and "+strings.ToLower(verb)+" the rest, or cancel.").FontSize(14)
			kit.Scroll(c).MaxHeight(240).Children(func() {
				kit.Column(c).Gap(4).Children(func() {
					for _, cf := range u.confl {
						kit.Text(c, "• "+logic.ConflictText(cf)).FontSize(14)
					}
				})
			})
			kit.Row(c).Gap(8).Justify(kit.End).Children(func() {
				if kit.Disable(kit.Button(c, "Cancel", kit.Outline), u.busy).Clicked() {
					u.open = false
				}
				if kit.Disable(kit.Button(c, "Skip these, "+strings.ToLower(verb)+" the rest", kit.Default), u.busy).Clicked() && !u.busy {
					run(logic.SkipAll(u.skip, u.confl))
				}
			})
			return
		}
		why := refused(u.plan, strings.ToLower(verb), it)
		text := why
		if why == "" {
			what := u.plan.Display
			if what == "" {
				what = "the turn's changes"
				if rewind > 0 {
					what = "their changes"
				}
			}
			text = "This reverts " + what + "."
			if u.redo {
				text = "This puts back " + what + "."
			}
			if u.partly {
				switch {
				case u.rest && u.redo:
					text += " The rest is in effect already."
				case u.rest:
					text += " The rest is undone already."
				case u.redo:
					text += " The items still in effect stay."
				default:
					text += " The items still undone stay."
				}
			}
			if u.migr > 0 {
				text += " Table changes from migrations stay, since migrations are never undone."
			}
		}
		kit.Text(c, text).FontSize(14).TextColor(kit.TokensOf(c).MutedForeground)
		kit.Row(c).Gap(8).Justify(kit.End).Children(func() {
			cancel := "Cancel"
			if why != "" {
				cancel = "Close"
			}
			if kit.Disable(kit.Button(c, cancel, kit.Outline), u.busy).Clicked() {
				u.open = false
			}
			if why == "" {
				what := noun
				if u.rest {
					what = "the rest"
				}
				if kit.Disable(kit.Button(c, verb+" "+what, kit.Default), u.busy).Clicked() && !u.busy {
					run(nil)
				}
			}
		})
	})
}

// refused says why a plan can't run, or "" when it can.
func refused(p agent.TurnPlan, verb, it string) string {
	its, is := "Its", "is"
	if it != "This turn" {
		its, is = "Their", "are"
	}
	if p.Past {
		return it + " " + is + " past the undo window. " + its + " changes are kept as history only."
	}
	if p.TooBig {
		return fmt.Sprintf("%s changed %s rows, more than %s can revert at once.", it, thousands(p.Entries), verb)
	}
	return ""
}

func thousands(n int) string {
	s := itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
