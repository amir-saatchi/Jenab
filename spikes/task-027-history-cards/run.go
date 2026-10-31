package main

// The live runs: each follow-up on each model, with today's window
// (NoHistoryCards) and with the app's cards. One lane per model; a lane
// runs rep by rep, both conditions in each, so a model stopped by a quota
// still has whole reps. A run already in -out is not run again, so a
// later day goes on where the last stopped.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/scenario"
)

// The conditions.
const (
	condToday = "today" // today's window: no cards
	condCards = "cards" // the app's history cards
)

var allConds = []string{condToday, condCards}

type harness struct {
	dir     string
	work    string // the runs' projects
	log     *slog.Logger
	timeout time.Duration
	budget  *budget
	models  *scenario.Models
	script  *scenario.Script
	set     *scenario.Set
	turns   map[string][]seedTurn
	plans   map[string]plan
	only    []string
	reps    int
	first   int
	mu      sync.Mutex // runs.jsonl and done
	done    map[string]bool
}

// line is one run in runs.jsonl.
type line struct {
	Model     string            `json:"model"`
	Cond      string            `json:"cond"`
	Scenario  string            `json:"scenario"`
	Kind      string            `json:"kind"` // repeat, paraphrase or control
	Rep       int               `json:"rep"`
	Error     string            `json:"error,omitempty"`
	Skipped   string            `json:"skipped,omitempty"`
	Stopped   bool              `json:"stopped,omitempty"` // a quota, a rate limit or the budget stopped the model: not counted
	Passed    bool              `json:"passed"`            // every check but info passed
	Done      bool              `json:"done"`              // passed, and each tool its checks need ran without an error
	Requests  int               `json:"requests"`          // the chat's requests
	Provider  int               `json:"provider_requests"` // all the provider got, titles and memory too
	Prompt    int               `json:"prompt_tokens"`
	CacheRead int               `json:"cache_read_tokens"`
	Output    int               `json:"output_tokens"`
	Seconds   float64           `json:"seconds"`
	History   int               `json:"history_calls"` // search_history and read_messages in the follow-up
	Planned   []int             `json:"planned,omitempty"`
	Cards     []int             `json:"cards,omitempty"` // the cards the follow-up's first request had
	Asserts   []scenario.Result `json:"asserts,omitempty"`
}

func runKey(model, cond, sc string, rep int) string {
	return fmt.Sprintf("%s|%s|%s|%d", model, cond, sc, rep)
}

// readDone finds the runs already in dir: a stopped run is run again.
func readDone(dir string) (map[string]bool, error) {
	out := map[string]bool{}
	ls, err := readLines(dir)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	for _, l := range ls {
		out[runKey(l.Model, l.Cond, l.Scenario, l.Rep)] = !l.Stopped
	}
	return out, err
}

