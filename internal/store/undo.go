package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

var (
	// ErrUndoConflict is an undo that later changes stand in the way of.
	// Nothing was changed; UndoResult.Conflicts lists them, and the caller
	// may skip them or give up (SPEC 2.6). It matches ErrConflict.
	ErrUndoConflict = fmt.Errorf("%w: later changes touch what the undo would revert", ErrConflict)
	// ErrUndoWindow is an undo of changes whose row entries the cleanup has
	// dropped, or is dropping: they are history only (SPEC 2.6).
	ErrUndoWindow = errors.New("store: the changes are past the undo window")
	// ErrNothingToUndo is an undo or a redo that finds nothing to revert.
	ErrNothingToUndo = errors.New("store: nothing to undo")
)

// UndoItem is one thing an undo reverts, as _jenab_change_rows names it:
// a row of an agent table by its primary key in SQLite's json_array form,
// or an internal target such as an object (_jenab_objects, object:<key>)
// or a chat's role (RoleItem).
type UndoItem struct {
	Table string `json:"table"`
	Key   string `json:"key"`
}

func itemOf(e undoEntry) UndoItem { return UndoItem{e.table, e.key} }

// ConflictReason says why an item stops an undo.
type ConflictReason string

const (
	ConflictChanged     ConflictReason = "changed"     // a later change touched it
	ConflictCascade     ConflictReason = "cascade"     // reverting it would make foreign keys change Reached, which the undo doesn't cover
	ConflictReferenced  ConflictReason = "referenced"  // Reached refers to it, or it refers to a row that is gone
	ConflictSchema      ConflictReason = "schema"      // its table or a column of it is gone
	ConflictConstraint  ConflictReason = "constraint"  // putting it back breaks a constraint
	ConflictUnsupported ConflictReason = "unsupported" // this kind of target can't be undone yet
)

// UndoConflict is an item the undo can't revert as things are.
type UndoConflict struct {
	Item    UndoItem       `json:"item"`
	Reason  ConflictReason `json:"reason"`
	Change  id.Change      `json:"change,omitempty"` // the later change, for ConflictChanged
	Source  id.Source      `json:"source,omitempty"` // who made it
	At      *time.Time     `json:"at,omitempty"`
	Reached *UndoItem      `json:"reached,omitempty"` // the other row, for ConflictCascade and ConflictReferenced
	Detail  string         `json:"detail,omitempty"`
}

// UndoOptions adjust an undo.
type UndoOptions struct {
	// Redo reverts the undo of the changes instead, putting them back.
	Redo bool `json:"redo"`
	// Skip lists items, from an earlier ErrUndoConflict, that the user
	// chose to leave as they are. The rest is reverted.
	Skip []UndoItem `json:"skip,omitempty"`
	// Check runs the undo and rolls it back, to see what it would do.
	Check bool `json:"check,omitempty"`
}

// TableRows is how many rows of one table an undo reverts.
type TableRows struct {
	Table string `json:"table"` // as _jenab_change_rows names it
	Rows  int    `json:"rows"`
}

// UndoPlan is what an undo or a redo reverts.
type UndoPlan struct {
	Changes int         `json:"changes"` // change entries
	Rows    []TableRows `json:"rows"`    // distinct rows per table, objects too
	Past    bool        `json:"past"`    // some are past the undo window, so it is refused
	Entries int         `json:"entries"` // row entries, as MaxUndoEntries counts them
	TooBig  bool        `json:"too_big"` // over MaxUndoEntries, so it is refused
}

// UndoState is what Undo and Redo would do now for a set of sources.
type UndoState struct {
	Undo       UndoPlan `json:"undo"` // nothing when Changes is 0
	Redo       UndoPlan `json:"redo"`
	Migrations int      `json:"migrations"` // migrations among the changes; undo leaves them (SPEC 2.6)
}

// UndoResult says what an undo did, or with ErrUndoConflict what stands in
// its way.
type UndoResult struct {
	Plan       UndoPlan       `json:"plan"`
	Reverted   []id.Change    `json:"reverted"`             // the changes reverted
	Written    []id.Change    `json:"written"`              // the undo's own entries, one per reverted change
	Skipped    []UndoItem     `json:"skipped,omitempty"`    // items left as they are
	Migrations []id.Change    `json:"migrations,omitempty"` // migrations kept
	Conflicts  []UndoConflict `json:"conflicts,omitempty"`
}

// internalNouns name the internal targets in a summary. Phase 3 and 4 add
// views and pipelines.
var internalNouns = map[string][2]string{
	"_jenab_objects": {"file", "files"},
	RoleItem:         {"chat role", "chat roles"},
	memoryTable:      {"project memory section", "project memory sections"},
	UserMemoryItem:   {"user memory section", "user memory sections"},
}

// Summary is the plan in a few words, e.g. "12 rows in btc_prices, 1 file".
func (p UndoPlan) Summary() string { return p.summary(func(s string) string { return s }) }

// Display is Summary for the UI: each table name sits in a first-strong
// bidi isolate, so a right-to-left name keeps the sentence in order. The
// model gets Summary, without them, so it doesn't copy them into SQL.
func (p UndoPlan) Display() string { return p.summary(isolate) }

func (p UndoPlan) summary(name func(string) string) string {
	var parts []string
	for _, t := range p.Rows {
		if n, ok := internalNouns[t.Table]; ok {
			parts = append(parts, plural(t.Rows, n[0], n[1]))
		} else {
			parts = append(parts, plural(t.Rows, "row", "rows")+" in "+name(t.Table))
		}
	}
	return strings.Join(parts, ", ")
}

// isolate wraps s in a first-strong bidi isolate (U+2068 ... U+2069).
func isolate(s string) string { return "⁨" + s + "⁩" }

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// undoDeferFK runs undo with PRAGMA defer_foreign_keys = ON, because rows
// on a cycle of foreign keys have no order that puts them back one by one
// (P2-02). Tests turn it off to check that the order alone works.
var undoDeferFK = true

// undoChange is a change an undo may revert: the tip of a chain that
// starts at a change from one of the sources. Each undo of a change is a
// change with the source undo:<change_id>, so the chain's length says
// whether the base change is in effect (even) or undone (odd).
type undoChange struct {
	id            id.Change
	target, op    string
	before, after sql.NullString
	depth         int
	baseOp        string
	base          [2]sql.NullString // the base change's before and after
}

func (c undoChange) migration() bool { return c.baseOp == "migrate" }

// versionEffect says what reverting c does to the object's versions: -1
// drops the version the change added, +1 brings back one an undo dropped,
// 0 leaves them. Only a save adds a version; each undo in the chain flips
// the effect of the change it reverts.
func (c undoChange) versionEffect() (int, error) {
	if c.baseOp != "save" {
		return 0, nil
	}
	before, err := parseRev(c.base[0])
	if err != nil {
		return 0, err
	}
	after, err := parseRev(c.base[1])
	if err != nil {
		return 0, err
	}
	if after == nil || (before != nil && after.Version == before.Version) {
		return 0, nil
	}
	if c.depth%2 == 1 {
		return 1, nil
	}
	return -1, nil
}

