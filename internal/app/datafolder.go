package app

import (
	"context"
	"path/filepath"

	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

// dataFolderError refuses dir as the next data folder: a relative path, the
// projects folder in use or a folder inside it, or a folder inside a
// project's folder (TASK-068). nil when dir can be used.
func (s *SettingsService) dataFolderError(ctx context.Context, dir string) error {
	projects := ""
	if s.started.DataFolder != "" {
		projects = filepath.Join(s.started.DataFolder, "projects")
	}
	var list []store.ProjectEntry
	if s.registry != nil {
		l, err := s.registry.Projects(ctx)
		if err != nil {
			return err
		}
		list = l
	}
	if msg := project.DataFolderProblem(dir, projects, list); msg != "" {
		return &UIError{Kind: KindInvalid, Message: msg}
	}
	return nil
}
