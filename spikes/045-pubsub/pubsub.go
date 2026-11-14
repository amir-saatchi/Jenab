package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// The definition under test: a subscription is a standing rule held by
// the app: when an event of type E, matching filter F, happens, it is
// delivered to chat C as a notice, written into C's history when C's next
// turn starts. A notice starts no turn. An event C caused itself isn't
// delivered to C. Everything here is spike-only: the app has none of it,
// and reaches it only through agent.Deps' Notices, Notes and Section
// hooks, which it leaves nil.

// The v1 app events.
const (
	evSchema = "schema_changed" // filtered by table
	evRun    = "pipeline_run"   // filtered by pipeline and outcome
)

// noticeEvent is a delivered event's notice kind.
const noticeEvent chat.NoticeKind = "event"

// noticeChars is a notice's limit.
const noticeChars = 1000

// channelDef is one of the project's channels, made in the setup.
type channelDef struct{ Name, Desc string }

var channels = []channelDef{
	{"alerts", "big price moves and other market news the other chats should act on"},
	{"data", "changes to the project's tables and data"},
	{"reviews", "findings about pipelines and their runs"},
}

func channelNames() []string {
	out := make([]string, len(channels))
	for i, c := range channels {
		out[i] = c.Name
	}
	return out
}

// subRec is a subscription.
type subRec struct {
	ID    string `json:"id"`
	Agent string `json:"agent"`
	sub
	By      string `json:"by"`  // "user" (the key, as if made in the panel) or "agent"
	Msg     int    `json:"msg"` // the message whose turn made it; 0 for the setup
	Ended   bool   `json:"ended,omitempty"`
	EndedAt int    `json:"ended_at,omitempty"` // the message whose turn ended it
}

// event is one event: an app event or a report published on a channel.
type event struct {
	N        int    `json:"n"`
	Kind     string `json:"kind"` // evSchema, evRun or a channel
	By       string `json:"by"`   // an agent's key, or "app"
	Table    string `json:"table,omitempty"`
	Pipeline string `json:"pipeline,omitempty"`
	Outcome  string `json:"outcome,omitempty"`
	Text     string `json:"text"` // the notice
	Msg      int    `json:"msg"`  // the message it came in, or before
}

// delivery is one notice given to a chat.
type delivery struct {
	Event int    `json:"event"`
	Agent string `json:"agent"`
	Msg   int    `json:"msg"` // the message whose turn got it
}

