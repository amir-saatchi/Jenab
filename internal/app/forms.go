package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/view"
	"github.com/amir-saatchi/jenab/internal/viewwrite"
)

// FormService writes data from the page screen (SPEC 5.3, 5.5, 5.6):
// form submits, a form's file uploads, row actions and the run_pipeline
// of a view's actions. Each write is one request with one change-log
// entry whose source is form:<view_id>, so Undo turn leaves it alone; the
// notice the page shows after it undoes it with Undo.
//
// The page re-runs its blocks after a write that has a Table. The data
// (viewwrite's types) goes to the kit's form block as it is: Value is the
// kit's FormValue field for field.
type FormService struct {
	base
	projects  *project.Manager
	now       func() time.Time // the project's local time: today(), relative dates
	pipelines viewwrite.Pipelines

	mu     sync.Mutex
	issued map[uploadRef]bool // the file keys Upload handed out in this app run that no submit has used yet
}

// uploadRef is a key Upload handed out for a form's file field.
type uploadRef struct {
	project     id.Project
	form, field string
	key         string
}

func (s *FormService) writer(proj *project.Project) *viewwrite.Writer {
	return &viewwrite.Writer{DB: proj.DB, Project: proj.ID, Now: s.now, Pipelines: s.pipelines,
		Issued: func(form, field, key string) bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.issued[uploadRef{proj.ID, form, field, key}]
		}}
}

func (s *FormService) issue(r uploadRef) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.issued == nil {
		s.issued = map[uploadRef]bool{}
	}
	s.issued[r] = true
}

// used returns the keys Upload handed out for the form v that values
// carry in its file fields, and forgets them: a submit uses each once.
func (s *FormService) used(p id.Project, v *view.View, values map[string]viewwrite.Value) []uploadRef {
	f, ok := v.Spec.(*view.Form)
	if !ok {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []uploadRef
	for _, fd := range f.Fields {
		if fd.Control != "file" {
			continue
		}
		r := uploadRef{p, v.ID, fd.Field, strings.TrimSpace(values[fd.Field].Text)}
		if s.issued[r] {
			delete(s.issued, r)
			out = append(out, r)
		}
	}
	return out
}

// view reads the view vid for a write: one that is gone or no longer
// valid is an error with its own message, as the page has nothing to
// write with.
func (s *FormService) view(ctx context.Context, proj *project.Project, vid string) (*view.View, error) {
	v, err := loadView(ctx, proj.DB, vid)
	var nv noView
	var inv *view.InvalidError
	switch {
	case errors.As(err, &nv):
		return nil, &UIError{Kind: KindNotFound, Message: nv.Error(), Details: nv.Error(), Cause: store.ErrNotFound}
	case errors.As(err, &inv):
		return nil, &UIError{Kind: KindInvalid, Message: inv.Error(), Details: inv.Error()}
	}
	return v, err
}

// request turns a request the view can't take into an invalid one.
func request(err error) error {
	if errors.Is(err, viewwrite.ErrRequest) {
		return &UIError{Kind: KindInvalid, Message: "The view can't do this.", Details: err.Error()}
	}
	return err
}

// Open reads the form fid to show it: its fields, select options, and an
// insert form's defaults or the row with key that an update form edits
// (the Key of a row action's Acted).
func (s *FormService) Open(ctx context.Context, p id.Project, fid string, key any) (f viewwrite.Form, err error) {
	defer s.guard("form.open", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		v, err := s.view(ctx, proj, fid)
		if err != nil {
			return err
		}
		f, err = s.writer(proj).Open(ctx, v, key)
		return request(err)
	})
	return f, err
}

