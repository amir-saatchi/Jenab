package project

import (
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Publisher sends project events to the frontend; app implements it with
// Wails events (project:notice, project:activity). It must not block.
type Publisher interface {
	Notice(Notice)
	Activity(Activity)
}

// NoticeKind says what a notice reports. Readers accept kinds they don't
// know, since later versions add more.
type NoticeKind string

const (
	// NoticeRecovered: the last session did not end cleanly; the checks
	// passed and the project works (SPEC 2.7).
	NoticeRecovered NoticeKind = "recovered"
	// NoticeDamaged: a database failed its quick_check, so the project is
	// open read-only and the latest snapshot is offered (SPEC 2.7).
	NoticeDamaged NoticeKind = "damaged"
)

// Notice is a message about one project for the user.
type Notice struct {
	Project id.Project `json:"project"`
	Kind    NoticeKind `json:"kind"`
	Text    string     `json:"text"`
	Damage  *Damage    `json:"damage,omitempty"`
}

// Damage describes a database that failed its quick_check.
type Damage struct {
	File     string `json:"file"`     // project.db or chats.db
	Problem  string `json:"problem"`  // SQLite's first problems
	Snapshot string `json:"snapshot"` // the latest snapshot of that file, or "" if there is none
}

// Reporter is anything that works in a project and can say what it is doing:
// chat runners and pipeline runs (Q19a).
type Reporter interface {
	Status() []Status
}

// Status is one piece of work in a project.
type Status struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`  // e.g. turn, task, run
	State        string    `json:"state"` // e.g. running, waiting, failed
	Started      time.Time `json:"started"`
	LastActivity time.Time `json:"last_activity"`
	Progress     string    `json:"progress,omitempty"`
	Err          string    `json:"err,omitempty"`
}

// Activity is one snapshot of a project's work: every Reporter's status and
// the writer's queue (Q19a). Open is false once the project has closed.
type Activity struct {
	Project id.Project        `json:"project"`
	Open    bool              `json:"open"`
	Leases  int               `json:"leases"`
	Work    []Status          `json:"work"`
	Writer  store.WriterStats `json:"writer"` // project.db; chats.db joins with P1-06
}

type noPublisher struct{}

func (noPublisher) Notice(Notice)     {}
func (noPublisher) Activity(Activity) {}
