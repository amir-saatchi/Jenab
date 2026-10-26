package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/backends"
	"github.com/amir-saatchi/jenab/internal/scenario"
	"github.com/amir-saatchi/jenab/internal/secret"
	"github.com/amir-saatchi/jenab/internal/store"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// One agent turn on the app's real orchestrator, with MCP tools, the way
// SPIKE-029 drives it: a new project and chat, one message, cards
// answered by a scripted user, and the chat and traces read back.

type harness struct {
	models  *provider.Registry
	llm     config.LLMSettings
	temp    string
	log     *slog.Logger
	redact  func(string) string
	timeout time.Duration
}

type agentRun struct {
	Model    string
	Style    string // native or describe
	Prompt   string
	Servers  []*server
	Instr    map[string]string // server instructions for block 4, by server
	Policy   func(c card) (chat.Grant, string)
	PreGrant []string // targets the project already has an Always for
}

type call struct {
	Tool   string `json:"tool"`
	Args   string `json:"args"`
	Error  bool   `json:"error,omitempty"`
	Result string `json:"result,omitempty"`
}

type cardRec struct {
	Tool     string     `json:"tool"`
	Args     string     `json:"args"`
	ReadOnly bool       `json:"read_only"`
	Grant    chat.Grant `json:"grant"`
	Note     string     `json:"note,omitempty"`
}

type runOut struct {
	Calls    []call    `json:"calls"`
	Cards    []cardRec `json:"cards"`
	Answer   string    `json:"answer"`
	Requests int       `json:"requests"`
	Prompt   int       `json:"prompt"`    // all prompt tokens
	Cached   int       `json:"cached"`    // of them, read from the cache
	Output   int       `json:"output"`    // output tokens
	First    int       `json:"first"`     // the first request's prompt tokens
	Tools    int       `json:"tools"`     // tools in the first request
	MCPCalls int       `json:"mcp_calls"` // calls that reached a server
	Errors   []string  `json:"errors,omitempty"`
	Seconds  float64   `json:"seconds"`
	Err      string    `json:"err,omitempty"`
}

// card is block 4: the servers' part.
func (a agentRun) card() string {
	var parts []string
	for _, s := range a.Servers {
		parts = append(parts, s.cardBlock(a.Style == "describe", a.Instr[s.ID]))
	}
	return strings.Join(parts, "\n")
}

func (a agentRun) tools() *tool.Registry {
	reg := tool.NewRegistry()
	if a.Style == "describe" {
		reg.Add(describeTools(a.Servers)...)
		return reg
	}
	for _, s := range a.Servers {
		reg.Add(s.nativeTools()...)
	}
	return reg
}

func (h *harness) run(ctx context.Context, a agentRun) (out runOut) {
	start := time.Now()
	defer func() { out.Seconds = time.Since(start).Seconds() }()
	if err := h.play(ctx, a, &out); err != nil {
		out.Err = h.redact(err.Error())
	}
	return out
}

