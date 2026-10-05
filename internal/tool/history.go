package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

// History tools (SPEC 8.1): search_history, read_messages and list_chats.

type searchHistoryArgs struct {
	Query     string `json:"query"`
	Scope     string `json:"scope"`
	Substring bool   `json:"substring"`
	Limit     int    `json:"limit"`
}

func searchHistory() Tool {
	return Func(Spec{
		Name: "search_history",
		Description: "Search the messages of this chat, or of every chat in the project, for words. " +
			"Returns snippets with chat IDs, turns and message IDs; read whole messages with read_messages. " +
			"Words match at the start of words. substring: true also finds words inside longer words, more slowly.",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {"type": "string", "minLength": 1, "description": "The words to find; every word must match."},
				"scope": {"enum": ["chat", "project"], "default": "chat", "description": "This chat, or every chat in the project."},
				"substring": {"type": "boolean", "default": false},
				"limit": {"type": "integer", "minimum": 1, "maximum": 100, "default": 20}
			},
			"required": ["query"],
			"additionalProperties": false
		}`),
		Effects: ReadsDB,
	}, func(ctx context.Context, env *Env, a searchHistoryArgs) (Result, error) {
		req := store.SearchReq{Query: a.Query, Substring: a.Substring, Limit: a.Limit}
		if a.Scope != "project" {
			req.Chat = env.Chat
		}
		res, err := env.Project.Chats.Search(ctx, req)
		if errors.Is(err, store.ErrBadQuery) {
			return Result{}, Errorf("%s words at most", count(store.MaxSearchTerms))
		}
		if err != nil {
			return Result{}, err
		}
		var titles map[id.Chat]string
		if req.Chat == "" {
			if titles, err = chatTitles(ctx, env); err != nil {
				return Result{}, err
			}
		}
		var b strings.Builder
		where := "this chat"
		if req.Chat == "" {
			where = "the project"
		}
		switch {
		case len(res.Hits) == 0:
			fmt.Fprintf(&b, "No messages in %s match.", where)
			if !a.Substring {
				b.WriteString(" substring: true also finds words inside longer words.")
			}
		case a.Substring:
			fmt.Fprintf(&b, "%d messages in %s match, newest first:", len(res.Hits), where)
		default:
			fmt.Fprintf(&b, "%d messages in %s match, best first:", len(res.Hits), where)
		}
		for _, h := range res.Hits {
			b.WriteString("\n- ")
			if req.Chat == "" {
				fmt.Fprintf(&b, "chat %s %s · ", h.Chat, quoted(titles[h.Chat]))
			}
			fmt.Fprintf(&b, "turn %d · %s · %s · message %s\n  %s", h.Turn, h.Role, when(h.CreatedAt), h.Message, h.Snippet)
		}
		if res.Stopped {
			b.WriteString("\n[the search stopped after 0.5 s and may have missed older messages; use fewer chats or other words]")
		}
		return Result{Text: b.String()}, nil
	})
}

func chatTitles(ctx context.Context, env *Env) (map[id.Chat]string, error) {
	cs, err := env.Project.Chats.Chats(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[id.Chat]string, len(cs))
	for _, c := range cs {
		m[c.ID] = c.Title
	}
	return m, nil
}

// when is a time as the context shows it.
func when(t time.Time) string { return t.Local().Format("2006-01-02 15:04") }

type readMessagesArgs struct {
	Chat string `json:"chat_id"`
	From int    `json:"from"`
	To   int    `json:"to"`
}

// maxReadTurns bounds the turns of one read_messages call.
const maxReadTurns = 20

func readMessages() Tool {
	return Func(Spec{
		Name: "read_messages",
		Description: "Read the messages of turns from through to of a chat, with their tool calls and results. " +
			fmt.Sprintf("At most %d turns a call. chat_id defaults to this chat.", maxReadTurns),
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"chat_id": {"type": "string"},
				"from": {"type": "integer", "minimum": 1},
				"to": {"type": "integer", "minimum": 1, "description": "Defaults to from."}
			},
			"required": ["from"],
			"additionalProperties": false
		}`),
		// Tool results in old turns can hold page text.
		Effects: ReadsDB | Untrusted,
	}, func(ctx context.Context, env *Env, a readMessagesArgs) (Result, error) {
		ch := id.Chat(a.Chat)
		if ch == "" {
			ch = env.Chat
		}
		if a.To == 0 {
			a.To = a.From
		}
		if a.To < a.From {
			return Result{}, Errorf("to (%d) is before from (%d)", a.To, a.From)
		}
		note := ""
		if a.To-a.From+1 > maxReadTurns {
			a.To = a.From + maxReadTurns - 1
			note = fmt.Sprintf("\n[only turns %d–%d; at most %d turns a call]", a.From, a.To, maxReadTurns)
		}
		ms, _, err := env.Project.Chats.Messages(ctx, ch, a.From, a.To)
		if errors.Is(err, store.ErrNotFound) {
			return Result{}, Errorf("no chat %s; list_chats lists them", ch)
		}
		if err != nil {
			return Result{}, err
		}
		if len(ms) == 0 {
			return Result{Text: fmt.Sprintf("No messages in turns %d–%d.", a.From, a.To)}, nil
		}
		var b strings.Builder
		for i, m := range ms {
			if i > 0 {
				b.WriteString("\n\n")
			}
			writeMessage(&b, m)
		}
		b.WriteString(note)
		return Result{Text: b.String()}, nil
	})
}

