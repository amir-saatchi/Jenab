package app

import (
	"context"
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
