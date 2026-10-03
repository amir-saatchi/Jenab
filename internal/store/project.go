package store

import (
	"context"
	"database/sql"
	"path/filepath"

	"github.com/amir-saatchi/jenab/internal/limit"
)

// ProjectFormat is project.db's storage format. Phase 1 has only the
// internal tables it needs; the rest of SPEC 2.2 comes with later phases.
var ProjectFormat = Format{Name: "project", Steps: []func(*sql.Tx) error{
	execAll(
		// Project ID, name, schema version, approval level, and later the
		// bucket lifecycle rules (SPEC 2.2, 4.4, 8.8).
		`CREATE TABLE _jenab_meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		) WITHOUT ROWID`,
		// Approved hosts, script hashes, destructive migrations and MCP
		// tools, with who approved them (SPEC 8.8).
		`CREATE TABLE _jenab_approvals (
			id         TEXT PRIMARY KEY,
			kind       TEXT NOT NULL,
			target     TEXT NOT NULL,
			answer     TEXT NOT NULL,
			note       TEXT NOT NULL DEFAULT '',
			source     TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX _jenab_approvals_target ON _jenab_approvals (kind, target)`,
	),
}}

// projectPragmas are for the project.db writer: a 64 MB cache, and a WAL
// that shrinks back to 64 MB after a large write (SPEC 7.2).
var projectPragmas = []string{"journal_size_limit = 67108864", "cache_size = -65536"}

// ProjectDB is a project's project.db.
type ProjectDB struct {
	*DB
}

// OpenProject opens dir/project.db, creating it if needed. Backups before a
// format update go to dir/snapshots.
func OpenProject(ctx context.Context, dir string) (*ProjectDB, error) {
	db, err := Open(ctx, filepath.Join(dir, "project.db"), Options{
		Format:        ProjectFormat,
		Snapshots:     filepath.Join(dir, "snapshots"),
		WriterPragmas: projectPragmas,
	})
	if err != nil {
		return nil, err
	}
	return &ProjectDB{DB: db}, nil
}

// OpenProjectReadOnly opens dir/project.db for reading only, for a file that
// failed its quick_check (SPEC 2.7).
func OpenProjectReadOnly(ctx context.Context, dir string) (*ProjectDB, error) {
	db, err := Open(ctx, filepath.Join(dir, "project.db"), Options{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return &ProjectDB{DB: db}, nil
}

// ReadProjectMeta reads _jenab_meta of a project that is not open, without
// starting a writer or updating the format, for rebuilding the registry
// (SPEC 2.1).
func ReadProjectMeta(ctx context.Context, dir string) (map[string]string, error) {
	p, err := OpenProjectReadOnly(ctx, dir)
	if err != nil {
		return nil, err
	}
	defer p.Close(ctx)
	return p.Meta(ctx)
}

// Meta returns every _jenab_meta entry.
func (p *ProjectDB) Meta(ctx context.Context) (map[string]string, error) {
	type kv struct{ k, v string }
	rows, err := Query(ctx, p.DB, "SELECT key, value FROM _jenab_meta", nil, func(r *sql.Rows) (kv, error) {
		var x kv
		return x, r.Scan(&x.k, &x.v)
	})
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.k] = r.v
	}
	return m, nil
}

// SetMeta writes the given _jenab_meta entries in one transaction.
func (p *ProjectDB) SetMeta(ctx context.Context, kv map[string]string) error {
	_, err := Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		for k, v := range kv {
			if _, err := tx.Exec(`INSERT INTO _jenab_meta (key, value) VALUES (?, ?)
				ON CONFLICT (key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
				return struct{}{}, err
			}
		}
		return struct{}{}, nil
	})
	return err
}

// execAll returns a format step that runs each statement in order.
func execAll(stmts ...string) func(*sql.Tx) error {
	return func(tx *sql.Tx) error {
		for _, s := range stmts {
			if _, err := tx.Exec(s); err != nil {
				return err
			}
		}
		return nil
	}
}
