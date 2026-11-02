package tool

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/docx"
	"github.com/amir-saatchi/jenab/internal/docx/docxtest"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/store"
)

func TestDocumentRead(t *testing.T) {
	env := testEnv(t)
	ctx := context.Background()
	lines := []string{"# Letter", "Dear Sara,", "The Bitcoin report is attached."}
	for i := range 10 {
		lines = append(lines, fmt.Sprintf("Line %d of the letter.", i))
	}
	o := put(t, env, "uploads/letter.docx", string(docxtest.Doc(lines...)), store.PutOptions{})
	if o.MIME != docx.MIME {
		t.Fatalf("stored as %q", o.MIME)
	}
	text := "# Letter\n\nDear Sara,\n\nThe Bitcoin report is attached.\n\nLine 0"

	// The header names the type; the ref is the document's own key.
	got := mustText(t, env, "bucket_read", `{"key": "uploads/letter.docx"}`)
	contains(t, got, "[object uploads/letter.docx — Word document — ", "showing first 25 — ref: uploads/letter.docx]\n"+text, "the next offset is ")
	if strings.Contains(got, "cache/") {
		t.Errorf("the cache key is shown:\n%s", got)
	}
	cache, err := env.Project.DB.HeadObject(ctx, TextKey(o.SHA256))
	if err != nil || cache.Source != "tool" || cache.Expires.IsZero() || !strings.HasPrefix(cache.MIME, "text/markdown") {
		t.Fatalf("cache %+v, %v", cache, err)
	}

	// A link to the file is read like its key; a link elsewhere isn't.
	link := bucket.Link(env.Project.ID, "uploads/letter.docx")
	if l := mustText(t, env, "bucket_read", `{"key": "`+link+`"}`); l != got {
		t.Errorf("by link:\n%s", l)
	}
	contains(t, toolError(t, env, "read_ref", `{"ref": "`+bucket.Link(id.Project(id.New()), "uploads/letter.docx")+`"}`), "a link to another project")
	contains(t, toolError(t, env, "read_ref", `{"ref": "jenab://x"}`), "jenab://x is not a link to a file")

	// read_ref and search_ref work in offsets of the text.
	got = mustText(t, env, "search_ref", `{"ref": "uploads/letter.docx", "query": "bitcoin"}`)
	off := strings.Index(text, "The Bitcoin")
	contains(t, got, "1 lines of uploads/letter.docx match:", fmt.Sprintf("- offset %d · line 5: The Bitcoin report", off))
	got = mustText(t, env, "read_ref", fmt.Sprintf(`{"ref": "%s", "offset": %d}`, link, off))
	contains(t, got, fmt.Sprintf("[ref — 71 tokens, showing 24 from offset %d — ref: uploads/letter.docx]\nThe Bitcoin report", off))

	// The text is made again when the cache is gone or has expired.
	if err := env.Project.DB.DeleteObject(ctx, limit.Interactive, "user", TextKey(o.SHA256)); err != nil {
		t.Fatal(err)
	}
	contains(t, mustText(t, env, "read_ref", `{"ref": "uploads/letter.docx"}`), "Dear Sara,")
	_, err = store.Do(ctx, env.Project.DB.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		_, err := tx.Exec(`UPDATE _jenab_objects SET updated_at = '2020-01-01T00:00:00.000Z' WHERE key = ?`, TextKey(o.SHA256))
		return struct{}{}, err
	})
	if err != nil {
		t.Fatal(err)
	}
	contains(t, mustText(t, env, "bucket_read", `{"key": "uploads/letter.docx"}`), "Dear Sara,")
	if c, err := env.Project.DB.HeadObject(ctx, TextKey(o.SHA256)); err != nil || expired(c) {
		t.Errorf("cache after expiry %+v, %v", c, err)
	}
}

func TestDocumentErrors(t *testing.T) {
	env := testEnv(t)
	put(t, env, "uploads/secret.docx", string(docxtest.OLE(true, 4096)), store.PutOptions{})
	put(t, env, "uploads/old.doc", string(docxtest.OLE(false, 4096)), store.PutOptions{})
	put(t, env, "uploads/broken.docx", string(docxtest.Zip(map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"word/document.xml": `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p a=b>`,
	})), store.PutOptions{})
	put(t, env, "uploads/a.zip", string(docxtest.Zip(map[string]string{"a.txt": "x"})), store.PutOptions{})
	for name, args := range map[string]string{
		"bucket_read": `{"key": %q}`,
		"read_ref":    `{"ref": %q}`,
		"search_ref":  `{"ref": %q, "query": "x"}`,
	} {
		read := func(key string) string { return toolError(t, env, name, fmt.Sprintf(args, key)) }
		contains(t, read("uploads/secret.docx"), "could not read uploads/secret.docx: it is password-protected")
		contains(t, read("uploads/old.doc"), "could not read uploads/old.doc: it is an old Word, Excel or PowerPoint file")
		contains(t, read("uploads/broken.docx"), "could not read uploads/broken.docx: it is damaged: ")
	}
	contains(t, toolError(t, env, "bucket_read", `{"key": "uploads/a.zip"}`), "uploads/a.zip is application/zip", "only text, Word documents and images")
	// Nothing was cached for files that can't be read.
	if l, err := env.Project.DB.ListObjects(context.Background(), "cache/text/", "", 10); err != nil || len(l.Objects) != 0 {
		t.Errorf("cache/text/ has %+v, %v", l, err)
	}
}
