// Package store owns the SQLite files: one writer goroutine and a reader
// pool per file, storage format versions, and registry.db (SPEC 2, 7).
package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"modernc.org/sqlite"
)

var (
	ErrNotFound    = errors.New("not found")
	ErrConflict    = errors.New("revision conflict")
	ErrClosed      = errors.New("store: database is closed")
	ErrNewerFormat = errors.New("store: the file was written by a newer version of the app")
	ErrReadOnly    = errors.New("store: the database is open read-only")
)

// SQLite limit IDs (sqlite3.h); modernc doesn't export names for them.
const (
	limitLength   = 0 // SQLITE_LIMIT_LENGTH
	limitAttached = 7 // SQLITE_LIMIT_ATTACHED
)

// Options configure one database file.
type Options struct {
	// Format is the file's storage format: the steps that bring it from
	// version 0 to the current one (SPEC 2.8).
	Format Format
	// Snapshots is where the backup goes before a format migration.
	Snapshots string
	// Readers is the size of the reader pool; 0 means 4.
	Readers int
	// WriterPragmas run on the write connection after the common ones,
	// e.g. the cache size for project.db (SPEC 7.2).
	WriterPragmas []string
	// ReadOnly opens only the reader pool, for a file that failed its
	// quick_check (SPEC 2.7). The format is not checked or updated, and
	// every write fails with ErrReadOnly.
	ReadOnly bool
}

// DB is one database file: a writer goroutine and a reader pool (SPEC 7.2).
type DB struct {
	path    string
	w       *writer
	wdb     *sql.DB // holds the one write connection
	readers *sql.DB

	connMu sync.Mutex // guards wconn while it is reopened after a panic
	wconn  *sql.Conn
	wprags []string

	closeOnce sync.Once
	closeErr  error
}

// dsn builds a modernc DSN. Pragmas in the DSN run on every new connection.
func dsn(path string, readOnly bool, pragmas ...string) string {
	q := url.Values{}
	for _, p := range pragmas {
		q.Add("_pragma", p)
	}
	if readOnly {
		q.Set("mode", "ro")
	}
	return "file:" + filepath.ToSlash(path) + "?" + q.Encode()
}

// common runs on every connection to a project, chats or registry file.
var common = []string{"busy_timeout(5000)", "foreign_keys(1)", "synchronous(NORMAL)"}

// Open opens path: it checks the format version, backs up and migrates an
// older file, and starts the writer. A file from a newer app is refused
// without being touched (ErrNewerFormat).
func Open(ctx context.Context, path string, o Options) (*DB, error) {
	if o.ReadOnly {
		return openReadOnly(path, o)
	}
	if err := prepareFormat(ctx, path, o.Format, o.Snapshots); err != nil {
		return nil, err
	}
	db := &DB{path: path, wprags: o.WriterPragmas}
	var err error
	db.wdb, err = sql.Open("sqlite", dsn(path, false, append([]string{"journal_mode(WAL)"}, common...)...))
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", filepath.Base(path), err)
	}
	db.wdb.SetMaxOpenConns(1)
	if err := db.openWriteConn(ctx); err != nil {
		db.wdb.Close()
		return nil, err
	}
	n := o.Readers
	if n <= 0 {
		n = 4
	}
	db.readers, err = sql.Open("sqlite", dsn(path, false, append(common, "query_only(1)")...))
	if err != nil {
		db.wconn.Close()
		db.wdb.Close()
		return nil, err
	}
	db.readers.SetMaxOpenConns(n)
	db.readers.SetMaxIdleConns(n)

	db.w = newWriter(db.writeConn, db.resetWriteConn)
	go db.w.loop()
	return db, nil
}

func openReadOnly(path string, o Options) (*DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	r, err := sql.Open("sqlite", dsn(path, false, append(common, "query_only(1)")...))
	if err != nil {
		return nil, err
	}
	n := o.Readers
	if n <= 0 {
		n = 4
	}
	r.SetMaxOpenConns(n)
	r.SetMaxIdleConns(n)
	return &DB{path: path, readers: r}, nil
}

// ReadOnly reports whether the file was opened read-only.
func (db *DB) ReadOnly() bool { return db.w == nil }

func (db *DB) openWriteConn(ctx context.Context) error {
	c, err := db.wdb.Conn(ctx)
	if err != nil {
		return fmt.Errorf("store: open %s: %w", filepath.Base(db.path), err)
	}
	if _, err := sqlite.Limit(c, limitAttached, 0); err != nil {
		c.Close()
		return err
	}
	for _, p := range db.wprags {
		if _, err := c.ExecContext(ctx, "PRAGMA "+p); err != nil {
			c.Close()
			return fmt.Errorf("store: PRAGMA %s: %w", p, err)
		}
	}
	db.connMu.Lock()
	db.wconn = c
	db.connMu.Unlock()
	return nil
}

func (db *DB) writeConn() *sql.Conn {
	db.connMu.Lock()
	defer db.connMu.Unlock()
	return db.wconn
}

