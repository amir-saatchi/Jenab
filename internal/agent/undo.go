package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

// TurnPlan is what Undo turn or Redo turn would revert, for the footer.
type TurnPlan struct {
	Changes int               `json:"changes"` // 0: nothing to revert
	Rows    []store.TableRows `json:"rows"`
	Summary string            `json:"summary"` // "12 rows in btc_prices, 1 file"
	Display string            `json:"display"` // Summary for the UI, table names in bidi isolates
	Past    bool              `json:"past"`    // past the undo window, so it is refused
	Entries int               `json:"entries"` // row entries, as store.MaxUndoEntries counts them
	TooBig  bool              `json:"too_big"` // over store.MaxUndoEntries, so it is refused
}

// TurnUndoState is what Undo turn and Redo turn would do now.
type TurnUndoState struct {
	Turn       int      `json:"turn"`
	Undo       TurnPlan `json:"undo"`
	Redo       TurnPlan `json:"redo"`
	Migrations int      `json:"migrations"` // kept by an undo (SPEC 2.6)
}

// UndoRequest is an Undo turn or a Redo turn from the footer, or its second
// try with the conflicts the user chose to skip.
type UndoRequest struct {
	Turn int              `json:"turn"`
	Redo bool             `json:"redo"`
	Skip []store.UndoItem `json:"skip,omitempty"`
}

// TurnConflict is a conflict with the chat that made the later change,
// when a message of a chat made it.
type TurnConflict struct {
	Conflict  store.UndoConflict `json:"conflict"`
	Chat      id.Chat            `json:"chat,omitempty"`
	ChatTitle string             `json:"chat_title,omitempty"`
}

// TurnUndo says what Undo turn did. With conflicts it did nothing: the
// user skips them, with UndoRequest.Skip, or cancels.
type TurnUndo struct {
	Turn       int            `json:"turn"`
	Redo       bool           `json:"redo"`
	Done       bool           `json:"done"`
	Summary    string         `json:"summary"`
	Migrations int            `json:"migrations"` // kept
	Skipped    int            `json:"skipped"`
	Conflicts  []TurnConflict `json:"conflicts,omitempty"`
}

func turnPlan(ps ...store.UndoPlan) TurnPlan {
	var all store.UndoPlan
	for _, p := range ps {
		all.Changes += p.Changes
		all.Rows = append(all.Rows, p.Rows...)
		all.Past = all.Past || p.Past
		all.Entries += p.Entries
		all.TooBig = all.TooBig || p.TooBig
	}
	return TurnPlan{Changes: all.Changes, Rows: all.Rows, Summary: all.Summary(), Display: all.Display(), Past: all.Past,
		Entries: all.Entries, TooBig: all.TooBig}
}

// turnSources are the sources of the turn's changes: message:<id> of each
// of its messages.
func turnSources(ctx context.Context, proj *project.Project, c id.Chat, turn int) ([]id.Source, error) {
	ms, err := proj.Chats.TurnMessages(ctx, c, turn)
	if err != nil {
		return nil, err
	}
	if len(ms) == 0 {
		return nil, fmt.Errorf("agent: turn %d has no messages: %w", turn, store.ErrNothingToUndo)
	}
	srcs := make([]id.Source, len(ms))
	for i, m := range ms {
		srcs[i] = id.SourceOf("message", string(m))
	}
	return srcs, nil
}

