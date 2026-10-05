package app

import (
	"context"

	"github.com/amir-saatchi/jenab/internal/sysmem"
)

// SystemService is what the bottom bar shows about the computer (SPEC 5.12).
type SystemService struct {
	base
}

// Memory returns the free and total memory (8.5); the bottom bar reads it
// about every 2 s while the window is visible.
func (s *SystemService) Memory(ctx context.Context) (r sysmem.Reading, err error) {
	defer s.guard("system.memory", &err)
	return sysmem.Read()
}
