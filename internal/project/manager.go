package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

// IdleAfter is how long a project stays open with no leases (SPEC 2.7).
const IdleAfter = 10 * time.Minute

// closeTimeout bounds an idle close: draining the writer and checkpointing
// (Q30 step 3).
const closeTimeout = 3 * time.Second

var (
	ErrClosed = errors.New("project: the app is shutting down")
	ErrNoName = errors.New("project: a project needs a name")
)

// Meta keys in _jenab_meta (SPEC 2.2).
const (
	metaID      = "id"
	metaName    = "name"
	metaCreated = "created_at"
)

// Deps are the Manager's dependencies.
type Deps struct {
	Paths    config.Paths
	Registry *store.Registry
	Log      *slog.Logger
	Events   Publisher     // nil sends nothing
	Idle     time.Duration // 0 means IdleAfter
}

// Manager keeps one Project per open project, with its leases (Q29).
type Manager struct {
	d Deps

	mu     sync.Mutex
	open   map[id.Project]*Project
	closed bool

	warnOnce sync.Once
	warning  *FolderWarning
}

// NewManager returns a Manager. It does no I/O.
func NewManager(d Deps) *Manager {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	if d.Events == nil {
		d.Events = noPublisher{}
	}
	if d.Idle <= 0 {
		d.Idle = IdleAfter
	}
	return &Manager{d: d, open: map[id.Project]*Project{}}
}

// FolderWarning reports whether the data folder is on a network drive or in
// a synced folder (SPEC 2.1). It is checked once.
func (m *Manager) FolderWarning() *FolderWarning {
	m.warnOnce.Do(func() { m.warning = CheckFolder(m.d.Paths.DataFolder) })
	return m.warning
}

// Create makes a project folder with the 2.1 layout and an empty project.db,
// and adds it to the registry. It does not open the project.
func (m *Manager) Create(ctx context.Context, name string) (id.Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrNoName
	}
	pid := id.Project(id.New())
	dir := filepath.Join(m.d.Paths.Projects, string(pid))
	if err := m.create(ctx, pid, name, dir); err != nil {
		os.RemoveAll(dir) // only what this call made: the ID is new
		return "", fmt.Errorf("project: create %q: %w", name, err)
	}
	return pid, nil
}

func (m *Manager) create(ctx context.Context, pid id.Project, name, dir string) error {
	for _, sub := range []string{"objects", "snapshots", "tmp"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return err
		}
	}
	db, err := store.OpenProject(ctx, dir)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	err = db.SetMeta(ctx, map[string]string{metaID: string(pid), metaName: name, metaCreated: now.Format(time.RFC3339Nano)})
	if err = errors.Join(err, db.Close(ctx)); err != nil {
		return err
	}
	return m.d.Registry.SaveProject(ctx, store.ProjectEntry{ID: pid, Name: name, Folder: dir, CreatedAt: now})
}

// List returns every project. Project folders the registry doesn't know,
// for example after registry.db was lost, are added first from their
// _jenab_meta (SPEC 2.1).
func (m *Manager) List(ctx context.Context) ([]store.ProjectEntry, error) {
	if err := m.scan(ctx); err != nil {
		m.d.Log.Warn("project: scan of the projects folder failed", "err", err)
	}
	return m.d.Registry.Projects(ctx)
}

func (m *Manager) scan(ctx context.Context) error {
	ents, err := os.ReadDir(m.d.Paths.Projects)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	known, err := m.d.Registry.Projects(ctx)
	if err != nil {
		return err
	}
	have := map[id.Project]bool{}
	for _, p := range known {
		have[p.ID] = true
	}
	for _, e := range ents {
		pid := id.Project(e.Name())
		if !e.IsDir() || !id.Valid(e.Name()) || have[pid] {
			continue
		}
		dir := filepath.Join(m.d.Paths.Projects, e.Name())
		meta, err := store.ReadProjectMeta(ctx, dir)
		if err != nil || meta[metaID] != string(pid) {
			m.d.Log.Warn("project: folder skipped, not a readable project", "folder", dir, "err", err)
			continue
		}
		created, err := time.Parse(time.RFC3339Nano, meta[metaCreated])
		if err != nil {
			created = time.Now()
		}
		if err := m.d.Registry.SaveProject(ctx, store.ProjectEntry{ID: pid, Name: meta[metaName], Folder: dir, CreatedAt: created}); err != nil {
			return err
		}
		m.d.Log.Info("project: added to the registry from its folder", "project", pid)
	}
	return nil
}

