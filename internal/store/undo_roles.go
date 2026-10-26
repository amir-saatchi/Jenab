package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
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

// roleTip is the tip of a chain of role changes, as undoChain for
// changes. A record that keeps the role (before = after) flips nothing:
// older undos wrote one for a skipped role. So eff, the undos that
// changed the role, says whether the base change is undone (odd), and
// before and after are those of real, the chain's last record that
// changed it.
type roleTip struct {
	id, real      int64
	chat          id.Chat
	before, after string
	eff           int
	at            string // when real was written
}

func (t roleTip) undone() bool { return t.eff%2 == 1 }

func roleTips(tx *sql.Tx, srcs string) ([]roleTip, error) {
	return queryTx(tx, `WITH RECURSIVE chain(base, id, depth, eff, real) AS (
			SELECT id, id, 0, 0, id FROM role_changes WHERE source IN (SELECT value FROM json_each(?1))
			UNION ALL
			SELECT chain.base, r.id, chain.depth + 1, chain.eff + (r.before <> r.after), CASE WHEN r.before <> r.after THEN r.id ELSE chain.real END
			FROM chain JOIN role_changes r ON r.source = 'undo:role:' || chain.id)
		SELECT t.id, t.real, x.chat_id, x.before, x.after, t.eff, x.at
		FROM (SELECT base, id, real, eff, max(depth) FROM chain GROUP BY base) t JOIN role_changes x ON x.id = t.real
		ORDER BY t.id`, []any{srcs}, func(r *sql.Rows) (roleTip, error) {
		var x roleTip
		return x, r.Scan(&x.id, &x.real, &x.chat, &x.before, &x.after, &x.eff, &x.at)
	})
}

