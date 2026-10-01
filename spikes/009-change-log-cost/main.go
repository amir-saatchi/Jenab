// SPIKE-009: cost of the change log and undo (SPEC 2.5, 2.6).
//
// Usage: go run .        prints markdown tables
package main

import (
	"database/sql"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func open(path string, l layout) *sql.DB {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	must(err)
	db.SetMaxOpenConns(1) // the writer connection; also keeps the temp table on one connection
	exec(db, baseSchema)
	exec(db, l.ddl)
	return db
}

func main() {
	wd, _ := os.Getwd()
	work := filepath.Join(wd, "work")
	os.RemoveAll(work)
	must(os.MkdirAll(work, 0o755))
	defer os.RemoveAll(work)

	fmt.Println("# SPIKE-009 results\n")
	fmt.Println("modernc.org/sqlite v1.59.0, WAL, `synchronous = NORMAL`. Before-images are stored per row in `_burrow_change_rows.before` as JSON.")
	for i, l := range layouts {
		fmt.Printf("\n# Layout %s\n", l.name)
		db := open(filepath.Join(work, fmt.Sprintf("p%d.db", i)), l)
		seedPrices(db, 100, 1000)
		writeCost(db, "default page cache (2 MB)")
		exec(db, "PRAGMA cache_size = -65536")
		writeCost(db, "writer page cache 64 MB")
		undoCost(db)
		atScale(db)
		db.Close()
		for c := range cleanups {
			year(filepath.Join(work, fmt.Sprintf("year%d-%d.db", i, c)), l, c)
		}
	}
}

// ---------------------------------------------------------------- data

func seedPrices(db *sql.DB, coins, days int) {
	exec(db, `WITH RECURSIVE c(i) AS (SELECT 0 UNION ALL SELECT i+1 FROM c WHERE i < ?),
		d(j) AS (SELECT 0 UNION ALL SELECT j+1 FROM d WHERE j < ?)
		INSERT INTO prices SELECT 'C' || i, date('2023-01-01', '+' || j || ' days'), 100 + i + j * 0.25, 1e6 + j,
		'coingecko.com/api/v3', 0, '2026-09-27T08:00:03Z' FROM c, d`, coins-1, days-1)
}

var trial int

// batch makes n rows: a share of them update random existing base rows, the rest are new.
func batch(n int, updateShare float64, runID int) [][]any {
	trial++
	rows := make([][]any, 0, n)
	seen := map[string]bool{}
	nUpd := int(float64(n) * updateShare)
	for len(rows) < nUpd {
		coin := fmt.Sprintf("C%d", rand.N(100))
		date := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, rand.N(1000)).Format("2006-01-02")
		if seen[coin+date] {
			continue
		}
		seen[coin+date] = true
		rows = append(rows, []any{coin, date, rand.Float64() * 1e5, rand.Float64() * 1e9, "coingecko.com/api/v3", runID, now()})
	}
	for i := 0; len(rows) < n; i++ {
		rows = append(rows, []any{fmt.Sprintf("T%d", trial), fmt.Sprintf("%06d", i), rand.Float64() * 1e5, rand.Float64() * 1e9, "coingecko.com/api/v3", runID, now()})
	}
	return rows
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// run writes rows in 500-row chunks and returns the change IDs.
func run(db *sql.DB, rows [][]any, m mode, source string) []int64 {
	var ids []int64
	for i := 0; i < len(rows); i += 500 {
		ids = append(ids, writeChunk(db, prices, rows[i:min(i+500, len(rows))], m, source, now()))
	}
	return ids
}

func snapshot(db *sql.DB) map[string]string {
	rows, err := db.Query("SELECT json_array(coin, date), json_array(price, volume, source, _run_id, _fetched_at) FROM prices")
	must(err)
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		m[k] = v
	}
	return m
}

func diff(got, want map[string]string) int {
	n := 0
	for k, v := range want {
		if got[k] != v {
			n++
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			n++
		}
	}
	return n
}

func median(d []time.Duration) time.Duration {
	s := slices.Clone(d)
	slices.Sort(s)
	return s[len(s)/2]
}

func ms(d time.Duration) string {
	if d < 10*time.Millisecond {
		return fmt.Sprintf("%.2f ms", float64(d.Microseconds())/1000)
	}
	return fmt.Sprintf("%.0f ms", float64(d.Microseconds())/1000)
}

// ---------------------------------------------------------------- 1. write cost

