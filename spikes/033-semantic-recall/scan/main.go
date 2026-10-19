// Command scan times a brute-force top-50 cosine scan over random unit vectors
// held as float32 and int8, loaded from SQLite BLOBs (SPIKE-033, scan benchmark).
package main

import (
	"database/sql"
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	dir     = flag.String("dir", os.TempDir(), "where to put the temporary SQLite files (needs ~6 GB)")
	widths  = flag.String("widths", "3072,768,384,256", "vector widths")
	sizes   = flag.String("sizes", "100000,250000", "vector counts per scan")
	queries = flag.Int("queries", 200, "queries per timing")
	seed    = flag.Int64("seed", 33, "random seed")
)

func ints(s string) (out []int) {
	for _, f := range strings.Split(s, ",") {
		n, err := strconv.Atoi(f)
		check(err)
		out = append(out, n)
	}
	return
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	flag.Parse()
	fmt.Printf("go %s, GOMAXPROCS=%d, queries=%d, top=%d, seed=%d\n\n", runtime.Version(), runtime.GOMAXPROCS(0), *queries, topK, *seed)
	fmt.Println("| width | form | vectors | load (s) | memory per 100k (MB) | 1 core p50 / p95 (ms) | all cores p50 / p95 (ms) |")
	fmt.Println("|---|---|---|---|---|---|---|")
	var disk []string
	for _, w := range ints(*widths) {
		r := rand.New(rand.NewSource(*seed + int64(w)))
		maxN := 0
		for _, n := range ints(*sizes) {
			maxN = max(maxN, n)
		}
		qs := make([][]float32, *queries)
		for i := range qs {
			qs[i] = unit(r, w)
		}
		paths := map[string]string{"float32": filepath.Join(*dir, fmt.Sprintf("scan-%d-f32.db", w)), "int8": filepath.Join(*dir, fmt.Sprintf("scan-%d-i8.db", w))}
		write(paths, w, maxN, r)
		for _, p := range paths {
			fi, err := os.Stat(p)
			check(err)
			disk = append(disk, fmt.Sprintf("%d %s %.1f", w, filepath.Base(p), float64(fi.Size())/float64(maxN)))
		}
		for _, n := range ints(*sizes) {
			for _, form := range []string{"float32", "int8"} {
				bench(paths[form], form, w, n, qs)
			}
		}
		for _, p := range paths {
			os.Remove(p)
		}
	}
	fmt.Println("\nSQLite file bytes per vector (file size / rows, includes page overhead):")
	for _, d := range disk {
		fmt.Println(" ", d)
	}
}

func open(path string) *sql.DB {
	db, err := sql.Open("sqlite", path)
	check(err)
	return db
}

// write fills one SQLite file per form: id, (scale,) BLOB. Both forms hold the same vectors.
func write(paths map[string]string, w, n int, r *rand.Rand) {
	for _, p := range paths {
		os.Remove(p)
	}
	f, i := open(paths["float32"]), open(paths["int8"])
	defer f.Close()
	defer i.Close()
	for _, db := range []*sql.DB{f, i} {
		db.SetMaxOpenConns(1)
		_, err := db.Exec(`PRAGMA journal_mode=off; PRAGMA synchronous=off`)
		check(err)
	}
	_, err := f.Exec(`CREATE TABLE vec (id INTEGER PRIMARY KEY, v BLOB NOT NULL)`)
	check(err)
	_, err = i.Exec(`CREATE TABLE vec (id INTEGER PRIMARY KEY, scale REAL NOT NULL, v BLOB NOT NULL)`)
	check(err)
	ft, _ := f.Begin()
	it, _ := i.Begin()
	fs, err := ft.Prepare(`INSERT INTO vec VALUES (?,?)`)
	check(err)
	is, err := it.Prepare(`INSERT INTO vec VALUES (?,?,?)`)
	check(err)
	fb, ib := make([]byte, w*4), make([]byte, w)
	q := make([]int8, w)
	for k := 1; k <= n; k++ {
		v := unit(r, w)
		for j, x := range v {
			binary.LittleEndian.PutUint32(fb[j*4:], math.Float32bits(x))
		}
		scale := quantize(v, q)
		for j, x := range q {
			ib[j] = byte(x)
		}
		_, err = fs.Exec(k, fb)
		check(err)
		_, err = is.Exec(k, scale, ib)
		check(err)
	}
	check(ft.Commit())
	check(it.Commit())
}

// load reads the first n rows the way the app would: one SELECT, scan each BLOB, copy into flat slices.
func load(path, form string, w, n int) *store {
	db := open(path)
	defer db.Close()
	s := &store{n: n, w: w}
	var rows *sql.Rows
	var err error
	if form == "float32" {
		s.f32 = make([]float32, n*w)
		rows, err = db.Query(`SELECT v FROM vec WHERE id <= ? ORDER BY id`, n)
	} else {
		s.i8, s.scales = make([]int8, n*w), make([]float32, n)
		rows, err = db.Query(`SELECT scale, v FROM vec WHERE id <= ? ORDER BY id`, n)
	}
	check(err)
	defer rows.Close()
	for k := 0; rows.Next(); k++ {
		var b sql.RawBytes
		if form == "float32" {
			check(rows.Scan(&b))
			dst := s.f32[k*w : (k+1)*w]
			for j := range dst {
				dst[j] = math.Float32frombits(binary.LittleEndian.Uint32(b[j*4:]))
			}
		} else {
			var sc float64
			check(rows.Scan(&sc, &b))
			s.scales[k] = float32(sc)
			dst := s.i8[k*w : (k+1)*w]
			for j := range dst {
				dst[j] = int8(b[j])
			}
		}
	}
	check(rows.Err())
	return s
}

func heap() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

func bench(path, form string, w, n int, qs [][]float32) {
	before := heap()
	t0 := time.Now()
	s := load(path, form, w, n)
	loadT := time.Since(t0)
	mem := float64(heap()-before) / float64(n) * 100000 / 1e6

	scanner := func(q []float32) scanner {
		if form == "float32" {
			return s.f32Scanner(q)
		}
		return s.i8Scanner(q)
	}
	timing := func(workers int) string {
		run(scanner(qs[0]), n, workers) // warm-up
		d := make([]float64, len(qs))
		for i, q := range qs {
			sc := scanner(q)
			t := time.Now()
			run(sc, n, workers)
			d[i] = time.Since(t).Seconds() * 1000
		}
		sort.Float64s(d)
		return fmt.Sprintf("%.2f / %.2f", d[len(d)/2], d[len(d)*95/100])
	}
	one := timing(1)
	all := timing(runtime.GOMAXPROCS(0))
	fmt.Printf("| %d | %s | %d | %.2f | %.1f | %s | %s |\n", w, form, n, loadT.Seconds(), mem, one, all)
}
