package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const schema = `
CREATE TABLE prices(coin TEXT NOT NULL, date TEXT NOT NULL, price REAL, volume REAL, source TEXT, _run_id INTEGER, _fetched_at TEXT,
  PRIMARY KEY (coin, date));
CREATE INDEX prices_date ON prices(date);
CREATE TABLE notes(id INTEGER PRIMARY KEY, chat TEXT, text TEXT, updated TEXT);
CREATE TABLE _burrow_changes(id INTEGER PRIMARY KEY, at TEXT, source TEXT, target TEXT, op TEXT, before TEXT, after TEXT);
CREATE TABLE _burrow_change_rows(change_id INTEGER NOT NULL, table_name TEXT NOT NULL, row_key TEXT NOT NULL, kind TEXT, before TEXT);
CREATE INDEX _burrow_change_rows_key ON _burrow_change_rows(table_name, row_key, change_id);
CREATE INDEX _burrow_change_rows_change ON _burrow_change_rows(change_id);
`

// seed adds one year of prices for 450 coins (164,250 rows) and 1,000 notes.
func seed(db *sql.DB) {
	exec(db, schema)
	exec(db, `WITH RECURSIVE c(i) AS (SELECT 0 UNION ALL SELECT i+1 FROM c WHERE i < 449),
		d(j) AS (SELECT 0 UNION ALL SELECT j+1 FROM d WHERE j < 364)
		INSERT INTO prices SELECT 'C' || i, date('2026-01-01', '+' || j || ' days'), 100 + i + j * 0.25, 1e6 + j,
		'coingecko.com/api/v3', 0, '2026-09-27T08:00:03Z' FROM c, d`)
	exec(db, `WITH RECURSIVE s(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM s WHERE i < 1000)
		INSERT INTO notes SELECT i, 'chat1', hex(zeroblob(250)), '2026-09-27T08:00:03Z' FROM s`)
}

var seedDay0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
var newDay0 = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

const (
	upsertPrice = `INSERT INTO prices VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(coin, date) DO UPDATE SET price = excluded.price,
		volume = excluded.volume, source = excluded.source, _run_id = excluded._run_id, _fetched_at = excluded._fetched_at`
	beforePrice = `SELECT json_object('coin', coin, 'date', date, 'price', price, 'volume', volume, 'source', source,
		'_run_id', _run_id, '_fetched_at', _fetched_at) FROM prices WHERE coin = ? AND date = ?`
	insertChange    = "INSERT INTO _burrow_changes(at, source, target, op) VALUES (?, ?, ?, 'upsert')"
	insertChangeRow = "INSERT INTO _burrow_change_rows VALUES (?, ?, ?, ?, ?)"
)

// load is the writer side of the workload. Only the writer goroutine uses it.
type load struct {
	w        *sql.DB
	runID    int
	newRows  int
	maxNote  int
	rows     int
	requests int
}

// pipelineChunk upserts 500 rows (150 corrections of existing rows, 350 new) with change log and before-images.
func (l *load) pipelineChunk() {
	l.runID++
	tx, err := l.w.Begin()
	must(err)
	defer tx.Rollback()
	at := now()
	res := exec(tx, insertChange, at, fmt.Sprint("run:", l.runID), "table:prices")
	cid, _ := res.LastInsertId()
	get, _ := tx.Prepare(beforePrice)
	up, _ := tx.Prepare(upsertPrice)
	cr, _ := tx.Prepare(insertChangeRow)
	for i := 0; i < 500; i++ {
		var coin, date string
		if i < 150 {
			coin, date = fmt.Sprintf("C%d", rand.N(450)), seedDay0.AddDate(0, 0, rand.N(365)).Format("2006-01-02")
		} else {
			coin, date = fmt.Sprintf("C%d", l.newRows%450), newDay0.AddDate(0, 0, l.newRows/450).Format("2006-01-02")
			l.newRows++
		}
		var before sql.NullString
		get.QueryRow(coin, date).Scan(&before) // no row: before stays NULL
		_, err := up.Exec(coin, date, rand.Float64()*1e5, rand.Float64()*1e9, "coingecko.com/api/v3", l.runID, at)
		must(err)
		kind := "i"
		if before.Valid {
			kind = "u"
		}
		_, err = cr.Exec(cid, "prices", fmt.Sprintf(`["%s","%s"]`, coin, date), kind, before)
		must(err)
	}
	must(tx.Commit())
	l.rows += 500
	l.requests++
}

