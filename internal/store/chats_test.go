package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

func openTestChats(t *testing.T) *ChatsDB {
	t.Helper()
	c, err := OpenChats(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close(context.Background()) })
	return c
}

func newChat(t *testing.T, c *ChatsDB, ch chat.Chat) chat.Chat {
	t.Helper()
	got, err := c.CreateChat(context.Background(), id.SourceUser, ch)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestChatsMother(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	m, err := c.EnsureMother(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if m.Kind != chat.KindMother || m.Title != MotherTitle || !m.TitleFixed || m.CreatedBy != id.SourceApp || m.Model != DefaultModel {
		t.Fatalf("Mother = %+v", m)
	}
	again, err := c.EnsureMother(ctx)
	if err != nil || again.ID != m.ID {
		t.Fatalf("second EnsureMother = %s, %v; want the same chat %s", again.ID, err, m.ID)
	}
	if _, err := c.Archive(ctx, m.ID, true); !errors.Is(err, ErrMother) {
		t.Errorf("Archive(Mother) = %v, want ErrMother", err)
	}
	if err := c.DeleteChat(ctx, m.ID); !errors.Is(err, ErrMother) {
		t.Errorf("DeleteChat(Mother) = %v, want ErrMother", err)
	}
	if _, err := c.SetRole(ctx, m.ID, "a role", id.SourceUser); !errors.Is(err, ErrMother) {
		t.Errorf("SetRole(Mother) = %v, want ErrMother", err)
	}
	// A second Mother can't be stored, even by a bug in the caller.
	dup := m
	dup.ID, dup.Title = id.Chat(id.New()), "Other"
	if _, err := Do(ctx, c.DB, 0, func(tx *sql.Tx) (struct{}, error) { return struct{}{}, insertChat(tx, dup) }); err == nil {
		t.Error("a second Mother chat was stored")
	}

	addTurn(t, c, m.ID, 1, "hello")
	if _, err := c.SaveNotes(ctx, chat.SessionNote{Chat: m.ID, Content: "notes"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Clear(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	after, err := c.Chat(ctx, m.ID)
	if err != nil || after.ID != m.ID || after.Archived {
		t.Fatalf("after Clear: %+v, %v", after, err)
	}
	if ms, _, _ := c.Messages(ctx, m.ID, 0, 0); len(ms) != 0 {
		t.Errorf("Clear left %d messages", len(ms))
	}
	if n, _ := c.Notes(ctx, m.ID); n.Revision != 0 || n.Content != "" {
		t.Errorf("Clear left the notes: %+v", n)
	}
}

func TestChatsMotherTitleTaken(t *testing.T) {
	c := openTestChats(t)
	newChat(t, c, chat.Chat{Title: "mother"})
	m, err := c.EnsureMother(context.Background())
	if err != nil || m.Title != "Mother 2" {
		t.Fatalf("EnsureMother = %q, %v; want Mother 2", m.Title, err)
	}
	if cs, _ := c.Chats(context.Background()); len(cs) != 2 || cs[0].ID != m.ID {
		t.Errorf("Chats = %+v, want Mother first though it is newer", cs)
	}
}

func TestChatsCreate(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	m, _ := c.EnsureMother(ctx)
	a := newChat(t, c, chat.Chat{})
	src := id.SourceOf("message", id.New())
	b, err := c.CreateChat(ctx, src, chat.Chat{Title: "  Reviewer ", Role: "Review pipelines.", Skills: []string{"sql"}, Model: "fast", DefaultPage: "p1"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Kind != chat.KindChat || a.Title != "" || a.TitleFixed || a.Model != DefaultModel || a.CreatedBy != id.SourceUser || a.Skills == nil {
		t.Errorf("a = %+v", a)
	}
	if b.Title != "Reviewer" || !b.TitleFixed || b.CreatedBy != src {
		t.Errorf("b = %+v", b)
	}
	all, err := c.Chats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]chat.Chat{m, a, b}, all); diff != "" {
		t.Errorf("Chats (-want +got):\n%s", diff)
	}
	if rc, _ := c.RoleChanges(ctx, b.ID); len(rc) != 1 || rc[0].After != "Review pipelines." || rc[0].Source != src {
		t.Errorf("role changes = %+v", rc)
	}
	if _, err := c.CreateChat(ctx, id.SourceUser, chat.Chat{Title: "reviewer"}); !errors.Is(err, ErrTitleTaken) {
		t.Errorf("a taken title: %v, want ErrTitleTaken", err)
	}
	if _, err := c.CreateChat(ctx, id.SourceUser, chat.Chat{Role: strings.Repeat("abcd", MaxRoleTokens+1)}); !errors.Is(err, ErrTooLong) {
		t.Errorf("a long role: %v, want ErrTooLong", err)
	}
	if _, err := c.CreateChat(ctx, id.SourceUser, chat.Chat{Title: "two\nlines"}); !errors.Is(err, ErrBadTitle) {
		t.Errorf("a bad title: %v, want ErrBadTitle", err)
	}
	if _, err := c.Chat(ctx, id.Chat(id.New())); !errors.Is(err, ErrNotFound) {
		t.Errorf("Chat(missing) = %v", err)
	}
}

func TestChatsTitles(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	a, b, f := newChat(t, c, chat.Chat{}), newChat(t, c, chat.Chat{}), newChat(t, c, chat.Chat{})
	set := func(ch id.Chat, title string, fixed bool) string {
		t.Helper()
		got, _, err := c.SetTitle(ctx, ch, title, fixed)
		if err != nil {
			t.Fatalf("SetTitle(%q, %v): %v", title, fixed, err)
		}
		return got
	}
	if got := set(a.ID, "BTC prices", false); got != "BTC prices" {
		t.Errorf("generated = %q", got)
	}
	if got := set(b.ID, "btc Prices", false); got != "btc Prices 2" {
		t.Errorf("a generated title another chat has = %q, want a number", got)
	}
	if got := set(a.ID, "BTC prices", false); got != "BTC prices" {
		t.Errorf("a chat's own title again = %q", got)
	}
	if _, _, err := c.SetTitle(ctx, f.ID, "BTC PRICES", true); !errors.Is(err, ErrTitleTaken) {
		t.Errorf("a fixed title another chat has: %v, want ErrTitleTaken", err)
	}
	if got := set(a.ID, "ETH tracker", true); got != "ETH tracker" {
		t.Errorf("fixed = %q", got)
	}
	if got := set(a.ID, "Something else", false); got != "ETH tracker" {
		t.Errorf("a generated title after a fixed one = %q, want it ignored", got)
	}
	if got, _ := c.Chat(ctx, a.ID); !got.TitleFixed || got.Title != "ETH tracker" {
		t.Errorf("a = %+v", got)
	}
	if got := set(a.ID, "Renamed", true); got != "Renamed" {
		t.Errorf("a fixed title over a fixed one = %q", got)
	}
	long := strings.Repeat("ب", MaxTitle)
	set(f.ID, long, true)
	if got := set(b.ID, long, false); got != strings.Repeat("ب", MaxTitle-2)+" 2" {
		t.Errorf("a long title with a number = %q", got)
	}
	for _, bad := range []string{"", "  ", "a\nb", strings.Repeat("x", MaxTitle+1)} {
		if _, _, err := c.SetTitle(ctx, a.ID, bad, true); !errors.Is(err, ErrBadTitle) {
			t.Errorf("SetTitle(%q) = %v, want ErrBadTitle", bad, err)
		}
	}
	if _, _, err := c.SetTitle(ctx, id.Chat(id.New()), "x", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetTitle(missing) = %v", err)
	}
}

func TestChatsRoles(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	a := newChat(t, c, chat.Chat{})
	mother := id.SourceOf("message", id.New())
	s1, err := c.SetRole(ctx, a.ID, "  Track ETH. ", id.SourceUser)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := c.SetRole(ctx, a.ID, "Track ETH.", mother) // no change: nothing recorded
	if err != nil || s2 != s1+1 {
		t.Fatalf("same role: seq %d → %d, %v", s1, s2, err)
	}
	if _, err := c.SetRole(ctx, a.ID, strings.Repeat("abcd", MaxRoleTokens), mother); err != nil {
		t.Fatalf("a role of exactly %d tokens: %v", MaxRoleTokens, err)
	}
	if _, err := c.SetRole(ctx, a.ID, strings.Repeat("abcd", MaxRoleTokens)+"abcd", mother); !errors.Is(err, ErrTooLong) {
		t.Fatalf("a role over the limit: %v", err)
	}
	if _, err := c.SetRole(ctx, a.ID, "", id.SourceUser); err != nil {
		t.Fatal(err)
	}
	rc, err := c.RoleChanges(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []RoleChange{
		{Before: "", After: "Track ETH.", Source: id.SourceUser},
		{Before: "Track ETH.", After: strings.Repeat("abcd", MaxRoleTokens), Source: mother},
		{Before: strings.Repeat("abcd", MaxRoleTokens), After: "", Source: id.SourceUser},
	}
	for i := range rc {
		if rc[i].At.IsZero() || time.Since(rc[i].At) > time.Minute {
			t.Errorf("change %d at %v", i, rc[i].At)
		}
		rc[i].At = time.Time{}
	}
	if diff := cmp.Diff(want, rc); diff != "" {
		t.Errorf("role changes (-want +got):\n%s", diff)
	}
	if _, err := c.SetRole(ctx, id.Chat(id.New()), "x", id.SourceUser); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetRole(missing) = %v", err)
	}
}

// addTurn writes a user message with one text part.
func addTurn(t *testing.T, c *ChatsDB, ch id.Chat, turn int, text string) chat.Message {
	t.Helper()
	m, _, err := c.AppendMessage(context.Background(), chat.Message{Chat: ch, Turn: turn, Role: chat.RoleUser,
		Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: text}}}})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func allParts() []chat.Part {
	at := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	return []chat.Part{
		{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: "let me look", Signature: "sig+/=="}},
		{Kind: chat.PartThinking, Thinking: &chat.Thinking{Redacted: "opaque"}},
		{Kind: chat.PartText, Text: &chat.Text{Text: "سلام، قیمت‌ها", Stopped: true}},
		{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: "c1", Name: "query", Args: json.RawMessage(`{"sql":"SELECT 1"}`), Extra: `{"google": {"thought_signature":"x<y"}}`}},
		{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "c1", Text: "first lines…", Ref: "cache/tool/c1.txt"}},
		{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "c2", Text: "cancelled by user", IsError: true}},
		{Kind: chat.PartImage, Image: &chat.Image{Ref: "images/a.png", MIME: "image/png", Alt: "a chart"}},
		{Kind: chat.PartNotice, Notice: &chat.Notice{Kind: chat.NoticeTaskFinished, Text: "task t1 finished"}},
		{Kind: chat.PartApproval, Approval: &chat.Approval{ID: "a1", Kind: "host", Ask: "Allow example.com?", Options: []string{"Allow", "Deny"}, Answer: "Allow", By: id.SourceUser, AnsweredAt: &at}},
		{Kind: chat.PartQuestion, Question: &chat.Question{
			Questions: []chat.QuestionItem{{Header: "Coin", Question: "Which coin?", Options: []chat.Option{{Label: "BTC", Recommended: true}, {Label: "ETH"}}}},
			Answers:   map[string][]string{"Coin": {"BTC"}}, AnsweredAt: &at}},
	}
}

