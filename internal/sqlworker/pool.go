package sqlworker

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/amir-saatchi/jenab/internal/sqlguard"
)

// Options configure a pool.
type Options struct {
	// Workers is how many queries run at once, each in its own worker, so
	// a view's query isn't stuck behind a 10 s one. 0 means 2.
	Workers int
	// Memory caps each worker, in bytes; 0 means DefaultMemory.
	Memory uint64
	// Idle is how long a worker waits for its next query before it is
	// stopped; 0 means 3 minutes.
	Idle time.Duration
}

// DefaultMemory is a worker's memory cap, in committed memory. A worker
// commits about 48 MiB before its first query, most of it modernc's
// SQLite, which it barely touches (its working set is about 8 MiB). The
// analytic queries in sqlworker_test.go peak at 52–100 MiB on 1M rows,
// since sorts go to temp files.
const DefaultMemory = 256 << 20

const defaultIdle = 3 * time.Minute

// startTimeout bounds a worker's start, which a query's Timeout doesn't
// count: the first start of a newly built app, while the virus scanner
// reads it, has taken 1.7 s.
const startTimeout = 30 * time.Second

var (
	// ErrClosed is a query on a closed pool, or one the closing stopped.
	ErrClosed    = errors.New("sqlworker: the pool is closed")
	errDisabled  = errors.New("sqlworker: this program can't run workers: call IfWorker first in main, or in TestMain")
	errSlowStart = fmt.Errorf("sqlworker: a worker didn't start within %v", startTimeout)
)

// MemoryError is a query that reached its worker's memory cap.
type MemoryError struct{ Limit uint64 }

func (e *MemoryError) Error() string {
	return fmt.Sprintf("the query used more than %d MiB and was stopped", e.Limit>>20)
}

// executable is the worker's program: this one, which must call IfWorker
// first in main or TestMain.
var executable = os.Executable

// started, when set, gets each worker as soon as its process runs; for
// tests.
var started func(*worker)

// Pool runs queries on one database file, in workers it starts when they
// are first needed. It is safe for concurrent use.
type Pool struct {
	path  string
	o     Options
	slots chan struct{} // a query holds one while it runs

	mu       sync.Mutex
	idle     []*worker
	all      map[*worker]bool
	closed   bool
	starting sync.WaitGroup // starts under way, which Close waits for
}

// New returns a pool for the database file at path. It starts no worker
// yet.
func New(path string, o Options) *Pool {
	if o.Workers <= 0 {
		o.Workers = 2
	}
	if o.Memory == 0 {
		o.Memory = DefaultMemory
	}
	if o.Idle <= 0 {
		o.Idle = defaultIdle
	}
	return &Pool{path: path, o: o, slots: make(chan struct{}, o.Workers), all: map[*worker]bool{}}
}