func writeCost(db *sql.DB, cache string) {
	fmt.Printf("\n## 1. Write cost: 5,000-row upsert in 500-row chunks, %s\n\n", cache)
	fmt.Println("Table `prices` with 100,000 rows. Median of 7 runs; the modes are interleaved.\n")
	fmt.Println("| Rows that update existing rows | Plain upsert | + row keys | + row keys and before-images | Before-images vs plain |")
	fmt.Println("|---|---|---|---|---|")
	for _, share := range []float64{0, 0.5, 1} {
		times := make([][]time.Duration, 3)
		for rep := 0; rep < 7; rep++ {
			for _, m := range []mode{modePlain, modeKeys, modeBefore} {
				rows := batch(5000, share, rep)
				start := time.Now()
				run(db, rows, m, "run:bench")
				times[m] = append(times[m], time.Since(start))
			}
		}
		p, k, b := median(times[0]), median(times[1]), median(times[2])
		fmt.Printf("| %.0f%% | %s | %s | %s | +%.0f%% |\n", share*100, ms(p), ms(k), ms(b), (float64(b)/float64(p)-1)*100)
	}
}

// ---------------------------------------------------------------- 2. undo

func undoCost(db *sql.DB) {
	fmt.Println("\n## 2. Undo of that upsert (50% updates)\n")
	fmt.Println("| Case | Conflict check | Undo | Rows restored correctly |")
	fmt.Println("|---|---|---|---|")

	// a) no conflicts
	pre := snapshot(db)
	ids := run(db, batch(5000, 0.5, 1), modeBefore, "run:1")
	start := time.Now()
	c := conflicts(db, ids)
	tc := time.Since(start)
	start = time.Now()
	uid := undo(db, prices, ids, c, "undo:run:1", now())
	tu := time.Since(start)
	fmt.Printf("| Revert run, no conflicts | %s (%d found) | %s | %s |\n", ms(tc), len(c), ms(tu), check(diff(snapshot(db), pre)))

	// Redo: undoing the undo brings the run back.
	start = time.Now()
	undo(db, prices, []int64{uid}, nil, "undo:undo", now())
	tr := time.Since(start)
	undo(db, prices, []int64{uid + 1}, nil, "undo:redo", now()) // and back again, to the pre-run state
	fmt.Printf("| Redo (undo the undo), then undo again | — | %s | %s |\n", ms(tr), check(diff(snapshot(db), pre)))

	// b) with conflicts: a later change from another chat touches 100 of the run's rows.
	pre = snapshot(db)
	rows := batch(5000, 0.5, 2)
	ids = run(db, rows, modeBefore, "run:2")
	var later [][]any
	for _, r := range append(slices.Clone(rows[:50]), rows[4950:]...) { // 50 updated + 50 inserted rows
		later = append(later, []any{r[0], r[1], 1.0, 2.0, "manual edit", 0, now()})
	}
	writeChunk(db, prices, later, modeBefore, "message:42", now())
	afterLater := snapshot(db)
	start = time.Now()
	c = conflicts(db, ids)
	tc = time.Since(start)
	start = time.Now()
	undo(db, prices, ids, c, "undo:run:2", now())
	tu = time.Since(start)
	want := map[string]string{}
	for k, v := range pre {
		want[k] = v
	}
	for _, k := range c { // skipped rows keep the later value
		want[k] = afterLater[k]
	}
	fmt.Printf("| Revert run, 100 rows changed later (skipped) | %s (%d found) | %s | %s |\n", ms(tc), len(c), ms(tu), check(diff(snapshot(db), want)))

	// c) undo a chat turn: 3 rows, repeated because one run is too short to time
	pre = snapshot(db)
	var tcs, tus []time.Duration
	for i := 0; i < 50; i++ {
		ids = []int64{writeChunk(db, prices, batch(3, 0.67, 0), modeBefore, "message:43", now())}
		start = time.Now()
		c = conflicts(db, ids)
		tcs = append(tcs, time.Since(start))
		start = time.Now()
		undo(db, prices, ids, c, "undo:turn", now())
		tus = append(tus, time.Since(start))
	}
	fmt.Printf("| Undo turn, 3 rows (median of 50) | %s | %s | %s |\n", ms(median(tcs)), ms(median(tus)), check(diff(snapshot(db), pre)))
}

func check(nDiff int) string {
	if nDiff == 0 {
		return "yes"
	}
	return fmt.Sprintf("**no: %d rows differ**", nDiff)
}

// ---------------------------------------------------------------- 3. at scale

