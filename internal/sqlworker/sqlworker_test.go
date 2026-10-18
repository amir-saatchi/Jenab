package sqlworker

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/sqlguard"
)

// TestMain lets the test binary be its own worker, and the app that
// starts one (helperParent).
func TestMain(m *testing.M) {
	IfWorker()
	if path := os.Getenv(parentEnv); path != "" {
		helperParent(path)
		return
	}
	os.Exit(m.Run())
}

// parentEnv names the database for helperParent.
const parentEnv = "SQLWORKER_TEST_PARENT"

// helperParent is an app with a worker: it starts one, prints its process
// ID and waits to be killed.
func helperParent(path string) {
	p := New(path, Options{})
	if _, err := p.Query(context.Background(), Request{Query: "SELECT 1", Max: 1}); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	fmt.Println(p.pids()[0])
	time.Sleep(time.Hour)
}

// openDB makes a WAL database at dir/project.db with a prices table of
// symbols × days rows, and keeps a connection open, as the app does.
func openDB(tb testing.TB, symbols, days int) string {
	tb.Helper()
	return openDBAt(tb, filepath.Join(tb.TempDir(), "project.db"), symbols, days)
}

func openDBAt(tb testing.TB, path string, symbols, days int) string {
	tb.Helper()
	db, err := sql.Open("sqlite", FileURI(path)+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { db.Close() })
	for _, q := range []string{
		"CREATE TABLE _jenab_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL) WITHOUT ROWID",
		"INSERT INTO _jenab_meta VALUES ('project_id', '01J')",
		`CREATE TABLE prices (symbol TEXT NOT NULL, day TEXT NOT NULL, open REAL, high REAL, low REAL,
			close REAL, volume INTEGER, PRIMARY KEY (symbol, day))`,
		fmt.Sprintf(`WITH RECURSIVE s(i) AS (SELECT 0 UNION ALL SELECT i+1 FROM s WHERE i < %d),
			d(j) AS (SELECT 0 UNION ALL SELECT j+1 FROM d WHERE j < %d),
			p AS (SELECT i, j, 100 + i + 10*sin(j/20.0 + i) + 5*sin(j/3.0) AS c FROM s, d)
			INSERT INTO prices SELECT printf('S%%03d', i), date('2000-01-01', '+' || j || ' days'),
				c - 0.5 + (j %% 3) * 0.4, c + 1, c - 1, c, 1000 + (i * j) %% 500 FROM p`, symbols-1, days-1),
	} {
		if _, err := db.Exec(q); err != nil {
			tb.Fatal(err)
		}
	}
	return path
}

func newPool(tb testing.TB, path string, o Options) *Pool {
	p := New(path, o)
	tb.Cleanup(p.Close)
	return p
}

func query(tb testing.TB, p *Pool, d time.Duration, q string, args ...any) (Result, error) {
	tb.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return p.Query(ctx, Request{Query: q, Args: args, Max: 100})
}

// peak is the largest peak memory of the pool's workers.
func (p *Pool) peak() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	var n uint64
	for w := range p.all {
		n = max(n, w.lim.peak())
	}
	return n
}

