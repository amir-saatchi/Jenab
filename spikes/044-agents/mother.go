package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// maxRoleTokens is a role's limit (8.6), counted as the app estimates.
const maxRoleTokens = 500

// newAgent creates a chat with a role and its tools, and registers them
// with the ChatTools hook.
func (r *run) newAgent(ctx context.Context, src id.Source, phase, title, role, message string, picks []string, sent bool) (id.Chat, error) {
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return "", err
	}
	defer p.Release()
	ch, err := p.Chats.CreateChat(ctx, src, chat.Chat{Title: title, Role: role, Model: r.j.Model})
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	r.agents[ch.ID] = toolsOf(picks)
	r.mu.Unlock()
	return ch.ID, nil
}

func roleTokens(role string) int {
	return provider.EstimateTokens(provider.Request{System: []provider.Block{{Text: role}}})
}

type createArgs struct {
	Title   string   `json:"title"`
	Role    string   `json:"role"`
	Message string   `json:"message"`
	Tools   []string `json:"tools"`
}

type sendArgs struct {
	Chat    string `json:"chat_id"`
	Message string `json:"message"`
}

// motherTools are create_chat and send_to_chat. Both return a task ID
// right away; the harness runs the target chat's turn after Mother's turn
// ends. Mother gets no finish notice: the runs end before it would
// matter.
func (r *run) motherTools() []tool.Tool {
	create := tool.Func(tool.Spec{Name: "create_chat", Mother: true,
		Description: "Start a new chat with a role for a lasting job, with the tools it needs, and send it a first message. " +
			"Returns a task ID right away; the chat's reply comes later in a finish notice.",
		Schema: createChatSchema(r.j.Cond == condSingle)},
		func(ctx context.Context, env *tool.Env, a createArgs) (tool.Result, error) {
			if strings.TrimSpace(a.Title) == "" || strings.TrimSpace(a.Message) == "" {
				return tool.Result{}, &tool.Error{Msg: "title and message are required", Err: tool.ErrArgs}
			}
			if len(a.Tools) == 0 {
				return tool.Result{}, &tool.Error{Msg: "tools is empty: give the chat the tools it needs for its job", Err: tool.ErrArgs}
			}
			if n := roleTokens(a.Role); n > maxRoleTokens {
				return tool.Result{}, &tool.Error{Msg: fmt.Sprintf("the role is about %d tokens; a role has at most %d", n, maxRoleTokens), Err: tool.ErrArgs}
			}
			r.mu.Lock()
			phase := r.phase
			r.mu.Unlock()
			ch, err := r.newAgent(ctx, env.Source, phase, a.Title, a.Role, a.Message, a.Tools, false)
			if err != nil {
				return tool.Result{}, &tool.Error{Msg: "the chat wasn't created: " + err.Error()}
			}
			ts := toolsOf(a.Tools)
			return tool.Result{Text: r.queue(creation{Phase: phase, Chat: string(ch), Title: a.Title, Role: a.Role, Message: a.Message,
				Picks: a.Tools, Groups: groupsOf(ts), Tools: ts})}, nil
		})
	send := tool.Func(tool.Spec{Name: "send_to_chat", Mother: true,
		Description: "Continue an existing chat: send it a message. Returns a task ID right away; the chat's reply comes later in a finish notice.",
		Schema:      schema(`"chat_id":{"type":"string"},"message":{"type":"string"}`, "chat_id", "message")},
		func(ctx context.Context, env *tool.Env, a sendArgs) (tool.Result, error) {
			p := env.Project
			cs, err := p.Chats.Chats(ctx)
			if err != nil {
				return tool.Result{}, err
			}
			for _, c := range cs {
				if string(c.ID) == a.Chat && c.Kind != chat.KindMother {
					r.mu.Lock()
					phase := r.phase
					ts := r.agents[c.ID]
					r.mu.Unlock()
					return tool.Result{Text: r.queue(creation{Phase: phase, Sent: true, Chat: a.Chat, Title: c.Title, Message: a.Message, Groups: groupsOf(ts), Tools: ts})}, nil
				}
			}
			return tool.Result{}, &tool.Error{Msg: fmt.Sprintf("no chat %q; the chat list has the IDs", a.Chat)}
		})
	return []tool.Tool{create, send}
}

// queue records a creation, keeps its message for after Mother's turn
// and returns the call's result.
func (r *run) queue(c creation) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks++
	r.res.Creations = append(r.res.Creations, c)
	r.pending = append(r.pending, c)
	what := "Started the chat"
	if c.Sent {
		what = "Sent the message to the chat"
	}
	return fmt.Sprintf("%s %s %q as task t_%d. Its reply comes in a finish notice; there is no need to poll.", what, c.Chat, c.Title, r.tasks)
}

func json_(s string, v any) error { return json.Unmarshal([]byte(s), v) }
