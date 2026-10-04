package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

func put(t *testing.T, p *ProjectDB, key, content string) Object {
	t.Helper()
	o, err := p.PutObject(context.Background(), limit.Interactive, id.SourceUser, key, strings.NewReader(content), PutOptions{Source: "upload"})
	if err != nil {
		t.Fatalf("put %s: %v", key, err)
	}
	return o
}

func storedHashes(t *testing.T, p *ProjectDB) []string {
	t.Helper()
	var hs []string
	for s, err := range p.objects.All(context.Background()) {
		if err != nil {
			t.Fatal(err)
		}
		hs = append(hs, s.Hash)
	}
	slices.Sort(hs)
	return hs
}

func read(t *testing.T, p *ProjectDB, key string) string {
	t.Helper()
	_, r, err := p.OpenObject(context.Background(), key)
	if err != nil {
		t.Fatalf("open %s: %v", key, err)
	}
	defer r.Close()
	b, _ := io.ReadAll(r)
	return string(b)
}

func TestObjectSameContentOneFile(t *testing.T) {
	p, _ := openTestProject(t)
	a := put(t, p, "a/one.txt", "same bytes")
	b := put(t, p, "b/two.txt", "same bytes")
	if a.SHA256 != b.SHA256 {
		t.Fatalf("hashes differ: %s %s", a.SHA256, b.SHA256)
	}
	if hs := storedHashes(t, p); len(hs) != 1 {
		t.Fatalf("%d files, want 1", len(hs))
	}
	if read(t, p, "a/one.txt") != "same bytes" || read(t, p, "b/two.txt") != "same bytes" {
		t.Fatal("content differs")
	}
}

func TestObjectPutAndHead(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	src := id.SourceOf("message", id.New())
	before := time.Now().Add(-time.Second)
	o, err := p.PutObject(ctx, limit.Background, src, "cache/pages/example.com/1.html", strings.NewReader("<html><body>hi</body></html>"),
		PutOptions{Source: "http", SourceURL: "https://example.com/", Metadata: json.RawMessage(`{"name":"index.html"}`)})
	if err != nil {
		t.Fatal(err)
	}
	h, err := p.HeadObject(ctx, o.Key)
	if err != nil {
		t.Fatal(err)
	}
	if h.MIME != "text/html; charset=utf-8" || h.Size != 28 || h.Version != 1 || h.Source != "http" || h.SourceURL != "https://example.com/" ||
		h.CreatedBy != src || string(h.Metadata) != `{"name":"index.html"}` || h.SHA256 != o.SHA256 || !h.UpdatedAt.Equal(o.UpdatedAt) {
		t.Fatalf("head = %+v\nput = %+v", h, o)
	}
	if h.UpdatedAt.Before(before) || h.Expires.Sub(h.UpdatedAt) != 30*24*time.Hour || !o.Expires.Equal(h.Expires) {
		t.Errorf("updated %v, expires %v; want 30 days later", h.UpdatedAt, h.Expires)
	}
	if n := put(t, p, "reports/a.txt", "x"); !n.Expires.IsZero() {
		t.Errorf("reports/ expires %v, want never", n.Expires)
	}
}

func TestObjectVersions(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	put(t, p, "k.txt", "v1")
	put(t, p, "k.txt", "v2")
	same := put(t, p, "k.txt", "v2") // same content: no new version
	if same.Version != 2 {
		t.Fatalf("version %d, want 2", same.Version)
	}
	if got := read(t, p, "k.txt"); got != "v2" {
		t.Fatalf("read %q", got)
	}
	vs, err := p.ObjectVersions(ctx, "k.txt")
	if err != nil || len(vs) != 2 || vs[0].Version != 2 || vs[1].Version != 1 || vs[0].Size != 2 || vs[1].CreatedBy != id.SourceUser {
		t.Fatalf("versions = %+v, %v", vs, err)
	}
	if len(storedHashes(t, p)) != 2 {
		t.Fatal("both versions' bytes should be kept")
	}
}

