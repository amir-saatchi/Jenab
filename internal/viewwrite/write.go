// Package viewwrite writes data from views (SPEC 5.3, 5.5, 5.6): form
// submits that insert or update a row, row actions that open a form, set
// fields or delete rows, one row or a bulk of the loaded page, and the
// run_pipeline of forms and actions through a seam that Phase 4 fills.
//
// Every write goes through the project's writer queue as one interactive
// request with one change-log entry, its source form:<view_id> (SPEC 2.5,
// 2.6), so Undo turn never touches it and its notice undoes it on its own
// (store.ProjectDB.UndoChange). Values are checked in Go against the
// column types before anything is written, and a constraint the write
// breaks comes back on its field. Reads (a form's load and its select
// options) take the views' read-only path, as viewrun's do. Nothing here
// calls a model or opens a connection.
package viewwrite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/viewrun"
)

// MaxBulk is the most rows a bulk row action writes at once (SPEC 5.3):
// the loaded page at most, as one write request and one change-log entry.
const MaxBulk = 500

// ErrRequest is a request the view can't take, such as an action index it
// doesn't have or several rows for an action without bulk: a mistake of
// the caller, not of the user.
var ErrRequest = errors.New("viewwrite: the view can't take this request")

func badRequest(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrRequest, fmt.Sprintf(format, a...))
}

// Writer writes one project's rows from its views. It is safe for
// concurrent use; the writer queue orders the writes.
type Writer struct {
	DB *store.ProjectDB
	// Project is the project's ID, for Pipelines.
	Project id.Project
	// Now is the time in the project's local zone: today(), a date
	// field's relative default and "since 08:00"; nil is time.Now.
	Now func() time.Time
	// Pipelines starts the runs of run_pipeline; nil means no pipeline
	// can run yet (Phase 4).
	Pipelines Pipelines
	// Issued reports whether Upload handed out key for the file field
	// field of the form form in this app run: only such a key, or on an
	// update the row's current one, goes in a file field. nil hands out
	// none.
	Issued func(form, field, key string) bool
}

func (w *Writer) issued(form, field, key string) bool {
	return w.Issued != nil && w.Issued(form, field, key)
}

// keyOf reads k, a row's key as the page gives it, for the key column
// col of t, and returns it as the column keeps it. The page passes
// viewrun's typed keys, but a key is never trusted: a whole number for
// an INTEGER column is an int64, or a float64 that holds one exactly
// (below 2^53: from there on a float may be a rounded other key), or its
// digits; text for a TEXT column is a string. Anything else is a
// request error, never a write to another row.
func keyOf(t store.Table, col string, k any) (any, error) {
	c, ok := t.Column(col)
	if !ok {
		return nil, badRequest("%s has no column %s", t.Name, col)
	}
	switch x := k.(type) {
	case nil:
		return nil, badRequest("a row of %s has no key", t.Name)
	case []byte:
		k = string(x)
	case int:
		k = int64(x)
	case json.Number:
		if i, err := x.Int64(); err == nil {
			k = i
		} else if f, err := x.Float64(); err == nil {
			k = f
		} else {
			k = string(x)
		}
	}
	bad := func(why string) error {
		return badRequest("%#v is not a key of %s: %s", k, t.Name, why)
	}
	switch aff := affinity(c.Type, t.Strict); aff {
	case "INTEGER":
		switch x := k.(type) {
		case int64:
			return x, nil
		case float64:
			if x == math.Trunc(x) && math.Abs(x) < 1<<53 {
				return int64(x), nil
			}
			return nil, bad("not a whole number a float holds exactly")
		case string:
			if i, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64); err == nil {
				return i, nil
			}
		}
		return nil, bad(c.Name + " takes whole numbers")
	case "TEXT":
		if x, ok := k.(string); ok {
			return x, nil
		}
		return nil, bad(c.Name + " takes text")
	default:
		switch x := k.(type) {
		case int64, string:
			return x, nil
		case float64:
			if math.IsNaN(x) || math.IsInf(x, 0) || (x == math.Trunc(x) && math.Abs(x) >= 1<<53) {
				return nil, bad("not a number a float holds exactly")
			}
			return x, nil
		}
		return nil, bad("a key is a number or text")
	}
}

func (w *Writer) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

func (w *Writer) runner() *viewrun.Runner { return &viewrun.Runner{Source: w.DB, Now: w.Now} }

// Source is the source of the writes from the view vid: form:<vid>, for
// its form submits and its row actions alike (SPEC 2.5).
func Source(vid string) id.Source { return id.SourceOf("form", vid) }

// ---- the seam for run_pipeline (SPEC 5.5, 5.6) ----

// Pipelines starts pipeline runs for forms and actions. Phase 4 brings the
// real one, with the run queue of SPEC 6.2; tests use a fake.
type Pipelines interface {
	// Run starts the pipeline pipeline of project p with inputs, a form's
	// fields by name (nil for an action), as src. A pipeline running
	// already starts no second run and gives a *RunningError.
	Run(ctx context.Context, p id.Project, pipeline string, inputs map[string]any, src id.Source) (id.Run, error)
}

// RunningError is a run_pipeline whose pipeline runs already: no second
// run is queued (SPEC 5.5).
type RunningError struct {
	Pipeline string
	Run      id.Run    // the run that is running
	Since    time.Time // when it started
}

func (e *RunningError) Error() string {
	return fmt.Sprintf("viewwrite: %s is already running since %s", e.Pipeline, e.Since.Format(time.RFC3339))
}

// Message is what the form or the button says, with the start in the
// zone of now: "already running since 08:00", with the date when it
// started on another day.
func (e *RunningError) Message(now time.Time) string {
	since := e.Since.In(now.Location())
	y, m, d := since.Date()
	ny, nm, nd := now.Date()
	if y == ny && m == nm && d == nd {
		return "already running since " + since.Format("15:04")
	}
	if y == ny {
		return "already running since " + since.Format("Jan 2, 15:04")
	}
	return "already running since " + since.Format("Jan 2 2006, 15:04")
}

// runPipeline starts pipeline as src; problem says why it didn't start.
func (w *Writer) runPipeline(ctx context.Context, pipeline string, inputs map[string]any, src id.Source) (run id.Run, problem string, err error) {
	if w.Pipelines == nil {
		return "", "Pipelines can't run yet.", nil
	}
	run, err = w.Pipelines.Run(ctx, w.Project, pipeline, inputs, src)
	var re *RunningError
	switch {
	case errors.As(err, &re):
		return "", re.Message(w.now()), nil
	case errors.Is(err, store.ErrNotFound):
		return "", fmt.Sprintf("There is no pipeline %s.", pipeline), nil
	}
	return run, "", err
}