// chatWrite is one chat turn: add a note (70%) or edit one, with change log.
func (l *load) chatWrite() {
	tx, err := l.w.Begin()
	must(err)
	defer tx.Rollback()
	at := now()
	res := exec(tx, insertChange, at, fmt.Sprint("message:", l.requests), "table:notes")
	cid, _ := res.LastInsertId()
	var id int
	if rand.N(10) < 7 {
		l.maxNote++
		id = l.maxNote
	} else {
		id = 1 + rand.N(l.maxNote)
	}
	var before sql.NullString
	tx.QueryRow("SELECT json_object('id', id, 'chat', chat, 'text', text, 'updated', updated) FROM notes WHERE id = ?", id).Scan(&before)
	exec(tx, "INSERT INTO notes VALUES (?, 'chat1', ?, ?) ON CONFLICT(id) DO UPDATE SET text = excluded.text, updated = excluded.updated",
		id, strings.Repeat("t", 500), at)
	kind := "i"
	if before.Valid {
		kind = "u"
	}
	exec(tx, insertChangeRow, cid, "notes", fmt.Sprintf("[%d]", id), kind, before)
	must(tx.Commit())
	l.rows++
	l.requests++
}

// reader runs view-like queries: read all rows, close, pause 50–150 ms.
func reader(ctx context.Context, r *sql.DB, lat *latencies) {
	for ctx.Err() == nil {
		from := seedDay0.AddDate(0, 0, rand.N(335))
		var q string
		var args []any
		switch rand.N(4) {
		case 0: // a table page
			q, args = "SELECT coin, date, price, volume FROM prices ORDER BY date DESC, coin LIMIT 50 OFFSET ?", []any{rand.N(2000)}
		case 1: // a chart
			q, args = "SELECT date, price FROM prices WHERE coin = ? AND date >= ? ORDER BY date", []any{fmt.Sprintf("C%d", rand.N(450)), from.Format("2006-01-02")}
		case 2: // a KPI over 30 days
			q, args = "SELECT count(*), avg(price), max(price) FROM prices WHERE date >= ? AND date < ?",
				[]any{from.Format("2006-01-02"), from.AddDate(0, 0, 30).Format("2006-01-02")}
		default: // the notes list
			q = "SELECT id, chat, text FROM notes ORDER BY id DESC LIMIT 20"
		}
		start := clock()
		rows, err := r.QueryContext(ctx, q, args...)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			panic(err)
		}
		for rows.Next() {
		}
		rows.Close()
		lat.add(since(start))
		select {
		case <-ctx.Done():
		case <-time.After(time.Duration(50+rand.N(100)) * time.Millisecond):
		}
	}
}

