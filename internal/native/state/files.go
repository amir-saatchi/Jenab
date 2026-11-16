package state

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/id"
)

// Keys, folders and links of a project's bucket (SPEC 4.1, 4.5), for the
// Files panel and the chat's jenab:// links: the React app's lib/files.ts.

// Cache is the app's own folder: fetched pages and long tool outputs. The
// panel lists it apart, as *App cache* (SPEC 5.12).
const Cache = "cache/"

// linksRe finds jenab:// links in text. A trailing "." or "/" is left
// out, as the end of a sentence. A project ID is a ULID in upper case.
var linksRe = regexp.MustCompile(`jenab://[0-9A-HJKMNP-TV-Z]{26}/[A-Za-z0-9._/-]*[A-Za-z0-9_-]`)

// ParseLink splits jenab://<project_id>/<key>; ok is false for anything
// else.
func ParseLink(s string) (p id.Project, key string, ok bool) {
	p, key, err := bucket.ParseLink(s)
	return p, key, err == nil
}

// LinkOf is a file's link.
func LinkOf(p id.Project, key string) string { return bucket.Link(p, key) }

// ParentOf is the folder a key is in: "" for the top, else ending in "/".
func ParentOf(key string) string {
	i := strings.LastIndex(strings.TrimSuffix(key, "/"), "/")
	if i < 0 {
		return ""
	}
	return key[:i+1]
}

// NameOf is the last segment of a key or folder.
func NameOf(key string) string {
	k := strings.TrimSuffix(key, "/")
	return k[strings.LastIndex(k, "/")+1:]
}

// Crumb is a step of the Files panel's path bar.
type Crumb struct {
	Label  string
	Prefix string
}

// Crumbs is the path bar for a folder: Files, then App cache when inside
// cache/, then each folder.
func Crumbs(prefix string) []Crumb {
	out := []Crumb{{"Files", ""}}
	at := ""
	for _, seg := range strings.Split(prefix, "/") {
		if seg == "" {
			continue
		}
		at += seg + "/"
		label := seg
		if at == Cache {
			label = "App cache"
		}
		out = append(out, Crumb{label, at})
	}
	return out
}

// ShownFolders leaves cache/ out of the top folder; it has its own entry.
func ShownFolders(prefix string, folders []string) []string {
	if prefix != "" {
		return folders
	}
	out := make([]string, 0, len(folders))
	for _, f := range folders {
		if f != Cache {
			out = append(out, f)
		}
	}
	return out
}

// FileKind is how the file view shows a type.
type FileKind string

const (
	KindMarkdown FileKind = "markdown"
	KindText     FileKind = "text"
	KindImage    FileKind = "image"
	KindAudio    FileKind = "audio"
	KindVideo    FileKind = "video"
	KindOther    FileKind = "other"
)

// KindOf is how the file view shows a type, from the MIME type detected
// when the file was stored (SPEC 4.7). HTML, XML and scripts are text,
// shown as text and never run. An SVG is a picture: the kit draws it with
// MyGo's SVG renderer, which runs nothing in it.
func KindOf(mime string) FileKind {
	base, _, _ := strings.Cut(mime, ";")
	base = strings.ToLower(strings.TrimSpace(base))
	switch {
	case base == "text/markdown":
		return KindMarkdown
	case base == "image/svg+xml":
		return KindImage
	case strings.HasPrefix(base, "image/"):
		if shownImages[base] {
			return KindImage
		}
		return KindOther
	case strings.HasPrefix(base, "audio/") || base == "application/ogg":
		return KindAudio
	case strings.HasPrefix(base, "video/"):
		return KindVideo
	case strings.HasPrefix(base, "text/") || base == "application/json" || base == "application/xml" || strings.HasSuffix(base, "+xml"):
		return KindText
	}
	return KindOther
}

var shownImages = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true, "image/avif": true,
	"image/bmp": true, "image/x-icon": true, "image/vnd.microsoft.icon": true,
}

// TypeName is a MIME type for people: its base, without parameters.
func TypeName(mime string) string {
	base, _, _ := strings.Cut(mime, ";")
	if base = strings.TrimSpace(base); base != "" {
		return base
	}
	return "unknown"
}

// FormatSize is a size in bytes for people: 900 B, 1.5 KB, 12 MB.
func FormatSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB"}
	v := float64(n) / 1024
	u := 0
	for v >= 1024 && u < len(units)-1 {
		v /= 1024
		u++
	}
	if v < 10 {
		return fmt.Sprintf("%.1f %s", v, units[u])
	}
	return fmt.Sprintf("%d %s", int64(math.Round(v)), units[u])
}

