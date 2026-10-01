package main

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const bigSchema = `
CREATE TABLE trades(id INTEGER PRIMARY KEY, coin TEXT NOT NULL, date TEXT NOT NULL, seq INTEGER NOT NULL,
  price REAL, volume REAL, source TEXT, note TEXT);
CREATE INDEX trades_coin_date ON trades(coin, date);
`

// fillTrades adds n rows of about 100 bytes; (coin, date, seq) is unique.
func fillTrades(w *sql.DB, n int) {
	for done := 0; done < n; done += 100_000 {
		exec(w, `WITH RECURSIVE s(i) AS (SELECT ? UNION ALL SELECT i+1 FROM s WHERE i < ?)
			INSERT INTO trades(coin, date, seq, price, volume, source, note)
			SELECT 'C' || (i % 450), date('2020-01-01', '+' || (i / 450 % 3650) || ' days'), i / 1642500,
			100 + (i % 997) * 0.5, 1e6 + i, 'coingecko.com/api/v3', hex(randomblob(12)) FROM s`, done, min(done+100_000, n)-1)
	}
}

// fillRate measures 500-row insert transactions into a growing indexed table, per writer page cache size.
func fillRate(work string, n int) {
	fmt.Printf("\n## 6. Chunked inserts into a growing indexed table, %d rows\n\n", n)
	fmt.Println("500-row transactions into `trades`: rows arrive in rowid order, but the index on `(coin, date)` is written in random order. Rows per second in each million.\n")
	var head []string
	for m := 0; m < n/1_000_000; m++ {
		head = append(head, fmt.Sprintf("%d–%dM", m, m+1))
	}
	fmt.Printf("| Writer page cache | %s | Total time |\n|---|%s---|\n", strings.Join(head, " | "), strings.Repeat("---|", len(head)))
	for _, cache := range []int{2, 64} {
		path := filepath.Join(work, fmt.Sprintf("fill-%d.db", cache))
		w := openWriter(path)
		exec(w, fmt.Sprintf("PRAGMA cache_size = %d", -cache*1024))
		exec(w, bigSchema)
		logf("fill %d rows with a %d MB cache", n, cache)
		var rates []string
		start := clock()
		for m := 0; m < n; m += 1_000_000 {
			s := clock()
			for done := m; done < m+1_000_000; done += 500 {
				exec(w, `WITH RECURSIVE s(i) AS (SELECT ? UNION ALL SELECT i+1 FROM s WHERE i < ?)
					INSERT INTO trades(coin, date, seq, price, volume, source, note)
					SELECT 'C' || (i % 450), date('2020-01-01', '+' || (i / 450 % 3650) || ' days'), i / 1642500,
					100 + (i % 997) * 0.5, 1e6 + i, 'coingecko.com/api/v3', hex(randomblob(12)) FROM s`, done, done+499)
			}
			rates = append(rates, fmt.Sprintf("%.0f", 1e6/since(s).Seconds()))
			logf("  %dM: %s rows/s", m/1_000_000+1, rates[len(rates)-1])
		}
		fmt.Printf("| %d MB | %s | %s |\n", cache, strings.Join(rates, " | "), ms(since(start)))
		w.Close()
	}
}

type rebuildTimes struct {
	copy, swap, checks, end, total time.Duration
	tmpPeak, wal                   int64
	reads                          *latencies
	readErrors                     int
}

// rebuild changes the primary key from id to (coin, date, seq), following SQLite's documented procedure.
func rebuild(w, r *sql.DB, path, tmp string, ordered, commit bool) rebuildTimes {
	var t rebuildTimes
	t.reads = &latencies{}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i := 0; i < 2; i++ { // readers keep using the table
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, rand.N(3600))
				s := clock()
				var n int
				var avg sql.NullFloat64
				err := r.QueryRow("SELECT count(*), avg(price) FROM trades WHERE coin = ? AND date >= ? AND date < ?",
					fmt.Sprintf("C%d", rand.N(450)), from.Format("2006-01-02"), from.AddDate(0, 0, 30).Format("2006-01-02")).Scan(&n, &avg)
				if err != nil {
					mu.Lock()
					t.readErrors++
					mu.Unlock()
				}
				t.reads.add(since(s))
				time.Sleep(20 * time.Millisecond)
			}
		}()
	}
	wg.Add(1)
	go func() { // temp space sampler
		defer wg.Done()
		free0 := diskFree(tmp)
		for ctx.Err() == nil {
			t.tmpPeak = max(t.tmpPeak, free0-diskFree(tmp))
			time.Sleep(50 * time.Millisecond)
		}
	}()

	exec(w, "PRAGMA foreign_keys = OFF")
	start := clock()
	tx, err := w.Begin()
	must(err)
	exec(tx, `CREATE TABLE trades_new(coin TEXT NOT NULL, date TEXT NOT NULL, seq INTEGER NOT NULL,
		price REAL, volume REAL, source TEXT, note TEXT, PRIMARY KEY (coin, date, seq))`)
	q := "INSERT INTO trades_new SELECT coin, date, seq, price, volume, source, note FROM trades"
	if ordered {
		q += " ORDER BY coin, date, seq"
	}
	exec(tx, q)
	t.copy = since(start)
	s := clock()
	exec(tx, "DROP TABLE trades")
	exec(tx, "ALTER TABLE trades_new RENAME TO trades")
	exec(tx, "CREATE INDEX trades_date ON trades(date)")
	t.swap = since(s)
	s = clock()
	rows, err := tx.Query("PRAGMA foreign_key_check")
	must(err)
	for rows.Next() {
	}
	rows.Close()
	var bad int
	must(tx.QueryRow("SELECT count(*) FROM sqlite_schema WHERE type IN ('trigger', 'view')").Scan(&bad))
	t.checks = since(s)
	s = clock()
	if commit {
		must(tx.Commit())
	} else {
		must(tx.Rollback())
	}
	t.end = since(s)
	t.wal = walSize(path)
	t.total = since(start)
	exec(w, "PRAGMA foreign_keys = ON")
	cancel()
	wg.Wait()
	return t
}