func (h *harness) play(ctx context.Context, a agentRun, out *runOut) error {
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	root, err := os.MkdirTemp(h.temp, "spike024-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	paths := config.Paths{Root: root, Registry: filepath.Join(root, "registry.db")}.WithDataFolder(filepath.Join(root, "data"))
	reg, err := store.OpenRegistry(ctx, paths.Registry)
	if err != nil {
		return err
	}
	defer reg.Close()
	pm := project.NewManager(project.Deps{Paths: paths, Registry: reg, Log: h.log})
	defer pm.CloseAll(context.WithoutCancel(ctx))
	pid, err := pm.Create(ctx, "MCP test")
	if err != nil {
		return err
	}
	p, err := pm.Open(ctx, pid)
	if err != nil {
		return err
	}
	for _, t := range a.PreGrant {
		if err := p.DB.RecordApproval(ctx, store.Approval{ID: id.Approval(id.New()), Kind: "mcp", Target: t,
			Answer: chat.GrantAlways, Source: id.SourceUser}); err != nil {
			p.Release()
			return err
		}
	}
	ch, err := p.Chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: "MCP test", Model: a.Model})
	p.Release()
	if err != nil {
		return err
	}
	set := config.Defaults()
	set.LLM = h.llm
	tr := agent.NewTraces()
	appCtx, stop := context.WithCancel(context.WithoutCancel(ctx))
	cardText := a.card()
	o := agent.New(agent.Deps{
		Context: appCtx, Projects: pm, Models: h.models, Tools: a.tools(), Events: nopPublisher{},
		Settings: func() config.Settings { return set }, Traces: tr, Log: h.log,
		Card: func(context.Context, *project.Project) (string, error) { return cardText, nil },
	})
	defer func() {
		o.Refuse()
		stop()
		wctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = o.Wait(wctx)
	}()
	for _, s := range a.Servers {
		s.st.newTurn()
	}
	if _, err := o.Send(ctx, pid, ch.ID, agent.UserMessage{Text: a.Prompt}); err != nil {
		return err
	}
	werr := h.idle(ctx, o, pid, ch.ID, a, out)
	rerr := h.read(context.WithoutCancel(ctx), pm, pid, ch.ID, tr, out)
	for _, s := range a.Servers {
		s.st.mu.Lock()
		out.MCPCalls += s.st.mcpCalls
		s.st.mu.Unlock()
	}
	return errors.Join(werr, rerr)
}

// idle waits for the turn to end and answers cards with the policy.
func (h *harness) idle(ctx context.Context, o *agent.Orchestrator, pid id.Project, cid id.Chat, a agentRun, out *runOut) error {
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	answered := map[string]bool{}
	for {
		l := o.Live(pid, cid)
		if !l.Running {
			return nil
		}
		if w := l.Waiting; w != nil {
			key := fmt.Sprintf("%s/%d", w.Message, w.Index)
			if !answered[key] {
				ans := agent.Answer{Message: w.Message, Index: w.Index, Grant: chat.GrantDeny, Note: "no opinion"}
				if w.Kind == chat.PartApproval {
					var c card
					var ok bool
					for _, s := range a.Servers {
						if c, ok = s.st.card(w.Text); ok {
							break
						}
					}
					g, note := chat.GrantDeny, "unknown card"
					if ok {
						g, note = a.Policy(c)
						if g == chat.GrantAlways && !c.AlwaysOK {
							g = chat.GrantOnce
						}
					}
					ans.Grant, ans.Note = g, note
					out.Cards = append(out.Cards, cardRec{Tool: c.Tool, Args: clip(string(c.Args), 300), ReadOnly: c.ReadOnly, Grant: g, Note: note})
				}
				if err := o.Answer(pid, cid, ans); err != nil && !errors.Is(err, agent.ErrNotWaiting) {
					return err
				}
				answered[key] = true
			}
		}
		select {
		case <-ctx.Done():
			o.Stop(pid, cid)
			return fmt.Errorf("the run took longer than %s", h.timeout)
		case <-t.C:
		}
	}
}

