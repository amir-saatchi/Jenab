package state

import (
	"context"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/id"
)

// ProjectMemoryAPI is a project's memory, which its settings edit
// (SPEC 3.3, TASK-062); *app.ProjectService has it.
type ProjectMemoryAPI interface {
	Memory(ctx context.Context, p id.Project) (app.MemoryView, error)
	SaveMemory(ctx context.Context, p id.Project, e app.MemoryEdit) (app.MemorySaved, error)
}

// ProjectMemory reads the project's memory.
func (p *Pages) ProjectMemory(proj id.Project, then func(app.MemoryView, error)) {
	pageCall(p.post, func() (app.MemoryView, error) {
		if p.memory == nil {
			return app.MemoryView{}, errNoService
		}
		return p.memory.Memory(background, proj)
	}, then)
}

// SaveProjectMemory saves an edit of a section of the project's memory.
func (p *Pages) SaveProjectMemory(proj id.Project, e app.MemoryEdit, then func(app.MemorySaved, error)) {
	pageCall(p.post, func() (app.MemorySaved, error) {
		if p.memory == nil {
			return app.MemorySaved{}, errNoService
		}
		return p.memory.SaveMemory(background, proj, e)
	}, then)
}

var _ ProjectMemoryAPI = (*app.ProjectService)(nil)
