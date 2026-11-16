package statetest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/sysmem"
)

// File is a file in a Fake project's bucket.
type File struct {
	Data  []byte
	MIME  string
	Title string // the original name, in any script
	// Versions are the older versions' sizes, oldest first; the current
	// one is added to them.
	Versions []int64
	Opens    bool
}

// FakeBucket is the files of the Fake's projects and what was done to
// them: the bucket and the bottom bar's readings (TASK-056).
type FakeBucket struct {
	Files    map[id.Project]map[string]File
	PageSize int // a List page; 100 when zero
	Deleted  map[id.Change]deletedFile
	Opened   []string // the keys OpenWithSystem was asked for
	Memory   sysmem.Reading
	Paused   []app.ProviderStatus
	changes  int
}

type deletedFile struct {
	p    id.Project
	key  string
	file File
}

// ProjectID is a valid project ID for Fake projects whose files have
// jenab:// links, which need a ULID.
const ProjectID id.Project = "01JABCDEFGHJKMNPQRSTVWXYZ0"

// AddProjectAs adds a project with the ID p, as AddProject does.
func (f *Fake) AddProjectAs(p id.Project, name string) id.Chat {
	f.mu.Lock()
	defer f.mu.Unlock()
	mother := chat.Chat{ID: id.Chat(string(p) + "-mother"), Kind: chat.KindMother, Title: "Mother", Model: "default", CreatedAt: time.Unix(0, 0)}
	f.Projects = append(f.Projects, app.ProjectItem{ID: p, Name: name, Folder: `C:\Projects\` + name, CreatedAt: time.Unix(0, 0)})
	f.Mothers[p] = mother.ID
	f.Chats[p] = append(f.Chats[p], app.ChatItem{Chat: mother, State: chat.StateIdle})
	return mother.ID
}

// AddFile stores a file in project p under key.
func (f *Fake) AddFile(p id.Project, key string, file File) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Bucket.Files == nil {
		f.Bucket.Files = map[id.Project]map[string]File{}
	}
	if f.Bucket.Files[p] == nil {
		f.Bucket.Files[p] = map[string]File{}
	}
	f.Bucket.Files[p][key] = file
}

// OpenedWithSystem returns the keys OpenWithSystem was asked for.
func (f *Fake) OpenedWithSystem() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.Bucket.Opened)
}

// SetMemory sets the bottom bar's memory reading.
func (f *Fake) SetMemory(r sysmem.Reading) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Bucket.Memory = r
}

// bucket is the Fake as state.BucketAPI, whose List clashes with the
// chats'.
type bucket struct{ f *Fake }

func notFound() error {
	return &app.UIError{Kind: app.KindNotFound, Message: "This file no longer exists."}
}

func object(key string, file File) app.Object {
	sum := sha256.Sum256(file.Data)
	return app.Object{Key: key, SHA256: hex.EncodeToString(sum[:]), MIME: file.MIME, Size: int64(len(file.Data)),
		Version: len(file.Versions) + 1, Source: "upload", CreatedBy: id.SourceUser, UpdatedAt: time.Unix(0, 0),
		Title: file.Title, Opens: file.Opens}
}

func (b bucket) List(_ context.Context, p id.Project, prefix, after string) (app.ObjectPage, error) {
	b.f.mu.Lock()
	defer b.f.mu.Unlock()
	files := b.f.Bucket.Files[p]
	seen := map[string]bool{}
	var names []string
	for key := range files {
		rest, ok := strings.CutPrefix(key, prefix)
		if !ok {
			continue
		}
		name := key
		if i := strings.Index(rest, "/"); i >= 0 {
			name = prefix + rest[:i+1]
		}
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	slices.Sort(names)
	from := 0
	if after != "" {
		from = slices.Index(names, after) + 1
	}
	size := b.f.Bucket.PageSize
	if size <= 0 {
		size = app.PageSize
	}
	pg := app.ObjectPage{Folders: []string{}, Objects: []app.Object{}}
	end := min(from+size, len(names))
	for _, n := range names[from:end] {
		if strings.HasSuffix(n, "/") {
			pg.Folders = append(pg.Folders, n)
		} else {
			pg.Objects = append(pg.Objects, object(n, files[n]))
		}
	}
	if end < len(names) {
		pg.Next = names[end-1]
	}
	return pg, nil
}

func (b bucket) file(p id.Project, key string) (File, bool) {
	file, ok := b.f.Bucket.Files[p][key]
	return file, ok
}

func (b bucket) Head(_ context.Context, p id.Project, key string) (app.Object, error) {
	b.f.mu.Lock()
	defer b.f.mu.Unlock()
	file, ok := b.file(p, key)
	if !ok {
		return app.Object{}, notFound()
	}
	return object(key, file), nil
}

func (b bucket) Versions(_ context.Context, p id.Project, key string) ([]app.ObjectVersion, error) {
	b.f.mu.Lock()
	defer b.f.mu.Unlock()
	file, ok := b.file(p, key)
	if !ok {
		return nil, notFound()
	}
	sizes := append(slices.Clone(file.Versions), int64(len(file.Data)))
	var out []app.ObjectVersion
	for i := len(sizes) - 1; i >= 0; i-- {
		out = append(out, app.ObjectVersion{Version: i + 1, Size: sizes[i], CreatedBy: id.SourceUser, CreatedAt: time.Unix(0, 0).UTC()})
	}
	return out, nil
}

func (b bucket) Read(_ context.Context, p id.Project, key string, max int64) (app.ObjectBytes, error) {
	b.f.mu.Lock()
	defer b.f.mu.Unlock()
	file, ok := b.file(p, key)
	if !ok {
		return app.ObjectBytes{}, notFound()
	}
	data := file.Data
	cut := int64(len(data)) > max
	if cut {
		data = data[:max]
	}
	return app.ObjectBytes{Object: object(key, file), Data: slices.Clone(data), Cut: cut}, nil
}

func (b bucket) Delete(_ context.Context, p id.Project, key string) (app.Deleted, error) {
	b.f.mu.Lock()
	defer b.f.mu.Unlock()
	file, ok := b.file(p, key)
	if !ok {
		return app.Deleted{}, notFound()
	}
	delete(b.f.Bucket.Files[p], key)
	b.f.Bucket.changes++
	ch := id.Change(fmt.Sprintf("ch%03d", b.f.Bucket.changes))
	if b.f.Bucket.Deleted == nil {
		b.f.Bucket.Deleted = map[id.Change]deletedFile{}
	}
	b.f.Bucket.Deleted[ch] = deletedFile{p, key, file}
	return app.Deleted{Change: ch}, nil
}

func (b bucket) UndoDelete(_ context.Context, p id.Project, change id.Change) error {
	b.f.mu.Lock()
	defer b.f.mu.Unlock()
	d, ok := b.f.Bucket.Deleted[change]
	if !ok || d.p != p {
		return &app.UIError{Kind: app.KindBusy, Message: "The delete is undone already, or past the undo window."}
	}
	delete(b.f.Bucket.Deleted, change)
	b.f.Bucket.Files[p][d.key] = d.file
	return nil
}

func (b bucket) OpenWithSystem(_ context.Context, p id.Project, key string) error {
	b.f.mu.Lock()
	defer b.f.mu.Unlock()
	file, ok := b.file(p, key)
	if !ok {
		return notFound()
	}
	if !file.Opens {
		return &app.UIError{Kind: app.KindInvalid, Message: "This type of file isn't opened outside the app."}
	}
	b.f.Bucket.Opened = append(b.f.Bucket.Opened, key)
	return nil
}

// status is the Fake as state.StatusAPI.
type status struct{ f *Fake }

func (s status) Memory(context.Context) (sysmem.Reading, error) {
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	return s.f.Bucket.Memory, nil
}

func (s status) Providers(context.Context) ([]app.ProviderStatus, error) {
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	return slices.Clone(s.f.Bucket.Paused), nil
}
