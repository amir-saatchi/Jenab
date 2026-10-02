package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testFormat has n steps: step 1 creates t, later steps add a column each.
func testFormat(n int) Format {
	f := Format{Name: "test"}
	f.Steps = append(f.Steps, execAll("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)"))
	for i := 2; i <= n; i++ {
		col := string(rune('a' + i))
		f.Steps = append(f.Steps, execAll("ALTER TABLE t ADD COLUMN "+col+" TEXT"))
	}
	return f
}

func openTest(t *testing.T, path string, f Format) *DB {
	t.Helper()
	db, err := Open(context.Background(), path, Options{Format: f, Snapshots: filepath.Join(filepath.Dir(path), "snapshots")})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// rawVersion reads user_version without going through Open.
func rawVersion(t *testing.T, path string) int {
	t.Helper()
	v, _, err := readVersion(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestNewerFormatIsRefusedUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "project.db")
	db := openTest(t, path, testFormat(3))
	mustExec(t, db, "INSERT INTO t (v) VALUES ('from the newer app')")
	db.Close(context.Background())

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Open(context.Background(), path, Options{Format: testFormat(2), Snapshots: filepath.Join(dir, "snapshots")})
	if !errors.Is(err, ErrNewerFormat) {
		t.Fatalf("err = %v, want ErrNewerFormat", err)
	}
	if !strings.Contains(err.Error(), "format 3") {
		t.Errorf("the error doesn't say which format: %v", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Error("the newer file changed")
	}
	if ents, _ := os.ReadDir(filepath.Join(dir, "snapshots")); len(ents) != 0 {
		t.Errorf("a backup was made of a file that wasn't migrated: %d files", len(ents))
	}
}

func TestNewerFormatWithWALIsRefusedUntouched(t *testing.T) {
	// A newer app that crashed leaves committed data in the WAL, with no
	// connection open. Opening and closing it read-write would checkpoint
	// that data into the file; the check must not.
	src := t.TempDir()
	path := filepath.Join(src, "project.db")
	db := openTest(t, path, testFormat(3))
	db.w.close(context.Background()) // stop without the checkpoint in Close
	c := db.writeConn()
	for _, q := range []string{"PRAGMA wal_autocheckpoint = 0", "INSERT INTO t (v) VALUES (randomblob(10000))"} {
		if _, err := c.ExecContext(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	// Copy the file and its WAL as a crash would leave them.
	crashed := t.TempDir()
	for _, n := range []string{"project.db", "project.db-wal"} {
		b, err := os.ReadFile(filepath.Join(src, n))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(crashed, n), b, 0o644)
	}
	c.Close()
	db.readers.Close()
	db.wdb.Close()

	path = filepath.Join(crashed, "project.db")
	before, _ := os.ReadFile(path)
	_, err := Open(context.Background(), path, Options{Format: testFormat(2), Snapshots: filepath.Join(crashed, "snapshots")})
	if !errors.Is(err, ErrNewerFormat) {
		t.Fatalf("err = %v", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Error("the check wrote to the newer file")
	}
	if st, err := os.Stat(path + "-wal"); err != nil || st.Size() == 0 {
		t.Errorf("the newer app's WAL was consumed: %v", err)
	}
}

func TestOlderFormatIsBackedUpAndMigrated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chats.db")
	db := openTest(t, path, testFormat(1))
	mustExec(t, db, "INSERT INTO t (v) VALUES ('kept')")
	db.Close(context.Background())

	for v := 2; v <= 4; v++ {
		db = openTest(t, path, testFormat(v))
		if got := one[int](t, db, "PRAGMA user_version"); got != v {
			t.Errorf("user_version = %d, want %d", got, v)
		}
		if got := one[string](t, db, "SELECT v FROM t"); got != "kept" {
			t.Errorf("row after the update to %d: %q", v, got)
		}
		db.Close(context.Background())
	}
	backups, _ := filepath.Glob(filepath.Join(dir, "snapshots", "test-pre-update-*.db"))
	if len(backups) != keepBackups {
		t.Fatalf("%d backups, want the last %d: %v", len(backups), keepBackups, backups)
	}
	for i, want := range []int{2, 3} { // the backups from before the updates to 3 and 4
		if !strings.HasSuffix(backups[i], "-v"+string(rune('0'+want))+".db") {
			t.Errorf("backup %d is %s, want one of version %d", i, filepath.Base(backups[i]), want)
		}
		if got := rawVersion(t, backups[i]); got != want {
			t.Errorf("%s has version %d", filepath.Base(backups[i]), got)
		}
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "snapshots", "*.partial*")); len(m) != 0 {
		t.Errorf("partial files left: %v", m)
	}
}

func TestFailedStepRollsBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "project.db")
	db := openTest(t, path, testFormat(1))
	mustExec(t, db, "INSERT INTO t (v) VALUES ('kept')")
	db.Close(context.Background())

	bad := testFormat(1)
	bad.Steps = append(bad.Steps, func(tx *sql.Tx) error {
		if _, err := tx.Exec("CREATE TABLE half (id INTEGER PRIMARY KEY)"); err != nil {
			return err
		}
		_, err := tx.Exec("ALTER TABLE missing ADD COLUMN x TEXT")
		return err
	})
	if _, err := Open(context.Background(), path, Options{Format: bad, Snapshots: filepath.Join(dir, "snapshots")}); err == nil || !strings.Contains(err.Error(), "step 2") {
		t.Fatalf("err = %v, want the failing step", err)
	}
	db = openTest(t, path, testFormat(1))
	defer db.Close(context.Background())
	if v := one[int](t, db, "PRAGMA user_version"); v != 1 {
		t.Errorf("user_version = %d after a failed update", v)
	}
	if n := one[int](t, db, "SELECT count(*) FROM sqlite_schema WHERE name = 'half'"); n != 0 {
		t.Error("a table from the failed step remains")
	}
}

func TestUnknownFileIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.db")
	raw, err := sql.Open("sqlite", dsn(path, false))
	if err != nil {
		t.Fatal(err)
	}
	raw.Exec("CREATE TABLE someone_elses (x)")
	raw.Close()
	if _, err := Open(context.Background(), path, Options{Format: testFormat(1)}); err == nil || !strings.Contains(err.Error(), "not a Jenab file") {
		t.Errorf("err = %v", err)
	}
}

func TestPartialCopiesAreRemovedOnOpen(t *testing.T) {
	dir := t.TempDir()
	snaps := filepath.Join(dir, "snapshots")
	os.MkdirAll(snaps, 0o755)
	for _, n := range []string{"snap-1.db.partial", "snap-1.db.partial-journal", "snap-2.db"} {
		os.WriteFile(filepath.Join(snaps, n), []byte("x"), 0o644)
	}
	p, err := OpenProject(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close(context.Background())
	ents, _ := os.ReadDir(snaps)
	if len(ents) != 1 || ents[0].Name() != "snap-2.db" {
		var names []string
		for _, e := range ents {
			names = append(names, e.Name())
		}
		t.Errorf("snapshots = %v, want only snap-2.db", names)
	}
}

func TestSnapshot(t *testing.T) {
	p, dir := openTestProject(t)
	mustExec(t, p.DB, "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)")
	mustExec(t, p.DB, "INSERT INTO t (v) VALUES ('a'), ('b')")
	target := filepath.Join(dir, "snapshots", "snap.db")
	os.MkdirAll(filepath.Dir(target), 0o755)
	if err := p.Snapshot(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target + ".partial"); !errors.Is(err, os.ErrNotExist) {
		t.Error("the .partial file is still there")
	}
	s, err := Open(context.Background(), target, Options{Format: ProjectFormat})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(context.Background())
	if n := one[int](t, s, "SELECT count(*) FROM t"); n != 2 {
		t.Errorf("rows in the snapshot = %d", n)
	}
}
