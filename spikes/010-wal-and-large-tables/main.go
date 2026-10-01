// SPIKE-010: WAL growth under load, checkpoints, and migrations on large tables (SPEC 7.2, 7.5).
//
// Usage:
//
//	walbig ckpt                    checkpoint modes on a large WAL
//	walbig big 1000000,5000000     table rebuild and other migration steps
//	walbig wal 60 10               WAL scenarios: W1 for 60 minutes, W2 and W3 for 10 minutes each
//	walbig fill 3000000            chunked inserts into a growing indexed table, 2 MB vs 64 MB cache
package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: walbig ckpt | big <sizes> | wal <W1 minutes> <W2/W3 minutes>")
		os.Exit(2)
	}
	wd, _ := os.Getwd()
	work := filepath.Join(wd, "work", os.Args[1])
	os.RemoveAll(work)
	must(os.MkdirAll(work, 0o755))
	defer os.RemoveAll(work)
	// SQLite puts sort files in the folder named by TMP, which is on C: here. The databases are on D:,
	// so the drop in free space on C: during a migration is the temp space it used.
	tmp := filepath.Join(os.TempDir(), "burrow-spike010")
	must(os.MkdirAll(tmp, 0o755))
	defer os.RemoveAll(tmp)
	os.Setenv("TMP", tmp)
	os.Setenv("TEMP", tmp)

	var version string
	db, _ := sql.Open("sqlite", ":memory:")
	db.QueryRow("SELECT sqlite_version()").Scan(&version)
	db.Close()
	fmt.Printf("modernc.org/sqlite v1.59.0 (SQLite %s), WAL, `synchronous = NORMAL`, 4 KB pages, default page cache (2 MB) unless noted. %d CPUs, %s.\n",
		version, runtime.NumCPU(), time.Now().Format("2006-01-02"))

	switch os.Args[1] {
	case "ckpt":
		checkpoints(work)
	case "big":
		var sizes []int
		for _, s := range strings.Split(os.Args[2], ",") {
			n, err := strconv.Atoi(s)
			must(err)
			sizes = append(sizes, n)
		}
		big(work, tmp, sizes)
	case "fill":
		n, err := strconv.Atoi(os.Args[2])
		must(err)
		fillRate(work, n)
	case "wal":
		m1, _ := strconv.Atoi(os.Args[2])
		m2, _ := strconv.Atoi(os.Args[3])
		walScenarios(work, time.Duration(m1)*time.Minute, time.Duration(m2)*time.Minute)
	}
}

// ---------------------------------------------------------------- connections

func dsn(path string, pragmas ...string) string {
	q := "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	for _, p := range pragmas {
		q += "&_pragma=" + p
	}
	return q
}

// openWriter opens the single write connection.
func openWriter(path string, pragmas ...string) *sql.DB {
	db, err := sql.Open("sqlite", dsn(path, pragmas...))
	must(err)
	db.SetMaxOpenConns(1)
	db.SetConnMaxIdleTime(0)
	return db
}

// openReaders opens a pool of read-only connections.
func openReaders(path string, n int) *sql.DB {
	db, err := sql.Open("sqlite", dsn(path, "query_only(1)"))
	must(err)
	db.SetMaxOpenConns(n)
	db.SetMaxIdleConns(n)
	return db
}

func checkpoint(db *sql.DB, mode string) (time.Duration, [3]int) {
	start := clock()
	var r [3]int
	must(db.QueryRow("PRAGMA wal_checkpoint("+mode+")").Scan(&r[0], &r[1], &r[2]))
	return since(start), r
}

func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

func walSize(path string) int64 { return fileSize(path + "-wal") }

// diskFree returns the free bytes on the volume holding dir.
func diskFree(dir string) int64 {
	var free, total, totalFree uint64
	p, _ := windows.UTF16PtrFromString(dir)
	must(windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree))
	return int64(free)
}

// ---------------------------------------------------------------- helpers

type execer interface {
	Exec(string, ...any) (sql.Result, error)
}

func exec(e execer, q string, args ...any) sql.Result {
	r, err := e.Exec(q, args...)
	if err != nil {
		panic(fmt.Sprintf("%v\n%s", err, q))
	}
	return r
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, time.Now().Format("15:04:05 ")+format+"\n", args...)
}

func mb(n int64) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }

func ms(d time.Duration) string {
	switch {
	case d >= 10*time.Second:
		return fmt.Sprintf("%.1f s", d.Seconds())
	case d < time.Millisecond:
		return fmt.Sprintf("%d µs", d.Microseconds())
	case d >= 10*time.Millisecond:
		return fmt.Sprintf("%.0f ms", float64(d.Microseconds())/1000)
	default:
		return fmt.Sprintf("%.2f ms", float64(d.Microseconds())/1000)
	}
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// clock and since use the performance counter: time.Now on this machine only moves every ~0.5 ms.
type stamp int64

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	qpc      = kernel32.NewProc("QueryPerformanceCounter")
	qpcFreq  = func() int64 {
		var f int64
		kernel32.NewProc("QueryPerformanceFrequency").Call(uintptr(unsafe.Pointer(&f)))
		return f
	}()
)

func clock() stamp {
	var c int64
	qpc.Call(uintptr(unsafe.Pointer(&c)))
	return stamp(c)
}

func since(s stamp) time.Duration {
	return time.Duration(float64(clock()-s) * 1e9 / float64(qpcFreq))
}

// latencies collects durations from several goroutines.
type latencies struct {
	mu sync.Mutex
	d  []time.Duration
}

func (l *latencies) add(d time.Duration) {
	l.mu.Lock()
	l.d = append(l.d, d)
	l.mu.Unlock()
}

func (l *latencies) pct(p float64) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.d) == 0 {
		return 0
	}
	s := slices.Clone(l.d)
	slices.Sort(s)
	return s[min(len(s)-1, int(float64(len(s))*p))]
}

func (l *latencies) String() string {
	return fmt.Sprintf("%s / %s / %s", ms(l.pct(0.5)), ms(l.pct(0.99)), ms(l.pct(1)))
}

func (l *latencies) n() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.d)
}
