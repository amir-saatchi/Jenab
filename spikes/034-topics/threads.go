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
	"github.com/amir-saatchi/jenab/internal/tool"
)

// thread is the chat's main thread or one of its topics (conditions 2
// and 3). Each topic is a chat of its own in the project; the harness
// keeps the parent link here. Each thread has its own notes and writes
// only those.
type thread struct {
	ID         string   `json:"id"` // "main", or "t1", "t2", …
	Chat       id.Chat  `json:"chat"`
	Title      string   `json:"title"`
	Labels     []string `json:"labels,omitempty"` // the scripted labels it was opened or merged for
	Summary    string   `json:"summary,omitempty"`
	Notes      []note   `json:"notes,omitempty"`
	Open       []string `json:"open,omitempty"`
	MergedInto string   `json:"merged_into,omitempty"`
	OpenedAt   int      `json:"opened_at"` // the message that opened it; -1 for the main thread
}

func (x *thread) main() bool { return x.ID == mainThread }

const mainThread = "main"

// note is one line added to a thread's or a subject's notes. Seq orders
// the notes of merged topics in time.
type note struct {
	Text   string `json:"text"`
	Source string `json:"source,omitempty"`
	Seq    int    `json:"seq"`
}

// proposal is one propose_topic call and the scripted user's answer.
type proposal struct {
	Msg      int    `json:"msg"`
	From     string `json:"from"`  // the thread it was made in
	Label    string `json:"label"` // the message's label
	Title    string `json:"title,omitempty"`
	TopicID  string `json:"topic_id,omitempty"`
	Accepted bool   `json:"accepted"`
	Into     string `json:"into,omitempty"` // the topic the message moved to
	// Why it was declined, or what an accepted one did: "new", "existing",
	// "noise" (a main message, or one already in its topic), "duplicate"
	// (a new topic for a label that has one), "wrong" (another topic),
	// "again" (a second proposal in one turn).
	Verdict string `json:"verdict"`
}

// threads are the main thread and the topics of one chat.
type threads struct {
	mu      sync.Mutex
	title   string // the chat's title
	role    string
	model   string
	propose bool     // condition 3: the agent has propose_topic
	labels  []string // the scripted messages' labels, by index

	// Round 2 (conditions 5–6, sessions.go): sub-sessions instead of topics.
	sessions  bool
	everyTurn bool                                                     // condition 5: every sub-session turn sends a notice up
	withSum   bool                                                     // condition 6: the list line has the summary
	asks      []bool                                                   // the messages that ask for a session, by index
	start     func(ctx context.Context, ch id.Chat, text string) error // sends a sub-session's first message
	started   map[id.Chat]int                                          // sub-sessions whose first turn, started at that message, isn't read yet
	creations []creation
	ups       []upNotice
	pending   []int // ups not given to the parent yet

	list      []*thread // the main thread first
	cur       map[id.Chat]int
	moves     map[id.Chat]*thread // accepted proposals the harness hasn't played yet
	proposals []proposal
	seq       int
}

func newThreads(sc scenarioDef, model string, mainChat id.Chat, cond int) *threads {
	th := &threads{title: sc.Title, role: sc.Role, model: model, propose: cond == condPropose,
		sessions: sessions(cond), everyTurn: cond == condNotice, withSum: cond == condSummary,
		cur: map[id.Chat]int{}, moves: map[id.Chat]*thread{}, started: map[id.Chat]int{}}
	for _, m := range sc.Messages {
		th.labels = append(th.labels, m.Label)
		th.asks = append(th.asks, len(m.Ask) > 0)
	}
	th.list = []*thread{{ID: mainThread, Chat: mainChat, Title: sc.Title, OpenedAt: -1}}
	return th
}

// at says which message the chat is playing, so the scripted user can
// answer its proposals.
func (th *threads) at(ch id.Chat, msg int) {
	th.mu.Lock()
	th.cur[ch] = msg
	th.mu.Unlock()
}

// byChat is the thread of a chat; nil for a chat that isn't one.
func (th *threads) byChat(ch id.Chat) *thread {
	for _, x := range th.list {
		if x.Chat == ch {
			return x
		}
	}
	return nil
}

