package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// Tools are the agent's own tools, for the tool registry. Phase 1 has
// ask_user.
func Tools() []tool.Tool {
	return []tool.Tool{askUser()}
}

type askArgs struct {
	Questions []chat.QuestionItem `json:"questions"`
}

// askUser is ask_user (8.8): a question form in the chat. The guidance in
// its description is the same for every model (1).
func askUser() tool.Tool {
	return tool.Func(tool.Spec{
		Name: "ask_user",
		Description: "Ask the user 1 to 4 questions, each with a short header and 2 to 4 options, and wait for the answers. " +
			"An Other field for free text is always added; with multi, several options can be picked. " +
			"Ask only about choices that are the user's to make and change what you do next. " +
			"Don't ask about what you can find out, or decide with a sensible default: then decide and say so. " +
			"Don't ask to confirm a plan the user has already given. " +
			`Returns the answers by header, such as {"Currency": "EUR"}, with any note.`,
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"questions": {
					"type": "array", "minItems": 1, "maxItems": 4,
					"items": {
						"type": "object",
						"properties": {
							"header": {"type": "string", "minLength": 1, "maxLength": 40, "description": "A short label, unique in the form."},
							"question": {"type": "string", "minLength": 1},
							"options": {
								"type": "array", "minItems": 2, "maxItems": 4,
								"items": {
									"type": "object",
									"properties": {
										"label": {"type": "string", "minLength": 1},
										"description": {"type": "string"},
										"recommended": {"type": "boolean"}
									},
									"required": ["label"],
									"additionalProperties": false
								}
							},
							"multi": {"type": "boolean", "description": "Several options can be picked."}
						},
						"required": ["header", "question", "options"],
						"additionalProperties": false
					}
				}
			},
			"required": ["questions"],
			"additionalProperties": false
		}`),
		Effects: tool.AsksUser,
	}, func(ctx context.Context, env *tool.Env, a askArgs) (tool.Result, error) {
		if env.Ask == nil {
			return tool.Result{}, tool.Errorf("no one can answer here; put the open question in your result")
		}
		q := chat.Question{Questions: a.Questions}
		if err := (chat.Part{Kind: chat.PartQuestion, Question: &q}).Validate(); err != nil {
			return tool.Result{}, &tool.Error{Msg: strings.TrimPrefix(err.Error(), chat.ErrInvalidPart.Error()+": "), Err: err}
		}
		got, err := env.Ask(ctx, q)
		if err != nil {
			return tool.Result{}, err
		}
		return tool.Result{Text: answerText(got)}, nil
	})
}

// answerText is a form's answer for the model: the answers by header as
// JSON, a list for multi questions, and the note. A message written
// instead of the form is the answer then.
func answerText(q chat.Question) string {
	if len(q.Answers) == 0 {
		return "The user wrote a message instead of answering the form: " + q.Note
	}
	m := map[string]any{}
	for _, it := range q.Questions {
		v, ok := q.Answers[it.Header]
		switch {
		case !ok:
		case it.Multi:
			m[it.Header] = v
		default:
			m[it.Header] = v[0]
		}
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(m) // a map of strings can't fail
	s := strings.TrimSpace(b.String())
	if q.Note != "" {
		s += "\nNote: " + q.Note
	}
	return s
}