// Submit writes the form fid with values by field, for an update form
// the row with key. Errors the user can fix come back in Submitted, on
// their fields; the error is for the rest.
//
// A file field takes only a key Upload handed out for it in this app
// run, or on an update the row's own. When the submit fails, the files
// uploaded for it go to the bucket's trash, as form:<fid>, and
// Submitted.Removed names their fields; files uploaded for a form that is
// never submitted stay.
func (s *FormService) Submit(ctx context.Context, p id.Project, fid string, key any, values map[string]viewwrite.Value) (sub viewwrite.Submitted, err error) {
	defer s.guard("form.submit", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		v, err := s.view(ctx, proj, fid)
		if err != nil {
			return err
		}
		sub, err = s.writer(proj).Submit(ctx, v, key, values)
		used := s.used(proj.ID, v, values)
		if err == nil && sub.OK() {
			return nil
		}
		// Not under ctx: a cancelled submit still clears its files.
		dctx := context.WithoutCancel(ctx)
		for _, r := range used {
			if _, derr := proj.DB.DeleteObjectChange(dctx, limit.Interactive, viewwrite.Source(fid), r.key); derr != nil && !errors.Is(derr, store.ErrNotFound) {
				s.log.Warn("app: an upload of a failed submit stays", "form", fid, "key", r.key, "err", s.redact(derr.Error()))
				continue
			}
			sub.Removed = append(sub.Removed, r.field)
		}
		return request(err)
	})
	return sub, err
}

// FormUpload is a file a form's file field stored. Key goes in the
// field's value (Value.Text) and is written when the form is submitted.
type FormUpload struct {
	Key  string `json:"key"`
	Name string `json:"name"` // as it was on the user's computer
	MIME string `json:"mime"`
	Size int64  `json:"size"`
}

// Upload stores the file at path for the file field field of the form fid,
// and hands out its key for that field in this app run. path must come
// from the user's file picker in the native UI, never from a page or the
// agent; only a regular file is read, so a pipe or a device can't block
// it. It is stored under the field's prefix with the file's name, never over another file:
// a taken key gets a number, as the composer's uploads. progress, if not
// nil, gets the share read so far, from 0 to 1, on the calling goroutine.
// The file stays in the bucket when the form isn't submitted; its own
// change-log entry is form:<fid>'s, so Undo turn leaves it too.
func (s *FormService) Upload(ctx context.Context, p id.Project, fid, field, path string, progress func(float64)) (up FormUpload, err error) {
	defer s.guard("form.upload", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		v, err := s.view(ctx, proj, fid)
		if err != nil {
			return err
		}
		f, ok := v.Spec.(*view.Form)
		if !ok {
			return request(fmt.Errorf("%w: %s is not a form", viewwrite.ErrRequest, fid))
		}
		var fd *view.FormField
		for i := range f.Fields {
			if f.Fields[i].Field == field && f.Fields[i].Control == "file" {
				fd = &f.Fields[i]
			}
		}
		if fd == nil {
			return request(fmt.Errorf("%w: %s has no file field %s", viewwrite.ErrRequest, fid, field))
		}
		info, err := os.Stat(path)
		if err != nil {
			return &UIError{Kind: KindInvalid, Message: "The file can't be read.", Details: err.Error()}
		}
		if err := regular(info, path); err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return &UIError{Kind: KindInvalid, Message: "The file can't be read.", Details: err.Error()}
		}
		defer file.Close()
		// What was opened, which may not be what was looked at.
		if info, err = file.Stat(); err != nil {
			return err
		}
		if err := regular(info, path); err != nil {
			return err
		}
		if info.Size() > store.MaxUpload {
			return &UIError{Kind: KindInvalid, Message: tooLarge(), Details: path}
		}
		name := filepath.Base(path)
		key, err := reserve(ctx, proj, fd.Prefix+strings.TrimPrefix(uploadKey(name), "uploads/"))
		if err != nil {
			return err
		}
		defer release(proj.ID, key)
		var r io.Reader = file
		if progress != nil {
			r = &counting{r: file, total: info.Size(), progress: progress}
			progress(0)
		}
		meta, _ := json.Marshal(map[string]string{"name": name})
		o, err := proj.DB.PutObject(ctx, limit.Interactive, viewwrite.Source(fid), key, r, store.PutOptions{Source: "upload", Metadata: meta})
		if errors.Is(err, bucket.ErrTooLarge) {
			return &UIError{Kind: KindInvalid, Message: tooLarge(), Details: err.Error()}
		}
		if err != nil {
			return err
		}
		if progress != nil {
			progress(1)
		}
		up = FormUpload{Key: o.Key, Name: name, MIME: o.MIME, Size: o.Size}
		s.issue(uploadRef{proj.ID, fid, field, o.Key})
		return nil
	})
	return up, err
}