// UndoState says what Undo turn and Redo turn would revert now for the
// chat's turn, roles and user memory included.
func (o *Orchestrator) UndoState(ctx context.Context, p id.Project, c id.Chat, turn int) (TurnUndoState, error) {
	s := TurnUndoState{Turn: turn}
	if err := o.enter(); err != nil {
		return s, err
	}
	defer o.leave()
	proj, err := o.d.Projects.Open(ctx, p)
	if err != nil {
		return s, err
	}
	defer proj.Release()
	srcs, err := turnSources(ctx, proj, c, turn)
	if errors.Is(err, store.ErrNothingToUndo) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	st, err := proj.DB.UndoState(ctx, srcs)
	if err != nil {
		return s, err
	}
	ru, rr, err := proj.Chats.RoleUndoState(ctx, srcs)
	if err != nil {
		return s, err
	}
	var mu, mr store.UndoPlan
	if proj.Registry != nil {
		if mu, mr, err = proj.Registry.UserMemoryUndoState(ctx, srcs); err != nil {
			return s, err
		}
	}
	s.Undo, s.Redo, s.Migrations = turnPlan(st.Undo, ru, mu), turnPlan(st.Redo, rr, mr), st.Migrations
	return s, nil
}

// UndoTurn reverts everything the chat's turn changed, or with r.Redo puts
// it back (SPEC 2.6): rows, objects, chat roles and user memory. Migrations stay. A
// later change to what it would revert stops it; the result lists the
// conflicts and nothing changed. When it is done, the chat gets a notice,
// so the model knows. It fails with ErrInTurn while the chat is in a turn.
//
// Roles live in chats.db, user memory in registry.db and the rest in
// project.db, which commit apart: the roles and user memory are checked
// first, so a conflict there stops all. A crash between them leaves the
// turn half undone; the footer then offers both Undo and Redo, and either
// finishes the job.
func (o *Orchestrator) UndoTurn(ctx context.Context, p id.Project, c id.Chat, r UndoRequest) (TurnUndo, error) {
	res := TurnUndo{Turn: r.Turn, Redo: r.Redo}
	if err := o.enter(); err != nil {
		return res, err
	}
	defer o.leave()
	proj, err := o.d.Projects.Open(ctx, p)
	if err != nil {
		return res, err
	}
	defer proj.Release()
	var seqs map[id.Chat]uint64
	res, seqs, err = o.undoTurn(ctx, proj, c, r)
	// Other chats' roles changed too; cs.mu is no longer held.
	for ch, seq := range seqs {
		if ch != c {
			o.Wrote(p, ch, seq)
		}
	}
	return res, err
}

