package store

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

// SPIKE-008's crash tests for the store, kept as permanent tests: a worker
// process (this test binary) does real work, gets killed at a random moment
// (TerminateProcess on Windows, SIGKILL elsewhere), and the test then opens
// the files and checks them. A kill is not a power cut: the OS still writes
// what it has buffered. The recovery scenario is in project.
//
// go test -run Crash ./internal/store; JENAB_CRASH_KILLS sets the kills per
// scenario (default 5). -short skips them.

const crashEnv = "JENAB_STORE_CRASH_WORKER"

func kills() int {
	if n, err := strconv.Atoi(os.Getenv("JENAB_CRASH_KILLS")); err == nil && n > 0 {
		return n
	}
	return 5
}

// TestCrashWorker is the worker side; it only runs inside a crash test.
func TestCrashWorker(t *testing.T) {
	spec := os.Getenv(crashEnv)
	if spec == "" {
		t.Skip("runs only as a crash test worker")
	}
	scenario, dir, _ := strings.Cut(spec, "|")
	ctx := context.Background()
	switch scenario {
	case "chunks":
		workChunks(ctx, dir)
	case "snapshots":
		workSnapshots(ctx, dir)
	case "format":
		workFormat(ctx, dir)
	case "objects":
		workObjects(ctx, dir)
	case "parts":
		workParts(ctx, dir)
	case "changes":
		workChanges(ctx, dir)
	case "migrations":
		workMigrations(ctx, dir)
	case "undo":
		workUndo(ctx, dir)
	}
	t.Fatal("the worker returned instead of being killed")
}

// killDuring starts the worker for scenario and kills it a random time
// after it reports READY. It returns the lines the worker printed after
// READY. The output is read to its end before Wait, which closes the pipe.
func killDuring(t *testing.T, scenario, dir string, maxDelay time.Duration) []string {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashWorker$", "-test.v")
	cmd.Env = append(os.Environ(), crashEnv+"="+scenario+"|"+dir)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan bool, 1)
	var scanErr error
	lines := make(chan []string, 1)
	go func() {
		var got []string
		started := false
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			switch {
			case sc.Text() == "READY":
				started = true
				ready <- true
			case started:
				got = append(got, sc.Text())
			}
		}
		scanErr = sc.Err()
		if !started {
			ready <- false
		}
		lines <- got
	}()
	select {
	case ok := <-ready:
		if !ok {
			cmd.Process.Kill() // in case only the reading stopped
			<-lines
			cmd.Wait()
			t.Fatalf("the worker's output ended before READY (read error: %v)", scanErr)
		}
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		<-lines
		cmd.Wait()
		t.Fatal("the worker never got READY")
	}
	time.Sleep(time.Duration(rand.Int64N(int64(maxDelay))))
	cmd.Process.Kill()
	got := <-lines // the pipe ends when the worker dies
	cmd.Wait()
	if scanErr != nil {
		t.Fatalf("reading the worker's output: %v", scanErr)
	}
	return got
}

func quickCheck(t *testing.T, db *DB) {
	t.Helper()
	if r := one[string](t, db, "PRAGMA quick_check"); r != "ok" {
		t.Fatalf("quick_check: %s", r)
	}
}

// ---- chunked writes: whole chunks or nothing ----

const chunkRows = 500

func workChunks(ctx context.Context, dir string) {
	p, err := OpenProject(ctx, dir)
	if err != nil {
		panic(err)
	}
	Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
		_, err := tx.Exec("CREATE TABLE IF NOT EXISTS t (chunk INTEGER, i INTEGER, pad TEXT, PRIMARY KEY (chunk, i))")
		return 0, err
	})
	fmt.Println("READY")
	for {
		_, err := Do(ctx, p.DB, limit.Background, func(tx *sql.Tx) (int, error) {
			var c int
			if err := tx.QueryRow("SELECT coalesce(max(chunk), 0) + 1 FROM t").Scan(&c); err != nil {
				return 0, err
			}
			st, err := tx.Prepare("INSERT INTO t VALUES (?, ?, randomblob(200))")
			if err != nil {
				return 0, err
			}
			defer st.Close()
			for i := range chunkRows {
				if _, err := st.Exec(c, i); err != nil {
					return 0, err
				}
			}
			return c, nil
		})
		if err != nil {
			panic(err)
		}
	}
}

func TestCrashChunkedWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("crash test")
	}
	dir := t.TempDir()
	for k := range kills() {
		killDuring(t, "chunks", dir, 400*time.Millisecond)
		p, err := OpenProject(context.Background(), dir)
		if err != nil {
			t.Fatalf("kill %d: open: %v", k+1, err)
		}
		quickCheck(t, p.DB)
		if n := one[int](t, p.DB, "SELECT count(*) FROM (SELECT chunk FROM t GROUP BY chunk HAVING count(*) <> ?)", chunkRows); n != 0 {
			t.Errorf("kill %d: %d chunks are partly written", k+1, n)
		}
		if gaps := one[int](t, p.DB, "SELECT max(chunk) - count(DISTINCT chunk) FROM t"); gaps != 0 {
			t.Errorf("kill %d: %d chunks missing between committed ones", k+1, gaps)
		}
		t.Logf("kill %d: %d chunks committed", k+1, one[int](t, p.DB, "SELECT count(DISTINCT chunk) FROM t"))
		p.Close(context.Background())
	}
}

// ---- snapshots: only complete copies carry a final name ----

const snapRows = 20000

func workSnapshots(ctx context.Context, dir string) {
	p, err := OpenProject(ctx, dir)
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			Do(ctx, p.DB, limit.Background, func(tx *sql.Tx) (int, error) {
				_, err := tx.Exec("INSERT INTO t (v) VALUES (randomblob(200))")
				return 0, err
			})
		}
	}()
	fmt.Println("READY")
	snaps := filepath.Join(dir, "snapshots")
	for {
		name := filepath.Join(snaps, fmt.Sprintf("snap-%d.db", time.Now().UnixNano()))
		if err := p.Snapshot(ctx, name); err != nil {
			panic(err)
		}
		m, _ := filepath.Glob(filepath.Join(snaps, "snap-*.db"))
		sort.Strings(m)
		for len(m) > 5 {
			os.Remove(m[0])
			m = m[1:]
		}
	}
}

func TestCrashSnapshots(t *testing.T) {
	if testing.Short() {
		t.Skip("crash test")
	}
	dir := t.TempDir()
	p, err := OpenProject(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, p.DB, "CREATE TABLE t (id INTEGER PRIMARY KEY, v BLOB)")
	mustExec(t, p.DB, "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?) INSERT INTO t (v) SELECT randomblob(200) FROM n", snapRows)
	p.Close(context.Background())
	os.MkdirAll(filepath.Join(dir, "snapshots"), 0o755)

	checked, midCopy := 0, 0
	for k := range kills() {
		killDuring(t, "snapshots", dir, 300*time.Millisecond)
		if m, _ := filepath.Glob(filepath.Join(dir, "snapshots", "*.partial")); len(m) > 0 {
			midCopy++
		}
		p, err := OpenProject(context.Background(), dir) // removes half-written copies
		if err != nil {
			t.Fatalf("kill %d: open: %v", k+1, err)
		}
		quickCheck(t, p.DB)
		p.Close(context.Background())
		m, _ := filepath.Glob(filepath.Join(dir, "snapshots", "*"))
		for _, f := range m {
			if filepath.Ext(f) != ".db" {
				t.Errorf("kill %d: %s left in snapshots", k+1, filepath.Base(f))
				continue
			}
			// Opened the way a user restoring it would: read-write, so SQLite
			// would roll back a hot journal.
			s, err := sql.Open("sqlite", dsn(f, false))
			if err != nil {
				t.Fatal(err)
			}
			var res string
			var n int
			err = s.QueryRow("PRAGMA quick_check").Scan(&res)
			if err == nil {
				err = s.QueryRow("SELECT count(*) FROM t").Scan(&n)
			}
			s.Close()
			if err != nil || res != "ok" || n < snapRows {
				t.Errorf("kill %d: snapshot %s is broken: %v, %s, %d rows", k+1, filepath.Base(f), err, res, n)
			}
			checked++
		}
	}
	t.Logf("%d snapshots checked; %d of %d kills hit a copy in progress", checked, midCopy, kills())
}

// ---- format update: the old version or the new one, never between ----

const formatRows = 60000

