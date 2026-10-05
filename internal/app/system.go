package app

import (
	"context"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/sysmem"
)

// SystemService is what the app asks of the computer: the memory for the
// bottom bar and desktop notifications (SPEC 5.12, 8.8).
type SystemService struct {
	base
	notify func(Notification) error // nil sends nothing, as in tests
}

// Memory returns the free and total memory (8.5); the bottom bar reads it
// about every 2 s while the window is visible.
func (s *SystemService) Memory(ctx context.Context) (r sysmem.Reading, err error) {
	defer s.guard("system.memory", &err)
	return sysmem.Read()
}

// Notification is a desktop notification for a chat that waits for the
// user. A click shows the window and sends app:open with the chat.
type Notification struct {
	Project id.Project `json:"project"`
	Chat    id.Chat    `json:"chat"`
	Title   string     `json:"title"`
	Body    string     `json:"body"`
}

// Notify shows n. The frontend calls it when a chat starts to wait while
// it isn't on screen or the window isn't focused (8.8).
func (s *SystemService) Notify(ctx context.Context, n Notification) (err error) {
	defer s.guard("system.notify", &err)
	if s.notify == nil {
		return nil
	}
	return s.notify(n)
}