func (o *Orchestrator) undoTurn(ctx context.Context, proj *project.Project, c id.Chat, r UndoRequest) (TurnUndo, map[id.Chat]uint64, error) {
	res := TurnUndo{Turn: r.Turn, Redo: r.Redo}
	cs := o.state(proj.ID, c)
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.running || cs.queued {
		return res, nil, ErrInTurn
	}
	if err := o.load(ctx, cs, proj); err != nil {
		return res, nil, err
	}
	srcs, err := turnSources(ctx, proj, c, r.Turn)
	if err != nil {
		return res, nil, err
	}
	opts := store.UndoOptions{Redo: r.Redo, Skip: r.Skip}
	check := opts
	check.Check = true
	sides := sidesOf(proj)
	var sc []store.UndoConflict
	for _, s := range sides {
		x, err := s.undo(ctx, srcs, check)
		if err != nil && !errors.Is(err, store.ErrUndoConflict) {
			return res, nil, err
		}
		sc = append(sc, x.conflicts...)
	}
	if len(sc) > 0 {
		// List the project's conflicts too, so one dialog has them all.
		pc, err := proj.DB.Undo(ctx, srcs, check)
		if err != nil && !errors.Is(err, store.ErrUndoConflict) && !errors.Is(err, store.ErrNothingToUndo) {
			return res, nil, err
		}
		res.Conflicts = o.label(ctx, proj, append(pc.Conflicts, sc...))
		return res, nil, nil
	}
	pr, err := proj.DB.Undo(ctx, srcs, opts)
	nothing := errors.Is(err, store.ErrNothingToUndo)
	switch {
	case errors.Is(err, store.ErrUndoConflict):
		res.Conflicts = o.label(ctx, proj, pr.Conflicts)
		return res, nil, nil
	case err != nil && !nothing:
		return res, nil, err
	}
	changed := !nothing
	plans := []store.UndoPlan{pr.Plan}
	skipped := len(pr.Skipped)
	var seqs map[id.Chat]uint64
	var sideErr error
	for _, s := range sides {
		x, err := s.undo(ctx, srcs, opts)
		lost := 0 // items changed since the check, left as they are
		if errors.Is(err, store.ErrUndoConflict) {
			if !changed { // nothing changed yet, so the dialog can say so
				res.Conflicts = o.label(ctx, proj, x.conflicts)
				return res, nil, nil
			}
			// A part is in: skip the items changed since the check, so
			// their chains flip with the rest, and report them as skipped.
			lost = len(x.conflicts)
			again := opts
			again.Skip = slices.Clone(opts.Skip)
			for _, c := range x.conflicts {
				again.Skip = append(again.Skip, c.Item)
			}
			x, err = s.undo(ctx, srcs, again)
			if err == nil {
				lost = 0 // in x.skipped now
			} else if errors.Is(err, store.ErrUndoConflict) { // changed again: leave them
				x, err = sideUndo{}, nil
			}
		}
		if err != nil {
			if !changed {
				return res, nil, err
			}
			// A part is in: say so in the chat, and that this one wasn't
			// undone. Undo turn again finishes it.
			o.d.Log.Error("agent: undo of a turn's "+s.name, "turn", r.Turn, "err", err)
			sideErr = errors.Join(sideErr, fmt.Errorf("%w: %w", s.notDone, err))
			x = sideUndo{}
		}
		plans = append(plans, x.plan)
		skipped += x.skipped + lost
		changed = changed || x.plan.Changes > 0
		if x.seq != nil {
			seqs = x.seq
		}
	}
	if !changed {
		return res, nil, fmt.Errorf("agent: turn %d: %w", r.Turn, store.ErrNothingToUndo)
	}
	if seq, ok := seqs[c]; ok {
		cs.bumpSeq(seq)
	}
	res.Done = true
	res.Summary = turnPlan(plans...).Summary
	res.Migrations = len(pr.Migrations)
	res.Skipped = skipped
	n := chat.Notice{Kind: chat.NoticeTurnUndone, Text: undoNotice(res)}
	if _, err := o.write(ctx, cs, proj, chat.Message{Chat: c, Turn: max(cs.turn, 1), Role: chat.RoleUser,
		Parts: []chat.Part{{Kind: chat.PartNotice, Notice: &n}}}); err != nil {
		// The undo is in; an error now would read as if it failed.
		o.d.Log.Warn("agent: the notice of an undo", "turn", r.Turn, "err", err)
	}
	return res, seqs, sideErr
}

// side is a part of a turn's undo outside project.db, which commits apart:
// chat roles in chats.db, user memory in registry.db (SPEC 2.6, 3.3).
type side struct {
	name    string
	notDone error // when it fails after project.db's part is in
	undo    func(ctx context.Context, srcs []id.Source, o store.UndoOptions) (sideUndo, error)
}

// sideUndo is what a side's undo did, or would do with o.Check.
type sideUndo struct {
	plan      store.UndoPlan
	skipped   int
	conflicts []store.UndoConflict
	seq       map[id.Chat]uint64 // roles: the chats' new sequence numbers
}