// Query runs req in a worker. When ctx ends, or req.Timeout passes, the
// worker is killed and Query returns the context's error; the next query
// starts a new one. The timeout counts the wait for a free worker but not
// a worker's start, which only ctx and startTimeout bound. A query that
// reaches the memory cap returns a *MemoryError. Refusals are
// *sqlguard.RejectError, and SQLite's errors come back with their text.
func (p *Pool) Query(ctx context.Context, req Request) (Result, error) {
	if !enabled.Load() {
		return Result{}, errDisabled
	}
	begin := time.Now()
	frame, err := encodeRequest(req)
	if err != nil {
		return Result{}, err
	}
	select {
	case p.slots <- struct{}{}:
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
	defer func() { <-p.slots }()
	var starts time.Duration
	for try := 0; ; try++ {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		w, took, err := p.take(ctx, try > 0)
		if err != nil {
			return Result{}, err
		}
		starts += took
		qctx, cancel := ctx, context.CancelFunc(func() {})
		if req.Timeout > 0 {
			qctx, cancel = context.WithDeadline(ctx, begin.Add(req.Timeout+starts))
		}
		if err := qctx.Err(); err != nil { // it waited its time for a slot
			cancel()
			p.put(w)
			return Result{}, err
		}
		res, err := w.query(qctx, frame)
		cancel()
		p.put(w)
		// A worker killed from outside while it waited never answers: the
		// query runs once more, in a new worker.
		var na noAnswer
		if try == 0 && took == 0 && errors.As(err, &na) {
			continue
		}
		return res, err
	}
}

// Warm starts a worker in the background, if the pool has none, so that
// the first query doesn't wait for one. In a program that doesn't call
// IfWorker it does nothing.
func (p *Pool) Warm() {
	if !enabled.Load() {
		return
	}
	go func() {
		p.mu.Lock()
		some := len(p.all) > 0
		p.mu.Unlock()
		if some {
			return
		}
		if w, _, err := p.take(context.Background(), true); err == nil {
			p.put(w)
		}
	}()
}

// Close stops every worker, and waits until they have exited. Queries
// still running return ErrClosed.
func (p *Pool) Close() {
	p.mu.Lock()
	p.closed = true
	ws := p.all
	p.idle, p.all = nil, map[*worker]bool{}
	p.mu.Unlock()
	for w := range ws {
		w.kill(whyClosed)
	}
	for w := range ws {
		<-w.exited
	}
	p.starting.Wait() // a start that finds the pool closed stops its worker
}

// take returns an idle worker, or, if there is none or fresh is set,
// starts one; took is how long the start took.
func (p *Pool) take(ctx context.Context, fresh bool) (w *worker, took time.Duration, err error) {
	p.mu.Lock()
	for !fresh && len(p.idle) > 0 {
		w := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]
		w.timer.Stop()
		if w.alive() {
			p.mu.Unlock()
			return w, 0, nil
		}
		delete(p.all, w)
	}
	if p.closed {
		p.mu.Unlock()
		return nil, 0, ErrClosed
	}
	p.starting.Add(1)
	p.mu.Unlock()
	defer p.starting.Done()
	begin := time.Now()
	w, err = p.start(ctx)
	return w, time.Since(begin), err
}

// start starts a worker and waits until it has opened the file. The pool
// knows the worker from the start, so Close stops it.
func (p *Pool) start(ctx context.Context) (*worker, error) {
	w, err := start(p.path, p.o.Memory)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	closed := p.closed
	if !closed {
		p.all[w] = true
	}
	p.mu.Unlock()
	if closed {
		w.kill(whyClosed)
		<-w.exited
		return nil, ErrClosed
	}
	ctx, cancel := context.WithTimeoutCause(ctx, startTimeout, errSlowStart)
	defer cancel()
	if _, err = w.query(ctx, nil); err == nil {
		return w, nil
	}
	if errors.Is(context.Cause(ctx), errSlowStart) {
		err = errSlowStart
	}
	p.mu.Lock()
	delete(p.all, w)
	p.mu.Unlock()
	w.kill(whyBroken)
	<-w.exited
	return nil, fmt.Errorf("sqlworker: start a worker: %w", err)
}

// put takes a worker back after a query: it waits for the next one, or
// is dropped if it was killed or died, or enough wait already.
func (p *Pool) put(w *worker) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || !w.alive() || len(p.idle) >= p.o.Workers {
		delete(p.all, w)
		w.kill(whyClosed)
		return
	}
	p.idle = append(p.idle, w)
	w.timer = time.AfterFunc(p.o.Idle, func() { p.retire(w) })
}

// retire stops a worker that waited Idle for a query.
func (p *Pool) retire(w *worker) {
	p.mu.Lock()
	i := slices.Index(p.idle, w)
	if i >= 0 {
		p.idle = slices.Delete(p.idle, i, i+1)
		delete(p.all, w)
	}
	p.mu.Unlock()
	if i >= 0 {
		w.kill(whyIdle)
	}
}

// pids returns the workers' process IDs, for tests.
func (p *Pool) pids() []int {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []int
	for w := range p.all {
		out = append(out, w.cmd.Process.Pid)
	}
	return out
}

// Why a worker was killed.
const (
	whyNone int32 = iota
	whyDeadline
	whyMemory
	whyClosed
	whyIdle
	whyBroken
)

