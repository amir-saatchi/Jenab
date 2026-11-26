package app

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/viewwrite"
)

var formsConfigs = []store.ConfigSave{
	{Kind: store.KindView, ID: "add_doc", Type: "form", Title: "Add a document", YAML: `version: 1
id: add_doc
title: Add a document
type: form
fields:
  - { field: keyword, control: text, required: true }
  - { field: weight, control: number }
  - { field: file, control: file, prefix: receipts/ }
submit: { action: insert, table: docs }
`},
	{Kind: store.KindView, ID: "edit_doc", Type: "form", Title: "Edit a document", YAML: `version: 1
id: edit_doc
title: Edit a document
type: form
load: "SELECT keyword, weight FROM docs WHERE id = :key"
fields:
  - { field: keyword, control: text, required: true }
  - { field: weight, control: number }
submit: { action: update, table: docs, key: id }
`},
	{Kind: store.KindView, ID: "attach_doc", Type: "form", Title: "Attach a document", YAML: `version: 1
id: attach_doc
title: Attach a document
type: form
fields:
  - { field: keyword, control: text, required: true }
  - { field: file, control: file, prefix: uploads/ }
submit: { action: insert, table: docs }
`},
	{Kind: store.KindView, ID: "doc_table", Type: "table", Title: "Documents", YAML: `version: 1
id: doc_table
title: Documents
type: table
query: "SELECT id, keyword FROM docs"
default_sort: { field: keyword, direction: asc }
rows_from: { table: docs, key: id }
columns:
  - { field: keyword }
row_actions:
  - { label: Edit, open_form: edit_doc }
  - { label: Delete, delete: true, confirm: true, bulk: true }
  - { label: Seen, set: { seen_at: "now()" }, bulk: true }
`},
}

