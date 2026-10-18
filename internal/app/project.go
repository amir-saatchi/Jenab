package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

// ProjectService is the project list and opening projects (SPEC 2.1, 2.7).
type ProjectService struct {
	base
	projects *project.Manager
	emit     func(name string, data any) // sends events; set by Bind
	exports  sync.Map                    // id.Project → *runningExport
}

// runningExport is an export the app runs, for StopExport and ExportStatus.
type runningExport struct {
	cancel context.CancelFunc
	mu     sync.Mutex
	last   project.ExportProgress
}

// ProjectItem is a row of the project list.
type ProjectItem struct {
	ID         id.Project `json:"id"`
	Name       string     `json:"name"`
	Folder     string     `json:"folder"`
	Pinned     bool       `json:"pinned"`
	LastOpened *time.Time `json:"last_opened,omitempty"` // nil if never opened
	CreatedAt  time.Time  `json:"created_at"`
}

func projectItem(e store.ProjectEntry) ProjectItem {
	it := ProjectItem{ID: e.ID, Name: e.Name, Folder: e.Folder, Pinned: e.Pinned, CreatedAt: e.CreatedAt}
	if !e.LastOpened.IsZero() {
		t := e.LastOpened
		it.LastOpened = &t
	}
	return it
}

// List returns every project.
func (s *ProjectService) List(ctx context.Context) (items []ProjectItem, err error) {
	defer s.guard("project.list", &err)
	es, err := s.projects.List(ctx)
	items = make([]ProjectItem, 0, len(es))
	for _, e := range es {
		items = append(items, projectItem(e))
	}
	return items, err
}

// OpenedProject is a project as the app shell needs it.
type OpenedProject struct {
	ID     id.Project `json:"id"`
	Name   string     `json:"name"`
	Mother id.Chat    `json:"mother"` // the Mother chat, opened first (8.6)
	Level  string     `json:"level"`  // the approval level (8.8)
	// Damage is set when the project is open read-only after damage (2.7).
	Damage *project.Damage `json:"damage,omitempty"`
}

// Create makes a project and opens it (*Create project*, 3.9).
func (s *ProjectService) Create(ctx context.Context, name string) (op OpenedProject, err error) {
	defer s.guard("project.create", &err)
	pid, err := s.projects.Create(ctx, name)
	if err != nil {
		return op, err
	}
	return s.open(ctx, pid)
}

// Open opens a project. Its notices, such as a recovery, come as
// project:notice events.
func (s *ProjectService) Open(ctx context.Context, p id.Project) (op OpenedProject, err error) {
	defer s.guard("project.open", &err)
	return s.open(ctx, p)
}

func (s *ProjectService) open(ctx context.Context, p id.Project) (op OpenedProject, err error) {
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		chats, err := proj.Chats.Chats(ctx)
		if err != nil {
			return err
		}
		for _, c := range chats {
			if c.Kind == chat.KindMother {
				op.Mother = c.ID
			}
		}
		l, err := proj.Level(ctx)
		op.ID, op.Name, op.Level, op.Damage = proj.ID, proj.Name, string(l), proj.Damage
		return err
	})
	return op, err
}

// Activity is the project's work now; changes come as project:activity
// events.
func (s *ProjectService) Activity(ctx context.Context, p id.Project) (a project.Activity, err error) {
	defer s.guard("project.activity", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		a = proj.Activity()
		return nil
	})
	return a, err
}

// FolderWarning says whether the data folder is a bad place for the
// projects, for the banner in 2.1; nil when it is fine.
func (s *ProjectService) FolderWarning(ctx context.Context) (w *project.FolderWarning, err error) {
	defer s.guard("project.folder_warning", &err)
	return s.projects.FolderWarning(), nil
}

// ExportEstimate is what an export of the project would copy now, for the
// export dialog.
func (s *ProjectService) ExportEstimate(ctx context.Context, p id.Project) (e project.ExportEstimate, err error) {
	defer s.guard("project.export_estimate", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		e, err = proj.Estimate(ctx)
		return err
	})
	return e, err
}

// ExportRequest is the place the user chose in the save dialog.
type ExportRequest struct {
	Target string               `json:"target"`
	Format project.ExportFormat `json:"format"`
	// Replace lets a zip export replace the file at Target; set it only
	// when the save dialog asked the user about it.
	Replace bool `json:"replace"`
	// IncludeApprovals keeps the approvals, approval level and private
	// hosts, for the user's own backup.
	IncludeApprovals bool `json:"include_approvals"`
}

// Export copies the project to the place in r while it stays in use
// (SPEC 2.1). Its progress comes as project:export events, and the last
// one says how it ended, so a dialog that ExportStatus found it for can
// follow it too. StopExport, or cancelling the call, stops it and removes
// what it wrote.
func (s *ProjectService) Export(ctx context.Context, p id.Project, r ExportRequest) (res project.ExportResult, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	run := &runningExport{cancel: cancel, last: project.ExportProgress{Project: p, Target: r.Target}}
	// One export per project: being in s.exports is what lets it run, so
	// StopExport and ExportStatus always find the one running.
	if _, running := s.exports.LoadOrStore(p, run); running {
		defer s.guard("project.export", &err)
		return res, project.ErrExportRunning
	}
	defer s.end(run, &res, &err) // after guard, with the error made a *UIError
	defer s.guard("project.export", &err)
	defer s.exports.CompareAndDelete(p, run)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		res, err = proj.Export(ctx, project.ExportOptions{Target: r.Target, Format: r.Format, Replace: r.Replace, IncludeApprovals: r.IncludeApprovals,
			Progress: func(pr project.ExportProgress) {
				run.mu.Lock()
				run.last = pr
				run.mu.Unlock()
				s.emit(EventExport, pr)
			}})
		return err
	})
	return res, err
}

// end sends an export's last event.
func (s *ProjectService) end(run *runningExport, res *project.ExportResult, err *error) {
	run.mu.Lock()
	pr := run.last
	run.mu.Unlock()
	if *err == nil {
		pr.Step, pr.Target, pr.Result = project.StepDone, res.Path, res
	} else {
		pr.Step, pr.Error = project.StepFailed, (*err).Error()
		var ue *UIError
		if errors.As(*err, &ue) {
			pr.Error, pr.Stopped = ue.Message, ue.Kind == KindClosing
		}
	}
	s.emit(EventExport, pr)
}

// StopExport stops the project's running export. Its Export call then
// fails, once what it wrote is removed. It reports whether one was running.
func (s *ProjectService) StopExport(ctx context.Context, p id.Project) (stopped bool, err error) {
	defer s.guard("project.stop_export", &err)
	if v, ok := s.exports.Load(p); ok {
		v.(*runningExport).cancel()
		return true, nil
	}
	return false, nil
}

// ExportStatus is the project's running export, as its last progress
// event; nil when none runs.
func (s *ProjectService) ExportStatus(ctx context.Context, p id.Project) (pr *project.ExportProgress, err error) {
	defer s.guard("project.export_status", &err)
	if v, ok := s.exports.Load(p); ok {
		run := v.(*runningExport)
		run.mu.Lock()
		last := run.last
		run.mu.Unlock()
		return &last, nil
	}
	return nil, nil
}
