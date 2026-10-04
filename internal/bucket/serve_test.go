package bucket

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
)

type nopCloser struct{ *strings.Reader }

func (nopCloser) Close() error { return nil }

// fakeOpener serves objects by key from a map of key → stored MIME.
func fakeOpener(pid id.Project, objs map[string]string) Opener {
	return func(_ context.Context, p id.Project, key string) (Served, error) {
		m, ok := objs[key]
		if p != pid || !ok {
			return Served{}, ErrNotFound
		}
		return Served{MIME: m, Hash: "h-" + key, ModTime: time.Unix(1_800_000_000, 0), Body: nopCloser{strings.NewReader("body of " + key)}}, nil
	}
}

func serve(t *testing.T, h http.Handler, method, path, dest string, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	if dest != "" {
		r.Header.Set("Sec-Fetch-Dest", dest)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestServeTypes(t *testing.T) {
	pid := id.Project(id.New())
	objs := map[string]string{
		"page.html":  "text/html; charset=utf-8",
		"x.xhtml":    "application/xhtml+xml",
		"x.xml":      "text/xml; charset=utf-8",
		"x.js":       "application/javascript",
		"feed.rss":   "application/rss+xml",
		"pic.svg":    "image/svg+xml",
		"pic.png":    "image/png",
		"song.mp3":   "audio/mpeg",
		"doc.pdf":    "application/pdf",
		"data.csv":   "text/csv; charset=utf-8",
		"data.json":  "application/json; charset=utf-8",
		"notes.txt":  "text/plain; charset=utf-8",
		"tool.exe":   "application/octet-stream",
		"arch.zip":   "application/zip",
		"broken.bin": "not a type",
	}
	h := Handler(fakeOpener(pid, objs))
	const text = "text/plain; charset=utf-8"
	cases := []struct {
		key, dest, ctype  string
		download, sandbox bool
	}{
		{"page.html", "document", text, false, true},
		{"page.html", "iframe", text, false, true},
		{"page.html", "", text, false, true},
		{"x.xhtml", "document", text, false, true},
		{"x.xml", "document", text, false, true},
		{"x.js", "script", text, false, true},
		{"feed.rss", "document", text, false, true},
		{"pic.svg", "image", "image/svg+xml", false, true},
		{"pic.svg", "", "image/svg+xml", false, true},
		{"pic.svg", "document", text, false, true},
		{"pic.svg", "iframe", text, false, true},
		{"pic.svg", "embed", text, false, true},
		{"pic.png", "image", "image/png", false, true},
		{"song.mp3", "audio", "audio/mpeg", false, true},
		{"doc.pdf", "document", "application/pdf", false, false},
		{"data.csv", "", "text/csv; charset=utf-8", false, true},
		{"data.json", "", "application/json; charset=utf-8", false, true},
		{"notes.txt", "document", text, false, true},
		{"tool.exe", "document", "application/octet-stream", true, true},
		{"arch.zip", "", "application/octet-stream", true, true},
		{"broken.bin", "", "application/octet-stream", true, true},
	}
	for _, c := range cases {
		w := serve(t, h, "GET", Route+string(pid)+"/"+c.key, c.dest)
		if w.Code != 200 {
			t.Errorf("%s (%s): status %d", c.key, c.dest, w.Code)
			continue
		}
		hd := w.Header()
		if got := hd.Get("Content-Type"); got != c.ctype {
			t.Errorf("%s (%s): Content-Type %q, want %q", c.key, c.dest, got, c.ctype)
		}
		if hd.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: no nosniff", c.key)
		}
		if got := hd.Get("Content-Disposition") == "attachment"; got != c.download {
			t.Errorf("%s: download %v, want %v", c.key, got, c.download)
		}
		csp := hd.Get("Content-Security-Policy")
		if got := strings.Contains(csp, "sandbox") && strings.Contains(csp, "default-src 'none'"); got != c.sandbox {
			t.Errorf("%s: CSP %q, sandbox want %v", c.key, csp, c.sandbox)
		}
		if body := w.Body.String(); body != "body of "+c.key {
			t.Errorf("%s: body %q", c.key, body)
		}
	}
}

func TestServeRefuses(t *testing.T) {
	pid := id.Project(id.New())
	h := Handler(fakeOpener(pid, map[string]string{"a.txt": "text/plain"}))
	other := id.Project(id.New())
	for _, path := range []string{
		Route + string(other) + "/a.txt", // another project
		Route + string(pid) + "/b.txt",   // missing key
		Route + string(pid) + "/../a.txt",
		Route + string(pid) + "/",
		Route + "not-an-id/a.txt",
		"/other/" + string(pid) + "/a.txt",
	} {
		if w := serve(t, h, "GET", path, ""); w.Code != http.StatusNotFound {
			t.Errorf("GET %s: %d, want 404", path, w.Code)
		}
	}
	// The handler checks the project ID itself, whatever the opener does.
	loose := Handler(func(context.Context, id.Project, string) (Served, error) {
		return Served{MIME: "text/plain", Body: nopCloser{strings.NewReader("x")}}, nil
	})
	if w := serve(t, loose, "GET", Route+"not-an-id/a.txt", ""); w.Code != http.StatusNotFound {
		t.Errorf("a bad project ID: %d, want 404", w.Code)
	}
	if w := serve(t, h, "POST", Route+string(pid)+"/a.txt", ""); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d, want 405", w.Code)
	}
	failing := Handler(func(context.Context, id.Project, string) (Served, error) { return Served{}, io.ErrUnexpectedEOF })
	w := serve(t, failing, "GET", Route+string(pid)+"/a.txt", "")
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "EOF") {
		t.Errorf("failing opener: %d %q", w.Code, w.Body.String())
	}
}

func TestServeCachingAndRanges(t *testing.T) {
	pid := id.Project(id.New())
	h := Handler(fakeOpener(pid, map[string]string{"a.txt": "text/plain"}))
	path := Route + string(pid) + "/a.txt"
	w := serve(t, h, "GET", path, "")
	if w.Header().Get("ETag") != `"h-a.txt"` {
		t.Fatalf("ETag %q", w.Header().Get("ETag"))
	}
	if w := serve(t, h, "GET", path, "", "If-None-Match", `"h-a.txt"`); w.Code != http.StatusNotModified {
		t.Errorf("If-None-Match: %d, want 304", w.Code)
	}
	if w := serve(t, h, "GET", path, "", "Range", "bytes=0-3"); w.Code != http.StatusPartialContent || w.Body.String() != "body" {
		t.Errorf("Range: %d %q", w.Code, w.Body.String())
	}
	if w := serve(t, h, "HEAD", path, ""); w.Code != 200 || w.Body.Len() != 0 {
		t.Errorf("HEAD: %d, %d bytes", w.Code, w.Body.Len())
	}
}
