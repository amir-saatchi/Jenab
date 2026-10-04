package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Bucket and ref tools (SPEC 8.1): bucket_list, bucket_read, bucket_put,
// bucket_delete, read_ref and search_ref. A ref is a bucket key.

// objectError turns the errors the model can fix into an *Error.
func objectError(err error, key string) error {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, bucket.ErrNotFound):
		return &Error{Msg: "no object at " + key, Err: err}
	case errors.Is(err, bucket.ErrBadKey), errors.Is(err, bucket.ErrTooLarge):
		return &Error{Msg: strings.TrimPrefix(err.Error(), "bucket: "), Err: err}
	case errors.Is(err, store.ErrReadOnly):
		return &Error{Msg: "the project is open read-only after damage, so nothing can be changed; tell the user", Err: err}
	}
	return err
}

// baseType is m without its parameters, such as the charset.
func baseType(m string) string {
	if b, _, err := mime.ParseMediaType(m); err == nil {
		return b
	}
	return m
}

// isText reports whether objects of type m are read as text.
func isText(m string) bool {
	base := baseType(m)
	return strings.HasPrefix(base, "text/") || base == "application/json" || base == "application/xml" ||
		strings.HasSuffix(base, "+json") || strings.HasSuffix(base, "+xml")
}

// isImage reports whether objects of type m can be shown to a model.
func isImage(m string) bool {
	switch baseType(m) {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}

func expired(o store.Object) bool { return !o.Expires.IsZero() && time.Now().After(o.Expires) }

// readText reads a text object from offset as a preview under a header
// with label, so a stub can be made from it later.
func readText(ctx context.Context, env *Env, key string, offset int, label func(store.Object) string) (Result, error) {
	o, rc, err := env.Project.DB.OpenObject(ctx, key)
	if err != nil {
		return Result{}, objectError(err, key)
	}
	defer rc.Close()
	if expired(o) {
		from := o.SourceURL
		if from == "" {
			from = "an unknown source"
		}
		return Result{Text: fmt.Sprintf("expired: %s came from %s and has expired; fetch it again.", key, from)}, nil
	}
	if !isText(o.MIME) {
		return Result{}, Errorf("%s is %s, not text", key, baseType(o.MIME))
	}
	if offset < 0 || int64(offset) > o.Size {
		return Result{}, Errorf("offset %d is outside %s, which has %s bytes", offset, key, count(int(o.Size)))
	}
	if _, err := rc.Seek(int64(offset), io.SeekStart); err != nil {
		return Result{}, err
	}
	max := env.PreviewTokens * 4
	buf, err := io.ReadAll(io.LimitReader(rc, int64(max+2*utf8.UTFMax)))
	if err != nil {
		return Result{}, err
	}
	// An offset inside a character starts at the next one.
	for len(buf) > 0 && !utf8.RuneStart(buf[0]) {
		buf, offset = buf[1:], offset+1
	}
	shown := string(buf)
	if len(shown) > max { // buf holds more than max when there is more
		shown = cut(shown, max)
	}
	total := count(int((o.Size + 3) / 4))
	end := offset + len(shown)
	var size string
	switch {
	case offset == 0 && int64(end) >= o.Size:
		size = total + " tokens"
	case offset == 0:
		size = fmt.Sprintf("%s tokens, showing first %s", total, count(tokens(shown)))
	default:
		size = fmt.Sprintf("%s tokens, showing %s from offset %d", total, count(tokens(shown)), offset)
	}
	text := strings.ToValidUTF8(strings.TrimRight(shown, "\n"), string(utf8.RuneError))
	out := fmt.Sprintf("[%s — %s — ref: %s]\n%s", label(o), size, key, text)
	if int64(end) < o.Size {
		out += "\n" + more(end)
	}
	return Result{Text: out, Ref: key}, nil
}

type readRefArgs struct {
	Ref    string `json:"ref"`
	Offset int    `json:"offset"`
}

func readRef() Tool {
	return Func(Spec{
		Name: "read_ref",
		Description: "Read a stored page or tool output (its ref) from a byte offset. " +
			"A preview ends with the next offset to read from.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"ref": {"type": "string", "minLength": 1},
				"offset": {"type": "integer", "minimum": 0, "default": 0}
			},
			"required": ["ref"],
			"additionalProperties": false
		}`),
		Effects: ReadsDB | Untrusted,
	}, func(ctx context.Context, env *Env, a readRefArgs) (Result, error) {
		return readText(ctx, env, a.Ref, a.Offset, func(store.Object) string { return "ref" })
	})
}

type searchRefArgs struct {
	Ref   string `json:"ref"`
	Query string `json:"query"`
}

// Limits of search_ref.
const (
	maxRefHits   = 20
	maxRefSearch = 20 << 20 // bytes searched
)

func searchRef() Tool {
	return Func(Spec{
		Name: "search_ref",
		Description: "Find the lines of a stored page or tool output (its ref) that have every word of the query, " +
			"at the start of words. Returns each line's offset for read_ref.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"ref": {"type": "string", "minLength": 1},
				"query": {"type": "string", "minLength": 1}
			},
			"required": ["ref", "query"],
			"additionalProperties": false
		}`),
		Effects: ReadsDB | Untrusted,
	}, func(ctx context.Context, env *Env, a searchRefArgs) (Result, error) {
		o, rc, err := env.Project.DB.OpenObject(ctx, a.Ref)
		if err != nil {
			return Result{}, objectError(err, a.Ref)
		}
		defer rc.Close()
		if expired(o) {
			return readText(ctx, env, a.Ref, 0, nil) // says it expired
		}
		if !isText(o.MIME) {
			return Result{}, Errorf("%s is %s, not text", a.Ref, baseType(o.MIME))
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxRefSearch))
		if err != nil {
			return Result{}, err
		}
		hits, total, err := store.SearchText(string(b), a.Query, maxRefHits)
		if errors.Is(err, store.ErrBadQuery) {
			return Result{}, Errorf("%d words at most", store.MaxSearchTerms)
		}
		if err != nil {
			return Result{}, err
		}
		var out strings.Builder
		if total == 0 {
			fmt.Fprintf(&out, "No lines of %s match.", a.Ref)
		} else {
			fmt.Fprintf(&out, "%d lines of %s match", total, a.Ref)
			if total > len(hits) {
				fmt.Fprintf(&out, "; the first %d", len(hits))
			}
			out.WriteString(":")
		}
		for _, h := range hits {
			fmt.Fprintf(&out, "\n- offset %d · line %d: %s", h.Offset, h.Line, h.Snippet)
		}
		if o.Size > maxRefSearch {
			fmt.Fprintf(&out, "\n[only the first %s MB were searched]", count(maxRefSearch>>20))
		}
		return Result{Text: out.String()}, nil
	})
}