// bigFormat's step 2 rebuilds t, which takes a moment with formatRows rows.
func bigFormat(n int) Format {
	f := Format{Name: "big", Steps: []func(*sql.Tx) error{
		execAll("CREATE TABLE t (id INTEGER PRIMARY KEY, v BLOB)"),
	}}
	if n >= 2 {
		f.Steps = append(f.Steps, execAll(
			"CREATE TABLE t2 (id INTEGER PRIMARY KEY, v BLOB, extra TEXT NOT NULL DEFAULT 'new')",
			"INSERT INTO t2 (id, v) SELECT id, v FROM t",
			"DROP TABLE t",
			"ALTER TABLE t2 RENAME TO t",
		))
	}
	return f
}

func workFormat(ctx context.Context, dir string) {
	fmt.Println("READY")
	db, err := Open(ctx, filepath.Join(dir, "big.db"), Options{Format: bigFormat(2), Snapshots: filepath.Join(dir, "snapshots")})
	if err != nil {
		panic(err)
	}
	db.Close(ctx)
	time.Sleep(time.Hour) // wait to be killed
}

func TestCrashFormatUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("crash test")
	}
	tmpl := filepath.Join(t.TempDir(), "big.db")
	db := openTest(t, tmpl, bigFormat(1))
	mustExec(t, db, "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?) INSERT INTO t (v) SELECT randomblob(300) FROM n", formatRows)
	db.Close(context.Background())
	orig, err := os.ReadFile(tmpl)
	if err != nil {
		t.Fatal(err)
	}

	// Time one update, so the kills land while it runs.
	trial := filepath.Join(t.TempDir(), "big.db")
	os.WriteFile(trial, orig, 0o644)
	start := time.Now()
	db = openTest(t, trial, bigFormat(2))
	took := time.Since(start)
	db.Close(context.Background())
	t.Logf("one update with its backup takes %v", took.Round(time.Millisecond))

	seen := map[int]int{}
	for k := range kills() {
		dir := t.TempDir()
		path := filepath.Join(dir, "big.db")
		os.WriteFile(path, orig, 0o644)
		killDuring(t, "format", dir, took)

		v := rawVersion(t, path)
		seen[v]++
		db, err := Open(context.Background(), path, Options{Format: bigFormat(v), Snapshots: filepath.Join(dir, "snapshots")})
		if err != nil {
			t.Fatalf("kill %d: open at version %d: %v", k+1, v, err)
		}
		quickCheck(t, db)
		if n := one[int](t, db, "SELECT count(*) FROM t"); n != formatRows {
			t.Errorf("kill %d: version %d has %d rows, want %d", k+1, v, n, formatRows)
		}
		cols := one[int](t, db, "SELECT count(*) FROM pragma_table_info('t')")
		if want := map[int]int{1: 2, 2: 3}[v]; cols != want {
			t.Errorf("kill %d: version %d has %d columns, want %d", k+1, v, cols, want)
		}
		db.Close(context.Background())

		// The backup, if one was finished, is the complete old version.
		m, _ := filepath.Glob(filepath.Join(dir, "snapshots", "*"))
		for _, f := range m {
			if filepath.Ext(f) != ".db" {
				continue // removed at the next open
			}
			if bv := rawVersion(t, f); bv != 1 {
				t.Errorf("kill %d: backup %s has version %d", k+1, filepath.Base(f), bv)
			}
		}
	}
	t.Logf("after the kills: %d at version 1, %d at version 2", seen[1], seen[2])
}

// ---- objects: no row ever points to missing bytes (N-30) ----

func workObjects(ctx context.Context, dir string) {
	p, err := OpenProject(ctx, dir)
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			if _, err := p.SweepObjects(ctx); err != nil {
				panic(err)
			}
		}
	}()
	fmt.Println("READY")
	for i := 0; ; i++ {
		data := make([]byte, rand.IntN(64<<10))
		for j := range data {
			data[j] = byte(rand.IntN(256))
		}
		if i%5 == 0 {
			data = fmt.Appendf(nil, "repeated %d", i%3) // stored once
		}
		if i%7 == 0 {
			p.objects.Put(ctx, bytes.NewReader(data), 0) // bytes with no row, for the sweep
			continue
		}
		if _, err := p.PutObject(ctx, limit.Background, id.SourceApp, fmt.Sprintf("k/%d", i%50), bytes.NewReader(data), PutOptions{Source: "tool"}); err != nil {
			panic(err)
		}
	}
}