func atScale(db *sql.DB) {
	var n int64
	db.QueryRow("SELECT count(*) FROM _burrow_change_rows").Scan(&n)
	// History for other rows: 2,000 older-looking changes of 500 rows, 20% with a before-image.
	start := time.Now()
	for n < 1_000_000 {
		tx, _ := db.Begin()
		res := exec(tx, "INSERT INTO _burrow_changes(at, source, target, op) VALUES (?, 'run:history', 'table:prices', 'upsert')", now())
		cid, _ := res.LastInsertId()
		exec(tx, `WITH RECURSIVE s(i) AS (SELECT 0 UNION ALL SELECT i+1 FROM s WHERE i < 499)
			INSERT INTO _burrow_change_rows SELECT ?, 'prices', json_array('H' || ?, printf('%06d', i)),
			CASE WHEN i % 5 = 0 THEN 'u' ELSE 'i' END,
			CASE WHEN i % 5 = 0 THEN json_object('coin', 'H', 'date', '2026-01-01', 'price', 123.45, 'volume', 1e9,
				'source', 'coingecko.com/api/v3', '_run_id', 7, '_fetched_at', '2026-09-27T08:00:03Z') END FROM s`, cid, cid)
		must(tx.Commit())
		n += 500
	}
	fill := time.Since(start)
	db.QueryRow("SELECT count(*) FROM _burrow_change_rows").Scan(&n)

	fmt.Printf("\n## 3. Conflict check with %d row entries in the change log\n\n", n)
	fmt.Printf("(Filling the history took %v.)\n\n", fill.Round(time.Millisecond))
	fmt.Println("| Case | Conflict check (median of 5) |")
	fmt.Println("|---|---|")
	rows := batch(5000, 0.5, 3)
	ids := run(db, rows, modeBefore, "run:3")
	var t []time.Duration
	for i := 0; i < 5; i++ {
		start := time.Now()
		conflicts(db, ids)
		t = append(t, time.Since(start))
	}
	fmt.Printf("| Revert a 5,000-row run, no conflicts | %s |\n", ms(median(t)))
	var later [][]any
	for _, r := range rows[:100] {
		later = append(later, []any{r[0], r[1], 1.0, 2.0, "manual edit", 0, now()})
	}
	writeChunk(db, prices, later, modeBefore, "message:44", now())
	t = nil
	var found int
	for i := 0; i < 5; i++ {
		start := time.Now()
		found = len(conflicts(db, ids))
		t = append(t, time.Since(start))
	}
	fmt.Printf("| Revert a 5,000-row run, 100 conflicts | %s (%d found) |\n", ms(median(t)), found)
	turn := []int64{writeChunk(db, prices, batch(3, 0.67, 0), modeBefore, "message:45", now())}
	const reps = 200
	start = time.Now()
	for i := 0; i < reps; i++ {
		conflicts(db, turn)
	}
	fmt.Printf("| Undo a 3-row turn (average of %d) | %s |\n", reps, ms(time.Since(start)/reps))
	sizes(db, "\nSize at this point")
}

// ---------------------------------------------------------------- 4. one year

var cleanups = []string{
	"no cleanup",
	"after 90 days, drop before-images, keep row entries",
	"after 90 days, drop before-images and row entries, keep a summary on the change",
}