func TestObjectDelete(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	o := put(t, p, "k.txt", "gone soon")
	if err := p.DeleteObject(ctx, limit.Interactive, id.SourceUser, "k.txt"); err != nil {
		t.Fatal(err)
	}
	_, err := p.HeadObject(ctx, "k.txt")
	if !errors.Is(err, ErrNotFound) || !errors.Is(err, bucket.ErrNotFound) {
		t.Fatalf("head after delete = %v, want both not-found errors", err)
	}
	if _, _, err := p.OpenObject(ctx, "k.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("open after delete = %v", err)
	}
	if err := p.DeleteObject(ctx, limit.Interactive, id.SourceUser, "k.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v", err)
	}
	// The bytes stay for undo: a version still points to them.
	if vs, _ := p.ObjectVersions(ctx, "k.txt"); len(vs) != 1 {
		t.Fatalf("versions after delete: %d", len(vs))
	}
	if rep, err := p.SweepObjects(ctx); err != nil || rep.Files != 0 {
		t.Fatalf("sweep = %+v, %v; want nothing removed", rep, err)
	}
	if hs := storedHashes(t, p); len(hs) != 1 || hs[0] != o.SHA256 {
		t.Fatal("the deleted object's bytes are gone")
	}
	// A new object under the key continues the numbering.
	if n := put(t, p, "k.txt", "back"); n.Version != 2 {
		t.Fatalf("version after re-put %d, want 2", n.Version)
	}
}

func TestObjectMove(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	put(t, p, "a.txt", "a1")
	put(t, p, "a.txt", "a2")
	put(t, p, "b.txt", "b1")
	if _, err := p.MoveObject(ctx, limit.Interactive, id.SourceUser, "a.txt", "b.txt"); !errors.Is(err, ErrObjectExists) {
		t.Fatalf("move onto b = %v, want ErrObjectExists", err)
	}
	if _, err := p.MoveObject(ctx, limit.Interactive, id.SourceUser, "nope.txt", "c.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("move missing = %v", err)
	}
	// b.txt is deleted but keeps a version; a.txt's two versions go after it.
	if err := p.DeleteObject(ctx, limit.Interactive, id.SourceUser, "b.txt"); err != nil {
		t.Fatal(err)
	}
	src := id.SourceOf("message", id.New())
	o, err := p.MoveObject(ctx, limit.Interactive, src, "a.txt", "b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if o.Key != "b.txt" || o.Version != 3 || o.CreatedBy != src || read(t, p, "b.txt") != "a2" {
		t.Fatalf("moved = %+v", o)
	}
	vs, _ := p.ObjectVersions(ctx, "b.txt")
	if len(vs) != 3 || vs[0].Version != 3 || vs[1].Version != 2 || vs[2].Version != 1 {
		t.Fatalf("versions = %+v", vs)
	}
	if _, err := p.HeadObject(ctx, "a.txt"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a.txt still there: %v", err)
	}
	if vs, _ := p.ObjectVersions(ctx, "a.txt"); len(vs) != 0 {
		t.Fatalf("a.txt kept %d versions", len(vs))
	}
}

func listAll(t *testing.T, p *ProjectDB, prefix string, max int) (keys, folders []string, pages int) {
	t.Helper()
	after := ""
	for {
		l, err := p.ListObjects(context.Background(), prefix, after, max)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		if len(l.Objects)+len(l.Folders) > max {
			t.Fatalf("page has %d entries, max %d", len(l.Objects)+len(l.Folders), max)
		}
		for _, o := range l.Objects {
			keys = append(keys, o.Key)
		}
		folders = append(folders, l.Folders...)
		if l.Next == "" {
			return
		}
		after = l.Next
		if pages > 100 {
			t.Fatal("listing doesn't end")
		}
	}
}