func TestCrashObjects(t *testing.T) {
	if testing.Short() {
		t.Skip("crash test")
	}
	dir := t.TempDir()
	ctx := context.Background()
	for k := range kills() {
		killDuring(t, "objects", dir, 500*time.Millisecond)
		p, err := OpenProject(ctx, dir)
		if err != nil {
			t.Fatalf("kill %d: open: %v", k+1, err)
		}
		quickCheck(t, p.DB)
		used, err := p.usedHashes(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for h := range used {
			r, err := p.objects.Open(ctx, h)
			if err != nil {
				t.Errorf("kill %d: a row points to missing bytes: %v", k+1, err)
				continue
			}
			sum := sha256.New()
			io.Copy(sum, r)
			r.Close()
			if hex.EncodeToString(sum.Sum(nil)) != h {
				t.Errorf("kill %d: the bytes of %s don't match their name", k+1, h)
			}
		}
		rep, err := p.SweepObjects(ctx)
		if err != nil {
			t.Fatal(err)
		}
		files := 0
		for s, err := range p.objects.All(ctx) {
			if err != nil {
				t.Fatal(err)
			}
			files++
			if !used[s.Hash] {
				t.Errorf("kill %d: %s has no row after the sweep", k+1, s.Hash)
			}
		}
		t.Logf("kill %d: %d hashes in use, %d files, the sweep removed %d", k+1, len(used), files, rep.Files)
		p.Close(ctx)
	}
}

// ---- streamed parts: a kill loses only the part in progress (P1-06) ----

// partText is the text of part i of message m, so the test can tell a
// whole part from a cut one.
func partText(m id.Message, i, n int) string {
	return strings.Repeat(fmt.Sprintf("%s/%d;", m, i), n)
}

func workParts(ctx context.Context, dir string) {
	c, err := OpenChats(ctx, dir)
	if err != nil {
		panic(err)
	}
	mother, err := c.EnsureMother(ctx)
	if err != nil {
		panic(err)
	}
	ms, _, err := c.Messages(ctx, mother.ID, 0, 0)
	if err != nil {
		panic(err)
	}
	turn := 1
	if len(ms) > 0 {
		turn = ms[len(ms)-1].Turn + 1
	}
	fmt.Println("READY")
	for ; ; turn++ {
		// The message and its tool call first, as before a tool runs.
		m, _, err := c.AppendMessage(ctx, chat.Message{Chat: mother.ID, Turn: turn, Role: chat.RoleAssistant,
			Parts: []chat.Part{{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: "c", Name: "t", Args: json.RawMessage("{}")}}}})
		if err != nil {
			panic(err)
		}
		fmt.Printf("PART %s 0\n", m.ID)
		for i := 1; i <= 5; i++ {
			// Stream the text into memory, as the chat runner does.
			var b strings.Builder
			n := 1 + rand.IntN(300)
			for range n {
				b.WriteString(fmt.Sprintf("%s/%d;", m.ID, i))
				if rand.IntN(50) == 0 {
					time.Sleep(time.Millisecond)
				}
			}
			idx, _, err := c.AppendPart(ctx, m.ID, chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: b.String()}})
			if err != nil || idx != i {
				panic(fmt.Sprint(idx, err))
			}
			fmt.Printf("PART %s %d\n", m.ID, i)
		}
	}
}

func TestCrashStreamedParts(t *testing.T) {
	if testing.Short() {
		t.Skip("crash test")
	}
	dir := t.TempDir()
	ctx := context.Background()
	reported := map[string]bool{}
	for k := range kills() {
		for _, l := range killDuring(t, "parts", dir, 400*time.Millisecond) {
			reported[strings.TrimPrefix(l, "PART ")] = true
		}
		c, err := OpenChats(ctx, dir)
		if err != nil {
			t.Fatalf("kill %d: open: %v", k+1, err)
		}
		quickCheck(t, c.DB)
		m, err := c.EnsureMother(ctx)
		if err != nil {
			t.Fatal(err)
		}
		ms, seq, err := c.Messages(ctx, m.ID, 0, 0)
		if err != nil {
			t.Fatalf("kill %d: %v", k+1, err)
		}
		stored, parts := map[string]bool{}, 0
		for _, msg := range ms {
			for i, p := range msg.Parts {
				parts++
				stored[fmt.Sprintf("%s %d", msg.ID, i)] = true
				if i == 0 {
					if p.ToolCall == nil {
						t.Errorf("kill %d: message %s starts with %s, not its tool call", k+1, msg.ID, p.Kind)
					}
					continue
				}
				// Whole parts only: the text is its unit repeated.
				unit := fmt.Sprintf("%s/%d;", msg.ID, i)
				if p.Text == nil || len(p.Text.Text) == 0 || len(p.Text.Text)%len(unit) != 0 || p.Text.Text != partText(msg.ID, i, len(p.Text.Text)/len(unit)) {
					t.Errorf("kill %d: part %s %d is not whole", k+1, msg.ID, i)
				}
			}
		}
		for r := range reported {
			if !stored[r] {
				t.Errorf("kill %d: part %s was written and is gone", k+1, r)
			}
		}
		// Every write bumps the sequence once, and each one wrote one part.
		if seq != uint64(parts) {
			t.Errorf("kill %d: seq %d, %d parts", k+1, seq, parts)
		}
		t.Logf("kill %d: %d messages, %d parts, %d reported", k+1, len(ms), parts, len(reported))
		c.Close(ctx)
	}
}