// undoEntry is a row entry of a change being reverted.
type undoEntry struct {
	rowid  int64
	change id.Change
	table  string
	key    string
	kind   string
	before sql.NullString
}

func sourcesJSON(sources []id.Source) string {
	b, _ := json.Marshal(sources)
	return string(b)
}

func idsJSON(ids []id.Change) string {
	b, _ := json.Marshal(ids)
	return string(b)
}

// undoBase picks the changes an undo starts from: those whose source is
// one of srcs, or whose ID is one of ids (JSON arrays).
type undoBase struct{ srcs, ids string }

func bySources(sources []id.Source) undoBase { return undoBase{srcs: sourcesJSON(sources), ids: "[]"} }

// undoTips returns the tip of each chain from the base, oldest first. A
// user memory edit's pointer (LogUserMemory) is left out: user memory is
// undone in registry.db.
func undoTips(tx *sql.Tx, b undoBase) ([]undoChange, error) {
	return queryTx(tx, `WITH RECURSIVE chain(base, id, depth) AS (
			SELECT id, id, 0 FROM _jenab_changes
				WHERE (source IN (SELECT value FROM json_each(?1)) OR id IN (SELECT value FROM json_each(?2)))
					AND target NOT LIKE 'memory:user/%'
			UNION ALL
			SELECT chain.base, c.id, chain.depth + 1 FROM chain JOIN _jenab_changes c ON c.source = 'undo:' || chain.id)
		SELECT t.id, c.target, c.op, c.before, c.after, t.depth, b.op, b.before, b.after
		FROM (SELECT base, id, max(depth) AS depth FROM chain GROUP BY base) t
		JOIN _jenab_changes c ON c.id = t.id JOIN _jenab_changes b ON b.id = t.base
		ORDER BY t.id`, []any{b.srcs, b.ids}, func(r *sql.Rows) (undoChange, error) {
		var c undoChange
		return c, r.Scan(&c.id, &c.target, &c.op, &c.before, &c.after, &c.depth, &c.baseOp, &c.base[0], &c.base[1])
	})
}

// cleanedTo is the cleanup's watermark: changes at or below it are past
// the undo window.
func cleanedTo(tx *sql.Tx) (string, error) {
	var mark string
	err := tx.QueryRow(`SELECT value FROM _jenab_meta WHERE key = ?`, metaCleanedTo).Scan(&mark)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return mark, err
}

// pick returns the tips an undo (redo false) or a redo reverts, and the
// migrations among them, which are never reverted.
func pick(tips []undoChange, redo bool) (out []undoChange, migrations []id.Change) {
	for _, c := range tips {
		if c.migration() {
			migrations = append(migrations, c.id)
			continue
		}
		if (c.depth%2 == 1) == redo {
			out = append(out, c)
		}
	}
	return out, migrations
}

// planOf counts the rows of cs by table. Every change counts, even one
// with no row entries, such as an update that left its rows as they were
// or an undo whose items were all skipped: reverting it still flips its
// chain, so the turn's state is the same for all its changes.
func planOf(tx *sql.Tx, cs []undoChange, mark string, skip map[UndoItem]bool) (UndoPlan, []undoChange, error) {
	var p UndoPlan
	if len(cs) == 0 {
		return p, nil, nil
	}
	ids := make([]id.Change, len(cs))
	for i, c := range cs {
		ids[i] = c.id
		if mark != "" && string(c.id) <= mark {
			p.Past = true
		}
	}
	type row struct{ table, key string }
	rows, err := queryTx(tx, `SELECT table_name, row_key FROM _jenab_change_rows
		WHERE change_id IN (SELECT value FROM json_each(?)) ORDER BY rowid`, []any{idsJSON(ids)}, func(r *sql.Rows) (row, error) {
		var x row
		return x, r.Scan(&x.table, &x.key)
	})
	if err != nil {
		return p, nil, err
	}
	p.Entries, p.TooBig = len(rows), len(rows) > maxUndoEntries
	seen := map[UndoItem]bool{}
	at := map[string]int{}
	for _, x := range rows {
		it := UndoItem{x.table, x.key}
		if seen[it] || skip[it] {
			continue
		}
		seen[it] = true
		i, ok := at[x.table]
		if !ok {
			i = len(p.Rows)
			at[x.table] = i
			p.Rows = append(p.Rows, TableRows{Table: x.table})
		}
		p.Rows[i].Rows++
	}
	p.Changes = len(cs)
	return p, cs, nil
}

// UndoState says what Undo and Redo would revert now for the changes from
// sources: the message:<id> of a turn's messages, or a run:<id>.
func (p *ProjectDB) UndoState(ctx context.Context, sources []id.Source) (UndoState, error) {
	return readTx(ctx, p.DB, func(tx *sql.Tx) (UndoState, error) {
		var s UndoState
		if len(sources) == 0 {
			return s, nil
		}
		tips, err := undoTips(tx, bySources(sources))
		if err != nil {
			return s, err
		}
		mark, err := cleanedTo(tx)
		if err != nil {
			return s, err
		}
		undo, migrations := pick(tips, false)
		redo, _ := pick(tips, true)
		s.Migrations = len(migrations)
		if s.Undo, _, err = planOf(tx, undo, mark, nil); err != nil {
			return s, err
		}
		s.Redo, _, err = planOf(tx, redo, mark, nil)
		return s, err
	})
}

// RevertRun is Undo for one pipeline run (SPEC 2.6). Its button comes with
// the pipeline page (Phase 4).
func (p *ProjectDB) RevertRun(ctx context.Context, run id.Run, o UndoOptions) (UndoResult, error) {
	return p.Undo(ctx, []id.Source{id.SourceOf("run", string(run))}, o)
}

// Undo reverts every change from sources, the message:<id> of a turn's
// messages or a run:<id> (SPEC 2.6), as one interactive write request in
// one transaction, so a crash leaves the whole undo or none of it.
//
//   - Row entries are reverted newest first, by their rowid, never by
//     change: the chunks of different requests take turns. An inserted row
//     is deleted, an updated or deleted one gets its before-image back.
//     Objects return to their revision before the change.
//   - The writes go the typed way, so foreign-key actions are followed and
//     logged, with PRAGMA defer_foreign_keys = ON. The object_ref check is
//     skipped: the row held the key once.
//   - Conflicts stop it before anything is kept: a later change to an
//     item, an action that would reach a row outside the undo, a row
//     outside it that still refers to a row it removes, a row it puts back
//     whose parent is gone, a table or column that is gone, a constraint.
//     The result lists them with ErrUndoConflict; o.Skip leaves them out.
//   - Changes past the cleanup's watermark are refused (ErrUndoWindow).
//   - Migrations are left as they are and listed (SPEC 2.6).
//   - Each reverted change gets its own entry with the source
//     undo:<change_id>, so Undo with o.Redo puts it back, and undoing again
//     after a redo reverts the redo.
func (p *ProjectDB) Undo(ctx context.Context, sources []id.Source, o UndoOptions) (UndoResult, error) {
	if len(sources) == 0 {
		return UndoResult{}, ErrNothingToUndo
	}
	for _, s := range sources {
		if err := checkSource(s); err != nil {
			return UndoResult{}, err
		}
	}
	var res UndoResult // kept outside, so a rolled-back undo still reports
	_, err := Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		res = UndoResult{}
		err := undoTx(tx, bySources(sources), o, &res)
		if err == nil && o.Check {
			err = errChecked
		}
		return struct{}{}, err
	})
	if errors.Is(err, errChecked) {
		err = nil
	}
	return res, err
}