// find is the topic with this id, following merges.
func (th *threads) find(tid string) *thread {
	tid = strings.TrimSpace(tid)
	for range len(th.list) + 1 {
		i := slices.IndexFunc(th.list, func(x *thread) bool { return strings.EqualFold(x.ID, tid) })
		if i < 0 || th.list[i].main() {
			return nil
		}
		if th.list[i].MergedInto == "" {
			return th.list[i]
		}
		tid = th.list[i].MergedInto
	}
	return nil
}

// topicFor is the live topic that holds a label; nil when none does.
func (th *threads) topicFor(label string) *thread {
	for _, x := range th.list {
		if !x.main() && x.MergedInto == "" && slices.Contains(x.Labels, label) {
			return x
		}
	}
	return nil
}

// TopicFor is topicFor under the lock.
func (th *threads) TopicFor(label string) *thread {
	th.mu.Lock()
	defer th.mu.Unlock()
	return th.topicFor(label)
}

// Thread is byChat under the lock.
func (th *threads) Thread(ch id.Chat) *thread {
	th.mu.Lock()
	defer th.mu.Unlock()
	return th.byChat(ch)
}

// kind is what the threads below the main one are called.
func (th *threads) kind() string {
	if th.sessions {
		return "sub-session"
	}
	return "topic"
}

// mainName is what the main thread is called.
func (th *threads) mainName() string {
	if th.sessions {
		return "the parent chat"
	}
	return "the main thread"
}

func (th *threads) topicIDs() string {
	var ids []string
	for _, x := range th.list {
		if !x.main() && x.MergedInto == "" {
			ids = append(ids, x.ID)
		}
	}
	if len(ids) == 0 {
		return "none"
	}
	return strings.Join(ids, ", ")
}

// nextID is the id the next topic gets. The caller holds th.mu.
func (th *threads) nextID() string {
	if th.sessions {
		return fmt.Sprintf("s%d", len(th.list))
	}
	return fmt.Sprintf("t%d", len(th.list))
}

// NextID is nextID under the lock.
func (th *threads) NextID() string {
	th.mu.Lock()
	defer th.mu.Unlock()
	return th.nextID()
}

// topicChatTitle is a topic's or sub-session's chat title. The store keeps chat titles
// unique in a project, and a topic may share its title with another
// chat, so the topic's id comes first.
func topicChatTitle(tid, title string) string { return tid + " · " + title }

// add makes a topic for a chat the caller has created. The caller holds
// th.mu.
func (th *threads) add(ch id.Chat, title, label string, msg int) *thread {
	x := &thread{ID: th.nextID(), Chat: ch, Title: title, OpenedAt: msg}
	if label != "" && label != mainLabel {
		x.Labels = []string{label}
	}
	th.list = append(th.list, x)
	return x
}

// Add is add under the lock: the scripted user opens a topic
// (condition 2).
func (th *threads) Add(ch id.Chat, title, label string, msg int) *thread {
	th.mu.Lock()
	defer th.mu.Unlock()
	return th.add(ch, title, label, msg)
}

// takeMove returns and forgets the topic an accepted proposal in ch moved
// the message to.
func (th *threads) takeMove(ch id.Chat) *thread {
	th.mu.Lock()
	defer th.mu.Unlock()
	t := th.moves[ch]
	delete(th.moves, ch)
	return t
}

// state is what the index shows after the title.
func state(open []string) string {
	if len(open) == 0 {
		return "nothing open"
	}
	return fmt.Sprintf("%d open", len(open))
}

// index is one line per live topic. The caller holds th.mu.
func (th *threads) index() string {
	var b strings.Builder
	for _, x := range th.list {
		if x.main() || x.MergedInto != "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s · %s · %s · chat %s", x.ID, x.Title, state(x.Open), x.Chat)
		if th.withSum && x.Summary != "" {
			b.WriteString(" · " + x.Summary)
		}
	}
	if b.Len() == 0 {
		return fmt.Sprintf("No %ss yet.", th.kind())
	}
	return b.String()
}

// notesText is a thread's notes as block 5 shows them.
func notesText(summary string, notes []note, open []string) string {
	if summary == "" && len(notes) == 0 && len(open) == 0 {
		return "No notes yet."
	}
	var b strings.Builder
	if summary != "" {
		b.WriteString("Summary: " + summary)
	}
	if len(notes) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Notes:")
		for _, n := range notes {
			b.WriteString("\n- " + n.Text)
		}
	}
	b.WriteString("\nOpen:")
	if len(open) == 0 {
		b.WriteString(" nothing")
	}
	for _, o := range open {
		b.WriteString("\n- " + o)
	}
	return strings.TrimSpace(b.String())
}