// sidesOf are proj's sides. The checks call the store; the undos go
// through undoRoles and undoUserMemory, which tests make fail.
func sidesOf(proj *project.Project) []side {
	ss := []side{{name: "chat roles", notDone: ErrRolesNotUndone,
		undo: func(ctx context.Context, srcs []id.Source, o store.UndoOptions) (sideUndo, error) {
			var r store.RoleUndo
			var err error
			if o.Check {
				r, err = proj.Chats.UndoRoles(ctx, srcs, o)
			} else {
				r, err = undoRoles(ctx, proj.Chats, srcs, o)
			}
			return sideUndo{r.Plan, len(r.Skipped), r.Conflicts, r.Seq}, err
		}}}
	if proj.Registry != nil {
		ss = append(ss, side{name: "user memory", notDone: ErrUserMemoryNotUndone,
			undo: func(ctx context.Context, srcs []id.Source, o store.UndoOptions) (sideUndo, error) {
				var r store.UserMemoryUndo
				var err error
				if o.Check {
					r, err = proj.Registry.UndoUserMemory(ctx, srcs, proj.ID, o)
				} else {
					r, err = undoUserMemory(ctx, proj.Registry, srcs, proj.ID, o)
				}
				return sideUndo{r.Plan, len(r.Skipped), r.Conflicts, nil}, err
			}})
	}
	return ss
}

// undoRoles is the roles' undo after project.db's; tests make it fail.
var undoRoles = func(ctx context.Context, c *store.ChatsDB, srcs []id.Source, o store.UndoOptions) (store.RoleUndo, error) {
	return c.UndoRoles(ctx, srcs, o)
}

// undoUserMemory is user memory's undo after project.db's and the roles';
// tests make it fail.
var undoUserMemory = func(ctx context.Context, r *store.Registry, srcs []id.Source, from id.Project, o store.UndoOptions) (store.UserMemoryUndo, error) {
	return r.UndoUserMemory(ctx, srcs, from, o)
}

// ErrRolesNotUndone: UndoTurn reverted the turn's changes in project.db,
// but its chat roles in chats.db failed. The result says what was done;
// Undo turn again reverts the roles.
var ErrRolesNotUndone = errors.New("agent: the turn was undone, but its chat roles weren't")

// ErrUserMemoryNotUndone: as ErrRolesNotUndone, for the turn's edits of
// user memory in registry.db.
var ErrUserMemoryNotUndone = errors.New("agent: the turn was undone, but its user memory edits weren't")

// undoNotice tells the model what Undo turn did, e.g. "[turn 3 was
// undone: 12 rows in prices; its migration was kept]".
func undoNotice(r TurnUndo) string {
	verb := "undone"
	if r.Redo {
		verb = "redone"
	}
	what := r.Summary
	if what == "" {
		what = "its changes"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[turn %d was %s: %s", r.Turn, verb, what)
	switch {
	case r.Migrations == 1:
		b.WriteString("; its migration was kept")
	case r.Migrations > 1:
		fmt.Fprintf(&b, "; its %d migrations were kept", r.Migrations)
	}
	switch {
	case r.Skipped == 1:
		b.WriteString("; 1 item changed since was left as it is")
	case r.Skipped > 1:
		fmt.Fprintf(&b, "; %d items changed since were left as they are", r.Skipped)
	}
	b.WriteString("]")
	return b.String()
}

// label adds the chat of each conflict whose later change came from a
// message, for the dialog.
func (o *Orchestrator) label(ctx context.Context, proj *project.Project, cs []store.UndoConflict) []TurnConflict {
	out := make([]TurnConflict, len(cs))
	var ms []id.Message
	for i, x := range cs {
		out[i].Conflict = x
		if x.Source.Kind() == "message" {
			ms = append(ms, id.Message(x.Source.ID()))
		}
	}
	if len(ms) == 0 {
		return out
	}
	chats, err := proj.Chats.MessageChats(ctx, ms)
	if err != nil {
		o.d.Log.Warn("agent: the chats of undo conflicts", "err", err)
		return out
	}
	titles := map[id.Chat]string{}
	for i := range out {
		if out[i].Conflict.Source.Kind() != "message" {
			continue
		}
		ch, ok := chats[id.Message(out[i].Conflict.Source.ID())]
		if !ok {
			continue
		}
		t, ok := titles[ch]
		if !ok {
			if info, err := proj.Chats.Chat(ctx, ch); err == nil {
				t = info.Title
			}
			titles[ch] = t
		}
		out[i].Chat, out[i].ChatTitle = ch, t
	}
	return out
}
