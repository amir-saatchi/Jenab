package agent

import (
	"context"
	"fmt"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
)

// Rewind (TASK-018): the user takes a chat back to the end of an earlier
// turn, from *Rewind to here* or *Edit and resend* on their message. The
// later turns leave the chat and its context, and go into the undo
// window like other deletions (SPEC 4.4): *Undo rewind* brings them back
// until the chat goes on. There are no branches.
//
//   - Between turns only, like Clear: ErrInTurn, ErrRefreshing.
//   - The history index, session notes, loaded skills and Refresh
//     memory's progress follow the chat (store.ChatsDB.Rewind).
//   - Project data the turns changed stays: the dialog lists it, and the
//     rewind's mark offers Undo turn for it (UndoRequest.Rewind). The
//     model gets a notice of what stays, so new rows don't surprise it.
//   - The next request starts a new cached prefix: a rewind is a cut
//     (3.6), as the window is built again.
//   - Nothing else outlives a turn today: subagents run inside their
//     call, and a desktop session ends with its turn. Background tasks,
//     create_chat and send_to_chat come with Phase 5.

// RewindState is what a rewind of the chat to Turn would do, for its
// dialog.
type RewindState struct {
	Turn     int `json:"turn"`      // the chat would end with this turn
	LastTurn int `json:"last_turn"` // the last turn that would leave
	// Changes is what the turns that would leave changed in the project
	// and is still in effect; it stays after the rewind.
	Changes TurnPlan `json:"changes"`
	Notes   bool     `json:"notes"`  // the session notes would go back
	Unload  []string `json:"unload"` // skills that would be unloaded
	Load    []string `json:"load"`   // skills that would be loaded again
}

// RewindResult is what Rewind did.
type RewindResult struct {
	Rewind   int64    `json:"rewind"` // for Undo rewind and the undo of its changes
	Turn     int      `json:"turn"`
	LastTurn int      `json:"last_turn"`
	Changes  TurnPlan `json:"changes"` // kept in the project
	Notes    bool     `json:"notes"`
	Skills   []string `json:"skills"` // the chat's skills now; nil if they didn't change
}

// RewindState says what a rewind of the chat to the end of turn would do.
func (o *Orchestrator) RewindState(ctx context.Context, p id.Project, c id.Chat, turn int) (RewindState, error) {
	s := RewindState{Turn: turn}
	if err := o.enter(); err != nil {
		return s, err
	}
	defer o.leave()
	proj, err := o.d.Projects.Open(ctx, p)
	if err != nil {
		return s, err
	}
	defer proj.Release()
	pv, err := proj.Chats.RewindPreview(ctx, c, turn)
	if err != nil {
		return s, err
	}
	st, err := turnState(ctx, proj, 0, sources(pv.Messages))
	if err != nil {
		return s, err
	}
	return RewindState{Turn: turn, LastTurn: pv.LastTurn, Changes: st.Undo, Notes: pv.Notes, Unload: pv.Unload, Load: pv.Load}, nil
}

// Rewind takes the chat back to the end of turn; 0 empties it, like
// Clear but with undo. It fails with ErrInTurn while the chat is in a
// turn, and with store.ErrNothingToRewind when it has no later turn.
func (o *Orchestrator) Rewind(ctx context.Context, p id.Project, c id.Chat, turn int) (RewindResult, error) {
	res := RewindResult{Turn: turn}
	if err := o.enter(); err != nil {
		return res, err
	}
	defer o.leave()
	proj, err := o.d.Projects.Open(ctx, p)
	if err != nil {
		return res, err
	}
	defer proj.Release()
	cs := o.state(p, c)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	switch {
	case cs.refresh.on:
		return res, ErrRefreshing
	case cs.running || cs.queued:
		return res, ErrInTurn
	}
	pv, err := proj.Chats.RewindPreview(ctx, c, turn)
	if err != nil {
		return res, err
	}
	st, err := turnState(ctx, proj, 0, sources(pv.Messages))
	if err != nil {
		return res, err
	}
	var notice *chat.Message
	if turn > 0 && st.Undo.Changes > 0 {
		n := chat.Notice{Kind: chat.NoticeRewound, Text: rewindNotice(turn+1, pv.LastTurn, st.Undo)}
		notice = &chat.Message{Chat: c, Turn: turn, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartNotice, Notice: &n}}}
	}
	cs.pub.Lock()
	r, err := proj.Chats.Rewind(ctx, c, turn, notice)
	if err == nil {
		cs.bumpSeq(r.Seq)
	}
	cs.pub.Unlock()
	if err != nil {
		return res, err
	}
	o.rewound(ctx, cs, proj, turn, turn == 0)
	return RewindResult{Rewind: r.ID, Turn: turn, LastTurn: r.LastTurn, Changes: st.Undo, Notes: r.Notes, Skills: r.Skills}, nil
}