// holder opens a read transaction, keeps it for hold, closes it, waits a second, and repeats.
func holder(ctx context.Context, r *sql.DB, hold time.Duration, holds *int) {
	bg := context.Background()
	for ctx.Err() == nil {
		c, err := r.Conn(bg)
		must(err)
		_, err = c.ExecContext(bg, "BEGIN")
		must(err)
		var n int
		must(c.QueryRowContext(bg, "SELECT count(*) FROM notes").Scan(&n))
		select {
		case <-ctx.Done():
		case <-time.After(hold):
		}
		c.ExecContext(bg, "ROLLBACK")
		c.Close()
		*holds++
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

type scenario struct {
	id, name string
	dur      time.Duration
	holder   bool
	pragmas  []string
	bucket   time.Duration // interval for the WAL-over-time line
}

type walResult struct {
	sc                    scenario
	rows, requests, holds int
	walMax, walEnd, dbEnd int64
	chunk, chat, read     *latencies
	idleCkpt              time.Duration
	idleRes               [3]int
	walAfterIdle          int64
	series                []int64 // WAL size once per second
}

func walScenarios(work string, d1, d2 time.Duration) {
	scs := []scenario{
		{"W1", "4 readers, 1 writer", d1, false, nil, 5 * time.Minute},
		{"W2", "W1 + a reader holding a read transaction for 60 s at a time", d2, true, nil, time.Minute},
		{"W3", "W2 + `journal_size_limit = 64 MB`", d2, true, []string{"journal_size_limit(67108864)"}, time.Minute},
	}
	fmt.Println("\n## 1–2. WAL size under constant use\n")
	fmt.Println("Writer: every 2 s a 500-row pipeline chunk (150 corrections, 350 new rows) and every 0.5 s a chat turn that adds or edits a note, all with change log and before-images.")
	fmt.Println("Readers: 4 goroutines running table pages, charts, 30-day KPIs and the notes list, each followed by a 50–150 ms pause. Automatic checkpoint at the default 1,000 pages.\n")
	var results []walResult
	for i, sc := range scs {
		logf("%s: %s for %v", sc.id, sc.name, sc.dur)
		results = append(results, runWAL(filepath.Join(work, fmt.Sprintf("wal%d.db", i)), sc))
	}
	fmt.Println("| Scenario | Duration | Rows written | WAL max | WAL at end | Database at end | Write, 500-row chunk p50 / p99 / max | Chat write p50 / p99 / max | Read p50 / p99 / max |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|")
	for _, r := range results {
		fmt.Printf("| %s: %s | %v | %d | %s | %s | %s | %s | %s | %s |\n", r.sc.id, r.sc.name, r.sc.dur, r.rows,
			mb(r.walMax), mb(r.walEnd), mb(r.dbEnd), r.chunk, r.chat, r.read)
	}
	fmt.Println("\nWAL file size over time (highest value in each interval):\n")
	for _, r := range results {
		var parts []string
		step := int(r.sc.bucket / time.Second)
		for i := 0; i < len(r.series); i += step {
			parts = append(parts, fmt.Sprintf("%.0f", float64(slices.Max(r.series[i:min(i+step, len(r.series))]))/(1<<20)))
		}
		fmt.Printf("- **%s**, per %v, MB: %s", r.sc.id, r.sc.bucket, strings.Join(parts, ", "))
		if r.sc.holder {
			fmt.Printf(" (%d holds of 60 s)", r.holds)
		}
		fmt.Println()
	}
	fmt.Println("\n## 3a. `wal_checkpoint(TRUNCATE)` when the load stops (idle)\n")
	fmt.Println("| Scenario | WAL before | Time | Result (busy, log, checkpointed) | WAL after |")
	fmt.Println("|---|---|---|---|---|")
	for _, r := range results {
		fmt.Printf("| %s | %s | %s | %v | %s |\n", r.sc.id, mb(r.walEnd), ms(r.idleCkpt), r.idleRes, mb(r.walAfterIdle))
	}
}

func runWAL(path string, sc scenario) walResult {
	w := openWriter(path, sc.pragmas...)
	defer w.Close()
	seed(w)
	checkpoint(w, "TRUNCATE")
	var nNotes int
	w.QueryRow("SELECT max(id) FROM notes").Scan(&nNotes)
	r := openReaders(path, 5)
	defer r.Close()

	res := walResult{sc: sc, chunk: &latencies{}, chat: &latencies{}, read: &latencies{}}
	l := &load{w: w, maxNote: nNotes}
	ctx, cancel := context.WithTimeout(context.Background(), sc.dur)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // writer
		defer wg.Done()
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for n := 0; ; n++ {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			if n%20 == 0 {
				s := clock()
				l.pipelineChunk()
				res.chunk.add(since(s))
			}
			if n%5 == 0 {
				s := clock()
				l.chatWrite()
				res.chat.add(since(s))
			}
		}
	}()
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); reader(ctx, r, res.read) }()
	}
	if sc.holder {
		wg.Add(1)
		go func() { defer wg.Done(); holder(ctx, r, time.Minute, &res.holds) }()
	}
	wg.Add(1)
	go func() { // sampler
		defer wg.Done()
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		last := clock()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			res.series = append(res.series, walSize(path))
			if since(last) >= time.Minute {
				logf("  %s: WAL %s, %d rows written", sc.id, mb(walSize(path)), l.rows)
				last = clock()
			}
		}
	}()
	wg.Wait()

	res.rows, res.requests = l.rows, l.requests
	res.walMax = slices.Max(res.series)
	res.walEnd = walSize(path)
	res.dbEnd = fileSize(path)
	res.idleCkpt, res.idleRes = checkpoint(w, "TRUNCATE")
	res.walAfterIdle = walSize(path)
	return res
}

