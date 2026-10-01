package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// This file is the Provider interface that Burrow would own. Every option in the spike is an
// adapter behind it, so the same agent loop and the same tests run on each of them.

type PartType string

const (
	PText       PartType = "text"
	PImage      PartType = "image"
	PThinking   PartType = "thinking"
	PToolCall   PartType = "tool_call"
	PToolResult PartType = "tool_result"
)

type Part struct {
	Type      PartType
	Text      string // text, thinking
	Signature string // thinking
	MIME      string // image
	Data      []byte // image (raw bytes)
	CallID    string // tool_call, tool_result
	Name      string // tool_call
	Args      string // tool_call: raw JSON
	Content   []Part // tool_result: text and image parts
	IsError   bool   // tool_result
	Cache     bool   // Anthropic: cache point after this part
}

type Message struct {
	Role  string // user, assistant, tool
	Parts []Part
}

type ToolDef struct {
	Name        string
	Description string
	Schema      map[string]any
}

type Request struct {
	Model          string
	System         []Part // context blocks 1-5 (SPEC 3.1), each its own block
	Messages       []Message
	Tools          []ToolDef
	MaxTokens      int
	ThinkingBudget int
}

type EventKind string

const (
	EvText      EventKind = "text"
	EvThinking  EventKind = "thinking"
	EvToolStart EventKind = "tool_start"
	EvToolDelta EventKind = "tool_delta"
	EvToolEnd   EventKind = "tool_end"
	EvUsage     EventKind = "usage"
	EvStop      EventKind = "stop"
)

type Event struct {
	Kind   EventKind
	Index  int
	Text   string // text, thinking, tool arg delta
	CallID string
	Name   string
	Usage  Usage
	Stop   string
}

type Usage struct {
	Input, Output, CacheWrite, CacheRead int
}

type Response struct {
	Message Message
	Usage   Usage
	Stop    string
}

type Provider interface {
	// Stream sends one request and returns when the response is complete. It never runs tools.
	Stream(ctx context.Context, req *Request, on func(Event)) (*Response, error)
}

// ProviderError is what the app would show. Adapters fill it from the library's error.
type ProviderError struct {
	Status     int
	Type       string
	RetryAfter time.Duration
	HasRetry   bool
	Msg        string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("status %d %s: %s", e.Status, e.Type, e.Msg)
}

var errUnsupported = errors.New("not supported by this option")

type Kind string

const (
	Anthropic Kind = "anthropic"
	OpenAI    Kind = "openai"
)

type Options struct {
	Kind       Kind
	BaseURL    string // root of the mock, e.g. http://127.0.0.1:1234; adapters add /v1 where the library needs it
	APIKey     string
	Model      string
	MaxRetries int // -1 = library default
}

type Option struct {
	Name     string
	Module   string
	Version  string
	Released string // from proxy.golang.org @latest
	License  string
	New      func(o Options) (Provider, error)
	// Classify turns the library's error into our ProviderError, if the library exposes enough.
	Classify func(err error) *ProviderError
}
