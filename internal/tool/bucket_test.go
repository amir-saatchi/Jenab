package tool

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/bucket"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/store"
)

func put(t *testing.T, env *Env, key, content string, o store.PutOptions) store.Object {
	t.Helper()
	if o.Source == "" {
		o.Source = "upload"
	}
	obj, err := env.Project.DB.PutObject(context.Background(), limit.Interactive, "user", key, strings.NewReader(content), o)
	if err != nil {
		t.Fatal(err)
	}
	return obj
}

func TestBucketPutAndDelete(t *testing.T) {
	env := testEnv(t)
	ctx := context.Background()
	got := mustText(t, env, "bucket_put", `{"key": "notes/plan.md", "content": "# Plan\nBuy low."}`)
	contains(t, got, "Stored notes/plan.md: version 1, text/markdown", "Link: jenab://"+string(env.Project.ID)+"/notes/plan.md")
	o, err := env.Project.DB.HeadObject(ctx, "notes/plan.md")
	if err != nil || o.Source != "agent" || o.CreatedBy != env.Source {
		t.Errorf("head %+v, %v", o, err)
	}
	contains(t, mustText(t, env, "bucket_put", `{"key": "notes/plan.md", "content": "# Plan\nSell high."}`), "version 2")
	contains(t, mustText(t, env, "bucket_put", `{"key": "notes/empty.txt", "content": ""}`), "Stored notes/empty.txt")

	// from_ref keeps a cached page under a lasting key, with its source.
	put(t, env, "cache/pages/example.com/ab.txt", "page text", store.PutOptions{Source: "http", SourceURL: "https://example.com/a", Metadata: []byte(`{"title":"A"}`)})
	contains(t, mustText(t, env, "bucket_put", `{"key": "sources/a.txt", "from_ref": "cache/pages/example.com/ab.txt"}`), "Stored sources/a.txt")
	kept, err := env.Project.DB.HeadObject(ctx, "sources/a.txt")
	if err != nil || kept.SourceURL != "https://example.com/a" || string(kept.Metadata) != `{"title":"A"}` || !kept.Expires.IsZero() {
		t.Errorf("kept %+v, %v", kept, err)
	}

	contains(t, toolError(t, env, "bucket_put", `{"key": "a.txt"}`), "give either content or from_ref")
	contains(t, toolError(t, env, "bucket_put", `{"key": "a.txt", "content": "x", "from_ref": "b"}`), "give either content or from_ref")
	contains(t, toolError(t, env, "bucket_put", `{"key": "../a.txt", "content": "x"}`), "bad key")
	contains(t, toolError(t, env, "bucket_put", `{"key": "a b.txt", "content": "x"}`), "bad key")
	contains(t, toolError(t, env, "bucket_put", `{"key": "a.txt", "from_ref": "nope.txt"}`), "no object at nope.txt")

	contains(t, mustText(t, env, "bucket_delete", `{"key": "notes/plan.md"}`), "Deleted notes/plan.md.")
	contains(t, toolError(t, env, "bucket_delete", `{"key": "notes/plan.md"}`), "no object at notes/plan.md")
	contains(t, toolError(t, env, "bucket_delete", `{"key": "/abs"}`), "bad key")
}

func TestBucketList(t *testing.T) {
	env := testEnv(t)
	for i := range 3 {
		put(t, env, fmt.Sprintf("reports/r%d.csv", i), "a,b\n1,2\n", store.PutOptions{})
	}
	put(t, env, "reports/2026/q1.md", "# Q1", store.PutOptions{})
	put(t, env, "cache/pages/x.org/1.txt", "x", store.PutOptions{Source: "http"})

	got := mustText(t, env, "bucket_list", `{}`)
	contains(t, got, `Under "":`, "- cache/ (folder)", "- reports/ (folder)")
	got = mustText(t, env, "bucket_list", `{"prefix": "reports/"}`)
	contains(t, got, "- reports/2026/ (folder)", "- reports/r0.csv · text/csv · 8 B · ", "- reports/r2.csv")
	if strings.Contains(got, "expires") || strings.Contains(got, "more:") {
		t.Errorf("list:\n%s", got)
	}
	contains(t, mustText(t, env, "bucket_list", `{"prefix": "cache/pages/x.org/"}`), " · expires ")
	contains(t, mustText(t, env, "bucket_list", `{"prefix": "nothing/"}`), `No objects under "nothing/".`)
	toolError(t, env, "bucket_list", `{"prefix": "../"}`)
}

