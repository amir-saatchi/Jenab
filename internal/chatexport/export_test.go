package chatexport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/secret"
)

// key is a fake key with characters JSON escapes, so the fixture holds it
// both raw and escaped.
const key = `sk-fixture-QX7Zp0<&>"/\9f8e7d6c5b4a`

type keyring map[string]string

func (k keyring) Get(n string) (string, error) {
	if v, ok := k[n]; ok {
		return v, nil
	}
	return "", secret.ErrNotFound
}
func (k keyring) Set(n, v string) error { k[n] = v; return nil }
func (k keyring) Delete(n string) error { delete(k, n); return nil }

func redactor() func(string) string {
	return secret.New(keyring{"provider:p": key}).Redactor("provider:p", "provider:missing")
}

var (
	t0      = time.Date(2026, 10, 13, 9, 12, 0, 0, time.UTC)
	project = id.Project("01JPROJECT")
)

func msg(turn int, role chat.Role, model string, parts ...chat.Part) chat.Message {
	return chat.Message{ID: id.Message(fmt.Sprintf("m%06d-%s", turn, role)), Chat: "c1", Turn: turn, Role: role, Model: model,
		CreatedAt: t0.Add(time.Duration(turn) * time.Minute), Parts: parts}
}

func text(s string) chat.Part { return chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: s}} }

func call(callID, name string, args any) chat.Part {
	b, _ := json.Marshal(args)
	return chat.Part{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: callID, Name: name, Args: b}}
}

func result(callID, s string, isErr bool) chat.Part {
	return chat.Part{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: callID, Text: s, IsError: isErr}}
}

func notice(kind chat.NoticeKind, s string) chat.Part {
	return chat.Part{Kind: chat.PartNotice, Notice: &chat.Notice{Kind: kind, Text: s, Details: "details " + key}}
}

// source serves ms in turn batches, checking that no read asks for more
// than Batch turns.
func source(t *testing.T, c chat.Chat, ms []chat.Message) (Source, *int) {
	reads := 0
	last := 0
	for _, m := range ms {
		last = max(last, m.Turn)
	}
	return Source{
		Project: project, ProjectName: "BTC prices", Chat: c, LastTurn: last,
		Notes: chat.SessionNote{Chat: c.ID, Content: "notes " + key, Summary: "summary " + key, Revision: 2},
		Turns: func(_ context.Context, from, to int) ([]chat.Message, error) {
			reads++
			if to-from+1 > Batch {
				t.Errorf("read turns %d to %d, more than %d", from, to, Batch)
			}
			var out []chat.Message
			for _, m := range ms {
				if m.Turn >= from && m.Turn <= to {
					out = append(out, m)
				}
			}
			return out, nil
		},
		Redact: redactor(),
		Now:    time.Date(2026, 11, 1, 14, 5, 0, 0, time.UTC),
		Loc:    time.FixedZone("CET", 3600),
	}, &reads
}

// long puts the key across the 80-character cut of a tool call's
// arguments.
var long = strings.Repeat("x", 50)

// secretChat has the key in every text field the chat stores.
func secretChat() (chat.Chat, []chat.Message) {
	c := chat.Chat{ID: "c1", Kind: chat.KindChat, Title: "Keys " + key, Role: "role " + key, Model: "default", CreatedBy: "user", CreatedAt: t0}
	q := &chat.Question{
		Questions:  []chat.QuestionItem{{Header: "Pick", Question: "Which " + key + "?", Options: []chat.Option{{Label: "a"}, {Label: "b"}}}},
		Answers:    map[string][]string{"Pick": {"other " + key}},
		Note:       "note " + key,
		AnsweredAt: &t0,
	}
	return c, []chat.Message{
		msg(1, chat.RoleUser, "", text("my key is "+key)),
		msg(1, chat.RoleAssistant, "m1",
			chat.Part{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: "thinking " + key}},
			text("you said "+key),
			call("t1", "fetch_page", map[string]string{"a_url": long, "z_auth": key}),
			call("t2", "query", map[string]string{"sql": key}),
			chat.Part{Kind: chat.PartApproval, Approval: &chat.Approval{ID: "a1", Kind: "host", Target: "x.test", Ask: "Allow " + key + "?",
				Details: "d " + key, Options: []chat.ApprovalOption{{Label: "Allow", Grant: chat.GrantOnce}, {Label: "Deny", Grant: chat.GrantDeny}},
				Answer: chat.GrantDeny, Note: "no " + key, By: "user"}},
			chat.Part{Kind: chat.PartQuestion, Question: q},
			chat.Part{Kind: chat.PartImage, Image: &chat.Image{Ref: "chats/c1/a.png", MIME: "image/png", Alt: "alt " + key}},
		),
		msg(1, chat.RoleTool, "", result("t1", "page with "+key, false), result("t2", "error: bad "+key+"\nmore", true),
			notice(chat.NoticeMemoryUpdated, "Memory "+key)),
	}
}