func TestObjectList(t *testing.T) {
	p, _ := openTestProject(t)
	for _, k := range []string{"a.txt", "b/1.txt", "b/2.txt", "b/c/3.txt", "c.txt", "d/x/y/z.txt", "e.txt", "images/btc/1.png", "images/btc/2.png", "images/eth.png", "z"} {
		put(t, p, k, k)
	}
	for _, max := range []int{1, 2, 3, 1000} {
		keys, folders, _ := listAll(t, p, "", max)
		if strings.Join(keys, ",") != "a.txt,c.txt,e.txt,z" || strings.Join(folders, ",") != "b/,d/,images/" {
			t.Errorf("max %d: keys %v, folders %v", max, keys, folders)
		}
		keys, folders, _ = listAll(t, p, "b/", max)
		if strings.Join(keys, ",") != "b/1.txt,b/2.txt" || strings.Join(folders, ",") != "b/c/" {
			t.Errorf("b/ max %d: keys %v, folders %v", max, keys, folders)
		}
		keys, folders, _ = listAll(t, p, "images/", max)
		if strings.Join(keys, ",") != "images/eth.png" || strings.Join(folders, ",") != "images/btc/" {
			t.Errorf("images/ max %d: keys %v, folders %v", max, keys, folders)
		}
	}
	// A prefix in the middle of a segment.
	keys, folders, _ := listAll(t, p, "images/e", 10)
	if strings.Join(keys, ",") != "images/eth.png" || len(folders) != 0 {
		t.Errorf("images/e: %v %v", keys, folders)
	}
	// One page of 1 per entry: 4 keys and 3 folders take 7 pages.
	if _, _, pages := listAll(t, p, "", 1); pages != 7 {
		t.Errorf("%d pages, want 7", pages)
	}
	if _, err := p.ListObjects(context.Background(), "/bad", "", 10); err == nil {
		t.Error("a bad prefix was listed")
	}
	// Expiry is filled in lists too.
	put(t, p, "cache/x", "x")
	l, _ := p.ListObjects(context.Background(), "cache/", "", 10)
	if len(l.Objects) != 1 || l.Objects[0].Expires.IsZero() {
		t.Errorf("cache/ list = %+v", l.Objects)
	}
}

func TestObjectListManyInFolder(t *testing.T) {
	p, _ := openTestProject(t)
	for i := range 50 {
		put(t, p, fmt.Sprintf("big/%03d", i), "x")
	}
	put(t, p, "next.txt", "x")
	keys, folders, pages := listAll(t, p, "", 2)
	if strings.Join(keys, ",") != "next.txt" || strings.Join(folders, ",") != "big/" || pages != 1 {
		t.Fatalf("keys %v, folders %v, %d pages", keys, folders, pages)
	}
}

func TestObjectRejects(t *testing.T) {
	p, dir := openTestProject(t)
	ctx := context.Background()
	cases := []struct {
		key string
		o   PutOptions
	}{
		{"../x", PutOptions{Source: "upload"}},
		{"a//b", PutOptions{Source: "upload"}},
		{"ok.txt", PutOptions{Source: "magic"}},
		{"ok.txt", PutOptions{Source: "upload", Metadata: json.RawMessage(`[1,2]`)}},
		{"ok.txt", PutOptions{Source: "upload", Metadata: json.RawMessage(`null`)}},
		{"ok.txt", PutOptions{Source: "upload", Metadata: json.RawMessage(`{"a":` + strings.Repeat(" ", maxMetadata) + `1}`)}},
		{"big.txt", PutOptions{Source: "upload", MaxSize: 3}},
	}
	for _, c := range cases {
		if _, err := p.PutObject(ctx, limit.Interactive, id.SourceUser, c.key, strings.NewReader("1234"), c.o); err == nil {
			t.Errorf("put %q %+v worked", c.key, c.o)
		}
	}
	if _, err := p.PutObject(ctx, limit.Interactive, id.SourceUser, "big.txt", strings.NewReader("1234"), PutOptions{Source: "upload", MaxSize: 3}); !errors.Is(err, bucket.ErrTooLarge) {
		t.Errorf("too large: %v", err)
	}
	if hs := storedHashes(t, p); len(hs) != 0 {
		t.Errorf("rejected puts left %d files", len(hs))
	}
	if n := one[int](t, p.DB, "SELECT count(*) FROM _jenab_objects"); n != 0 {
		t.Errorf("%d rows", n)
	}
	ents, _ := os.ReadDir(filepath.Join(dir, "tmp"))
	if len(ents) != 0 {
		t.Errorf("tmp/ has %d entries", len(ents))
	}
}

