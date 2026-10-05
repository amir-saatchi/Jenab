package app

import (
	"context"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

// SnapshotTurns is how many of a chat's last turns a snapshot holds; the
// frontend loads older ones with Messages as the user scrolls up.
const SnapshotTurns = 30

// ChatService is the chats of a project (SPEC 8.3, 8.6, 8.8).
type ChatService struct {
	base
	orch     *agent.Orchestrator
	projects *project.Manager
}

// ChatItem is a row of the chat list.
type ChatItem struct {
	Chat chat.Chat `json:"chat"`
	// Status is "in a turn", "waiting for the user" or "" when idle.
	Status       string     `json:"status"`
	LastActivity *time.Time `json:"last_activity,omitempty"` // the newest message; nil without messages
}

// List returns the project's chats, the Mother chat first.
func (s *ChatService) List(ctx context.Context, p id.Project) (items []ChatItem, err error) {
	defer s.guard("chat.list", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		chats, err := proj.Chats.Chats(ctx)
		if err != nil {
			return err
		}
		last, err := proj.Chats.LastActivity(ctx)
		if err != nil {
			return err
		}
		items = make([]ChatItem, 0, len(chats))
		for _, c := range chats {
			it := ChatItem{Chat: c, Status: s.orch.ChatStatus(c.ID)}
			if t, ok := last[c.ID]; ok {
				it.LastActivity = &t
			}
			items = append(items, it)
		}
		return nil
	})
	return items, err
}

// Create adds a chat for the user, with an optional title and role.
func (s *ChatService) Create(ctx context.Context, p id.Project, title, role string) (c chat.Chat, err error) {
	defer s.guard("chat.create", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		c, err = proj.Chats.CreateChat(ctx, id.SourceUser, chat.Chat{Title: title, Role: role})
		return err
	})
	return c, err
}

// ChatSnapshot is a chat as it is now (Q32): its last turns, the sequence
// number they are current at, and the turn still running. Events with a
// lower Seq are already in it.
type ChatSnapshot struct {
	Chat     chat.Chat      `json:"chat"`
	Messages []chat.Message `json:"messages"`
	From     int            `json:"from"` // the first turn in Messages; older ones come from Messages
	Seq      uint64         `json:"seq"`
	Live     agent.Live     `json:"live"` // the answer still streaming, a wait or a retry
}

// Snapshot returns the chat with its last SnapshotTurns turns. Live is
// read first, so a part that finishes meanwhile is in Messages.
func (s *ChatService) Snapshot(ctx context.Context, p id.Project, c id.Chat) (snap ChatSnapshot, err error) {
	defer s.guard("chat.snapshot", &err)
	snap.Live = s.orch.Live(p, c)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		ch, err := proj.Chats.Chat(ctx, c)
		if err != nil {
			return err
		}
		last, err := proj.Chats.LastTurn(ctx, c)
		if err != nil {
			return err
		}
		snap.Chat, snap.From = ch, max(1, last-SnapshotTurns+1)
		snap.Messages, snap.Seq, err = proj.Chats.Messages(ctx, c, snap.From, 0)
		return err
	})
	if snap.Messages == nil {
		snap.Messages = []chat.Message{}
	}
	return snap, err
}

// Messages returns the chat's turns from through to, for scrolling up;
// to 0 means up to the last turn.
func (s *ChatService) Messages(ctx context.Context, p id.Project, c id.Chat, from, to int) (ms []chat.Message, err error) {
	defer s.guard("chat.messages", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		ms, _, err = proj.Chats.Messages(ctx, c, from, to)
		return err
	})
	if ms == nil {
		ms = []chat.Message{}
	}
	return ms, err
}

// Send adds the user's message; the turn's progress comes as events.
// Pictures come with the composer's attachments (P1-15).
func (s *ChatService) Send(ctx context.Context, p id.Project, c id.Chat, text string) (m id.Message, err error) {
	defer s.guard("chat.send", &err)
	return s.orch.Send(ctx, p, c, agent.UserMessage{Text: text})
}

// Stop ends the chat's turn (*Stop*, 8.3).
func (s *ChatService) Stop(ctx context.Context, p id.Project, c id.Chat) (err error) {
	defer s.guard("chat.stop", &err)
	s.orch.Stop(p, c)
	return nil
}