// The topics rule, the same for every model and every thread (SPEC 1).
// Condition 3 adds the proposal paragraph.
const topicsRule = `This chat has a main thread and topics. A topic is a thread one level below the chat for one subject the user comes back to, such as a table or a pipeline. Each thread has its own history and its own notes. The main thread is for small questions, overview and questions that span topics; it sees the topic index. A topic sees the main thread's notes, read-only, and its own.
Notes keep the state of this thread's work across turns. When your work in this thread creates, changes or decides something, call update_notes: add a note that says what changed and why, update the summary when the current state changed, and keep open as the full list of what is still waiting, and on what. Call it together with your last tool call. Skip it for plain questions. Facts that hold for the whole project, such as conventions and decisions with their reasons, also go to memory with update_memory.
Other threads aren't in your context. Find their work with search_history (scope project) and read_messages with their chat_id.
When the user says two topics are one thing, join them with merge_topics.`

const proposeRule = `Only the user opens topics. When a message starts a new subject the user is likely to come back to, call propose_topic with a short title named after the subject. When a message belongs to an existing topic and you are not in it, call propose_topic with its id. Don't propose a topic for a plain question, for an overview or for a question that spans topics, and never propose a second topic for a subject that has one. If the user accepts, the message moves to that topic and is answered there: end your turn with one short line. If they decline, answer here.`

func (th *threads) rule() string {
	if th.propose {
		return topicsRule + "\n" + proposeRule
	}
	return topicsRule
}

// block is block 5 for a chat: in the main thread its notes and the
// topic index, in a topic the main thread's notes (read-only) and its
// own. It is built at a cut, so it is as of the last cut.
func (th *threads) block(ch id.Chat) string {
	th.mu.Lock()
	defer th.mu.Unlock()
	x := th.byChat(ch)
	if x == nil {
		return ""
	}
	if th.sessions {
		return th.sessionBlock(x)
	}
	main := th.list[0]
	if x.main() {
		return "Session notes and topics\n" + th.rule() +
			"\n\nThis chat's notes (as of the last cut):\n" + notesText(x.Summary, x.Notes, x.Open) +
			"\n\nTopics of this chat (as of the last cut):\n" + th.index()
	}
	return fmt.Sprintf("Session notes\nYou are in topic %s %q of the chat %q. Its main thread is chat %s.\n%s", x.ID, x.Title, th.title, main.Chat, th.rule()) +
		"\n\nThe main thread's notes (read-only; only the main thread writes them):\n" + notesText(main.Summary, main.Notes, main.Open) +
		"\n\nThis topic's notes (as of the last cut):\n" + notesText(x.Summary, x.Notes, x.Open)
}

// tools are update_notes and merge_topics, and propose_topic in
// condition 3. The tool registry is shared by all chats, so each tool acts
// on the thread it is called in (env.Chat).
func (th *threads) tools() []tool.Tool {
	ts := []tool.Tool{th.updateNotes(), th.merge()}
	if th.propose {
		ts = append(ts, th.proposeTopic())
	}
	if th.sessions {
		ts = append(ts, th.createSession())
	}
	return ts
}