// Open returns the open project and takes a lease on it; call Release when
// done. The first Open opens the databases and, if the last session did not
// end cleanly, recovers the project first (SPEC 2.7).
func (m *Manager) Open(ctx context.Context, pid id.Project) (*Project, error) {
	for {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return nil, ErrClosed
		}
		p := m.open[pid]
		if p == nil {
			p = &Project{ID: pid, m: m, ready: make(chan struct{}), done: make(chan struct{}), leases: 1}
			m.open[pid] = p
			m.mu.Unlock()
			p.err = m.load(ctx, p)
			if p.err != nil {
				m.mu.Lock()
				delete(m.open, pid)
				m.mu.Unlock()
				close(p.done)
			}
			close(p.ready)
			if p.err != nil {
				return nil, p.err
			}
			m.d.Events.Activity(p.Activity())
			return p, nil
		}
		if p.closing {
			m.mu.Unlock()
			select {
			case <-p.done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		p.leases++
		m.stopIdle(p)
		m.mu.Unlock()
		select {
		case <-p.ready:
		case <-ctx.Done():
			p.Release()
			return nil, ctx.Err()
		}
		if p.err != nil {
			return nil, p.err
		}
		return p, nil
	}
}

// load opens the project's storage, recovering it first if the lock file
// is there.
func (m *Manager) load(ctx context.Context, p *Project) error {
	e, err := m.d.Registry.Project(ctx, p.ID)
	if err != nil {
		return err
	}
	p.Name, p.Dir = e.Name, e.Folder
	if _, err := os.Stat(filepath.Join(p.Dir, "project.db")); err != nil {
		return fmt.Errorf("project %s: %w", p.ID, err)
	}
	crashed := false
	if _, err := os.Stat(lockPath(p.Dir)); err == nil {
		crashed = true
		m.d.Log.Warn("project: the last session did not end cleanly; recovering", "project", p.ID)
		if p.Damage, err = recoverProject(ctx, p.Dir); err != nil {
			return fmt.Errorf("project %s: recovery: %w", p.ID, err)
		}
	}
	if err := writeLock(p.Dir); err != nil {
		return fmt.Errorf("project %s: %w", p.ID, err)
	}
	if p.Damage != nil {
		p.DB, err = store.OpenProjectReadOnly(ctx, p.Dir)
	} else {
		p.DB, err = store.OpenProject(ctx, p.Dir)
	}
	if err != nil {
		if !crashed {
			os.Remove(lockPath(p.Dir)) // nothing was opened; the files are as they were
		}
		return fmt.Errorf("project %s: %w", p.ID, err)
	}
	p.ctx, p.cancel = context.WithCancel(context.Background())
	if err := m.d.Registry.TouchProject(ctx, p.ID, time.Now()); err != nil {
		m.d.Log.Warn("project: could not save the open time", "project", p.ID, "err", err)
	}
	switch {
	case p.Damage != nil:
		m.d.Log.Error("project: damaged, opened read-only", "project", p.ID, "file", p.Damage.File, "problem", p.Damage.Problem)
		m.d.Events.Notice(Notice{Project: p.ID, Kind: NoticeDamaged, Damage: p.Damage,
			Text: p.Damage.File + " is damaged, so the project is open read-only."})
	case crashed:
		m.d.Events.Notice(Notice{Project: p.ID, Kind: NoticeRecovered,
			Text: "The last session did not end cleanly. The project was checked and works."})
	}
	return nil
}

// release gives back one lease; the last one starts the idle timer.
func (m *Manager) release(p *Project) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p.leases <= 0 {
		m.d.Log.Error("project: Release without a lease", "project", p.ID)
		return
	}
	p.leases--
	// A failed open never reaches 0 here: its opener keeps its lease.
	if p.leases > 0 || p.closing {
		return
	}
	p.gen++
	g := p.gen
	p.idle = time.AfterFunc(m.d.Idle, func() { m.closeIdle(p, g) })
}

// stopIdle stops the idle timer. A timer that already fired sees the new
// generation and does nothing. m.mu is held.
func (m *Manager) stopIdle(p *Project) {
	if p.idle != nil {
		p.idle.Stop()
		p.idle = nil
	}
	p.gen++
}

func (m *Manager) closeIdle(p *Project, gen int) {
	m.mu.Lock()
	if p.gen != gen || p.leases > 0 || p.closing {
		m.mu.Unlock()
		return
	}
	p.closing = true
	p.idle = nil
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	m.finish(ctx, p)
}

// finish closes p and removes it from the open set.
func (m *Manager) finish(ctx context.Context, p *Project) error {
	err := p.close(ctx)
	if err != nil {
		m.d.Log.Error("project: close", "project", p.ID, "err", err)
	} else {
		m.d.Log.Info("project: closed", "project", p.ID)
	}
	m.mu.Lock()
	if m.open[p.ID] == p {
		delete(m.open, p.ID)
	}
	m.mu.Unlock()
	close(p.done)
	m.d.Events.Activity(Activity{Project: p.ID, Work: []Status{}})
	return err
}

// Busy reports whether any project has work holding a lease, for the update
// restart, which waits until nothing runs (Q30).
func (m *Manager) Busy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.open {
		if p.leases > 0 {
			return true
		}
	}
	return false
}

// CloseAll closes every project for shutdown, leases or not (Q30 steps 3–4),
// and refuses later opens. Projects still opening finish opening first.
func (m *Manager) CloseAll(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	var mine, others []*Project
	for _, p := range m.open {
		m.stopIdle(p)
		if p.closing {
			others = append(others, p) // an idle close is already running
			continue
		}
		p.closing = true
		mine = append(mine, p)
	}
	m.mu.Unlock()

	errs := make([]error, len(mine))
	var wg sync.WaitGroup
	for i, p := range mine {
		wg.Go(func() {
			select {
			case <-p.ready:
			case <-ctx.Done():
				errs[i] = fmt.Errorf("project %s: still opening at shutdown: %w", p.ID, ctx.Err())
				return
			}
			if p.err == nil {
				errs[i] = m.finish(ctx, p)
			}
		})
	}
	wg.Wait()
	for _, p := range others {
		select {
		case <-p.done:
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf("project %s: still closing at shutdown: %w", p.ID, ctx.Err()))
		}
	}
	return errors.Join(errs...)
}

// safely runs fn and logs a panic instead of passing it on (Q26).
func (m *Manager) safely(what string, pid id.Project, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			m.d.Log.Error("project: panic", "in", what, "project", pid, "panic", r)
		}
	}()
	fn()
}
