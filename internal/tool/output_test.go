package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/chat"
)

func TestCut(t *testing.T) {
	for _, tc := range []struct {
		s    string
		max  int
		want string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"line one\nline two is long", 12, "line one\n"},
		{"word word word word", 12, "word word "},
		{"x\nabcdefghijklmnop", 12, "x\nabcdefghij"}, // the break is too early
		{"aaaaaaaaaaaaaaaaaaaa", 12, "aaaaaaaaaaaa"},
		{"ééééééé", 5, "éé"}, // never inside a character
	} {
		if got := cut(tc.s, tc.max); got != tc.want {
			t.Errorf("cut(%q, %d) = %q, want %q", tc.s, tc.max, got, tc.want)
		}
	}
}

func TestCount(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 1500: "1,500", 1234567: "1,234,567", -2500: "-2,500"} {
		if got := count(n); got != want {
			t.Errorf("count(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestPreviewAndStub(t *testing.T) {
	text := strings.Repeat("The price rose today.\n", 40) // 880 bytes, 220 tokens
	label := `page "Laptop X review" — example.com`
	ref := "cache/pages/example.com/ab12f9.txt"
	p := preview(label, ref, text, 25)
	head, rest, _ := strings.Cut(p, "\n")
	if want := `[page "Laptop X review" — example.com — 220 tokens, showing first 22 — ref: cache/pages/example.com/ab12f9.txt]`; head != want {
		t.Errorf("header\n%s\nwant\n%s", head, want)
	}
	if !strings.HasSuffix(rest, "\n[use read_ref(ref, offset) or search_ref(ref, query) for more; the next offset is 88]") {
		t.Errorf("footer:\n%s", rest)
	}
	if body := strings.TrimSuffix(rest, "\n"+more(88)); body+"\n" != text[:88] {
		t.Errorf("body %q", body)
	}
	if got, want := Stub(chat.ToolResult{Text: p, Ref: ref}), `[page "Laptop X review" — example.com — ref: cache/pages/example.com/ab12f9.txt]`; got != want {
		t.Errorf("stub %s, want %s", got, want)
	}
	whole := preview(label, ref, "Short page.", 25)
	if whole != "[page \"Laptop X review\" — example.com — 3 tokens — ref: "+ref+"]\nShort page." {
		t.Errorf("whole: %q", whole)
	}
	if got := Stub(chat.ToolResult{Text: whole, Ref: ref}); got != `[page "Laptop X review" — example.com — ref: `+ref+"]" {
		t.Errorf("stub of whole %s", got)
	}
	// A title with " — " and "tokens" in it keeps its words.
	odd := preview(`page "A — 5 tokens" — x.org`, "r", text, 25)
	if got := Stub(chat.ToolResult{Text: odd, Ref: "r"}); got != `[page "A — 5 tokens" — x.org — ref: r]` {
		t.Errorf("odd stub %s", got)
	}
	// A header without a size keeps every segment.
	if got := Stub(chat.ToolResult{Text: `[page — x.org — ref: k]
body`, Ref: "k"}); got != "[page — x.org — ref: k]" {
		t.Errorf("no size: %s", got)
	}
	if got := Stub(chat.ToolResult{Text: "small result"}); got != "small result" {
		t.Errorf("no ref: %s", got)
	}
	if got := Stub(chat.ToolResult{Text: "not a header", Ref: "k"}); got != "[ref: k]" {
		t.Errorf("unknown: %s", got)
	}
}

func TestOutput(t *testing.T) {
	env := testEnv(t)
	ctx := context.Background()
	c := Call{ID: "c7", Env: env}

	got, err := Output(ctx, c, "query", 0, Result{Text: "small"}, nil)
	if err != nil || got != (chat.ToolResult{CallID: "c7", Text: "small"}) {
		t.Errorf("small: %+v, %v", got, err)
	}
	got, err = Output(ctx, c, "fetch_page", 0, Result{Text: "own preview", Ref: "cache/pages/x/1.txt"}, nil)
	if err != nil || got.Ref != "cache/pages/x/1.txt" || got.Text != "own preview" {
		t.Errorf("own ref: %+v, %v", got, err)
	}
	got, err = Output(ctx, c, "query", 0, Result{}, Errorf("no table %s", "prices"))
	if err != nil || !got.IsError || got.Text != "no table prices" {
		t.Errorf("tool error: %+v, %v", got, err)
	}
	boom := errors.New("disk full")
	if _, err := Output(ctx, c, "query", 0, Result{}, boom); err != boom {
		t.Errorf("system error: %v", err)
	}

	big := strings.Repeat("row ", 100) // 400 bytes, 100 tokens
	got, err = Output(ctx, c, "query", 3, Result{Text: big}, nil)
	if err != nil {
		t.Fatal(err)
	}
	key := "cache/tool/" + string(env.Chat) + "/" + string(env.Message) + "-3"
	if got.Ref != key || got.IsError {
		t.Errorf("big: %+v", got)
	}
	contains(t, got.Text, "[output of query — 100 tokens, showing first 25 — ref: "+key+"]", "the next offset is 100")
	o, rc, err := env.Project.DB.OpenObject(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != big || o.Source != "tool" || o.CreatedBy != env.Source {
		t.Errorf("stored %q from %s by %s", b, o.Source, o.CreatedBy)
	}
	if Stub(got) != "[output of query — ref: "+key+"]" {
		t.Errorf("stub %s", Stub(got))
	}
}

func TestReadOnToTheEnd(t *testing.T) {
	// Reading on from each next offset gives back the text exactly, also
	// with characters of 2, 3 and 4 bytes.
	env := testEnv(t)
	text := strings.Repeat("قیمت بیت"+zwnj+"کوین 🚀 rose; Straße ok.\n", 30)
	args, _ := json.Marshal(map[string]string{"key": "notes/a.md", "content": text})
	if _, err := call(t, env, "bucket_put", string(args)); err != nil {
		t.Fatal(err)
	}
	nextOffset := regexp.MustCompile(`\n\[use read_ref\(ref, offset\) or search_ref\(ref, query\) for more; the next offset is (\d+)\]$`)
	var got strings.Builder
	offset := 0
	for range 100 {
		r, err := call(t, env, "read_ref", fmt.Sprintf(`{"ref": "notes/a.md", "offset": %d}`, offset))
		if err != nil {
			t.Fatal(err)
		}
		if r.Ref != "notes/a.md" {
			t.Errorf("ref %q", r.Ref)
		}
		_, body, _ := strings.Cut(r.Text, "\n")
		if !utf8.ValidString(body) || strings.ContainsRune(body, utf8.RuneError) {
			t.Fatalf("broken text at offset %d", offset)
		}
		m := nextOffset.FindStringSubmatchIndex(body)
		if m == nil {
			got.WriteString(body)
			break
		}
		shown := body[:m[0]]
		next, _ := strconv.Atoi(body[m[2]:m[3]])
		got.WriteString(shown)
		// The preview drops line breaks at its end; put them back.
		got.WriteString(strings.Repeat("\n", next-offset-len(shown)))
		offset = next
	}
	if strings.TrimRight(got.String(), "\n") != strings.TrimRight(text, "\n") {
		t.Errorf("read back differs:\n%q\nwant\n%q", got.String(), text)
	}
}