// UndoChange is Undo for one change, by its ID, such as a memory edit from
// its chip (SPEC 3.3).
func (p *ProjectDB) UndoChange(ctx context.Context, c id.Change, o UndoOptions) (UndoResult, error) {
	var res UndoResult
	_, err := Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		res = UndoResult{}
		b, _ := json.Marshal([]id.Change{c})
		err := undoTx(tx, undoBase{srcs: "[]", ids: string(b)}, o, &res)
		if err == nil && o.Check {
			err = errChecked
		}
		return struct{}{}, err
	})
	if errors.Is(err, errChecked) {
		err = nil
	}
	return res, err
}

// errChecked rolls back an undo run with Check.
var errChecked = errors.New("store: undo checked")

func undoTx(tx *sql.Tx, b undoBase, o UndoOptions, res *UndoResult) error {
	if undoDeferFK {
		if _, err := tx.Exec("PRAGMA defer_foreign_keys = ON"); err != nil { // ends with the transaction
			return err
		}
	}
	tips, err := undoTips(tx, b)
	if err != nil {
		return err
	}
	mark, err := cleanedTo(tx)
	if err != nil {
		return err
	}
	cs, migrations := pick(tips, o.Redo)
	if !o.Redo {
		res.Migrations = migrations
	}
	skip := map[UndoItem]bool{}
	for _, it := range o.Skip {
		skip[it] = true
	}
	res.Plan, cs, err = planOf(tx, cs, mark, skip)
	if err != nil {
		return err
	}
	if res.Plan.Past {
		return fault("%w", ErrUndoWindow)
	}
	if res.Plan.TooBig {
		return &UndoTooBigError{Rows: res.Plan.Entries, Max: maxUndoEntries}
	}
	if len(cs) == 0 {
		return fault("%w", ErrNothingToUndo)
	}
	u := &undoRun{tx: tx, skip: skip, set: map[UndoItem]bool{}, changes: map[id.Change]*undoChange{},
		written: map[id.Change]*change{}, done: map[id.Change]bool{}, first: map[id.Change]int64{}}
	ids := make([]id.Change, len(cs))
	for i := range cs {
		ids[i] = cs[i].id
		u.changes[cs[i].id] = &cs[i]
	}
	es, err := queryTx(tx, `SELECT rowid, change_id, table_name, row_key, kind, before FROM _jenab_change_rows
		WHERE change_id IN (SELECT value FROM json_each(?)) ORDER BY rowid DESC`, []any{idsJSON(ids)}, func(r *sql.Rows) (undoEntry, error) {
		var e undoEntry
		return e, r.Scan(&e.rowid, &e.change, &e.table, &e.key, &e.kind, &e.before)
	})
	if err != nil {
		return err
	}
	if widen(skip, es, u.changes) {
		if res.Plan, cs, err = planOf(tx, cs, mark, skip); err != nil {
			return err
		}
	}
	for _, e := range es {
		if f, ok := u.first[e.change]; !ok || e.rowid < f {
			u.first[e.change] = e.rowid
		}
		if it := itemOf(e); skip[it] {
			if !slices.Contains(res.Skipped, it) {
				res.Skipped = append(res.Skipped, it)
			}
		} else {
			u.set[it] = true
		}
	}
	if err := u.later(ids); err != nil {
		return err
	}
	if err := u.revert(es); err != nil {
		return err
	}
	if err := u.checkKeys(); err != nil {
		return err
	}
	if len(u.conflicts) > 0 {
		res.Conflicts = u.conflicts
		return ErrUndoConflict
	}
	// Every change gets its entry, even one that had nothing left to
	// revert, so its chain flips with the others.
	for _, c := range cs {
		w := u.entry(c.id)
		if err := writeChange(tx, *w, nil); err != nil {
			return err
		}
		res.Reverted = append(res.Reverted, c.id)
		res.Written = append(res.Written, w.id)
	}
	return nil
}

// MaxUndoEntries is the most row entries one undo reverts. An undo is one
// transaction, so a crash leaves all of it or none, and it holds the
// writer while it runs. Past the page cache, SQLite writes pages out
// mid-transaction, so the time grows faster than the rows and depends on
// the disk: on a 100,000-row table (TestUndoTooBig, JENAB_UNDO_ROWS), a
// 10,000-row turn took 0.7 s to undo and 1.0 s to redo, 25,000 rows 2.1 s
// and 3.3 s, 50,000 rows 2 to 6 s and 2.5 to 27 s under load. 20,000
// keeps one undo near 2 s. Larger undos are refused; Phase 4's Revert run
// gets a chunked, resumable form.
const MaxUndoEntries = 20_000

var maxUndoEntries = MaxUndoEntries // tests lower it

// ErrUndoTooBig matches an UndoTooBigError.
var ErrUndoTooBig = errors.New("store: too many rows to undo at once")

// UndoTooBigError is an undo of more than MaxUndoEntries row entries.
type UndoTooBigError struct{ Rows, Max int }

func (e *UndoTooBigError) Error() string {
	return fmt.Sprintf("this turn changed %d rows; undo can revert at most %d at once", e.Rows, e.Max)
}

func (e *UndoTooBigError) Is(target error) bool { return target == ErrUndoTooBig }

// widen adds to skip what must be skipped with what the user chose: every
// row of a table skipped as a whole ({table, ""}, a later migration's
// conflict), and every item of an object change one of whose items is
// skipped, since a move and the rows that point to the object go back
// together or not at all. One feeds the other, so it runs until skip
// stops growing. It reports whether skip grew.
func widen(skip map[UndoItem]bool, es []undoEntry, cs map[id.Change]*undoChange) bool {
	if len(skip) == 0 {
		return false
	}
	grew := false
	for {
		more := false
		whole := map[id.Change]bool{}
		for _, e := range es {
			it := itemOf(e)
			if !skip[it] && skip[UndoItem{Table: e.table}] {
				skip[it], more = true, true
			}
			if skip[it] && strings.HasPrefix(cs[e.change].target, "object:") {
				whole[e.change] = true
			}
		}
		for _, e := range es {
			if it := itemOf(e); !skip[it] && whole[e.change] {
				skip[it], more = true, true
			}
		}
		if !more {
			return grew
		}
		grew = true
	}
}

