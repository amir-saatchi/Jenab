package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

// RoleItem is the UndoItem table for a chat's role, which lives in
// chats.db (role_changes, SPEC 8.6); its key is the chat ID.
const RoleItem = "_jenab_chat_role"

// roleUndoSource is the source of the undo of role change n:
// undo:role:<n>, since role changes are numbered in chats.db and are not
// entries of project.db's change log.
func roleUndoSource(n int64) id.Source { return id.SourceOf("undo", "role:"+strconv.FormatInt(n, 10)) }

// roleTip is the tip of a chain of role changes, as undoTips for changes.
type roleTip struct {
	id            int64
	chat          id.Chat
	before, after string
	depth         int
}

func roleTips(tx *sql.Tx, srcs string) ([]roleTip, error) {
	return queryTx(tx, `WITH RECURSIVE chain(base, id, depth) AS (
			SELECT id, id, 0 FROM role_changes WHERE source IN (SELECT value FROM json_each(?1))
			UNION ALL
			SELECT chain.base, r.id, chain.depth + 1 FROM chain JOIN role_changes r ON r.source = 'undo:role:' || chain.id)
		SELECT t.id, r.chat_id, r.before, r.after, t.depth
		FROM (SELECT base, id, max(depth) AS depth FROM chain GROUP BY base) t JOIN role_changes r ON r.id = t.id
		ORDER BY t.id`, []any{srcs}, func(r *sql.Rows) (roleTip, error) {
		var x roleTip
		return x, r.Scan(&x.id, &x.chat, &x.before, &x.after, &x.depth)
	})
}

// pickRoles returns the tips an undo, or with redo a redo, reverts. A
// no-op tip is left out: reverting it changes nothing, so it doesn't
// count in the plan nor flip again.
func pickRoles(tips []roleTip, redo bool) []roleTip {
	var out []roleTip
	for _, t := range tips {
		if !t.noop() && (t.depth%2 == 1) == redo {
			out = append(out, t)
		}
	}
	return out
}

// noop reports whether the tip changed nothing: the record an undo
// writes for a skipped role, so its chain flips. SetRole records no
// change that keeps the role.
func (t roleTip) noop() bool { return t.before == t.after }

func rolePlan(ts []roleTip, skip map[UndoItem]bool) UndoPlan {
	chats := map[id.Chat]bool{}
	for _, t := range ts {
		if !skip[UndoItem{RoleItem, string(t.chat)}] {
			chats[t.chat] = true
		}
	}
	p := UndoPlan{Changes: len(ts)}
	if len(chats) > 0 {
		p.Rows = []TableRows{{Table: RoleItem, Rows: len(chats)}}
	}
	return p
}

// RoleUndoState is what UndoRoles and a redo would revert now: the role
// changes from sources, as UndoState for project.db.
func (c *ChatsDB) RoleUndoState(ctx context.Context, sources []id.Source) (undo, redo UndoPlan, err error) {
	type out struct{ undo, redo UndoPlan }
	o, err := readTx(ctx, c.DB, func(tx *sql.Tx) (out, error) {
		if len(sources) == 0 {
			return out{}, nil
		}
		tips, err := roleTips(tx, sourcesJSON(sources))
		if err != nil {
			return out{}, err
		}
		return out{rolePlan(pickRoles(tips, false), nil), rolePlan(pickRoles(tips, true), nil)}, nil
	})
	return o.undo, o.redo, err
}

// RoleUndo says what UndoRoles did.
type RoleUndo struct {
	Plan      UndoPlan           `json:"plan"`
	Seq       map[id.Chat]uint64 `json:"seq"`               // the new sequence number of each chat whose role changed
	Skipped   []UndoItem         `json:"skipped,omitempty"` // the roles left as they are, as o.Skip asked
	Conflicts []UndoConflict     `json:"conflicts,omitempty"`
}