func readLines(dir string) ([]line, error) {
	f, err := os.Open(filepath.Join(dir, "runs.jsonl"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []line
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var l line
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
		out = append(out, l)
	}
	return out, sc.Err()
}

func (h *harness) write(l line) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.done[runKey(l.Model, l.Cond, l.Scenario, l.Rep)] = !l.Stopped
	f, err := os.OpenFile(filepath.Join(h.dir, "runs.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("runs.jsonl: %v", err)
		return
	}
	defer f.Close()
	raw, _ := json.Marshal(l)
	f.Write(append(raw, '\n'))
}

// skip is why a run is not sent: a follow-up with no card has the same
// requests with cards as without, so its cards run is today's.
func (h *harness) skip(cond, sc string) bool {
	p := h.plans[sc]
	return cond == condCards && len(p.Cards) == 0
}

// lane runs one model's reps.
func (h *harness) lane(ctx context.Context, model string, conds []string) {
	for rep := h.first; rep < h.first+h.reps; rep++ {
		for _, cond := range conds {
			if ctx.Err() != nil {
				return
			}
			if err := h.runRep(ctx, model, cond, rep); err != nil && ctx.Err() == nil {
				log.Printf("%s %s r%d: %v", model, cond, rep, err)
			}
		}
	}
}

// runRep runs the follow-ups not yet run of one rep, model and condition.
func (h *harness) runRep(ctx context.Context, model, cond string, rep int) error {
	var ids []string
	h.mu.Lock()
	for _, sc := range h.set.Scenarios {
		if (len(h.only) == 0 || slices.Contains(h.only, sc.ID)) && !h.skip(cond, sc.ID) && !h.done[runKey(model, cond, sc.ID, rep)] {
			ids = append(ids, sc.ID)
		}
	}
	h.mu.Unlock()
	if len(ids) == 0 {
		return nil
	}
	scens := map[string]*scenario.Scenario{}
	for _, sc := range h.set.Scenarios {
		scens[sc.ID] = sc
	}
	kind := providerKind(h.models, model)
	// The lane runs one at a time: the run's traces and chat, for the
	// cards its follow-up's first request had.
	var traces *agent.Traces
	var key agent.TraceKey
	limits := 0 // the kind's rate limits when the run started
	ms := *h.models
	ms.Run = []string{model}
	opt := scenario.Options{
		Models: &ms, Backends: counted(h.budget), Reps: 1, Timeout: h.timeout, Parallel: 1, Only: ids, Log: h.log,
		Script: h.script, Temp: h.work,
		Deps: func(d *agent.Deps) {
			d.NoHistoryCards = cond == condToday
			traces = d.Traces
		},
		Setup: func(ctx context.Context, sc *scenario.Scenario, p *project.Project, c id.Chat) error {
			limits = h.budget.rateLimits(kind)
			key = agent.TraceKey{Project: p.ID, Chat: c, Turn: followUp}
			return seedProject(ctx, p.Chats, c, chatOf(sc.ID), h.turns)
		},
		Progress: func(r scenario.Run) {
			r.Rep = rep
			tr := fromTurn(r.Transcript, followUp)
			p := h.plans[r.Scenario]
			l := line{
				Model: r.Model, Cond: cond, Scenario: r.Scenario, Kind: p.Kind, Rep: rep,
				Error: r.Error, Skipped: r.Skipped, Passed: passed(r),
				Requests: r.Requests, Provider: r.ProviderRequests, Prompt: r.Prompt, CacheRead: r.CacheRead,
				Output: r.Output, Seconds: r.Seconds, History: historyCalls(tr), Planned: p.Cards, Asserts: r.Asserts,
			}
			l.Done = l.Passed && worked(scens[r.Scenario], r, tr)
			if t, ok := traces.Turn(key); ok {
				l.Cards = sentCards(t)
			}
			// A run stopped by a quota, or broken while rate limited, is
			// not counted, and a resume runs it again.
			if !l.Passed && (h.budget.why(kind) != "" || (l.Error != "" && h.budget.rateLimits(kind) > limits)) {
				l.Stopped = true
			}
			h.write(l)
			log.Printf("%-34s %-5s %-10s r%d %s", model, cond, r.Scenario, rep, status(l))
		},
	}
	if h.script != nil {
		opt.Backends = nil
		opt.Scale = 0.01
	}
	rp, err := scenario.RunSet(ctx, h.set, opt)
	if rp != nil {
		for i := range rp.Runs {
			rp.Runs[i].Rep = rep
		}
		if _, werr := rp.Write(filepath.Join(h.dir, cond, slug(model), "r"+strconv.Itoa(rep)), nil); werr != nil {
			err = errors.Join(err, werr)
		}
	}
	return err
}

var reCard = regexp.MustCompile(`(?m)^\[Turn (\d+) of this chat, before the recent turns\.`)

// sentCards are the cards in the turn's first request.
func sentCards(t agent.TurnTrace) []int {
	if len(t.Requests) == 0 {
		return nil
	}
	var out []int
	for _, m := range t.Requests[0].Req.Messages {
		if m.Role != chat.RoleUser {
			continue
		}
		for _, p := range m.Parts {
			if p.Text == nil {
				continue
			}
			for _, g := range reCard.FindAllStringSubmatch(p.Text.Text, -1) {
				n, _ := strconv.Atoi(g[1])
				out = append(out, n)
			}
		}
	}
	return out
}

// passed is a run with every check but info passed.
func passed(r scenario.Run) bool {
	if r.Skipped != "" || r.Error != "" {
		return false
	}
	for _, a := range r.Asserts {
		if a.Status == scenario.Fail && a.Test != "info" {
			return false
		}
	}
	return true
}

// worked is each passed tool_called check having a call of its tool that
// ran without an error: the runner's check also counts a refused call.
// Results come in the order of the scenario's checks.
func worked(sc *scenario.Scenario, r scenario.Run, tr string) bool {
	if sc == nil || len(sc.Asserts) != len(r.Asserts) {
		return true
	}
	ok := map[string]bool{}
	for _, c := range parseCalls(tr) {
		if !c.failed {
			ok[c.tool] = true
		}
	}
	for i, a := range sc.Asserts {
		if a.Type != "tool_called" || a.Test == "info" || r.Asserts[i].Status != scenario.Pass {
			continue
		}
		tools := append(slices.Clone(a.Tools), a.Tool)
		if !slices.ContainsFunc(tools, func(t string) bool { return t != "" && ok[t] }) {
			return false
		}
	}
	return true
}

type callLine struct {
	tool   string
	failed bool
}

// parseCalls reads a transcript's calls: "> call X {args}", and "> result
// X (error):" for a failed one, which belongs to the oldest open call of X.
func parseCalls(tr string) []callLine {
	var out []callLine
	open := map[string][]int{}
	for l := range strings.SplitSeq(tr, "\n") {
		switch {
		case strings.HasPrefix(l, "> call "):
			name, _, _ := strings.Cut(strings.TrimPrefix(l, "> call "), " ")
			open[name] = append(open[name], len(out))
			out = append(out, callLine{tool: name})
		case strings.HasPrefix(l, "> result "):
			name, mark, _ := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(l, "> result "), ":"), " ")
			if q := open[name]; len(q) > 0 {
				open[name] = q[1:]
				out[q[0]].failed = mark == "(error)"
			}
		}
	}
	return out
}

