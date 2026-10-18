// Package sqlworker runs agent queries (SPEC 2.2) in worker processes that
// the OS caps and kills. Preparing a query can't be interrupted, one call
// of a function can run for seconds, and SQLite in modernc's build has no
// memory limit of its own, so a query is bounded from outside: at its
// deadline its worker is killed, and on Windows a Job Object caps the
// worker's memory. The app and its other connections never run agent SQL.
//
// A worker is the app's own executable started with Flag (IfWorker). It
// opens project.db read-only, runs all three layers of sqlguard again,
// and sends back at most ResultCap bytes of rows.
package sqlworker

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"time"

	"modernc.org/sqlite"

	"github.com/amir-saatchi/jenab/internal/sqlguard"
)

const (
	// CountCap is where counting a query's rows stops. Counting runs the
	// query a second time, inside the same statement, only when the rows
	// were cut; it skips the columns, so even 100,000 rows of a plain
	// table take milliseconds.
	CountCap = 100_000
	// ResultCap bounds the bytes of Values: the rows after it are left
	// out, as if max were lower.
	ResultCap = 8 << 20
)

// Request is one query: at most Max rows (at least 1) of Query with Args,
// which are values database/sql accepts, or sql.NamedArg.
type Request struct {
	Query string
	Args  []any
	Max   int
	// Timeout, if set, is the query's deadline, counted from Pool.Query
	// without the time a worker takes to start (Pool.Query).
	Timeout time.Duration
}

// Result is a query's rows.
type Result struct {
	Columns []string
	// Values are at most max rows, as the driver gives them: int64,
	// float64, string, []byte or nil, and time.Time for a column declared
	// DATE, DATETIME or TIMESTAMP.
	Values [][]any
	Total  int  // the query's rows in all; when Capped, it has more
	Cut    bool // Values stops before the last row
	Capped bool // counting stopped at CountCap rows
	Full   bool // Values stops early because its rows reached ResultCap
}

// Run runs req on c, a reader with sqlguard.Prepare and Restrict set, in
// one read transaction, so the table map, EXPLAIN and the query all see
// the same schema. A worker runs each query with it; it is exported for
// benchmarks that compare the worker with running in the app.
func Run(ctx context.Context, c *sql.Conn, g *sqlguard.Guard, req Request) (Result, error) {
	n := max(req.Max, 1)
	// Layer 1, before SQLite sees any of the text, and the statement that
	// also counts the rows.
	stmt, err := sqlguard.Wrap(req.Query, n, CountCap)
	if err != nil {
		return Result{}, err
	}
	// BEGIN can't be cut off by ctx: had it begun the transaction and still
	// returned ctx's error, the reader would stay inside it. BEGIN alone
	// doesn't touch the file.
	if _, err := c.ExecContext(context.WithoutCancel(ctx), "BEGIN"); err != nil {
		return Result{}, err
	}
	defer func() {
		// A reader must not stay inside a transaction: it would keep the
		// WAL from being reset (7.2).
		if _, err := c.ExecContext(context.WithoutCancel(ctx), "ROLLBACK"); err != nil {
			c.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	if err := g.Explain(ctx, c, req.Query); err != nil { // layer 2
		return Result{}, err
	}
	rows, err := c.QueryContext(ctx, stmt, req.Args...)
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return Result{}, err
	}
	last := len(cols) - 1 // Wrap's total
	out := Result{Columns: cols[:last]}
	size := 0
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return Result{}, err
		}
		total, _ := vals[last].(int64)
		out.Total = int(total)
		if size += rowSize(vals[:last]); size > ResultCap && len(out.Values) > 0 {
			out.Full = true
			break
		}
		out.Values = append(out.Values, vals[:last:last])
	}
	if err := rows.Err(); err != nil {
		return Result{}, err
	}
	if out.Total > CountCap {
		out.Total, out.Capped = CountCap, true
	}
	// The second count can come out lower for a query that uses random().
	out.Total = max(out.Total, len(out.Values))
	out.Cut = out.Total > len(out.Values)
	return out, nil
}

// rowSize is about the bytes a row's values take, in memory and on the
// pipe.
func rowSize(vals []any) int {
	n := 0
	for _, v := range vals {
		switch v := v.(type) {
		case string:
			n += len(v)
		case []byte:
			n += len(v)
		}
		n += 16
	}
	return n
}

// errorCode is SQLite's result code for err, or 0.
func errorCode(err error) int {
	var se *sqlite.Error
	if errors.As(err, &se) {
		return se.Code()
	}
	return 0
}

// sqliteNoMem is SQLITE_NOMEM: an allocation failed, here at the
// worker's memory cap.
const sqliteNoMem = 7