func TestObjectReadOnly(t *testing.T) {
	p, dir := openTestProject(t)
	ctx := context.Background()
	put(t, p, "a.txt", "hello")
	p.Close(ctx)
	ro, err := OpenProjectReadOnly(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close(ctx)
	if got := read(t, ro, "a.txt"); got != "hello" {
		t.Fatalf("read %q", got)
	}
	if _, err := ro.PutObject(ctx, limit.Interactive, id.SourceUser, "b.txt", strings.NewReader("new bytes"), PutOptions{Source: "upload"}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("put = %v, want ErrReadOnly", err)
	}
	if hs := storedHashes(t, ro); len(hs) != 1 {
		t.Fatalf("%d files, want 1: the read-only put wrote bytes", len(hs))
	}
	if _, err := ro.SweepObjects(ctx); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("sweep = %v", err)
	}
}

func TestObjectSweep(t *testing.T) {
	p, dir := openTestProject(t)
	ctx := context.Background()
	kept := put(t, p, "a.txt", "kept")
	// Bytes with no row, as a crash between the file and the row leaves them.
	orphan, err := p.objects.Put(ctx, strings.NewReader("orphan"), 0)
	if err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "tmp", "old.partial")
	os.WriteFile(old, []byte("x"), 0o644)
	os.Chtimes(old, time.Now().Add(-48*time.Hour), time.Now().Add(-48*time.Hour))
	fresh := filepath.Join(dir, "tmp", "fresh.partial")
	os.WriteFile(fresh, []byte("x"), 0o644)

	rep, err := p.SweepObjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 1 || rep.Bytes != 6 || rep.Tmp != 1 {
		t.Fatalf("sweep = %+v, want 1 file of 6 bytes and 1 tmp file", rep)
	}
	if hs := storedHashes(t, p); len(hs) != 1 || hs[0] != kept.SHA256 || hs[0] == orphan.Hash {
		t.Fatalf("left %v", hs)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("a fresh tmp file was removed")
	}
}

// TestObjectSweepDuringPuts runs sweeps while puts store content that was
// left without a row, so a put and the sweep race for the same file. No
// row may end up pointing to a missing file.
func TestObjectSweepDuringPuts(t *testing.T) {
	p, _ := openTestProject(t)
	ctx := context.Background()
	const n = 40
	for i := range n {
		p.objects.Put(ctx, strings.NewReader(fmt.Sprintf("content %d", i)), 0) // orphans to start with
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := p.SweepObjects(ctx); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	for i := range n {
		put(t, p, fmt.Sprintf("k/%d", i), fmt.Sprintf("content %d", i))
	}
	close(stop)
	wg.Wait()
	for i := range n {
		if got := read(t, p, fmt.Sprintf("k/%d", i)); got != fmt.Sprintf("content %d", i) {
			t.Fatalf("k/%d = %q", i, got)
		}
	}
}

func TestLifecycleExpires(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	var l Lifecycle
	if err := json.Unmarshal([]byte(defaultLifecycle), &l); err != nil {
		t.Fatal(err)
	}
	if got := l.expires("cache/a", now); !got.Equal(now.AddDate(0, 0, 30)) {
		t.Errorf("cache/a expires %v", got)
	}
	if got := l.expires("cachex/a", now); !got.IsZero() {
		t.Errorf("cachex/a expires %v", got)
	}
	l.Rules = append(l.Rules, LifecycleRule{Prefix: "cache/pages/", ExpireAfterDays: 7}, LifecycleRule{Prefix: "c", ExpireAfterDays: 90},
		LifecycleRule{Prefix: "cache/", ExpireAfterDays: 5}) // a repeated prefix: the first rule wins
	if got := l.expires("cache/pages/x", now); !got.Equal(now.AddDate(0, 0, 7)) {
		t.Errorf("the longest prefix should win: %v", got)
	}
	if got := l.expires("cache/x", now); !got.Equal(now.AddDate(0, 0, 30)) {
		t.Errorf("cache/x: %v", got)
	}
}
