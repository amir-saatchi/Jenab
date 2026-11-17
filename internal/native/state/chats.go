package state

import (
	"context"
	"slices"
	"time"

	"github.com/amir-saatchi/jenab/internal/app"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

// ChatsAPI is what Chats reads; *app.ChatService has it.
type ChatsAPI interface {
	List(ctx context.Context, p id.Project) ([]app.ChatItem, error)
	Waiting(ctx context.Context) ([]app.WaitingItem, error)
	Create(ctx context.Context, p id.Project, title, role string) (chat.Chat, error)
}

// ListWithStatus applies a chat:status event to a project's chat list. Events
// for chats not in the list are dropped: the next List has them. It
// returns items itself when nothing changed.
func ListWithStatus(items []app.ChatItem, s chat.Status) []app.ChatItem {
	i := slices.IndexFunc(items, func(it app.ChatItem) bool { return it.Chat.ID == s.Chat })
	if i < 0 || items[i].State == s.State {
		return items
	}
	next := slices.Clone(items)
	next[i].State = s.State
	return next
}

// WithWaiting applies a chat:status event to the Waiting list of every
// project: a wait is added or replaced, and removed once the chat moves
// on. It returns ws itself when nothing changed.
func WithWaiting(ws []app.WaitingItem, s chat.Status, title string) []app.WaitingItem {
	rest := slices.DeleteFunc(slices.Clone(ws), func(w app.WaitingItem) bool { return w.Chat == s.Chat && w.Project == s.Project })
	if s.State != chat.StateWaiting || s.Waiting == nil {
		if len(rest) == len(ws) {
			return ws
		}
		return rest
	}
	return append(rest, app.WaitingItem{Project: s.Project, Chat: s.Chat, Title: title, Waiting: *s.Waiting})
}

// Visible is the chat list as shown: Mother first, then the others by
// their last activity, newest first; archived chats are left out.
func Visible(items []app.ChatItem) []app.ChatItem {
	when := func(it app.ChatItem) time.Time {
		if it.LastActivity != nil {
			return *it.LastActivity
		}
		return it.Chat.CreatedAt
	}
	out := slices.DeleteFunc(slices.Clone(items), func(it app.ChatItem) bool { return it.Chat.Archived })
	slices.SortStableFunc(out, func(a, b app.ChatItem) int {
		ma, mb := a.Chat.Kind == chat.KindMother, b.Chat.Kind == chat.KindMother
		if ma != mb {
			if ma {
				return -1
			}
			return 1
		}
		return when(b).Compare(when(a))
	})
	return out
}

// Chats is the chat lists of the projects opened, and the chats of every
// project that wait for the user (state/chats.ts).
type Chats struct {
	ByProject map[id.Project][]app.ChatItem
	Waiting   []app.WaitingItem

	// OnList, if set, gets each project's chats as a new list has them,
	// after the threads of the chats gone are dropped; the stores prune
	// what else they keep per chat with it.
	OnList func(p id.Project, chats []id.Chat)

	api     ChatsAPI
	post    Post
	threads *Threads
	// loads and waiting count the reads started, so an answer older than
	// the last one started is dropped, as Pages.LoadUsage does.
	loads   map[id.Project]int
	waiting int
}

// NewChats makes the store; a chat a new list no longer has is dropped
// from threads.
func NewChats(api ChatsAPI, post Post, threads *Threads) *Chats {
	return &Chats{ByProject: map[id.Project][]app.ChatItem{}, api: api, post: post, threads: threads, loads: map[id.Project]int{}}
}

// Load reads a project's chat list. When two reads overlap, the one
// started last is kept, whichever answers last; the channel of the other
// gets its error or nil.
func (s *Chats) Load(p id.Project) <-chan error {
	done := make(chan error, 1)
	s.loads[p]++
	seq := s.loads[p]
	go func() {
		items, err := s.api.List(background, p)
		s.post(func() {
			if err == nil && seq == s.loads[p] {
				s.ByProject[p] = items
				ids := make([]id.Chat, len(items))
				for i, it := range items {
					ids[i] = it.Chat.ID
				}
				// A chat no longer listed was deleted: its thread goes too.
				if s.threads != nil {
					s.threads.Gone(p, ids)
				}
				if s.OnList != nil {
					s.OnList(p, ids)
				}
			}
			done <- err
		})
	}()
	return done
}

// LoadWaiting reads the chats that wait for the user, keeping the read
// started last as Load does.
func (s *Chats) LoadWaiting() <-chan error {
	done := make(chan error, 1)
	s.waiting++
	seq := s.waiting
	go func() {
		ws, err := s.api.Waiting(background)
		s.post(func() {
			if err == nil && seq == s.waiting {
				s.Waiting = ws
			}
			done <- err
		})
	}()
	return done
}

// Create makes a chat in the project and reads the list again; then gets
// the new chat once the list has it.
func (s *Chats) Create(p id.Project, then func(id.Chat, error)) {
	go func() {
		c, err := s.api.Create(background, p, "", "")
		s.post(func() {
			if err != nil {
				then("", err)
				return
			}
			loaded := s.Load(p)
			go func() {
				err := <-loaded
				s.post(func() { then(c.ID, err) })
			}()
		})
	}()
}

// Status applies a chat:status event to the lists.
func (s *Chats) Status(st chat.Status) {
	items := s.ByProject[st.Project]
	title := ""
	if i := slices.IndexFunc(items, func(it app.ChatItem) bool { return it.Chat.ID == st.Chat }); i >= 0 {
		title = items[i].Chat.Title
	} else if i := slices.IndexFunc(s.Waiting, func(w app.WaitingItem) bool { return w.Chat == st.Chat }); i >= 0 {
		title = s.Waiting[i].Title
	}
	if items != nil {
		s.ByProject[st.Project] = ListWithStatus(items, st)
	}
	s.Waiting = WithWaiting(s.Waiting, st, title)
}