func year(path string, l layout, cleanup int) {
	db := open(path, l)
	defer db.Close()
	exec(db, "INSERT INTO _burrow_memory VALUES ('project', 'goals', ?, 1)", strings.Repeat("m", 1000))
	exec(db, "INSERT INTO _burrow_views VALUES ('price_table', ?, 1)", strings.Repeat("v", 2000))
	day0 := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	noteID := 0
	var yesterday [][]any
	var cleanTimes []time.Duration
	watermark := int64(0)
	start := time.Now()
	for d := 0; d < 365; d++ {
		day := day0.AddDate(0, 0, d)
		at := day.Format(time.RFC3339)
		// Pipeline: 450 new rows (one per coin) + 50 corrections of yesterday's rows.
		var rows [][]any
		for c := 0; c < 450; c++ {
			rows = append(rows, []any{fmt.Sprintf("C%d", c), day.Format("2006-01-02"), rand.Float64() * 1e5, rand.Float64() * 1e9, "coingecko.com/api/v3", d, at})
		}
		for i := 0; i < 50 && i < len(yesterday); i++ {
			r := slices.Clone(yesterday[i*9])
			r[2] = rand.Float64() * 1e5
			rows = append(rows, r)
		}
		writeChunk(db, prices, rows, modeBefore, fmt.Sprint("run:", d), at)
		yesterday = rows[:450]
		// 20 chat turns: 14 add two notes, 3 edit a note, 2 edit memory, 1 saves a view.
		for t := 0; t < 20; t++ {
			src := fmt.Sprintf("message:%d-%d", d, t)
			switch {
			case t < 14:
				noteID += 2
				writeChunk(db, notes, [][]any{{noteID - 1, "chat1", strings.Repeat("n", 200)}, {noteID, "chat1", strings.Repeat("o", 200)}}, modeBefore, src, at)
			case t < 17:
				writeChunk(db, notes, [][]any{{rand.N(noteID) + 1, "chat1", strings.Repeat("e", 200)}}, modeBefore, src, at)
			case t < 19:
				writeConfig(db, "memory", "goals", strings.Repeat(string(rune('a'+t)), 1000), src, at)
			default:
				writeConfig(db, "view", "price_table", strings.Repeat(string(rune('a'+d%26)), 2000), src, at)
			}
		}
		// Daily cleanup: drop before-images older than 90 days (from the last watermark on).
		if cleanup > 0 {
			s := time.Now()
			var cut sql.NullInt64
			db.QueryRow("SELECT max(id) FROM _burrow_changes WHERE at < ?", day.AddDate(0, 0, -90).Format(time.RFC3339)).Scan(&cut)
			if cut.Valid && cut.Int64 > watermark {
				tx, _ := db.Begin()
				if cleanup == 1 {
					exec(tx, "UPDATE _burrow_change_rows SET before = NULL WHERE change_id > ? AND change_id <= ? AND before IS NOT NULL", watermark, cut.Int64)
				} else {
					exec(tx, `UPDATE _burrow_changes SET after = (SELECT json_object('rows', count(*), 'inserted', sum(kind = 'i'), 'updated', sum(kind = 'u'))
						FROM _burrow_change_rows WHERE change_id = _burrow_changes.id) WHERE id > ? AND id <= ?`, watermark, cut.Int64)
					exec(tx, "DELETE FROM _burrow_change_rows WHERE change_id > ? AND change_id <= ?", watermark, cut.Int64)
				}
				exec(tx, "UPDATE _burrow_changes SET before = NULL WHERE id > ? AND id <= ? AND before IS NOT NULL", watermark, cut.Int64)
				must(tx.Commit())
				watermark = cut.Int64
			}
			cleanTimes = append(cleanTimes, time.Since(s))
		}
	}
	elapsed := time.Since(start)
	exec(db, "PRAGMA wal_checkpoint(TRUNCATE)")
	fmt.Printf("\n## 4. One simulated year: %s\n\n", cleanups[cleanup])
	fmt.Println("Per day: one pipeline run of 500 rows (450 new, 50 corrections) and 20 chat turns (14 add two notes, 3 edit a note, 2 edit a 1 KB memory section, 1 saves a 2 KB view config).")
	fmt.Printf("Simulated in %v.", elapsed.Round(time.Millisecond))
	if cleanup > 0 {
		fmt.Printf(" Daily cleanup: median %s, max %s.", ms(median(cleanTimes)), ms(slices.Max(cleanTimes)))
	}
	fmt.Println()
	sizes(db, "")
}

// sizes prints space per table (indexes included) from dbstat.
func sizes(db *sql.DB, title string) {
	rows, err := db.Query(`SELECT coalesce(s.tbl_name, d.name), sum(d.pgsize) FROM dbstat d
		LEFT JOIN sqlite_schema s ON s.name = d.name GROUP BY 1`)
	must(err)
	per := map[string]int64{}
	var total int64
	for rows.Next() {
		var n string
		var b int64
		rows.Scan(&n, &b)
		per[n] = b
		total += b
	}
	rows.Close()
	var free, pageSize int64
	db.QueryRow("PRAGMA freelist_count").Scan(&free)
	db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	var counts [3]int64
	db.QueryRow("SELECT (SELECT count(*) FROM _burrow_changes), (SELECT count(*) FROM _burrow_change_rows), (SELECT count(*) FROM _burrow_change_rows WHERE before IS NOT NULL)").Scan(&counts[0], &counts[1], &counts[2])

	if title != "" {
		fmt.Println(title + ":\n")
	} else {
		fmt.Println()
	}
	fmt.Println("| Table (with its indexes) | Size | Share |")
	fmt.Println("|---|---|---|")
	var names []string
	for n := range per {
		names = append(names, n)
	}
	sort.Slice(names, func(a, b int) bool { return per[names[a]] > per[names[b]] })
	log := per["_burrow_changes"] + per["_burrow_change_rows"]
	for _, n := range names {
		if per[n] < 64*1024 {
			continue
		}
		fmt.Printf("| %s | %.1f MB | %.0f%% |\n", n, float64(per[n])/(1<<20), float64(per[n])*100/float64(total))
	}
	fmt.Printf("| **Change log total** | **%.1f MB** | **%.0f%%** |\n", float64(log)/(1<<20), float64(log)*100/float64(total))
	fmt.Printf("| Database (used pages) / free pages | %.1f MB / %.1f MB | |\n", float64(total)/(1<<20), float64(free*pageSize)/(1<<20))
	fmt.Printf("\nChange entries: %d. Row entries: %d, of which %d still hold a before-image.\n", counts[0], counts[1], counts[2])
}
