// SPIKE-011: which FTS5 tokenizer and Go normalization give good search_history
// results for English, German and Persian?
//
// Usage: go run . > results.md
package main

import (
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type setup struct {
	name      string
	tokenizer string // "" for the union setup
	prefix    bool
}

var setups = []setup{
	{"unicode61", "unicode61 remove_diacritics 2", false},
	{"unicode61, prefix query", "unicode61 remove_diacritics 2", true},
	{"porter unicode61", "porter unicode61 remove_diacritics 2", false},
	{"trigram", "trigram remove_diacritics 1", false},
	{"unicode61 prefix ∪ trigram", "", true},
}

func main() {
	var v string
	db0 := open(":memory:")
	db0.QueryRow("select sqlite_version()").Scan(&v)
	fmt.Printf("# SPIKE-011 results\n\nmodernc.org/sqlite v1.59.0 (SQLite %s), Go %s, %s.\n", v, runtime.Version(), time.Now().Format("2006-01-02"))
	db0.Close()
	relevance()
	hostileQueries()
	scale(100_000)
}

func open(path string) *sql.DB {
	db, err := sql.Open("sqlite", path)
	must(err)
	db.SetMaxOpenConns(1)
	return db
}

// ---- 1. relevance on the labelled set ----

type outcome struct {
	found, want, extra int
	hits               []string
}

func relevance() {
	ids := make([]string, 0, len(messages))
	for id := range messages {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	results := map[string]map[string]outcome{} // setup|norm -> query -> outcome
	var order []string
	for _, n := range normalizers {
		dbs := map[string]*sql.DB{}
		for _, tok := range []string{"unicode61 remove_diacritics 2", "porter unicode61 remove_diacritics 2", "trigram remove_diacritics 1"} {
			db := open(":memory:")
			_, err := db.Exec(fmt.Sprintf(`CREATE VIRTUAL TABLE f USING fts5(body, tokenize = '%s')`, tok))
			must(err)
			for i, id := range ids {
				_, err := db.Exec(`INSERT INTO f(rowid, body) VALUES (?, ?)`, i+1, n.f(messages[id]))
				must(err)
			}
			dbs[tok] = db
		}
		for _, s := range setups {
			key := s.name + " | " + n.name
			order = append(order, key)
			results[key] = map[string]outcome{}
			for _, q := range queries {
				var hits map[string]bool
				if s.tokenizer == "" {
					hits = search(dbs["unicode61 remove_diacritics 2"], n.build(q.q, true), ids)
					for h := range search(dbs["trigram remove_diacritics 1"], n.build(q.q, false), ids) {
						hits[h] = true
					}
				} else {
					hits = search(dbs[s.tokenizer], n.build(q.q, s.prefix), ids)
				}
				results[key][q.q] = score(q.want, hits)
			}
		}
		for _, db := range dbs {
			db.Close()
		}
	}

	fmt.Print("\n## 1. Labelled set\n\n")
	fmt.Printf("%d messages (German, English, Persian, mixed; some only there to catch false hits) and %d queries with the messages a user would want. The query is normalized like the text, split on spaces, and every term quoted (`\"term\"`, or `\"term\"*` for prefix).\n\n", len(messages), len(queries))
	fmt.Println("| Setup | Normalization | Wanted hits found | False hits | Queries fully right |")
	fmt.Println("|---|---|---|---|---|")
	total := 0
	for _, q := range queries {
		total += len(q.want)
	}
	for _, key := range order {
		f, x, right := 0, 0, 0
		for _, o := range results[key] {
			f += o.found
			x += o.extra
			if o.found == o.want && o.extra == 0 {
				right++
			}
		}
		parts := strings.SplitN(key, " | ", 2)
		fmt.Printf("| %s | %s | %d of %d | %d | %d of %d |\n", parts[0], parts[1], f, total, x, right, len(queries))
	}

	fmt.Print("\n### Per query, with Go normalization (ZWNJ → space)\n\n")
	fmt.Print("Cell: wanted hits found / wanted, then false hits as `+n`. **Bold** = fully right.\n\n")
	fmt.Print("| Group | Query | Wanted |")
	for _, s := range setups {
		fmt.Printf(" %s |", s.name)
	}
	fmt.Print("\n|---|---|---|")
	for range setups {
		fmt.Print("---|")
	}
	fmt.Println()
	for _, q := range queries {
		fmt.Printf("| %s | `%s` | %s |", q.group, q.q, strings.Join(q.want, " "))
		for _, s := range setups {
			o := results[s.name+" | Go, ZWNJ → space"][q.q]
			cell := fmt.Sprintf("%d/%d", o.found, o.want)
			if o.extra > 0 {
				cell += fmt.Sprintf(" +%d (%s)", o.extra, strings.Join(extraIDs(q.want, o.hits), " "))
			}
			if o.found == o.want && o.extra == 0 {
				cell = "**" + cell + "**"
			}
			fmt.Printf(" %s |", cell)
		}
		fmt.Println()
	}

	fmt.Print("\n### Persian queries by normalization (unicode61, prefix query)\n\n")
	fmt.Println("| Query | none | ZWNJ → space | ZWNJ removed | ZWNJ removed, query also tries split |")
	fmt.Println("|---|---|---|---|---|")
	for _, q := range queries {
		if q.group != "Persian" {
			continue
		}
		fmt.Printf("| `%s` |", q.q)
		for _, n := range normalizers {
			o := results["unicode61, prefix query | "+n.name][q.q]
			fmt.Printf(" %d/%d%s |", o.found, o.want, plus(o.extra))
		}
		fmt.Println()
	}
}

func plus(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(" +%d", n)
}

func search(db *sql.DB, fq string, ids []string) map[string]bool {
	hits := map[string]bool{}
	if fq == "" {
		return hits
	}
	rows, err := db.Query(`SELECT rowid FROM f WHERE f MATCH ?`, fq)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query %q: %v\n", fq, err)
		return hits
	}
	defer rows.Close()
	for rows.Next() {
		var r int
		rows.Scan(&r)
		hits[ids[r-1]] = true
	}
	return hits
}

func score(want []string, hits map[string]bool) outcome {
	o := outcome{want: len(want)}
	w := map[string]bool{}
	for _, id := range want {
		w[id] = true
		if hits[id] {
			o.found++
		}
	}
	for h := range hits {
		o.hits = append(o.hits, h)
		if !w[h] {
			o.extra++
		}
	}
	return o
}

func extraIDs(want, hits []string) []string {
	w := map[string]bool{}
	for _, id := range want {
		w[id] = true
	}
	var x []string
	for _, h := range hits {
		if !w[h] {
			x = append(x, h)
		}
	}
	sort.Strings(x)
	return x
}

// ---- 2. hostile queries ----

func hostileQueries() {
	fmt.Print("\n## 2. Query builder with hostile input\n\n")
	fmt.Print("Each input goes through `ftsQuery` and then `MATCH`, on unicode61 (prefix), trigram, and unicode61 with `ftsQueryZWNJ` (plus the Persian inputs `می‌خواهم \"` and `بیت‌کوین OR`).\n\n")
	fmt.Println("| Input | FTS5 query | unicode61 | trigram | ftsQueryZWNJ |")
	fmt.Println("|---|---|---|---|---|")
	u, t := open(":memory:"), open(":memory:")
	defer u.Close()
	defer t.Close()
	u.Exec(`CREATE VIRTUAL TABLE f USING fts5(body, tokenize = 'unicode61 remove_diacritics 2')`)
	t.Exec(`CREATE VIRTUAL TABLE f USING fts5(body, tokenize = 'trigram remove_diacritics 1')`)
	for _, db := range []*sql.DB{u, t} {
		db.Exec(`INSERT INTO f(body) VALUES ('btc and preis near a b'), ('NOT x OR y')`)
	}
	for _, h := range append(hostile, "می‌خواهم \"", "بیت‌کوین OR") {
		fq := ftsQuery(h, true)
		cell := func(db *sql.DB, fq string) string {
			if fq == "" {
				return "not run (empty query)"
			}
			var n int
			if err := db.QueryRow(`SELECT count(*) FROM f WHERE f MATCH ?`, fq).Scan(&n); err != nil {
				return "**error:** " + err.Error()
			}
			return fmt.Sprintf("ok, %d rows", n)
		}
		fmt.Printf("| `%s` | `%s` | %s | %s | %s |\n", strings.ReplaceAll(h, "|", "\\|"), fq, cell(u, fq), cell(t, ftsQuery(h, false)), cell(u, ftsQueryZWNJ(h, true)))
	}
}

// ---- 3. scale ----

func scale(n int) {
	work, _ := filepath.Abs("work")
	os.RemoveAll(work)
	os.MkdirAll(work, 0o755)
	defer os.RemoveAll(work)
	rng := rand.New(rand.NewSource(11))
	gen := newGenerator(rng)
	texts := make([]string, n)
	var textBytes int64
	for i := range texts {
		texts[i] = normalize(gen.message(), " ")
		textBytes += int64(len(texts[i]))
	}
	fmt.Printf("\n## 3. %d messages\n\n", n)
	fmt.Printf("Synthetic messages: 50%% English-like, 25%% German-like (umlauts, compounds), 25%% Persian-like (ZWNJ joins), about 5%% with code or URLs. Word frequencies follow a Zipf curve. Average %d bytes of text per message, %.1f MB in total. Normalized with ZWNJ → space. Contentless tables (`content = ''`, `contentless_delete = 1`), so the size is the index alone; the text itself stays in the messages table. Top 20 by `bm25`, run on a warm cache.\n\n", textBytes/int64(n), float64(textBytes)/(1<<20))
	fmt.Println("| Tokenizer | Build (500-row transactions) | Index size | Index / text | Query p50 | p95 | max | Queries with no hit |")
	fmt.Println("|---|---|---|---|---|---|---|---|")
	qs := gen.queries(rng, 60)
	for _, tok := range []string{"unicode61 remove_diacritics 2", "porter unicode61 remove_diacritics 2", "trigram remove_diacritics 1"} {
		path := filepath.Join(work, strings.Fields(tok)[0]+".db")
		db := open(path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
		_, err := db.Exec(fmt.Sprintf(`CREATE VIRTUAL TABLE f USING fts5(body, content = '', contentless_delete = 1, tokenize = '%s')`, tok))
		must(err)
		start := clock()
		for i := 0; i < n; i += 500 {
			tx, _ := db.Begin()
			st, _ := tx.Prepare(`INSERT INTO f(rowid, body) VALUES (?, ?)`)
			for j := i; j < i+500 && j < n; j++ {
				_, err := st.Exec(j+1, texts[j])
				must(err)
			}
			st.Close()
			must(tx.Commit())
		}
		build := since(start)
		db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
		var size int64
		if err := db.QueryRow(`SELECT sum(pgsize) FROM dbstat WHERE name LIKE 'f\_%' ESCAPE '\'`).Scan(&size); err != nil {
			st, _ := os.Stat(path)
			size = st.Size()
		}
		prefix := !strings.HasPrefix(tok, "trigram")
		var times []time.Duration
		empty := 0
		for round := 0; round < 2; round++ { // first round warms the cache
			times = times[:0]
			empty = 0
			for _, q := range qs {
				t0 := clock()
				rows, err := db.Query(`SELECT rowid FROM f WHERE f MATCH ? ORDER BY bm25(f) LIMIT 20`, ftsQuery(q, prefix))
				must(err)
				c := 0
				for rows.Next() {
					c++
				}
				rows.Close()
				times = append(times, since(t0))
				if c == 0 {
					empty++
				}
			}
		}
		sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
		fmt.Printf("| %s | %.1f s | %.1f MB | %.2f | %s | %s | %s | %d of %d |\n", tok, build.Seconds(), float64(size)/(1<<20), float64(size)/float64(textBytes),
			ms(times[len(times)/2]), ms(times[int(math.Ceil(float64(len(times))*0.95))-1]), ms(times[len(times)-1]), empty, len(qs))
		db.Close()
	}

	// substring scan without an index: body LIKE '%term%' for every term
	db := open(filepath.Join(work, "plain.db") + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	must2(db.Exec(`CREATE TABLE m(id INTEGER PRIMARY KEY, body TEXT)`))
	tx, _ := db.Begin()
	for i, s := range texts {
		must2(tx.Exec(`INSERT INTO m VALUES (?, ?)`, i+1, s))
	}
	must(tx.Commit())
	var scan []time.Duration
	for round := 0; round < 2; round++ {
		scan = scan[:0]
		for _, q := range qs {
			var where []string
			var args []any
			for _, t := range strings.Fields(q) {
				where = append(where, `body LIKE ? ESCAPE '\'`)
				args = append(args, "%"+strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(t)+"%")
			}
			t0 := clock()
			var c int
			must(db.QueryRow(`SELECT count(*) FROM m WHERE `+strings.Join(where, " AND "), args...).Scan(&c))
			scan = append(scan, since(t0))
		}
	}
	db.Close()
	sort.Slice(scan, func(i, j int) bool { return scan[i] < scan[j] })
	fmt.Printf("| no index: `body LIKE '%%term%%'` over all rows (count) | — | 0 | 0 | %s | %s | %s | — |\n",
		ms(scan[len(scan)/2]), ms(scan[int(math.Ceil(float64(len(scan))*0.95))-1]), ms(scan[len(scan)-1]))
	fmt.Println("\nQueries: 60 terms drawn from the three vocabularies (frequent, middle and rare words, some two-word queries, and some two-letter Persian words).")
}

func ms(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%d µs", d.Microseconds())
	}
	return fmt.Sprintf("%.1f ms", float64(d.Microseconds())/1000)
}

func must2(_ sql.Result, err error) { must(err) }

func must(err error) {
	if err != nil {
		panic(err)
	}
}