// Retry tries the failed request again (*Retry*, *Retry now*).
func (s *ChatService) Retry(ctx context.Context, p id.Project, c id.Chat) (err error) {
	defer s.guard("chat.retry", &err)
	return s.orch.Retry(ctx, p, c)
}

// Answer answers the approval card or question form the turn waits for.
func (s *ChatService) Answer(ctx context.Context, p id.Project, c id.Chat, a agent.Answer) (err error) {
	defer s.guard("chat.answer", &err)
	return s.orch.Answer(p, c, a)
}

// Clear removes the chat's messages and session notes (8.6).
func (s *ChatService) Clear(ctx context.Context, p id.Project, c id.Chat) (err error) {
	defer s.guard("chat.clear", &err)
	return s.orch.Clear(ctx, p, c)
}

// Delete removes the chat.
func (s *ChatService) Delete(ctx context.Context, p id.Project, c id.Chat) (err error) {
	defer s.guard("chat.delete", &err)
	return s.orch.Delete(ctx, p, c)
}

// Rename sets the chat's title; a title the user sets is never generated
// again. It returns the title as stored.
func (s *ChatService) Rename(ctx context.Context, p id.Project, c id.Chat, title string) (t string, err error) {
	defer s.guard("chat.rename", &err)
	err = s.write(ctx, p, c, func(proj *project.Project) (seq uint64, err error) {
		t, seq, err = proj.Chats.SetTitle(ctx, c, title, true)
		return seq, err
	})
	return t, err
}

// SetRole sets the chat's role; the model sees it from the next cut (8.6).
func (s *ChatService) SetRole(ctx context.Context, p id.Project, c id.Chat, role string) (err error) {
	defer s.guard("chat.set_role", &err)
	return s.write(ctx, p, c, func(proj *project.Project) (uint64, error) {
		return proj.Chats.SetRole(ctx, c, role, id.SourceUser)
	})
}

// SetModel sets the chat's model from the model picker (3.9).
func (s *ChatService) SetModel(ctx context.Context, p id.Project, c id.Chat, model string) (err error) {
	defer s.guard("chat.set_model", &err)
	return s.write(ctx, p, c, func(proj *project.Project) (uint64, error) {
		return proj.Chats.SetModel(ctx, c, model)
	})
}

// Archive archives the chat, or brings it back.
func (s *ChatService) Archive(ctx context.Context, p id.Project, c id.Chat, archived bool) (err error) {
	defer s.guard("chat.archive", &err)
	return s.write(ctx, p, c, func(proj *project.Project) (uint64, error) {
		return proj.Chats.Archive(ctx, c, archived)
	})
}

// write runs a change to the chat outside a turn and tells the
// Orchestrator its new sequence number, so later events carry it.
func (s *ChatService) write(ctx context.Context, p id.Project, c id.Chat, fn func(*project.Project) (uint64, error)) error {
	return open(ctx, s.projects, p, func(proj *project.Project) error {
		seq, err := fn(proj)
		if err == nil {
			s.orch.Wrote(p, c, seq)
		}
		return err
	})
}

// SetLevel sets the project's approval level from the chat's chip (8.8):
// strict, standard or auto.
func (s *ChatService) SetLevel(ctx context.Context, p id.Project, c id.Chat, level string) (err error) {
	defer s.guard("chat.set_level", &err)
	l, err := project.ParseLevel(level)
	if err != nil {
		return &levelError{level: level}
	}
	return s.orch.SetLevel(ctx, p, c, l)
}

// SearchRequest is a history search (SPEC 2.3).
type SearchRequest struct {
	Query     string  `json:"query"`
	Chat      id.Chat `json:"chat,omitempty"` // "" searches every chat of the project
	Substring bool    `json:"substring"`      // *Search inside words*
	Limit     int     `json:"limit"`          // 0 means 20; at most 100
}

// Search searches the project's chat history.
func (s *ChatService) Search(ctx context.Context, p id.Project, q SearchRequest) (r store.SearchResult, err error) {
	defer s.guard("chat.search", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		r, err = proj.Chats.Search(ctx, store.SearchReq{Query: q.Query, Chat: q.Chat, Substring: q.Substring, Limit: q.Limit})
		return err
	})
	if r.Hits == nil {
		r.Hits = []store.Hit{}
	}
	return r, err
}
