package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/amir-saatchi/jenab/internal/sqlguard"
	"github.com/amir-saatchi/jenab/internal/sqlworker"
)

// Agent queries (SPEC 2.2): read-only SQL that agents write, through the
// three layers of sqlguard, in a worker process (sqlworker).

// agentTimeout bounds query and db.query; views will pass 2 s (2.2).
const agentTimeout = 10 * time.Second

// Rows is the result of a guarded query.
type Rows struct {
	Columns []string
	// Values are at most max rows, as the driver gives them: int64,
	// float64, string, []byte or nil. Text is as stored, also in a column
	// declared DATE, DATETIME or TIMESTAMP.
	Values [][]any
	Total  int  // the query's rows in all; when Capped, it has more
	Cut    bool // Values stops before the last row
	Capped bool // counting stopped at sqlworker.CountCap rows
	Full   bool // Values stops early because its rows reached 8 MiB
}

// AgentQuery runs an agent's query through the three guard layers and
// returns at most max rows (at least 1), how many rows the query has, and
// whether the rows were cut. The query gets 10 s, and its worker is killed
// when they run out or when it reaches its memory cap. A layer that
// refuses it returns a *sqlguard.RejectError; SQLite's own errors (no such
// column) come back with their text.
func (p *ProjectDB) AgentQuery(ctx context.Context, q string, args []any, max int) (Rows, error) {
	return p.guardedQuery(ctx, q, args, max, agentTimeout)
}

// guardedQuery is AgentQuery with the caller's deadline d: 10 s for query
// and db.query, 2 s for views (2.2), not counting the time a worker takes
// to start. Layer 1 runs here first, so a query it refuses costs no
// worker; the worker runs all three layers again.
func (p *ProjectDB) guardedQuery(ctx context.Context, q string, args []any, n int, d time.Duration) (Rows, error) {
	if err := sqlguard.Check(q); err != nil {
		return Rows{}, err
	}
	r, err := p.workers.Query(ctx, sqlworker.Request{Query: q, Args: args, Max: max(n, 1), Timeout: d})
	switch {
	case errors.Is(err, sqlworker.ErrClosed):
		return Rows{}, ErrClosed
	case errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil:
		return Rows{}, fmt.Errorf("the query ran longer than %v and was stopped: %w", d, err)
	case err != nil:
		return Rows{}, err
	}
	return Rows(r), nil
}
