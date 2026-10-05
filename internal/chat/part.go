package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
)

// PartKind is the type column of message_parts (SPEC 2.3).
type PartKind string

const (
	PartText       PartKind = "text"
	PartThinking   PartKind = "thinking"
	PartToolCall   PartKind = "tool_call"
	PartToolResult PartKind = "tool_result"
	PartImage      PartKind = "image"
	PartNotice     PartKind = "notice"
	PartApproval   PartKind = "approval"
	PartQuestion   PartKind = "question"
)

// Part is one part of a message: Kind plus exactly one matching field (Q7).
// JSON, the generated TypeScript and the database stay plain this way.
type Part struct {
	Kind       PartKind    `json:"kind"`
	Text       *Text       `json:"text,omitempty"`
	Thinking   *Thinking   `json:"thinking,omitempty"`
	ToolCall   *ToolCall   `json:"tool_call,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`
	Image      *Image      `json:"image,omitempty"`
	Notice     *Notice     `json:"notice,omitempty"`
	Approval   *Approval   `json:"approval,omitempty"`
	Question   *Question   `json:"question,omitempty"`
}

// Text is text the model or the user wrote.
type Text struct {
	Text    string `json:"text"`
	Stopped bool   `json:"stopped,omitempty"` // cut off by Stop and kept (8.3)
}

// Thinking is the model's thinking. It is sent back to the provider
// unchanged, signature included, before its tool calls (SPEC 3.8).
type Thinking struct {
	Text      string `json:"text"`
	Signature string `json:"signature,omitempty"`
	Redacted  string `json:"redacted,omitempty"` // a redacted block's opaque data; Text is empty then
}

// ToolCall is a call the model asked for.
type ToolCall struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
	// Extra is a provider's data for the call, sent back unchanged, such as
	// Gemini's extra_content with its thought signature (SPEC 3.8). It is raw
	// JSON kept as a string: encoding/json compacts and escapes a RawMessage,
	// and a string comes back byte for byte.
	Extra string `json:"extra,omitempty"`
}

// ToolResult is a tool's answer to one call. A large output is stored in the
// bucket: Ref is its key and Text the preview (SPEC 3.7).
type ToolResult struct {
	CallID  string `json:"call_id"`
	Text    string `json:"text"`
	Ref     string `json:"ref,omitempty"`
	IsError bool   `json:"is_error,omitempty"` // also "cancelled by user" after Stop (8.3)
}

// Image is an image stored in the bucket (4.7); the bytes are never in a part.
type Image struct {
	Ref  string `json:"ref"`
	MIME string `json:"mime"`
	Alt  string `json:"alt,omitempty"`
}

// NoticeKind says what a notice reports. Readers accept kinds they don't
// know, since later versions add more.
type NoticeKind string

const (
	NoticeTaskFinished  NoticeKind = "task_finished"  // starts a finish turn (8.3)
	NoticeRunFinished   NoticeKind = "run_finished"   // a scheduled run (3.1)
	NoticePageOpened    NoticeKind = "page_opened"    // the user opened a page or changed its filters (5.9)
	NoticeMemoryChanged NoticeKind = "memory_changed" // between cuts (3.6)
	NoticeApprovalLevel NoticeKind = "approval_level" // 8.8
	NoticeEarlyStop     NoticeKind = "early_stop"     // a turn ended after a failed save (10)
	NoticeTurnFailed    NoticeKind = "turn_failed"    // a provider error stopped the turn; the card offers Retry (8.3)
	NoticeAnswerCut     NoticeKind = "answer_cut"     // the answer hit the output limit, was refused or was empty (8.3)
)

// Notice is a short note from the app to the agent, shown in the chat.
type Notice struct {
	Kind NoticeKind `json:"kind"`
	Text string     `json:"text"`
}

// Approval is an approval card (SPEC 8.8). Kind and Target name what is
// approved, as _jenab_approvals stores it: a host, a script's hash.
type Approval struct {
	ID      id.Approval      `json:"id"`
	Kind    string           `json:"kind"` // host, migration, starlark, connection, mcp, command
	Target  string           `json:"target"`
	Ask     string           `json:"ask"`           // what is asked
	Why     string           `json:"why,omitempty"` // one line from the agent
	Risk    string           `json:"risk"`
	Details string           `json:"details,omitempty"`
	Options []ApprovalOption `json:"options"` // e.g. "Allow for this project", "Deny"
	// The answer, once given.
	Answer     Grant      `json:"answer,omitempty"`
	Note       string     `json:"note,omitempty"` // a Deny note, or the message written instead
	By         id.Source  `json:"by,omitempty"`   // "user", or "auto" for the Auto level
	AnsweredAt *time.Time `json:"answered_at,omitempty"`
	// Stopped: Stop or the project's close ended the wait, without an answer.
	Stopped bool `json:"stopped,omitempty"`
}

// ApprovalOption is one button of an approval card.
type ApprovalOption struct {
	Label string `json:"label"`
	Grant Grant  `json:"grant"`
}

// Grant is what an approval option gives.
type Grant string

const (
	GrantOnce   Grant = "once"   // this call only
	GrantAlways Grant = "always" // kept for the project; the target isn't asked for again
	GrantDeny   Grant = "deny"
)

// Option returns the card's option with grant g.
func (a *Approval) Option(g Grant) (ApprovalOption, bool) {
	for _, o := range a.Options {
		if o.Grant == g {
			return o, true
		}
	}
	return ApprovalOption{}, false
}

