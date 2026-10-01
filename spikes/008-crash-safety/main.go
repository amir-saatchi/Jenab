// SPIKE-008: kill the process in the middle of work and check the project afterwards.
//
// Usage: go run . [iterations]      (default 20 per scenario)
// The harness starts this same binary as a worker ("worker <scenario> <dir>"),
// kills it at a random moment, then recovers and verifies the project.
package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type scenario struct {
	id, desc string
	// demo scenarios show what goes wrong without a rule; problems are expected there.
	demo    bool
	crashAt string // if set, the worker exits hard when it reaches this phase, instead of a random kill
	minKill time.Duration
	maxKill time.Duration
	setup   func(dir string) error
	work    func(dir string)
	recover func(dir string, db *sql.DB) map[string]int
	verify  func(dir string, db *sql.DB) []string
	cleanup func(dir string, db *sql.DB) // housekeeping between iterations, not part of the test
}

func dsn(path string) string {
	return "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
}

func openDB(path string) *sql.DB {
	db, err := sql.Open("sqlite", dsn(path))
	must(err)
	db.SetMaxOpenConns(1)
	must(db.Ping())
	return db
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func phase(p string) {
	fmt.Println("PHASE", p)
	if os.Getenv("CRASH_AT") == p {
		os.Exit(3) // no deferred code, no cleanup: same effect as a kill at this point
	}
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func main() {
	if len(os.Args) >= 4 && os.Args[1] == "worker" {
		for _, s := range scenarios {
			if s.id == os.Args[2] {
				s.work(os.Args[3])
				return
			}
		}
		panic("unknown scenario " + os.Args[2])
	}
	iterations := 20
	only := ""
	for _, a := range os.Args[1:] {
		if n, err := strconv.Atoi(a); err == nil {
			iterations = n
		} else {
			only = a
		}
	}
	self, err := os.Executable()
	must(err)
	wd, _ := os.Getwd()
	base := filepath.Join(wd, "work")
	os.RemoveAll(base)
	defer os.RemoveAll(base)

	fmt.Printf("# SPIKE-008 results\n\nmodernc.org/sqlite v1.59.0, WAL, `synchronous = NORMAL`. %d hard kills per scenario (TerminateProcess on Windows).\n", iterations)
	var summary []string
	for _, s := range scenarios {
		if only != "" && s.id != only {
			continue
		}
		summary = append(summary, run(s, self, filepath.Join(base, s.id), iterations))
	}
	fmt.Println("\n## Summary\n")
	fmt.Println("| Scenario | Problems after recovery | Result |")
	fmt.Println("|---|---|---|")
	for _, l := range summary {
		fmt.Println(l)
	}
}

func run(s scenario, self, dir string, iterations int) string {
	for _, d := range []string{"objects", "snapshots", "tmp"} {
		must(os.MkdirAll(filepath.Join(dir, d), 0o755))
	}
	must(s.setup(dir))

	phases := map[string]int{}
	notes := map[string]int{}
	var problems []string
	var recTimes []time.Duration
	earlyExit := 0

	for i := 0; i < iterations; i++ {
		cmd := exec.Command(self, "worker", s.id, dir)
		cmd.Stderr = os.Stderr
		if s.crashAt != "" {
			cmd.Env = append(os.Environ(), "CRASH_AT="+s.crashAt)
		}
		out, err := cmd.StdoutPipe()
		must(err)
		must(cmd.Start())
		var mu sync.Mutex
		last := "startup"
		ready := make(chan struct{})
		go func() {
			sc := bufio.NewScanner(out)
			once := false
			for sc.Scan() {
				line := sc.Text()
				if line == "READY" && !once {
					once = true
					close(ready)
				} else if strings.HasPrefix(line, "PHASE ") {
					mu.Lock()
					last = line[6:]
					mu.Unlock()
				}
			}
		}()
		exited := make(chan error, 1)
		go func() { exited <- cmd.Wait() }()
		select {
		case <-ready:
		case err := <-exited:
			panic(fmt.Sprintf("%s: worker exited before READY: %v", s.id, err))
		case <-time.After(60 * time.Second):
			panic(s.id + ": worker not ready")
		}
		wait := s.minKill + rand.N(s.maxKill-s.minKill)
		if s.crashAt != "" {
			wait = time.Minute
		}
		select {
		case <-time.After(wait):
			mu.Lock()
			phases[last]++
			mu.Unlock()
			cmd.Process.Kill()
			<-exited
		case err := <-exited:
			if s.crashAt != "" {
				mu.Lock()
				phases[last]++
				mu.Unlock()
				break
			}
			earlyExit++
			problems = append(problems, fmt.Sprintf("iteration %d: worker exited on its own: %v", i+1, err))
		}

		// Restart: open, check, recover, verify.
		start := time.Now()
		db := openDB(filepath.Join(dir, "project.db"))
		var qc string
		db.QueryRow("PRAGMA quick_check").Scan(&qc)
		if qc != "ok" {
			problems = append(problems, fmt.Sprintf("iteration %d: quick_check: %s", i+1, qc))
		}
		for k, v := range s.recover(dir, db) {
			notes[k] += v
		}
		recTimes = append(recTimes, time.Since(start))
		for _, p := range s.verify(dir, db) {
			problems = append(problems, fmt.Sprintf("iteration %d: %s", i+1, p))
		}
		if s.cleanup != nil {
			s.cleanup(dir, db)
		}
		db.Close()
	}

	fmt.Printf("\n## %s — %s\n\n", s.id, s.desc)
	if s.demo {
		fmt.Println("*Demonstration without the rule: problems are expected here.*\n")
	}
	fmt.Println("| Measure | Value |")
	fmt.Println("|---|---|")
	fmt.Printf("| Killed during | %s |\n", histogram(phases))
	if len(notes) > 0 {
		fmt.Printf("| Recovery actions | %s |\n", histogram(notes))
	}
	sort.Slice(recTimes, func(a, b int) bool { return recTimes[a] < recTimes[b] })
	var total time.Duration
	for _, t := range recTimes {
		total += t
	}
	fmt.Printf("| Open + quick_check + recovery | avg %v, max %v |\n",
		(total / time.Duration(len(recTimes))).Round(time.Millisecond), recTimes[len(recTimes)-1].Round(time.Millisecond))
	fmt.Printf("| project.db size at the end | %s |\n", size(filepath.Join(dir, "project.db")))
	fmt.Printf("| Problems after recovery | %d |\n", len(problems))
	for i, p := range problems {
		if i == 10 {
			fmt.Printf("\n... and %d more\n", len(problems)-10)
			break
		}
		if i == 0 {
			fmt.Println()
		}
		fmt.Println("- " + p)
	}
	res := "OK"
	switch {
	case s.demo && len(problems) > 0:
		res = "problems shown, as expected"
	case s.demo:
		res = "no problem hit in this run"
	case len(problems) > 0:
		res = "**FAIL**"
	}
	return fmt.Sprintf("| %s — %s | %d | %s |", s.id, s.desc, len(problems), res)
}

func histogram(m map[string]int) string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}

func size(path string) string {
	var total int64
	for _, suffix := range []string{"", "-wal"} {
		if st, err := os.Stat(path + suffix); err == nil {
			total += st.Size()
		}
	}
	return fmt.Sprintf("%.1f MB (with -wal)", float64(total)/(1<<20))
}

// integrity runs the full check and the foreign key check.
func integrity(db *sql.DB, label string) []string {
	var p []string
	var res string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&res); err != nil || res != "ok" {
		p = append(p, fmt.Sprintf("%s integrity_check: %s %v", label, res, err))
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err == nil {
		if rows.Next() {
			p = append(p, label+": foreign key violations")
		}
		rows.Close()
	}
	return p
}

func count(db *sql.DB, q string, args ...any) int64 {
	var n int64
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		panic(q + ": " + err.Error())
	}
	return n
}