// UndoRewind brings back the turns a rewind took out of the chat. Only
// the chat's newest rewind can be undone, while the chat has not gone on
// (store.ErrRewindMoved), and within the undo window
// (store.ErrRewindGone).
func (o *Orchestrator) UndoRewind(ctx context.Context, p id.Project, c id.Chat, rewind int64) error {
	if err := o.enter(); err != nil {
		return err
	}
	defer o.leave()
	proj, err := o.d.Projects.Open(ctx, p)
	if err != nil {
		return err
	}
	defer proj.Release()
	cs := o.state(p, c)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	switch {
	case cs.refresh.on:
		return ErrRefreshing
	case cs.running || cs.queued:
		return ErrInTurn
	}
	cs.pub.Lock()
	r, err := proj.Chats.UndoRewind(ctx, c, rewind)
	if err == nil {
		cs.bumpSeq(r.Seq)
	}
	cs.pub.Unlock()
	if err != nil {
		return err
	}
	o.rewound(ctx, cs, proj, r.LastTurn, false)
	return nil
}

// rewound sets the chat's state after a rewind or its undo: it is read
// from the store again, and the window is built again at the next turn,
// a cut with a new cached prefix (3.6). Traces of turns after turn go.
// With empty, the chat has no turn left, so a title still being made
// from its first turn is dropped, as after Clear. cs.mu is held.
func (o *Orchestrator) rewound(ctx context.Context, cs *chatState, proj *project.Project, turn int, empty bool) {
	if empty {
		cs.clears++
	}
	cs.loaded, cs.lastUser, cs.retry, cs.win = false, "", nil, window{}
	if err := o.load(ctx, cs, proj); err != nil {
		// The next Send or Retry reads it again.
		o.d.Log.Warn("agent: reading the chat after a rewind", "chat", cs.key.c, "err", err)
	}
	o.d.Traces.dropAfter(cs.key.p, cs.key.c, turn)
	o.publishStatus(cs)
}

// RewindUndoState says what Undo turn and Redo turn would revert now for
// the changes of the turns a rewind took out, for the rewind's mark.
func (o *Orchestrator) RewindUndoState(ctx context.Context, p id.Project, c id.Chat, rewind int64) (TurnUndoState, error) {
	if err := o.enter(); err != nil {
		return TurnUndoState{}, err
	}
	defer o.leave()
	proj, err := o.d.Projects.Open(ctx, p)
	if err != nil {
		return TurnUndoState{}, err
	}
	defer proj.Release()
	ms, err := proj.Chats.RewindMessages(ctx, c, rewind)
	if err != nil {
		return TurnUndoState{}, err
	}
	return turnState(ctx, proj, 0, sources(ms))
}

// sources are message:<id> of each of ms, the sources of their changes.
func sources(ms []id.Message) []id.Source {
	out := make([]id.Source, len(ms))
	for i, m := range ms {
		out[i] = id.SourceOf("message", string(m))
	}
	return out
}

// turnsText is "turn 4" or "turns 4–6".
func turnsText(first, last int) string {
	if first == last {
		return fmt.Sprintf("turn %d", first)
	}
	return fmt.Sprintf("turns %d–%d", first, last)
}

// rewindNotice tells the model that the user took turns first to last out
// of the chat, and what of their changes stays in the project, e.g.
// "[the user took turns 4–6 out of this chat; what they changed in the
// project stays: 12 rows in prices]". It says nothing of what the turns
// were about.
func rewindNotice(first, last int, kept TurnPlan) string {
	what := kept.Summary
	if what == "" {
		what = "their changes"
	}
	return fmt.Sprintf("[the user took %s out of this chat; what they changed in the project stays: %s]", turnsText(first, last), what)
}
