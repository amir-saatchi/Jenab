// Package chat has the chat types every layer shares: chats, messages,
// their parts, session notes and the chat events (CODE-OUTLINE `chat`, Q5,
// Q7). It holds types only, with no storage or provider code, and imports
// only id.
//
// JSON field names are snake_case, as in SPEC 2.3. Parts are stored as JSON
// in chats.db and sent to the frontend, so a renamed field is a format change.
package chat

import (
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
)

// Kind is the kind of chat (SPEC 8.6).
type Kind string

const (
	KindMother Kind = "mother" // one per project; can be cleared, not archived or deleted
	KindChat   Kind = "chat"
)

// Chat is one row of chats.db's chats table (SPEC 2.3).
type Chat struct {
	ID          id.Chat   `json:"id"`
	Kind        Kind      `json:"kind"`
	Title       string    `json:"title"`
	TitleFixed  bool      `json:"title_fixed"` // set by the user, so it is never generated again
	Role        string    `json:"role"`        // at most 500 tokens (8.6)
	Skills      []string  `json:"skills"`      // 8.9
	Model       string    `json:"model"`       // 3.9; new chats use "default"
	DefaultPage string    `json:"default_page"`
	CreatedBy   id.Source `json:"created_by"` // "app" (the Mother chat), "user", or "message:<id>" of Mother's create_chat call
	CreatedAt   time.Time `json:"created_at"`
	Archived    bool      `json:"archived"`
}

// Role is who wrote a message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Usage is the token count of one model response (SPEC 3.8). Cache reads and
// writes are kept apart, since they are priced apart.
type Usage struct {
	Input      int `json:"input"` // not read from the cache
	Output     int `json:"output"`
	CacheRead  int `json:"cache_read"`
	CacheWrite int `json:"cache_write"`
}

// Message is one message with its parts (SPEC 2.3). A turn is one user
// message plus everything the agent does until it replies.
type Message struct {
	ID        id.Message `json:"id"`
	Chat      id.Chat    `json:"chat"`
	Turn      int        `json:"turn"`
	Role      Role       `json:"role"`
	Model     string     `json:"model,omitempty"` // assistant messages only
	Usage     Usage      `json:"usage"`
	CreatedAt time.Time  `json:"created_at"`
	Parts     []Part     `json:"parts"`
}

// Validate checks every part.
func (m Message) Validate() error {
	for i, p := range m.Parts {
		if err := p.Validate(); err != nil {
			return &PartError{Index: i, Err: err}
		}
	}
	return nil
}

// SessionNote is a chat's session notes (SPEC 3.4).
type SessionNote struct {
	Chat      id.Chat   `json:"chat"`
	Content   string    `json:"content"`
	Revision  int       `json:"revision"`
	UpdatedAt time.Time `json:"updated_at"`
}