func TestBucketListPages(t *testing.T) {
	env := testEnv(t)
	for i := range maxList + 5 {
		put(t, env, fmt.Sprintf("many/%03d.txt", i), fmt.Sprint(i), store.PutOptions{})
	}
	got := mustText(t, env, "bucket_list", `{"prefix": "many/"}`)
	contains(t, got, "many/099.txt", `[more: bucket_list(prefix: "many/", after: "many/099.txt")]`)
	if strings.Contains(got, "many/100.txt") {
		t.Error("first page too long")
	}
	got = mustText(t, env, "bucket_list", `{"prefix": "many/", "after": "many/099.txt"}`)
	contains(t, got, "many/100.txt", "many/104.txt")
	if strings.Contains(got, "many/099.txt") || strings.Contains(got, "more:") {
		t.Errorf("second page:\n%s", got)
	}
}

func TestBucketRead(t *testing.T) {
	env := testEnv(t)
	put(t, env, "notes/short.md", "Short note.", store.PutOptions{})
	r, err := call(t, env, "bucket_read", `{"key": "notes/short.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "[object notes/short.md — text/markdown — 3 tokens — ref: notes/short.md]\nShort note." {
		t.Errorf("short: %q", r.Text)
	}
	long := strings.Repeat("A long line of notes.\n", 20)
	put(t, env, "notes/long.txt", long, store.PutOptions{})
	got := mustText(t, env, "bucket_read", `{"key": "notes/long.txt"}`)
	contains(t, got, "[object notes/long.txt — text/plain — 110 tokens, showing first 22 — ref: notes/long.txt]",
		"the next offset is 88")

	var img bytes.Buffer
	png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	put(t, env, "images/a.png", img.String(), store.PutOptions{})
	r, err = call(t, env, "bucket_read", `{"key": "images/a.png"}`)
	if err != nil || len(r.Images) != 1 || r.Images[0].Ref != "images/a.png" || r.Images[0].MIME != "image/png" {
		t.Errorf("image: %+v, %v", r, err)
	}
	contains(t, r.Text, "[image images/a.png — image/png — ")

	put(t, env, "files/a.pdf", "%PDF-1.7 binary", store.PutOptions{})
	contains(t, toolError(t, env, "bucket_read", `{"key": "files/a.pdf"}`), "files/a.pdf is application/pdf", "only text and images")
	contains(t, toolError(t, env, "bucket_read", `{"key": "files/none.txt"}`), "no object at files/none.txt")
}

func TestReadRef(t *testing.T) {
	env := testEnv(t)
	text := strings.Repeat("0123456789\n", 30) // 330 bytes
	put(t, env, "cache/tool/x/1", text, store.PutOptions{Source: "tool"})
	got := mustText(t, env, "read_ref", `{"ref": "cache/tool/x/1"}`)
	contains(t, got, "[ref — 83 tokens, showing first 25 — ref: cache/tool/x/1]\n0123456789\n", "the next offset is 99]")
	got = mustText(t, env, "read_ref", `{"ref": "cache/tool/x/1", "offset": 99}`)
	contains(t, got, "[ref — 83 tokens, showing 25 from offset 99 — ref: cache/tool/x/1]\n0123456789", "the next offset is 198]")
	got = mustText(t, env, "read_ref", `{"ref": "cache/tool/x/1", "offset": 297}`)
	if strings.Contains(got, "next offset") || !strings.HasSuffix(got, "0123456789\n0123456789\n0123456789") {
		t.Errorf("last part:\n%s", got)
	}
	contains(t, mustText(t, env, "read_ref", `{"ref": "cache/tool/x/1", "offset": 330}`), "showing 0 from offset 330")
	contains(t, toolError(t, env, "read_ref", `{"ref": "cache/tool/x/1", "offset": 331}`), "offset 331 is outside cache/tool/x/1, which has 330 bytes")
	toolError(t, env, "read_ref", `{"ref": "cache/tool/x/1", "offset": -1}`)
	contains(t, toolError(t, env, "read_ref", `{"ref": "cache/none"}`), "no object at cache/none")

	// An offset inside a character starts at the next one.
	put(t, env, "fa.txt", "سلام دنیا", store.PutOptions{})
	got = mustText(t, env, "read_ref", `{"ref": "fa.txt", "offset": 1}`)
	contains(t, got, "from offset 2 — ref: fa.txt]\nلام دنیا")

	// An expired object says where it came from.
	put(t, env, "cache/pages/example.com/old.txt", "old page", store.PutOptions{Source: "http", SourceURL: "https://example.com/old"})
	_, err := store.Do(context.Background(), env.Project.DB.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		_, err := tx.Exec(`UPDATE _jenab_objects SET updated_at = '2020-01-01T00:00:00.000Z' WHERE key = 'cache/pages/example.com/old.txt'`)
		return struct{}{}, err
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string]string{
		"read_ref":   `{"ref": "cache/pages/example.com/old.txt"}`,
		"search_ref": `{"ref": "cache/pages/example.com/old.txt", "query": "old"}`,
	} {
		r, err := call(t, env, name, args)
		if err != nil || r.Text != "expired: cache/pages/example.com/old.txt came from https://example.com/old and has expired; fetch it again." {
			t.Errorf("%s expired: %q, %v", name, r.Text, err)
		}
	}
	var img bytes.Buffer
	png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	put(t, env, "i.png", img.String(), store.PutOptions{})
	contains(t, toolError(t, env, "read_ref", `{"ref": "i.png"}`), "i.png is image/png, not text")
	contains(t, toolError(t, env, "search_ref", `{"ref": "i.png", "query": "x"}`), "i.png is image/png, not text")
}

func TestSearchRef(t *testing.T) {
	env := testEnv(t)
	page := "# Prices\n\nBitcoin rose to 60,000 EUR today.\n\n| Coin | Price |\n|---|---|\n| Ethereum | 2,500 EUR |\n\n" +
		"The Bitcoinpreis fell later.\nقیمت بيت" + zwnj + "كوين بالا رفت.\n"
	put(t, env, "cache/pages/x/p.txt", page, store.PutOptions{Source: "http"})
	got := mustText(t, env, "search_ref", `{"ref": "cache/pages/x/p.txt", "query": "bitcoin eur"}`)
	off := strings.Index(page, "Bitcoin rose")
	contains(t, got, "1 lines of cache/pages/x/p.txt match:", fmt.Sprintf("- offset %d · line 3: Bitcoin rose to 60,000 EUR today.", off))
	// Persian: ی and ي, ک and ك, the ZWNJ form.
	got = mustText(t, env, "search_ref", `{"ref": "cache/pages/x/p.txt", "query": "بیت`+zwnj+`کوین"}`)
	contains(t, got, "1 lines", "line 10: قیمت")
	// Words inside words don't match; the start of a word does.
	contains(t, mustText(t, env, "search_ref", `{"ref": "cache/pages/x/p.txt", "query": "preis"}`), "No lines of cache/pages/x/p.txt match.")
	contains(t, mustText(t, env, "search_ref", `{"ref": "cache/pages/x/p.txt", "query": "ether"}`), "line 7: | Ethereum | 2,500 EUR |")

	// read_ref from a hit's offset starts at its line.
	got = mustText(t, env, "read_ref", fmt.Sprintf(`{"ref": "cache/pages/x/p.txt", "offset": %d}`, off))
	if _, body, _ := strings.Cut(got, "\n"); !strings.HasPrefix(body, "Bitcoin rose") {
		t.Errorf("read from the hit:\n%s", got)
	}

	var lines strings.Builder
	for i := range 30 {
		fmt.Fprintf(&lines, "row %d price\n", i)
	}
	put(t, env, "many.txt", lines.String(), store.PutOptions{})
	got = mustText(t, env, "search_ref", `{"ref": "many.txt", "query": "price"}`)
	contains(t, got, "30 lines of many.txt match; the first 20:", "line 20: row 19 price")
	if strings.Contains(got, "row 20 ") {
		t.Errorf("more than 20 hits:\n%s", got)
	}
	contains(t, toolError(t, env, "search_ref", fmt.Sprintf(`{"ref": "many.txt", "query": %q}`, strings.Repeat("a ", 33))), "32 words at most")
	contains(t, toolError(t, env, "search_ref", `{"ref": "none.txt", "query": "a"}`), "no object at none.txt")
}

func TestObjectError(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string // "" = not a *Error
	}{
		{fmt.Errorf("get: %w", store.ErrNotFound), "no object at k"},
		{fmt.Errorf("open: %w", bucket.ErrNotFound), "no object at k"},
		{fmt.Errorf("%w: a//b", bucket.ErrBadKey), "bad key: a//b"},
		{bucket.ErrTooLarge, "the file is too large"},
		{fmt.Errorf("put: %w", store.ErrReadOnly), "open read-only after damage"},
		{errors.New("disk full"), ""},
	} {
		err := objectError(tc.err, "k")
		var te *Error
		switch {
		case tc.want == "" && (errors.As(err, &te) || err != tc.err):
			t.Errorf("%v: got %v, want it as it is", tc.err, err)
		case tc.want != "" && (!errors.As(err, &te) || !strings.Contains(te.Msg, tc.want) || !errors.Is(err, tc.err)):
			t.Errorf("%v: got %v, want %q", tc.err, err, tc.want)
		}
	}
}