// Question is a question form from ask_user (SPEC 8.8).
type Question struct {
	Questions []QuestionItem `json:"questions"`
	// Answers maps a header to the picked labels or the Other text, once
	// answered. A message written instead goes in Note.
	Answers    map[string][]string `json:"answers,omitempty"`
	Note       string              `json:"note,omitempty"`
	AnsweredAt *time.Time          `json:"answered_at,omitempty"`
	Stopped    bool                `json:"stopped,omitempty"` // as for Approval
}

// QuestionItem is one question of a form.
type QuestionItem struct {
	Header   string   `json:"header"`
	Question string   `json:"question"`
	Options  []Option `json:"options"`
	Multi    bool     `json:"multi,omitempty"`
}

// Option is one choice; an Other field is always added by the UI.
type Option struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
}

// PartError says which part of a message is invalid.
type PartError struct {
	Index int
	Err   error
}

func (e *PartError) Error() string { return fmt.Sprintf("chat: part %d: %v", e.Index, e.Err) }
func (e *PartError) Unwrap() error { return e.Err }

// ErrInvalidPart is wrapped by every error from Part.Validate.
var ErrInvalidPart = errors.New("invalid part")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidPart}, a...)...)
}

// Validate checks that exactly one field is set, that it matches Kind, and
// that the field itself is well formed.
func (p Part) Validate() error {
	set := map[PartKind]bool{
		PartText:       p.Text != nil,
		PartThinking:   p.Thinking != nil,
		PartToolCall:   p.ToolCall != nil,
		PartToolResult: p.ToolResult != nil,
		PartImage:      p.Image != nil,
		PartNotice:     p.Notice != nil,
		PartApproval:   p.Approval != nil,
		PartQuestion:   p.Question != nil,
	}
	if _, ok := set[p.Kind]; !ok {
		return invalid("unknown kind %q", p.Kind)
	}
	n := 0
	for _, s := range set {
		if s {
			n++
		}
	}
	if n != 1 || !set[p.Kind] {
		return invalid("kind %q needs exactly its own field set, %d fields are set", p.Kind, n)
	}
	switch p.Kind {
	case PartThinking:
		if p.Thinking.Redacted != "" && p.Thinking.Text != "" {
			return invalid("redacted thinking has text")
		}
	case PartToolCall:
		c := p.ToolCall
		if c.ID == "" || c.Name == "" {
			return invalid("tool call without an ID or name")
		}
		if !json.Valid(c.Args) {
			return invalid("tool call %s: arguments are not valid JSON", c.ID)
		}
		if c.Extra != "" && !json.Valid([]byte(c.Extra)) {
			return invalid("tool call %s: extra is not valid JSON", c.ID)
		}
	case PartToolResult:
		if p.ToolResult.CallID == "" {
			return invalid("tool result without a call ID")
		}
	case PartImage:
		if p.Image.Ref == "" || p.Image.MIME == "" {
			return invalid("image without a ref or MIME type")
		}
	case PartNotice:
		if p.Notice.Kind == "" || p.Notice.Text == "" {
			return invalid("notice without a kind or text")
		}
	case PartApproval:
		a := p.Approval
		return a.validate()
	case PartQuestion:
		return p.Question.validate()
	}
	return nil
}

// validate checks a card: 2 or more options with labels and different
// grants, one of them Deny, and an answer that is one of them.
func (a *Approval) validate() error {
	if a.ID == "" || a.Kind == "" || a.Target == "" || a.Ask == "" {
		return invalid("approval without an ID, kind, target or ask")
	}
	if len(a.Options) < 2 {
		return invalid("approval %s has %d options, needs at least 2", a.ID, len(a.Options))
	}
	seen := map[Grant]bool{}
	for _, o := range a.Options {
		if o.Label == "" {
			return invalid("approval %s has an option without a label", a.ID)
		}
		if o.Grant != GrantOnce && o.Grant != GrantAlways && o.Grant != GrantDeny {
			return invalid("approval %s: unknown grant %q", a.ID, o.Grant)
		}
		if seen[o.Grant] {
			return invalid("approval %s has two %s options", a.ID, o.Grant)
		}
		seen[o.Grant] = true
	}
	if !seen[GrantDeny] {
		return invalid("approval %s has no deny option", a.ID)
	}
	if _, ok := a.Option(a.Answer); a.Answer != "" && !ok {
		return invalid("approval %s: the answer %q is not an option", a.ID, a.Answer)
	}
	return nil
}

// validate checks the ask_user shape: 1–4 questions with distinct headers,
// each with 2–4 options (SPEC 8.8).
func (q *Question) validate() error {
	if len(q.Questions) < 1 || len(q.Questions) > 4 {
		return invalid("%d questions, want 1 to 4", len(q.Questions))
	}
	seen := map[string]bool{}
	for _, it := range q.Questions {
		if it.Header == "" || it.Question == "" {
			return invalid("question without a header or text")
		}
		if seen[it.Header] {
			return invalid("header %q is used twice", it.Header)
		}
		seen[it.Header] = true
		if len(it.Options) < 2 || len(it.Options) > 4 {
			return invalid("question %q has %d options, want 2 to 4", it.Header, len(it.Options))
		}
		for _, o := range it.Options {
			if o.Label == "" {
				return invalid("question %q has an option without a label", it.Header)
			}
		}
	}
	return nil
}
