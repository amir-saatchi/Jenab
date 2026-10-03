package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/amir-saatchi/jenab/internal/limit"
)

// A request's state. The switch from waiting to taken or given up is one
// compare-and-swap, so a request its caller gave up on never runs (SPEC 7.3).
const (
	waiting int32 = iota
	taken
	givenUp
)

// job is one queued request. run gets the writer's connection; Do wraps its
// function in a transaction, Checkpoint runs outside one.
type job interface {
	state() *atomic.Int32
	run(ctx context.Context, c *sql.Conn) // sends its own reply
	fail(err error)                       // replies with err without running
}

// writer owns the only write connection of a file and runs one request at a
// time from two queues, with the 10:1 rule (SPEC 7.2, 7.3).
type writer struct {
	ctx   context.Context // for the requests it runs; never cancelled by callers
	conn  func() *sql.Conn
	reset func(cause any) // after a panic: roll back and reopen the connection (Q26)

	mu      sync.Mutex
	queues  [2][]job // by limit.Priority
	fair    limit.Fair
	wake    chan struct{}
	closing bool
	abort   bool // Close ran out of time: fail what is still queued
	done    chan struct{}

	// stats
	current   atomic.Pointer[time.Time]
	completed atomic.Int64
	lastErr   atomic.Pointer[string]
}

func newWriter(conn func() *sql.Conn, reset func(any)) *writer {
	return &writer{
		ctx:   context.Background(),
		conn:  conn,
		reset: reset,
		wake:  make(chan struct{}, 1),
		done:  make(chan struct{}),
	}
}

// WriterStats is a snapshot for the runtime panel (Q19a).
type WriterStats struct {
	Interactive, Background int           // requests waiting
	Busy                    bool          // a request is running
	Age                     time.Duration // how long it has run
	Done                    int64         // requests finished since open
	LastError               string
}

func (w *writer) stats() WriterStats {
	w.mu.Lock()
	s := WriterStats{Interactive: len(w.queues[limit.Interactive]), Background: len(w.queues[limit.Background])}
	w.mu.Unlock()
	if t := w.current.Load(); t != nil {
		s.Busy, s.Age = true, time.Since(*t)
	}
	s.Done = w.completed.Load()
	if e := w.lastErr.Load(); e != nil {
		s.LastError = *e
	}
	return s
}

// enqueue adds j, or fails it at once if the writer is closing.
func (w *writer) enqueue(p limit.Priority, j job) {
	if p != limit.Background {
		p = limit.Interactive
	}
	w.mu.Lock()
	if w.closing {
		w.mu.Unlock()
		j.fail(ErrClosed)
		return
	}
	w.queues[p] = append(w.queues[p], j)
	w.mu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// next takes the next request, or reports that the writer should stop.
func (w *writer) next() (job, bool) {
	for {
		w.mu.Lock()
		in, bg := len(w.queues[limit.Interactive]) > 0, len(w.queues[limit.Background]) > 0
		if w.abort {
			for _, q := range w.queues {
				for _, j := range q {
					if j.state().CompareAndSwap(waiting, givenUp) {
						j.fail(ErrClosed)
					}
				}
			}
			w.queues = [2][]job{}
			in, bg = false, false
		}
		if in || bg {
			p := w.fair.Next(in, bg)
			j := w.queues[p][0]
			w.queues[p][0] = nil
			w.queues[p] = w.queues[p][1:]
			w.mu.Unlock()
			return j, true
		}
		stop := w.closing
		w.mu.Unlock()
		if stop {
			return nil, false
		}
		<-w.wake
	}
}

func (w *writer) loop() {
	defer close(w.done)
	for {
		j, ok := w.next()
		if !ok {
			return
		}
		if !j.state().CompareAndSwap(waiting, taken) {
			continue // its caller gave up; skip it (Q21)
		}
		now := time.Now()
		w.current.Store(&now)
		w.runSafe(j)
		w.current.Store(nil)
		w.completed.Add(1)
	}
}

// runSafe runs one request and recovers a panic, so the writer keeps
// serving (Q26). The request then gets an error.
func (w *writer) runSafe(j job) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("store: write request panicked: %v", r)
			w.setErr(err)
			if w.reset != nil {
				w.reset(r)
			}
			j.fail(&PanicError{Value: r, Stack: debug.Stack()})
		}
	}()
	j.run(w.ctx, w.conn())
}

func (w *writer) setErr(err error) {
	s := err.Error()
	w.lastErr.Store(&s)
}

