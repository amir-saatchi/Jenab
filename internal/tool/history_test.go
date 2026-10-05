package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

var zwnj = string(rune(0x200C))

func addMessage(t *testing.T, env *Env, ch id.Chat, turn int, role chat.Role, parts ...chat.Part) chat.Message {
	t.Helper()
	m, _, err := env.Project.Chats.AppendMessage(context.Background(), chat.Message{Chat: ch, Turn: turn, Role: role, Parts: parts})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func text(s string) chat.Part { return chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: s}} }

func TestSearchHistory(t *testing.T) {
	env := testEnv(t)
	ctx := context.Background()
	other, err := env.Project.Chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: "قیمت" + zwnj + "ها"})
	if err != nil {
		t.Fatal(err)
	}
	here := addMessage(t, env, env.Chat, 1, chat.RoleUser, text("What is the Bitcoin price today?"))
	addMessage(t, env, env.Chat, 2, chat.RoleAssistant, text("The Bitcoinpreis is 60,000 EUR."))
	there := addMessage(t, env, other.ID, 1, chat.RoleUser, text("Track the bitcoin price every day."))

	got := mustText(t, env, "search_history", `{"query": "bitcoin price"}`)
	contains(t, got, "1 messages in this chat match, best first:", "turn 1 · user · ", "message "+string(here.ID), "What is the Bitcoin price today?")
	if strings.Contains(got, string(there.ID)) || strings.Contains(got, "- chat ") {
		t.Errorf("chat scope shows another chat:\n%s", got)
	}

	got = mustText(t, env, "search_history", `{"query": "bitcoin price", "scope": "project"}`)
	contains(t, got, "2 messages in the project match", "chat "+string(other.ID)+` "قیمت`+zwnj+`ها" · turn 1`, string(here.ID))

	got = mustText(t, env, "search_history", `{"query": "preis"}`)
	contains(t, got, "No messages in this chat match. substring: true also finds words inside longer words.")
	got = mustText(t, env, "search_history", `{"query": "preis", "substring": true}`)
	contains(t, got, "1 messages in this chat match, newest first:", "Bitcoinpreis")

	got = mustText(t, env, "search_history", `{"query": "price", "scope": "project", "limit": 1}`)
	if strings.Count(got, "\n- ") != 1 {
		t.Errorf("limit 1:\n%s", got)
	}
	msg := toolError(t, env, "search_history", fmt.Sprintf(`{"query": %q}`, strings.Repeat("w ", 33)))
	contains(t, msg, "32 words at most")
	toolError(t, env, "search_history", `{"query": ""}`)
	toolError(t, env, "search_history", `{"query": "x", "scope": "everything"}`)
	toolError(t, env, "search_history", `{"query": "x", "limit": 101}`)
}

