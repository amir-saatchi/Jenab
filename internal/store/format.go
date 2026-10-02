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
	"time"
)

// Format is a file's storage format: Steps[i] brings the file from version
// i to i+1, so the current version is len(Steps). Each file stores its
// version in PRAGMA user_version (SPEC 2.8).
type Format struct {
	Name  string // file name without .db, used for backup names
	Steps []func(*sql.Tx) error
}

func (f Format) current() int { return len(f.Steps) }

// keepBackups is how many pre-update backups of a file are kept (SPEC 2.8).
const keepBackups = 2

// prepareFormat makes sure path has format f's current version before the
// writer opens it:
//   - a missing or empty file is created at the current version;
//   - an older file is backed up into snapshots, then migrated in one
//     transaction;
//   - a newer file is refused, and the check never writes to it.
func prepareFormat(ctx context.Context, path string, f Format, snapshots string) error {
	if f.current() == 0 {
		return fmt.Errorf("store: format %q has no steps", f.Name)
	}
	if snapshots != "" {
		if err := RemovePartials(snapshots); err != nil {
			return err
		}
	}
	st, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist) || err == nil && st.Size() == 0:
		return migrate(ctx, path, f, 0)
	case err != nil:
		return fmt.Errorf("store: %w", err)
	}
	v, tables, err := readVersion(ctx, path)
	if err != nil {
		return err
	}
	switch {
	case v > f.current():
		return fmt.Errorf("%w: %s has format %d, this app reads up to %d", ErrNewerFormat, filepath.Base(path), v, f.current())
	case v == f.current():
		return nil
	case v == 0 && tables > 0:
		return fmt.Errorf("store: %s is not a Jenab file (format 0 with %d tables)", filepath.Base(path), tables)
	case v > 0:
		if snapshots == "" {
			return fmt.Errorf("store: %s needs a backup before its format update, but no snapshot folder is set", filepath.Base(path))
		}
		if err := backupBeforeUpdate(ctx, path, f.Name, v, snapshots); err != nil {
			return err
		}
	}
	return migrate(ctx, path, f, v)
}

// readVersion reads user_version on a read-only connection, so a file from
// a newer app is left as it is.
func readVersion(ctx context.Context, path string) (v, tables int, err error) {
	db, err := sql.Open("sqlite", dsn(path, true, "busy_timeout(5000)"))
	if err != nil {
		return 0, 0, err
	}
	defer db.Close()
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return 0, 0, fmt.Errorf("store: read the format of %s: %w", filepath.Base(path), err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE type = 'table'").Scan(&tables); err != nil {
		return 0, 0, err
	}
	return v, tables, nil
}

// migrate runs the steps from version `from` to the current one and sets
// user_version, all in one transaction.
func migrate(ctx context.Context, path string, f Format, from int) error {
	db, err := sql.Open("sqlite", dsn(path, false, append([]string{"journal_mode(WAL)"}, common...)...))
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	c, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("store: open %s: %w", filepath.Base(path), err)
	}
	defer c.Close()
	_, err = inTx(ctx, c, func(tx *sql.Tx) (struct{}, error) {
		for v := from; v < f.current(); v++ {
			if err := f.Steps[v](tx); err != nil {
				return struct{}{}, fmt.Errorf("store: %s format step %d: %w", f.Name, v+1, err)
			}
		}
		// PRAGMA takes no parameters; the number comes from len(Steps).
		_, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", f.current()))
		return struct{}{}, err
	})
	return err
}

// backupBeforeUpdate copies path into snapshots with the safe-copy rule and
// keeps the last keepBackups copies of this file.
func backupBeforeUpdate(ctx context.Context, path, name string, v int, snapshots string) error {
	if err := os.MkdirAll(snapshots, 0o755); err != nil {
		return fmt.Errorf("store: %w", err)
	}
	prefix := name + "-pre-update-"
	target := filepath.Join(snapshots, prefix+time.Now().UTC().Format("20060102-150405.000")+fmt.Sprintf("-v%d.db", v))
	if err := vacuumInto(ctx, path, target); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(snapshots, prefix+"*.db"))
	sort.Strings(old) // the time comes first in the name
	for len(old) > keepBackups {
		os.Remove(old[0])
		old = old[1:]
	}
	return nil
}

// vacuumInto copies the database at path to target on its own plain
// connection, outside the reader pool (query_only rejects VACUUM INTO). It
// follows the safe-copy rule: write target.partial, fsync it, then rename,
// so only complete copies ever carry the final name (SPEC 7.5, SPIKE-008).
func vacuumInto(ctx context.Context, path, target string) error {
	partial := target + ".partial"
	removePartial(partial)
	db, err := sql.Open("sqlite", dsn(path, false, common...))
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", partial); err != nil {
		removePartial(partial)
		return fmt.Errorf("store: copy %s: %w", filepath.Base(path), err)
	}
	if err := syncFile(partial); err != nil {
		removePartial(partial)
		return err
	}
	if err := os.Rename(partial, target); err != nil {
		removePartial(partial)
		return fmt.Errorf("store: %w", err)
	}
	return nil
}

func syncFile(name string) error {
	f, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	err = f.Sync()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("store: fsync %s: %w", filepath.Base(name), err)
	}
	return nil
}

func removePartial(p string) {
	for _, s := range []string{"", "-journal", "-wal", "-shm"} {
		os.Remove(p + s)
	}
}

// RemovePartials deletes copies that a crash left half-written in dir, with
// their -journal files (SPIKE-008 S4b). Opening one would roll it back to an
// empty database.
func RemovePartials(dir string) error {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	for _, e := range ents {
		n := e.Name()
		if i := strings.Index(n, ".partial"); i >= 0 && !e.IsDir() {
			switch n[i:] {
			case ".partial", ".partial-journal", ".partial-wal", ".partial-shm":
				if err := os.Remove(filepath.Join(dir, n)); err != nil && !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("store: %w", err)
				}
			}
		}
	}
	return nil
}

// Snapshot copies the database to target with the safe-copy rule, for
// snapshots before destructive migrations and for export (SPEC 7.5, 2.1).
// It blocks neither readers nor the writer.
func (db *DB) Snapshot(ctx context.Context, target string) error {
	return vacuumInto(ctx, db.path, target)
}