// writeMessage writes m as read_messages shows it. Thinking is left out.
func writeMessage(b *strings.Builder, m chat.Message) {
	fmt.Fprintf(b, "— turn %d · %s · %s · message %s", m.Turn, m.Role, when(m.CreatedAt), m.ID)
	for _, p := range m.Parts {
		if p.Kind == chat.PartThinking {
			continue
		}
		b.WriteByte('\n')
		switch p.Kind {
		case chat.PartText:
			b.WriteString(p.Text.Text)
			if p.Text.Stopped {
				b.WriteString(" [stopped by the user]")
			}
		case chat.PartToolCall:
			fmt.Fprintf(b, "[tool call %s: %s %s]", p.ToolCall.ID, p.ToolCall.Name, p.ToolCall.Args)
		case chat.PartToolResult:
			r := p.ToolResult
			kind := "tool result"
			if r.IsError {
				kind = "tool error"
			}
			fmt.Fprintf(b, "[%s %s]\n%s", kind, r.CallID, r.Text)
		case chat.PartImage:
			fmt.Fprintf(b, "[image %s %s]", p.Image.Ref, p.Image.MIME)
		case chat.PartNotice:
			fmt.Fprintf(b, "[notice %s: %s]", p.Notice.Kind, p.Notice.Text)
		case chat.PartApproval:
			a := p.Approval
			ans := "not answered"
			if o, ok := a.Option(a.Answer); ok && a.Answer != "" {
				ans = o.Label
			}
			fmt.Fprintf(b, "[approval %s: %s — %s]", a.Kind, a.Ask, ans)
		case chat.PartQuestion:
			q := p.Question
			var asked []string
			for _, it := range q.Questions {
				asked = append(asked, it.Header+": "+it.Question)
			}
			ans := "not answered"
			if q.AnsweredAt != nil {
				j, _ := json.Marshal(q.Answers)
				ans = string(j)
			}
			fmt.Fprintf(b, "[question %s — %s]", strings.Join(asked, "; "), ans)
		default:
			fmt.Fprintf(b, "[%s part]", p.Kind)
		}
	}
}

func listChats() Tool {
	return Func(Spec{
		Name:        "list_chats",
		Description: "List the project's chats: ID, title, role and status. The Mother chat is first.",
		Schema:      json.RawMessage(`{"type": "object", "properties": {}, "additionalProperties": false}`),
		Effects:     ReadsDB,
	}, func(ctx context.Context, env *Env, _ struct{}) (Result, error) {
		cs, err := env.Project.Chats.Chats(ctx)
		if err != nil {
			return Result{}, err
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%d chats:", len(cs))
		for _, c := range cs {
			fmt.Fprintf(&b, "\n- %s %s", c.ID, quoted(c.Title))
			if c.Kind == chat.KindMother {
				b.WriteString(" · Mother")
			}
			if c.ID == env.Chat {
				b.WriteString(" · this chat")
			}
			if role, _, _ := strings.Cut(strings.TrimSpace(c.Role), "\n"); role != "" {
				fmt.Fprintf(&b, " · role: %s", role)
			}
			if env.ChatStatus != nil {
				if s := env.ChatStatus(c.ID); s != "" {
					b.WriteString(" · " + s)
				}
			}
			if c.Archived {
				b.WriteString(" · archived")
			}
		}
		return Result{Text: b.String()}, nil
	})
}