// ---------------------------------------------------------------- 3. checkpoint modes

func checkpoints(work string) {
	fmt.Println("\n## 3. Checkpoint modes on a large WAL\n")
	fmt.Println(`Automatic checkpoints are turned off while the WAL is filled with 500-row updates of prices. "Reader holding" means a read transaction was opened before the WAL was filled and is still open.` + "\n")
	fmt.Println("| WAL | Case | Time | Result (busy, log, checkpointed) | WAL file after |")
	fmt.Println("|---|---|---|---|---|")
	for _, size := range []int64{64 << 20, 256 << 20} {
		path := filepath.Join(work, fmt.Sprintf("ckpt-%d.db", size>>20))
		w := openWriter(path, "wal_autocheckpoint(0)")
		seed(w)
		r := openReaders(path, 2)
		fill := func() {
			checkpoint(w, "TRUNCATE")
			for walSize(path) < size {
				tx, _ := w.Begin()
				st, _ := tx.Prepare("UPDATE prices SET price = ? WHERE coin = ? AND date = ?")
				for i := 0; i < 500; i++ {
					st.Exec(rand.Float64()*1e5, fmt.Sprintf("C%d", rand.N(450)), seedDay0.AddDate(0, 0, rand.N(365)).Format("2006-01-02"))
				}
				must(tx.Commit())
			}
		}
		row := func(c string, d time.Duration, res [3]int) {
			fmt.Printf("| %s | %s | %s | %v | %s |\n", mb(size), c, ms(d), res, mb(walSize(path)))
		}
		logf("checkpoints on a %s WAL", mb(size))

		fill()
		d, res := checkpoint(w, "PASSIVE")
		row("PASSIVE, no reader", d, res)
		exec(w, "UPDATE notes SET updated = ? WHERE id = 1", now())
		fmt.Printf("| %s | … then one small write | — | — | %s |\n", mb(size), mb(walSize(path)))
		fill()
		exec(w, "PRAGMA journal_size_limit = 67108864")
		checkpoint(w, "PASSIVE")
		exec(w, "UPDATE notes SET updated = ? WHERE id = 1", now())
		fmt.Printf("| %s | PASSIVE with `journal_size_limit = 64 MB`, then one small write | — | — | %s |\n", mb(size), mb(walSize(path)))
		exec(w, "PRAGMA journal_size_limit = -1")

		fill()
		d, res = checkpoint(w, "TRUNCATE")
		row("TRUNCATE, no reader", d, res)

		bg := context.Background()
		c, _ := r.Conn(bg)
		_, err := c.ExecContext(bg, "BEGIN")
		must(err)
		var n int
		must(c.QueryRowContext(bg, "SELECT count(*) FROM notes").Scan(&n))
		fill()
		d, res = checkpoint(w, "PASSIVE")
		row("PASSIVE, reader holding", d, res)
		d, res = checkpoint(w, "TRUNCATE")
		row("TRUNCATE, reader holding (waits for `busy_timeout`)", d, res)
		start := clock()
		rows, err := r.Query("SELECT coin, date, price FROM prices WHERE coin = 'C1'")
		must(err)
		for rows.Next() {
		}
		rows.Close()
		fmt.Printf("| %s | a new read while the WAL is full | %s | — | — |\n", mb(size), ms(since(start)))
		c.ExecContext(bg, "ROLLBACK")
		c.Close()
		d, res = checkpoint(w, "TRUNCATE")
		row("TRUNCATE after the reader closed", d, res)
		r.Close()
		w.Close()
	}
}
