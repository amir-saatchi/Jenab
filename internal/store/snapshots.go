package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/sqlworker"
)

// Snapshots before destructive migrations (SPEC 7.5): a safe copy of
// project.db in snapshots/, named project-migration-<ULID>.db, so the
// names sort by time. The last keepSnapshots are kept.

const (
	snapshotPrefix = "project-migration-"
	keepSnapshots  = 5
)

// ErrNoSnapshot is a snapshot name that is not one of the project's.
var ErrNoSnapshot = errors.New("store: no such snapshot")

func (p *ProjectDB) snapshotDir() string { return filepath.Join(filepath.Dir(p.path), "snapshots") }

// takeSnapshot copies project.db into snapshots/ and returns the
// snapshot's name.
func (p *ProjectDB) takeSnapshot(ctx context.Context) (string, error) {
	dir := p.snapshotDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("store: %w", err)
	}
	name := snapshotPrefix + id.New() + ".db"
	if err := p.Snapshot(ctx, filepath.Join(dir, name)); err != nil {
		return "", err
	}
	return name, nil
}

// pruneSnapshots drops the oldest snapshots beyond keepSnapshots, after
// a migration committed.
func (p *ProjectDB) pruneSnapshots() {
	dir := p.snapshotDir()
	old := snapshotNames(dir)
	for len(old) > keepSnapshots {
		os.Remove(filepath.Join(dir, old[0]))
		old = old[1:]
	}
}

// snapshotNames lists the complete snapshots in dir, oldest first.
// A .partial copy never has the final name, so it is not listed.
func snapshotNames(dir string) []string {
	ents, _ := os.ReadDir(dir)
	var out []string
	for _, e := range ents {
		n := e.Name()
		if !e.IsDir() && strings.HasPrefix(n, snapshotPrefix) && strings.HasSuffix(n, ".db") {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// SnapshotInfo is a snapshot the user can restore.
type SnapshotInfo struct {
	Name string
	// WritesSince is how many changes the project logged after it: the
	// writes a restore loses (SPEC 7.5). User memory's pointers aren't
	// counted: user memory is in registry.db, which a restore leaves alone.
	WritesSince int
	Size        int64
}

// Snapshots lists the project's migration snapshots, newest first.
func (p *ProjectDB) Snapshots(ctx context.Context) ([]SnapshotInfo, error) {
	dir := p.snapshotDir()
	names := snapshotNames(dir)
	var out []SnapshotInfo
	for i := len(names) - 1; i >= 0; i-- {
		last, err := snapshotLastChange(ctx, filepath.Join(dir, names[i]))
		if _, serr := os.Stat(filepath.Join(dir, names[i])); errors.Is(serr, os.ErrNotExist) {
			continue // pruned since the listing
		}
		if err != nil {
			return nil, err
		}
		var n int
		if _, err := readTx(ctx, p.DB, func(tx *sql.Tx) (struct{}, error) {
			return struct{}{}, tx.QueryRowContext(ctx, `SELECT count(*) FROM _jenab_changes WHERE id > ? AND target NOT LIKE 'memory:user/%'`, last).Scan(&n)
		}); err != nil {
			return nil, err
		}
		out = append(out, SnapshotInfo{Name: names[i], WritesSince: n, Size: fileSize(filepath.Join(dir, names[i]))})
	}
	return out, nil
}

// snapshotLastChange is the ID of the last change in the database at path, read
// without writing to it; "" for none.
func snapshotLastChange(ctx context.Context, path string) (string, error) {
	db, err := sql.Open("sqlite", sqlworker.FileURI(path)+"?mode=ro&immutable=1")
	if err != nil {
		return "", err
	}
	defer db.Close()
	var last sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT max(id) FROM _jenab_changes`).Scan(&last); err != nil {
		return "", fmt.Errorf("store: read %s: %w", filepath.Base(path), err)
	}
	return last.String, nil
}

// RestoreSnapshot puts snapshot name back as dir/project.db, for a
// project that is closed. The current file first gets its WAL folded in
// and dropped, so no WAL of it applies to the restored file; then the
// snapshot is copied with the safe-copy rule. It returns how many
// changes the restore lost.
func RestoreSnapshot(ctx context.Context, dir, name string) (int, error) {
	snaps := filepath.Join(dir, "snapshots")
	if filepath.Base(name) != name || !strings.HasPrefix(name, snapshotPrefix) {
		return 0, ErrNoSnapshot
	}
	src := filepath.Join(snaps, name)
	if _, err := os.Stat(src); err != nil {
		return 0, ErrNoSnapshot
	}
	last, err := snapshotLastChange(ctx, src)
	if err != nil {
		return 0, err
	}
	target := filepath.Join(dir, "project.db")
	db, err := sql.Open("sqlite", dsn(target, false, common...))
	if err != nil {
		return 0, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var lost int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM _jenab_changes WHERE id > ? AND target NOT LIKE 'memory:user/%'`, last).Scan(&lost); err != nil {
		return 0, fmt.Errorf("store: restore: %w", err)
	}
	// journal_mode can't leave WAL while another connection has the file
	// open, so an open project fails here.
	var mode string
	if err := db.QueryRowContext(ctx, `PRAGMA journal_mode = DELETE`).Scan(&mode); err != nil || mode != "delete" {
		return 0, fmt.Errorf("store: restore: the project is still open (%v)", err)
	}
	if err := db.Close(); err != nil {
		return 0, err
	}
	if err := vacuumInto(ctx, src, target, nil); err != nil {
		return 0, err
	}
	return lost, nil
}
