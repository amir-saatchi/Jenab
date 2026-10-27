package scenario

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/appinfo"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/config"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/provider/fake"
	"github.com/amir-saatchi/jenab/internal/secret"
	"github.com/amir-saatchi/jenab/internal/skill"
	"github.com/amir-saatchi/jenab/internal/store"
)

// Options are how a set runs.
type Options struct {
	Models   *Models // the providers and the models to run
	Backends map[provider.Kind]provider.Factory
	Reps     int     // runs of each scenario on each model; 0 means 1
	Scale    float64 // multiplies tool delays and message times; 0 means 1
	Timeout  time.Duration
	Parallel int      // runs at a time; 0 means 1
	Only     []string // scenario IDs to run; empty runs all
	Temp     string   // where the runs' projects go; "" is the system's
	Keep     bool     // keep the runs' projects
	Log      *slog.Logger
	Progress func(Run) // called after each run
	// Settings changes the app settings of every run, after the set's
	// and the scenario's.
	Settings func(*config.Settings)
	// Script plays scripted answers instead of a model, for runs without
	// keys; Models are then ScriptedModels.
	Script *Script
	// Grounding sets SPIKE-032's number check over the sets' and the
	// scenarios'.
	Grounding *GroundingSpec
	// Deps changes each run's orchestrator deps before it starts, such as
	// SPIKE-028's tool registry and index. Spike-only: nil leaves them as
	// the runner builds them.
	Deps func(*agent.Deps)
	// Call maps a tool call to the call the checks read, such as
	// SPIKE-028's call_tool to the tool it runs; the transcript shows
	// both. Spike-only: nil reads every call as it is.
	Call func(name, args string) (string, string)
	// Setup changes the chat under test before the first message, such as
	// SPIKE-028's earlier turns in a long chat. Spike-only: nil leaves the
	// chat as the runner makes it.
	Setup func(ctx context.Context, sc *Scenario, p *project.Project, c id.Chat) error
}

// DefaultTimeout bounds one run.
const DefaultTimeout = 10 * time.Minute

// Run runs every scenario of the set on every model and returns the
// report. Each run gets a new project, so runs don't share anything.
func RunSet(ctx context.Context, s *Set, opt Options) (*Report, error) {
	if opt.Models == nil || len(opt.Models.Run) == 0 {
		return nil, errors.New("scenario: no models to run")
	}
	if opt.Reps <= 0 {
		opt.Reps = 1
	}
	if opt.Scale <= 0 {
		opt.Scale = 1
	}
	if opt.Timeout <= 0 {
		opt.Timeout = DefaultTimeout
	}
	if opt.Parallel <= 0 {
		opt.Parallel = 1
	}
	if opt.Log == nil {
		opt.Log = slog.New(slog.DiscardHandler)
	}
	for _, n := range opt.Only {
		if !slices.ContainsFunc(s.Scenarios, func(sc *Scenario) bool { return sc.ID == n }) {
			return nil, fmt.Errorf("scenario: no scenario %s in the set", n)
		}
	}
	builtin, err := skill.Builtin()
	if err != nil {
		return nil, err
	}
	// A set's skill replaces the built-in one of its name, as its fake
	// tools replace the real ones.
	own := builtin.For(chat.KindMother)
	own = slices.DeleteFunc(own, func(b skill.Skill) bool {
		return slices.ContainsFunc(s.Skills, func(sk skill.Skill) bool { return sk.Name == b.Name })
	})
	skills, err := skill.New(append(own, s.Skills...)...)
	if err != nil {
		return nil, fmt.Errorf("scenario: the set's skills: %w", err)
	}
	type job struct {
		model string
		sc    *Scenario
		rep   int
		i     int
	}
	// Models take turns, so parallel runs spread over the providers.
	var jobs []job
	for _, sc := range s.Scenarios {
		if len(opt.Only) > 0 && !slices.Contains(opt.Only, sc.ID) {
			continue
		}
		for rep := 1; rep <= opt.Reps; rep++ {
			for _, m := range opt.Models.Run {
				jobs = append(jobs, job{m, sc, rep, len(jobs)})
			}
		}
	}
	rep := &Report{Set: s.Title, Dir: s.Dir, Started: time.Now().UTC(), Scale: opt.Scale, Reps: opt.Reps, Models: opt.Models.Run}
	runs := make([]Run, len(jobs))
	ch := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for range opt.Parallel {
		wg.Go(func() {
			for j := range ch {
				r := (&run{opt: opt, sc: j.sc, skills: skills, model: j.model}).do(ctx, j.rep)
				runs[j.i] = r
				if opt.Progress != nil {
					mu.Lock()
					opt.Progress(r)
					mu.Unlock()
				}
			}
		})
	}
	for _, j := range jobs {
		if ctx.Err() != nil {
			break
		}
		ch <- j
	}
	close(ch)
	wg.Wait()
	for _, r := range runs {
		if r.Model != "" {
			rep.Runs = append(rep.Runs, r)
		}
	}
	rep.Took = time.Since(rep.Started).Seconds()
	rep.summarize()
	return rep, ctx.Err()
}

