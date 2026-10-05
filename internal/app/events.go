package app

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
)

// The events the frontend gets (Q32). Each payload carries its IDs, and
// chat events the chat's sequence number.
const (
	EventDelta    = "chat:delta"
	EventPart     = "chat:part"
	EventStatus   = "chat:status"
	EventNotice   = "project:notice"
	EventActivity = "project:activity"
	EventOpen     = "app:open" // a desktop notification was clicked
	// run:status comes with the pipelines (Phase 3).
)

// Open is a chat to show, after its notification was clicked.
type Open struct {
	Project id.Project `json:"project"`
	Chat    id.Chat    `json:"chat"`
}

// init is the only one in the app (CODE-OUTLINE 0). The bindings
// generator reads these calls to write the events' TypeScript types.
func init() {
	application.RegisterEvent[chat.Delta](EventDelta)
	application.RegisterEvent[chat.PartDone](EventPart)
	application.RegisterEvent[chat.Status](EventStatus)
	application.RegisterEvent[project.Notice](EventNotice)
	application.RegisterEvent[project.Activity](EventActivity)
	application.RegisterEvent[Open](EventOpen)
}

// Publisher sends events to the frontend. It implements the Publisher of
// agent and of project. Emitting never blocks.
type Publisher struct {
	emit func(name string, data any)
}

func (p *Publisher) Delta(d chat.Delta)          { p.emit(EventDelta, d) }
func (p *Publisher) Part(d chat.PartDone)        { p.emit(EventPart, d) }
func (p *Publisher) Status(s chat.Status)        { p.emit(EventStatus, s) }
func (p *Publisher) Notice(n project.Notice)     { p.emit(EventNotice, n) }
func (p *Publisher) Activity(a project.Activity) { p.emit(EventActivity, a) }