// read collects the turn's calls, answer, requests and tokens.
func (h *harness) read(ctx context.Context, pm *project.Manager, pid id.Project, cid id.Chat, tr *agent.Traces, out *runOut) error {
	p, err := pm.Open(ctx, pid)
	if err != nil {
		return err
	}
	defer p.Release()
	ms, _, err := p.Chats.Messages(ctx, cid, 1, 0)
	if err != nil {
		return err
	}
	results := map[string]*chat.ToolResult{}
	for _, m := range ms {
		for _, pt := range m.Parts {
			if pt.ToolResult != nil {
				results[pt.ToolResult.CallID] = pt.ToolResult
			}
		}
	}
	var final, anyText string
	for _, m := range ms {
		if m.Role != chat.RoleAssistant {
			continue
		}
		var text strings.Builder
		n := 0
		for _, pt := range m.Parts {
			switch {
			case pt.ToolCall != nil:
				c := call{Tool: pt.ToolCall.Name, Args: string(pt.ToolCall.Args)}
				if r := results[pt.ToolCall.ID]; r != nil {
					c.Error, c.Result = r.IsError, clip(r.Text, 600)
				}
				out.Calls = append(out.Calls, c)
				n++
			case pt.Text != nil:
				text.WriteString(pt.Text.Text)
			}
		}
		if t := strings.TrimSpace(text.String()); t != "" {
			anyText = t
			if n == 0 {
				final = t
			}
		}
	}
	out.Answer = final
	if out.Answer == "" {
		out.Answer = anyText
	}
	for _, t := range tr.List() {
		if t.Chat != cid {
			continue
		}
		for i, q := range t.Requests {
			out.Requests++
			pr := q.Usage.Input + q.Usage.CacheRead + q.Usage.CacheWrite
			out.Prompt += pr
			out.Cached += q.Usage.CacheRead
			out.Output += q.Usage.Output
			if i == 0 && out.First == 0 {
				out.First = pr
				out.Tools = len(q.Req.Tools)
			}
			if q.Err != "" {
				out.Errors = append(out.Errors, h.redact(clip(q.Err, 300)))
			}
		}
	}
	return nil
}

type nopPublisher struct{}

func (nopPublisher) Delta(chat.Delta)   {}
func (nopPublisher) Part(chat.PartDone) {}
func (nopPublisher) Status(chat.Status) {}

// registry builds the app's provider registry for the named models
// (SPIKE-029's). Keys come from the .env file and stay in memory; they
// are returned only so errors can be redacted.
func registry(modelsFile, envFile string, names []string, log *slog.Logger) (*provider.Registry, []string, config.LLMSettings, error) {
	mf, err := scenario.ReadModels(modelsFile)
	if err != nil {
		return nil, nil, config.LLMSettings{}, err
	}
	env := map[string]string{}
	if envFile != "" {
		if env, err = scenario.ReadEnvFile(envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, nil, config.LLMSettings{}, err
		}
	}
	llm := config.Defaults().LLM
	llm.Providers = map[string]config.ProviderSettings{}
	llm.ProviderMaxParallelCalls = map[string]int{}
	llm.Models = map[string]string{"default": names[0], "fast": names[0]}
	kr := &keyring{m: map[string]string{}}
	var keys []string
	for _, n := range names {
		pname, mid, ok := strings.Cut(n, "/")
		i := slices.IndexFunc(mf.Providers, func(p scenario.ProviderSpec) bool { return p.Name == pname })
		if !ok || i < 0 {
			return nil, nil, llm, fmt.Errorf("no provider for %s in %s", n, modelsFile)
		}
		spec := mf.Providers[i]
		j := slices.IndexFunc(spec.Models, func(m config.ModelSettings) bool { return m.ID == mid })
		if j < 0 {
			return nil, nil, llm, fmt.Errorf("no model %s in %s", n, modelsFile)
		}
		ps := llm.Providers[pname]
		ps.Kind, ps.BaseURL = spec.Kind, spec.BaseURL
		ps.Models = append(ps.Models, spec.Models[j])
		llm.Providers[pname] = ps
		llm.ProviderMaxParallelCalls[pname] = 1
		if spec.KeyEnv != "" {
			v := env[spec.KeyEnv]
			if v == "" {
				return nil, nil, llm, fmt.Errorf("%s needs its key in %s", pname, spec.KeyEnv)
			}
			kr.m[provider.KeyName(pname)] = v
			keys = append(keys, v)
		}
	}
	return provider.NewRegistry(provider.Deps{
		Settings: llm, Secrets: secret.New(kr), Gate: limit.NewGate(llm.MaxParallelCalls),
		Backends: backends.All(), Catalog: provider.MustCatalog(), Log: log,
	}), keys, llm, nil
}

type keyring struct {
	mu sync.Mutex
	m  map[string]string
}

func (k *keyring) Get(name string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[name]
	if !ok {
		return "", secret.ErrNotFound
	}
	return v, nil
}

func (k *keyring) Set(name, v string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[name] = v
	return nil
}

func (k *keyring) Delete(name string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, name)
	return nil
}