func TestPoolQuery(t *testing.T) {
	p := newPool(t, openDB(t, 3, 10), Options{})
	r, err := query(t, p, 5*time.Second, "SELECT symbol, count(*) AS n, ? AS a, :b AS b FROM prices GROUP BY symbol",
		int64(7), sql.Named("b", []byte{0, 1}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(r.Columns, ",") != "symbol,n,a,b" || len(r.Values) != 3 || r.Total != 3 || r.Cut {
		t.Fatalf("%+v", r)
	}
	if v := r.Values[2]; v[0] != "S002" || v[1] != int64(10) || v[2] != int64(7) || string(v[3].([]byte)) != "\x00\x01" {
		t.Errorf("%#v", v)
	}
	// Refusals and SQLite's errors cross the pipe with what they say, and
	// the worker stays.
	_, err = query(t, p, 5*time.Second, "SELECT * FROM _jenab_meta")
	var re *sqlguard.RejectError
	if !errors.As(err, &re) || re.Layer != 1 || err.Error() != "line 1:15: `_jenab_meta` is internal" {
		t.Errorf("refusal: %v", err)
	}
	_, err = query(t, p, 5*time.Second, "SELECT nope FROM prices")
	if err == nil || errors.As(err, &re) || !strings.Contains(err.Error(), "no such column: nope") {
		t.Errorf("SQLite error: %v", err)
	}
	pids := p.pids()
	if _, err := query(t, p, 5*time.Second, "SELECT 1"); err != nil || len(pids) != 1 || p.pids()[0] != pids[0] {
		t.Errorf("worker not kept: %v, %v, %v", err, pids, p.pids())
	}
	// A bad argument fails before it reaches the worker.
	if _, err := query(t, p, time.Second, "SELECT ?", struct{}{}); err == nil || !strings.Contains(err.Error(), "converting argument $1") {
		t.Errorf("bad argument: %v", err)
	}
}

// TestPoolDeadline: at the deadline the worker is killed, whatever it is
// doing, and the next query gets a new one.
func TestPoolDeadline(t *testing.T) {
	p := newPool(t, openDB(t, 3, 10), Options{Workers: 1})
	for _, q := range []string{
		"WITH RECURSIVE r(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM r) SELECT count(*) FROM r",
		// One call that SQLite can't interrupt.
		"SELECT instr(printf('%.*c', 200000, 'x'), printf('%.*c', 60000, 'x') || 'y')",
	} {
		before := p.pids()
		start := time.Now()
		_, err := query(t, p, 300*time.Millisecond, q)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("%.50s: %v", q, err)
		}
		if d := time.Since(start); d > 600*time.Millisecond {
			t.Errorf("%.50s: stopped after %v", q, d)
		}
		if r, err := query(t, p, 5*time.Second, "SELECT count(*) FROM prices"); err != nil || r.Values[0][0] != int64(30) {
			t.Fatalf("next query: %v %v", r, err)
		}
		if len(before) > 0 && p.pids()[0] == before[0] {
			t.Errorf("%.50s: the worker was kept", q)
		}
	}
}

// TestPoolMemoryCap: a query over the cap is stopped with a clear error.
func TestPoolMemoryCap(t *testing.T) {
	p := newPool(t, openDB(t, 3, 10), Options{Memory: 64 << 20})
	start := time.Now()
	_, err := query(t, p, 10*time.Second,
		"WITH RECURSIVE r(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM r WHERE i < 200) SELECT length(jsonb_group_array(printf('%.*c', 900000, 'x'))) FROM r")
	var me *MemoryError
	if !errors.As(err, &me) || err.Error() != "the query used more than 64 MiB and was stopped" {
		t.Fatalf("%v", err)
	}
	t.Logf("stopped after %v", time.Since(start))
	if _, err := query(t, p, 5*time.Second, "SELECT 1"); err != nil {
		t.Errorf("next query: %v", err)
	}
}

// TestPoolConcurrent: a slow query doesn't hold up a quick one.
func TestPoolConcurrent(t *testing.T) {
	p := newPool(t, openDB(t, 3, 10), Options{})
	query(t, p, 5*time.Second, "SELECT 1") // one worker started already
	var wg sync.WaitGroup
	wg.Go(func() {
		query(t, p, 2*time.Second, "WITH RECURSIVE r(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM r) SELECT count(*) FROM r")
	})
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	if _, err := query(t, p, 5*time.Second, "SELECT count(*) FROM prices"); err != nil {
		t.Error(err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("the quick query waited %v", d)
	}
	wg.Wait()
}

// TestPoolIdleAndClose: an idle worker exits, and Close stops a running
// query.
func TestPoolIdleAndClose(t *testing.T) {
	p := newPool(t, openDB(t, 3, 10), Options{Idle: 100 * time.Millisecond})
	if _, err := query(t, p, 5*time.Second, "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(p.pids()) > 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := len(p.pids()); n != 0 {
		t.Errorf("%d workers after the idle time", n)
	}
	done := make(chan error)
	go func() {
		_, err := query(t, p, 10*time.Second, "WITH RECURSIVE r(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM r) SELECT count(*) FROM r")
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)
	p.Close()
	if err := <-done; !errors.Is(err, ErrClosed) {
		t.Errorf("running query: %v", err)
	}
	if _, err := query(t, p, time.Second, "SELECT 1"); !errors.Is(err, ErrClosed) {
		t.Errorf("after Close: %v", err)
	}
}

// TestProtocol: values cross the pipe as they are, and a frame that
// doesn't fit is an error, never a panic or a huge allocation.
func TestProtocol(t *testing.T) {
	when := time.Date(2026, 10, 18, 8, 0, 3, 0, time.UTC)
	in := Result{Columns: []string{"a", "b"}, Values: [][]any{{int64(-3), 1.5}, {"بیت‌کوین", []byte{0}}, {nil, when}},
		Total: 7, Cut: true, Full: true}
	b := encodeResponse(in, nil)
	out, err := decodeResponse(b[4:])
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(out) != fmt.Sprint(in) || !out.Values[2][1].(time.Time).Equal(when) {
		t.Errorf("got %v\nwant %v", out, in)
	}
	for i := range len(b) - 4 {
		if _, err := decodeResponse(b[4 : 4+i]); err == nil {
			t.Errorf("a frame cut at %d decodes", i)
		}
	}
	// A row count no frame could hold.
	e := newEnc()
	e.byte(respRows)
	e.uvarint(1)
	e.str("a")
	e.uvarint(1 << 40)
	if _, err := decodeResponse(e.b[4:]); err == nil {
		t.Error("a huge row count decodes")
	}
	huge := []byte{0xff, 0xff, 0xff, 0x7f}
	if _, err := readFrame(bufio.NewReader(strings.NewReader(string(huge)))); err == nil {
		t.Error("a frame over maxFrame is read")
	}
	req := Request{Query: "SELECT ?", Args: []any{1, "x", true, nil, sql.Named("n", 2.5)}, Max: 3}
	f, err := encodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeRequest(f[4:])
	want := Request{Query: "SELECT ?", Args: []any{int64(1), "x", true, nil, sql.Named("n", 2.5)}, Max: 3}
	if err != nil || fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("request %v %v", got, err)
	}
	// NULLs take a byte each in the frame and 16 in memory: rows of them
	// that a worker can't send are refused before they are made.
	e = newEnc()
	e.byte(respRows)
	e.uvarint(1)
	e.str("a")
	e.uvarint(4 << 20)
	e.b = append(e.b, make([]byte, 4<<20)...)
	e.uvarint(0)
	e.byte(0)
	if _, err := decodeResponse(e.b[4:]); err == nil {
		t.Error("4M NULL rows decode")
	}
}

// TestPoolValues: arguments bind, and values come back, as they do in the
// app: a nil []byte is NULL, a zero-length blob a nil []byte, and a time
// is the driver's text with its zone's name.
func TestPoolValues(t *testing.T) {
	path := openDB(t, 1, 1)
	req := Request{Query: "SELECT ?1 IS NULL AS a, typeof(?2) AS b, ?2 AS c, x'' AS d, ?3 AS e, typeof(?3) AS f",
		Args: []any{[]byte(nil), []byte{}, time.Date(2026, 10, 18, 8, 0, 0, 0, time.FixedZone("IRST", 12600))}, Max: 1}
	p := newPool(t, path, Options{})
	got, err := p.Query(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	want, err := runQuery(db, sqlguard.New(), req)
	if err != nil {
		t.Fatal(err)
	}
	if g, w := fmt.Sprintf("%#v", got.Values), fmt.Sprintf("%#v", want.Values); g != w {
		t.Errorf("worker:     %s\nin the app: %s", g, w)
	}
	if !strings.Contains(fmt.Sprint(want.Values), "IRST") {
		t.Errorf("the zone's name is lost: %v", want.Values)
	}
}

func TestFileURI(t *testing.T) {
	for in, want := range map[string]string{
		"/a/b c/p.db":         "file:/a/b%20c/p.db",
		"/x#y/%41?z/p.db":     "file:/x%23y/%2541%3Fz/p.db",
		"C:/Users/ا/p.db":     "file:C:/Users/%D8%A7/p.db",
		"//server/share/p.db": "file:////server/share/p.db",
	} {
		if got := FileURI(in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}

// TestPoolOddPaths: a folder's name that means something in a URI opens
// the file in that folder, and the worker can't write to it.
func TestPoolOddPaths(t *testing.T) {
	names := []string{"a#b", "%41", "with space", "پروژه‌ها"}
	if runtime.GOOS != "windows" {
		names = append(names, "what?") // Windows refuses '?' in names
	}
	for _, name := range names {
		parent := t.TempDir()
		dir := filepath.Join(parent, name)
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := openDBAt(t, filepath.Join(dir, "project.db"), 2, 3)
		if es, _ := os.ReadDir(parent); len(es) != 1 || es[0].Name() != name {
			t.Errorf("%s: the parent folder holds %v", name, es)
		}
		p := newPool(t, path, Options{})
		r, err := query(t, p, 5*time.Second, "SELECT count(*) FROM prices")
		if err != nil || len(r.Values) != 1 || r.Values[0][0] != int64(6) {
			t.Errorf("%s: %v %v", name, r.Values, err)
		}
		// mode=ro holds without query_only.
		db, err := sql.Open("sqlite", dsn(path))
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		_, err = db.Exec("PRAGMA query_only(0)")
		if err == nil {
			_, err = db.Exec("CREATE TABLE x (a)")
		}
		if err == nil || !strings.Contains(err.Error(), "readonly") {
			t.Errorf("%s: the worker's connection writes: %v", name, err)
		}
		db.Close()
	}
}

// TestPoolStartError: a worker that can't open the file says why.
func TestPoolStartError(t *testing.T) {
	p := newPool(t, filepath.Join(t.TempDir(), "none", "project.db"), Options{})
	_, err := query(t, p, 5*time.Second, "SELECT 1")
	if err == nil || !strings.Contains(err.Error(), "start a worker: unable to open database file") {
		t.Errorf("%v", err)
	}
	if n := len(p.pids()); n != 0 {
		t.Errorf("%d workers kept", n)
	}
}

// TestPoolCloseWhileStarting: Close waits for workers that are still
// starting, so none runs after it.
func TestPoolCloseWhileStarting(t *testing.T) {
	path := openDB(t, 1, 1)
	var mu sync.Mutex
	var ws []*worker
	started = func(w *worker) {
		mu.Lock()
		ws = append(ws, w)
		mu.Unlock()
	}
	defer func() { started = nil }()
	for i := range 10 {
		p := New(path, Options{})
		p.Warm()
		go p.Query(context.Background(), Request{Query: "SELECT 1", Max: 1})
		time.Sleep(time.Duration(i) * 10 * time.Millisecond)
		p.Close()
		mu.Lock()
		for _, w := range ws {
			select {
			case <-w.exited:
			default:
				t.Errorf("run %d: worker %d runs after Close", i, w.cmd.Process.Pid)
			}
		}
		ws = nil
		mu.Unlock()
	}
}

// TestPoolOutsideKill: a query right after its idle worker was killed from
// outside runs in a new one.
func TestPoolOutsideKill(t *testing.T) {
	p := newPool(t, openDB(t, 1, 1), Options{Workers: 1})
	for i := range 5 {
		if _, err := query(t, p, 5*time.Second, "SELECT 1"); err != nil {
			t.Fatal(err)
		}
		p.mu.Lock()
		for w := range p.all {
			w.lim.kill() // as another program would: the pool isn't told
		}
		p.mu.Unlock()
		if _, err := query(t, p, 5*time.Second, "SELECT 1"); err != nil {
			t.Errorf("run %d: %v", i, err)
		}
	}
}

// TestPoolWarm: Warm starts one worker, in the background.
func TestPoolWarm(t *testing.T) {
	p := newPool(t, openDB(t, 1, 1), Options{})
	p.Warm()
	deadline := time.Now().Add(10 * time.Second)
	for len(p.idlePIDs()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	pids := p.idlePIDs()
	if len(pids) != 1 {
		t.Fatalf("idle workers: %v", pids)
	}
	if _, err := query(t, p, 5*time.Second, "SELECT 1"); err != nil || p.pids()[0] != pids[0] {
		t.Errorf("the query didn't use the warm worker: %v %v", err, p.pids())
	}
}

// idlePIDs are the process IDs of the idle workers.
func (p *Pool) idlePIDs() []int {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []int
	for _, w := range p.idle {
		out = append(out, w.cmd.Process.Pid)
	}
	return out
}

// analytic are realistic queries over prices: none may be refused.
var analytic = []struct{ name, q string }{
	{"moving averages", `SELECT symbol, day, close,
		avg(close) OVER (PARTITION BY symbol ORDER BY day ROWS 19 PRECEDING) AS ma20,
		avg(close) OVER (PARTITION BY symbol ORDER BY day ROWS 49 PRECEDING) AS ma50,
		avg(close) OVER (PARTITION BY symbol ORDER BY day ROWS 199 PRECEDING) AS ma200
		FROM prices ORDER BY symbol, day`},
	{"monthly performance", `WITH m AS (
			SELECT symbol, substr(day, 1, 7) AS month, min(day) AS first, max(day) AS last
			FROM prices GROUP BY symbol, month),
		r AS (
			SELECT m.symbol, m.month, (pl.close - pf.close) / pf.close AS ret
			FROM m JOIN prices pf ON pf.symbol = m.symbol AND pf.day = m.first
			JOIN prices pl ON pl.symbol = m.symbol AND pl.day = m.last),
		ranked AS (
			SELECT symbol, month, ret, rank() OVER (PARTITION BY month ORDER BY ret DESC) AS rnk,
				dense_rank() OVER (PARTITION BY symbol ORDER BY ret DESC) AS best,
				percent_rank() OVER (PARTITION BY month ORDER BY ret) AS pct,
				ntile(4) OVER (PARTITION BY month ORDER BY ret) AS quartile
			FROM r)
		SELECT * FROM ranked WHERE rnk <= 3 ORDER BY month, rnk`},
	{"pair correlation", `WITH ret AS (
			SELECT symbol, day, close / lag(close) OVER (PARTITION BY symbol ORDER BY day) - 1 AS r
			FROM prices WHERE symbol <= 'S009'),
		p AS (
			SELECT a.symbol AS s1, b.symbol AS s2, a.r AS x, b.r AS y
			FROM ret a JOIN ret b ON a.day = b.day AND a.symbol < b.symbol
			WHERE a.r IS NOT NULL AND b.r IS NOT NULL)
		SELECT s1, s2, count(*) AS n,
			(avg(x*y) - avg(x)*avg(y)) / (sqrt(avg(x*x) - avg(x)*avg(x)) * sqrt(avg(y*y) - avg(y)*avg(y))) AS corr
		FROM p GROUP BY s1, s2 ORDER BY corr DESC`},
	{"drawdowns", `WITH peak AS (
			SELECT symbol, day, close,
				max(close) OVER (PARTITION BY symbol ORDER BY day ROWS UNBOUNDED PRECEDING) AS peak
			FROM prices),
		dd AS (
			SELECT symbol, day, close / peak - 1 AS drawdown,
				row_number() OVER (PARTITION BY symbol ORDER BY close / peak) AS worst
			FROM peak)
		SELECT symbol, min(drawdown) AS max_drawdown, avg(drawdown) AS avg_drawdown,
			sum(drawdown < -0.1) AS days_below_10pct, max(CASE WHEN worst = 1 THEN day END) AS worst_day
		FROM dd GROUP BY symbol ORDER BY max_drawdown`},
	{"daily report", `SELECT day,
			count(*) AS n, avg(close) AS avg_close, min(low) AS low, max(high) AS high, sum(volume) AS volume,
			avg(close - open) AS avg_move, sum(close > open) AS up, sum(close < open) AS down,
			max(close) - min(close) AS spread, total(close * volume) / sum(volume) AS vwap,
			avg(high - low) AS avg_range, group_concat(CASE WHEN close > open + 0.6 THEN symbol END) AS movers,
			lag(avg(close)) OVER w AS prev_avg,
			avg(close) / lag(avg(close)) OVER w - 1 AS change,
			avg(avg(close)) OVER (w ROWS 6 PRECEDING) AS avg7,
			avg(avg(close)) OVER (w ROWS 29 PRECEDING) AS avg30,
			max(max(high)) OVER (w ROWS 29 PRECEDING) AS high30,
			min(min(low)) OVER (w ROWS 29 PRECEDING) AS low30,
			sum(sum(volume)) OVER (w ROWS 6 PRECEDING) AS volume7,
			rank() OVER (ORDER BY sum(volume) DESC) AS volume_rank,
			percent_rank() OVER (ORDER BY avg(close)) AS price_pct,
			row_number() OVER w AS day_no,
			lead(avg(close)) OVER w AS next_avg
		FROM prices GROUP BY day WINDOW w AS (ORDER BY day) ORDER BY day`},
}

// TestAnalyticQueries runs each analytic query on 100,000 rows in a
// worker of its own, and logs its time and the worker's peak memory.
func TestAnalyticQueries(t *testing.T) {
	path := openDB(t, 50, 2000)
	for _, a := range analytic {
		p := newPool(t, path, Options{})
		start := time.Now()
		r, err := query(t, p, 10*time.Second, a.q)
		if err != nil {
			t.Errorf("%s: %v", a.name, err)
			continue
		}
		t.Logf("%-20s %2d columns, %6d rows, %v, peak %d MiB", a.name, len(r.Columns), r.Total,
			time.Since(start).Round(time.Millisecond), p.peak()>>20)
		p.Close()
	}
}

func BenchmarkQuery(b *testing.B) {
	path := openDB(b, 3, 10)
	const q = "SELECT symbol, close FROM prices WHERE symbol = 'S001' AND day = '2000-01-05'"
	b.Run("worker", func(b *testing.B) {
		p := newPool(b, path, Options{})
		ctx := context.Background()
		for b.Loop() {
			if _, err := p.Query(ctx, Request{Query: q, Max: 10}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("in-process", func(b *testing.B) {
		db, err := sql.Open("sqlite", dsn(path))
		if err != nil {
			b.Fatal(err)
		}
		defer db.Close()
		db.SetMaxOpenConns(1)
		g := sqlguard.New()
		for b.Loop() {
			if _, err := runQuery(db, g, Request{Query: q, Max: 10}); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkStart is a new worker's first query.
func BenchmarkStart(b *testing.B) {
	path := openDB(b, 3, 10)
	for b.Loop() {
		p := New(path, Options{})
		if _, err := p.Query(context.Background(), Request{Query: "SELECT 1", Max: 1}); err != nil {
			b.Fatal(err)
		}
		p.Close()
	}
}

func BenchmarkAnalytic(b *testing.B) {
	path := openDB(b, 50, 2000)
	p := newPool(b, path, Options{})
	for _, a := range analytic {
		b.Run(strings.ReplaceAll(a.name, " ", "_"), func(b *testing.B) {
			for b.Loop() {
				if _, err := p.Query(context.Background(), Request{Query: a.q, Max: 100}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