func TestReadMessages(t *testing.T) {
	env := testEnv(t)
	at := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	addMessage(t, env, env.Chat, 1, chat.RoleUser, text("Fetch the page."))
	addMessage(t, env, env.Chat, 1, chat.RoleAssistant,
		chat.Part{Kind: chat.PartThinking, Thinking: &chat.Thinking{Text: "secret thoughts"}},
		text("Fetching."),
		chat.Part{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: "c1", Name: "fetch_page", Args: json.RawMessage(`{"url":"https://example.com"}`)}},
	)
	addMessage(t, env, env.Chat, 1, chat.RoleTool,
		chat.Part{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "c1", Text: "[page — example.com — 3 tokens — ref: k]\nHi.", Ref: "k"}},
		chat.Part{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: "c2", Text: "cancelled by user", IsError: true}},
	)
	addMessage(t, env, env.Chat, 2, chat.RoleAssistant,
		chat.Part{Kind: chat.PartImage, Image: &chat.Image{Ref: "images/a.png", MIME: "image/png"}},
		chat.Part{Kind: chat.PartNotice, Notice: &chat.Notice{Kind: chat.NoticeTaskFinished, Text: "task t1 finished"}},
		chat.Part{Kind: chat.PartApproval, Approval: &chat.Approval{ID: "a1", Kind: "host", Target: "example.com", Ask: "Allow example.com?", Options: []chat.ApprovalOption{{Label: "Allow", Grant: chat.GrantAlways}, {Label: "Deny", Grant: chat.GrantDeny}}, Answer: chat.GrantAlways, AnsweredAt: &at}},
		chat.Part{Kind: chat.PartQuestion, Question: &chat.Question{
			Questions: []chat.QuestionItem{{Header: "Coin", Question: "Which coin?", Options: []chat.Option{{Label: "BTC"}, {Label: "ETH"}}}},
			Answers:   map[string][]string{"Coin": {"BTC"}}, AnsweredAt: &at}},
	)
	for turn := 3; turn <= 30; turn++ {
		addMessage(t, env, env.Chat, turn, chat.RoleUser, text(fmt.Sprintf("message %d", turn)))
	}

	got := mustText(t, env, "read_messages", `{"from": 1}`)
	contains(t, got, "— turn 1 · user · ", "Fetch the page.", "— turn 1 · assistant · ", "Fetching.\n[tool call c1: fetch_page {\"url\":\"https://example.com\"}]",
		"[tool result c1]\n[page — example.com — 3 tokens — ref: k]\nHi.", "[tool error c2]\ncancelled by user")
	if strings.Contains(got, "secret thoughts") || strings.Contains(got, "turn 2") {
		t.Errorf("turn 1:\n%s", got)
	}
	got = mustText(t, env, "read_messages", `{"from": 2, "to": 2}`)
	contains(t, got, "[image images/a.png image/png]", "[notice task_finished: task t1 finished]",
		"[approval host: Allow example.com? — Allow]", `[question Coin: Which coin? — {"Coin":["BTC"]}]`)

	got = mustText(t, env, "read_messages", `{"from": 5, "to": 30}`)
	contains(t, got, "message 24", "[only turns 5–24; at most 20 turns a call]")
	if strings.Contains(got, "message 25") || strings.Contains(got, "message 4\n") {
		t.Errorf("range:\n%s", got)
	}
	contains(t, mustText(t, env, "read_messages", `{"from": 40, "to": 41}`), "No messages in turns 40–41.")
	contains(t, toolError(t, env, "read_messages", `{"from": 3, "to": 2}`), "to (2) is before from (3)")
	contains(t, toolError(t, env, "read_messages", `{"chat_id": "01NOPE", "from": 1}`), "no chat 01NOPE")
	toolError(t, env, "read_messages", `{"from": 0}`)

	mother, err := env.Project.Chats.EnsureMother(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	addMessage(t, env, mother.ID, 1, chat.RoleUser, text("hello Mother"))
	contains(t, mustText(t, env, "read_messages", fmt.Sprintf(`{"chat_id": %q, "from": 1}`, mother.ID)), "hello Mother")
}

func TestListChats(t *testing.T) {
	env := testEnv(t)
	ctx := context.Background()
	old, err := env.Project.Chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: "Old"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Project.Chats.Archive(ctx, old.ID, true); err != nil {
		t.Fatal(err)
	}
	status := func(c id.Chat) string {
		if c == env.Chat {
			return "in a turn"
		}
		return ""
	}
	withStatus := *env
	withStatus.ChatStatus = status
	r, err := callWith(t, Deps{}, &withStatus, "list_chats", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(r.Text, "\n")
	if len(lines) != 4 || lines[0] != "3 chats:" || !strings.HasSuffix(lines[1], " · Mother") {
		t.Fatalf("got:\n%s", r.Text)
	}
	contains(t, r.Text, "- "+string(env.Chat)+` "Prices" · this chat · role: Track coin prices. · in a turn`,
		"- "+string(old.ID)+` "Old" · archived`)
	contains(t, mustText(t, env, "list_chats", ``), "3 chats:")
	toolError(t, env, "list_chats", `{"all": true}`)
}