// regular refuses a folder, a pipe, a device or anything else that isn't
// a regular file.
func regular(info os.FileInfo, path string) error {
	switch {
	case info.IsDir():
		return &UIError{Kind: KindInvalid, Message: "Choose a file, not a folder.", Details: path}
	case !info.Mode().IsRegular():
		return &UIError{Kind: KindInvalid, Message: "Choose a regular file.", Details: path + ": " + info.Mode().String()}
	}
	return nil
}

// counting reports the share of a file read so far.
type counting struct {
	r        io.Reader
	n, total int64
	last     float64
	progress func(float64)
}

func (c *counting) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.n += int64(n)
	if c.total > 0 {
		// Every 1% at most, and never 1 before the file is stored.
		if share := min(float64(c.n)/float64(c.total), 0.99); share-c.last >= 0.01 {
			c.last = share
			c.progress(share)
		}
	}
	return n, err
}

// RowAction runs the row action action (its index in row_actions) of the
// table view vid on the chosen rows of the loaded page, by their keys
// (viewrun.Table.Keys). An action with confirm asks first: Acted.Confirm
// is the question, and the page calls again with confirmed.
func (s *FormService) RowAction(ctx context.Context, p id.Project, vid string, action int, keys []any, confirmed bool) (a viewwrite.Acted, err error) {
	defer s.guard("form.row_action", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		v, err := s.view(ctx, proj, vid)
		if err != nil {
			return err
		}
		a, err = s.writer(proj).RowAction(ctx, v, action, keys, confirmed)
		return request(err)
	})
	return a, err
}

// Action runs the action action (its index in actions) of the view vid:
// a run_pipeline starts its pipeline; open_page and open_form say what to
// open.
func (s *FormService) Action(ctx context.Context, p id.Project, vid string, action int, confirmed bool) (a viewwrite.Acted, err error) {
	defer s.guard("form.action", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		v, err := s.view(ctx, proj, vid)
		if err != nil {
			return err
		}
		a, err = s.writer(proj).Action(ctx, v, action, confirmed)
		return request(err)
	})
	return a, err
}

// Undo undoes a write of a form, a row action or an upload: the Undo of
// its notice. It undoes only a change whose source is form:<view_id>;
// any other, such as an agent's or a migration's, is not found.
//
// Both refusals are KindBusy, as the other undo paths report them; Cause
// tells them apart for errors.Is: store.ErrUndoConflict when a later
// change to the same rows or file stops it, store.ErrNothingToUndo when
// it is undone already or past the undo window.
func (s *FormService) Undo(ctx context.Context, p id.Project, change id.Change) (err error) {
	defer s.guard("form.undo", &err)
	return open(ctx, s.projects, p, func(proj *project.Project) error {
		srcs, err := store.Query(ctx, proj.DB.DB, "SELECT source FROM _jenab_changes WHERE id = ?", []any{string(change)},
			func(r *sql.Rows) (string, error) {
				var src string
				return src, r.Scan(&src)
			})
		if err != nil {
			return err
		}
		if len(srcs) == 0 || id.Source(srcs[0]).Kind() != "form" {
			return &UIError{Kind: KindNotFound, Message: "There is no such change of a form.", Details: string(change), Cause: store.ErrNotFound}
		}
		r, err := proj.DB.UndoChange(ctx, change, store.UndoOptions{})
		switch {
		case errors.Is(err, store.ErrUndoConflict):
			msg := "These rows were changed again since, so this can't be undone."
			if n := len(r.Conflicts); n > 1 {
				msg = fmt.Sprintf("%d of these rows were changed again since, so this can't be undone.", n)
			}
			return &UIError{Kind: KindBusy, Message: msg, Details: err.Error(), Cause: store.ErrUndoConflict}
		case errors.Is(err, store.ErrNothingToUndo):
			return &UIError{Kind: KindBusy, Message: "This is undone already, or past the undo window.", Details: err.Error(), Cause: store.ErrNothingToUndo}
		}
		return err
	})
}
