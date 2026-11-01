package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/chatexport"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
)

// A chat exports as Markdown and JSON from its stored messages, with the
// provider's key replaced even before any turn read it.
func TestChatExport(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	op, err := e.svc.Project.Create(ctx, "قیمت‌ها")
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.svc.Chat.Create(ctx, op.ID, "بیت‌کوین", "")
	if err != nil {
		t.Fatal(err)
	}
	proj, err := e.pm.Open(ctx, op.ID)
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{"url": "https://x.test/?key=" + testKey})
	for _, m := range []chat.Message{
		{Chat: c.ID, Turn: 1, Role: chat.RoleUser, Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "قیمت امروز چقدر است؟ کلید من " + testKey}}}},
		{Chat: c.ID, Turn: 1, Role: chat.RoleAssistant, Model: "m1", Parts: []chat.Part{
			{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: "t1", Name: "fetch_page", Args: args}}}},
		{Chat: c.ID, Turn: 1, Role: chat.RoleTool, Parts: []chat.Part{
			{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "t1", Text: "echo " + testKey}}}},
		{Chat: c.ID, Turn: 1, Role: chat.RoleAssistant, Model: "m1", Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: "حدود ۶۸٬۰۰۰ دلار."}}}},
	} {
		if _, _, err := proj.Chats.AppendMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	proj.Release()

	dir := t.TempDir()
	md, err := e.svc.Chat.Export(ctx, op.ID, c.ID, ChatExportRequest{Target: filepath.Join(dir, "chat.md"), Format: chatexport.FormatMarkdown})
	if err != nil || md.Messages != 4 {
		t.Fatalf("%+v, %v", md, err)
	}
	js, err := e.svc.Chat.Export(ctx, op.ID, c.ID, ChatExportRequest{Target: filepath.Join(dir, "chat.json"), Format: chatexport.FormatJSON})
	if err != nil || js.Messages != 4 {
		t.Fatalf("%+v, %v", js, err)
	}
	// The Markdown has the result's size, not its text.
	for path, n := range map[string]int{md.Path: 2, js.Path: 3} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), testKey) || strings.Count(string(b), "[secret:provider:p]") != n {
			t.Errorf("%s: the key isn't replaced %d times:\n%s", path, n, b)
		}
	}
	b, _ := os.ReadFile(md.Path)
	for _, want := range []string{
		"<div dir=\"rtl\">\n\n# بیت‌کوین\n\n</div>\n",
		"- Project: <bdi>قیمت‌ها</bdi>\n- Model: default (p/m1)\n",
		"<div dir=\"rtl\">\n\nحدود ۶۸٬۰۰۰ دلار.\n\n</div>\n",
		"→ 29 B\n",
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %q in:\n%s", want, b)
		}
	}

	if _, err := e.svc.Chat.Export(ctx, op.ID, c.ID, ChatExportRequest{Target: filepath.Join(dir, "chat.pdf"), Format: "pdf"}); err == nil {
		t.Error("a pdf export worked")
	} else {
		uiErr(t, err, KindInvalid)
	}
	_, err = e.svc.Chat.Export(ctx, op.ID, c.ID, ChatExportRequest{Target: filepath.Join(dir, "missing", "chat.md"), Format: chatexport.FormatMarkdown})
	uiErr(t, err, KindInvalid)
}

// A turn's text exports once it is stored.
func TestChatExportAfterTurn(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, t.TempDir())
	op, err := e.svc.Project.Create(ctx, "Turns")
	if err != nil {
		t.Fatal(err)
	}
	e.fp.Push(fake.Text("The key you sent was " + testKey))
	if _, err := e.svc.Chat.Send(ctx, op.ID, op.Mother, "Hi"); err != nil {
		t.Fatal(err)
	}
	e.idle(op.Mother)
	res, err := e.svc.Chat.Export(ctx, op.ID, op.Mother, ChatExportRequest{Target: filepath.Join(t.TempDir(), "m.md"), Format: chatexport.FormatMarkdown})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(res.Path)
	if !strings.Contains(string(b), "\nHi\n") || !strings.Contains(string(b), "The key you sent was [secret:provider:p]") {
		t.Errorf("got:\n%s", b)
	}
}
