package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/store"
)

// The allow-list, on the types the detector finds for real file starts.
func TestSystemExt(t *testing.T) {
	png := "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"
	pdf := "%PDF-1.7\n%âãÏÓ\n"
	mp3 := "ID3\x03\x00\x00\x00\x00\x00\x00"
	zip := "PK\x03\x04\x14\x00\x06\x00"
	pe := "MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xff\xff\x00\x00"
	ole := "\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1\x00\x00" // .msi, .doc
	lnk := "L\x00\x00\x00\x01\x14\x02\x00\x00\x00\x00\x00\xc0\x00\x00\x00\x00\x00\x00F"
	gz := "\x1f\x8b\x08\x00\x00\x00\x00\x00"
	for _, c := range []struct{ key, head, want string }{
		{"pics/a.png", png, ".png"},
		{"docs/report.pdf", pdf, ".pdf"},
		{"music/song.mp3", mp3, ".mp3"},
		{"notes/a.txt", "hello", ".txt"},
		{"notes/a.md", "# Hello", ".md"},
		{"data/a.csv", "a,b\n1,2\n", ".csv"},
		{"docs/letter.docx", zip, ".docx"},
		{"docs/sheet.XLSX", zip, ".xlsx"},
		{"docs/macro.docm", zip, ""},
		{"docs/macro.xlsm", zip, ""},
		{"docs/archive.zip", zip, ""},
		{"docs/archive.tar.gz", gz, ""},
		{"bin/setup.exe", pe, ""},
		{"bin/lib.dll", pe, ""},
		{"bin/setup.msi", ole, ""},
		{"docs/old.doc", ole, ""},
		{"links/app.lnk", lnk, ""},
		{"evil.pdf", pe, ""},           // the extension lies: a program
		{"run.txt.exe", pe, ""},        // a program behind a text name
		{"photo.png.exe", png, ".png"}, // the bytes decide: a picture opens as one
		{"scripts/a.bat", "@echo off\r\ndel *\r\n", ".txt"},
		{"scripts/a.ps1", "Remove-Item -Recurse C:\\", ".txt"},
		{"scripts/a.js", "WScript.Echo(1)", ".txt"},
		{"pages/a.hta", "<html><script>x()</script></html>", ".txt"},
		{"pages/a.html", "<!DOCTYPE html><html><body>hi", ".txt"},
		{"pics/a.svg", `<svg xmlns="http://www.w3.org/2000/svg"><script>x()</script></svg>`, ".txt"},
		{"data/a.xml", `<?xml version="1.0"?><a/>`, ".txt"},
	} {
		mt := bucket.DetectMIME([]byte(c.head), c.key)
		if got := systemExt(mt, c.key); got != c.want {
			t.Errorf("%s (%s): %q, want %q", c.key, mt, got, c.want)
		}
	}
	if got := systemExt("not a type", "a.png"); got != "" {
		t.Errorf("a broken type: %q", got)
	}
}

func TestCopyName(t *testing.T) {
	for key, want := range map[string]string{
		"docs/report.pdf":      "report.txt",
		"run.txt.exe":          "run.txt.txt",
		"a/b/CON.txt":          "_CON.txt",
		"a/nul.tar.gz":         "_nul.tar.txt",
		"a/.hidden":            "file.txt",
		"cache/tool/x/m-1":     "m-1.txt",
		"cache/pages/h/9d.txt": "9d.txt",
	} {
		if got := copyName(key, ".txt"); got != want {
			t.Errorf("copyName(%q) = %q, want %q", key, got, want)
		}
	}
	if got := copyName("docs/report.pdf", ".pdf"); got != "report.pdf" {
		t.Errorf("copyName = %q", got)
	}
}

func TestMetaTitle(t *testing.T) {
	for m, want := range map[string]string{
		``:   "",
		`{}`: "",
		`{"title":"گزارش هفتگی  Bitcoin"}`:   "گزارش هفتگی Bitcoin",
		`{"name":"قرارداد.pdf","title":"x"}`: "قرارداد.pdf",
		`{"name":3,"title":"t"}`:             "t",
		`{"name":"  \n "}`:                   "",
	} {
		if got := metaTitle(json.RawMessage(m)); got != want {
			t.Errorf("metaTitle(%s) = %q, want %q", m, got, want)
		}
	}
}