func (th *threads) updateNotes() tool.Tool {
	type args struct {
		Title   string    `json:"title"`
		Summary *string   `json:"summary"`
		Note    string    `json:"note"`
		Open    *[]string `json:"open"`
	}
	return tool.Func(tool.Spec{
		Name: "update_notes",
		Description: th.words("Update this thread's notes: the state of its work, what changed and why, and what is still waiting. " +
			"Each thread (the main thread or a topic) has its own notes and writes only those. Fields you leave out stay as they are. " +
			"summary is the current state and replaces the old one. note is added to the notes, which are never rewritten: say what changed and why. " +
			"open lists what is still waiting, and on what; it replaces the old list, so send all of it, or [] when nothing is waiting. " +
			"title renames a topic; the main thread keeps the chat's title."),
		Schema: json.RawMessage(`{
			"type":"object","additionalProperties":false,
			"properties":{
				"title":{"type":"string","description":"a ` + th.kind() + `'s new title, named after its subject"},
				"summary":{"type":"string","description":"the current state; replaces the old summary"},
				"note":{"type":"string","description":"added to the notes: what changed and why"},
				"open":{"type":"array","items":{"type":"string"},"description":"the full list of what is still waiting; replaces the old one"}}}`),
		Effects: tool.ReadsDB,
	}, func(_ context.Context, env *tool.Env, a args) (tool.Result, error) {
		th.mu.Lock()
		defer th.mu.Unlock()
		x := th.byChat(env.Chat)
		if x == nil {
			return tool.Result{}, tool.Errorf("this chat has no notes")
		}
		if x.MergedInto != "" {
			x = th.find(x.MergedInto)
		}
		if t := strings.TrimSpace(a.Title); t != "" {
			if x.main() {
				return tool.Result{}, tool.Errorf("%s keeps the chat's title; title is for %ss", th.mainName(), th.kind())
			}
			x.Title = t
		}
		if a.Summary != nil {
			x.Summary = *a.Summary
		}
		if t := strings.TrimSpace(a.Note); t != "" {
			th.seq++
			x.Notes = append(x.Notes, note{Text: t, Source: string(env.Message), Seq: th.seq})
		}
		if a.Open != nil {
			x.Open = slices.Clone(*a.Open)
		}
		name := th.mainName() + "'s notes"
		if !x.main() {
			name = fmt.Sprintf("%s %s %q", th.kind(), x.ID, x.Title)
		}
		return tool.Result{Text: fmt.Sprintf("Updated %s: %s.", name, state(x.Open))}, nil
	})
}

// merge is merge_topics. The model writes the title; the notes are joined
// in time order and the open items added, so nothing is rewritten. The
// other ids lead to the kept topic from now on, and their chats stay
// readable with read_messages.
func (th *threads) merge() tool.Tool {
	type args struct {
		IDs   []string `json:"ids"`
		Title string   `json:"title"`
	}
	name, example := "merge_topics", "t1"
	if th.sessions {
		name, example = "merge_sessions", "s1"
	}
	return tool.Func(tool.Spec{
		Name: name,
		Description: th.words("Join topics that the user says are one thing. The first id is kept; the others' notes and open items move into it in time order, and their ids lead to it from now on. " +
			"Their messages stay where they are, readable with read_messages. title is the joined title, named after the subject."),
		Schema: json.RawMessage(`{
			"type":"object","required":["ids","title"],"additionalProperties":false,
			"properties":{
				"ids":{"type":"array","items":{"type":"string"},"minItems":2,"description":"the ` + th.kind() + `s to join, such as ` + example + `; the first is kept"},
				"title":{"type":"string"}}}`),
		Effects: tool.ReadsDB,
	}, func(_ context.Context, _ *tool.Env, a args) (tool.Result, error) {
		th.mu.Lock()
		defer th.mu.Unlock()
		var xs []*thread
		for _, tid := range a.IDs {
			x := th.find(tid)
			if x == nil {
				return tool.Result{}, tool.Errorf("no %s %s (%ss: %s)", th.kind(), tid, th.kind(), th.topicIDs())
			}
			if !slices.Contains(xs, x) {
				xs = append(xs, x)
			}
		}
		if len(xs) < 2 {
			return tool.Result{}, tool.Errorf("%s needs two different %ss", name, th.kind())
		}
		if strings.TrimSpace(a.Title) == "" {
			return tool.Result{}, tool.Errorf("%s needs title, the joined title", name)
		}
		keep := xs[0]
		var was []string
		for _, x := range xs {
			was = append(was, fmt.Sprintf("%s (%s)", x.ID, x.Title))
		}
		var summaries []string
		for _, x := range xs {
			if x.Summary != "" {
				summaries = append(summaries, x.Summary)
			}
		}
		for _, x := range xs[1:] {
			keep.Notes = append(keep.Notes, x.Notes...)
			for _, o := range x.Open {
				if !slices.Contains(keep.Open, o) {
					keep.Open = append(keep.Open, o)
				}
			}
			for _, l := range x.Labels {
				if !slices.Contains(keep.Labels, l) {
					keep.Labels = append(keep.Labels, l)
				}
			}
			x.MergedInto = keep.ID
		}
		slices.SortStableFunc(keep.Notes, func(a, b note) int { return a.Seq - b.Seq })
		keep.Title = strings.TrimSpace(a.Title)
		keep.Summary = strings.Join(summaries, " / ")
		return tool.Result{Text: fmt.Sprintf("Merged %s into %s %q: %s.", strings.Join(was, " + "), keep.ID, keep.Title, state(keep.Open))}, nil
	})
}

