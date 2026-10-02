package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"modernc.org/sqlite"

	"github.com/amir-saatchi/jenab/internal/id"
)

// RegistryFormat is registry.db's storage format (SPEC 2.4).
var RegistryFormat = Format{Name: "registry", Steps: []func(*sql.Tx) error{
	execAll(
		`CREATE TABLE projects (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			folder      TEXT NOT NULL,
			workspace   TEXT NOT NULL DEFAULT '',
			pinned      INTEGER NOT NULL DEFAULT 0,
			last_opened TEXT,
			created_at  TEXT NOT NULL
		)`,
		// Used from later phases (SPEC 3.3, 6.9, 8.7).
		`CREATE TABLE user_memory (
			section    TEXT PRIMARY KEY,
			content    TEXT NOT NULL,
			revision   INTEGER NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE connections (
			name       TEXT PRIMARY KEY,
			config     TEXT NOT NULL,
			revision   INTEGER NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE mcp_servers (
			name       TEXT PRIMARY KEY,
			config     TEXT NOT NULL,
			projects   TEXT NOT NULL DEFAULT '[]',
			revision   INTEGER NOT NULL,
			updated_at TEXT NOT NULL
		)`,
	),
}}

// Registry is registry.db: written rarely, so one connection with
// busy_timeout and no writer goroutine (SPEC 2.4).
type Registry struct {
	db *sql.DB
	mu sync.Mutex // one caller at a time on the connection
	c  *sql.Conn
}

// ProjectEntry is a row of the project list.
type ProjectEntry struct {
	ID         id.Project
	Name       string
	Folder     string
	Workspace  string // the code workspace folder, if any (SPEC 8.5)
	Pinned     bool
	LastOpened time.Time // zero if never opened
	CreatedAt  time.Time
}

// OpenRegistry opens or creates registry.db at path. Backups before a format
// update go to the snapshots folder next to it.
func OpenRegistry(ctx context.Context, path string) (*Registry, error) {
	if err := prepareFormat(ctx, path, RegistryFormat, filepath.Join(filepath.Dir(path), "snapshots")); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn(path, false, append([]string{"journal_mode(WAL)"}, common...)...))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	c, err := db.Conn(ctx)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("store: open registry: %w", err)
	}
	if _, err := sqlite.Limit(c, limitAttached, 0); err != nil {
		c.Close()
		db.Close()
		return nil, err
	}
	return &Registry{db: db, c: c}, nil
}

// Close closes the registry.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return errors.Join(r.c.Close(), r.db.Close())
}

const projectCols = "id, name, folder, workspace, pinned, last_opened, created_at"

// Projects lists every project: pinned first, then the last opened.
func (r *Registry) Projects(ctx context.Context) ([]ProjectEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows, err := r.c.QueryContext(ctx, "SELECT "+projectCols+" FROM projects ORDER BY pinned DESC, last_opened DESC NULLS LAST, created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProjectEntry
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Project returns one project, or ErrNotFound.
func (r *Registry) Project(ctx context.Context, pid id.Project) (ProjectEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, err := scanProject(r.c.QueryRowContext(ctx, "SELECT "+projectCols+" FROM projects WHERE id = ?", string(pid)))
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectEntry{}, fmt.Errorf("project %s: %w", pid, ErrNotFound)
	}
	return p, err
}

// SaveProject adds the project or replaces its row.
func (r *Registry) SaveProject(ctx context.Context, p ProjectEntry) error {
	if !id.Valid(string(p.ID)) {
		return fmt.Errorf("store: project id %q is not a ULID", p.ID)
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.c.ExecContext(ctx, `INSERT INTO projects (`+projectCols+`) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name, folder = excluded.folder, workspace = excluded.workspace,
			pinned = excluded.pinned, last_opened = excluded.last_opened`,
		string(p.ID), p.Name, p.Folder, p.Workspace, p.Pinned, timeOrNull(p.LastOpened), formatTime(p.CreatedAt))
	return err
}

// TouchProject sets the time a project was last opened.
func (r *Registry) TouchProject(ctx context.Context, pid id.Project, t time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	res, err := r.c.ExecContext(ctx, "UPDATE projects SET last_opened = ? WHERE id = ?", formatTime(t), string(pid))
	return notFoundIfNone(res, err, pid)
}

// DeleteProject removes a project from the list. Its folder is not touched.
func (r *Registry) DeleteProject(ctx context.Context, pid id.Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	res, err := r.c.ExecContext(ctx, "DELETE FROM projects WHERE id = ?", string(pid))
	return notFoundIfNone(res, err, pid)
}

func notFoundIfNone(res sql.Result, err error, pid id.Project) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("project %s: %w", pid, ErrNotFound)
	}
	return nil
}

type scanner interface{ Scan(...any) error }

func scanProject(s scanner) (ProjectEntry, error) {
	var p ProjectEntry
	var pid, created string
	var last sql.NullString
	if err := s.Scan(&pid, &p.Name, &p.Folder, &p.Workspace, &p.Pinned, &last, &created); err != nil {
		return ProjectEntry{}, err
	}
	p.ID = id.Project(pid)
	var err error
	if p.CreatedAt, err = parseTime(created); err != nil {
		return ProjectEntry{}, err
	}
	if last.Valid {
		if p.LastOpened, err = parseTime(last.String); err != nil {
			return ProjectEntry{}, err
		}
	}
	return p, nil
}

// Times are stored as UTC text with milliseconds, so they sort as text.
const timeLayout = "2006-01-02T15:04:05.000Z"

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseTime(s string) (time.Time, error) { return time.Parse(timeLayout, s) }

func timeOrNull(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return formatTime(t)
}