func TestBucketFiles(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	op, err := e.svc.Project.Create(ctx, "Files")
	if err != nil {
		t.Fatal(err)
	}
	proj, err := e.pm.Open(ctx, op.ID)
	if err != nil {
		t.Fatal(err)
	}
	put := func(key, body, meta string) {
		t.Helper()
		o := store.PutOptions{Source: "upload"}
		if meta != "" {
			o.Metadata = json.RawMessage(meta)
		}
		if _, err := proj.DB.PutObject(ctx, limit.Interactive, id.SourceUser, key, strings.NewReader(body), o); err != nil {
			t.Fatal(err)
		}
	}
	put("docs/a.txt", "one", `{"name":"یادداشت.txt"}`)
	put("bin/run.txt.exe", "MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xff\xff\x00\x00", "")
	proj.Release()

	b := e.svc.Bucket
	o, err := b.Head(ctx, op.ID, "docs/a.txt")
	if err != nil || o.Title != "یادداشت.txt" || !o.Opens || o.Version != 1 {
		t.Fatalf("Head = %+v, %v", o, err)
	}
	if o, err := b.Head(ctx, op.ID, "bin/run.txt.exe"); err != nil || o.Opens {
		t.Fatalf("Head of a program = %+v, %v", o, err)
	}

	// Open with the system app: a read-only copy in the temp folder.
	var opened []string
	b.openFile = func(p string) error { opened = append(opened, p); return nil }
	b.tempDir = t.TempDir()
	if err := b.OpenWithSystem(ctx, op.ID, "docs/a.txt"); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || filepath.Base(opened[0]) != "a.txt" || !strings.HasPrefix(opened[0], filepath.Join(b.tempDir, openDir)) {
		t.Fatalf("opened %q", opened)
	}
	if got, err := os.ReadFile(opened[0]); err != nil || string(got) != "one" {
		t.Fatalf("the copy holds %q, %v", got, err)
	}
	if fi, err := os.Stat(opened[0]); err != nil || fi.Mode().Perm()&0o200 != 0 {
		t.Errorf("the copy is writable: %v, %v", fi.Mode(), err)
	}
	if runtime.GOOS == "windows" {
		if mark, err := os.ReadFile(opened[0] + ":Zone.Identifier"); err != nil || !strings.Contains(string(mark), "ZoneId=3") {
			t.Errorf("no Mark of the Web: %q, %v", mark, err)
		}
	}
	if err := b.OpenWithSystem(ctx, op.ID, "docs/a.txt"); err != nil || len(opened) != 2 || opened[1] != opened[0] {
		t.Fatalf("a second open: %q, %v", opened, err)
	}
	err = b.OpenWithSystem(ctx, op.ID, "bin/run.txt.exe")
	var ue *UIError
	if !errors.As(err, &ue) || ue.Kind != KindInvalid || len(opened) != 2 {
		t.Fatalf("a program: %v, opened %q", err, opened)
	}
	if err := b.OpenWithSystem(ctx, op.ID, "docs/none.txt"); err == nil {
		t.Fatal("a missing file opened")
	} else {
		uiErr(t, err, KindNotFound)
	}

	// Copies older than a day go; newer ones stay.
	dir := filepath.Dir(opened[0])
	b.cleanOpened()
	if _, err := os.Stat(opened[0]); err != nil {
		t.Fatalf("a new copy went: %v", err)
	}
	old := time.Now().Add(-2 * openKeep)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatal(err)
	}
	b.cleanOpened()
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an old copy stayed: %v", err)
	}

	// Delete, and undo it.
	d, err := b.Delete(ctx, op.ID, "docs/a.txt")
	if err != nil || d.Change == "" {
		t.Fatalf("Delete = %+v, %v", d, err)
	}
	if _, err := b.Head(ctx, op.ID, "docs/a.txt"); err == nil {
		t.Fatal("the file is still there")
	}
	if err := b.UndoDelete(ctx, op.ID, d.Change); err != nil {
		t.Fatal(err)
	}
	if o, err := b.Head(ctx, op.ID, "docs/a.txt"); err != nil || o.Title != "یادداشت.txt" {
		t.Fatalf("after undo: %+v, %v", o, err)
	}
	if err := b.UndoDelete(ctx, op.ID, d.Change); err == nil {
		t.Fatal("a second undo")
	} else {
		uiErr(t, err, KindBusy)
	}

	// A file saved under the key since stops the undo.
	d, err = b.Delete(ctx, op.ID, "docs/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	proj, err = e.pm.Open(ctx, op.ID)
	if err != nil {
		t.Fatal(err)
	}
	put("docs/a.txt", "new", "")
	proj.Release()
	if err := b.UndoDelete(ctx, op.ID, d.Change); err == nil {
		t.Fatal("the undo ran over a newer file")
	} else {
		uiErr(t, err, KindBusy)
	}
	if _, err := b.Delete(ctx, op.ID, "docs/none.txt"); err == nil {
		t.Fatal("deleted a missing file")
	} else {
		uiErr(t, err, KindNotFound)
	}
	if e.pm.Busy() {
		t.Error("a call kept a lease")
	}
}
