package app

import (
	"context"
	"time"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/chatexport"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/provider"
	"github.com/amir-saatchi/jenab/internal/secret"
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
	secrets  *secret.Store // for Export's redaction; nil redacts nothing
	settings *Settings     // the providers whose keys Export hides, and the model aliases; may be nil
	registry *store.Registry
}

// ChatItem is a row of the chat list.
type ChatItem struct {
	Chat         chat.Chat  `json:"chat"`
	State        chat.State `json:"state"`                   // as chat:status sends it
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
			it := ChatItem{Chat: c, State: s.orch.State(p, c.ID)}
			if t, ok := last[c.ID]; ok {
				it.LastActivity = &t
			}
			items = append(items, it)
		}
		return nil
	})
	return items, err
}

// WaitingItem is a chat that waits for the user, for *Waiting* in the
// rail (SPEC 5.12).
type WaitingItem struct {
	Project id.Project   `json:"project"`
	Chat    id.Chat      `json:"chat"`
	Title   string       `json:"title"`
	Waiting chat.Waiting `json:"waiting"`
}

// Waiting lists the chats of every project that wait for the user. Later
// changes come as chat:status events.
func (s *ChatService) Waiting(ctx context.Context) (items []WaitingItem, err error) {
	defer s.guard("chat.waiting", &err)
	items = []WaitingItem{}
	for _, w := range s.orch.Waits() {
		it := WaitingItem{Project: w.Project, Chat: w.Chat, Waiting: w.Waiting}
		err := open(ctx, s.projects, w.Project, func(proj *project.Project) error {
			c, err := proj.Chats.Chat(ctx, w.Chat)
			it.Title = c.Title
			return err
		})
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// One project that can't be read doesn't hide the others.
			s.log.Warn("app: a waiting chat not read", "project", w.Project, "chat", w.Chat, "err", s.redact(err.Error()))
			continue
		}
		items = append(items, it)
	}
	return items, nil
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
	// Rewinds are the chat's rewinds still in the undo window, for their
	// marks (TASK-018).
	Rewinds []store.RewindInfo `json:"rewinds"`
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
		if err != nil {
			return err
		}
		snap.Rewinds, err = proj.Chats.Rewinds(ctx, c)
		return err
	})
	if snap.Messages == nil {
		snap.Messages = []chat.Message{}
	}
	if snap.Rewinds == nil {
		snap.Rewinds = []store.RewindInfo{}
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

// Notes returns the chat's session notes, for the chat header (SPEC 3.4).
// They are read-only here: the agent and Refresh memory write them.
func (s *ChatService) Notes(ctx context.Context, p id.Project, c id.Chat) (n chat.SessionNote, err error) {
	defer s.guard("chat.notes", &err)
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		n, err = proj.Chats.Notes(ctx, c)
		return err
	})
	return n, err
}

