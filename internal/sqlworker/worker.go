package sqlworker

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/amir-saatchi/jenab/internal/sqlguard"
)

// Flag is the first argument of a worker process; the second is the
// database file and the third the memory cap in bytes.
const Flag = "--sql-worker"

// enabled is set once IfWorker has returned: this program can be started
// as a worker. A pool in a program that never called it, such as another
// package's tests, starts none.
var enabled atomic.Bool

// IfWorker makes this process a worker, if the pool started it as one,
// and exits when the pool is done with it. Call it first in main, before
// anything else starts, and in TestMain of a package whose tests run
// agent queries.
func IfWorker() {
	if len(os.Args) != 4 || os.Args[1] != Flag {
		enabled.Store(true)
		return
	}
	limit, _ := strconv.ParseUint(os.Args[3], 10, 64)
	os.Exit(serve(os.Args[2], limit, os.Stdin, os.Stdout))
}

// workerPragmas run on the worker's connection. mode=ro in the DSN means
// it can't write at all, whatever the query does; query_only and the
// limits come again with each query (sqlguard.Prepare, Restrict).
// trusted_schema off keeps functions out of anything stored in the
// schema. temp_store is the default, files: see tempStore.
var workerPragmas = []string{"busy_timeout(5000)", "query_only(1)", "trusted_schema(0)", tempStore}

// tempStore keeps sorts and temporary tables in files, which SQLite
// deletes when the worker ends, however it ends; the deadline bounds how
// much they can hold. In memory they would count against the cap: on 1M
// rows the analytic queries in sqlworker_test.go peaked at up to 299 MiB
// that way, against at most 100 MiB with files.
const tempStore = "temp_store(1)"

func dsn(path string) string {
	q := url.Values{"mode": {"ro"}}
	for _, p := range workerPragmas {
		q.Add("_pragma", p)
	}
	return FileURI(path) + "?" + q.Encode()
}

// FileURI is path as a file: URI for SQLite, before its query. Every byte
// but letters, digits and -._~/: is escaped, so a '?', '#' or '%' in a
// folder's name stays in the name: modernc takes the query from the first
// '?', and SQLite ends the path at '#' and decodes %XX. A UNC path
// (//server/share) gets an empty authority first.
func FileURI(path string) string {
	p := filepath.ToSlash(path)
	var b strings.Builder
	b.WriteString("file:")
	if strings.HasPrefix(p, "//") {
		b.WriteString("//")
	}
	const hex = "0123456789ABCDEF"
	for i := 0; i < len(p); i++ {
		switch c := p[i]; {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', strings.IndexByte("-._~/:", c) >= 0:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}

// serve answers requests from in until it closes: the pool is done with
// the worker, or the app has ended. Its first frame says that it has
// opened the file, or why it couldn't.
func serve(path string, limit uint64, in io.Reader, out io.Writer) int {
	watchMemory(limit)
	db, err := sql.Open("sqlite", dsn(path))
	if err == nil {
		db.SetMaxOpenConns(1)
		err = db.Ping()
	}
	if _, werr := out.Write(encodeResponse(Result{}, err)); err != nil || werr != nil {
		return 1
	}
	g := sqlguard.New()
	reqs := make(chan []byte)
	go func() {
		r := bufio.NewReader(in)
		for {
			b, err := readFrame(r)
			if err != nil {
				// The pool closed the pipe, or the app ended: stop at once,
				// even in the middle of a query.
				if !errors.Is(err, io.EOF) {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				os.Exit(0)
			}
			reqs <- b
		}
	}()
	for b := range reqs {
		req, err := decodeRequest(b)
		var res Result
		if err == nil {
			res, err = runQuery(db, g, req)
		}
		if _, err := out.Write(encodeResponse(res, err)); err != nil {
			return 1
		}
	}
	return 0
}

// runQuery runs one request on the worker's one connection, with the agent
// limits set again first.
func runQuery(db *sql.DB, g *sqlguard.Guard, req Request) (Result, error) {
	ctx := context.Background()
	c, err := db.Conn(ctx)
	if err != nil {
		return Result{}, err
	}
	defer c.Close()
	if err := sqlguard.Prepare(ctx, c); err != nil {
		return Result{}, err
	}
	if err := sqlguard.Restrict(c); err != nil {
		return Result{}, err
	}
	return Run(ctx, c, g, req)
}
