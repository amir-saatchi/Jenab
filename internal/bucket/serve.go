package bucket

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
)

// Served is an object opened for the handler.
type Served struct {
	MIME    string // as detected when it was stored
	Hash    string
	ModTime time.Time
	Body    io.ReadSeekCloser
}

// Opener opens a project's object for the handler. It returns ErrNotFound
// when there is no such project or key.
type Opener func(ctx context.Context, p id.Project, key string) (Served, error)

// Route is where the frontend loads objects: /objects/<project_id>/<key>
// (SPEC 4.5).
const Route = "/objects/"

// The policy for anything shown as a page: nothing loads or runs, and the
// page gets an opaque origin. SVG through <img> never runs scripts anyway.
const sandboxCSP = "default-src 'none'; img-src data:; style-src 'unsafe-inline'; sandbox"

// Handler serves objects with the rules for untrusted files (SPEC 4.7):
//   - the type comes from the stored MIME, with nosniff;
//   - HTML, XML and other text that a browser could run are sent as
//     text/plain, so they are shown, never rendered;
//   - SVG is sent as an image only when the browser loads it for <img>;
//     opened any other way it is text;
//   - types that are not shown in the app are downloads, never opened.
//
// It is mounted on the Wails asset server in P1-13.
func Handler(open Opener) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			h.Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rest, ok := strings.CutPrefix(r.URL.Path, Route)
		pid, key, _ := strings.Cut(rest, "/")
		if !ok || !id.Valid(pid) || CheckKey(key) != nil {
			http.NotFound(w, r)
			return
		}
		obj, err := open(r.Context(), id.Project(pid), key)
		if errors.Is(err, ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "the object could not be read", http.StatusInternalServerError)
			return
		}
		defer obj.Body.Close()
		ctype, download, sandbox := servedType(obj.MIME, r.Header.Get("Sec-Fetch-Dest"))
		h.Set("Content-Type", ctype)
		if sandbox {
			h.Set("Content-Security-Policy", sandboxCSP)
		}
		if download {
			h.Set("Content-Disposition", "attachment")
		}
		h.Set("Cache-Control", "no-cache") // a key can get new content; the ETag makes the check cheap
		h.Set("ETag", `"`+obj.Hash+`"`)
		http.ServeContent(w, r, "", obj.ModTime, obj.Body)
	})
}

// servedType decides how a stored type is sent. dest is the request's
// Sec-Fetch-Dest; "" when the browser doesn't send it.
func servedType(stored, dest string) (ctype string, download, sandbox bool) {
	const asText = "text/plain; charset=utf-8"
	base, params, err := mime.ParseMediaType(stored)
	if err != nil {
		return "application/octet-stream", true, true
	}
	switch {
	case base == "image/svg+xml":
		if dest != "" && dest != "image" {
			return asText, false, true
		}
		return base, false, true
	case shownImages[base], strings.HasPrefix(base, "audio/"), strings.HasPrefix(base, "video/"):
		return base, false, true
	case base == "application/pdf":
		return base, false, false // the built-in viewer doesn't run in a sandbox
	case shownText[base]:
		if cs := params["charset"]; cs != "" {
			return base + "; charset=" + cs, false, true
		}
		return base, false, true
	case strings.HasPrefix(base, "text/"), strings.HasSuffix(base, "+xml"), base == "application/xml", base == "application/xhtml+xml", base == "application/javascript":
		return asText, false, true // HTML, XML and scripts: shown as text
	}
	return "application/octet-stream", true, true
}

var shownImages = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
	"image/avif": true, "image/bmp": true, "image/x-icon": true, "image/vnd.microsoft.icon": true,
}

var shownText = map[string]bool{
	"text/plain": true, "text/csv": true, "text/tab-separated-values": true,
	"text/markdown": true, "application/json": true,
}
