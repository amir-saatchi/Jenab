package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/tool"
)

// Round 2 (conditions 5 and 6): visible sub-sessions. They are round 1's
// topics with another way in and a way up. The parent starts one with
// create_session, or the scripted user opens one; a sub-session's replies
// reach the parent as notices (the finish notice in both conditions, and
// every turn's in condition 5), and in condition 6 the parent's list line
// has the sub-session's summary.

// creation is one create_session call.
type creation struct {
	Msg     int    `json:"msg"`
	From    string `json:"from"`  // the thread it was called in
	Label   string `json:"label"` // the message's label
	Title   string `json:"title"`
	Message string `json:"message"`
	Session string `json:"session,omitempty"` // the sub-session it made
	Asked   bool   `json:"asked,omitempty"`   // the message asked for a session
	Error   string `json:"error,omitempty"`
}

// upNotice is one notice from a sub-session to the parent.
type upNotice struct {
	Msg       int    `json:"msg"` // the message whose turn ended in the sub-session
	Session   string `json:"session"`
	Finish    bool   `json:"finish,omitempty"` // the turn create_session started
	Text      string `json:"text"`
	Delivered int    `json:"delivered"` // the message whose parent turn got it; -1 while none has
}

// noticeChars is how much of a sub-session's reply a notice carries.
const noticeChars = 1000

func sessions(cond int) bool { return cond == condNotice || cond == condSummary }

// words puts round 1's topic wording into the sub-session wording.
func (th *threads) words(s string) string {
	if !th.sessions {
		return s
	}
	return strings.NewReplacer("the main thread", "the parent chat", "Each thread", "Each chat", "this thread's", "this chat's",
		"topics", "sub-sessions", "topic", "sub-session").Replace(s)
}

// The sub-session rules, the same for every model (SPEC 1). The parent's
// follows delegation guidance v1 (SPEC 8.6), cut down to sub-sessions.
const parentRule = `This chat can have sub-sessions: chats one level below it, each for one piece of work the user comes back to, such as a table or a pipeline. The user sees them under this chat and can open and continue them. Each has its own history and its own notes, and sees this chat's notes, read-only.
Start one with create_session when the user asks for a separate session, or for lasting work the user will come back to. Give it a short title named after the work, and the task as its first message with everything it needs: it doesn't see this chat's history. Then don't do that work here. Answer plain questions, overviews and questions that span sub-sessions yourself, and never start a second sub-session for work that has one.
When a sub-session's reply reaches you as a notice, pass on what matters in your own words. Don't send the work again or repeat what you already said.
Notes keep the state of this chat's work across turns. When your work here creates, changes or decides something, call update_notes: add a note that says what changed and why, update the summary when the current state changed, and keep open as the full list of what is still waiting, and on what. Call it together with your last tool call. Skip it for plain questions. Facts that hold for the whole project, such as conventions and decisions with their reasons, also go to memory with update_memory.
Sub-sessions' work isn't in your context. Find it with search_history (scope project) and read_messages with their chat_id.
When the user says two sub-sessions are one thing, join them with merge_sessions.`

const subRule = `This chat is a sub-session: a chat one level below its parent, for one piece of work. It has its own history and its own notes, and sees the parent's notes, read-only. It can't start sub-sessions.
Notes keep the state of this sub-session's work across turns. When your work here creates, changes or decides something, call update_notes: add a note that says what changed and why, update the summary when the current state changed, and keep open as the full list of what is still waiting, and on what. Call it together with your last tool call. Skip it for plain questions. Facts that hold for the whole project, such as conventions and decisions with their reasons, also go to memory with update_memory.
Other chats aren't in your context. Find their work with search_history (scope project) and read_messages with their chat_id.`

// sessionBlock is block 5 in conditions 5–6: in the parent its notes and
// the list of sub-sessions, in a sub-session the parent's notes
// (read-only) and its own. The caller holds th.mu.
func (th *threads) sessionBlock(x *thread) string {
	parent := th.list[0]
	if x.main() {
		return "Session notes and sub-sessions\n" + parentRule +
			"\n\nThis chat's notes (as of the last cut):\n" + notesText(x.Summary, x.Notes, x.Open) +
			"\n\nSub-sessions of this chat (as of the last cut):\n" + th.index()
	}
	return fmt.Sprintf("Session notes\nYou are in sub-session %s %q of the chat %q. Its parent is chat %s.\n%s", x.ID, x.Title, th.title, parent.Chat, subRule) +
		"\n\nThe parent's notes (read-only; only the parent writes them):\n" + notesText(parent.Summary, parent.Notes, parent.Open) +
		"\n\nThis sub-session's notes (as of the last cut):\n" + notesText(x.Summary, x.Notes, x.Open)
}