// run is one scenario on one model.
type run struct {
	opt    Options
	sc     *Scenario
	skills *skill.Set
	model  string // "provider/id"
	db     *sql.DB
	fp     *fake.Provider // the scripted model, with Options.Script

	// The app: opened at the start and again after a restart.
	paths  config.Paths
	traces *agent.Traces // kept across restarts, so the record has every request
	reg    *store.Registry
	pm     *project.Manager
	o      *agent.Orchestrator
	stop   context.CancelFunc

	pid   id.Project
	cid   id.Chat   // the chat under test now
	chats []id.Chat // the chats under test, in order: more with a new_chat message
}

func (r *run) do(ctx context.Context, rep int) Run {
	out := Run{Model: r.model, Scenario: r.sc.ID, Rep: rep}
	if why := r.sc.skip(); why != "" {
		out.Skipped = why
		return out
	}
	start := time.Now()
	rec, err := r.play(ctx)
	out.Seconds = time.Since(start).Seconds()
	if err != nil {
		out.Error = r.opt.Models.redact(err.Error())
	}
	if rec != nil {
		out.Turns, out.Requests, out.Prompt, out.PeakPrompt, out.Output = rec.turns(), len(rec.requests), rec.prompt(), rec.peak(), rec.output()
		out.SubRequests = rec.subRequests
		u := rec.usage()
		out.Input, out.CacheRead, out.CacheWrite = u.Input, u.CacheRead, u.CacheWrite
		if e := rec.failure(); e != "" && out.Error == "" {
			out.Error = r.opt.Models.redact(e)
		}
		if g := r.grounding(); g != nil && g.Mode != "" {
			rec.grounding = g
			out.GroundMode, out.GroundK, out.GroundVariant = g.Mode, g.agent().Threshold(), g.Variant()
			out.Grounding, out.GroundExtra = rec.groundResults(), rec.groundExtra()
		}
		out.Asserts = check(rec, r.sc)
		out.Transcript = r.opt.Models.redact(rec.transcript())
	}
	return out
}

// play runs the scenario and records it.
func (r *run) play(ctx context.Context) (*record, error) {
	ctx, cancel := context.WithTimeout(ctx, r.opt.Timeout)
	defer cancel()
	// late reports a run out of time as such, wherever it hit: setting up,
	// waiting for the chat, for a message's time, or in a restart.
	late := func(err error) error {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("the run took longer than %s", r.opt.Timeout)
		}
		return err
	}
	root, err := os.MkdirTemp(r.opt.Temp, "jenab-scenario-")
	if err != nil {
		return nil, err
	}
	if !r.opt.Keep {
		defer os.RemoveAll(root)
	}
	if r.sc.set.Fixture != "" {
		if r.db, err = openFixture(ctx, r.sc.set.Fixture); err != nil {
			return nil, late(err)
		}
		defer r.db.Close()
	}
	r.paths = config.Paths{Root: root, Registry: filepath.Join(root, "registry.db")}.WithDataFolder(filepath.Join(root, "data"))
	r.traces = agent.NewTraces()
	if r.opt.Script != nil {
		r.fp = fake.New(r.opt.Script.replies(r.sc.ID)...) // one per run, so a restart goes on with it
	}
	if err := r.open(ctx); err != nil {
		return nil, late(err)
	}
	defer func() { r.close(context.WithoutCancel(ctx)) }()
	if r.pid, err = r.pm.Create(ctx, r.sc.set.Title); err != nil {
		return nil, late(err)
	}
	if err := r.setup(ctx); err != nil {
		return nil, late(err)
	}
	if err := r.spikeSetup(ctx); err != nil {
		return nil, late(err)
	}
	sent, err := r.send(ctx)
	if err = late(err); ctx.Err() != nil && r.o != nil {
		r.o.Stop(r.pid, r.cid)
	}
	rec, rerr := r.record(context.WithoutCancel(ctx), r.traces, sent)
	return rec, errors.Join(err, rerr)
}

