package scenario

// SPIKE-046: training examples from scenario runs. With Options.Examples a
// run keeps every request the providers answered, with the answer, and
// writes them beside the transcript as <scenario>_r<rep>.examples.jsonl.
// Export then gathers the passing runs of training sets into chat.jsonl
// and subagents.jsonl. Spike-only: the app doesn't use any of it.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/provider"
)

// Example is one request and the model's answer, in the chat format of
// Ollama's /api/chat and most fine-tuning tools: the system prompt as one
// system message, the messages, and the answer as the last assistant
// message. Thinking is left out.
type Example struct {
	Messages []ExampleMessage `json:"messages"`
	Tools    []ExampleTool    `json:"tools,omitempty"`
	Meta     ExampleMeta      `json:"meta"`
}

// ExampleMessage is a message as Ollama's /api/chat takes it.
type ExampleMessage struct {
	Role      string        `json:"role"`
	Content   string        `json:"content"`
	ToolCalls []ExampleCall `json:"tool_calls,omitempty"`
	ToolName  string        `json:"tool_name,omitempty"` // a tool result's tool
}

// ExampleCall is a tool call, its arguments an object.
type ExampleCall struct {
	Type     string          `json:"type"`
	Function ExampleFunction `json:"function"`
}

// ExampleTool is a tool the request offered.
type ExampleTool struct {
	Type     string          `json:"type"`
	Function ExampleFunction `json:"function"`
}

// ExampleFunction is a tool's definition or a call's name and arguments.
type ExampleFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
}

// ExampleMeta says where an example is from. Agent is "chat" for the chat
// under test, else the subagent's name; Turn and Request are the chat's.
type ExampleMeta struct {
	Set      string `json:"set,omitempty"`
	Split    string `json:"split,omitempty"`
	Scenario string `json:"scenario,omitempty"`
	Model    string `json:"model,omitempty"`
	Harness  string `json:"harness,omitempty"`
	Rep      int    `json:"rep,omitempty"`
	Agent    string `json:"agent"`
	Turn     int    `json:"turn,omitempty"`
	Request  int    `json:"request,omitempty"`
	Stop     string `json:"stop"`
}

// AgentChat is ExampleMeta.Agent for the chat under test.
const AgentChat = "chat"

// exchange is a request a provider answered in full, and its answer.
type exchange struct {
	key   [32]byte // the request's messages and tools, to find the chat's traced requests
	ex    Example
	agent string // the system's one block's name, for a subagent
	one   bool   // the system is one block, as a subagent's is
}

// capture keeps a run's exchanges.
type capture struct {
	mu  sync.Mutex
	got []exchange
}

func capturing(backends map[provider.Kind]provider.Factory, c *capture) map[provider.Kind]provider.Factory {
	out := make(map[provider.Kind]provider.Factory, len(backends))
	for k, f := range backends {
		out[k] = func(conn provider.Connection) (provider.Provider, error) {
			p, err := f(conn)
			if err != nil {
				return nil, err
			}
			return capturer{p, c}, nil
		}
	}
	return out
}

type capturer struct {
	provider.Provider
	c *capture
}

func (c capturer) Stream(ctx context.Context, req provider.Request) iter.Seq2[provider.Event, error] {
	// The request as it is now: the agent appends to its messages later.
	ex := exampleOf(req)
	key := exampleKey(ex)
	return func(yield func(provider.Event, error) bool) {
		var parts []chat.Part
		for ev, err := range c.Provider.Stream(ctx, req) {
			if err == nil {
				switch ev.Kind {
				case provider.EventPart:
					if ev.Part != nil {
						parts = append(parts, *ev.Part)
					}
				case provider.EventDone:
					ex.Messages = append(ex.Messages, assistantMessage(parts))
					ex.Meta.Stop = string(ev.Stop)
					x := exchange{key: key, ex: ex, one: len(req.System) == 1}
					if x.one {
						x.agent = req.System[0].Name
					}
					c.c.mu.Lock()
					c.c.got = append(c.c.got, x)
					c.c.mu.Unlock()
				}
			}
			if !yield(ev, err) {
				return
			}
		}
	}
}

