package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"modernc.org/sqlite"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/sqlguard"
)

// DependentTx is a migration's transaction as its dependents see it
// (SPEC 7.5): the schema after its steps, the configs it saved so far,
// and the SQL check of a view's query on the migration's own connection,
// since no reader sees the new schema before the commit. It is only
// valid during the Dependent call it is given to.
type DependentTx struct {
	ctx context.Context
	c   *sql.Conn
	tx  *sql.Tx
	src id.Source // migration:<id>, for the configs it saves

	saved   []string       // config targets saved, no-ops too, in order
	changes []ConfigChange // the saves that changed a config, for project:configs after the commit

	after bool           // the steps have run
	memo  map[string]any // what dependents keep between the checks before and after
}

// AfterSteps reports whether the migration's steps have run: false for
// the check before them, true for the updates and the check after.
func (t *DependentTx) AfterSteps() bool { return t.after }

// Memo is a map the dependents keep things in from the check before the
// steps to the one after, for this migration only. Keys start with the
// dependent's kind.
func (t *DependentTx) Memo() map[string]any {
	if t.memo == nil {
		t.memo = map[string]any{}
	}
	return t.memo
}

// Context is the migration's context.
func (t *DependentTx) Context() context.Context { return t.ctx }

// Tx is the migration's transaction.
func (t *DependentTx) Tx() *sql.Tx { return t.tx }

// Source is migration:<id>, the source of what a dependent saves: an Undo
// turn doesn't revert it, as it doesn't revert the migration (SPEC 2.6).
func (t *DependentTx) Source() id.Source { return t.src }

// Tables lists the agent tables, by name without case, as
// ProjectDB.Tables does.
func (t *DependentTx) Tables() ([]string, error) {
	names, err := listAgentTables(t.ctx, t.tx)
	slices.SortFunc(names, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return names, err
}

// TableShape describes the agent table name without its row count, as
// ProjectDB.TableShape does; ErrNotFound when there is none.
func (t *DependentTx) TableShape(name string) (Table, error) {
	return describe(t.ctx, t.tx, name, false)
}

// QueryColumns is ProjectDB.QueryColumns on the migration's connection:
// the text check, the EXPLAIN check and the prepare, without running q.
// SQLite's own errors, such as "no such column", match IsSQLiteError.
func (t *DependentTx) QueryColumns(q string) ([]QueryColumn, error) {
	if t.c == nil {
		return nil, fault("store: no connection to prepare a query on")
	}
	if err := sqlguard.New().Explain(t.ctx, t.c, q); err != nil {
		return nil, err
	}
	cols, err := sqlguard.Columns(t.c, q)
	if err != nil {
		return nil, err
	}
	out := make([]QueryColumn, len(cols))
	for i, c := range cols {
		out[i] = QueryColumn{Name: c.Name, Table: c.Table, Column: c.Origin}
	}
	return out, nil
}

// Configs lists the live configs of kind, by ID.
func (t *DependentTx) Configs(kind ConfigKind) ([]Config, error) { return ConfigsTx(t.tx, kind) }

// Config returns the live config cid of kind ("" for either), or nil.
func (t *DependentTx) Config(kind ConfigKind, cid string) (*Config, error) {
	return liveIn(t.tx, kind, NormConfigID(cid))
}

// Object reports whether the bucket has a file at key.
func (t *DependentTx) Object(key string) (bool, error) {
	if bucket.CheckKey(key) != nil {
		return false, nil
	}
	var n int
	err := t.tx.QueryRowContext(t.ctx, `SELECT count(*) FROM _jenab_objects WHERE key = ?`, key).Scan(&n)
	return n > 0, err
}

// SaveConfig saves a config in the migration's transaction with
// SaveConfigTx, under the migration's source, whatever s.Source says. The
// migration publishes the change after its commit.
func (t *DependentTx) SaveConfig(s ConfigSave) (ConfigWrite, error) {
	s.Source = t.src
	w, err := SaveConfigTx(t.tx, s)
	if err != nil {
		return w, err
	}
	t.saved = append(t.saved, w.Config.Target())
	if w.Op != "" {
		t.changes = append(t.changes, changeOf(w.Config))
	}
	return w, nil
}

// IsSQLiteError reports whether err is SQLite's own, such as "no such
// column" from DependentTx.QueryColumns.
func IsSQLiteError(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se)
}