// proposeTopic is propose_topic (condition 3). The scripted user answers
// inside the call, as they would answer a card: they accept a proposal
// that matches the message's label and decline any other.
func (th *threads) proposeTopic() tool.Tool {
	type args struct {
		Title   string `json:"title"`
		TopicID string `json:"topic_id"`
		Reason  string `json:"reason"`
	}
	return tool.Func(tool.Spec{
		Name: "propose_topic",
		Description: "Ask the user to move their current message into a topic: a new one (title) or an existing one (topic_id). Give one of the two. " +
			"The user accepts or declines at once. If they accept, the message is answered in that topic, with that topic's context: end your turn with one short line and don't do the work here. " +
			"If they decline, answer here.",
		Schema: json.RawMessage(`{
			"type":"object","additionalProperties":false,
			"properties":{
				"title":{"type":"string","description":"a new topic's title, named after its subject"},
				"topic_id":{"type":"string","description":"an existing topic, such as t2"},
				"reason":{"type":"string","description":"one short line for the user"}}}`),
		Effects: tool.ReadsDB,
	}, func(ctx context.Context, env *tool.Env, a args) (tool.Result, error) {
		title, tid := strings.TrimSpace(a.Title), strings.TrimSpace(a.TopicID)
		if (title == "") == (tid == "") {
			return tool.Result{}, tool.Errorf("give either title, for a new topic, or topic_id, for an existing one")
		}
		th.mu.Lock()
		defer th.mu.Unlock()
		from := th.byChat(env.Chat)
		if from == nil {
			return tool.Result{}, tool.Errorf("this chat has no topics")
		}
		msg := th.cur[env.Chat]
		label := th.labels[msg]
		p := proposal{Msg: msg, From: from.ID, Label: label, Title: title, TopicID: tid}
		var into *thread
		switch {
		case th.moves[env.Chat] != nil:
			p.Verdict = "again"
		case tid != "":
			t := th.find(tid)
			if t == nil {
				return tool.Result{}, tool.Errorf("no topic %s (topics: %s)", tid, th.topicIDs())
			}
			p.TopicID = t.ID
			switch {
			case label == mainLabel || slices.Contains(from.Labels, label) || t == from:
				p.Verdict = "noise"
			case slices.Contains(t.Labels, label):
				p.Verdict, into = "existing", t
			default:
				p.Verdict = "wrong"
			}
		default:
			switch {
			case label == mainLabel || slices.Contains(from.Labels, label):
				p.Verdict = "noise"
			case th.topicFor(label) != nil:
				p.Verdict = "duplicate"
			default:
				c, err := env.Project.Chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: topicChatTitle(th.nextID(), title), Role: th.role, Model: th.model})
				if err != nil {
					return tool.Result{}, err
				}
				p.Verdict, into = "new", th.add(c.ID, title, label, msg)
			}
		}
		if into == nil {
			th.proposals = append(th.proposals, p)
			return tool.Result{Text: "The user declined: answer here, in this thread."}, nil
		}
		p.Accepted, p.Into = true, into.ID
		th.proposals = append(th.proposals, p)
		th.moves[env.Chat] = into
		return tool.Result{Text: fmt.Sprintf("The user accepted. Their message moves to topic %s %q and is answered there. End this turn now with one short line; don't do the work here.", into.ID, into.Title)}, nil
	})
}

func (th *threads) snapshot() ([]thread, []proposal) {
	th.mu.Lock()
	defer th.mu.Unlock()
	out := make([]thread, len(th.list))
	for i, x := range th.list {
		out[i] = *x
		out[i].Labels = slices.Clone(x.Labels)
		out[i].Notes = slices.Clone(x.Notes)
		out[i].Open = slices.Clone(x.Open)
	}
	return out, slices.Clone(th.proposals)
}

// openOf is the number of open items of the chat's thread, following
// merges.
func (th *threads) openOf(ch id.Chat) int {
	th.mu.Lock()
	defer th.mu.Unlock()
	x := th.byChat(ch)
	if x == nil {
		return 0
	}
	if x.MergedInto != "" {
		x = th.find(x.MergedInto)
	}
	return len(x.Open)
}