// resetWriteConn closes and reopens the write connection after a request
// panicked mid-transaction (Q26). The transaction was already rolled back.
func (db *DB) resetWriteConn(any) {
	db.connMu.Lock()
	old := db.wconn
	db.connMu.Unlock()
	if old != nil {
		// ErrBadConn makes database/sql drop the connection instead of reusing it.
		old.Raw(func(any) error { return driver.ErrBadConn })
		old.Close()
	}
	if err := db.openWriteConn(context.Background()); err != nil {
		db.w.setErr(err)
	}
}

// Path is the file's path.
func (db *DB) Path() string { return db.path }

// Stats is the writer's queue per priority, the current request and its
// age, and the last error.
func (db *DB) Stats() WriterStats {
	if db.w == nil {
		return WriterStats{}
	}
	return db.w.stats()
}

// Close drains the writer (queued requests still run unless ctx ends
// first), checkpoints the WAL with a 100 ms busy timeout, and closes every
// connection.
func (db *DB) Close(ctx context.Context) error {
	db.closeOnce.Do(func() {
		if db.w == nil {
			db.closeErr = db.readers.Close()
			return
		}
		db.w.close(ctx)
		c := db.writeConn()
		var errs []error
		if c != nil {
			if _, err := checkpoint(context.Background(), c); err != nil {
				errs = append(errs, err)
			}
			errs = append(errs, c.Close())
		}
		errs = append(errs, db.readers.Close(), db.wdb.Close())
		db.closeErr = errors.Join(errs...)
	})
	return db.closeErr
}

// Checkpoint runs wal_checkpoint(TRUNCATE) on the writer, for a project that
// went idle (SPEC 2.7) or after a long migration (7.5). It reports false if
// a reader held it up; it is then retried at the next idle.
func (db *DB) Checkpoint(ctx context.Context) (bool, error) {
	return doConn(ctx, db, 0, checkpoint)
}

// checkpoint waits at most 100 ms for readers, so it never blocks the
// writer for the full busy timeout (SPEC 7.2).
func checkpoint(ctx context.Context, c *sql.Conn) (bool, error) {
	if _, err := c.ExecContext(ctx, "PRAGMA busy_timeout = 100"); err != nil {
		return false, err
	}
	defer c.ExecContext(ctx, "PRAGMA busy_timeout = 5000")
	var busy, log, done int
	if err := c.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &log, &done); err != nil {
		if isBusy(err) {
			return false, nil
		}
		return false, err
	}
	return busy == 0, nil
}

// Query runs q on a reader connection and scans every row before it
// returns, so no cursor stays open (SPEC 7.2). Pass a ctx with a deadline:
// a cancelled query is interrupted.
func Query[T any](ctx context.Context, db *DB, q string, args []any, scan func(*sql.Rows) (T, error)) ([]T, error) {
	c, err := db.reader(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	rows, err := c.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// reader takes a connection from the pool and sets the guards again, so a
// connection that something changed can't be used without them (SPEC 2.2).
func (db *DB) reader(ctx context.Context) (*sql.Conn, error) {
	c, err := db.readers.Conn(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrConnDone) || err.Error() == "sql: database is closed" {
			return nil, ErrClosed
		}
		return nil, err
	}
	if _, err := c.ExecContext(ctx, "PRAGMA query_only = ON"); err != nil {
		c.Close()
		return nil, err
	}
	for _, l := range [][2]int{{limitAttached, 0}, {limitLength, 16 << 20}} {
		if _, err := sqlite.Limit(c, l[0], l[1]); err != nil {
			c.Close()
			return nil, err
		}
	}
	return c, nil
}

func isBusy(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code()&0xff == 5 // SQLITE_BUSY and its extended codes
}

// QuickCheck runs PRAGMA quick_check on the file at path, on its own
// connection, before the file is opened for use (SPEC 2.7). It returns ""
// for a sound file and SQLite's first problems otherwise. A missing file
// has nothing to check. Opening the file lets SQLite finish its own
// recovery of a crashed WAL, which is safe; nothing else is written.
func QuickCheck(ctx context.Context, path string) (string, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	db, err := sql.Open("sqlite", dsn(path, false, "busy_timeout(5000)"))
	if err != nil {
		return "", err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "PRAGMA quick_check(5)")
	if err != nil {
		if isCorrupt(err) {
			return err.Error(), nil
		}
		return "", fmt.Errorf("store: quick_check %s: %w", filepath.Base(path), err)
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return "", err
		}
		lines = append(lines, l)
	}
	if err := rows.Err(); err != nil {
		if isCorrupt(err) {
			return err.Error(), nil
		}
		return "", err
	}
	if len(lines) == 1 && lines[0] == "ok" {
		return "", nil
	}
	return strings.Join(lines, "; "), nil
}

// isCorrupt reports SQLITE_CORRUPT and SQLITE_NOTADB, which mean the file
// is damaged rather than that the check could not run.
func isCorrupt(err error) bool {
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return false
	}
	c := se.Code() & 0xff
	return c == 11 || c == 26
}
