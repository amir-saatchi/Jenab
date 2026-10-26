package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	m3client "github.com/mark3labs/mcp-go/client"
	m3transport "github.com/mark3labs/mcp-go/client/transport"
	m3 "github.com/mark3labs/mcp-go/mcp"
	gosdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The two SDKs behind one small interface, so the same checks and the
// same agent tools run on either. Tools and results are read through
// their JSON, so both SDKs are judged on what is on the wire.

type annotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

type toolInfo struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations *annotations    `json:"annotations,omitempty"`
}

func (t toolInfo) readOnly() bool {
	return t.Annotations != nil && t.Annotations.ReadOnlyHint != nil && *t.Annotations.ReadOnlyHint
}

// destructive is the MCP default: a tool that isn't read-only is
// destructive unless it says destructiveHint: false.
func (t toolInfo) destructive() bool {
	if t.readOnly() {
		return false
	}
	return t.Annotations == nil || t.Annotations.DestructiveHint == nil || *t.Annotations.DestructiveHint
}

type callResult struct {
	Text    string
	Images  []image
	IsError bool
}

type image struct {
	MIME string
	Data string // base64
}

type progressFunc func(progress, total float64, msg string)

type mcpClient interface {
	SDK() string
	Version() string // the negotiated protocol version
	Tools(ctx context.Context) ([]toolInfo, error)
	Call(ctx context.Context, name string, args json.RawMessage, progress progressFunc) (callResult, error)
	Close() error
	PID() int // the server process, 0 over HTTP
}