// createSession is create_session. It makes the sub-session's chat, puts
// it under the message's label (unless the label has one), and sends the
// first message, whose turn runs at once. The harness waits for it before
// the next message.
func (th *threads) createSession() tool.Tool {
	type args struct {
		Title   string `json:"title"`
		Message string `json:"message"`
	}
	return tool.Func(tool.Spec{
		Name: "create_session",
		Description: "Start a sub-session under this chat: a chat one level below it, for one piece of work, that the user sees under this chat and can open and continue. " +
			"message is its first user message: the task, with everything it needs, since it doesn't see this chat's history. Its turn starts at once; when it ends, its reply reaches this chat as a notice. " +
			"Only a chat that isn't a sub-session can start one.",
		Schema: json.RawMessage(`{
			"type":"object","required":["title","message"],"additionalProperties":false,
			"properties":{
				"title":{"type":"string","description":"a short title, named after the work"},
				"message":{"type":"string","description":"the sub-session's first message: the task and everything it needs"}}}`),
		Effects: tool.ReadsDB,
	}, func(ctx context.Context, env *tool.Env, a args) (tool.Result, error) {
		title, msg := strings.TrimSpace(a.Title), strings.TrimSpace(a.Message)
		if title == "" || msg == "" {
			return tool.Result{}, tool.Errorf("create_session needs title and message")
		}
		th.mu.Lock()
		from := th.byChat(env.Chat)
		if from == nil {
			th.mu.Unlock()
			return tool.Result{}, tool.Errorf("this chat can't start sub-sessions")
		}
		i := th.cur[env.Chat]
		c := creation{Msg: i, From: from.ID, Label: th.labels[i], Title: title, Message: msg, Asked: th.asks[i]}
		if !from.main() {
			c.Error = "one level"
			th.creations = append(th.creations, c)
			th.mu.Unlock()
			return tool.Result{}, tool.Errorf("a sub-session can't start sub-sessions; only its parent can")
		}
		ch, err := env.Project.Chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: topicChatTitle(th.nextID(), title), Role: th.role, Model: th.model})
		if err != nil {
			th.mu.Unlock()
			return tool.Result{}, err
		}
		label := c.Label
		if th.topicFor(label) != nil {
			label = "" // the label keeps the sub-session it has
		}
		x := th.add(ch.ID, title, label, i)
		th.started[x.Chat] = i
		th.cur[x.Chat] = i // its first turn plays the same message
		c.Session = x.ID
		th.creations = append(th.creations, c)
		th.mu.Unlock()
		if err := th.start(ctx, x.Chat, msg); err != nil {
			return tool.Result{}, err
		}
		return tool.Result{Text: fmt.Sprintf("Started sub-session %s %q (chat %s). Its turn is running; its reply reaches this chat as a notice.", x.ID, title, x.Chat)}, nil
	})
}

// Started is the sub-sessions create_session started at message i whose
// first turn isn't read yet, so the harness waits for it and reads it.
func (th *threads) Started(i int) []id.Chat {
	th.mu.Lock()
	defer th.mu.Unlock()
	var out []id.Chat
	for _, x := range th.list {
		if n, ok := th.started[x.Chat]; ok && n == i {
			out = append(out, x.Chat)
		}
	}
	return out
}

// up queues a notice to the parent when a sub-session's turn ends: for the
// turn create_session started, and in condition 5 for every turn.
func (th *threads) up(ch id.Chat, msg int, reply string) {
	th.mu.Lock()
	defer th.mu.Unlock()
	if !th.sessions {
		return
	}
	x := th.byChat(ch)
	if x == nil || x.main() {
		return
	}
	_, finish := th.started[ch]
	delete(th.started, ch)
	if !finish && !th.everyTurn {
		return
	}
	if r := []rune(strings.TrimSpace(reply)); len(r) > noticeChars {
		reply = string(r[:noticeChars]) + "…"
	} else if len(r) == 0 {
		reply = "(no reply)"
	}
	th.pending = append(th.pending, len(th.ups))
	th.ups = append(th.ups, upNotice{Msg: msg, Session: x.ID, Finish: finish, Text: fmt.Sprintf("Session %s · %s: %s", x.ID, x.Title, strings.TrimSpace(reply)), Delivered: -1})
}

// notices is the Notices hook: the parent gets the queued notices once.
func (th *threads) notices(ch id.Chat) []chat.Notice {
	th.mu.Lock()
	defer th.mu.Unlock()
	if x := th.byChat(ch); x == nil || !x.main() || len(th.pending) == 0 {
		return nil
	}
	var ns []chat.Notice
	for _, k := range th.pending {
		th.ups[k].Delivered = th.cur[ch]
		ns = append(ns, chat.Notice{Kind: agent.NoticeSessionUpdate, Text: th.ups[k].Text})
	}
	th.pending = nil
	return ns
}

func (th *threads) sessionsSnapshot() ([]creation, []upNotice) {
	th.mu.Lock()
	defer th.mu.Unlock()
	return append([]creation(nil), th.creations...), append([]upNotice(nil), th.ups...)
}
