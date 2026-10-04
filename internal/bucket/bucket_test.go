package bucket

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
)

func TestCheckKey(t *testing.T) {
	good := []string{"a", "images/btc/2026-09-27.png", "cache/pages/example.com/9d04.txt", "A_b-c.d", "..a/b..", strings.Repeat("a", MaxKey)}
	for _, k := range good {
		if err := CheckKey(k); err != nil {
			t.Errorf("CheckKey(%q) = %v, want ok", k, err)
		}
	}
	bad := []string{"", "/a", "a/", "a//b", "..", "a/../b", "./a", "a/.", "a b", "a\\b", "ä", "a?b", "a#b", "a%2Fb", "a:b", strings.Repeat("a", MaxKey+1)}
	for _, k := range bad {
		if err := CheckKey(k); !errors.Is(err, ErrBadKey) {
			t.Errorf("CheckKey(%q) = %v, want ErrBadKey", k, err)
		}
	}
}

func TestCheckPrefix(t *testing.T) {
	for _, p := range []string{"", "a", "a/", "images/b", "images/btc/", "a/.", "a/.."} {
		if err := CheckPrefix(p); err != nil {
			t.Errorf("CheckPrefix(%q) = %v, want ok", p, err)
		}
	}
	for _, p := range []string{"/", "/a", "a//", "../a", "a b", "a/../"} {
		if err := CheckPrefix(p); err == nil {
			t.Errorf("CheckPrefix(%q) = nil, want an error", p)
		}
	}
}

func TestLink(t *testing.T) {
	pid := id.Project(id.New())
	l := Link(pid, "images/a.png")
	if l != "jenab://"+string(pid)+"/images/a.png" {
		t.Fatalf("Link = %q", l)
	}
	gp, gk, err := ParseLink(l)
	if err != nil || gp != pid || gk != "images/a.png" {
		t.Fatalf("ParseLink = %q, %q, %v", gp, gk, err)
	}
	for _, s := range []string{"https://x/" + string(pid) + "/a", "jenab://not-an-id/a", "jenab://" + string(pid), "jenab://" + string(pid) + "/", "jenab://" + string(pid) + "/../a"} {
		if _, _, err := ParseLink(s); err == nil {
			t.Errorf("ParseLink(%q) worked", s)
		}
	}
}

func newFolder(t *testing.T) (*Folder, string) {
	dir := t.TempDir()
	return NewFolder(filepath.Join(dir, "objects"), filepath.Join(dir, "tmp")), dir
}

func files(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out
}