// undoRun is one undo inside its transaction.
type undoRun struct {
	tx        *sql.Tx
	set       map[UndoItem]bool // the items of the reverted changes, less the skipped ones
	skip      map[UndoItem]bool
	changes   map[id.Change]*undoChange
	written   map[id.Change]*change // the undo's entry for each reverted change
	done      map[id.Change]bool    // internal targets reverted once per change
	first     map[id.Change]int64   // the rowid of each reverted change's first row entry
	logged    []rowEntry            // the undo's own row entries, for checkKeys
	conflicts []UndoConflict
}

func (u *undoRun) conflict(c UndoConflict) {
	for _, x := range u.conflicts {
		if x.Item == c.Item && x.Reason == c.Reason && x.Change == c.Change && (x.Reached == nil) == (c.Reached == nil) &&
			(x.Reached == nil || *x.Reached == *c.Reached) {
			return
		}
	}
	u.conflicts = append(u.conflicts, c)
}

// has reports whether a conflict is listed for it.
func (u *undoRun) has(it UndoItem) bool {
	for _, x := range u.conflicts {
		if x.Item == it {
			return true
		}
	}
	return false
}

// later finds the changes outside the undo that touched one of its items
// after the undo's first entry for it: one index search per item (SPEC
// 2.5). A change whose chain is undone now (an odd number of undos), and
// whose last undo put the item back, is in effect for it no more, so
// neither it nor its undos count: turns undone newest first don't stand
// in each other's way. A chain undone with the item skipped still counts,
// and so does a redone chain. A later migration of a table, which logs no
// rows, stands in the way of every row of the table (migrations).
func (u *undoRun) later(ids []id.Change) error {
	type hit struct {
		item   UndoItem
		change id.Change
		source id.Source
		at     string
	}
	hits, err := queryTx(u.tx, `WITH mine AS (SELECT table_name, row_key, min(rowid) AS first FROM _jenab_change_rows
			WHERE change_id IN (SELECT value FROM json_each(?1)) GROUP BY table_name, row_key)
		SELECT DISTINCT m.table_name, m.row_key, c.id, c.source, c.at FROM mine m
		JOIN _jenab_change_rows r ON r.table_name = m.table_name AND r.row_key = m.row_key AND r.rowid > m.first
		JOIN _jenab_changes c ON c.id = r.change_id
		WHERE r.change_id NOT IN (SELECT value FROM json_each(?1))
		ORDER BY c.id`, []any{idsJSON(ids)}, func(r *sql.Rows) (hit, error) {
		var h hit
		return h, r.Scan(&h.item.Table, &h.item.Key, &h.change, &h.source, &h.at)
	})
	if err != nil {
		return err
	}
	var found []id.Change
	for _, h := range hits {
		if u.set[h.item] && !slices.Contains(found, h.change) {
			found = append(found, h.change)
		}
	}
	tips, err := undoneChains(u.tx, found)
	if err != nil {
		return err
	}
	type put struct {
		tip  id.Change
		item UndoItem
	}
	var ask []put
	for _, h := range hits {
		if tip, ok := tips[h.change]; ok && u.set[h.item] {
			ask = append(ask, put{tip, h.item})
		}
	}
	back := map[put]bool{}
	if len(ask) > 0 {
		js := make([][3]string, len(ask))
		for i, a := range ask {
			js[i] = [3]string{string(a.tip), a.item.Table, a.item.Key}
		}
		arg, err := json.Marshal(js)
		if err != nil {
			return err
		}
		got, err := queryTx(u.tx, `SELECT j.value ->> 0, j.value ->> 1, j.value ->> 2 FROM json_each(?) j
			WHERE EXISTS (SELECT 1 FROM _jenab_change_rows r WHERE r.table_name = j.value ->> 1
				AND r.row_key = j.value ->> 2 AND r.change_id = j.value ->> 0)`, []any{string(arg)}, func(r *sql.Rows) (put, error) {
			var p put
			return p, r.Scan(&p.tip, &p.item.Table, &p.item.Key)
		})
		if err != nil {
			return err
		}
		for _, p := range got {
			back[p] = true
		}
	}
	for _, h := range hits {
		if !u.set[h.item] {
			continue
		}
		if tip, ok := tips[h.change]; ok && back[put{tip, h.item}] {
			continue
		}
		c := UndoConflict{Item: h.item, Reason: ConflictChanged, Change: h.change, Source: h.source}
		if t, err := parseTime(h.at); err == nil {
			c.At = &t
		}
		u.conflict(c)
	}
	return u.migrations(ids)
}

// migrations lists a migration after a reverted change of a table as a
// schema conflict of the whole table ({table, ""}): it logs no rows, so
// the row entries can't tell what it changed, and an undo would write
// over it. Each migration leaves a marker entry per table it changes
// (rowMigrated), so "after" is the commit order, as in later. Skipping
// the item skips the table's rows (widen). Migrations are never undone,
// so all of them count.
func (u *undoRun) migrations(ids []id.Change) error {
	type hit struct {
		table  string
		change id.Change
		source id.Source
		at     string
	}
	hits, err := queryTx(u.tx, `WITH mine AS (SELECT table_name, min(rowid) AS first FROM _jenab_change_rows
			WHERE change_id IN (SELECT value FROM json_each(?1)) GROUP BY table_name)
		SELECT DISTINCT m.table_name, c.id, c.source, c.at FROM mine m
		JOIN _jenab_change_rows r ON r.table_name = m.table_name AND r.row_key = '' AND r.rowid > m.first
		JOIN _jenab_changes c ON c.id = r.change_id
		WHERE r.kind = '`+rowMigrated+`' AND r.change_id NOT IN (SELECT value FROM json_each(?1))
		ORDER BY c.id`, []any{idsJSON(ids)}, func(r *sql.Rows) (hit, error) {
		var h hit
		return h, r.Scan(&h.table, &h.change, &h.source, &h.at)
	})
	if err != nil {
		return err
	}
	live := map[string]bool{}
	for it := range u.set {
		live[it.Table] = true
	}
	for _, h := range hits {
		if !live[h.table] {
			continue
		}
		c := UndoConflict{Item: UndoItem{Table: h.table}, Reason: ConflictSchema, Change: h.change, Source: h.source,
			Detail: "a later migration changed the table"}
		if t, err := parseTime(h.at); err == nil {
			c.At = &t
		}
		u.conflict(c)
	}
	return nil
}

