package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/view"
	"github.com/amir-saatchi/jenab/internal/viewrun"
)

// ViewService runs a project's views and pages for the page screen (SPEC
// 5.7): each block's query on the read-only path through the query
// workers, 2 s a block, the blocks of a page in parallel. It calls no
// model and opens no connection (R-48).
//
// Page and View run while the caller waits, and stop when ctx ends. Start
// runs a page in the background under a key, as the page open in a chat:
// starting again under the same key, as when the filters change, stops
// the run before it, and Stop stops it when the page closes.
type ViewService struct {
	base
	projects *project.Manager
	now      func() time.Time // the project's local time, for relative dates

	mu   sync.Mutex
	runs map[string]*viewRun // the newest run under each key
	gens map[string]uint64   // each key's run count: a run's answers pass while it is the newest
}

// viewRun is a page running in the background.
type viewRun struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func (s *ViewService) runner(proj *project.Project) *viewrun.Runner {
	return &viewrun.Runner{Source: proj.DB, Now: s.now}
}

// View runs the view vid on its own, with its own filters.
func (s *ViewService) View(ctx context.Context, p id.Project, vid string, req viewrun.ViewRequest) (b viewrun.Block, err error) {
	defer s.guard("view.run", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		v, err := loadView(ctx, proj.DB, vid)
		if err != nil {
			return err
		}
		b = s.runner(proj).View(ctx, v, req)
		return ctx.Err()
	})
	return b, err
}

// Page runs the data blocks of the page pid in parallel. out gets the
// page's filters first, then each block as it answers, one call at a time
// and from other goroutines; it should return quickly, as posting to the
// UI thread does. A view the page shows that is gone, or can't be read,
// is an error on its block. Page returns when every block has answered,
// or ctx's error once its queries have stopped.
func (s *ViewService) Page(ctx context.Context, p id.Project, pid string, req viewrun.PageRequest, out func(viewrun.Answer)) (err error) {
	defer s.guard("view.page", &err)
	return open(ctx, s.projects, p, func(proj *project.Project) error {
		c, err := proj.DB.Config(ctx, store.KindPage, pid)
		if err != nil {
			return err
		}
		page, err := view.ParsePage(view.Default, []byte(c.YAML))
		if err != nil {
			return fmt.Errorf("the page %s can't be read: %w", pid, err)
		}
		views := map[string]viewOrError{}
		for _, pv := range viewrun.Places(page) {
			if _, ok := views[pv.View]; !ok {
				v, err := loadView(ctx, proj.DB, pv.View)
				views[pv.View] = viewOrError{v, err}
			}
		}
		return s.runner(proj).Page(ctx, page, func(vid string) (*view.View, error) {
			v := views[vid]
			return v.v, v.err
		}, req, out)
	})
}

type viewOrError struct {
	v   *view.View
	err error
}

// loadView reads and parses the saved view vid. One that is gone is "There
// is no view vid.", as its block says.
func loadView(ctx context.Context, db *store.ProjectDB, vid string) (*view.View, error) {
	c, err := db.Config(ctx, store.KindView, vid)
	if errors.Is(err, store.ErrNotFound) {
		return nil, noView(vid)
	}
	if err != nil {
		return nil, err
	}
	v, err := view.ParseView(view.Default, []byte(c.YAML))
	if err != nil {
		return nil, fmt.Errorf("The view %s can't be read: %v.", vid, err)
	}
	return v, nil
}

// noView is a view that isn't there: "There is no view vid." on its
// block, not found from View.
type noView string

func (v noView) Error() string { return fmt.Sprintf("There is no view %s.", string(v)) }
func (v noView) Unwrap() error { return store.ErrNotFound }

// Start runs the page pid as Page does, in the background, under key (as
// the chat the page is open in). A run already under key is stopped
// first: the new run starts once the old one's queries have stopped, and
// no answer of the old run reaches out after Start returns, except one
// out call already under way (as with Stop). done, if not
// nil, gets Page's error when the run ends: nil, or context.Canceled when
// it was stopped. out and done may call Start and Stop.
func (s *ViewService) Start(key string, p id.Project, pid string, req viewrun.PageRequest, out func(viewrun.Answer), done func(error)) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &viewRun{cancel: cancel, done: make(chan struct{})}
	s.mu.Lock()
	if s.runs == nil {
		s.runs, s.gens = map[string]*viewRun{}, map[string]uint64{}
	}
	s.gens[key]++
	gen := s.gens[key]
	old := s.runs[key]
	s.runs[key] = r
	s.mu.Unlock()
	if old != nil {
		old.cancel()
	}
	// deliver passes an answer on while the run is still the newest
	// under key and wasn't stopped. The check is under the lock; out is
	// called without it.
	deliver := func(a viewrun.Answer) {
		if s.current(key, gen) {
			out(a)
		}
	}
	go func() {
		defer close(r.done)
		if old != nil {
			<-old.done // its queries have stopped, and it calls out no more
		}
		var err error
		if ctx.Err() == nil {
			err = s.Page(ctx, p, pid, req, deliver)
		}
		stopped := ctx.Err() != nil
		s.mu.Lock()
		if s.runs[key] == r {
			delete(s.runs, key)
		}
		s.mu.Unlock()
		cancel()
		if done != nil {
			if stopped {
				err = context.Canceled
			}
			done(err)
		}
	}()
}

// current reports whether gen is still the newest run under key.
func (s *ViewService) current(key string, gen uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gens[key] == gen
}

// Stop stops the run under key, as when its page closes: no answer of it
// reaches out after Stop returns, except one out call already under way,
// and its queries are being stopped (done tells when they have). It
// doesn't wait for them, so out may call it. It does nothing when no run
// is under key.
func (s *ViewService) Stop(key string) {
	s.mu.Lock()
	r := s.runs[key]
	if r != nil {
		// The run stays under key until it ends, so a Start after this
		// waits for it.
		s.gens[key]++
	}
	s.mu.Unlock()
	if r != nil {
		r.cancel()
	}
}