// exampleOf is a request in the chat format, as the Ollama backend sends
// it (provider/ollama body): the system blocks joined into one message, a
// user message's text and notices joined, no tools when the request asks
// for an answer without calls.
func exampleOf(req provider.Request) Example {
	var ex Example
	if req.ToolChoice != provider.ToolsNone {
		for _, t := range req.Tools {
			ex.Tools = append(ex.Tools, ExampleTool{Type: "function", Function: ExampleFunction{Name: t.Name, Description: t.Description, Parameters: t.Schema}})
		}
	}
	var sys []string
	for _, b := range req.System {
		sys = append(sys, b.Text)
	}
	if len(sys) > 0 {
		ex.Messages = append(ex.Messages, ExampleMessage{Role: "system", Content: strings.Join(sys, "\n\n")})
	}
	names := map[string]string{} // call ID → tool, for tool_name
	for _, m := range req.Messages {
		switch m.Role {
		case chat.RoleUser:
			var text []string
			for _, p := range m.Parts {
				switch {
				case p.Text != nil:
					text = append(text, p.Text.Text)
				case p.Notice != nil:
					text = append(text, p.Notice.Text)
				}
			}
			ex.Messages = append(ex.Messages, ExampleMessage{Role: "user", Content: strings.Join(text, "\n\n")})
		case chat.RoleAssistant:
			for _, p := range m.Parts {
				if p.ToolCall != nil {
					names[p.ToolCall.ID] = p.ToolCall.Name
				}
			}
			ex.Messages = append(ex.Messages, assistantMessage(m.Parts))
		case chat.RoleTool:
			for _, p := range m.Parts {
				if p.ToolResult != nil {
					ex.Messages = append(ex.Messages, ExampleMessage{Role: "tool", Content: p.ToolResult.Text, ToolName: names[p.ToolResult.CallID]})
				}
			}
		}
	}
	return ex
}

// assistantMessage is an assistant message's text and calls, without its
// thinking.
func assistantMessage(parts []chat.Part) ExampleMessage {
	m := ExampleMessage{Role: "assistant"}
	var text strings.Builder
	for _, p := range parts {
		switch {
		case p.Text != nil:
			text.WriteString(p.Text.Text)
		case p.ToolCall != nil:
			m.ToolCalls = append(m.ToolCalls, ExampleCall{Type: "function", Function: ExampleFunction{Name: p.ToolCall.Name, Arguments: provider.ObjectArgs(p.ToolCall.Args)}})
		}
	}
	m.Content = text.String()
	return m
}

func exampleKey(ex Example) [32]byte {
	b, _ := json.Marshal(struct {
		M []ExampleMessage
		T []ExampleTool
	}{ex.Messages, ex.Tools})
	return sha256.Sum256(b)
}

