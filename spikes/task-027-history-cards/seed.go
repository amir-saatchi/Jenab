package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

// seed writes the turns into chat c as the app would have: the user's
// message, the calls, their results and the answer.
func seed(ctx context.Context, db *store.ChatsDB, c id.Chat, turns []seedTurn) error {
	text := func(s string) []chat.Part { return []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: s}}} }
	for i, t := range turns {
		n := i + 1
		ms := []chat.Message{{Chat: c, Turn: n, Role: chat.RoleUser, Parts: text(t.User)}}
		if len(t.Calls) > 0 {
			var calls, results []chat.Part
			for k, sc := range t.Calls {
				raw, err := json.Marshal(sc.Args)
				if err != nil {
					return err
				}
				cid := fmt.Sprintf("seed-%d-%d", n, k+1)
				calls = append(calls, chat.Part{Kind: chat.PartToolCall, ToolCall: &chat.ToolCall{ID: cid, Name: sc.Name, Args: raw}})
				results = append(results, chat.Part{Kind: chat.PartToolResult, ToolResult: &chat.ToolResult{CallID: cid, Text: sc.Result}})
			}
			ms = append(ms,
				chat.Message{Chat: c, Turn: n, Role: chat.RoleAssistant, Parts: calls},
				chat.Message{Chat: c, Turn: n, Role: chat.RoleTool, Parts: results})
		}
		ms = append(ms, chat.Message{Chat: c, Turn: n, Role: chat.RoleAssistant, Parts: text(t.Answer)})
		for _, m := range ms {
			if _, _, err := db.AppendMessage(ctx, m); err != nil {
				return fmt.Errorf("turn %d: %w", n, err)
			}
		}
	}
	return nil
}