func export(t *testing.T, f Format, s Source) string {
	t.Helper()
	var b bytes.Buffer
	if _, err := Write(context.Background(), &b, f, s); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// No form of the key, nor its start, is in either format.
func TestKeysNeverAppear(t *testing.T) {
	c, ms := secretChat()
	esc, _ := json.Marshal(key)
	plain := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(key) // JSON without HTML escaping
	forms := []string{key, string(esc[1 : len(esc)-1]), plain, key[:14]}
	for _, f := range []Format{FormatMarkdown, FormatJSON} {
		s, _ := source(t, c, ms)
		out := export(t, f, s)
		for _, k := range forms {
			if strings.Contains(out, k) {
				t.Errorf("%s holds %q:\n%s", f, k, out)
			}
		}
		if n := strings.Count(out, "[secret:provider:p]"); n < 10 {
			t.Errorf("%s: %d redactions, want 10 or more:\n%s", f, n, out)
		}
	}
}

func TestMarkdown(t *testing.T) {
	c, ms := secretChat()
	s, _ := source(t, c, ms)
	s.Model = "default (p/m1)"
	out := export(t, FormatMarkdown, s)
	for _, want := range []string{
		"# Keys [secret:provider:p]\n",
		"- Project: BTC prices\n- Model: default (p/m1)\n- Created: 2026-10-13 10:12 +01:00\n- Exported: 2026-11-01 15:05 +01:00\n\n---\n",
		"### You · 2026-10-13 10:13\n\nmy key is [secret:provider:p]\n",
		"### Assistant · m1\n\nyou said [secret:provider:p]\n\n- Tool `fetch_page`",
		"- Tool `query` `{\"sql\":\"[secret:provider:p]\"}` → error: error: bad [secret:provider:p]\n",
		"- Approval: Allow [secret:provider:p]? → Deny (user): no [secret:provider:p]\n",
		"- Question: Which [secret:provider:p]? → other [secret:provider:p]\n- Answered with a message: note [secret:provider:p]\n",
		"![alt \\[secret:provider:p\\]](jenab://01JPROJECT/chats/c1/a.png)\n",
		"- Notice: Memory [secret:provider:p]\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	for _, not := range []string{"thinking", "### Tool", "details", "role "} {
		if strings.Contains(out, not) {
			t.Errorf("holds %q:\n%s", not, out)
		}
	}
	// Arguments are cut to 80 characters, after the key was replaced.
	want := "- Tool `fetch_page` `{\"a_url\":\"" + long + "\",\"z_auth\":\"[secret:…` → 45 B\n"
	if !strings.Contains(out, want) {
		t.Errorf("missing %q in:\n%s", want, out)
	}
}

// Right-to-left text is wrapped so Markdown viewers show it right to left,
// and left-to-right text is left as it is.
func TestMarkdownRTL(t *testing.T) {
	c := chat.Chat{ID: "c1", Kind: chat.KindChat, Title: "قیمت بیت‌کوین", Model: "m1", CreatedAt: t0}
	ms := []chat.Message{
		msg(1, chat.RoleUser, "", text("قیمت امروز بیت‌کوین چقدر است؟")),
		msg(1, chat.RoleAssistant, "m1", call("t1", "fetch_page", map[string]string{"url": "https://api.example.com/btc"}),
			notice(chat.NoticeTaskFinished, "کار تمام شد."), text("**۶۸٬۰۰۰** دلار.\n\n- یک\n- دو")),
		msg(1, chat.RoleTool, "", result("t1", "{\"usd\":68000}", false)),
		msg(2, chat.RoleUser, "", text("Thanks, in English please")),
		msg(2, chat.RoleAssistant, "m1", text("123 Bitcoin is about $68,000.")),
	}
	s, _ := source(t, c, ms)
	out := export(t, FormatMarkdown, s)
	for _, want := range []string{
		"<div dir=\"rtl\">\n\n# قیمت بیت‌کوین\n\n</div>\n",
		"<div dir=\"rtl\">\n\nقیمت امروز بیت‌کوین چقدر است؟\n\n</div>\n",
		"- Notice: <bdi>کار تمام شد.</bdi>\n",
		"<div dir=\"rtl\">\n\n**۶۸٬۰۰۰** دلار.\n\n- یک\n- دو\n\n</div>\n",
		"- Tool `fetch_page` `{\"url\":\"https://api.example.com/btc\"}` → 13 B\n",
		"\nThanks, in English please\n",
		"\n123 Bitcoin is about $68,000.\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Count(out, "<div") != 3 {
		t.Errorf("want 3 wrapped blocks:\n%s", out)
	}
}

func TestJSON(t *testing.T) {
	c, ms := secretChat()
	s, _ := source(t, c, ms)
	out := export(t, FormatJSON, s)
	var got struct {
		Format     string           `json:"format"`
		Version    int              `json:"version"`
		ExportedAt time.Time        `json:"exported_at"`
		Project    projectRef       `json:"project"`
		Chat       chat.Chat        `json:"chat"`
		Notes      chat.SessionNote `json:"notes"`
		Messages   []chat.Message   `json:"messages"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
	if got.Format != FormatName || got.Version != Version || got.Project.ID != project || got.Project.Name != "BTC prices" ||
		!got.ExportedAt.Equal(s.Now) || got.Chat.Role != "role [secret:provider:p]" || got.Notes.Summary != "summary [secret:provider:p]" {
		t.Errorf("header: %+v", got)
	}
	if len(got.Messages) != len(ms) {
		t.Fatalf("%d messages, want %d", len(got.Messages), len(ms))
	}
	// Every part is as stored, thinking included, with only the key
	// replaced.
	for i, m := range got.Messages {
		if len(m.Parts) != len(ms[i].Parts) {
			t.Fatalf("message %d: %d parts, want %d", i, len(m.Parts), len(ms[i].Parts))
		}
		for j, p := range m.Parts {
			if err := p.Validate(); err != nil {
				t.Errorf("message %d part %d: %v", i, j, err)
			}
		}
	}
	if th := got.Messages[1].Parts[0].Thinking; th == nil || th.Text != "thinking [secret:provider:p]" {
		t.Errorf("thinking: %+v", th)
	}
	var args map[string]string
	if err := json.Unmarshal(got.Messages[1].Parts[2].ToolCall.Args, &args); err != nil || args["z_auth"] != "[secret:provider:p]" {
		t.Errorf("args: %v, %v", args, err)
	}
}

// A long chat is read in batches of turns and written whole.
func TestManyTurns(t *testing.T) {
	const turns = 1200
	c := chat.Chat{ID: "c1", Kind: chat.KindChat, Title: "Long", Model: "m1", CreatedAt: t0}
	var ms []chat.Message
	for i := 1; i <= turns; i++ {
		cid := fmt.Sprint("t", i)
		ms = append(ms,
			msg(i, chat.RoleUser, "", text(fmt.Sprintf("سؤال %d", i))),
			msg(i, chat.RoleAssistant, "m1", call(cid, "query", map[string]string{"sql": "SELECT 1"})),
			msg(i, chat.RoleTool, "", result(cid, strings.Repeat("r", 2000), false)),
		)
		// The answer is another assistant message of the same turn: it
		// keeps the turn's heading.
		ms = append(ms, chat.Message{ID: id.Message(fmt.Sprintf("m%06d-z", i)), Chat: "c1", Turn: i, Role: chat.RoleAssistant, Model: "m1",
			CreatedAt: t0, Parts: []chat.Part{text(fmt.Sprint("answer ", i))}})
	}
	for _, f := range []Format{FormatMarkdown, FormatJSON} {
		s, reads := source(t, c, ms)
		dir := t.TempDir()
		res, err := WriteFile(context.Background(), filepath.Join(dir, "long."+string(f)), f, s)
		if err != nil {
			t.Fatal(err)
		}
		if res.Messages != len(ms) || *reads != (turns+Batch-1)/Batch {
			t.Errorf("%s: %d messages in %d reads", f, res.Messages, *reads)
		}
		b, err := os.ReadFile(res.Path)
		if err != nil || int64(len(b)) != res.Size {
			t.Fatalf("%s: %d bytes, result says %d: %v", f, len(b), res.Size, err)
		}
		out := string(b)
		switch f {
		case FormatMarkdown:
			if n := strings.Count(out, "### You"); n != turns {
				t.Errorf("%d user headings", n)
			}
			if n := strings.Count(out, "### Assistant"); n != turns {
				t.Errorf("%d assistant headings, want one per turn", n)
			}
			if n := strings.Count(out, "- Tool `query` `{\"sql\":\"SELECT 1\"}` → 2.0 KB\n"); n != turns {
				t.Errorf("%d tool lines", n)
			}
		case FormatJSON:
			var v struct{ Messages []json.RawMessage }
			if err := json.Unmarshal(b, &v); err != nil || len(v.Messages) != len(ms) {
				t.Errorf("%d messages: %v", len(v.Messages), err)
			}
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 1 {
			t.Errorf("%s: %d files left in the folder", f, len(entries))
		}
	}
}

// A failed export leaves nothing behind, and a file that was there stays.
func TestWriteFileFails(t *testing.T) {
	c, ms := secretChat()
	dir := t.TempDir()
	target := filepath.Join(dir, "chat.md")
	if err := os.WriteFile(target, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := source(t, c, ms)
	boom := errors.New("boom")
	s.Turns = func(context.Context, int, int) ([]chat.Message, error) { return nil, boom }
	if _, err := WriteFile(context.Background(), target, FormatMarkdown, s); !errors.Is(err, boom) || errors.Is(err, ErrWrite) {
		t.Fatalf("got %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "before" {
		t.Errorf("the file was changed: %q", b)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("%d files left", len(entries))
	}
	s, _ = source(t, c, ms)
	if _, err := WriteFile(context.Background(), filepath.Join(dir, "missing", "chat.md"), FormatMarkdown, s); !errors.Is(err, ErrWrite) {
		t.Errorf("a missing folder: %v", err)
	}
	if _, err := WriteFile(context.Background(), target, "pdf", s); !errors.Is(err, ErrFormat) {
		t.Errorf("pdf: %v", err)
	}
	// It replaces a file that is there.
	res, err := WriteFile(context.Background(), target, FormatJSON, s)
	if err != nil || res.Path != target {
		t.Fatalf("%+v, %v", res, err)
	}
	if b, _ := os.ReadFile(target); !bytes.HasPrefix(b, []byte(`{"format":"jenab.chat","version":1,`)) {
		t.Errorf("not replaced: %.60s", b)
	}
}

func TestHelpers(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"a`b", "``a`b``"},
		{"`x", "`` `x ``"},
		{"plain", "`plain`"},
	} {
		if got := code(c.in); got != c.want {
			t.Errorf("code(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := cut("abcdef", 3); got != "abc…" {
		t.Errorf("cut: %q", got)
	}
	if got := cut("سلام دنیا", 5); got != "سلام…" {
		t.Errorf("cut rtl: %q", got)
	}
	for s, want := range map[string]bool{"۱۲۳ سلام": true, "123 hello": false, "«سلام» hi": true, "": false, "- یک": true} {
		if rtl(s) != want {
			t.Errorf("rtl(%q) = %v", s, !want)
		}
	}
}