func big(work, tmp string, sizes []int) {
	for _, n := range sizes {
		path := filepath.Join(work, fmt.Sprintf("big-%d.db", n))
		w := openWriter(path)
		exec(w, bigSchema)
		logf("filling %d rows", n)
		s := clock()
		fillTrades(w, n)
		fill := since(s)
		checkpoint(w, "TRUNCATE")
		r := openReaders(path, 2)

		fmt.Printf("\n## 4. Table rebuild, %d rows\n\n", n)
		fmt.Printf("Table `trades` (%s including its index, filled in %s). The rebuild changes the primary key from `id` to `(coin, date, seq)`: create the new table, copy, drop, rename, create an index, `foreign_key_check`, schema guard. Two readers query the table all the time.\n\n",
			mb(fileSize(path)), ms(fill))
		fmt.Println("| Variant | Copy | Drop, rename, index | Checks | Commit / rollback | **Writer held** | Temp space, peak | WAL file after | Reads during: p50 / p99 / max (count) | Read errors |")
		fmt.Println("|---|---|---|---|---|---|---|---|---|---|")
		variants := []struct {
			name            string
			ordered, commit bool
			cacheMB         int
		}{
			{"old order, default cache, rolled back (try)", false, false, 0},
			{"new key order, default cache, rolled back (try)", true, false, 0},
			{"old order, 64 MB cache, rolled back (try)", false, false, 64},
			{"new key order, 64 MB cache, rolled back (try)", true, false, 64},
			{"old order, 64 MB cache, committed", false, true, 64},
		}
		for _, v := range variants {
			logf("  rebuild: %s", v.name)
			if v.cacheMB > 0 {
				exec(w, fmt.Sprintf("PRAGMA cache_size = %d", -v.cacheMB*1024))
			}
			t := rebuild(w, r, path, tmp, v.ordered, v.commit)
			exec(w, "PRAGMA cache_size = -2000")
			fmt.Printf("| %s | %s | %s | %s | %s | **%s** | %s | %s | %s | %d |\n", v.name, ms(t.copy), ms(t.swap), ms(t.checks), ms(t.end),
				ms(t.total), mb(t.tmpPeak), mb(t.wal), fmt.Sprintf("%s (%d)", t.reads, t.reads.n()), t.readErrors)
			if !v.commit {
				checkpoint(w, "TRUNCATE")
			}
		}
		fmt.Printf("\nAfter the commit: WAL file %s. ", mb(walSize(path)))
		d, res := checkpoint(w, "TRUNCATE")
		fmt.Printf("`wal_checkpoint(TRUNCATE)` took %s, result %v.\n", ms(d), res)

		fmt.Printf("\n## 5. Other migration steps, %d rows\n\n", n)
		fmt.Println("Each step is its own transaction, committed. The WAL is truncated before each step. Time includes the commit and the automatic checkpoint that follows it.\n")
		fmt.Println("| Step | SQL | Writer held | WAL file after |")
		fmt.Println("|---|---|---|---|")
		steps := []struct{ name, sql string }{
			{"add_column, nullable", "ALTER TABLE trades ADD COLUMN fee REAL"},
			{"add_column, NOT NULL DEFAULT 0", "ALTER TABLE trades ADD COLUMN flag INTEGER NOT NULL DEFAULT 0"},
			{"add_column with CHECK", "ALTER TABLE trades ADD COLUMN qty REAL CHECK (qty >= 0)"},
			{"rename_column", "ALTER TABLE trades RENAME COLUMN note TO comment"},
			{"create_index, one text column", "CREATE INDEX trades_source ON trades(source)"},
			{"create_index, two columns", "CREATE INDEX trades_coin_price ON trades(coin, price)"},
			{"drop_index", "DROP INDEX trades_coin_price"},
			{"create_table + copy_data (daily averages)", "CREATE TABLE daily(coin TEXT, date TEXT, avg_price REAL, PRIMARY KEY (coin, date)); INSERT INTO daily SELECT coin, date, avg(price) FROM trades GROUP BY coin, date"},
			{"drop_column", "ALTER TABLE trades DROP COLUMN comment"},
			{"drop_table", "DROP TABLE daily"},
		}
		for _, st := range steps {
			logf("  step: %s", st.name)
			checkpoint(w, "TRUNCATE")
			s := clock()
			tx, err := w.Begin()
			must(err)
			exec(tx, st.sql)
			must(tx.Commit())
			fmt.Printf("| %s | `%s` | %s | %s |\n", st.name, st.sql, ms(since(s)), mb(walSize(path)))
		}
		r.Close()
		w.Close()
	}
}