// pickRoles returns the tips an undo, or with redo a redo, reverts.
func pickRoles(tips []roleTip, redo bool) []roleTip {
	var out []roleTip
	for _, t := range tips {
		if t.undone() == redo {
			out = append(out, t)
		}
	}
	return out
}

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
// changes from sources, as UndoState for project.db. Its newest action is
// the newest record that changed a role, other than the change itself.
func (c *ChatsDB) RoleUndoState(ctx context.Context, sources []id.Source) (UndoState, error) {
	return readTx(ctx, c.DB, func(tx *sql.Tx) (UndoState, error) {
		var s UndoState
		if len(sources) == 0 {
			return s, nil
		}
		tips, err := roleTips(tx, sourcesJSON(sources))
		if err != nil {
			return s, err
		}
		s.Undo, s.Redo = rolePlan(pickRoles(tips, false), nil), rolePlan(pickRoles(tips, true), nil)
		for _, t := range tips {
			if t.eff > 0 {
				s.Last = s.Last.newer(&UndoAction{At: t.at, Redo: !t.undone()})
			}
		}
		return s, nil
	})
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
// changes cancel out (laterRoles). A skipped role is left as it is, with
// no record: it stays in effect, and a later undo tries it again. No role
// change to revert is not an error: a turn rarely changes a role.
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
			set[t.id], set[t.real] = true, true
			if it := (UndoItem{RoleItem, string(t.chat)}); skip[it] {
				if !slices.Contains(res.Skipped, it) {
					res.Skipped = append(res.Skipped, it)
				}
				continue
			}
			if f, ok := first[t.chat]; !ok || t.real < f {
				first[t.chat] = t.real
			}
		}
		if res.Conflicts, err = laterRoles(tx, first, set); err != nil {
			return struct{}{}, err
		}
		if len(res.Conflicts) > 0 {
			return struct{}{}, ErrUndoConflict
		}
		reverted := 0
		for i := len(ts) - 1; i >= 0; i-- {
			t := ts[i]
			if skip[UndoItem{RoleItem, string(t.chat)}] {
				continue
			}
			var cur string
			if err := tx.QueryRow(`SELECT role FROM chats WHERE id = ?`, t.chat).Scan(&cur); err != nil {
				return struct{}{}, err
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
			reverted++
			seq, err := bump(tx, t.chat)
			if err != nil {
				return struct{}{}, err
			}
			res.Seq[t.chat] = seq
		}
		// As Undo: the plan counts the changes reverted, none when all
		// were skipped.
		res.Plan.Changes = reverted
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

// laterRoles lists, one per chat, the role changes after first[chat]
// that aren't the undo's own (set) and that changed the role, as later
// does for project.db: the later changes of one chain don't count when
// they are even in number, or when the chain started before and is
// undone now.
func laterRoles(tx *sql.Tx, first map[id.Chat]int64, set map[int64]bool) ([]UndoConflict, error) {
	type later struct {
		chat id.Chat
		id   int64
		src  id.Source
		at   string
	}
	var ls []later
	for _, ch := range slices.Sorted(maps.Keys(first)) {
		l, err := queryTx(tx, `SELECT chat_id, id, source, at FROM role_changes
			WHERE chat_id = ? AND id > ? AND before <> after ORDER BY id`, []any{ch, first[ch]},
			func(r *sql.Rows) (later, error) {
				var l later
				return l, r.Scan(&l.chat, &l.id, &l.src, &l.at)
			})
		if err != nil {
			return nil, err
		}
		for _, x := range l {
			if !set[x.id] {
				ls = append(ls, x)
			}
		}
	}
	if len(ls) == 0 {
		return nil, nil
	}
	ids := make([]int64, len(ls))
	for i, l := range ls {
		ids[i] = l.id
	}
	chains, err := roleChains(tx, ids)
	if err != nil {
		return nil, err
	}
	count := map[int64]int{}
	started := map[int64]bool{}
	for _, l := range ls {
		root := chains[l.id].root
		count[root]++
		if l.id == root {
			started[root] = true
		}
	}
	var out []UndoConflict
	listed := map[id.Chat]bool{}
	for _, l := range ls {
		ch := chains[l.id]
		if count[ch.root]%2 == 0 || (!started[ch.root] && ch.undone) || listed[l.chat] {
			continue
		}
		listed[l.chat] = true
		x := UndoConflict{Item: UndoItem{RoleItem, string(l.chat)}, Reason: ConflictChanged, Source: l.src, Detail: "the chat's role changed since"}
		if at, err := parseTime(l.at); err == nil {
			x.At = &at
		}
		out = append(out, x)
	}
	return out, nil
}

// numberedChain is the chain an entry of a numbered log is in (role
// changes, user memory): its base entry, and
// whether the chain is undone now (roleTip, userTip).
type numberedChain struct {
	root   int64
	undone bool
}

// roleChains returns the chain of each of the role changes ids.
func roleChains(tx *sql.Tx, ids []int64) (map[int64]numberedChain, error) {
	out := map[int64]numberedChain{}
	for _, n := range ids {
		out[n] = numberedChain{root: n}
	}
	js, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	type row struct {
		start int64
		ch    numberedChain
	}
	got, err := queryTx(tx, `WITH RECURSIVE up(start, id, source) AS (
			SELECT id, id, source FROM role_changes WHERE id IN (SELECT value FROM json_each(?1))
			UNION ALL
			SELECT up.start, p.id, p.source FROM up JOIN role_changes p
				ON up.source LIKE 'undo:role:%' AND p.id = CAST(substr(up.source, 11) AS INTEGER)),
		roots(start, root) AS (SELECT start, id FROM up WHERE source NOT LIKE 'undo:role:%'),
		down(root, id, depth, eff) AS (
			SELECT DISTINCT root, root, 0, 0 FROM roots
			UNION ALL
			SELECT down.root, c.id, down.depth + 1, down.eff + (c.before <> c.after)
			FROM down JOIN role_changes c ON c.source = 'undo:role:' || down.id),
		tips(root, eff, depth) AS (SELECT root, eff, max(depth) FROM down GROUP BY root)
		SELECT r.start, r.root, t.eff % 2 = 1 FROM roots r JOIN tips t ON t.root = r.root`, []any{string(js)}, func(r *sql.Rows) (row, error) {
		var x row
		return x, r.Scan(&x.start, &x.ch.root, &x.ch.undone)
	})
	for _, x := range got {
		out[x.start] = x.ch
	}
	return out, err
}
