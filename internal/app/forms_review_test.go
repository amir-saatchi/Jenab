package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/viewwrite"
)

// head reports whether project p's bucket has key.
func head(t *testing.T, e *env, p id.Project, key string) bool {
	t.Helper()
	var found bool
	err := open(context.Background(), e.pm, p, func(proj *project.Project) error {
		_, err := proj.DB.HeadObject(context.Background(), key)
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		found = err == nil
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func file(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("%PDF-1.4 receipt"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Upload reads only a regular file: a folder and a device are refused
// before anything is opened or stored.
func TestFormUploadRegularOnly(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	p := formsProject(t, e)
	fs := e.svc.Forms
	for path, want := range map[string]string{
		t.TempDir(): "Choose a file, not a folder.",
		os.DevNull:  "Choose a regular file.", // a character device on Windows and Unix alike
	} {
		_, err := fs.Upload(ctx, p, "add_doc", "file", path, nil)
		var ue *UIError
		if !errors.As(err, &ue) || ue.Kind != KindInvalid || ue.Message != want {
			t.Errorf("%s: %v, want %q", path, err, want)
		}
	}
	if n := count(t, e, p, "SELECT count(*) FROM _jenab_changes WHERE target LIKE 'object:%'"); n != 0 {
		t.Errorf("%d objects stored", n)
	}
}

// A file field takes only a key Upload handed out for it, once; a submit
// that fails moves its uploads to the trash as the form's, and an upload
// for a form never submitted stays.
func TestFormUploadKeys(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	p := formsProject(t, e)
	fs := e.svc.Forms

	// A file in the bucket that this form didn't upload.
	other, err := fs.Upload(ctx, p, "attach_doc", "file", file(t, "other.pdf"), nil)
	if err != nil {
		t.Fatal(err)
	}
	sub, err := fs.Submit(ctx, p, "add_doc", nil, map[string]viewwrite.Value{"keyword": {Text: "a"}, "file": {Text: "receipts/x.pdf"}})
	if err != nil || sub.Errors["file"] != "is not a file uploaded to this field" {
		t.Errorf("never uploaded %+v, %v", sub, err)
	}
	sub, err = fs.Submit(ctx, p, "add_doc", nil, map[string]viewwrite.Value{"keyword": {Text: "a"}, "file": {Text: other.Key}})
	if err != nil || sub.Errors["file"] != "is not a file uploaded to this field" || len(sub.Removed) != 0 || !head(t, e, p, other.Key) {
		t.Errorf("another form's upload %+v, %v", sub, err)
	}

	// A submit that fails on its keyword: the upload goes to the trash.
	up, err := fs.Upload(ctx, p, "add_doc", "file", file(t, "r.pdf"), nil)
	if err != nil {
		t.Fatal(err)
	}
	sub, err = fs.Submit(ctx, p, "add_doc", nil, map[string]viewwrite.Value{"keyword": {Text: "k1"}, "file": {Text: up.Key}})
	if err != nil || sub.Errors["keyword"] != "a row with this value already exists" || len(sub.Removed) != 1 || sub.Removed[0] != "file" {
		t.Fatalf("failed submit %+v, %v", sub, err)
	}
	if head(t, e, p, up.Key) {
		t.Errorf("%s is still in the bucket", up.Key)
	}
	if n := count(t, e, p, "SELECT count(*) FROM _jenab_changes WHERE source = 'form:add_doc' AND target = ?", "object:"+up.Key); n != 2 {
		t.Errorf("%d entries for %s as form:add_doc, want the upload and its delete", n, up.Key)
	}
	// Its key is spent.
	sub, _ = fs.Submit(ctx, p, "add_doc", nil, map[string]viewwrite.Value{"keyword": {Text: "new"}, "file": {Text: up.Key}})
	if sub.Errors["file"] != "is not a file uploaded to this field" {
		t.Errorf("a trashed key %+v", sub)
	}

	// A good submit spends its key too; an upload never submitted stays.
	up, err = fs.Upload(ctx, p, "add_doc", "file", file(t, "s.pdf"), nil)
	if err != nil {
		t.Fatal(err)
	}
	kept, err := fs.Upload(ctx, p, "add_doc", "file", file(t, "kept.pdf"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if sub, err := fs.Submit(ctx, p, "add_doc", nil, map[string]viewwrite.Value{"keyword": {Text: "s"}, "file": {Text: up.Key}}); err != nil || !sub.OK() {
		t.Fatalf("submit %+v, %v", sub, err)
	}
	sub, _ = fs.Submit(ctx, p, "add_doc", nil, map[string]viewwrite.Value{"keyword": {Text: "s2"}, "file": {Text: up.Key}})
	if sub.Errors["file"] != "is not a file uploaded to this field" || len(sub.Removed) != 0 || !head(t, e, p, up.Key) {
		t.Errorf("a used key %+v", sub)
	}
	if !head(t, e, p, kept.Key) {
		t.Errorf("the upload never submitted is gone")
	}
}

// Undo takes only a form's changes, and says apart a later change that
// stops it from one undone already.
func TestFormUndo(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	p := formsProject(t, e)
	fs := e.svc.Forms

	// The agent's change, and one that isn't there: not found.
	var agent id.Change
	err := open(ctx, e.pm, p, func(proj *project.Project) error {
		res, err := proj.DB.Insert(ctx, limit.Interactive, id.SourceOf("message", id.New()), "docs", []store.Row{{"keyword": "agent"}}, store.ConflictFail)
		agent = res.Change
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []id.Change{agent, id.Change(id.New())} {
		err := fs.Undo(ctx, p, c)
		uiErr(t, err, KindNotFound)
		if !errors.Is(err, store.ErrNotFound) {
			t.Errorf("%s: %v", c, err)
		}
	}
	if n := count(t, e, p, "SELECT count(*) FROM docs WHERE keyword = 'agent'"); n != 1 {
		t.Errorf("the agent's row was undone")
	}

	// A form's insert, then an edit of its row: the insert can't be
	// undone, a conflict.
	ins, err := fs.Submit(ctx, p, "add_doc", nil, map[string]viewwrite.Value{"keyword": {Text: "fresh"}})
	if err != nil || !ins.OK() {
		t.Fatalf("insert %+v, %v", ins, err)
	}
	rowID := count(t, e, p, "SELECT id FROM docs WHERE keyword = 'fresh'")
	upd, err := fs.Submit(ctx, p, "edit_doc", int64(rowID), map[string]viewwrite.Value{"keyword": {Text: "fresher"}, "weight": {Text: "2"}})
	if err != nil || !upd.OK() {
		t.Fatalf("update %+v, %v", upd, err)
	}
	err = fs.Undo(ctx, p, ins.Change)
	uiErr(t, err, KindBusy)
	if !errors.Is(err, store.ErrUndoConflict) || errors.Is(err, store.ErrNothingToUndo) {
		t.Errorf("conflict: %v", err)
	}
	// The edit undoes; undone twice is nothing to undo.
	if err := fs.Undo(ctx, p, upd.Change); err != nil {
		t.Fatal(err)
	}
	err = fs.Undo(ctx, p, upd.Change)
	uiErr(t, err, KindBusy)
	if !errors.Is(err, store.ErrNothingToUndo) || errors.Is(err, store.ErrUndoConflict) {
		t.Errorf("twice: %v", err)
	}
}

// A bulk through the service: 500 rows each chosen twice is 500 rows.
func TestFormBulkDuplicates(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	p := formsProject(t, e)
	fs := e.svc.Forms
	var keys []any
	for i := 1; i <= 500; i++ {
		keys = append(keys, int64(i), float64(i))
	}
	a, err := fs.RowAction(ctx, p, "doc_table", 1, keys, false)
	if err != nil || a.Confirm != "Delete 500 rows?" {
		t.Fatalf("confirm %+v, %v", a, err)
	}
	a, err = fs.RowAction(ctx, p, "doc_table", 1, keys, true)
	if err != nil || a.Rows != 500 || count(t, e, p, "SELECT count(*) FROM docs") != 100 {
		t.Fatalf("delete %+v, %v", a, err)
	}
	if n := count(t, e, p, "SELECT count(*) FROM _jenab_changes WHERE source = 'form:doc_table'"); n != 1 {
		t.Errorf("%d entries", n)
	}
}

// The composer's route and a form's file field take keys from one
// reservation: an upload of the same name while the other is still
// streaming gets the next free key.
func TestUploadsShareReservation(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	p := formsProject(t, e)
	pr, pw := io.Pipe()
	r := httptest.NewRequest(http.MethodPost, "/"+string(p), pr)
	r.Header.Set(UploadNameHeader, url.PathEscape("same.pdf"))
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		UploadHandler(e.pm).ServeHTTP(rec, r)
	}()
	// Wait until the composer's upload holds its key.
	taken := func() bool {
		reserved.mu.Lock()
		defer reserved.mu.Unlock()
		return reserved.taken[string(p)+"/uploads/same.pdf"]
	}
	for deadline := time.Now().Add(10 * time.Second); !taken(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			pw.Close()
			t.Fatal("the composer's upload never reserved its key")
		}
	}
	up, err := e.svc.Forms.Upload(ctx, p, "attach_doc", "file", file(t, "same.pdf"), nil)
	if err != nil || up.Key != "uploads/same-2.pdf" {
		t.Errorf("form upload %+v, %v", up, err)
	}
	if _, err := pw.Write([]byte("%PDF-1.4 composer")); err != nil {
		t.Fatal(err)
	}
	pw.Close()
	<-done
	var got Uploaded
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.Key != "uploads/same.pdf" {
		t.Errorf("composer upload %d %s", rec.Code, rec.Body)
	}
	if !head(t, e, p, "uploads/same.pdf") || !head(t, e, p, "uploads/same-2.pdf") {
		t.Error("both files should be stored")
	}
}