// open starts the app: the registry, the projects and the orchestrator.
func (r *run) open(ctx context.Context) error {
	reg, err := store.OpenRegistry(ctx, r.paths.Registry)
	if err != nil {
		return err
	}
	r.reg = reg
	r.pm = project.NewManager(project.Deps{Paths: r.paths, Registry: reg, Log: r.opt.Log})
	set := r.settings()
	backends := r.opt.Backends
	if r.fp != nil {
		backends = map[provider.Kind]provider.Factory{scriptedKind: r.fp.Factory()}
	}
	models := provider.NewRegistry(provider.Deps{
		Settings: set.LLM, Secrets: secret.New(r.opt.Models.keyring()), Gate: limit.NewGate(set.LLM.MaxParallelCalls),
		Backends: backends, Catalog: provider.MustCatalog(), Log: r.opt.Log,
	})
	appCtx, stop := context.WithCancel(context.WithoutCancel(ctx))
	r.stop = stop
	card := func(context.Context, *project.Project) (string, error) { return r.sc.set.Card, nil }
	if r.sc.set.ProjectCard {
		card = func(ctx context.Context, p *project.Project) (string, error) { return p.DB.Card(ctx, r.now()) }
	}
	d := agent.Deps{
		Context: appCtx, Projects: r.pm, Models: models, Tools: r.tools(), Settings: func() config.Settings { return set },
		Skills: r.skills, Traces: r.traces, Log: r.opt.Log,
		Card:      card,
		Now:       r.now,
		Grounding: r.grounding().agent(),
	}
	if r.opt.Deps != nil {
		r.opt.Deps(&d)
	}
	r.o = agent.New(d)
	return nil
}

// grounding is the number check of the run (SPIKE-032): the runner's,
// the scenario's or the set's.
func (r *run) grounding() *GroundingSpec {
	if r.opt.Grounding != nil {
		return r.opt.Grounding
	}
	return r.sc.Grounding
}

// now is the clock: the set's day at the time of day now, so block 1's
// date matches the fixture's card.
func (r *run) now() time.Time {
	n := time.Now()
	day := r.sc.set.Today
	if day.IsZero() {
		return n
	}
	return time.Date(day.Year(), day.Month(), day.Day(), n.Hour(), n.Minute(), n.Second(), n.Nanosecond(), time.Local)
}

// close shuts the app down as it does on quit: no new turns, running ones
// stopped, then the projects and the registry closed. A closed app is
// closed again without effect.
func (r *run) close(ctx context.Context) error {
	if r.o == nil {
		return nil
	}
	defer func() { r.o = nil }()
	r.o.Refuse()
	r.stop()
	wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := r.o.Wait(wctx); err != nil {
		r.opt.Log.Warn("scenario: turns still running at the end", "scenario", r.sc.ID, "err", err)
	}
	return errors.Join(r.pm.CloseAll(ctx), r.reg.Close())
}

// restart closes the app and opens it again from disk.
func (r *run) restart(ctx context.Context) error {
	if err := r.close(ctx); err != nil {
		return fmt.Errorf("restart: %w", err)
	}
	if err := r.open(ctx); err != nil {
		return fmt.Errorf("restart: %w", err)
	}
	return nil
}

// setup fills the project's database and adds the set's other chats and
// the chat under test.
func (r *run) setup(ctx context.Context) error {
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return err
	}
	defer p.Release()
	if r.sc.set.Data != "" {
		_, err := store.Do(ctx, p.DB.DB, limit.Interactive, func(tx *sql.Tx) (sql.Result, error) {
			return tx.ExecContext(ctx, r.sc.set.Data)
		})
		if err != nil {
			return fmt.Errorf("data: %w", err)
		}
	}
	for _, c := range r.sc.set.Chats {
		ch, err := p.Chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: c.Title, Role: c.Role})
		if err != nil {
			return fmt.Errorf("chat %q: %w", c.Title, err)
		}
		if c.Notes != "" {
			if _, err := p.Chats.SaveNotes(ctx, chat.SessionNote{Chat: ch.ID, Content: c.Notes}); err != nil {
				return fmt.Errorf("chat %q: notes: %w", c.Title, err)
			}
		}
	}
	if r.sc.Context == "mother" {
		cs, err := p.Chats.Chats(ctx)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(cs, func(c chat.Chat) bool { return c.Kind == chat.KindMother })
		if i < 0 {
			return errors.New("the project has no " + appinfo.Mother + " chat")
		}
		r.cid = cs[i].ID
		r.chats = []id.Chat{r.cid}
		if len(r.sc.Skills) > 0 {
			_, err = p.Chats.SetSkills(ctx, r.cid, r.sc.Skills)
		}
		return err
	}
	title := r.sc.Title
	if title == "" {
		title = r.sc.ID
	}
	if len([]rune(title)) > 60 {
		title = string([]rune(title)[:60])
	}
	return r.newChat(ctx, p, title)
}