// pubRec is one publish call: refused (an unknown channel) or a report.
type pubRec struct {
	Msg     int    `json:"msg"`
	Agent   string `json:"agent"`
	Channel string `json:"channel"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Report  string `json:"report,omitempty"`
	Refused bool   `json:"refused,omitempty"`
}

// bus is a run's subscription store, events and reports.
type bus struct {
	mu      sync.Mutex
	cond    int
	keys    map[id.Chat]string // chat → agent key
	chats   map[string]id.Chat
	subs    []subRec
	events  []event
	pending map[string][]int
	deliv   []delivery
	pubs    []pubRec
	own     int // deliveries skipped because the chat caused the event
	dups    int // subscribe calls for a subscription the chat already had
	msg     int // the message playing now
	nsub    int
}

func newBus(cond int) *bus {
	return &bus{cond: cond, keys: map[id.Chat]string{}, chats: map[string]id.Chat{}, pending: map[string][]int{}}
}

func (b *bus) addChat(key string, ch id.Chat) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.keys[ch] = key
	b.chats[key] = ch
}

func (b *bus) setMsg(n int) {
	b.mu.Lock()
	b.msg = n
	b.mu.Unlock()
}

// subscribe adds a subscription, or returns the one the chat already has.
// The caller holds b.mu.
func (b *bus) subscribe(agent string, s sub, by string) (subRec, bool) {
	for _, x := range b.subs {
		if x.Agent == agent && !x.Ended && x.sub == s {
			return x, false
		}
	}
	b.nsub++
	x := subRec{ID: fmt.Sprintf("s_%d", b.nsub), Agent: agent, sub: s, By: by, Msg: b.msg}
	b.subs = append(b.subs, x)
	return x, true
}

func matches(s sub, e event) bool {
	if s.Event != e.Kind {
		return false
	}
	eq := func(f, v string) bool { return f == "" || f == "*" || strings.EqualFold(f, v) }
	return eq(s.Table, e.Table) && eq(s.Pipeline, e.Pipeline) && eq(s.Outcome, e.Outcome)
}

// emit records an event and queues its notice for each chat with a
// matching subscription, once per chat, except the chat that caused it.
func (b *bus) emit(e event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e.N, e.Msg = len(b.events), b.msg
	e.Text = clipRunes(e.Text, noticeChars)
	b.events = append(b.events, e)
	got := map[string]bool{}
	for _, s := range b.subs {
		if s.Ended || got[s.Agent] || !matches(s.sub, e) {
			continue
		}
		got[s.Agent] = true
		if s.Agent == e.By {
			b.own++
			continue
		}
		b.pending[s.Agent] = append(b.pending[s.Agent], e.N)
	}
}

// notices is the Notices hook: the chat's queued notices, each given once.
func (b *bus) notices(_ context.Context, _ *project.Project, ch chat.Chat) ([]chat.Notice, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := b.keys[ch.ID]
	var out []chat.Notice
	for _, n := range b.pending[key] {
		out = append(out, chat.Notice{Kind: noticeEvent, Text: b.events[n].Text})
		b.deliv = append(b.deliv, delivery{Event: n, Agent: key, Msg: b.msg})
	}
	delete(b.pending, key)
	return out, nil
}

// subsText lists a chat's subscriptions. The caller holds b.mu.
func (b *bus) subsText(agent string) string {
	var lines []string
	for _, s := range b.subs {
		if s.Agent == agent && !s.Ended {
			lines = append(lines, fmt.Sprintf("- %s: %s", s.ID, s.sub))
		}
	}
	if len(lines) == 0 {
		return "- none"
	}
	return strings.Join(lines, "\n")
}

// block5 is the Notes hook's text after the session notes: the chat's
// subscriptions in conditions 2–5, and in condition 5 the channels with
// their last 20 titles. It is built at cuts, like the rest of block 5.
func (b *bus) block5(agent string) string {
	if b.cond == condNone {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var s strings.Builder
	s.WriteString("This chat's subscriptions (as of the last cut). Each matching event reaches this chat as a notice at the start of its next turn:\n" + b.subsText(agent))
	if b.cond == condSection {
		s.WriteString("\n\nThe project's channels (as of the last cut), each with its last 20 titles. Publish on one with publish:")
		for _, c := range channels {
			var titles []string
			for _, p := range b.pubs {
				if p.Channel == c.Name && !p.Refused {
					titles = append(titles, fmt.Sprintf("%q (%s, %s)", p.Title, agentByKey(p.Agent).Title, p.Report))
				}
			}
			if len(titles) > 20 {
				titles = titles[len(titles)-20:]
			}
			last := "none yet"
			if len(titles) > 0 {
				last = strings.Join(titles, "; ")
			}
			fmt.Fprintf(&s, "\n- %s: %s. Last titles: %s", c.Name, c.Desc, last)
		}
	}
	return s.String()
}

// publishRule is condition 5's prompt section, in block 1 of each chat
// that has publish.
const publishRule = "Before you end a turn whose work changed something another chat depends on, publish it on its channel. Don't publish routine work."

func jsonSchema(s string) json.RawMessage { return json.RawMessage(s) }

// tools are the spike's tools: subscribe and unsubscribe (condition 3),
// publish and read_report (conditions 4 and 5). The same descriptions for
// every model (SPEC 1).
func (b *bus) tools() []tool.Tool {
	type subArgs struct {
		Event  string `json:"event"`
		Filter struct {
			Table    string `json:"table"`
			Pipeline string `json:"pipeline"`
			Outcome  string `json:"outcome"`
		} `json:"filter"`
	}
	subscribe := tool.Func(tool.Spec{Name: "subscribe",
		Description: "Subscribe this chat to an app event. Each event that matches reaches this chat as a short notice at the start of its next turn; it doesn't start a turn. " +
			"Events: schema_changed (a table or column was added, changed or removed; filter by table) and pipeline_run (a pipeline run ended; filter by pipeline, and by outcome: success or failed). " +
			"Leave a filter field out to match any. Events this chat causes itself aren't delivered to it. A subscription lasts until unsubscribe. " +
			"Every notice costs tokens, so subscribe only to what this chat's job needs.",
		Schema: jsonSchema(`{"type":"object","required":["event"],"properties":{
			"event":{"type":"string","enum":["schema_changed","pipeline_run"]},
			"filter":{"type":"object","properties":{
				"table":{"type":"string","description":"schema_changed: only this table"},
				"pipeline":{"type":"string","description":"pipeline_run: only this pipeline's runs"},
				"outcome":{"type":"string","enum":["success","failed"],"description":"pipeline_run: only runs with this outcome"}}}}}`)},
		func(_ context.Context, env *tool.Env, a subArgs) (tool.Result, error) {
			if a.Event != evSchema && a.Event != evRun {
				return tool.Result{}, &tool.Error{Msg: "event is schema_changed or pipeline_run", Err: tool.ErrArgs}
			}
			s := sub{Event: a.Event, Table: strings.TrimSpace(a.Filter.Table), Pipeline: strings.TrimSpace(a.Filter.Pipeline), Outcome: strings.TrimSpace(a.Filter.Outcome)}
			if a.Event == evSchema && (s.Pipeline != "" || s.Outcome != "") {
				return tool.Result{}, &tool.Error{Msg: "schema_changed is filtered by table only", Err: tool.ErrArgs}
			}
			if a.Event == evRun && s.Table != "" {
				return tool.Result{}, &tool.Error{Msg: "pipeline_run is filtered by pipeline and outcome only", Err: tool.ErrArgs}
			}
			if s.Outcome != "" && s.Outcome != "success" && s.Outcome != "failed" {
				return tool.Result{}, &tool.Error{Msg: "outcome is success or failed", Err: tool.ErrArgs}
			}
			if s.Table == "*" {
				s.Table = ""
			}
			if s.Pipeline == "*" {
				s.Pipeline = ""
			}
			b.mu.Lock()
			defer b.mu.Unlock()
			agent := b.keys[env.Chat]
			x, made := b.subscribe(agent, s, "agent")
			what := "Subscribed"
			if !made {
				b.dups++
				what = "This chat already has this subscription"
			}
			return tool.Result{Text: fmt.Sprintf("%s: %s = %s. This chat's subscriptions:\n%s", what, x.ID, x.sub, b.subsText(agent))}, nil
		})
	unsubscribe := tool.Func(tool.Spec{Name: "unsubscribe", Description: "End one of this chat's subscriptions, by its ID.",
		Schema: jsonSchema(`{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`)},
		func(_ context.Context, env *tool.Env, a struct {
			ID string `json:"id"`
		}) (tool.Result, error) {
			b.mu.Lock()
			defer b.mu.Unlock()
			agent := b.keys[env.Chat]
			for i, s := range b.subs {
				if s.ID == a.ID && s.Agent == agent && !s.Ended {
					b.subs[i].Ended, b.subs[i].EndedAt = true, b.msg
					return tool.Result{Text: fmt.Sprintf("Ended %s. This chat's subscriptions:\n%s", a.ID, b.subsText(agent))}, nil
				}
			}
			return tool.Result{}, &tool.Error{Msg: fmt.Sprintf("this chat has no subscription %q; it has:\n%s", a.ID, b.subsText(agent))}
		})
	publish := tool.Func(tool.Spec{Name: "publish",
		Description: "Publish a report on one of the project's channels. Chats that subscribe to the channel get its title as a notice at the start of their next turn, and read the body with read_report. " +
			"Publish when your work changed or found something another chat depends on; don't publish routine work. The title is one line that says what happened.",
		Schema: jsonSchema(`{"type":"object","required":["channel","title","body"],"properties":{
			"channel":{"type":"string","description":"one of the project's channels"},
			"title":{"type":"string","description":"one line that says what happened"},
			"body":{"type":"string","description":"the report: what happened, with the numbers and IDs a reader needs"}}}`)},
		func(_ context.Context, env *tool.Env, a struct {
			Channel string `json:"channel"`
			Title   string `json:"title"`
			Body    string `json:"body"`
		}) (tool.Result, error) {
			b.mu.Lock()
			agent := b.keys[env.Chat]
			p := pubRec{Msg: b.msg, Agent: agent, Channel: strings.TrimSpace(a.Channel), Title: strings.TrimSpace(a.Title), Body: a.Body}
			if !slices.Contains(channelNames(), p.Channel) {
				p.Refused = true
				b.pubs = append(b.pubs, p)
				b.mu.Unlock()
				var list []string
				for _, c := range channels {
					list = append(list, fmt.Sprintf("%s (%s)", c.Name, c.Desc))
				}
				return tool.Result{}, &tool.Error{Msg: fmt.Sprintf("there is no channel %q. This project's channels are: %s", p.Channel, strings.Join(list, "; "))}
			}
			if p.Title == "" {
				b.mu.Unlock()
				return tool.Result{}, &tool.Error{Msg: "title is empty", Err: tool.ErrArgs}
			}
			p.Title = clipRunes(p.Title, 200)
			p.Report = fmt.Sprintf("rp_%d", len(b.pubs)+1)
			b.pubs = append(b.pubs, p)
			b.mu.Unlock()
			b.emit(event{Kind: p.Channel, By: agent,
				Text: fmt.Sprintf("[%s by chat %q: %s · report %s, read it with read_report]", p.Channel, agentByKey(agent).Title, p.Title, p.Report)})
			return tool.Result{Text: fmt.Sprintf("Published report %s on %s.", p.Report, p.Channel)}, nil
		})
	readReport := tool.Func(tool.Spec{Name: "read_report", Description: "Read a report published on a channel, by its ID (from a notice or the channel list).",
		Schema: jsonSchema(`{"type":"object","required":["id"],"properties":{"id":{"type":"string"}}}`)},
		func(_ context.Context, _ *tool.Env, a struct {
			ID string `json:"id"`
		}) (tool.Result, error) {
			b.mu.Lock()
			defer b.mu.Unlock()
			for _, p := range b.pubs {
				if p.Report != "" && p.Report == strings.TrimSpace(a.ID) {
					return tool.Result{Text: fmt.Sprintf("Report %s on %s by chat %q:\n%s\n\n%s", p.Report, p.Channel, agentByKey(p.Agent).Title, p.Title, p.Body)}, nil
				}
			}
			return tool.Result{}, &tool.Error{Msg: fmt.Sprintf("no report %q", a.ID)}
		})
	return []tool.Tool{subscribe, unsubscribe, publish, readReport}
}

// snapshot copies what the scoring reads.
func (b *bus) snapshot() ([]subRec, []event, []delivery, []pubRec, int, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.subs), slices.Clone(b.events), slices.Clone(b.deliv), slices.Clone(b.pubs), b.own, b.dups
}

// clipRunes shortens s to n runes.
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
