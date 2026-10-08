// Package provider is Jenab's side of the LLM APIs (SPEC 3.8). Each backend
// (provider/anthropic, provider/openai, provider/ollama, provider/fake)
// implements Provider and handles its own protocol only. The Registry adds
// the rules every provider shares: the limits, the pause per provider, the
// stall timeouts, the truncation check and secret redaction.
package provider

import (
	"context"
	"encoding/json"
	"iter"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
)

// Provider is one connected API.
//
// Stream sends one request and yields its events: deltas while text and
// thinking arrive, each part once it is finished (in message order), and
// EventDone last. A stream that ends without the API's end marker, or whose
// tool-call arguments are not valid JSON, ends with a Transport error
// instead (SPEC 3.8). Stream never retries; it yields at most one error, and
// nothing after it.
type Provider interface {
	Stream(ctx context.Context, req Request) iter.Seq2[Event, error]
	// Models lists the models the key can use.
	Models(ctx context.Context) ([]ModelInfo, error)
}

// Request is one model request.
type Request struct {
	Model     string // the provider's model ID; the Registry resolves aliases and "provider/model"
	System    []Block
	Messages  []chat.Message
	Tools     []ToolDef
	MaxTokens int
	// Thinking asks for the model's thinking where it has it. False turns
	// it off where the API allows (Ollama: think false).
	Thinking bool
	// Known is the model's catalog entry, set by the Registry; nil for a
	// model the catalog doesn't know. Backends read protocol facts from it.
	Known *Model
	// Context is the model's context window in tokens, set by the Registry
	// from the catalog or the settings. Ollama sends it as num_ctx.
	Context int
	// Image reads an image part's bytes from the bucket. Images in the
	// messages need it.
	Image func(ctx context.Context, ref string) ([]byte, error)
}

// Block is a piece of the system prompt. Cache marks a cache point after it
// (SPEC 3.1); APIs without explicit cache points ignore it.
type Block struct {
	Text  string
	Cache bool
	Name  string // what the block is, for the turn inspector (SPEC 8.4); not sent
}

// ToolDef is a tool as the model sees it.
type ToolDef struct {
	Name        string
	Description string
	Schema      json.RawMessage // JSON Schema of the arguments, an object
}

// EventKind says what an Event carries.
type EventKind string

const (
	// EventDelta: Text is new text of the part being written; PartKind is
	// text or thinking. For the screen only; the parts are what is stored.
	EventDelta EventKind = "delta"
	// EventPart: Part is a finished part: text, thinking (with its
	// signature) or a tool call (with its extras).
	EventPart EventKind = "part"
	// EventDone: Usage and Stop. Always the last event of a complete stream.
	EventDone EventKind = "done"
	// EventWait: the provider is paused after a rate limit or an
	// overload (Paused says which); Wait is how long until the request is
	// sent. Wait 0 means the wait is over and the request is sent now.
	// Only the Registry sends it.
	EventWait EventKind = "wait"
)

// Event is one streamed event.
type Event struct {
	Kind     EventKind
	PartKind chat.PartKind
	Text     string
	Part     *chat.Part
	Usage    *chat.Usage
	Stop     StopReason
	Wait     time.Duration
	Paused   ErrorKind // EventWait: why the provider is paused
}

// StopReason is why the model stopped.
type StopReason string

const (
	StopEnd       StopReason = "end"        // the answer is complete
	StopToolUse   StopReason = "tool_use"   // it wants its tool calls run
	StopMaxTokens StopReason = "max_tokens" // cut at MaxTokens
	StopRefused   StopReason = "refused"    // a content filter or refusal
)

// ModelInfo is a model the provider lists, with what it reports about it.
// Zero means not reported.
type ModelInfo struct {
	ID      string
	Name    string
	Context int
	Output  int
}
