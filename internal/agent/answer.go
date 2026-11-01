package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// Answering with a write (TASK-033). A tool whose Spec has Answers gets
// an answer argument. When the model's message has one and every call of
// the message succeeds, the answer is written as the turn's last message
// and the turn ends, without a request whose only work is to say "done".
// The text is the same for every model (1).

// answerArg is the argument's name.
const answerArg = "answer"

// answerProp is the argument's schema.
const answerProp = `{"type": "string", "description": "If this call finishes what the user asked: your reply to the user. ` +
	`When every call in your message succeeds, the app shows it and the turn ends. ` +
	`Empty when you need a result to reply or have more to do."}`

// answerLine joins block 1's lines on how turns work when a tool can
// carry the answer.
const answerLine = "- If a write finishes what the user asked, put your reply in its answer argument instead of answering after the result: when the write succeeds, the turn ends with it. Leave it empty while something is left to do, such as saving a changed instruction to memory first."

// answerSchemas are the tools' schemas with the argument, by the schema
// they were made from: built once, so every request sends the same bytes
// and the cached prefix holds (3.1).
var answerSchemas sync.Map // string → json.RawMessage

// toolDef is the tool as the model sees it.
func toolDef(tl tool.Tool) provider.ToolDef {
	s := tl.Spec()
	schema := s.Schema
	if s.Answers {
		schema = withAnswer(schema)
	}
	return provider.ToolDef{Name: s.Name, Description: s.Description, Schema: schema}
}

// withAnswer adds the answer argument as the schema's last property, and
// to its required ones: a model weighs a required argument at every call,
// and may leave it empty. The tool checks its arguments against its own
// schema, without the answer. A schema without an object of properties is
// left as it is.
func withAnswer(schema json.RawMessage) json.RawMessage {
	if v, ok := answerSchemas.Load(string(schema)); ok {
		return v.(json.RawMessage)
	}
	out := schema
	var top map[string]json.RawMessage
	if json.Unmarshal(schema, &top) == nil {
		props := bytes.TrimSpace(top["properties"])
		if len(props) >= 2 && props[0] == '{' && props[len(props)-1] == '}' {
			body := bytes.TrimSpace(props[1 : len(props)-1])
			sep := ", "
			if len(body) == 0 {
				sep = ""
			}
			top["properties"] = json.RawMessage(`{` + string(body) + sep + `"` + answerArg + `": ` + answerProp + `}`)
			var req []string
			_ = json.Unmarshal(top["required"], &req) // none: only the answer
			if r, err := json.Marshal(append(req, answerArg)); err == nil {
				top["required"] = r
			}
			if b, err := json.Marshal(top); err == nil {
				out = b
			}
		}
	}
	answerSchemas.Store(string(schema), out)
	return out
}

// splitAnswer takes the answer argument out of a call's arguments. Other
// arguments are kept as they were; arguments that aren't an object, or
// have no answer, come back unchanged.
func splitAnswer(args json.RawMessage) (json.RawMessage, string) {
	var m map[string]json.RawMessage
	if json.Unmarshal(args, &m) != nil {
		return args, ""
	}
	raw, ok := m[answerArg]
	if !ok {
		return args, ""
	}
	delete(m, answerArg)
	rest, err := json.Marshal(m)
	if err != nil {
		return args, ""
	}
	var answer string
	_ = json.Unmarshal(raw, &answer) // not a string: no answer
	return rest, strings.TrimSpace(answer)
}

// callAnswer is the answers that a message's calls carry, joined in their
// order, or "".
func (r *runner) callAnswer(calls []chat.ToolCall) string {
	var out []string
	for _, c := range calls {
		if tl, ok := r.o.d.Tools.Get(c.Name); ok && tl.Spec().Answers {
			if _, a := splitAnswer(c.Args); a != "" && !slices.Contains(out, a) {
				out = append(out, a)
			}
		}
	}
	return strings.Join(out, "\n\n")
}

// answerWith ends the turn with the answer a write carried: it is stored
// as the turn's last assistant message, as if a request had returned it,
// and gets the number check as such an answer does (SPIKE-032).
func (r *runner) answerWith(ctx context.Context, t *turn, req provider.Request, answer string) error {
	// The ID is made under cs.mu, as a try's is, so a message the user
	// sends now sorts after the answer.
	r.cs.mu.Lock()
	mid := id.Message(id.New())
	r.cs.mu.Unlock()
	m := chat.Message{ID: mid, Chat: t.ch.ID, Turn: t.n, Role: chat.RoleAssistant, Model: t.model,
		Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: answer}}}}
	if err := r.writeMessage(ctx, m); err != nil {
		return err
	}
	if r.o.d.Grounding.Mode != GroundingOff {
		t.said = append(t.said, answer)
	}
	return r.ground(ctx, t, req)
}