// WhoText is a created_by source (SPEC 2.5) as the panel says it.
func WhoText(source id.Source) string {
	kind, _, _ := strings.Cut(string(source), ":")
	switch kind {
	case "user":
		return "you"
	case "message", "auto":
		return "the agent"
	case "run":
		return "a pipeline"
	case "app":
		return "the app"
	case "":
		return "unknown"
	}
	return kind
}

// Piece is a run of text, or a jenab:// link in it.
type Piece struct {
	Text string
	Link bool
}

// SplitLinks cuts text at its jenab:// links.
func SplitLinks(text string) []Piece {
	var out []Piece
	at := 0
	for _, m := range linksRe.FindAllStringIndex(text, -1) {
		if _, _, ok := ParseLink(text[m[0]:m[1]]); !ok {
			continue
		}
		if m[0] > at {
			out = append(out, Piece{Text: text[at:m[0]]})
		}
		out = append(out, Piece{Text: text[m[0]:m[1]], Link: true})
		at = m[1]
	}
	if at < len(text) {
		out = append(out, Piece{Text: text[at:]})
	}
	return out
}

// BucketAPI is the bucket as the Files panel and the file view use it;
// app.NativeBucket has it.
type BucketAPI interface {
	List(ctx context.Context, p id.Project, prefix, after string) (app.ObjectPage, error)
	Head(ctx context.Context, p id.Project, key string) (app.Object, error)
	Versions(ctx context.Context, p id.Project, key string) ([]app.ObjectVersion, error)
	Read(ctx context.Context, p id.Project, key string, max int64) (app.ObjectBytes, error)
	Delete(ctx context.Context, p id.Project, key string) (app.Deleted, error)
	UndoDelete(ctx context.Context, p id.Project, change id.Change) error
	OpenWithSystem(ctx context.Context, p id.Project, key string) error
}

var _ BucketAPI = app.NativeBucket{}

// Folder is what the panel shows of a folder: the pages loaded so far.
type Folder struct {
	Folders []string
	Objects []app.Object
	Next    string // "" when all is loaded
}

// ListFunc lists a page of a folder, as BucketAPI.List.
type ListFunc func(ctx context.Context, p id.Project, prefix, after string) (app.ObjectPage, error)

// ListFolder loads a folder's first page, or with after the next one, and
// adds it to shown, which may be nil. The top folder leaves cache/ out
// (SPEC 5.12).
func ListFolder(ctx context.Context, list ListFunc, p id.Project, prefix string, shown *Folder, after string) (Folder, error) {
	pg, err := list(ctx, p, prefix, after)
	if err != nil {
		return Folder{}, err
	}
	var f Folder
	if shown != nil {
		f.Folders = append(f.Folders, shown.Folders...)
		f.Objects = append(f.Objects, shown.Objects...)
	}
	f.Folders = append(f.Folders, ShownFolders(prefix, pg.Folders)...)
	f.Objects = append(f.Objects, pg.Objects...)
	f.Next = pg.Next
	return f, nil
}

// TextLimit is how much of a text file the view shows; the rest opens
// with the system app.
const TextLimit = 1 << 20

// ImageLimit is the largest picture the view shows; a larger one shows
// its name, type and size, as other types do.
const ImageLimit = 32 << 20

// FolderView is the Files panel's folder in a project.
type FolderView struct {
	Prefix string
	Folder *Folder // nil while the first page loads
	Err    string
	// HasCache tells that cache/ holds something, so the top folder shows
	// *App cache*.
	HasCache bool
	Loading  bool // a page after the first is loading

	rev int
	seq int
}

// FileRef names a file of a project.
type FileRef struct {
	Project id.Project
	Key     string
}

// FileView is a file opened in the main area.
type FileView struct {
	Object   *app.Object // nil while it loads, and after a failure
	NotFound bool
	Err      string
	// Data is the start of the file, for text and pictures: TextLimit
	// bytes of text, a whole picture up to ImageLimit. Loaded tells it is
	// read; ReadErr says why it couldn't be.
	Data     []byte
	Cut      bool
	Loaded   bool
	ReadErr  string
	Versions []app.ObjectVersion

	rev int
	seq int
}

// Files is the bucket's store: the Files panel's folders and the files
// open in the main area, read through the services. Where the panel is,
// and the count of changes made from the app, are the Shell's.
type Files struct {
	Folders map[id.Project]*FolderView
	Views   map[FileRef]*FileView

	api  BucketAPI
	post Post
	seq  int
}

// NewFiles makes the store; api may be nil, as in screens' tests that
// show no files.
func NewFiles(api BucketAPI, post Post) *Files {
	return &Files{Folders: map[id.Project]*FolderView{}, Views: map[FileRef]*FileView{}, api: api, post: post}
}

func (f *Files) next() int { f.seq++; return f.seq }