// parseResult reads a CallToolResult from its JSON.
func parseResult(v any) (callResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return callResult{}, err
	}
	var r struct {
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Data     string `json:"data"`
			MIMEType string `json:"mimeType"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return callResult{}, err
	}
	out := callResult{IsError: r.IsError}
	var texts []string
	for _, c := range r.Content {
		switch c.Type {
		case "text":
			texts = append(texts, c.Text)
		case "image":
			out.Images = append(out.Images, image{MIME: c.MIMEType, Data: c.Data})
		default:
			texts = append(texts, "["+c.Type+" content]")
		}
	}
	out.Text = strings.Join(texts, "\n")
	return out, nil
}

func toToolInfo(v any) (toolInfo, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return toolInfo{}, err
	}
	var t toolInfo
	return t, json.Unmarshal(b, &t)
}

var tokens atomic.Int64

// ---- modelcontextprotocol/go-sdk ----

type goClient struct {
	s        *gosdk.ClientSession
	pid      int
	mu       sync.Mutex
	progress map[string]progressFunc
}

// jobTransport starts the command suspended, puts it in the job, then
// resumes it.
type jobTransport struct {
	inner *gosdk.CommandTransport
	job   *job
}

func (t *jobTransport) Connect(ctx context.Context) (gosdk.Connection, error) {
	if t.job != nil {
		suspended(t.inner.Command)
	}
	c, err := t.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	if t.job != nil {
		if err := t.job.adopt(t.inner.Command.Process.Pid); err != nil {
			c.Close()
			return nil, err
		}
	}
	return c, nil
}

func newGoClient() (*goClient, *gosdk.Client) {
	g := &goClient{progress: map[string]progressFunc{}}
	c := gosdk.NewClient(&gosdk.Implementation{Name: "jenab-spike024", Version: "0.1"}, &gosdk.ClientOptions{
		Capabilities: &gosdk.ClientCapabilities{},
		ProgressNotificationHandler: func(_ context.Context, r *gosdk.ProgressNotificationClientRequest) {
			g.mu.Lock()
			f := g.progress[fmt.Sprint(r.Params.ProgressToken)]
			g.mu.Unlock()
			if f != nil {
				f(r.Params.Progress, r.Params.Total, r.Params.Message)
			}
		},
	})
	return g, c
}

func goStdio(ctx context.Context, cmd *exec.Cmd, j *job) (*goClient, error) {
	g, c := newGoClient()
	t := &jobTransport{inner: &gosdk.CommandTransport{Command: cmd}, job: j}
	s, err := c.Connect(ctx, t, nil)
	if err != nil {
		if cmd.Process != nil {
			g.pid = cmd.Process.Pid
		}
		return g, err
	}
	g.s, g.pid = s, cmd.Process.Pid
	return g, nil
}

func goHTTP(ctx context.Context, url string) (*goClient, error) {
	g, c := newGoClient()
	s, err := c.Connect(ctx, &gosdk.StreamableClientTransport{Endpoint: url}, nil)
	if err != nil {
		return nil, err
	}
	g.s = s
	return g, nil
}

func (g *goClient) SDK() string { return "go-sdk" }
func (g *goClient) PID() int    { return g.pid }

func (g *goClient) Version() string {
	if r := g.s.InitializeResult(); r != nil {
		return r.ProtocolVersion
	}
	return ""
}

func (g *goClient) Tools(ctx context.Context) ([]toolInfo, error) {
	var out []toolInfo
	for t, err := range g.s.Tools(ctx, nil) {
		if err != nil {
			return out, err
		}
		ti, err := toToolInfo(t)
		if err != nil {
			return out, err
		}
		out = append(out, ti)
	}
	return out, nil
}

func (g *goClient) Call(ctx context.Context, name string, args json.RawMessage, progress progressFunc) (callResult, error) {
	p := &gosdk.CallToolParams{Name: name, Arguments: args}
	if progress != nil {
		tok := fmt.Sprintf("p%d", tokens.Add(1))
		p.SetProgressToken(tok)
		g.mu.Lock()
		g.progress[tok] = progress
		g.mu.Unlock()
		defer func() {
			g.mu.Lock()
			delete(g.progress, tok)
			g.mu.Unlock()
		}()
	}
	r, err := g.s.CallTool(ctx, p)
	if err != nil {
		return callResult{}, err
	}
	return parseResult(r)
}

func (g *goClient) Close() error {
	if g.s == nil {
		return nil
	}
	return g.s.Close()
}

// ---- mark3labs/mcp-go ----

type m3Client struct {
	c        *m3client.Client
	version  string
	pid      int
	mu       sync.Mutex
	progress map[string]progressFunc
}

func (m *m3Client) init(ctx context.Context) error {
	m.progress = map[string]progressFunc{}
	m.c.OnNotification(func(n m3.JSONRPCNotification) {
		if n.Method != "notifications/progress" {
			return
		}
		f := n.Params.AdditionalFields
		m.mu.Lock()
		fn := m.progress[fmt.Sprint(f["progressToken"])]
		m.mu.Unlock()
		if fn != nil {
			p, _ := f["progress"].(float64)
			t, _ := f["total"].(float64)
			msg, _ := f["message"].(string)
			fn(p, t, msg)
		}
	})
	if err := m.c.Start(ctx); err != nil {
		return err
	}
	req := m3.InitializeRequest{}
	req.Params.ClientInfo = m3.Implementation{Name: "jenab-spike024", Version: "0.1"}
	r, err := m.c.Initialize(ctx, req)
	if err != nil {
		return err
	}
	m.version = r.ProtocolVersion
	return nil
}

func m3Stdio(ctx context.Context, name string, args []string, j *job) (*m3Client, error) {
	var cmd *exec.Cmd
	t := m3transport.NewStdioWithOptions(name, nil, args, m3transport.WithCommandFunc(
		func(ctx context.Context, command string, env, args []string) (*exec.Cmd, error) {
			cmd = exec.Command(command, args...)
			if j != nil {
				suspended(cmd)
			}
			return cmd, nil
		}))
	m := &m3Client{}
	if err := t.Start(ctx); err != nil {
		return m, err
	}
	m.pid = cmd.Process.Pid
	if j != nil {
		if err := j.adopt(m.pid); err != nil {
			t.Close()
			return m, err
		}
	}
	m.c = m3client.NewClient(t)
	return m, m.init(ctx)
}

// m3HTTP connects over Streamable HTTP; version pins the protocol
// ("" lets the SDK pick).
func m3HTTP(ctx context.Context, url, version string) (*m3Client, error) {
	t, err := m3transport.NewStreamableHTTP(url)
	if err != nil {
		return nil, err
	}
	var opts []m3client.ClientOption
	if version != "" {
		opts = append(opts, m3client.WithProtocolVersion(version))
	}
	m := &m3Client{c: m3client.NewClient(t, opts...)}
	return m, m.init(ctx)
}

func (m *m3Client) SDK() string     { return "mcp-go" }
func (m *m3Client) PID() int        { return m.pid }
func (m *m3Client) Version() string { return m.version }

func (m *m3Client) Tools(ctx context.Context) ([]toolInfo, error) {
	r, err := m.c.ListTools(ctx, m3.ListToolsRequest{})
	if err != nil {
		return nil, err
	}
	var out []toolInfo
	for _, t := range r.Tools {
		ti, err := toToolInfo(t)
		if err != nil {
			return out, err
		}
		out = append(out, ti)
	}
	return out, nil
}

func (m *m3Client) Call(ctx context.Context, name string, args json.RawMessage, progress progressFunc) (callResult, error) {
	req := m3.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	if progress != nil {
		tok := fmt.Sprintf("p%d", tokens.Add(1))
		req.Params.Meta = &m3.Meta{ProgressToken: tok}
		m.mu.Lock()
		m.progress[tok] = progress
		m.mu.Unlock()
		defer func() {
			m.mu.Lock()
			delete(m.progress, tok)
			m.mu.Unlock()
		}()
	}
	r, err := m.c.CallTool(ctx, req)
	if err != nil {
		return callResult{}, err
	}
	return parseResult(r)
}

func (m *m3Client) Close() error {
	if m.c == nil {
		return nil
	}
	return m.c.Close()
}
