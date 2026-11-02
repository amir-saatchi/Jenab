package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/docx"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// UploadRoute is where the composer's attach button sends a file:
// POST /upload/<project_id> with the file as the body and its name,
// percent-encoded, in UploadNameHeader (TASK-021). A file is stored under
// uploads/<name>, never over another: a taken key gets a number.
//
// A binding would carry 100 MB as JSON, so the body comes as is. The
// header is the guard against a page that is not the app's: a form can't
// set it, and a fetch from another origin, or from a sandboxed object,
// can't send it without a preflight, which this route refuses.
const UploadRoute = "/upload/"

// UploadNameHeader holds the file's original name, percent-encoded.
const UploadNameHeader = "X-Jenab-Upload-Name"

// Uploaded is what the route answers for a stored file.
type Uploaded struct {
	Key  string `json:"key"`
	Link string `json:"link"` // jenab://<project>/<key>, which the message carries
	MIME string `json:"mime"`
	Size int64  `json:"size"`
	Name string `json:"name"` // as it was on the user's computer
	// Note says why a document can't be read, as in "it is
	// password-protected"; it is stored anyway.
	Note string `json:"note,omitempty"`
}

// maxUploadName is the longest name part of an upload's key, in bytes.
const maxUploadName = 120

type uploads struct {
	projects *project.Manager
	mu       sync.Mutex
	taken    map[string]bool // keys being uploaded now, by "<project>/<key>"
}

func uploadHandler(pm *project.Manager) http.Handler {
	return &uploads{projects: pm, taken: map[string]bool{}}
}

func (u *uploads) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		uploadError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	enc := r.Header.Get(UploadNameHeader)
	if enc == "" || (r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin") {
		uploadError(w, http.StatusForbidden, "uploads come from the app only")
		return
	}
	name, err := url.PathUnescape(enc)
	if err != nil {
		uploadError(w, http.StatusBadRequest, "the file name is not percent-encoded")
		return
	}
	// Wails mounts a route's handler with the route cut off the path.
	pid := strings.Trim(strings.TrimPrefix(r.URL.Path, UploadRoute), "/")
	if !id.Valid(pid) {
		uploadError(w, http.StatusNotFound, "no such project")
		return
	}
	if r.ContentLength > store.MaxUpload {
		uploadError(w, http.StatusRequestEntityTooLarge, tooLarge())
		return
	}
	proj, err := u.projects.Open(r.Context(), id.Project(pid))
	if errors.Is(err, store.ErrNotFound) {
		uploadError(w, http.StatusNotFound, "no such project")
		return
	}
	if err != nil {
		uploadError(w, http.StatusInternalServerError, "the project could not be opened")
		return
	}
	defer proj.Release()
	up, err := u.store(r.Context(), proj, name, r)
	switch {
	case errors.Is(err, bucket.ErrTooLarge):
		uploadError(w, http.StatusRequestEntityTooLarge, tooLarge())
		return
	case errors.Is(err, store.ErrReadOnly):
		uploadError(w, http.StatusConflict, "the project is open read-only, so files can't be added")
		return
	case err != nil:
		uploadError(w, http.StatusInternalServerError, "the file could not be stored")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(up)
}

// store stores the body under a free key made from name, then makes a
// document's text, so the agent reads it at once.
func (u *uploads) store(ctx context.Context, proj *project.Project, name string, r *http.Request) (Uploaded, error) {
	key, err := u.reserve(ctx, proj, uploadKey(name))
	if err != nil {
		return Uploaded{}, err
	}
	defer u.release(proj.ID, key)
	meta, _ := json.Marshal(map[string]string{"name": name})
	o, err := proj.DB.PutObject(ctx, limit.Interactive, id.SourceUser, key, r.Body, store.PutOptions{Source: "upload", Metadata: meta})
	if err != nil {
		return Uploaded{}, err
	}
	up := Uploaded{Key: o.Key, Link: bucket.Link(proj.ID, o.Key), MIME: o.MIME, Size: o.Size, Name: name}
	if docx.IsWord(o.MIME) || strings.HasPrefix(o.MIME, docx.MIMEOLE) {
		up.Note = documentNote(ctx, proj, o)
	}
	return up, nil
}

// documentNote makes the text of document o and says why it can't be
// read, or "".
func documentNote(ctx context.Context, proj *project.Project, o store.Object) string {
	_, body, err := proj.DB.OpenObject(ctx, o.Key)
	if err != nil {
		return ""
	}
	_, text, err := tool.DocumentText(ctx, proj.DB, limit.Interactive, id.SourceUser, o, body)
	body.Close()
	if err == nil {
		text.Close()
		return ""
	}
	var de *docx.Error
	if errors.As(err, &de) {
		return de.Msg
	}
	return "" // not the file's fault: the text is made again when it is read
}

// reserve picks key, or key with a number before its extension, that no
// object and no upload in progress has.
func (u *uploads) reserve(ctx context.Context, proj *project.Project, key string) (string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	stem, ext := splitExt(key)
	for n := 1; n <= 1000; n++ {
		k := key
		if n > 1 {
			k = stem + "-" + strconv.Itoa(n) + ext
		}
		if u.taken[string(proj.ID)+"/"+k] {
			continue
		}
		_, err := proj.DB.HeadObject(ctx, k)
		if errors.Is(err, store.ErrNotFound) {
			u.taken[string(proj.ID)+"/"+k] = true
			return k, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("upload: no free key for %s", key)
}

func (u *uploads) release(p id.Project, key string) {
	u.mu.Lock()
	delete(u.taken, string(p)+"/"+key)
	u.mu.Unlock()
}

// uploadKey is uploads/<name> with name made to follow the key rules
// (SPEC 4.1): each run of other characters becomes "-", and a name with
// nothing left is "file". There is room left for a number.
func uploadKey(name string) string {
	name = name[strings.LastIndexAny(name, `/\`)+1:]
	stem, ext := splitExt(name)
	if e := strings.TrimPrefix(ext, "."); len(e) > 15 || keyPart(e) != e {
		stem, ext = name, "" // not an extension
	}
	stem = keyPart(stem)
	if len(stem) > maxUploadName-len(ext) {
		stem = strings.TrimRight(stem[:maxUploadName-len(ext)], "-.")
	}
	if stem == "" {
		stem = "file"
	}
	return "uploads/" + stem + ext
}

// keyPart keeps the characters a key may have and turns each run of
// others into one "-"; dots and dashes are trimmed from the ends, so the
// part is never "." or "..".
func keyPart(s string) string {
	var b strings.Builder
	dash := false
	for _, c := range s {
		if c < 0x80 && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			b.WriteRune(c)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-.")
}

// splitExt splits name before its extension, which is "" for a name
// such as ".env" or "a.".
func splitExt(name string) (stem, ext string) {
	ext = path.Ext(name)
	if ext == "." || len(ext) == len(name) {
		return name, ""
	}
	return name[:len(name)-len(ext)], ext
}

func tooLarge() string {
	return fmt.Sprintf("the file is larger than %d MB", store.MaxUpload>>20)
}

func uploadError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