// UndoRoles reverts the role changes from sources, newest first, in one
// transaction: each chat gets the role it had before, recorded as a role
// change with the source undo:role:<n>, so o.Redo puts it back (SPEC 2.6,
// 8.6). A later role change of the chat outside the undo is a conflict
// (ErrUndoConflict), unless o.Skip has the chat's RoleItem, or the later
// change's chain is undone now and its last undo put the role back. A
// skipped role still gets its record, one that keeps the role, so its
// chain flips. No role change to revert is not an error: a turn rarely
// changes a role.
func (c *ChatsDB) UndoRoles(ctx context.Context, sources []id.Source, o UndoOptions) (RoleUndo, error) {
	var res RoleUndo
	if len(sources) == 0 {
		return res, nil
	}
	_, err := Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		res = RoleUndo{Seq: map[id.Chat]uint64{}}
		tips, err := roleTips(tx, sourcesJSON(sources))
		if err != nil {
			return struct{}{}, err
		}
		skip := map[UndoItem]bool{}
		for _, it := range o.Skip {
			skip[it] = true
		}
		ts := pickRoles(tips, o.Redo)
		res.Plan = rolePlan(ts, skip)
		if len(ts) == 0 {
			return struct{}{}, nil
		}
		set := map[int64]bool{}
		first := map[id.Chat]int64{}
		for _, t := range ts {
			set[t.id] = true
			if it := (UndoItem{RoleItem, string(t.chat)}); skip[it] {
				if !slices.Contains(res.Skipped, it) {
					res.Skipped = append(res.Skipped, it)
				}
				continue
			}
			if f, ok := first[t.chat]; !ok || t.id < f {
				first[t.chat] = t.id
			}
		}
		type later struct {
			chat id.Chat
			id   int64
			src  id.Source
			at   string
		}
		var ls []later
		for ch, f := range first {
			l, err := queryTx(tx, `SELECT chat_id, id, source, at FROM role_changes
				WHERE chat_id = ? AND id > ? AND before <> after ORDER BY id`, []any{ch, f},
				func(r *sql.Rows) (later, error) {
					var l later
					return l, r.Scan(&l.chat, &l.id, &l.src, &l.at)
				})
			if err != nil {
				return struct{}{}, err
			}
			for _, x := range l {
				if !set[x.id] {
					ls = append(ls, x)
				}
			}
		}
		ids := make([]int64, len(ls))
		for i, l := range ls {
			ids[i] = l.id
		}
		undone, err := undoneRoles(tx, ids)
		if err != nil {
			return struct{}{}, err
		}
		listed := map[id.Chat]bool{}
		for _, l := range ls {
			if undone[l.id] || listed[l.chat] {
				continue
			}
			listed[l.chat] = true
			x := UndoConflict{Item: UndoItem{RoleItem, string(l.chat)}, Reason: ConflictChanged, Source: l.src, Detail: "the chat's role changed since"}
			if at, err := parseTime(l.at); err == nil {
				x.At = &at
			}
			res.Conflicts = append(res.Conflicts, x)
		}
		if len(res.Conflicts) > 0 {
			return struct{}{}, ErrUndoConflict
		}
		for i := len(ts) - 1; i >= 0; i-- {
			t := ts[i]
			var cur string
			if err := tx.QueryRow(`SELECT role FROM chats WHERE id = ?`, t.chat).Scan(&cur); err != nil {
				return struct{}{}, err
			}
			if skip[UndoItem{RoleItem, string(t.chat)}] {
				// Nothing to revert, but the chain flips, as an undo of a
				// change with every row skipped does in project.db. A
				// record that keeps the role is no later change.
				if err := recordRole(tx, t.chat, cur, cur, roleUndoSource(t.id)); err != nil {
					return struct{}{}, err
				}
				continue
			}
			if cur != t.after { // changed without a record
				res.Conflicts = append(res.Conflicts, UndoConflict{Item: UndoItem{RoleItem, string(t.chat)}, Reason: ConflictChanged,
					Detail: "the chat's role changed since"})
				continue
			}
			if _, err := tx.Exec(`UPDATE chats SET role = ? WHERE id = ?`, t.before, t.chat); err != nil {
				return struct{}{}, err
			}
			if err := recordRole(tx, t.chat, t.after, t.before, roleUndoSource(t.id)); err != nil {
				return struct{}{}, err
			}
			seq, err := bump(tx, t.chat)
			if err != nil {
				return struct{}{}, err
			}
			res.Seq[t.chat] = seq
		}
		if len(res.Conflicts) > 0 {
			return struct{}{}, ErrUndoConflict
		}
		if o.Check {
			return struct{}{}, errChecked
		}
		return struct{}{}, nil
	})
	if errors.Is(err, errChecked) {
		err = nil
	}
	if err != nil && !errors.Is(err, ErrUndoConflict) {
		return res, fmt.Errorf("store: undo of role changes: %w", err)
	}
	return res, err
}

// undoneRoles returns which of the role changes ids are in a chain that is
// undone now (an odd number of undos) whose last undo changed the role
// back, as undoneChains for project.db: a later change that is in effect
// no more.
func undoneRoles(tx *sql.Tx, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	js, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	got, err := queryTx(tx, `WITH RECURSIVE up(start, id, source) AS (
			SELECT id, id, source FROM role_changes WHERE id IN (SELECT value FROM json_each(?1))
			UNION ALL
			SELECT up.start, p.id, p.source FROM up JOIN role_changes p
				ON up.source LIKE 'undo:role:%' AND p.id = CAST(substr(up.source, 11) AS INTEGER)),
		roots(start, root) AS (SELECT start, id FROM up WHERE source NOT LIKE 'undo:role:%'),
		down(root, id, depth) AS (
			SELECT DISTINCT root, root, 0 FROM roots
			UNION ALL
			SELECT down.root, c.id, down.depth + 1 FROM down JOIN role_changes c ON c.source = 'undo:role:' || down.id),
		tips(root, id, depth) AS (SELECT root, id, max(depth) FROM down GROUP BY root)
		SELECT r.start FROM roots r JOIN tips t ON t.root = r.root JOIN role_changes x ON x.id = t.id
		WHERE t.depth % 2 = 1 AND x.before <> x.after`, []any{string(js)}, func(r *sql.Rows) (int64, error) {
		var n int64
		return n, r.Scan(&n)
	})
	for _, n := range got {
		out[n] = true
	}
	return out, err
}
