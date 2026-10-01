package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// schemaFor is the newest schema version each app version understands.
func schemaFor(v string) int {
	if v == "1.0.0" || strings.HasPrefix(v, "0.") {
		return 1
	}
	return 2
}

type appDB struct {
	mu     sync.Mutex
	db     *sql.DB
	closed bool
}

func dsn(path string, ro bool) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	if ro {
		q.Add("mode", "ro")
	} else {
		q.Add("_pragma", "journal_mode(WAL)")
	}
	return "file:" + filepath.ToSlash(path) + "?" + q.Encode()
}

func openDB(path string) (*appDB, error) {
	want := schemaFor(version)
	// Look at the schema version read-only first, so an older app never
	// writes a single byte (not even a WAL switch) into a newer database.
	have := 0
	if _, err := os.Stat(path); err == nil {
		ro, err := sql.Open("sqlite", dsn(path, true))
		if err != nil {
			return nil, err
		}
		err = ro.QueryRow("PRAGMA user_version").Scan(&have)
		ro.Close()
		if err != nil {
			return nil, fmt.Errorf("read schema version: %w", err)
		}
	}
	if have > want {
		return nil, fmt.Errorf("%s has schema version %d, but Burrow %s only understands up to %d. It was not opened or changed. Install the newer Burrow again or restore a backup from %s",
			filepath.Base(path), have, version, want, filepath.Join(filepath.Dir(path), "backups"))
	}
	db, err := sql.Open("sqlite", dsn(path, false))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	a := &appDB{db: db}
	if have == 0 {
		if err := a.create(want); err != nil {
			db.Close()
			return nil, err
		}
		lg.Log("db-created", map[string]any{"schema": want})
		return a, nil
	}
	if have < want {
		if err := a.migrate(path, have, want); err != nil {
			db.Close()
			return nil, err
		}
	} else {
		lg.Log("db-opened", map[string]any{"schema": have})
	}
	return a, nil
}

var schemaV1 = []string{
	`CREATE TABLE items(id INTEGER PRIMARY KEY, note TEXT NOT NULL, created_at TEXT NOT NULL)`,
}

var v1to2 = []string{
	`ALTER TABLE items ADD COLUMN source TEXT NOT NULL DEFAULT 'v1'`,
	`CREATE TABLE runs(id INTEGER PRIMARY KEY, started_at TEXT NOT NULL, version TEXT NOT NULL)`,
	`UPDATE items SET source = 'migrated'`,
}

func (a *appDB) create(want int) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := append([]string{}, schemaV1...)
	if want >= 2 {
		stmts = append(stmts, v1to2...)
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", want)); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *appDB) migrate(path string, have, want int) error {
	// 1. Backup while nobody else writes (we are the only process on it).
	bdir := filepath.Join(filepath.Dir(path), "backups")
	if err := os.MkdirAll(bdir, 0o755); err != nil {
		return err
	}
	bpath := filepath.Join(bdir, fmt.Sprintf("app-v%d-%s.db", have, time.Now().Format("20060102-150405.000")))
	t0 := time.Now()
	if _, err := a.db.Exec("VACUUM INTO ?", bpath); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	bi, _ := os.Stat(bpath)
	var bsize int64
	if bi != nil {
		bsize = bi.Size()
	}
	lg.Log("db-backup", map[string]any{"path": bpath, "bytes": bsize, "ms": ms(time.Since(t0))})

	// 2. Migrate forward in ONE transaction, including the version bump.
	t1 := time.Now()
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, s := range v1to2 {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("migrate %d->%d: %w", have, want, err)
		}
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", want)); err != nil {
		return err
	}
	if cfg.FailMigration {
		return errors.New("migrate 1->2: injected failure before COMMIT (rolled back)")
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	lg.Log("db-migrated", map[string]any{"from": have, "to": want, "ms": ms(time.Since(t1))})
	return nil
}

// fakePipeline simulates a pipeline run: a burst of write transactions.
func (a *appDB) fakePipeline(d time.Duration) {
	defer busy.Store(false)
	lg.Log("pipeline-start", map[string]any{"ms": ms(d)})
	end := time.Now().Add(d)
	n := 0
	for time.Now().Before(end) {
		a.mu.Lock()
		if !a.closed {
			_, _ = a.db.Exec(`INSERT INTO items(note, created_at) VALUES(?, ?)`, "pipeline row", time.Now().Format(time.RFC3339Nano))
			n++
		}
		a.mu.Unlock()
		time.Sleep(50 * time.Millisecond)
	}
	lg.Log("pipeline-end", map[string]any{"rows": n})
}

func (a *appDB) touch() (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return 0, errors.New("db closed")
	}
	if _, err := a.db.Exec(`INSERT INTO items(note, created_at) VALUES(?, ?)`, "alive "+version, time.Now().Format(time.RFC3339Nano)); err != nil {
		return 0, err
	}
	var n int
	err := a.db.QueryRow(`SELECT count(*) FROM items`).Scan(&n)
	return n, err
}

func (a *appDB) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	return a.db.Close()
}