// worker is one worker process.
type worker struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Reader
	lim    *limits
	errs   tail
	memory uint64
	exited chan struct{} // closed when the process has ended and lim is released
	why    atomic.Int32  // the first reason it was killed
	timer  *time.Timer   // while idle; guarded by the pool's mu
}

func start(path string, memory uint64) (*worker, error) {
	exe, err := executable()
	if err != nil {
		return nil, fmt.Errorf("sqlworker: %w", err)
	}
	cmd := exec.Command(exe, Flag, path, strconv.FormatUint(memory, 10))
	configure(cmd)
	w := &worker{cmd: cmd, memory: memory, exited: make(chan struct{})}
	cmd.Stderr = &w.errs
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("sqlworker: %w", err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("sqlworker: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("sqlworker: start a worker: %w", err)
	}
	w.in, w.out = in, bufio.NewReaderSize(out, 64<<10)
	// The worker only opens the file before its first request, which is
	// sent after this.
	if w.lim, err = limit(cmd, memory); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, fmt.Errorf("sqlworker: limit a worker: %w", err)
	}
	go w.lim.watch(func() { w.kill(whyMemory) })
	go func() {
		cmd.Wait()
		w.lim.release()
		close(w.exited)
	}()
	if started != nil {
		started(w)
	}
	return w, nil
}

func (w *worker) alive() bool {
	select {
	case <-w.exited:
		return false
	default:
		return w.why.Load() == whyNone
	}
}

// kill ends the worker; the first reason given is kept.
func (w *worker) kill(why int32) {
	w.why.CompareAndSwap(whyNone, why)
	w.lim.kill()
}

// query sends one request and reads its answer; with no frame, it reads
// the frame a new worker sends once it is ready. ctx ending kills the
// worker, which ends the read.
func (w *worker) query(ctx context.Context, frame []byte) (Result, error) {
	stop := context.AfterFunc(ctx, func() { w.kill(whyDeadline) })
	var b []byte
	var err error
	if frame != nil {
		_, err = w.in.Write(frame)
	}
	sent := err == nil
	if sent {
		b, err = readFrame(w.out)
	}
	if stop() && err == nil {
		res, err := decodeResponse(b)
		var we *workerError
		switch {
		case errors.As(err, &we) && we.code&0xff == sqliteNoMem:
			// SQLite got no more memory: the worker is at its cap.
			w.kill(whyMemory)
		case err != nil && we == nil && !isReject(err):
			w.kill(whyBroken) // a frame it can't have sent
			return Result{}, err
		default:
			return res, err
		}
	}
	// The worker is gone or going; once it has exited, its job has told
	// whether it reached the cap.
	w.kill(whyBroken)
	<-w.exited
	switch {
	case w.why.Load() == whyMemory || w.lim.memoryHit():
		return Result{}, &MemoryError{Limit: w.memory}
	case w.why.Load() == whyClosed:
		return Result{}, ErrClosed
	case ctx.Err() != nil:
		return Result{}, ctx.Err()
	}
	msg := strings.TrimSpace(w.errs.String())
	if msg == "" {
		msg = w.cmd.ProcessState.String()
	}
	if err == nil {
		err = errors.New("no answer") // not seen: ctx or the cap ended it
	}
	err = fmt.Errorf("sqlworker: the worker stopped: %s: %w", msg, err)
	if !sent || errors.Is(err, io.EOF) {
		return Result{}, noAnswer{err}
	}
	return Result{}, err
}

// noAnswer is a worker that stopped before it read the request or wrote
// any of its answer.
type noAnswer struct{ error }

func (e noAnswer) Unwrap() error { return e.error }

func isReject(err error) bool {
	var re *sqlguard.RejectError
	return errors.As(err, &re)
}

// tail keeps the last bytes a worker wrote to stderr, for the error when
// it dies.
type tail struct {
	mu sync.Mutex
	b  []byte
}

const tailSize = 2 << 10

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > tailSize {
		t.b = append(t.b[:0], t.b[len(t.b)-tailSize:]...)
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.b)
}
