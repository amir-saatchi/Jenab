// Package project opens, closes and recovers projects (SPEC 2.1, 2.7, Q29).
// A Project owns its storage and its lock file; the work in it (chat
// runners, runs) lives in agent and pipeline and holds a lease while it
// runs.
package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Project is an open project. Get one with Manager.Open and give it back
// with Release.
type Project struct {
	ID    id.Project
	Name  string
	Dir   string
	DB    *store.ProjectDB
	Chats *store.ChatsDB
	// Damage is set when a database failed its quick_check after a crash.
	// The project is then open read-only (SPEC 2.7).
	Damage *Damage

	m      *Manager
	ctx    context.Context // cancelled when the project closes (Q20, Q22)
	cancel context.CancelFunc
	ready  chan struct{} // closed when opening has finished
	err    error         // the opening error; read after ready
	done   chan struct{} // closed when the project has closed

	// Guarded by m.mu.
	leases  int
	gen     int // bumped whenever the idle timer is stopped
	idle    *time.Timer
	closing bool

	mu        sync.Mutex
	next      int
	reporters map[int]Reporter
	onClose   map[int]func()
}

// Context is cancelled when the project closes. Background tasks and
// scheduled runs use it rather than the turn that started them (Q22).
func (p *Project) Context() context.Context { return p.ctx }

// ReadOnly reports whether the project is open read-only after damage.
func (p *Project) ReadOnly() bool { return p.Damage != nil }

// Release gives back the lease from Open. The last release starts the idle
// timer; the project closes when no one opens it for the idle time.
func (p *Project) Release() { p.m.release(p) }

// Report registers r, so Activity includes its work (Q19a).
func (p *Project) Report(r Reporter) (unregister func()) {
	return add(p, &p.reporters, r)
}

// OnClose registers fn to run when the project closes, before its
// databases close: for work that lives as long as the project, such as MCP
// servers (Phase 5).
func (p *Project) OnClose(fn func()) (remove func()) {
	return add(p, &p.onClose, fn)
}

func add[T any](p *Project, m *map[int]T, v T) func() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if *m == nil {
		*m = map[int]T{}
	}
	k := p.next
	p.next++
	(*m)[k] = v
	return func() {
		p.mu.Lock()
		delete(*m, k)
		p.mu.Unlock()
	}
}

// Activity is one snapshot of the project's work.
func (p *Project) Activity() Activity {
	p.mu.Lock()
	rs := make([]Reporter, 0, len(p.reporters))
	for _, k := range sortedKeys(p.reporters) {
		rs = append(rs, p.reporters[k])
	}
	p.mu.Unlock()
	a := Activity{Project: p.ID, Name: p.Name, Open: true, Work: []Status{}, Writer: p.DB.Stats(), Chats: p.Chats.Stats()}
	for _, r := range rs {
		a.Work = append(a.Work, r.Status()...)
	}
	p.m.mu.Lock()
	a.Leases, a.Open = p.leases, !p.closing
	p.m.mu.Unlock()
	return a
}

func sortedKeys[T any](m map[int]T) []int {
	ks := make([]int, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}

// close runs the OnClose functions, newest first, then drains the writer,
// checkpoints, closes the databases and removes the lock file (Q30 steps
// 3–4). If the databases don't close cleanly, the lock stays, so the next
// open checks them.
func (p *Project) close(ctx context.Context) error {
	p.mu.Lock()
	ks := sortedKeys(p.onClose)
	fns := make([]func(), 0, len(ks))
	for _, k := range slices.Backward(ks) {
		fns = append(fns, p.onClose[k])
	}
	p.onClose = nil
	p.mu.Unlock()
	for _, fn := range fns {
		p.m.safely("OnClose", p.ID, fn)
	}
	p.cancel()
	err := errors.Join(p.Chats.Close(ctx), p.DB.Close(ctx))
	if err == nil {
		if rmErr := os.Remove(lockPath(p.Dir)); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			err = rmErr
		}
	}
	return err
}

func lockPath(dir string) string { return filepath.Join(dir, "jenab.lock") }