// close stops taking new requests and waits until the queued ones are done.
// If ctx ends first, the ones still waiting fail with ErrClosed; a running one
// always finishes.
func (w *writer) close(ctx context.Context) {
	w.mu.Lock()
	w.closing = true
	w.mu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default:
	}
	select {
	case <-w.done:
		return
	case <-ctx.Done():
	}
	w.mu.Lock()
	w.abort = true
	w.mu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default:
	}
	<-w.done
}

// PanicError is returned to the caller whose write request panicked.
type PanicError struct {
	Value any
	Stack []byte
}

func (e *PanicError) Error() string { return fmt.Sprintf("store: write request panicked: %v", e.Value) }

// ---- Do: a typed write in one transaction ----

type result[T any] struct {
	v   T
	err error
}

type txJob[T any] struct {
	st    atomic.Int32
	fn    func(*sql.Tx) (T, error)
	reply chan result[T] // room for one, so the writer never waits on a caller that left
}

func (j *txJob[T]) state() *atomic.Int32 { return &j.st }
func (j *txJob[T]) fail(err error)       { j.reply <- result[T]{err: err} }

func (j *txJob[T]) run(ctx context.Context, c *sql.Conn) {
	var r result[T]
	r.v, r.err = inTx(ctx, c, j.fn)
	j.reply <- r
}

// inTx runs fn in one transaction. A panic rolls back before it goes on up.
func inTx[T any](ctx context.Context, c *sql.Conn, fn func(*sql.Tx) (T, error)) (v T, err error) {
	tx, err := c.BeginTx(ctx, nil)
	if err != nil {
		return v, err
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()
	v, err = fn(tx)
	if err != nil {
		if rb := tx.Rollback(); rb != nil && !errors.Is(rb, sql.ErrTxDone) {
			err = errors.Join(err, rb)
		}
		var zero T
		return zero, err
	}
	if err := tx.Commit(); err != nil {
		var zero T
		return zero, err
	}
	return v, nil
}

// Do runs fn in one write transaction on db's writer and returns its result
// (Q27). While the request waits in the queue, cancelling ctx gives it up and
// fn never runs. Once the writer has taken it, it runs to the end and Do
// waits for the result, even if ctx is cancelled (SPEC 7.3). fn must do only
// database work (SPEC 7.4). Code outside store uses the typed requests, not
// Do.
func Do[T any](ctx context.Context, db *DB, p limit.Priority, fn func(*sql.Tx) (T, error)) (T, error) {
	if db.w == nil {
		var zero T
		return zero, ErrReadOnly
	}
	j := &txJob[T]{fn: fn, reply: make(chan result[T], 1)}
	return await(ctx, db.w, p, j, j.reply)
}

func await[T any](ctx context.Context, w *writer, p limit.Priority, j job, reply chan result[T]) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	w.enqueue(p, j)
	select {
	case r := <-reply:
		if r.err != nil {
			w.setErrIfDB(r.err)
		}
		return r.v, r.err
	case <-ctx.Done():
		if j.state().CompareAndSwap(waiting, givenUp) {
			return zero, ctx.Err()
		}
		r := <-reply // taken (or failed by close): wait for the answer
		return r.v, r.err
	}
}

// setErrIfDB records errors from SQLite for Stats, but not the caller's own
// errors such as a revision conflict.
func (w *writer) setErrIfDB(err error) {
	if errors.Is(err, ErrConflict) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrClosed) {
		return
	}
	w.setErr(err)
}

// ---- conn jobs: work outside a transaction, such as a checkpoint ----

type connJob[T any] struct {
	st    atomic.Int32
	fn    func(context.Context, *sql.Conn) (T, error)
	reply chan result[T]
}

func (j *connJob[T]) state() *atomic.Int32 { return &j.st }
func (j *connJob[T]) fail(err error)       { j.reply <- result[T]{err: err} }
func (j *connJob[T]) run(ctx context.Context, c *sql.Conn) {
	var r result[T]
	r.v, r.err = j.fn(ctx, c)
	j.reply <- r
}

func doConn[T any](ctx context.Context, db *DB, p limit.Priority, fn func(context.Context, *sql.Conn) (T, error)) (T, error) {
	if db.w == nil {
		var zero T
		return zero, ErrReadOnly
	}
	j := &connJob[T]{fn: fn, reply: make(chan result[T], 1)}
	return await(ctx, db.w, p, j, j.reply)
}
