package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/docx"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Documents (TASK-021): a Word file is read through its text, Markdown
// kept in cache/text/<sha256>-v<n>.md. The text is made at upload, and
// again when the cache has expired or a better extractor has a new
// version. The ref stays the document's own key: the cache key is never
// shown to the model, and offsets are in bytes of the text.

// TextKey is where the text of the content with hash sha is cached.
func TextKey(sha string) string { return fmt.Sprintf("cache/text/%s-v%d.md", sha, docx.Version) }

// isDocument reports whether objects of type m are read through their
// text. An OLE file is one only to say why it can't be read.
func isDocument(m string) bool { return docx.IsWord(m) || baseType(m) == docx.MIMEOLE }

// typeName is a type as a header names it.
func typeName(m string) string {
	if docx.IsWord(m) {
		return "Word document"
	}
	return baseType(m)
}

// DocumentText returns the text of document o, whose content is body:
// the cached text, or text made now and cached. A file that can't be
// read returns a *docx.Error. If the text can't be cached, as in a
// project open read-only, it is returned anyway.
func DocumentText(ctx context.Context, db *store.ProjectDB, pr limit.Priority, src id.Source, o store.Object, body io.ReadSeeker) (store.Object, io.ReadSeekCloser, error) {
	key := TextKey(o.SHA256)
	t, rc, err := db.OpenObject(ctx, key)
	if err == nil && !expired(t) {
		return t, rc, nil
	}
	if rc != nil {
		rc.Close()
	}
	md, err := docx.Extract(ctx, docx.ReaderAt(body), o.Size)
	if err != nil {
		return store.Object{}, nil, err
	}
	meta, _ := json.Marshal(map[string]string{"title": "Text of " + o.Key})
	t, err = db.PutObject(ctx, pr, src, key, strings.NewReader(md), store.PutOptions{Source: "tool", Metadata: meta})
	if err != nil {
		if ctx.Err() != nil {
			return store.Object{}, nil, ctx.Err()
		}
		return store.Object{Key: key, MIME: "text/markdown; charset=utf-8", Size: int64(len(md))}, nopCloser{strings.NewReader(md)}, nil
	}
	if _, rc, err = db.OpenObject(ctx, key); err != nil {
		return store.Object{}, nil, err
	}
	return t, rc, nil
}

type nopCloser struct{ io.ReadSeeker }

func (nopCloser) Close() error { return nil }

// documentText opens the text of document o for a tool. A file that
// can't be read is an *Error that names it.
func documentText(ctx context.Context, env *Env, o store.Object, body io.ReadSeeker) (store.Object, io.ReadSeekCloser, error) {
	t, rc, err := DocumentText(ctx, env.Project.DB, env.Priority, env.Source, o, body)
	var de *docx.Error
	if errors.As(err, &de) {
		return store.Object{}, nil, Errorf("could not read %s: %s", o.Key, de.Msg)
	}
	return t, rc, err
}

// refKey is the key of a ref, which may also be given as the object's
// jenab:// link, as the user's message has it.
func refKey(env *Env, ref string) (string, error) {
	if !strings.HasPrefix(ref, "jenab://") {
		return ref, nil
	}
	p, key, err := bucket.ParseLink(ref)
	if err != nil {
		return "", Errorf("%s is not a link to a file", ref)
	}
	if p != env.Project.ID {
		return "", Errorf("%s is a link to another project, which can't be read from here", ref)
	}
	return key, nil
}