// Folder is the panel's folder prefix in project p as of rev, the Shell's
// count of changes: it lists the folder when it isn't listed yet, or the
// prefix or rev moved on.
func (f *Files) Folder(p id.Project, prefix string, rev int) *FolderView {
	v := f.Folders[p]
	if v != nil && v.Prefix == prefix && v.rev == rev {
		return v
	}
	keep := v != nil && v.Prefix == prefix // the same folder again: what it shows stays until the list is in
	nv := &FolderView{Prefix: prefix, rev: rev, seq: f.next()}
	if keep {
		nv.Folder, nv.HasCache = v.Folder, v.HasCache
	}
	f.Folders[p] = nv
	if f.api == nil {
		nv.Err = "Files can't be read here."
		return nv
	}
	seq := nv.seq
	go func() {
		fo, err := ListFolder(background, f.api.List, p, prefix, nil, "")
		hasCache := false
		if prefix == "" && err == nil {
			if pg, cerr := f.api.List(background, p, Cache, ""); cerr == nil {
				hasCache = len(pg.Folders)+len(pg.Objects) > 0
			}
		}
		f.post(func() {
			v := f.Folders[p]
			if v == nil || v.seq != seq {
				return
			}
			if err != nil {
				v.Err, v.Folder = errorText(err), nil
				return
			}
			v.Err, v.Folder, v.HasCache = "", &fo, hasCache
		})
	}()
	return nv
}

// More loads the next page of the panel's folder in p, once.
func (f *Files) More(p id.Project) {
	v := f.Folders[p]
	if v == nil || v.Folder == nil || v.Folder.Next == "" || v.Loading || f.api == nil {
		return
	}
	v.Loading = true
	shown, seq, prefix := *v.Folder, v.seq, v.Prefix
	go func() {
		fo, err := ListFolder(background, f.api.List, p, prefix, &shown, shown.Next)
		f.post(func() {
			v := f.Folders[p]
			if v == nil || v.seq != seq {
				return
			}
			v.Loading = false
			if err != nil {
				v.Err = errorText(err)
				return
			}
			v.Folder = &fo
		})
	}()
}

// Reload lists the panel's folder in p again.
func (f *Files) Reload(p id.Project) {
	if v := f.Folders[p]; v != nil {
		v.rev = -1
	}
}

// View is file key of project p as of rev, the Shell's count of changes:
// its head, its versions, and for text and pictures its bytes, read
// again when rev moves on.
func (f *Files) View(p id.Project, key string, rev int) *FileView {
	ref := FileRef{p, key}
	v := f.Views[ref]
	if v != nil && v.rev == rev {
		return v
	}
	nv := &FileView{rev: rev, seq: f.next()}
	if v != nil { // shown until the new reads are in
		nv.Object, nv.Data, nv.Cut, nv.Loaded, nv.Versions = v.Object, v.Data, v.Cut, v.Loaded, v.Versions
	}
	f.Views[ref] = nv
	if f.api == nil {
		nv.Err = "Files can't be read here."
		return nv
	}
	seq := nv.seq
	current := func() *FileView {
		if v := f.Views[ref]; v != nil && v.seq == seq {
			return v
		}
		return nil
	}
	go func() {
		o, err := f.api.Head(background, p, key)
		if err != nil {
			f.post(func() {
				if v := current(); v != nil {
					v.Object, v.Data, v.Loaded, v.Versions = nil, nil, false, nil
					v.NotFound = isNotFound(err)
					v.Err = errorText(err)
				}
			})
			return
		}
		f.post(func() {
			if v := current(); v != nil {
				v.Object, v.NotFound, v.Err = &o, false, ""
			}
		})
		vs, verr := f.api.Versions(background, p, key)
		f.post(func() {
			if v := current(); v != nil {
				if verr != nil {
					vs = nil
				}
				v.Versions = vs
			}
		})
		max := int64(0)
		switch KindOf(o.MIME) {
		case KindText, KindMarkdown:
			max = TextLimit
		case KindImage:
			if o.Size <= ImageLimit {
				max = ImageLimit
			}
		}
		if max == 0 {
			f.post(func() {
				if v := current(); v != nil {
					v.Data, v.Cut, v.Loaded = nil, false, true
				}
			})
			return
		}
		ob, rerr := f.api.Read(background, p, key, max)
		f.post(func() {
			v := current()
			if v == nil {
				return
			}
			v.Loaded = true
			if rerr != nil {
				v.Data, v.ReadErr = nil, errorText(rerr)
				if isNotFound(rerr) {
					v.ReadErr = "The file no longer exists."
				}
				return
			}
			v.Data, v.Cut, v.ReadErr = ob.Data, ob.Cut, ""
		})
	}()
	return nv
}

// isNotFound tells a service's not_found error.
func isNotFound(err error) bool {
	ue, ok := errors.AsType[*app.UIError](err)
	return ok && ue.Kind == app.KindNotFound
}