// Send adds the user's message; the turn's progress comes as events.
// quote, if not nil, is the passage of an earlier message of the chat it
// replies to: its text and message, and cut when the frontend cut it; the
// turn and role are read from the source (TASK-019). Pictures come with
// the composer's attachments (P1-15).
func (s *ChatService) Send(ctx context.Context, p id.Project, c id.Chat, text string, quote *chat.Quote) (m id.Message, err error) {
	defer s.guard("chat.send", &err)
	return s.orch.Send(ctx, p, c, agent.UserMessage{Text: text, Quote: quote})
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

// UndoState says what Undo turn and Redo turn would revert now for the
// chat's turn, for its footer (SPEC 2.6).
func (s *ChatService) UndoState(ctx context.Context, p id.Project, c id.Chat, turn int) (st agent.TurnUndoState, err error) {
	defer s.guard("chat.undo_state", &err)
	return s.orch.UndoState(ctx, p, c, turn)
}

// UndoTurn reverts what the chat's turn changed, or puts it back. With
// conflicts it changes nothing and lists them for the dialog; the user
// skips them, with r.Skip, or cancels.
func (s *ChatService) UndoTurn(ctx context.Context, p id.Project, c id.Chat, r agent.UndoRequest) (res agent.TurnUndo, err error) {
	defer s.guard("chat.undo_turn", &err)
	return s.orch.UndoTurn(ctx, p, c, r)
}

// RewindState says what a rewind of the chat to the end of turn would do,
// for its dialog: the turns that leave, the project data they changed
// that stays, and the notes and skills that go back (TASK-018).
func (s *ChatService) RewindState(ctx context.Context, p id.Project, c id.Chat, turn int) (st agent.RewindState, err error) {
	defer s.guard("chat.rewind_state", &err)
	return s.orch.RewindState(ctx, p, c, turn)
}

// Rewind takes the chat back to the end of turn, from *Rewind to here*
// or *Edit and resend* (TASK-018). The later turns leave the chat and can
// come back with UndoRewind until the chat goes on.
func (s *ChatService) Rewind(ctx context.Context, p id.Project, c id.Chat, turn int) (r agent.RewindResult, err error) {
	defer s.guard("chat.rewind", &err)
	return s.orch.Rewind(ctx, p, c, turn)
}

// UndoRewind brings back the turns the chat's newest rewind took out.
func (s *ChatService) UndoRewind(ctx context.Context, p id.Project, c id.Chat, rewind int64) (err error) {
	defer s.guard("chat.undo_rewind", &err)
	return s.orch.UndoRewind(ctx, p, c, rewind)
}

// RewindUndoState says what Undo turn and Redo turn would revert now for
// the changes of the turns a rewind took out, for its mark.
func (s *ChatService) RewindUndoState(ctx context.Context, p id.Project, c id.Chat, rewind int64) (st agent.TurnUndoState, err error) {
	defer s.guard("chat.rewind_undo_state", &err)
	return s.orch.RewindUndoState(ctx, p, c, rewind)
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

// ChatExportRequest is the place the user chose in the save dialog, and
// the format.
type ChatExportRequest struct {
	Target string            `json:"target"`
	Format chatexport.Format `json:"format"`
}

// Export saves the chat as a Markdown or JSON file (SPEC 2.3). It reads
// only what is stored; a part still streaming is left out. Every
// provider's key and field, and every secret the app has read, is
// replaced with [secret:NAME] (6.7).
func (s *ChatService) Export(ctx context.Context, p id.Project, c id.Chat, r ChatExportRequest) (res chatexport.Result, err error) {
	defer s.guard("chat.export", &err)
	redact := s.exportRedactor()
	err = open(ctx, s.projects, p, func(proj *project.Project) error {
		ch, err := proj.Chats.Chat(ctx, c)
		if err != nil {
			return err
		}
		notes, err := proj.Chats.Notes(ctx, c)
		if err != nil {
			return err
		}
		last, err := proj.Chats.LastTurn(ctx, c)
		if err != nil {
			return err
		}
		res, err = chatexport.WriteFile(ctx, r.Target, r.Format, chatexport.Source{
			Project: proj.ID, ProjectName: proj.Name, Chat: ch, Model: s.modelName(ch.Model), Notes: notes, LastTurn: last,
			Turns: func(ctx context.Context, from, to int) ([]chat.Message, error) {
				ms, _, err := proj.Chats.Messages(ctx, c, from, to)
				return ms, err
			},
			Redact: redact,
		})
		return err
	})
	return res, err
}

// exportRedactor reads every connected provider's key and fields once, for
// one export.
func (s *ChatService) exportRedactor() func(string) string {
	if s.secrets == nil {
		return nil
	}
	var names []string
	if s.settings != nil {
		for name, ps := range s.settings.Get().LLM.Providers {
			names = append(names, provider.KeyName(name))
			base := ps.BaseURL
			if base == "" {
				base = provider.DefaultBaseURL(provider.Kind(ps.Kind))
			}
			for _, f := range provider.Placeholders(base) {
				names = append(names, provider.FieldName(name, f))
			}
		}
	}
	return s.secrets.Redactor(names...)
}

// modelName is the chat's model for an export's header: an alias with the
// model it stands for now, e.g. "default (gemini/gemini-2.5-flash)".
func (s *ChatService) modelName(model string) string {
	if s.settings == nil {
		return model
	}
	if ref := s.settings.Get().LLM.Models[model]; ref != "" && ref != model {
		return model + " (" + ref + ")"
	}
	return model
}
