package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

// BucketService lists a project's files for the Files panel (SPEC 4,
// 5.12). The files themselves load from /objects/<project_id>/<key>, never
// through a binding (Q31).
type BucketService struct {
	base
	projects *project.Manager
	openFile func(path string) error // opens a file with the system app; nil opens nothing, as in tests
	tempDir  string                  // where opened copies go; "" is the OS temp folder
}

// Object is a file in the bucket.
type Object struct {
	Key       string     `json:"key"`
	SHA256    string     `json:"sha256"`
	MIME      string     `json:"mime"`
	Size      int64      `json:"size"`
	Version   int        `json:"version"`
	Source    string     `json:"source"` // upload, http, pipeline, agent or tool
	SourceURL string     `json:"source_url,omitempty"`
	CreatedBy id.Source  `json:"created_by"`
	UpdatedAt time.Time  `json:"updated_at"`
	Expires   *time.Time `json:"expires,omitempty"`
	// Title is the file's original name or a page's title from its
	// metadata; "" if it has none. Unlike the key, it may be in any script.
	Title string `json:"title,omitempty"`
	// Opens says whether *Open with the system app* opens it (4.7).
	Opens bool `json:"opens"`
}

func object(o store.Object) Object {
	out := Object{Key: o.Key, SHA256: o.SHA256, MIME: o.MIME, Size: o.Size, Version: o.Version,
		Source: o.Source, SourceURL: o.SourceURL, CreatedBy: o.CreatedBy, UpdatedAt: o.UpdatedAt,
		Title: metaTitle(o.Metadata), Opens: systemExt(o.MIME, o.Key) != ""}
	if !o.Expires.IsZero() {
		t := o.Expires
		out.Expires = &t
	}
	return out
}

// metaTitle is the name or title in an object's metadata, on one line.
func metaTitle(m json.RawMessage) string {
	var meta struct {
		Name  any `json:"name"`
		Title any `json:"title"`
	}
	if len(m) == 0 || json.Unmarshal(m, &meta) != nil {
		return ""
	}
	for _, v := range []any{meta.Name, meta.Title} {
		if t, ok := v.(string); ok && strings.TrimSpace(t) != "" {
			t = strings.Join(strings.Fields(t), " ")
			if r := []rune(t); len(r) > 200 {
				t = string(r[:200]) + "…"
			}
			return t
		}
	}
	return ""
}

// ObjectPage is one page of a folder.
type ObjectPage struct {
	Objects []Object `json:"objects"`
	Folders []string `json:"folders"` // ending in "/"
	Next    string   `json:"next"`    // pass as after for the next page; "" at the end
}

// PageSize is how many objects and folders a page holds.
const PageSize = 100

// List lists the files and folders directly under prefix.
func (s *BucketService) List(ctx context.Context, p id.Project, prefix, after string) (pg ObjectPage, err error) {
	defer s.guard("bucket.list", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		l, err := proj.DB.ListObjects(ctx, prefix, after, PageSize)
		if err != nil {
			return err
		}
		pg = ObjectPage{Objects: make([]Object, 0, len(l.Objects)), Folders: l.Folders, Next: l.Next}
		for _, o := range l.Objects {
			pg.Objects = append(pg.Objects, object(o))
		}
		return nil
	})
	if pg.Folders == nil {
		pg.Folders = []string{}
	}
	return pg, err
}

// ObjectVersion is one version of a file.
type ObjectVersion struct {
	Version   int       `json:"version"`
	SHA256    string    `json:"sha256"`
	Size      int64     `json:"size"`
	CreatedBy id.Source `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// Versions lists a file's versions, the newest first.
func (s *BucketService) Versions(ctx context.Context, p id.Project, key string) (vs []ObjectVersion, err error) {
	defer s.guard("bucket.versions", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		ovs, err := proj.DB.ObjectVersions(ctx, key)
		for _, v := range ovs {
			vs = append(vs, ObjectVersion{Version: v.Version, SHA256: v.SHA256, Size: v.Size, CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt})
		}
		return err
	})
	if vs == nil {
		vs = []ObjectVersion{}
	}
	return vs, err
}

// Head returns a file's current version, for the panel's file view.
func (s *BucketService) Head(ctx context.Context, p id.Project, key string) (o Object, err error) {
	defer s.guard("bucket.head", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		so, err := proj.DB.HeadObject(ctx, key)
		o = object(so)
		return err
	})
	return o, err
}

// Deleted is a delete from the Files panel: Change undoes it.
type Deleted struct {
	Change id.Change `json:"change"`
}

// Delete removes a file as the user. Its versions and bytes stay, so
// UndoDelete can put it back while the change is in the undo window
// (SPEC 2.6, 4.4).
func (s *BucketService) Delete(ctx context.Context, p id.Project, key string) (d Deleted, err error) {
	defer s.guard("bucket.delete", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		d.Change, err = proj.DB.DeleteObjectChange(ctx, limit.Interactive, id.SourceUser, key)
		return err
	})
	return d, err
}

// UndoDelete puts back a file that Delete removed. A file saved again
// under the same key since then stops it.
func (s *BucketService) UndoDelete(ctx context.Context, p id.Project, change id.Change) (err error) {
	defer s.guard("bucket.undo_delete", &err)
	return open(ctx, s.projects, p, func(proj *project.Project) error {
		_, err := proj.DB.UndoChange(ctx, change, store.UndoOptions{})
		switch {
		case errors.Is(err, store.ErrUndoConflict):
			return &UIError{Kind: KindBusy, Message: "A file was saved under this name since, so the delete can't be undone.", Details: err.Error()}
		case errors.Is(err, store.ErrNothingToUndo):
			return &UIError{Kind: KindBusy, Message: "The delete is undone already, or past the undo window.", Details: err.Error()}
		}
		return err
	})
}

// objectHandler serves /objects/<project_id>/<key> with bucket.Handler's
// rules for untrusted files (4.7). Each request leases the project until
// its body is sent.
func objectHandler(pm *project.Manager) http.Handler {
	h := bucket.Handler(func(ctx context.Context, p id.Project, key string) (bucket.Served, error) {
		proj, err := pm.Open(ctx, p)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return bucket.Served{}, bucket.ErrNotFound // no such project
			}
			return bucket.Served{}, err
		}
		o, body, err := proj.DB.OpenObject(ctx, key)
		if err != nil {
			proj.Release()
			return bucket.Served{}, err
		}
		return bucket.Served{MIME: o.MIME, Hash: o.SHA256, ModTime: o.UpdatedAt, Body: &leased{body, proj}}, nil
	})
	// Wails mounts a route's handler with the route cut off the path.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, bucket.Route) {
			r = r.Clone(r.Context())
			r.URL.Path = bucket.Route + strings.TrimPrefix(r.URL.Path, "/")
		}
		h.ServeHTTP(w, r)
	})
}

// leased is an object's body that gives back the project's lease when it
// is closed.
type leased struct {
	io.ReadSeekCloser
	proj *project.Project
}

func (l *leased) Close() error {
	err := l.ReadSeekCloser.Close()
	l.proj.Release()
	return err
}