type bucketListArgs struct {
	Prefix string `json:"prefix"`
	After  string `json:"after"`
}

// maxList bounds the entries of one bucket_list call.
const maxList = 100

func bucketList() Tool {
	return Func(Spec{
		Name: "bucket_list",
		Description: "List the bucket's objects and folders under a prefix such as \"reports/\". " +
			fmt.Sprintf("At most %d entries a call; pass after to get the next ones.", maxList),
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"prefix": {"type": "string", "default": ""},
				"after": {"type": "string", "description": "The last key or folder of the previous call."}
			},
			"additionalProperties": false
		}`),
		Effects: ReadsDB,
	}, func(ctx context.Context, env *Env, a bucketListArgs) (Result, error) {
		l, err := env.Project.DB.ListObjects(ctx, a.Prefix, a.After, maxList)
		if err != nil {
			return Result{}, objectError(err, a.Prefix)
		}
		var b strings.Builder
		if len(l.Objects)+len(l.Folders) == 0 {
			fmt.Fprintf(&b, "No objects under %q.", a.Prefix)
			return Result{Text: b.String()}, nil
		}
		fmt.Fprintf(&b, "Under %q:", a.Prefix)
		for _, f := range l.Folders {
			fmt.Fprintf(&b, "\n- %s (folder)", f)
		}
		for _, o := range l.Objects {
			fmt.Fprintf(&b, "\n- %s · %s · %s · %s", o.Key, baseType(o.MIME), bytesize(o.Size), when(o.UpdatedAt))
			if !o.Expires.IsZero() {
				fmt.Fprintf(&b, " · expires %s", o.Expires.Local().Format("2006-01-02"))
			}
		}
		if l.Next != "" {
			fmt.Fprintf(&b, "\n[more: bucket_list(prefix: %q, after: %q)]", a.Prefix, l.Next)
		}
		return Result{Text: b.String()}, nil
	})
}

// bytesize writes n as 980 B, 12.5 KB or 3.1 MB.
func bytesize(n int64) string {
	switch {
	case n < 1<<10:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}

type keyArgs struct {
	Key string `json:"key"`
}

func bucketRead() Tool {
	return Func(Spec{
		Name: "bucket_read",
		Description: "Read an object from the bucket: text as a preview with its ref, " +
			"images for models that can see them.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {"key": {"type": "string", "minLength": 1}},
			"required": ["key"],
			"additionalProperties": false
		}`),
		Effects: ReadsDB | Untrusted,
	}, func(ctx context.Context, env *Env, a keyArgs) (Result, error) {
		o, err := env.Project.DB.HeadObject(ctx, a.Key)
		if err != nil {
			return Result{}, objectError(err, a.Key)
		}
		if isImage(o.MIME) {
			return Result{
				Text:   fmt.Sprintf("[image %s — %s — %s]", o.Key, baseType(o.MIME), bytesize(o.Size)),
				Images: []chat.Image{{Ref: o.Key, MIME: o.MIME}},
			}, nil
		}
		if !isText(o.MIME) {
			return Result{}, Errorf("%s is %s (%s); only text and images can be read", o.Key, baseType(o.MIME), bytesize(o.Size))
		}
		return readText(ctx, env, a.Key, 0, func(o store.Object) string { return "object " + o.Key + " — " + baseType(o.MIME) })
	})
}