// undoneChains returns which of cs are in a chain that is undone now,
// with the chain's tip, its last undo: the chain's base change and each
// undo in it (source undo:<id>), when the chain has an odd number of
// undos.
func undoneChains(tx *sql.Tx, cs []id.Change) (map[id.Change]id.Change, error) {
	out := map[id.Change]id.Change{}
	if len(cs) == 0 {
		return out, nil
	}
	type pair struct{ start, tip id.Change }
	ps, err := queryTx(tx, `WITH RECURSIVE up(start, id, source) AS (
			SELECT id, id, source FROM _jenab_changes WHERE id IN (SELECT value FROM json_each(?1))
			UNION ALL
			SELECT up.start, p.id, p.source FROM up JOIN _jenab_changes p
				ON up.source LIKE 'undo:%' AND p.id = substr(up.source, 6)),
		roots(start, root) AS (SELECT start, id FROM up WHERE source NOT LIKE 'undo:%'),
		down(root, id, depth) AS (
			SELECT DISTINCT root, root, 0 FROM roots
			UNION ALL
			SELECT down.root, c.id, down.depth + 1 FROM down JOIN _jenab_changes c ON c.source = 'undo:' || down.id),
		tips(root, id, depth) AS (SELECT root, id, max(depth) FROM down GROUP BY root)
		SELECT r.start, t.id FROM roots r JOIN tips t ON t.root = r.root
		WHERE t.depth % 2 = 1`, []any{idsJSON(cs)}, func(r *sql.Rows) (pair, error) {
		var p pair
		return p, r.Scan(&p.start, &p.tip)
	})
	for _, p := range ps {
		out[p.start] = p.tip
	}
	return out, err
}

// log writes row entries of the undo of change c, in the order the undo
// made them, so a redo going backwards by rowid replays them right.
func (u *undoRun) log(c id.Change, rows []rowEntry) error {
	if len(rows) == 0 {
		return nil
	}
	w := u.entry(c)
	if err := writeRowEntries(u.tx, w.id, rows); err != nil {
		return err
	}
	u.logged = append(u.logged, rows...)
	return nil
}

// entry is the undo's change entry for the reverted change c.
func (u *undoRun) entry(c id.Change) *change {
	w := u.written[c]
	if w == nil {
		nc := newChange(id.SourceOf("undo", string(c)), u.changes[c].target, "undo")
		w = &nc
		u.written[c] = w
	}
	return w
}

// Undo actions on an agent table's rows.
const (
	actRemove = iota // the change inserted the row: delete it
	actPut           // it updated or deleted the row: put the before-image back
)

func actionOf(kind string) int {
	if kind == rowInserted {
		return actRemove
	}
	return actPut
}

// revert goes through the entries, newest first, in batches of up to
// chunkSize entries of one change, one table and one action.
func (u *undoRun) revert(es []undoEntry) error {
	for i := 0; i < len(es); {
		e := es[i]
		if u.skip[itemOf(e)] {
			i++
			continue
		}
		if internalName(e.table) {
			if err := u.revertInternal(e, es); err != nil {
				return err
			}
			i++
			continue
		}
		act := actionOf(e.kind)
		batch := []undoEntry{e}
		keys := map[string]bool{e.key: true}
		j := i + 1
		for ; j < len(es) && len(batch) < chunkSize; j++ {
			n := es[j]
			if n.change != e.change || n.table != e.table || actionOf(n.kind) != act || keys[n.key] {
				break
			}
			if u.skip[itemOf(n)] {
				continue
			}
			keys[n.key] = true
			batch = append(batch, n)
		}
		i = j
		tab, err := loadTable(u.tx, e.table)
		if err != nil {
			if !errors.Is(err, ErrInvalid) {
				return err
			}
			for _, b := range batch {
				u.conflict(UndoConflict{Item: itemOf(b), Reason: ConflictSchema, Detail: err.Error()})
			}
			continue
		}
		if err := u.batch(tab, act, batch); err != nil {
			return err
		}
	}
	return nil
}

// batch runs one batch inside a savepoint. If it fails on the request's
// own terms or reaches outside the undo, it is rolled back and its entries
// run one by one, so each conflict is pinned on its item and the rest
// still runs, and all conflicts are found in one go.
func (u *undoRun) batch(tab *agentTable, act int, es []undoEntry) error {
	var ready []undoEntry
	if act == actPut {
		for _, e := range es {
			if err := checkBefore(tab, e); err != nil {
				u.conflict(UndoConflict{Item: itemOf(e), Reason: ConflictSchema, Detail: err.Error()})
				continue
			}
			ready = append(ready, e)
		}
		if len(ready) == 0 {
			return nil
		}
		es = ready
	}
	if _, err := u.tx.Exec("SAVEPOINT jenab_undo"); err != nil {
		return err
	}
	var l *chunkLog
	var cs []UndoConflict
	var err error
	if act == actRemove {
		l, err = u.remove(tab, es)
	} else {
		l, err = u.put(tab, es)
	}
	if err == nil {
		cs = u.reached(l, es)
	}
	if err == nil && len(cs) == 0 {
		if err := u.log(es[0].change, append(l.cascade, l.rows...)); err != nil {
			return err
		}
		_, err := u.tx.Exec("RELEASE jenab_undo")
		return err
	}
	if _, rerr := u.tx.Exec("ROLLBACK TO jenab_undo"); rerr != nil {
		return errors.Join(err, rerr)
	}
	if _, rerr := u.tx.Exec("RELEASE jenab_undo"); rerr != nil {
		return errors.Join(err, rerr)
	}
	if err != nil && !errors.Is(err, ErrInvalid) {
		return err
	}
	if len(es) > 1 {
		for _, e := range es {
			if err := u.batch(tab, act, []undoEntry{e}); err != nil {
				return err
			}
		}
		return nil
	}
	if err != nil {
		cs = append(cs, UndoConflict{Item: itemOf(es[0]), Reason: ConflictConstraint, Detail: err.Error()})
	}
	for _, c := range cs {
		u.conflict(c)
	}
	return nil
}

// reached returns the rows foreign-key actions changed that are not items
// of the undo, pinned on the batch's first entry; batch runs a batch of
// more than one again row by row to pin them right.
func (u *undoRun) reached(l *chunkLog, es []undoEntry) []UndoConflict {
	var out []UndoConflict
	for _, e := range l.cascade {
		it := UndoItem{e.table, e.key}
		if u.set[it] {
			continue
		}
		out = append(out, UndoConflict{Item: itemOf(es[0]), Reason: ConflictCascade, Reached: &it})
	}
	return out
}

// remove deletes the rows a change inserted, as Delete does.
func (u *undoRun) remove(tab *agentTable, es []undoEntry) (*chunkLog, error) {
	l := &chunkLog{seen: map[string]bool{}}
	keys := make([]string, len(es))
	for i, e := range es {
		keys[i] = e.key
	}
	kj := "[" + strings.Join(keys, ",") + "]"
	cas, err := reach(u.tx, tab, root{kj, nil})
	if err != nil {
		return nil, err
	}
	var before map[string]string
	if cas != nil {
		if before, err = tab.befores(u.tx, kj); err != nil {
			return nil, err
		}
	}
	gone, err := queryTx(u.tx, fmt.Sprintf("DELETE FROM %s WHERE %s RETURNING %s, %s", quote(tab.name), tab.keyMatch(),
		tab.keyExpr(), tab.beforeExpr()), []any{kj}, func(r *sql.Rows) ([2]string, error) {
		var kb [2]string
		return kb, r.Scan(&kb[0], &kb[1])
	})
	if err != nil {
		return nil, checkErr(err)
	}
	for _, kb := range gone {
		if b, ok := before[kb[0]]; ok {
			kb[1] = b
		}
		l.add(tab.name, kb[0], rowDeleted, sql.NullString{String: kb[1], Valid: true})
		delete(before, kb[0])
	}
	for _, k := range keys { // deleted by an action before the statement reached it
		if b, ok := before[k]; ok {
			l.add(tab.name, k, rowDeleted, sql.NullString{String: b, Valid: true})
		}
	}
	return l, cas.log(u.tx, l)
}