// examples are the run's exchanges as JSONL: the chat's traced requests,
// with their turn, and the subagents'. Others, such as a title's or
// Refresh memory's, are left out.
func (c *capture) examples(rec *record) string {
	if c == nil {
		return ""
	}
	chatReq := map[[32]byte]int{} // key → index in rec.requests
	for i, q := range rec.reqs {
		if k := exampleKey(exampleOf(q)); len(q.Messages) > 0 {
			if _, ok := chatReq[k]; !ok {
				chatReq[k] = i
			}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var b bytes.Buffer
	for _, x := range c.got {
		ex := x.ex
		if i, ok := chatReq[x.key]; ok {
			ex.Meta.Agent, ex.Meta.Request = AgentChat, i+1
			if i < len(rec.reqTurn) {
				ex.Meta.Turn = rec.reqTurn[i]
			}
		} else if x.one && x.agent != "" && len(ex.Tools) > 0 {
			ex.Meta.Agent = x.agent
		} else {
			continue
		}
		line, err := json.Marshal(ex)
		if err != nil {
			continue
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// ExamplesFile is a run's examples file, beside its transcript.
func ExamplesFile(stampDir string, r Run) string {
	d := filepath.Join(stampDir, unsafeName.ReplaceAllString(r.Model, "_"))
	if r.Harness != "" {
		d = filepath.Join(d, r.Harness)
	}
	return filepath.Join(d, fmt.Sprintf("%s_r%d.examples.jsonl", r.Scenario, r.Rep))
}

// ExportOptions are what Export gathers and how it checks it.
type ExportOptions struct {
	Out string // the folder for chat.jsonl and subagents.jsonl
	// Splits are the sets' splits to take; empty is train alone. A report
	// of another split is an error, so a held-out set doesn't train by
	// mistake.
	Splits []string
	// Scrub fails the export on personal IDs; nil checks nothing.
	Scrub *Scrubber
}

// ExportStats are what Export wrote.
type ExportStats struct {
	Reports, Runs, Passed, Chat, Subagent int
}

// Export reads reports (their JSON files), takes each passing run's
// examples, adds where they are from, and writes them as
// <out>/chat.jsonl and <out>/subagents.jsonl. A run without an examples
// file (run without -examples) is an error. If the scrub check finds a
// personal ID, nothing is written.
func Export(reports []string, opt ExportOptions) (ExportStats, error) {
	var st ExportStats
	splits := opt.Splits
	if len(splits) == 0 {
		splits = []string{"train"}
	}
	var chatOut, subOut bytes.Buffer
	var found []string
	for _, path := range reports {
		rep, err := ReadReport(path)
		if err != nil {
			return st, err
		}
		if !slices.Contains(splits, rep.Split) {
			return st, fmt.Errorf("%s: set %q is split %q, not %s; held-out sets never train", path, rep.Set, rep.Split, strings.Join(splits, " or "))
		}
		st.Reports++
		stampDir := strings.TrimSuffix(path, filepath.Ext(path))
		for _, r := range rep.Runs {
			if r.Skipped != "" {
				continue
			}
			st.Runs++
			if !r.Passed() {
				continue
			}
			st.Passed++
			f := ExamplesFile(stampDir, r)
			b, err := os.ReadFile(f)
			if errors.Is(err, os.ErrNotExist) {
				return st, fmt.Errorf("%s: no examples for %s r%d; run with -examples", path, r.Scenario, r.Rep)
			} else if err != nil {
				return st, err
			}
			sc := bufio.NewScanner(bytes.NewReader(b))
			sc.Buffer(nil, 64<<20)
			for n := 1; sc.Scan(); n++ {
				var ex Example
				if err := json.Unmarshal(sc.Bytes(), &ex); err != nil {
					return st, fmt.Errorf("%s:%d: %w", f, n, err)
				}
				if ex.Meta.Stop != string(provider.StopEnd) && ex.Meta.Stop != string(provider.StopToolUse) {
					continue // cut at max tokens, or refused
				}
				ex.Meta.Set, ex.Meta.Split, ex.Meta.Scenario, ex.Meta.Model, ex.Meta.Harness, ex.Meta.Rep = rep.Set, rep.Split, r.Scenario, r.Model, r.Harness, r.Rep
				line, err := json.Marshal(ex)
				if err != nil {
					return st, err
				}
				if what := opt.Scrub.Find(string(line)); what != "" {
					found = append(found, fmt.Sprintf("%s:%d: %s", f, n, what))
					continue
				}
				out := &chatOut
				if ex.Meta.Agent == AgentChat {
					st.Chat++
				} else {
					out, st.Subagent = &subOut, st.Subagent+1
				}
				out.Write(line)
				out.WriteByte('\n')
			}
			if err := sc.Err(); err != nil {
				return st, fmt.Errorf("%s: %w", f, err)
			}
		}
	}
	if len(found) > 0 {
		return st, fmt.Errorf("personal IDs in the examples; nothing written:\n  %s", strings.Join(found, "\n  "))
	}
	if err := os.MkdirAll(opt.Out, 0o755); err != nil {
		return st, err
	}
	if err := os.WriteFile(filepath.Join(opt.Out, "chat.jsonl"), chatOut.Bytes(), 0o644); err != nil {
		return st, err
	}
	return st, os.WriteFile(filepath.Join(opt.Out, "subagents.jsonl"), subOut.Bytes(), 0o644)
}

// Scrubber finds personal IDs in text: e-mail addresses, home folders,
// API keys, and the literal values it is given, such as the git user's
// name. Find names what it found, never the value.
type Scrubber struct {
	rules []scrubRule
}

type scrubRule struct {
	name string
	re   *regexp.Regexp
}

var builtinScrub = []scrubRule{
	{"an e-mail address", regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`)},
	{"a home folder", regexp.MustCompile(`(?i)\b[A-Z]:(?:\\\\|\\|/)(?:Users|Documents and Settings)(?:\\\\|\\|/)|(?:^|[\s"'(=:])/(?:home|Users)/[^/\s"']+`)},
	{"an API key", regexp.MustCompile(`\b(?:sk-(?:ant-|proj-)?[A-Za-z0-9_-]{20,}|AIza[0-9A-Za-z_-]{30,}|nvapi-[A-Za-z0-9_-]{20,}|hf_[A-Za-z0-9]{20,}|gsk_[A-Za-z0-9]{20,}|ghp_[A-Za-z0-9]{20,}|xox[abp]-[A-Za-z0-9-]{10,})`)},
}

// NewScrubber is the built-in rules, each literal (matched as a whole
// word, ignoring case; empty ones and ones under 3 characters are
// skipped), and each extra regex.
func NewScrubber(literals map[string]string, deny []string) (*Scrubber, error) {
	s := &Scrubber{rules: slices.Clone(builtinScrub)}
	names := make([]string, 0, len(literals))
	for name := range literals {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		v := strings.TrimSpace(literals[name])
		if len([]rune(v)) < 3 {
			continue
		}
		s.rules = append(s.rules, scrubRule{name, regexp.MustCompile(`(?i)(?:^|\W)` + regexp.QuoteMeta(v) + `(?:\W|$)`)})
	}
	for _, d := range deny {
		re, err := regexp.Compile(d)
		if err != nil {
			return nil, fmt.Errorf("deny %q: %w", d, err)
		}
		s.rules = append(s.rules, scrubRule{"the deny pattern " + d, re})
	}
	return s, nil
}

// Find names the first rule text breaks, or "" if none. A nil Scrubber
// finds nothing.
func (s *Scrubber) Find(text string) string {
	if s == nil {
		return ""
	}
	for _, r := range s.rules {
		if r.re.MatchString(text) {
			return r.name
		}
	}
	return ""
}
