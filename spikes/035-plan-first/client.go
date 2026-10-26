package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"

	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// SPIKE-024's go-sdk client, cut down to stdio: the fake server is one
// process with no children, so Close is enough to stop it (no Job Object).

type annotations struct {
	ReadOnlyHint    *bool `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool `json:"destructiveHint,omitempty"`
	OpenWorldHint   *bool `json:"openWorldHint,omitempty"`
}

type toolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations *annotations    `json:"annotations,omitempty"`
}

func (t toolInfo) readOnly() bool {
	return t.Annotations != nil && t.Annotations.ReadOnlyHint != nil && *t.Annotations.ReadOnlyHint
}

// destructive and openWorld use MCP's defaults: a tool that isn't
// read-only deletes and reaches outside unless its hints say otherwise.
func (t toolInfo) destructive() bool {
	if t.readOnly() {
		return false
	}
	return t.Annotations == nil || t.Annotations.DestructiveHint == nil || *t.Annotations.DestructiveHint
}

func (t toolInfo) openWorld() bool {
	if t.readOnly() {
		return false
	}
	return t.Annotations == nil || t.Annotations.OpenWorldHint == nil || *t.Annotations.OpenWorldHint
}

type callResult struct {
	Text    string
	IsError bool
}

type client struct{ s *gosdk.ClientSession }

func stdio(ctx context.Context, cmd *exec.Cmd) (*client, error) {
	c := gosdk.NewClient(&gosdk.Implementation{Name: "jenab-spike035", Version: "0.1"}, nil)
	s, err := c.Connect(ctx, &gosdk.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return nil, err
	}
	return &client{s: s}, nil
}

func (c *client) Tools(ctx context.Context) ([]toolInfo, error) {
	var out []toolInfo
	for t, err := range c.s.Tools(ctx, nil) {
		if err != nil {
			return out, err
		}
		b, err := json.Marshal(t)
		if err != nil {
			return out, err
		}
		var ti toolInfo
		if err := json.Unmarshal(b, &ti); err != nil {
			return out, err
		}
		out = append(out, ti)
	}
	return out, nil
}

func (c *client) Call(ctx context.Context, name string, args json.RawMessage) (callResult, error) {
	r, err := c.s.CallTool(ctx, &gosdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return callResult{}, err
	}
	var texts []string
	for _, ct := range r.Content {
		if t, ok := ct.(*gosdk.TextContent); ok {
			texts = append(texts, t.Text)
		}
	}
	return callResult{Text: strings.Join(texts, "\n"), IsError: r.IsError}, nil
}

func (c *client) Close() error { return c.s.Close() }
