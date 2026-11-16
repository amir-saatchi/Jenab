package state

import (
	"context"
	"strings"
	"unicode"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
)

// ProjectsAPI is what Projects reads; *app.ProjectService has it.
type ProjectsAPI interface {
	List(ctx context.Context) ([]app.ProjectItem, error)
	FolderWarning(ctx context.Context) (*project.FolderWarning, error)
	Create(ctx context.Context, name string) (app.OpenedProject, error)
	Open(ctx context.Context, p id.Project) (app.OpenedProject, error)
}

// Projects is the project list and the projects opened this session, with
// their Mother chat and approval level (state/projects.ts).
type Projects struct {
	Loaded bool
	List   []app.ProjectItem
	Opened map[id.Project]app.OpenedProject
	// FolderWarning is the 2.1 warning for the data folder, or nil.
	FolderWarning *project.FolderWarning

	api  ProjectsAPI
	post Post
}

// NewProjects makes the store.
func NewProjects(api ProjectsAPI, post Post) *Projects {
	return &Projects{Opened: map[id.Project]app.OpenedProject{}, api: api, post: post}
}

// Load reads the project list and the folder warning.
func (s *Projects) Load() <-chan error {
	done := make(chan error, 1)
	go func() {
		list, err := s.api.List(background)
		var w *project.FolderWarning
		if err == nil {
			w, err = s.api.FolderWarning(background)
		}
		s.post(func() {
			if err == nil {
				s.Loaded, s.List, s.FolderWarning = true, list, w
			}
			done <- err
		})
	}()
	return done
}

// Create makes a project and opens it.
func (s *Projects) Create(name string, then func(app.OpenedProject, error)) {
	go func() {
		op, err := s.api.Create(background, name)
		var list []app.ProjectItem
		if err == nil {
			list, err = s.api.List(background)
		}
		s.post(func() {
			if err == nil {
				s.List = list
				s.Opened[op.ID] = op
			}
			then(op, err)
		})
	}()
}

// Open opens a project, once a session.
func (s *Projects) Open(p id.Project, then func(app.OpenedProject, error)) {
	if op, ok := s.Opened[p]; ok {
		then(op, nil)
		return
	}
	go func() {
		op, err := s.api.Open(background, p)
		s.post(func() {
			if err == nil {
				s.Opened[op.ID] = op
			}
			then(op, err)
		})
	}()
}

// SetLevel sets an opened project's approval level.
func (s *Projects) SetLevel(p id.Project, level string) {
	if op, ok := s.Opened[p]; ok {
		op.Level = level
		s.Opened[p] = op
	}
}

// Initial is a project's mark in the rail: its first letter.
func Initial(name string) string {
	for _, r := range strings.TrimSpace(name) {
		return strings.ToUpper(string(r))
	}
	return "?"
}

// Initials is a chat's mark in the rail: the first letters of its first
// two words, or its first two characters.
func Initials(title string) string {
	words := strings.FieldsFunc(title, unicode.IsSpace)
	switch len(words) {
	case 0:
		return "?"
	case 1:
		r := []rune(words[0])
		return string(r[:min(2, len(r))])
	}
	return strings.ToUpper(string([]rune(words[0])[0]) + string([]rune(words[1])[0]))
}