func TestChatsMessages(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	a, other := newChat(t, c, chat.Chat{}), newChat(t, c, chat.Chat{})
	seq := func() uint64 {
		s, err := c.Seq(ctx, a.ID)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s0 := seq()
	u := addTurn(t, c, a.ID, 1, "hi")
	addTurn(t, c, other.ID, 1, "elsewhere")
	if seq() != s0+1 {
		t.Fatalf("seq after one message = %d, want %d", seq(), s0+1)
	}
	parts := allParts()
	// The assistant message with its tool call, before the tool runs.
	am, s, err := c.AppendMessage(ctx, chat.Message{Chat: a.ID, Turn: 1, Role: chat.RoleAssistant, Model: "m", Parts: parts[:4]})
	if err != nil {
		t.Fatal(err)
	}
	if s != s0+2 || am.ID == "" || am.CreatedAt.IsZero() {
		t.Fatalf("AppendMessage = %+v, seq %d", am, s)
	}
	for i, p := range parts[4:] {
		idx, s, err := c.AppendPart(ctx, am.ID, p)
		if err != nil {
			t.Fatalf("part %d: %v", 4+i, err)
		}
		if idx != 4+i || s != s0+3+uint64(i) {
			t.Fatalf("part %d: index %d, seq %d", 4+i, idx, s)
		}
	}
	usage := chat.Usage{Input: 10, Output: 20, CacheRead: 30, CacheWrite: 40}
	if _, err := c.SetUsage(ctx, am.ID, usage); err != nil {
		t.Fatal(err)
	}
	addTurn(t, c, a.ID, 2, "next")
	addTurn(t, c, a.ID, 3, "last")

	ms, got, err := c.Messages(ctx, a.ID, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != seq() {
		t.Errorf("Messages seq %d, chat seq %d", got, seq())
	}
	am.Parts, am.Usage = parts, usage
	u.CreatedAt, am.CreatedAt = u.CreatedAt.UTC(), am.CreatedAt.UTC()
	if diff := cmp.Diff([]chat.Message{u, am}, ms); diff != "" {
		t.Fatalf("turn 1 (-want +got):\n%s", diff)
	}
	if ms[1].Parts[3].ToolCall.Extra != parts[3].ToolCall.Extra {
		t.Errorf("Extra changed: %q", ms[1].Parts[3].ToolCall.Extra)
	}
	if r := one[string](t, c.DB, "SELECT ref || '|' || preview FROM message_parts WHERE message_id = ? AND seq = 4", am.ID); r != "cache/tool/c1.txt|first lines…" {
		t.Errorf("ref|preview = %q", r)
	}
	if r := one[string](t, c.DB, "SELECT ref || '|' || preview FROM message_parts WHERE message_id = ? AND seq = 6", am.ID); r != "images/a.png|" {
		t.Errorf("image ref|preview = %q", r)
	}
	if r := one[string](t, c.DB, "SELECT ref || '|' || preview FROM message_parts WHERE message_id = ? AND seq = 5", am.ID); r != "|" {
		t.Errorf("a small result has ref|preview %q", r)
	}
	for _, tc := range []struct{ from, to, want int }{{2, 3, 2}, {2, 0, 2}, {0, 0, 4}, {4, 0, 0}} {
		ms, _, err := c.Messages(ctx, a.ID, tc.from, tc.to)
		if err != nil || len(ms) != tc.want {
			t.Errorf("Messages(%d, %d) = %d messages, %v; want %d", tc.from, tc.to, len(ms), err, tc.want)
		}
	}
	if ms, _, _ := c.Messages(ctx, a.ID, 3, 3); len(ms) != 1 || ms[0].Parts[0].Text.Text != "last" {
		t.Errorf("turn 3 = %+v", ms)
	}

	bad := chat.Part{Kind: chat.PartText}
	if _, _, err := c.AppendPart(ctx, am.ID, bad); !errors.Is(err, chat.ErrInvalidPart) {
		t.Errorf("an invalid part: %v", err)
	}
	if _, _, err := c.AppendMessage(ctx, chat.Message{Chat: a.ID, Turn: 4, Role: chat.RoleUser, Parts: []chat.Part{bad}}); !errors.Is(err, chat.ErrInvalidPart) {
		t.Errorf("a message with an invalid part: %v", err)
	}
	if _, _, err := c.AppendMessage(ctx, chat.Message{Chat: a.ID, Turn: 0, Role: chat.RoleUser}); err == nil {
		t.Error("turn 0 was written")
	}
	if _, _, err := c.AppendMessage(ctx, chat.Message{Chat: id.Chat(id.New()), Turn: 1, Role: chat.RoleUser}); !errors.Is(err, ErrNotFound) {
		t.Errorf("a message in a missing chat: %v", err)
	}
	if _, _, err := c.AppendPart(ctx, id.Message(id.New()), parts[2]); !errors.Is(err, ErrNotFound) {
		t.Errorf("a part of a missing message: %v", err)
	}
	if _, err := c.SetUsage(ctx, id.Message(id.New()), usage); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetUsage(missing) = %v", err)
	}
	if _, _, err := c.Messages(ctx, id.Chat(id.New()), 0, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("Messages(missing chat) = %v", err)
	}
	if _, err := c.Seq(ctx, id.Chat(id.New())); !errors.Is(err, ErrNotFound) {
		t.Errorf("Seq(missing chat) = %v", err)
	}
	if n := one[int](t, c.DB, "SELECT count(*) FROM messages WHERE chat_id = ?", a.ID); n != 4 {
		t.Errorf("%d messages after the failed writes, want 4", n)
	}
}

func TestChatsUnknownPartType(t *testing.T) {
	c := openTestChats(t)
	a := newChat(t, c, chat.Chat{})
	m := addTurn(t, c, a.ID, 1, "hi")
	mustExec(t, c.DB, "UPDATE message_parts SET type = 'hologram' WHERE message_id = ?", m.ID)
	if _, _, err := c.Messages(context.Background(), a.ID, 0, 0); err == nil || !strings.Contains(err.Error(), "hologram") {
		t.Errorf("Messages with an unknown part type = %v", err)
	}
}

func TestChatsSeqOnEveryWrite(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	a := newChat(t, c, chat.Chat{})
	m := addTurn(t, c, a.ID, 1, "hi")
	last, _ := c.Seq(ctx, a.ID)
	writes := map[string]func() (uint64, error){
		"SetTitle": func() (uint64, error) { _, s, err := c.SetTitle(ctx, a.ID, "T", true); return s, err },
		"SetRole":  func() (uint64, error) { return c.SetRole(ctx, a.ID, "r", id.SourceUser) },
		"SetModel": func() (uint64, error) { return c.SetModel(ctx, a.ID, "smart") },
		"Archive":  func() (uint64, error) { return c.Archive(ctx, a.ID, true) },
		"Restore":  func() (uint64, error) { return c.Archive(ctx, a.ID, false) },
		"SetUsage": func() (uint64, error) { return c.SetUsage(ctx, m.ID, chat.Usage{Output: 1}) },
		"Notes": func() (uint64, error) {
			_, err := c.SaveNotes(ctx, chat.SessionNote{Chat: a.ID, Content: "n"})
			s, _ := c.Seq(ctx, a.ID)
			return s, err
		},
		"Clear": func() (uint64, error) { return c.Clear(ctx, a.ID) },
	}
	for _, name := range []string{"SetTitle", "SetRole", "SetModel", "Archive", "Restore", "SetUsage", "Notes", "Clear"} {
		s, err := writes[name]()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if s != last+1 {
			t.Errorf("%s: seq %d, want %d", name, s, last+1)
		}
		last = s
	}
	got, _ := c.Chat(ctx, a.ID)
	if got.Model != "smart" || got.Archived || got.Role != "r" || got.Title != "T" {
		t.Errorf("after the writes: %+v", got)
	}
	if _, err := c.SetModel(ctx, a.ID, " "); err == nil {
		t.Error("an empty model was saved")
	}
	if _, err := c.Archive(ctx, id.Chat(id.New()), true); !errors.Is(err, ErrNotFound) {
		t.Errorf("Archive(missing) = %v", err)
	}
	if _, err := c.Clear(ctx, id.Chat(id.New())); !errors.Is(err, ErrNotFound) {
		t.Errorf("Clear(missing) = %v", err)
	}
}

func TestChatsClearAndDelete(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	a := newChat(t, c, chat.Chat{Title: "Keep", Role: "a role", Model: "fast"})
	b := newChat(t, c, chat.Chat{})
	for _, ch := range []id.Chat{a.ID, b.ID} {
		m := addTurn(t, c, ch, 1, "hi")
		mustExec(t, c.DB, "INSERT INTO review_chunks VALUES (?, ?, ?, '[]', '2026-01-01T00:00:00.000Z')", ch, m.ID, m.ID)
		mustExec(t, c.DB, "UPDATE chats SET memory_reviewed_up_to = ? WHERE id = ?", m.ID, ch)
		if _, err := c.SaveNotes(ctx, chat.SessionNote{Chat: ch, Content: "n"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.Clear(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	count := func(q string, ch id.Chat) int { return one[int](t, c.DB, q, ch) }
	for _, q := range []string{
		"SELECT count(*) FROM messages WHERE chat_id = ?",
		"SELECT count(*) FROM message_parts WHERE message_id IN (SELECT id FROM messages WHERE chat_id = ?)",
		"SELECT count(*) FROM session_notes WHERE chat_id = ?",
		"SELECT count(*) FROM review_chunks WHERE chat_id = ?",
		"SELECT count(*) FROM chats WHERE id = ? AND memory_reviewed_up_to <> ''",
	} {
		if n := count(q, a.ID); n != 0 {
			t.Errorf("after Clear: %s = %d", q, n)
		}
		if n := count(q, b.ID); n != 1 {
			t.Errorf("Clear of another chat changed %s: %d", q, n)
		}
	}
	if n := one[int](t, c.DB, "SELECT count(*) FROM message_parts"); n != 1 {
		t.Errorf("%d parts left, want only b's", n)
	}
	kept, _ := c.Chat(ctx, a.ID)
	if kept.Title != "Keep" || kept.Role != "a role" || kept.Model != "fast" {
		t.Errorf("Clear changed the chat: %+v", kept)
	}
	if rc, _ := c.RoleChanges(ctx, a.ID); len(rc) != 1 {
		t.Errorf("Clear changed the role history: %d changes", len(rc))
	}

	if err := c.DeleteChat(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"messages", "message_parts", "session_notes", "review_chunks"} {
		if n := one[int](t, c.DB, "SELECT count(*) FROM "+q); n != 0 {
			t.Errorf("DeleteChat left %d rows in %s", n, q)
		}
	}
	if err := c.DeleteChat(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if n := one[int](t, c.DB, "SELECT count(*) FROM role_changes"); n != 0 {
		t.Errorf("DeleteChat left %d role changes", n)
	}
	if err := c.DeleteChat(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteChat(deleted) = %v", err)
	}
}

func TestSessionNotes(t *testing.T) {
	c := openTestChats(t)
	ctx := context.Background()
	a := newChat(t, c, chat.Chat{})
	if n, err := c.Notes(ctx, a.ID); err != nil || n.Revision != 0 || n.Chat != a.ID {
		t.Fatalf("no notes yet: %+v, %v", n, err)
	}
	r, err := c.SaveNotes(ctx, chat.SessionNote{Chat: a.ID, Content: "plan: one"})
	if err != nil || r != 1 {
		t.Fatalf("first save = %d, %v", r, err)
	}
	if _, err := c.SaveNotes(ctx, chat.SessionNote{Chat: a.ID, Content: "stale", Revision: 0}); !errors.Is(err, ErrConflict) {
		t.Fatalf("a save from revision 0 = %v, want ErrConflict", err)
	}
	if _, err := c.SaveNotes(ctx, chat.SessionNote{Chat: a.ID, Content: "ahead", Revision: 2}); !errors.Is(err, ErrConflict) {
		t.Fatalf("a save from revision 2 = %v, want ErrConflict", err)
	}
	full := strings.Repeat("abcd", MaxNotesTokens)
	if r, err = c.SaveNotes(ctx, chat.SessionNote{Chat: a.ID, Content: full, Revision: 1}); err != nil || r != 2 {
		t.Fatalf("second save = %d, %v", r, err)
	}
	n, err := c.Notes(ctx, a.ID)
	if err != nil || n.Content != full || n.Revision != 2 || n.UpdatedAt.IsZero() {
		t.Fatalf("Notes = rev %d, %d bytes, %v, %v", n.Revision, len(n.Content), n.UpdatedAt, err)
	}
	if _, err := c.SaveNotes(ctx, chat.SessionNote{Chat: a.ID, Content: full + "abcd", Revision: 2}); !errors.Is(err, ErrTooLong) {
		t.Errorf("notes over the limit: %v", err)
	}
	if _, err := c.SaveNotes(ctx, chat.SessionNote{Chat: id.Chat(id.New()), Content: "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("notes of a missing chat: %v", err)
	}
}

func TestChatsReadOnly(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	c, err := OpenChats(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := c.EnsureMother(ctx)
	addTurn(t, c, m.ID, 1, "hi")
	c.Close(ctx)
	ro, err := OpenChatsReadOnly(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close(ctx)
	if ms, _, err := ro.Messages(ctx, m.ID, 0, 0); err != nil || len(ms) != 1 {
		t.Fatalf("read-only Messages = %d, %v", len(ms), err)
	}
	if _, err := ro.Clear(ctx, m.ID); !errors.Is(err, ErrReadOnly) {
		t.Errorf("read-only Clear = %v", err)
	}
}