// ---- the change log: whole chunks, each with its entry and message (P2-02) ----

const logRequest = 6 * chunkRows // rows per request

func workChanges(ctx context.Context, dir string) {
	p, err := OpenProject(ctx, dir)
	if err != nil {
		panic(err)
	}
	c, err := OpenChats(ctx, dir)
	if err != nil {
		panic(err)
	}
	mother, err := c.EnsureMother(ctx)
	if err != nil {
		panic(err)
	}
	Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
		_, err := tx.Exec("CREATE TABLE IF NOT EXISTS t (req INTEGER, i INTEGER, pad TEXT, PRIMARY KEY (req, i))")
		return 0, err
	})
	req := 0
	Query(ctx, p.DB, "SELECT coalesce(max(req), 0) FROM t", nil, func(r *sql.Rows) (int, error) { return 0, r.Scan(&req) })
	ms, _, _ := c.Messages(ctx, mother.ID, 0, 0)
	turn := len(ms) + 1
	// Interactive writes take turns with the chunks.
	go func() {
		for i := 0; ; i++ {
			p.Insert(ctx, limit.Interactive, id.SourceUser, "t", []Row{{"req": -1, "i": i % 20, "pad": "x"}}, ConflictUpsert)
		}
	}()
	fmt.Println("READY")
	for ; ; turn++ {
		req++
		// The message first, as before a tool runs (SPEC 2.3).
		m, _, err := c.AppendMessage(ctx, chat.Message{Chat: mother.ID, Turn: turn, Role: chat.RoleAssistant,
			Parts: []chat.Part{{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: "c", Name: "insert", Args: json.RawMessage("{}")}}}})
		if err != nil {
			panic(err)
		}
		rows := make([]Row, logRequest)
		for i := range rows {
			rows[i] = Row{"req": req, "i": i, "pad": strings.Repeat("p", 200)}
		}
		if _, err := p.Insert(ctx, limit.Background, id.SourceOf("message", string(m.ID)), "t", rows, ConflictFail); err != nil {
			panic(err)
		}
	}
}