// checkBefore checks that e's before-image fits the table as it is now.
func checkBefore(tab *agentTable, e undoEntry) error {
	if !e.before.Valid {
		return fmt.Errorf("the entry of %s %s has no before-image", tab.name, e.key)
	}
	row, err := decodeRow(e.before.String)
	if err != nil {
		return err
	}
	for c := range row {
		if _, err := tab.column(c); err != nil {
			return fmt.Errorf("table %s no longer has the column %s", tab.name, c)
		}
	}
	for _, c := range tab.pk {
		if _, ok := row[c]; !ok {
			return fmt.Errorf("the key column %s of %s is not in the before-image", c, tab.name)
		}
	}
	return nil
}

// decodeRow reads a before-image: a JSON object of the row's columns, with
// a BLOB as {"$blob": "<hex>"} (P2-02).
func decodeRow(s string) (map[string]any, error) {
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	var row map[string]any
	if err := d.Decode(&row); err != nil {
		return nil, fmt.Errorf("store: a before-image that is not a JSON object: %w", err)
	}
	for c, v := range row {
		switch x := v.(type) {
		case nil, string:
		case json.Number:
			n, err := value(x)
			if f, _ := x.Float64(); err != nil && math.IsInf(f, 0) {
				// SQLite stores ±Inf as a REAL, and its JSON writes it as
				// ±9.0e+999, which overflows: keep it. Tool input stays
				// strict (value).
				n, err = f, nil
			}
			if err != nil {
				return nil, fmt.Errorf("store: column %s: %w", c, err)
			}
			row[c] = n
		case map[string]any:
			h, ok := x["$blob"].(string)
			if !ok || len(x) != 1 {
				return nil, fmt.Errorf("store: column %s holds an object that is not a stored BLOB", c)
			}
			b, err := hex.DecodeString(h)
			if err != nil {
				return nil, fmt.Errorf("store: column %s: %w", c, err)
			}
			if b == nil {
				b = []byte{}
			}
			row[c] = b
		default:
			return nil, fmt.Errorf("store: column %s holds a %T", c, v)
		}
	}
	return row, nil
}

// put gives rows their before-images back with an upsert of every column
// the image has; columns added since keep their values, or their defaults
// for a row that comes back. A row already as it was is left alone.
func (u *undoRun) put(tab *agentTable, es []undoEntry) (*chunkLog, error) {
	l := &chunkLog{seen: map[string]bool{}}
	keys := make([]string, len(es))
	for i, e := range es {
		keys[i] = e.key
	}
	cur, err := tab.befores(u.tx, "["+strings.Join(keys, ",")+"]")
	if err != nil {
		return nil, err
	}
	type item struct {
		e   undoEntry
		row map[string]any
	}
	var items []item
	for _, e := range es {
		if b, ok := cur[e.key]; ok && b == e.before.String {
			continue
		}
		raw, err := decodeRow(e.before.String)
		if err != nil {
			return nil, err
		}
		row := make(map[string]any, len(raw))
		for c, v := range raw {
			n, err := tab.column(c)
			if err != nil {
				return nil, err
			}
			row[n] = v
		}
		items = append(items, item{e, row})
	}
	if len(items) == 0 {
		return l, nil
	}
	// The rows that are there get an update, which foreign keys may pass on.
	var set []string
	for _, c := range tab.cols {
		if !tab.isKey(c) && slices.ContainsFunc(items, func(it item) bool { _, ok := it.row[c]; return ok }) {
			set = append(set, c)
		}
	}
	var gs []given
	for _, it := range items {
		if _, ok := cur[it.e.key]; !ok {
			continue
		}
		g := given{Key: json.RawMessage(it.e.key), Vals: make([]any, len(set)), Has: make([]bool, len(set))}
		for i, c := range set {
			g.Vals[i], g.Has[i] = it.row[c]
		}
		gs = append(gs, g)
	}
	roots, err := tab.changing(u.tx, set, gs)
	if err != nil {
		return nil, err
	}
	cas, err := reach(u.tx, tab, roots...)
	if err != nil {
		return nil, err
	}
	stmts := map[string]*sql.Stmt{}
	defer func() {
		for _, s := range stmts {
			s.Close()
		}
	}()
	for _, it := range items {
		cols := make([]string, 0, len(it.row))
		for _, c := range tab.cols {
			if _, ok := it.row[c]; ok {
				cols = append(cols, c)
			}
		}
		sig := strings.Join(cols, "\x00")
		st := stmts[sig]
		if st == nil {
			if st, err = u.tx.Prepare(tab.insertSQL(cols, ConflictUpsert)); err != nil {
				return nil, err
			}
			stmts[sig] = st
		}
		args := make([]any, len(cols))
		for i, c := range cols {
			args[i] = it.row[c]
		}
		var key string
		err := st.QueryRow(args...).Scan(&key)
		switch {
		case errors.Is(err, sql.ErrNoRows): // a key-only row that is there
			continue
		case err != nil:
			return nil, fmt.Errorf("store: %s %s: %w", tab.name, it.e.key, checkErr(err))
		}
		if b, ok := cur[key]; ok {
			l.add(tab.name, key, rowUpdated, sql.NullString{String: b, Valid: true})
		} else {
			l.add(tab.name, key, rowInserted, sql.NullString{})
		}
	}
	return l, cas.log(u.tx, l)
}

// ---- internal targets ----

// reverter puts an internal target back to its revision before change c,
// and logs what it did with u.log. It runs once per change, at the newest
// of the change's entries for the target. P2-09 adds memory sections.
type reverter func(u *undoRun, c *undoChange) error

var reverters = map[string]reverter{
	"_jenab_objects": (*undoRun).revertObject,
	memoryTable:      (*undoRun).revertMemory,
}

func (u *undoRun) revertInternal(e undoEntry, es []undoEntry) error {
	rev := reverters[e.table]
	if rev == nil {
		u.conflict(UndoConflict{Item: itemOf(e), Reason: ConflictUnsupported, Detail: e.table + " can't be undone yet"})
		return nil
	}
	if u.done[e.change] {
		return nil
	}
	u.done[e.change] = true
	// A change whose items for this target are partly skipped is skipped.
	for _, o := range es {
		if o.change == e.change && o.table == e.table && u.skip[itemOf(o)] {
			return nil
		}
	}
	return rev(u, u.changes[e.change])
}

