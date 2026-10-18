package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

// changeTables is project.db format step 3: the change log (SPEC 2.5) and
// the column annotations that object references are checked with (4.5).
//   - _jenab_changes has one entry per write request. Its ID is a ULID, so
//     entries sort in the order they were made; the index on source finds a
//     turn's or a run's changes for undo (2.6).
//   - _jenab_change_rows is a rowid table: its rowid is the order the rows
//     were written in, even when the chunks of one request and other
//     requests take turns (see chunks).
var changeTables = execAll(
	`CREATE TABLE _jenab_changes (
		id     TEXT PRIMARY KEY,
		at     TEXT NOT NULL,
		source TEXT NOT NULL,
		target TEXT NOT NULL,
		op     TEXT NOT NULL,
		before TEXT,
		after  TEXT
	)`,
	`CREATE INDEX _jenab_changes_source ON _jenab_changes (source)`,
	`CREATE TABLE _jenab_change_rows (
		change_id  TEXT NOT NULL,
		table_name TEXT NOT NULL,
		row_key    TEXT NOT NULL,
		kind       TEXT NOT NULL,
		before     TEXT
	)`,
	`CREATE INDEX _jenab_change_rows_key ON _jenab_change_rows (table_name, row_key, change_id)`,
	`CREATE INDEX _jenab_change_rows_change ON _jenab_change_rows (change_id)`,
	`CREATE TABLE IF NOT EXISTS _jenab_columns (
		table_name  TEXT NOT NULL,
		column_name TEXT NOT NULL,
		kind        TEXT NOT NULL,
		PRIMARY KEY (table_name, column_name)
	) WITHOUT ROWID`,
)

// Row entry kinds (SPEC 2.5).
const (
	rowInserted = "i"
	rowUpdated  = "u"
	rowDeleted  = "d"
)

// change is one entry of _jenab_changes.
type change struct {
	id     id.Change
	at     time.Time
	source id.Source
	target string // table:<name>, object:<key>, …
	op     string // insert, upsert, update, delete, save, move, …
	before sql.NullString
	after  sql.NullString
}

// rowEntry is one row a change touched. key is the primary key as a JSON
// array, or the target for objects; before is the row as a JSON object,
// NULL for an inserted row.
type rowEntry struct {
	table, key, kind string
	before           sql.NullString
}

func newChange(src id.Source, target, op string) change {
	return change{id: id.Change(id.New()), at: time.Now().UTC().Truncate(time.Millisecond), source: src, target: target, op: op}
}

func checkSource(src id.Source) error {
	if src.Kind() == "" {
		return fault("store: a write needs a source (SPEC 2.5)")
	}
	return nil
}

// writeChange writes c and its row entries inside the write's own
// transaction.
func writeChange(tx *sql.Tx, c change, rows []rowEntry) error {
	if _, err := tx.Exec(`INSERT INTO _jenab_changes (id, at, source, target, op, before, after) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		string(c.id), formatTime(c.at), string(c.source), c.target, c.op, c.before, c.after); err != nil {
		return err
	}
	return writeRowEntries(tx, c.id, rows)
}

// writeRowEntries adds rows to change cid with one statement.
func writeRowEntries(tx *sql.Tx, cid id.Change, rows []rowEntry) error {
	if len(rows) == 0 {
		return nil
	}
	list := make([][4]any, len(rows))
	for i, r := range rows {
		list[i] = [4]any{r.table, r.key, r.kind, nil}
		if r.before.Valid {
			list[i][3] = r.before.String
		}
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO _jenab_change_rows (change_id, table_name, row_key, kind, before)
		SELECT ?, value->>0, value->>1, value->>2, value->>3 FROM json_each(?)`, string(cid), string(b))
	return err
}

// chunkSize is the rows per transaction of a large write (SPEC 7.4).
const chunkSize = 500

// WriteResult says what a write request did. After an error it counts the
// chunks that committed before it.
type WriteResult struct {
	Change   id.Change // its change-log entry; "" if nothing changed
	Inserted int
	Updated  int
	Deleted  int
	Skipped  int // rows that were already there (skip), unchanged, or not found
	Cascaded int // rows that foreign-key actions deleted or changed
}