func formsProject(t *testing.T, e *env) id.Project {
	t.Helper()
	ctx := context.Background()
	op, err := e.svc.Project.Create(ctx, "Docs")
	if err != nil {
		t.Fatal(err)
	}
	err = open(ctx, e.pm, op.ID, func(p *project.Project) error {
		_, err := store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
			for _, s := range []string{
				`CREATE TABLE docs(id INTEGER PRIMARY KEY, keyword TEXT NOT NULL UNIQUE, weight INTEGER, file TEXT, seen_at TEXT)`,
				`INSERT INTO _jenab_columns VALUES ('docs', 'file', 'object_ref')`,
				`WITH RECURSIVE c(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM c WHERE i < 600)
				 INSERT INTO docs(id, keyword) SELECT i, 'k' || i FROM c`,
			} {
				if _, err := tx.Exec(s); err != nil {
					return struct{}{}, err
				}
			}
			return struct{}{}, nil
		})
		if err != nil {
			return err
		}
		for _, c := range formsConfigs {
			c.Source = id.SourceUser
			if _, err := p.DB.SaveConfig(ctx, c); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return op.ID
}

// count runs a count query on project p.
func count(t *testing.T, e *env, p id.Project, q string, args ...any) int {
	t.Helper()
	var n int
	err := open(context.Background(), e.pm, p, func(proj *project.Project) error {
		rows, err := store.Query(context.Background(), proj.DB.DB, q, args, func(r *sql.Rows) (int, error) {
			var v int
			return v, r.Scan(&v)
		})
		if err == nil {
			n = rows[0]
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// The page screen's calls: open, submit with a constraint error, upload a
// file with its progress and submit its key, a bulk row action after its
// confirm, and each write's undo from its notice.
func TestFormService(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	p := formsProject(t, e)
	fs := e.svc.Forms

	f, err := fs.Open(ctx, p, "add_doc", nil)
	if err != nil || len(f.Fields) != 3 || f.Fields[2].Prefix != "receipts/" {
		t.Fatalf("open %+v, %v", f, err)
	}
	sub, err := fs.Submit(ctx, p, "add_doc", nil, map[string]viewwrite.Value{"keyword": {Text: "k1"}, "weight": {Text: "x"}})
	if err != nil || sub.Errors["weight"] != "is not a number" {
		t.Fatalf("type error %+v, %v", sub, err)
	}
	sub, err = fs.Submit(ctx, p, "add_doc", nil, map[string]viewwrite.Value{"keyword": {Text: "k1"}})
	if err != nil || sub.Errors["keyword"] != "a row with this value already exists" {
		t.Fatalf("constraint %+v, %v", sub, err)
	}

	// A file: stored under the field's prefix with its name, the progress
	// reported, then the key submitted with the form.
	path := filepath.Join(t.TempDir(), "Receipt Nov 2026.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4 "+strings.Repeat("x", 300_000)), 0o600); err != nil {
		t.Fatal(err)
	}
	var shares []float64
	up, err := fs.Upload(ctx, p, "add_doc", "file", path, func(s float64) { shares = append(shares, s) })
	if err != nil || up.Key != "receipts/Receipt-Nov-2026.pdf" || up.Name != "Receipt Nov 2026.pdf" || up.MIME != "application/pdf" {
		t.Fatalf("upload %+v, %v", up, err)
	}
	if len(shares) < 3 || shares[0] != 0 || shares[len(shares)-1] != 1 {
		t.Errorf("progress %v", shares)
	}
	for i := 1; i < len(shares); i++ {
		if shares[i] < shares[i-1] {
			t.Errorf("progress goes back: %v", shares)
		}
	}
	again, err := fs.Upload(ctx, p, "add_doc", "file", path, nil)
	if err != nil || again.Key != "receipts/Receipt-Nov-2026-2.pdf" {
		t.Errorf("second upload %+v, %v", again, err)
	}
	if n := count(t, e, p, "SELECT count(*) FROM _jenab_changes WHERE source = 'form:add_doc' AND target LIKE 'object:%'"); n != 2 {
		t.Errorf("%d upload entries as form:add_doc", n)
	}
	sub, err = fs.Submit(ctx, p, "add_doc", nil, map[string]viewwrite.Value{"keyword": {Text: "receipt"}, "file": {Text: up.Key}})
	if err != nil || !sub.OK() || sub.Change == "" {
		t.Fatalf("submit with file %+v, %v", sub, err)
	}
	if n := count(t, e, p, "SELECT count(*) FROM docs WHERE file = ?", up.Key); n != 1 {
		t.Errorf("%d rows with the file", n)
	}
	if err := fs.Undo(ctx, p, sub.Change); err != nil {
		t.Fatal(err)
	}
	if n := count(t, e, p, "SELECT count(*) FROM docs WHERE keyword = 'receipt'"); n != 0 {
		t.Errorf("the insert is still there after its undo")
	}
	uiErr(t, fs.Undo(ctx, p, sub.Change), KindBusy)

	// Upload refusals: no such field, no such file.
	_, err = fs.Upload(ctx, p, "add_doc", "keyword", path, nil)
	uiErr(t, err, KindInvalid)
	_, err = fs.Upload(ctx, p, "add_doc", "file", filepath.Join(t.TempDir(), "none.pdf"), nil)
	uiErr(t, err, KindInvalid)

	// Row actions: open_form, then a bulk delete of 500 after its confirm.
	a, err := fs.RowAction(ctx, p, "doc_table", 0, []any{int64(4)}, false)
	if err != nil || a.OpenForm != "edit_doc" || a.Key != int64(4) {
		t.Fatalf("open_form %+v, %v", a, err)
	}
	f, err = fs.Open(ctx, p, a.OpenForm, a.Key)
	if err != nil || f.Values["keyword"].Text != "k4" {
		t.Fatalf("edit form %+v, %v", f, err)
	}
	keys := make([]any, 500)
	for i := range keys {
		keys[i] = int64(i + 1)
	}
	a, err = fs.RowAction(ctx, p, "doc_table", 1, keys, false)
	if err != nil || a.Confirm != "Delete 500 rows?" {
		t.Fatalf("unconfirmed %+v, %v", a, err)
	}
	a, err = fs.RowAction(ctx, p, "doc_table", 1, keys, true)
	if err != nil || a.Rows != 500 || a.Change == "" {
		t.Fatalf("bulk delete %+v, %v", a, err)
	}
	if n := count(t, e, p, "SELECT count(*) FROM _jenab_changes WHERE source = 'form:doc_table'"); n != 1 {
		t.Errorf("%d entries for the bulk delete", n)
	}
	if err := fs.Undo(ctx, p, a.Change); err != nil {
		t.Fatal(err)
	}
	if n := count(t, e, p, "SELECT count(*) FROM docs"); n != 600 {
		t.Errorf("%d rows after the undo", n)
	}
	a, err = fs.RowAction(ctx, p, "doc_table", 2, append(keys, int64(501)), false)
	if err != nil || a.Problem != "At most 500 rows at once; 501 are chosen." {
		t.Errorf("501 %+v, %v", a, err)
	}
	// Several rows to an action without bulk, a view that isn't there.
	_, err = fs.RowAction(ctx, p, "doc_table", 0, keys[:2], false)
	uiErr(t, err, KindInvalid)
	_, err = fs.Open(ctx, p, "nope", nil)
	uiErr(t, err, KindNotFound)
	_, err = fs.RowAction(ctx, p, "add_doc", 0, keys[:1], false)
	uiErr(t, err, KindInvalid)
}
