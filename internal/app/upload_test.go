package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/docx"
	"github.com/amir-saatchi/jenab/internal/docx/docxtest"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

func TestUploadKey(t *testing.T) {
	for name, want := range map[string]string{
		"report.docx":                     "uploads/report.docx",
		"Q3 Report (final).DOCX":          "uploads/Q3-Report-final.DOCX",
		"نامه.docx":                       "uploads/file.docx",
		"قرارداد ۲ - Vertrag Müller.pdf":  "uploads/Vertrag-M-ller.pdf",
		`C:\Users\x\notes.txt`:            "uploads/notes.txt",
		"../../etc/passwd":                "uploads/passwd",
		"..":                              "uploads/file",
		".env":                            "uploads/env",
		"archive.tar.gz":                  "uploads/archive.tar.gz",
		"a.b c":                           "uploads/a.b-c",
		"no extension":                    "uploads/no-extension",
		"":                                "uploads/file",
		"x." + strings.Repeat("y", 20):    "uploads/x." + strings.Repeat("y", 20),
		strings.Repeat("ab", 200) + ".md": "uploads/" + strings.Repeat("ab", 200)[:maxUploadName-3] + ".md",
	} {
		got := uploadKey(name)
		if got != want {
			t.Errorf("uploadKey(%q) = %q, want %q", name, got, want)
		}
		if err := bucket.CheckKey(got); err != nil {
			t.Errorf("uploadKey(%q) = %q: %v", name, got, err)
		}
	}
}

func upload(t *testing.T, h http.Handler, p id.Project, name string, body []byte, edit func(*http.Request)) (*httptest.ResponseRecorder, Uploaded) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/"+string(p), bytes.NewReader(body))
	r.Header.Set(UploadNameHeader, url.PathEscape(name))
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if edit != nil {
		edit(r)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	var up Uploaded
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &up); err != nil {
			t.Fatal(err)
		}
	}
	return rec, up
}

func TestUpload(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	op, err := e.svc.Project.Create(ctx, "Uploads")
	if err != nil {
		t.Fatal(err)
	}
	h := uploadHandler(e.pm)

	// A Word document: stored with its name, its text made at once.
	doc := docxtest.Doc("# نامه", "سلام، این یک می‌خواهم است.")
	rec, up := upload(t, h, op.ID, "نامهٔ اداری.docx", doc, nil)
	if rec.Code != http.StatusOK || up.Key != "uploads/file.docx" || up.Link != bucket.Link(op.ID, "uploads/file.docx") ||
		up.MIME != docx.MIME || up.Size != int64(len(doc)) || up.Name != "نامهٔ اداری.docx" || up.Note != "" {
		t.Fatalf("docx: %d %s %+v", rec.Code, rec.Body, up)
	}
	proj, err := e.pm.Open(ctx, op.ID)
	if err != nil {
		t.Fatal(err)
	}
	o, err := proj.DB.HeadObject(ctx, "uploads/file.docx")
	if err != nil || o.Source != "upload" || o.CreatedBy != id.SourceUser || metaTitle(o.Metadata) != "نامهٔ اداری.docx" {
		t.Errorf("object %+v, %v", o, err)
	}
	if _, err := proj.DB.HeadObject(ctx, tool.TextKey(o.SHA256)); err != nil {
		t.Errorf("no text: %v", err)
	}
	proj.Release()

	// A taken key gets a number; the first file is kept.
	for i, want := range []string{"uploads/file-2.docx", "uploads/file-3.docx"} {
		if _, up := upload(t, h, op.ID, "سند.docx", doc, nil); up.Key != want {
			t.Errorf("upload %d: %q, want %q", i+2, up.Key, want)
		}
	}
	// So do uploads of one name at the same time.
	var wg sync.WaitGroup
	keys := make(chan string, 4)
	for range 4 {
		wg.Go(func() {
			_, up := upload(t, h, op.ID, "same.txt", []byte("same"), nil)
			keys <- up.Key
		})
	}
	wg.Wait()
	close(keys)
	seen := map[string]bool{}
	for k := range keys {
		seen[k] = true
	}
	if len(seen) != 4 || !seen["uploads/same.txt"] || !seen["uploads/same-4.txt"] {
		t.Errorf("same name at once: %v", seen)
	}

	// A file that can't be read is stored, with a note that says why.
	_, up = upload(t, h, op.ID, "secret.docx", docxtest.OLE(true, 4096), nil)
	if up.Key != "uploads/secret.docx" || up.Note != "it is password-protected" {
		t.Errorf("encrypted: %+v", up)
	}
	_, up = upload(t, h, op.ID, "old.doc", docxtest.OLE(false, 4096), nil)
	if up.Key != "uploads/old.doc" || !strings.HasPrefix(up.Note, "it is an old Word") {
		t.Errorf("old: %+v", up)
	}
	_, up = upload(t, h, op.ID, "photo.png", []byte("\x89PNG\r\n\x1a\n"), nil)
	if up.Key != "uploads/photo.png" || up.MIME != "image/png" || up.Note != "" {
		t.Errorf("png: %+v", up)
	}

	// Refused: not a POST, no name header, another site, too large, no project.
	for name, c := range map[string]struct {
		edit func(*http.Request)
		code int
	}{
		"GET":       {func(r *http.Request) { r.Method = http.MethodGet }, http.StatusMethodNotAllowed},
		"no name":   {func(r *http.Request) { r.Header.Del(UploadNameHeader) }, http.StatusForbidden},
		"cross":     {func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }, http.StatusForbidden},
		"bad name":  {func(r *http.Request) { r.Header.Set(UploadNameHeader, "%zz") }, http.StatusBadRequest},
		"too large": {func(r *http.Request) { r.ContentLength = store.MaxUpload + 1 }, http.StatusRequestEntityTooLarge},
		"project":   {func(r *http.Request) { r.URL.Path = "/" + id.New() }, http.StatusNotFound},
		"not an id": {func(r *http.Request) { r.URL.Path = "/x" }, http.StatusNotFound},
	} {
		rec, _ := upload(t, h, op.ID, "refused.txt", []byte("x"), c.edit)
		var body struct{ Error string }
		json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != c.code || body.Error == "" {
			t.Errorf("%s: %d %s, want %d", name, rec.Code, rec.Body, c.code)
		}
	}
	// A body larger than it said is stopped by the store.
	rec, _ = upload(t, h, op.ID, "big.bin", nil, func(r *http.Request) {
		r.Body = io.NopCloser(io.LimitReader(zeros{}, store.MaxUpload+1))
		r.ContentLength = -1
	})
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("big body: %d %s", rec.Code, rec.Body)
	}
	proj, _ = e.pm.Open(ctx, op.ID)
	if _, err := proj.DB.HeadObject(ctx, "uploads/refused.txt"); err == nil {
		t.Error("a refused upload was stored")
	}
	if _, err := proj.DB.HeadObject(ctx, "uploads/big.bin"); err == nil {
		t.Error("a file over the limit was stored")
	}
	proj.Release()
	if e.pm.Busy() {
		t.Error("the upload route kept a lease")
	}
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