func (r *WriteResult) add(o WriteResult) {
	if o.Change != "" {
		r.Change = o.Change
	}
	r.Inserted += o.Inserted
	r.Updated += o.Updated
	r.Deleted += o.Deleted
	r.Skipped += o.Skipped
	r.Cascaded += o.Cascaded
}

// chunkLog collects one chunk's row entries and counts.
type chunkLog struct {
	target  string
	cascade []rowEntry // rows foreign-key actions reach, logged before rows (see cascade.log)
	rows    []rowEntry
	seen    map[string]bool // table and key: a row is logged once per chunk, with its first before-image
	res     WriteResult
}

func (l *chunkLog) add(table, key, kind string, before sql.NullString) {
	k := table + "\x00" + key
	if l.seen[k] {
		return
	}
	l.seen[k] = true
	l.rows = append(l.rows, rowEntry{table: table, key: key, kind: kind, before: before})
	switch kind {
	case rowInserted:
		l.res.Inserted++
	case rowUpdated:
		l.res.Updated++
	case rowDeleted:
		l.res.Deleted++
	}
}

// chunks runs a write request of n items in chunks of chunkSize, each in
// its own transaction (SPEC 7.4); fn does the work of items [from, to).
//
// One request is one change entry (SPEC 2.5), however many chunks it has:
// the first chunk that changes a row writes the entry, and later chunks add
// their row entries to it. Each chunk commits its rows, their row entries
// and, the first time, the entry itself together, so a crash or an error
// leaves whole chunks only, an entry only with the rows of at least one
// chunk, and no row entry without its entry. A cut-short request then looks
// like an interrupted run (2.7): undo reverts what committed. Other requests
// may run between the chunks; the row entries' rowids keep the true order.
//
// Once the first chunk has run, the rest run even if ctx is cancelled, as
// for any request the writer has taken (7.3). So an import of a million
// rows can't be stopped once it has started; the caps on a request's rows
// (P2-05) bound how long that takes.
func (p *ProjectDB) chunks(ctx context.Context, pr limit.Priority, src id.Source, op string, n int,
	fn func(tx *sql.Tx, from, to int, l *chunkLog) error) (WriteResult, error) {
	var res WriteResult
	if err := checkSource(src); err != nil {
		return res, err
	}
	c := newChange(src, "", op)
	written := false // the entry has committed
	for from := 0; from < n; from += chunkSize {
		to := min(from+chunkSize, n)
		r, err := Do(ctx, p.DB, pr, func(tx *sql.Tx) (WriteResult, error) {
			l := &chunkLog{seen: map[string]bool{}}
			if err := fn(tx, from, to, l); err != nil {
				return WriteResult{}, err
			}
			rows := append(l.cascade, l.rows...)
			if len(rows) == 0 {
				return l.res, nil
			}
			var err error
			if written {
				err = writeRowEntries(tx, c.id, rows)
			} else {
				c.target = l.target
				err = writeChange(tx, c, rows)
			}
			l.res.Change = c.id
			return l.res, err
		})
		if err != nil {
			return res, err
		}
		if r.Change != "" {
			written = true
		}
		res.add(r)
		ctx = context.WithoutCancel(ctx)
	}
	return res, nil
}

// ---- cleanup ----

// ChangeKeep is how long row entries, and with them undo, are kept by
// default (SPEC 2.5, 2.6).
const ChangeKeep = 90 * 24 * time.Hour

// Meta keys of the cleanup.
const (
	metaCleanedTo = "changes_cleaned_to" // the newest change whose row entries are dropped, or being dropped
	metaCleanedAt = "changes_cleaned_at" // when the cleanup last ran
)

// cleanChanges and cleanRows bound one cleanup transaction, so it stays
// short (SPEC 7.4); a change with more rows than cleanRows takes several.
const (
	cleanChanges = 200
	cleanRows    = 5000
)

// CleanReport says what a cleanup dropped.
type CleanReport struct {
	Changes int // entries whose row entries were dropped
	Rows    int // row entries dropped
}