type bucketPutArgs struct {
	Key     string  `json:"key"`
	Content *string `json:"content"`
	FromRef string  `json:"from_ref"`
}

func bucketPut() Tool {
	return Func(Spec{
		Name: "bucket_put",
		Description: "Store text under a key in the bucket, or keep a stored page or tool output (from_ref) under a lasting key. " +
			"Give content or from_ref. Keys are parts of letters, digits, '.', '_' and '-' joined by '/'. " +
			"Objects under cache/ expire.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"key": {"type": "string", "minLength": 1},
				"content": {"type": "string"},
				"from_ref": {"type": "string", "minLength": 1}
			},
			"required": ["key"],
			"additionalProperties": false
		}`),
		Effects: Bucket,
	}, func(ctx context.Context, env *Env, a bucketPutArgs) (Result, error) {
		if (a.Content == nil) == (a.FromRef == "") {
			return Result{}, Errorf("give either content or from_ref")
		}
		opt := store.PutOptions{Source: "agent"}
		var r io.Reader
		if a.Content != nil {
			r = strings.NewReader(*a.Content)
		} else {
			from, rc, err := env.Project.DB.OpenObject(ctx, a.FromRef)
			if err != nil {
				return Result{}, objectError(err, a.FromRef)
			}
			defer rc.Close()
			r, opt.SourceURL, opt.Metadata = rc, from.SourceURL, from.Metadata
		}
		o, err := env.Project.DB.PutObject(ctx, env.Priority, env.Source, a.Key, r, opt)
		if err != nil {
			return Result{}, objectError(err, a.Key)
		}
		return Result{Text: fmt.Sprintf("Stored %s: version %d, %s, %s. Link: %s",
			o.Key, o.Version, baseType(o.MIME), bytesize(o.Size), bucket.Link(env.Project.ID, o.Key))}, nil
	})
}

func bucketDelete() Tool {
	return Func(Spec{
		Name:        "bucket_delete",
		Description: "Delete a key from the bucket. Its versions are kept for a while, so the delete can be undone.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {"key": {"type": "string", "minLength": 1}},
			"required": ["key"],
			"additionalProperties": false
		}`),
		Effects: Bucket,
	}, func(ctx context.Context, env *Env, a keyArgs) (Result, error) {
		if err := env.Project.DB.DeleteObject(ctx, env.Priority, env.Source, a.Key); err != nil {
			return Result{}, objectError(err, a.Key)
		}
		return Result{Text: "Deleted " + a.Key + "."}, nil
	})
}