// objectRev is an object revision as objectRevision writes it.
type objectRev struct {
	Key       string          `json:"key"`
	SHA256    string          `json:"sha256"`
	MIME      string          `json:"mime"`
	Size      int64           `json:"size"`
	Version   int             `json:"version"`
	Metadata  json.RawMessage `json:"metadata"`
	Source    string          `json:"source"`
	SourceURL string          `json:"source_url"`
	CreatedBy string          `json:"created_by"`
	UpdatedAt string          `json:"updated_at"`
}

func parseRev(s sql.NullString) (*objectRev, error) {
	if !s.Valid {
		return nil, nil
	}
	var r objectRev
	if err := json.Unmarshal([]byte(s.String), &r); err != nil {
		return nil, fmt.Errorf("store: an object revision: %w", err)
	}
	if len(r.Metadata) == 0 {
		r.Metadata = json.RawMessage("{}")
	}
	return &r, nil
}

// revertObject puts an object back from its revision after the change
// (now) to the one before it (want): a put is taken back, a deleted object
// returns, a moved one moves back with its versions. A version the change
// added is dropped, and one an earlier undo dropped comes back; its bytes
// are kept while a change inside the undo window points to them
// (SweepObjects).
func (u *undoRun) revertObject(c *undoChange) error {
	now, err := parseRev(c.after)
	if err != nil {
		return err
	}
	want, err := parseRev(c.before)
	if err != nil {
		return err
	}
	if now == nil && want == nil {
		return nil
	}
	item := UndoItem{"_jenab_objects", c.target}
	// The log's conflict check covers changes; this covers the state.
	if now != nil {
		cur, err := objectIn(u.tx, now.Key)
		if err != nil {
			return err
		}
		if cur == nil || cur.SHA256 != now.SHA256 || cur.Version != now.Version {
			// later has it already when a logged change did it.
			if it := (UndoItem{"_jenab_objects", objectTarget(now.Key)}); !u.has(it) {
				u.conflict(UndoConflict{Item: it, Reason: ConflictChanged, Detail: "the object changed since"})
			}
			return nil
		}
	}
	if want != nil && (now == nil || want.Key != now.Key) {
		cur, err := objectIn(u.tx, want.Key)
		if err != nil {
			return err
		}
		if cur != nil {
			if it := (UndoItem{"_jenab_objects", objectTarget(want.Key)}); !u.has(it) {
				u.conflict(UndoConflict{Item: it, Reason: ConflictChanged, Detail: "another object has the key now"})
			}
			return nil
		}
	}
	if now != nil && (want == nil || want.Key != now.Key) {
		// Rows outside the undo that a later change made hold the key would
		// point to nothing. Rows that held it before the change are left,
		// as DeleteObject leaves them (SPEC 4.3).
		refs, err := refsTo(u.tx, now.Key)
		if err != nil {
			return err
		}
		n := len(u.conflicts)
		for _, r := range refs {
			if u.set[r] {
				continue
			}
			var last sql.NullInt64
			if err := u.tx.QueryRow(`SELECT max(rowid) FROM _jenab_change_rows WHERE table_name = ? AND row_key = ?`, r.Table, r.Key).Scan(&last); err != nil {
				return err
			}
			if last.Valid && last.Int64 > u.first[c.id] {
				u.conflict(UndoConflict{Item: item, Reason: ConflictReferenced, Reached: &r})
			}
		}
		if len(u.conflicts) > n {
			return nil
		}
	}
	tx := u.tx
	var rows []rowEntry
	switch {
	case now != nil && want != nil && now.Key != want.Key: // a move
		shift := want.Version - now.Version
		if _, err := tx.Exec(`UPDATE _jenab_object_versions SET key = ?, version = version + ? WHERE key = ? AND version > ?`,
			want.Key, shift, now.Key, max(0, -shift)); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM _jenab_objects WHERE key = ?`, now.Key); err != nil {
			return err
		}
		rows = []rowEntry{
			{table: "_jenab_objects", key: objectTarget(now.Key), kind: rowDeleted, before: c.after},
			{table: "_jenab_objects", key: objectTarget(want.Key), kind: rowInserted},
		}
	default:
		eff, err := c.versionEffect()
		if err != nil {
			return err
		}
		switch {
		case eff < 0 && now != nil:
			if _, err := tx.Exec(`DELETE FROM _jenab_object_versions WHERE key = ? AND version = ? AND sha256 = ?`,
				now.Key, now.Version, now.SHA256); err != nil {
				return err
			}
		case eff > 0 && want != nil:
			if _, err := tx.Exec(`INSERT INTO _jenab_object_versions (key, version, sha256, size, created_by, created_at)
				VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`, want.Key, want.Version, want.SHA256, want.Size, want.CreatedBy, want.UpdatedAt); err != nil {
				return err
			}
		}
		switch {
		case want == nil:
			if _, err := tx.Exec(`DELETE FROM _jenab_objects WHERE key = ?`, now.Key); err != nil {
				return err
			}
			rows = []rowEntry{{table: "_jenab_objects", key: objectTarget(now.Key), kind: rowDeleted, before: c.after}}
		case now == nil:
			rows = []rowEntry{{table: "_jenab_objects", key: objectTarget(want.Key), kind: rowInserted}}
		default:
			rows = []rowEntry{{table: "_jenab_objects", key: objectTarget(want.Key), kind: rowUpdated, before: c.after}}
		}
	}
	if want != nil {
		if _, err := tx.Exec(`INSERT INTO _jenab_objects (key, sha256, mime, size, metadata, source, source_url, created_by, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (key) DO UPDATE SET sha256 = excluded.sha256, mime = excluded.mime, size = excluded.size,
				metadata = excluded.metadata, source = excluded.source, source_url = excluded.source_url,
				created_by = excluded.created_by, updated_at = excluded.updated_at`,
			want.Key, want.SHA256, want.MIME, want.Size, string(want.Metadata), want.Source, want.SourceURL, want.CreatedBy, want.UpdatedAt); err != nil {
			return err
		}
	}
	if err := u.log(c.id, rows); err != nil {
		return err
	}
	w := u.written[c.id]
	w.before, w.after = c.after, c.before
	return nil
}

// refsTo returns the rows whose object_ref columns hold key (SPEC 4.5).
func refsTo(tx *sql.Tx, key string) ([]UndoItem, error) {
	tables, err := queryTx(tx, `SELECT DISTINCT table_name FROM _jenab_columns WHERE kind = 'object_ref' ORDER BY table_name`, nil,
		func(r *sql.Rows) (string, error) {
			var t string
			return t, r.Scan(&t)
		})
	if err != nil {
		return nil, err
	}
	var out []UndoItem
	for _, name := range tables {
		t, err := loadTable(tx, name)
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrNotAgentTable) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if len(t.refs) == 0 {
			continue
		}
		where := make([]string, len(t.refs))
		args := make([]any, len(t.refs))
		for i, c := range t.refs {
			where[i], args[i] = quote(c)+" = ?", key
		}
		keys, err := queryTx(tx, fmt.Sprintf("SELECT %s FROM %s WHERE %s", t.keyExpr(), quote(t.name), strings.Join(where, " OR ")), args,
			func(r *sql.Rows) (string, error) {
				var k string
				return k, r.Scan(&k)
			})
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			out = append(out, UndoItem{t.name, k})
		}
	}
	return out, nil
}

// ---- foreign keys at the end ----

// checkKeys finds, before commit, what the deferred foreign keys would
// fail on, so it is a conflict with its item and not a failed commit: a
// row the undo put back or changed whose parent is gone, and a row that
// refers to a value the undo removed. Only the rows the undo wrote are
// looked at, not whole tables.
func (u *undoRun) checkKeys() error {
	type rows struct {
		live []string   // keys of rows the undo put back or changed
		gone []rowEntry // rows it deleted or changed, with their before-images
	}
	by := map[string]*rows{}
	var order []string
	for _, e := range u.logged {
		if internalName(e.table) {
			continue
		}
		r := by[e.table]
		if r == nil {
			r = &rows{}
			by[e.table] = r
			order = append(order, e.table)
		}
		if e.kind != rowDeleted {
			r.live = append(r.live, e.key)
		}
		if e.kind != rowInserted && e.before.Valid {
			r.gone = append(r.gone, e)
		}
	}
	for _, name := range order {
		tab, err := loadTable(u.tx, name)
		if err != nil {
			if errors.Is(err, ErrInvalid) {
				continue
			}
			return err
		}
		r := by[name]
		if err := u.checkParents(tab, r.live); err != nil {
			return err
		}
		if err := u.checkChildren(tab, r.gone); err != nil {
			return err
		}
	}
	return nil
}

// parentKey is a foreign key of a table: its columns and the parent's.
type parentKey struct {
	parent   string
	from, to []string
}

func parentKeys(tx *sql.Tx, t *agentTable) ([]parentKey, error) {
	type part struct {
		id     int
		parent string
		from   string
		to     sql.NullString
	}
	parts, err := queryTx(tx, `SELECT id, "table", "from", "to" FROM pragma_foreign_key_list(?) ORDER BY id, seq`, []any{t.name},
		func(r *sql.Rows) (part, error) {
			var p part
			return p, r.Scan(&p.id, &p.parent, &p.from, &p.to)
		})
	if err != nil {
		return nil, err
	}
	var out []parentKey
	for i, p := range parts {
		if i == 0 || p.id != parts[i-1].id {
			out = append(out, parentKey{parent: p.parent})
		}
		k := &out[len(out)-1]
		k.from = append(k.from, p.from)
		if p.to.Valid && p.to.String != "" {
			k.to = append(k.to, p.to.String)
		}
	}
	return out, nil
}

func (u *undoRun) checkParents(tab *agentTable, live []string) error {
	if len(live) == 0 {
		return nil
	}
	fks, err := parentKeys(u.tx, tab)
	if err != nil || len(fks) == 0 {
		return err
	}
	kj := "[" + strings.Join(live, ",") + "]"
	for _, fk := range fks {
		parent, err := loadTable(u.tx, fk.parent)
		if err != nil {
			continue // the commit reports a parent the change log can't handle
		}
		to := fk.to
		if len(to) == 0 {
			to = parent.pk
		}
		if len(to) != len(fk.from) {
			continue
		}
		notNull := make([]string, len(fk.from))
		match := make([]string, len(fk.from))
		for i, c := range fk.from {
			notNull[i] = "_jenab_c." + quote(c) + " IS NOT NULL"
			match[i] = "_jenab_p." + quote(to[i]) + " = _jenab_c." + quote(c)
		}
		keys, err := queryTx(u.tx, fmt.Sprintf("SELECT %s FROM %s AS _jenab_c WHERE %s AND %s AND NOT EXISTS (SELECT 1 FROM %s AS _jenab_p WHERE %s)",
			tab.keyExprAs("_jenab_c"), quote(tab.name), tab.keyMatchAs("_jenab_c"), strings.Join(notNull, " AND "), quote(parent.name),
			strings.Join(match, " AND ")), []any{kj}, func(r *sql.Rows) (string, error) {
			var k string
			return k, r.Scan(&k)
		})
		if err != nil {
			return err
		}
		for _, k := range keys {
			u.conflict(UndoConflict{Item: UndoItem{tab.name, k}, Reason: ConflictReferenced,
				Detail: fmt.Sprintf("it refers to a row of %s that is gone", parent.name)})
		}
	}
	return nil
}

func (u *undoRun) checkChildren(tab *agentTable, gone []rowEntry) error {
	if len(gone) == 0 {
		return nil
	}
	fks, err := (&cascade{fks: map[string][]foreignKey{}}).foreignKeysTo(u.tx, tab)
	if err != nil || len(fks) == 0 {
		return err
	}
	befores := make([]string, len(gone))
	for i, e := range gone {
		befores[i] = e.before.String
	}
	list := "[" + strings.Join(befores, ",") + "]"
	for _, fk := range fks {
		child, err := loadTable(u.tx, fk.child)
		if err != nil {
			continue
		}
		match := make([]string, len(fk.from))
		still := make([]string, len(fk.from))
		for i, c := range fk.from {
			l := literal(fk.to[i])
			old := fmt.Sprintf(`CASE WHEN json_type(_jenab_o.value -> %s) = 'object' THEN unhex(_jenab_o.value -> %s ->> '$."$blob"') ELSE _jenab_o.value ->> %s END`, l, l, l)
			match[i] = "_jenab_c." + quote(c) + " = " + old
			still[i] = "_jenab_p." + quote(fk.to[i]) + " = _jenab_c." + quote(c)
		}
		type hit struct {
			i   int
			key string
		}
		hits, err := queryTx(u.tx, fmt.Sprintf("SELECT DISTINCT _jenab_o.key, %s FROM json_each(?) AS _jenab_o JOIN %s AS _jenab_c ON %s WHERE NOT EXISTS (SELECT 1 FROM %s AS _jenab_p WHERE %s)",
			child.keyExprAs("_jenab_c"), quote(child.name), strings.Join(match, " AND "), quote(tab.name), strings.Join(still, " AND ")),
			[]any{list}, func(r *sql.Rows) (hit, error) {
				var h hit
				return h, r.Scan(&h.i, &h.key)
			})
		if err != nil {
			return err
		}
		for _, h := range hits {
			reached := UndoItem{child.name, h.key}
			u.conflict(UndoConflict{Item: UndoItem{tab.name, gone[h.i].key}, Reason: ConflictReferenced, Reached: &reached,
				Detail: fmt.Sprintf("a row of %s still refers to it", child.name)})
		}
	}
	return nil
}