func TestFolderPutOnce(t *testing.T) {
	f, dir := newFolder(t)
	ctx := context.Background()
	a, err := f.Put(ctx, strings.NewReader("hello"), 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.Put(ctx, strings.NewReader("hello"), 0)
	if err != nil {
		t.Fatal(err)
	}
	const want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if a.Hash != want || b.Hash != want || a.Size != 5 || string(a.Head) != "hello" {
		t.Fatalf("Put = %+v, %+v", a, b)
	}
	if got := files(t, filepath.Join(dir, "objects")); len(got) != 1 || got[0] != filepath.Join(dir, "objects", "2c", "f2", want) {
		t.Fatalf("objects = %v, want one file at 2c/f2/<hash>", got)
	}
	if got := files(t, filepath.Join(dir, "tmp")); len(got) != 0 {
		t.Fatalf("tmp/ left %v", got)
	}
	r, err := f.Open(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(r)
	r.Close()
	if string(data) != "hello" {
		t.Fatalf("Open read %q", data)
	}
	if _, err := f.Open(ctx, strings.Repeat("0", 64)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open(missing) = %v, want ErrNotFound", err)
	}
	if _, err := f.Open(ctx, "../../x"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("Open(not a hash) = %v", err)
	}
	if err := f.Delete(ctx, want); err != nil {
		t.Fatal(err)
	}
	if err := f.Delete(ctx, want); err != nil {
		t.Fatalf("second Delete = %v, want nil", err)
	}
}

func TestFolderHeadIsFirstBytes(t *testing.T) {
	f, _ := newFolder(t)
	data := bytes.Repeat([]byte("0123456789"), 100)
	b, err := f.Put(context.Background(), iotestHalf{bytes.NewReader(data)}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b.Head, data[:headSize]) || b.Size != int64(len(data)) {
		t.Fatalf("head %d bytes, size %d", len(b.Head), b.Size)
	}
}

// iotestHalf returns at most 7 bytes per Read, so the head is filled in
// pieces.
type iotestHalf struct{ r io.Reader }

func (h iotestHalf) Read(p []byte) (int, error) { return h.r.Read(p[:min(len(p), 7)]) }

func TestFolderTooLarge(t *testing.T) {
	f, dir := newFolder(t)
	if _, err := f.Put(context.Background(), strings.NewReader("12345"), 5); err != nil {
		t.Fatalf("exactly max: %v", err)
	}
	if _, err := f.Put(context.Background(), strings.NewReader("123456"), 5); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("over max: %v, want ErrTooLarge", err)
	}
	if got := files(t, filepath.Join(dir, "objects")); len(got) != 1 {
		t.Fatalf("objects = %v, want only the first", got)
	}
	if got := files(t, filepath.Join(dir, "tmp")); len(got) != 0 {
		t.Fatalf("tmp/ left %v", got)
	}
}

func TestFolderPutCancelled(t *testing.T) {
	f, dir := newFolder(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.Put(ctx, strings.NewReader("x"), 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("Put = %v, want context.Canceled", err)
	}
	if got := files(t, dir); len(got) != 0 {
		t.Fatalf("left %v", got)
	}
}

func TestFolderAllAndCleanTmp(t *testing.T) {
	f, dir := newFolder(t)
	ctx := context.Background()
	b, _ := f.Put(ctx, strings.NewReader("one"), 0)
	// Files that aren't the bucket's: a wrong place, a bad name, a folder.
	for _, wrong := range []string{filepath.Join(dir, "objects", "00", "00", b.Hash), filepath.Join(dir, "objects", b.Hash[:2], "00", b.Hash)} {
		os.MkdirAll(filepath.Dir(wrong), 0o755)
		os.WriteFile(wrong, []byte("one"), 0o644)
	}
	os.WriteFile(filepath.Join(dir, "objects", "README"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "objects", b.Hash[:2], b.Hash[2:4], "notes.txt"), nil, 0o644)
	var got []string
	for s, err := range f.All(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, s.Hash)
		if s.Size != 3 {
			t.Errorf("size %d", s.Size)
		}
	}
	if len(got) != 1 || got[0] != b.Hash {
		t.Fatalf("All = %v, want just %s", got, b.Hash)
	}

	old, fresh := filepath.Join(dir, "tmp", "old.partial"), filepath.Join(dir, "tmp", "fresh.partial")
	os.WriteFile(old, nil, 0o644)
	os.WriteFile(fresh, nil, 0o644)
	os.Chtimes(old, time.Now().Add(-25*time.Hour), time.Now().Add(-25*time.Hour))
	n, err := f.CleanTmp(ctx, time.Now().Add(-24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("CleanTmp = %d, %v; want 1", n, err)
	}
	if _, err := os.Stat(old); err == nil {
		t.Error("the old file is still there")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("the fresh file was removed")
	}
}

func TestDetectMIME(t *testing.T) {
	png := "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"
	cases := []struct{ head, key, want string }{
		{png, "a.txt", "image/png"},
		{"<!DOCTYPE html><html><body>hi", "a.png", "text/html; charset=utf-8"},
		{"<html><script>x</script>", "a.csv", "text/html; charset=utf-8"},
		{`<svg xmlns="http://www.w3.org/2000/svg"></svg>`, "a.txt", "image/svg+xml"},
		{"\xef\xbb\xbf  <?xml version=\"1.0\"?>\n<!-- made by hand -->\n<!DOCTYPE svg PUBLIC \"-//W3C//DTD SVG 1.1//EN\" \"x.dtd\">\n<SVG>", "a", "image/svg+xml"},
		{"<svgfoo/>", "a.svg", "text/plain; charset=utf-8"},
		{"<?xml version=\"1.0\"?><note/>", "a.svg", "text/xml; charset=utf-8"},
		{"a,b\n1,2\n", "data/x.CSV", "text/csv; charset=utf-8"},
		{`{"a":1}`, "x.json", "application/json; charset=utf-8"},
		{"# Title\n", "x.md", "text/markdown; charset=utf-8"},
		{"plain words", "x", "text/plain; charset=utf-8"},
		{"\x00\x01\x02\x03", "x.csv", "application/octet-stream"},
	}
	for _, c := range cases {
		if got := DetectMIME([]byte(c.head), c.key); got != c.want {
			t.Errorf("DetectMIME(%q, %q) = %q, want %q", c.head, c.key, got, c.want)
		}
	}
}