// spikeSetup runs Options.Setup on the chat under test (SPIKE-028,
// spike-only).
func (r *run) spikeSetup(ctx context.Context) error {
	if r.opt.Setup == nil {
		return nil
	}
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return err
	}
	defer p.Release()
	return r.opt.Setup(ctx, r.sc, p, r.cid)
}

// newChat makes a chat under test with the scenario's role and skills, and
// sends the next messages there.
func (r *run) newChat(ctx context.Context, p *project.Project, title string) error {
	ch, err := p.Chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: title, Role: r.sc.Role, Skills: r.sc.Skills})
	if err != nil {
		return err
	}
	r.cid = ch.ID
	r.chats = append(r.chats, ch.ID)
	return nil
}

// settings are the app's defaults with the set's and the scenario's
// settings, and the model under test as default and fast, so a title
// request, if any, goes to it too.
func (r *run) settings() config.Settings {
	s := r.sc.settings
	s.LLM.Providers = r.opt.Models.settings()
	s.LLM.Models = map[string]string{"default": r.model, "fast": r.model}
	if r.opt.Settings != nil {
		r.opt.Settings(&s)
	}
	return s
}

// sent is a scripted message the run sent.
type sent struct {
	index int
	id    id.Message
	at    time.Duration // since the start
}

// send plays the messages: one without a time waits until the chat is
// idle; one with a time is sent then, also during a turn. A restart comes
// once the chat is idle. At the end it waits until the chat is idle.
func (r *run) send(ctx context.Context) ([]sent, error) {
	start := time.Now()
	var out []sent
	for i, m := range r.sc.Messages {
		if m.At == nil {
			if err := r.idle(ctx); err != nil {
				return out, err
			}
		} else if err := r.sleep(ctx, time.Duration(*m.At)-time.Duration(float64(time.Since(start))/r.opt.Scale)); err != nil {
			return out, err
		}
		if m.Restart {
			if err := r.restart(ctx); err != nil {
				return out, err
			}
		}
		if m.NewChat {
			if err := r.nextChat(ctx); err != nil {
				return out, fmt.Errorf("message %d: %w", i+1, err)
			}
		}
		mid, err := r.o.Send(ctx, r.pid, r.cid, agent.UserMessage{Text: m.Text})
		if err != nil {
			return out, fmt.Errorf("message %d: %w", i+1, err)
		}
		out = append(out, sent{index: i, id: mid, at: time.Since(start)})
	}
	return out, r.idle(ctx)
}

// nextChat makes the next chat under test, for a new_chat message.
func (r *run) nextChat(ctx context.Context) error {
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return err
	}
	defer p.Release()
	title := r.sc.Title
	if title == "" {
		title = r.sc.ID
	}
	return r.newChat(ctx, p, fmt.Sprintf("%.50s (%d)", title, len(r.chats)+1))
}

// pollEvery is how often the runner looks at the chat.
const pollEvery = 25 * time.Millisecond

// idle waits until the chat's turn has ended. A card or form the turn
// waits for is answered as a user without an opinion would: an approval
// once, a question with its recommended or first option.
func (r *run) idle(ctx context.Context) error {
	t := time.NewTicker(pollEvery)
	defer t.Stop()
	for {
		l := r.o.Live(r.pid, r.cid)
		if !l.Running {
			return nil
		}
		if w := l.Waiting; w != nil {
			if err := r.answer(ctx, *w); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (r *run) answer(ctx context.Context, w chat.Waiting) error {
	a := agent.Answer{Message: w.Message, Index: w.Index}
	if w.Kind == chat.PartApproval {
		a.Grant = chat.GrantOnce
	} else {
		q, err := r.question(ctx, w)
		if err != nil {
			return err
		}
		a.Answers = map[string][]string{}
		for _, it := range q.Questions {
			pick := it.Options[0].Label
			for _, o := range it.Options {
				if o.Recommended {
					pick = o.Label
					break
				}
			}
			a.Answers[it.Header] = []string{pick}
		}
	}
	err := r.o.Answer(r.pid, r.cid, a)
	if errors.Is(err, agent.ErrNotWaiting) {
		return nil // answered or stopped meanwhile
	}
	return err
}

// question reads the form a turn waits for.
func (r *run) question(ctx context.Context, w chat.Waiting) (*chat.Question, error) {
	p, err := r.pm.Open(ctx, r.pid)
	if err != nil {
		return nil, err
	}
	defer p.Release()
	ms, _, err := p.Chats.Messages(ctx, r.cid, 1, 0)
	if err != nil {
		return nil, err
	}
	for _, m := range ms {
		if m.ID == w.Message && w.Index < len(m.Parts) {
			if q := m.Parts[w.Index].Question; q != nil && len(q.Questions) > 0 {
				return q, nil
			}
		}
	}
	return nil, fmt.Errorf("the form the turn waits for is not in %s", w.Message)
}