// CleanChanges drops the row entries of changes older than keep (0 means
// ChangeKeep) and keeps the entries themselves (SPEC 2.5). Table changes
// get their row counts per kind in after, e.g. {"d":0,"i":450,"u":50};
// objects, configs and memory keep their revisions there. It works forward
// from the watermark in _jenab_meta, in short background transactions of at
// most cleanRows row entries. Each one moves the watermark and the counts
// with the rows it drops, so a crash loses nothing and repeats nothing. The
// watermark moves onto a change with its first dropped rows, so undo, which
// refuses changes at or below it (2.6), never sees a change with part of
// its rows. The app runs it daily.
func (p *ProjectDB) CleanChanges(ctx context.Context, keep time.Duration) (CleanReport, error) {
	if keep <= 0 {
		keep = ChangeKeep
	}
	cutoff := formatTime(time.Now().Add(-keep))
	var rep CleanReport
	for {
		b, err := Do(ctx, p.DB, limit.Background, func(tx *sql.Tx) (cleaned, error) { return cleanBatch(tx, cutoff) })
		rep.Changes += b.Changes
		rep.Rows += b.Rows
		if err != nil {
			return rep, err
		}
		if !b.more {
			break
		}
	}
	return rep, p.SetMeta(ctx, map[string]string{metaCleanedAt: formatTime(time.Now())})
}

// CleanedAt is when CleanChanges last finished; zero if never.
func (p *ProjectDB) CleanedAt(ctx context.Context) (time.Time, error) {
	m, err := p.Meta(ctx)
	if err != nil || m[metaCleanedAt] == "" {
		return time.Time{}, err
	}
	return parseTime(m[metaCleanedAt])
}

type cleaned struct {
	CleanReport
	more bool // the batch stopped at its size, not at the cutoff
}

// cleanBatch drops up to cleanRows row entries: first what is left of the
// change at the watermark, then those of the changes after it, oldest
// first, until one is newer than cutoff. IDs are ULIDs, so their order is
// the order of at.
func cleanBatch(tx *sql.Tx, cutoff string) (cleaned, error) {
	var out cleaned
	var mark string
	err := tx.QueryRow(`SELECT value FROM _jenab_meta WHERE key = ?`, metaCleanedTo).Scan(&mark)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	budget := cleanRows
	if mark != "" {
		n, err := dropRows(tx, mark, budget)
		if err != nil {
			return out, err
		}
		out.Rows += n
		if budget -= n; budget == 0 {
			out.more = true
			return out, nil
		}
	}
	type entry struct{ id, at string }
	list, err := queryTx(tx, `SELECT id, at FROM _jenab_changes WHERE id > ? ORDER BY id LIMIT ?`,
		[]any{mark, cleanChanges}, func(r *sql.Rows) (entry, error) {
			var e entry
			return e, r.Scan(&e.id, &e.at)
		})
	if err != nil {
		return out, err
	}
	cut := false // reached a change inside the window
	for _, e := range list {
		if e.at >= cutoff {
			cut = true
			break
		}
		n, err := dropRows(tx, e.id, budget)
		if err != nil {
			return out, err
		}
		out.Changes++
		out.Rows += n
		mark = e.id
		if budget -= n; budget == 0 {
			break
		}
	}
	out.more = !cut && out.Changes > 0 && (budget == 0 || len(list) == cleanChanges)
	if out.Changes == 0 {
		return out, nil
	}
	_, err = tx.Exec(`INSERT INTO _jenab_meta (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, metaCleanedTo, mark)
	return out, err
}

// dropRows drops up to n row entries of change cid and, for a table
// change, adds them to its counts per kind.
func dropRows(tx *sql.Tx, cid string, n int) (int, error) {
	kinds, err := queryTx(tx, `DELETE FROM _jenab_change_rows WHERE rowid IN
		(SELECT rowid FROM _jenab_change_rows WHERE change_id = ? LIMIT ?) RETURNING kind`, []any{cid, n},
		func(r *sql.Rows) (string, error) {
			var k string
			return k, r.Scan(&k)
		})
	if err != nil || len(kinds) == 0 {
		return 0, err
	}
	count := map[string]int{}
	for _, k := range kinds {
		count[k]++
	}
	_, err = tx.Exec(`UPDATE _jenab_changes SET after = json_object('d', coalesce(after->>'d', 0) + ?,
		'i', coalesce(after->>'i', 0) + ?, 'u', coalesce(after->>'u', 0) + ?) WHERE id = ? AND target LIKE 'table:%'`,
		count[rowDeleted], count[rowInserted], count[rowUpdated], cid)
	return len(kinds), err
}