func TestCrashChangeLog(t *testing.T) {
	if testing.Short() {
		t.Skip("crash test")
	}
	dir := t.TempDir()
	ctx := context.Background()
	cut, reqs := 0, 0
	for k := range kills() {
		killDuring(t, "changes", dir, 600*time.Millisecond)
		p, err := OpenProject(ctx, dir)
		if err != nil {
			t.Fatalf("kill %d: open: %v", k+1, err)
		}
		quickCheck(t, p.DB)
		// Whole chunks only, from the first one on.
		if n := one[int](t, p.DB, "SELECT count(*) FROM (SELECT req FROM t WHERE req > 0 GROUP BY req HAVING count(*) % ? <> 0 OR max(i) + 1 <> count(*))", chunkRows); n != 0 {
			t.Errorf("kill %d: %d requests have part of a chunk", k+1, n)
		}
		// Every row has its row entry, and every row entry its change.
		if n := one[int](t, p.DB, "SELECT count(*) FROM t WHERE json_array(req, i) NOT IN (SELECT row_key FROM _jenab_change_rows WHERE table_name = 't')"); n != 0 {
			t.Errorf("kill %d: %d rows without a row entry", k+1, n)
		}
		if n := one[int](t, p.DB, "SELECT count(*) FROM _jenab_change_rows r WHERE NOT EXISTS (SELECT 1 FROM _jenab_changes c WHERE c.id = r.change_id)"); n != 0 {
			t.Errorf("kill %d: %d row entries without their change", k+1, n)
		}
		// One entry per request, holding whole chunks of it, never empty.
		if n := one[int](t, p.DB, `SELECT count(*) FROM _jenab_changes c WHERE c.source LIKE 'message:%' AND
			(SELECT count(*) FROM _jenab_change_rows r WHERE r.change_id = c.id) NOT IN (500, 1000, 1500, 2000, 2500, 3000)`); n != 0 {
			t.Errorf("kill %d: %d entries are empty or hold part of a chunk", k+1, n)
		}
		if n := one[int](t, p.DB, `SELECT count(*) FROM (SELECT change_id FROM _jenab_change_rows WHERE table_name = 't' AND row_key NOT LIKE '[-1,%'
			GROUP BY change_id HAVING count(DISTINCT row_key->>0) <> 1)`); n != 0 {
			t.Errorf("kill %d: %d entries mix requests", k+1, n)
		}
		reqs = one[int](t, p.DB, "SELECT count(DISTINCT req) FROM t WHERE req > 0")
		if n := one[int](t, p.DB, "SELECT count(*) FROM _jenab_changes WHERE source LIKE 'message:%'"); n != reqs {
			t.Errorf("kill %d: %d entries for %d requests", k+1, n, reqs)
		}
		cut = one[int](t, p.DB, "SELECT count(*) FROM (SELECT req FROM t WHERE req > 0 GROUP BY req HAVING count(*) < ?)", logRequest)
		// Never an entry without its message.
		srcs, err := Query(ctx, p.DB, "SELECT source FROM _jenab_changes WHERE source LIKE 'message:%'", nil, func(r *sql.Rows) (string, error) {
			var s string
			return s, r.Scan(&s)
		})
		if err != nil {
			t.Fatal(err)
		}
		p.Close(ctx)
		c, err := OpenChats(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		mother, _ := c.EnsureMother(ctx)
		ms, _, err := c.Messages(ctx, mother.ID, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		have := map[string]bool{}
		for _, m := range ms {
			have["message:"+string(m.ID)] = true
		}
		for _, s := range srcs {
			if !have[s] {
				t.Errorf("kill %d: change from %s, which is not in chats.db", k+1, s)
			}
		}
		c.Close(ctx)
		t.Logf("kill %d: %d requests logged, %d messages", k+1, reqs, len(ms))
	}
	t.Logf("%d of %d requests were cut between chunks", cut, reqs)
	// The kills must have hit requests in the middle, or the checks above
	// proved nothing about cut requests.
	if cut == 0 {
		t.Errorf("no kill cut a request between its chunks in %d kills", kills())
	}
}

// ---- undo: the whole undo or nothing (P2-03) ----

// The turn updates, deletes and inserts rows of u, so the table has one of
// two states: before the turn (undoBefore) and after it (undoAfter), each
// as count(*) and total(v).
const undoRows = 5000

var (
	undoTurn   = id.SourceOf("message", "01CRASHUNDO")
	undoBefore = [2]int{undoRows, 0}
	undoAfter  = [2]int{undoRows - 1000 + 1500, 2500 + 2*1500}
)

func workUndo(ctx context.Context, dir string) {
	p, err := OpenProject(ctx, dir)
	if err != nil {
		panic(err)
	}
	var n int
	Query(ctx, p.DB, "SELECT count(*) FROM sqlite_schema WHERE name = 'u'", nil, func(r *sql.Rows) (int, error) { return 0, r.Scan(&n) })
	if n == 0 {
		if _, err := Do(ctx, p.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
			_, err := tx.Exec("CREATE TABLE u (i INTEGER PRIMARY KEY, v INTEGER NOT NULL, pad TEXT)")
			return 0, err
		}); err != nil {
			panic(err)
		}
		rows := make([]Row, undoRows)
		for i := range rows {
			rows[i] = Row{"i": i, "v": 0, "pad": strings.Repeat("p", 100)}
		}
		if _, err := p.Insert(ctx, limit.Interactive, id.SourceUser, "u", rows, ConflictFail); err != nil {
			panic(err)
		}
		upd := make([]Row, 2500)
		for i := range upd {
			upd[i] = Row{"i": i, "v": 1, "pad": strings.Repeat("q", 100)}
		}
		del := make([]Row, 1000)
		for i := range del {
			del[i] = Row{"i": 2500 + i}
		}
		ins := make([]Row, 1500)
		for i := range ins {
			ins[i] = Row{"i": undoRows + i, "v": 2, "pad": strings.Repeat("r", 100)}
		}
		if _, err := p.Insert(ctx, limit.Interactive, undoTurn, "u", upd, ConflictUpsert); err != nil {
			panic(err)
		}
		if _, err := p.Delete(ctx, limit.Interactive, undoTurn, "u", del); err != nil {
			panic(err)
		}
		if _, err := p.Insert(ctx, limit.Interactive, undoTurn, "u", ins, ConflictFail); err != nil {
			panic(err)
		}
	}
	fmt.Println("READY")
	for {
		st, err := p.UndoState(ctx, []id.Source{undoTurn})
		if err != nil {
			panic(err)
		}
		if _, err := p.Undo(ctx, []id.Source{undoTurn}, UndoOptions{Redo: st.Undo.Changes == 0}); err != nil {
			panic(err)
		}
	}
}

func TestCrashUndo(t *testing.T) {
	if testing.Short() {
		t.Skip("crash test")
	}
	dir := t.TempDir()
	ctx := context.Background()
	undos := 0
	for k := range kills() {
		killDuring(t, "undo", dir, 2*time.Second)
		p, err := OpenProject(ctx, dir)
		if err != nil {
			t.Fatalf("kill %d: open: %v", k+1, err)
		}
		quickCheck(t, p.DB)
		var got [2]int
		got[0] = one[int](t, p.DB, "SELECT count(*) FROM u")
		got[1] = one[int](t, p.DB, "SELECT total(v) FROM u")
		st, err := p.UndoState(ctx, []id.Source{undoTurn})
		if err != nil {
			t.Fatal(err)
		}
		switch got {
		case undoAfter:
			if st.Undo.Changes != 3 || st.Redo.Changes != 0 {
				t.Errorf("kill %d: the turn is in effect, but the state is %+v", k+1, st)
			}
		case undoBefore:
			if st.Undo.Changes != 0 || st.Redo.Changes != 3 {
				t.Errorf("kill %d: the turn is undone, but the state is %+v", k+1, st)
			}
		default:
			t.Errorf("kill %d: u is half undone: %d rows, total %d", k+1, got[0], got[1])
		}
		// Every undo change has all its row entries.
		if n := one[int](t, p.DB, `SELECT count(*) FROM _jenab_changes c WHERE c.source LIKE 'undo:%' AND
			(SELECT count(*) FROM _jenab_change_rows r WHERE r.change_id = c.id) NOT IN (1000, 1500, 2500)`); n != 0 {
			t.Errorf("kill %d: %d undo changes miss row entries", k+1, n)
		}
		undos = one[int](t, p.DB, "SELECT count(*) FROM _jenab_changes WHERE source LIKE 'undo:%'")
		if undos%3 != 0 {
			t.Errorf("kill %d: %d undo changes, not whole undos of 3", k+1, undos)
		}
		p.Close(ctx)
		t.Logf("kill %d: %d undos and redos so far", k+1, undos/3)
	}
	if undos == 0 {
		t.Errorf("no undo finished in %d kills", kills())
	}
}

// ---- migrations: the old schema or the new one, and whole snapshots ----

const migrationRows = 100_000

// migrationShapes are t's two shapes. Each migration rebuilds t from one
// to the other, keeping id, a and the number, and dropping the j column,
// so it is destructive and takes a snapshot first.
var migrationShapes = [2]struct{ cols, num, junk string }{
	{"id,a,b,ja", "b", "ja"},
	{"id,a,c,jb", "c", "jb"},
}

func migrationTo(i int) string {
	s, from := migrationShapes[i], migrationShapes[1-i]
	return fmt.Sprintf(`[{"op": "rebuild_table", "table": "t", "columns": [{"name": "id", "type": "INTEGER"}, {"name": "a", "type": "TEXT"},
		{"name": %q, "type": "REAL"}, {"name": %q, "type": "TEXT"}], "primary_key": ["id"], "mapping": {%[1]q: %[3]q, %[2]q: "'x'"}}]`,
		s.num, s.junk, from.num)
}

func tShape(t *testing.T, db *DB) int {
	t.Helper()
	cols := one[string](t, db, "SELECT group_concat(name) FROM (SELECT name FROM pragma_table_info('t') ORDER BY cid)")
	for i, s := range migrationShapes {
		if cols == s.cols {
			return i
		}
	}
	t.Fatalf("t has columns %s", cols)
	return 0
}

func workMigrations(ctx context.Context, dir string) {
	p, err := OpenProject(ctx, dir)
	if err != nil {
		panic(err)
	}
	fmt.Println("READY")
	for {
		d, err := p.DescribeTable(ctx, "t")
		if err != nil {
			panic(err)
		}
		next := 1
		if d.Columns[2].Name == "c" {
			next = 0
		}
		var m Migration
		if err := json.Unmarshal([]byte(migrationTo(next)), &m.Steps); err != nil {
			panic(err)
		}
		r, err := p.Migrate(ctx, id.SourceOf("migration", id.New()), m, MigrateOptions{Approved: true})
		if err != nil {
			panic(err)
		}
		fmt.Println("MIGRATED", r.SchemaVersion)
	}
}

func TestCrashMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("crash test")
	}
	dir := t.TempDir()
	p, err := OpenProject(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, p.DB, "CREATE TABLE t (id INTEGER PRIMARY KEY, a TEXT, b REAL, ja TEXT)")
	mustExec(t, p.DB, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?)
		INSERT INTO t SELECT i, 'row ' || i, i * 1.5, 'x' FROM n`, migrationRows)
	sum := one[float64](t, p.DB, "SELECT sum(b) FROM t")
	p.Close(context.Background())

	// check is t in one shape, whole.
	check := func(k int, db *DB, what string) int {
		shape := tShape(t, db)
		n := one[int](t, db, "SELECT count(*) FROM t")
		s := one[float64](t, db, fmt.Sprintf("SELECT sum(%s) FROM t", migrationShapes[shape].num))
		if n != migrationRows || s != sum {
			t.Errorf("kill %d: %s: %d rows, sum %v", k+1, what, n, s)
		}
		return shape
	}
	midSnap, done, last := 0, 0, 0
	for k := range kills() {
		out := killDuring(t, "migrations", dir, 4*time.Second)
		if m, _ := filepath.Glob(filepath.Join(dir, "snapshots", "*.partial")); len(m) > 0 {
			midSnap++
		}
		p, err := OpenProject(context.Background(), dir) // removes half-written copies
		if err != nil {
			t.Fatalf("kill %d: open: %v", k+1, err)
		}
		quickCheck(t, p.DB)
		check(k, p.DB, "project.db")
		v, _ := p.SchemaVersion(context.Background())
		if n := one[int](t, p.DB, "SELECT count(*) FROM _jenab_changes WHERE op = 'migrate'"); n != v {
			t.Errorf("kill %d: schema version %d, %d migrations logged", k+1, v, n)
		}
		if v < last {
			t.Errorf("kill %d: schema version went back from %d to %d", k+1, last, v)
		}
		// A migration the worker saw commit is there.
		for _, l := range out {
			var seen int
			if _, err := fmt.Sscanf(l, "MIGRATED %d", &seen); err == nil && seen > v {
				t.Errorf("kill %d: migration %d committed, but the version is %d", k+1, seen, v)
			}
		}
		done += v - last
		last = v
		p.Close(context.Background())
		m, _ := filepath.Glob(filepath.Join(dir, "snapshots", "*"))
		// Each commit prunes to keepSnapshots; a kill after the copy leaves
		// one more.
		if slices.ContainsFunc(out, func(l string) bool { return strings.HasPrefix(l, "MIGRATED") }) && len(m) > keepSnapshots+1 {
			t.Errorf("kill %d: %d snapshots", k+1, len(m))
		}
		for _, f := range m {
			if !strings.HasPrefix(filepath.Base(f), snapshotPrefix) || filepath.Ext(f) != ".db" {
				t.Errorf("kill %d: %s left in snapshots", k+1, filepath.Base(f))
				continue
			}
			s, err := Open(context.Background(), f, Options{ReadOnly: true})
			if err != nil {
				t.Fatalf("kill %d: %s: %v", k+1, filepath.Base(f), err)
			}
			quickCheck(t, s)
			check(k, s, filepath.Base(f))
			s.Close(context.Background())
		}
	}
	t.Logf("%d migrations committed in %d kills; %d kills hit a snapshot in progress", done, kills(), midSnap)
}
