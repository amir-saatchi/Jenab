package app

import (
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
)

// The events the UI gets (Q32). Each payload carries its IDs, and
// chat events the chat's sequence number.
const (
	EventDelta    = "chat:delta"
	EventPart     = "chat:part"
	EventStatus   = "chat:status"
	EventNotice   = "project:notice"
	EventActivity = "project:activity"
	EventOpen     = "app:open" // a desktop notification was clicked
	EventExport   = "project:export"
	// EventConfigs is a change to views and pages (project.ConfigsChanged):
	// open pages read them again, and a deleted one closes (SPEC 5.9).
	EventConfigs = "project:configs"
	// EventLinks is a change to the saved links (project.LinksChanged): the
	// Links tab reads them again (SPEC 4.6, P3-11).
	EventLinks = "project:links"
	// run:status comes with the pipelines (Phase 3).
)

// Open is a chat to show, after its notification was clicked.
type Open struct {
	Project id.Project `json:"project"`
	Chat    id.Chat    `json:"chat"`
}

// Publisher sends events to the UI. It implements the Publisher of agent
// and of project. Emitting never blocks. The UI layer gives it emit:
// cmd/desktop posts each event to the UI thread, which applies it to the
// stores (internal/native/state).
type Publisher struct {
	emit func(name string, data any)
}

// NewPublisher makes a Publisher that sends every event through emit,
// which must not block.
func NewPublisher(emit func(name string, data any)) *Publisher {
	return &Publisher{emit: emit}
}

func (p *Publisher) Delta(d chat.Delta)               { p.emit(EventDelta, d) }
func (p *Publisher) Part(d chat.PartDone)             { p.emit(EventPart, d) }
func (p *Publisher) Status(s chat.Status)             { p.emit(EventStatus, s) }
func (p *Publisher) Notice(n project.Notice)          { p.emit(EventNotice, n) }
func (p *Publisher) Activity(a project.Activity)      { p.emit(EventActivity, a) }
func (p *Publisher) Configs(c project.ConfigsChanged) { p.emit(EventConfigs, c) }
func (p *Publisher) Links(c project.LinksChanged)     { p.emit(EventLinks, c) }

// Open asks the UI to show a chat, after its notification was clicked.
func (p *Publisher) Open(o Open) { p.emit(EventOpen, o) }
