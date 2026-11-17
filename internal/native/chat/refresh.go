package chat

import (
	"github.com/amir-saatchi/jenab/internal/agent"
	jc "github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/native/chat/logic"
	"github.com/amir-saatchi/jenab/internal/native/kit"
	"github.com/amir-saatchi/jenab/internal/native/state"
)

// Refresh memory (SPEC 3.5): the button in the chat's header, as
// RefreshButton in refresh.tsx. The header is one line as high as the
// window's buttons (TASK-063), so the React app's split button is one bar
// button whose menu has both refreshes, each with what it costs under it.
// A long one asks first, with its cost; the chat's status shows the
// progress, and Stop cancels it. The batch's chip is in chips.go.

// refresh is a chat's Refresh memory button between frames.
type refresh struct {
	open   bool          // its menu shows
	shown  bool          // it showed in the last frame
	thread *state.Thread // the estimates under the items are of it
	lines  [2]string     // under Refresh memory and Full refresh
	asked  int           // the estimates asked for, so a late one is dropped
	busy   bool          // a refresh is being weighed or started
}

// refreshAsk is a long refresh waiting on its dialog.
type refreshAsk struct {
	project id.Project
	chat    id.Chat
	full    bool
	est     agent.RefreshEstimate
}

// refreshItems are the menu's items: a refresh of the turns not reviewed
// yet, and a full one.
var refreshItems = [2]string{"Refresh memory", "Full refresh: review the whole chat again"}

func (s *Screen) refreshOf(ch id.Chat) *refresh {
	r := s.refreshes[ch]
	if r == nil {
		r = &refresh{}
		s.refreshes[ch] = r
	}
	return r
}

// HeaderActions puts the chat's buttons in its header, before its ⋯
// menu: Refresh memory, off while the chat is busy.
func (s *Screen) HeaderActions(c *kit.Context, t *state.Thread) {
	r := s.refreshOf(t.Chat.ID)
	off := t.State != jc.StateIdle || r.busy
	kit.BarMenuButton(c, kit.IconBrain, "Refresh memory", &r.open, off, func(m *kit.Menu) {
		for i, label := range refreshItems {
			if m.Item(label).Subtitle(r.lines[i]).Chosen() {
				s.startRefresh(t, r, i == 1)
			}
		}
	})
	if r.open && (!r.shown || r.thread != t) {
		s.estimate(t, r)
	}
	r.shown = r.open
}

// estimate fills the menu's lines with what each refresh would cost,
// each time the menu opens and as the chat changes under it.
func (s *Screen) estimate(t *state.Thread, r *refresh) {
	r.thread = t
	r.asked++
	asked := r.asked
	p, ch := t.Project, t.Chat.ID
	for i := range refreshItems {
		r.lines[i] = "Counting the turns"
		s.st.Actions.RefreshEstimate(p, ch, i == 1, func(e agent.RefreshEstimate, err error) {
			if r.asked != asked {
				return
			}
			if err != nil {
				r.lines[i] = "" // choosing it says what is wrong
				return
			}
			r.lines[i] = logic.EstimateLine(e)
		})
	}
}

// startRefresh weighs a refresh, then starts it, or asks first when it is
// long.
func (s *Screen) startRefresh(t *state.Thread, r *refresh, full bool) {
	if r.busy {
		return
	}
	r.busy = true
	p, ch := t.Project, t.Chat.ID
	s.st.Actions.RefreshEstimate(p, ch, full, func(e agent.RefreshEstimate, err error) {
		if err != nil {
			r.busy = false
			s.st.Error(err)
			return
		}
		if e.Long {
			s.long, s.longOpen = &refreshAsk{project: p, chat: ch, full: full, est: e}, true
			return
		}
		s.runRefresh(r, p, ch, full)
	})
}

func (s *Screen) runRefresh(r *refresh, p id.Project, ch id.Chat, full bool) {
	r.busy = true
	s.st.Actions.RefreshMemory(p, ch, full, func(err error) {
		r.busy = false
		s.st.Error(err)
	})
}

// refreshDialog asks before a long refresh, with what it costs. It goes
// with its chat: another chat showing drops it.
func (s *Screen) refreshDialog(c *kit.Context, t *state.Thread) {
	a := s.long
	if a == nil {
		return
	}
	r := s.refreshOf(a.chat)
	if a.chat != t.Chat.ID || !s.longOpen {
		s.long, s.longOpen, r.busy = nil, false, false
		return
	}
	model := logic.ModelName(s.st.Settings.Models, a.est.Model)
	switch kit.AlertDialog(c, &s.longOpen, logic.EstimateTitle(a.est, a.full), logic.EstimateText(a.est, model), "Cancel", "Refresh") {
	case 1:
		s.long = nil
		s.runRefresh(r, a.project, a.chat, a.full)
	case 0, kit.AlertDismissed:
		s.long, r.busy = nil, false
	}
}