func historyCalls(tr string) int {
	n := 0
	for _, c := range parseCalls(tr) {
		if c.tool == "search_history" || c.tool == "read_messages" {
			n++
		}
	}
	return n
}

// fromTurn is the transcript from turn n's heading on.
func fromTurn(tr string, n int) string {
	if i := strings.Index(tr, fmt.Sprintf("\n## Turn %d\n", n)); i >= 0 {
		return tr[i:]
	}
	return ""
}

func status(l line) string {
	cards := ""
	if l.Cond == condCards {
		cards = fmt.Sprintf(", cards %v", l.Cards)
	}
	switch {
	case l.Skipped != "":
		return "skip"
	case l.Stopped:
		return "stopped: " + clip(l.Error, 160)
	case l.Error != "":
		return "broke: " + clip(l.Error, 160)
	case l.Passed && !l.Done:
		return fmt.Sprintf("checks pass, but a needed tool never ran without an error (%d req%s)", l.Requests, cards)
	case l.Done:
		return fmt.Sprintf("done (%d req, %d history calls%s)", l.Requests, l.History, cards)
	}
	var failed []string
	for _, a := range l.Asserts {
		if a.Status == scenario.Fail && a.Test != "info" {
			failed = append(failed, a.Type)
		}
	}
	return fmt.Sprintf("fail: %s (%d req, %d history calls%s)", strings.Join(failed, ", "), l.Requests, l.History, cards)
}

// pickModels reads the models file and keys, or a script.
func pickModels(names []string, modelsFile, envFile string) (*scenario.Models, *scenario.Script, error) {
	if len(names) == 1 {
		if file, ok := strings.CutPrefix(names[0], "scripted:"); ok {
			s, err := scenario.ReadScript(file)
			if err != nil {
				return nil, nil, err
			}
			return scenario.ScriptedModels(s), s, nil
		}
	}
	mf, err := scenario.ReadModels(modelsFile)
	if err != nil {
		return nil, nil, err
	}
	env := scenario.Env{}
	if env.File, err = scenario.ReadEnvFile(envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	m, err := mf.Select(names, env)
	return m, nil, err
}

// providerKind is the kind of the model's provider.
func providerKind(ms *scenario.Models, model string) provider.Kind {
	name, _, _ := strings.Cut(model, "/")
	for _, p := range ms.Providers {
		if p.Name == name {
			return provider.Kind(p.Kind)
		}
	}
	return ""
}

func slug(model string) string { return strings.NewReplacer("/", "_", ":", "_").Replace(model) }

func split(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}
