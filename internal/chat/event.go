package chat

import (
	"time"

	"github.com/amir-saatchi/jenab/internal/id"
)

// Event payloads (Q32): chat:delta, chat:part and chat:status. Each carries
// its IDs and the chat's sequence number, which goes up with every change.
// The frontend drops events older than its snapshot and asks for a new
// snapshot when it sees a gap.
//
// An answer is written once it is complete, so while it streams the
// frontend builds a temporary message from its deltas. The first PartDone
// for that message replaces the temporary copy whole: after Stop the
// stored parts can differ from the streamed ones. Deltas for a message
// that is neither stored nor Status.Streaming are dropped; a failed try
// clears Streaming, and the next try streams a new message.
//
// An approval card or question form is written when the turn starts to
// wait for it, and again, at the same index, once it is answered or
// closed: a PartDone for an index the frontend has replaces that part
// (SPEC 8.8). A card that is neither answered nor stopped nor the chat's
// Status.Waiting was left by a crash, and is closed.

// Delta is streamed text for a part still in progress. At most one per 16 ms
// per chat (Q17).
type Delta struct {
	Project id.Project `json:"project"`
	Chat    id.Chat    `json:"chat"`
	Seq     uint64     `json:"seq"`
	Message id.Message `json:"message"`
	Part    int        `json:"part"` // the part's index in the message
	Kind    PartKind   `json:"kind"` // text or thinking
	Text    string     `json:"text"` // new text since the last delta
	// Offset is where Text starts in the part's text, in UTF-16 units as
	// JavaScript counts, so a chat opened mid-answer can skip the deltas
	// Live.Text already holds.
	Offset int `json:"offset"`
}

// PartDone is a finished part, as it was written to chats.db.
type PartDone struct {
	Project id.Project `json:"project"`
	Chat    id.Chat    `json:"chat"`
	Seq     uint64     `json:"seq"`
	Message id.Message `json:"message"`
	Turn    int        `json:"turn"` // the message's turn
	Index   int        `json:"index"`
	Part    Part       `json:"part"`
}

// State is what a chat is doing.
type State string

const (
	StateIdle    State = "idle"
	StateWorking State = "working" // a turn is running
	StateWaiting State = "waiting" // a turn waits for an approval or an answer (8.8)
)

// Status is a chat's state, with its running background tasks.
type Status struct {
	Project id.Project `json:"project"`
	Chat    id.Chat    `json:"chat"`
	Seq     uint64     `json:"seq"`
	State   State      `json:"state"`
	Tasks   int        `json:"tasks"`             // background tasks still running (8.3)
	Waiting *Waiting   `json:"waiting,omitempty"` // set when State is waiting
	Retry   *Retry     `json:"retry,omitempty"`   // set while a turn waits to retry a request (8.3)
	// Streaming is the answer being streamed now, not yet stored.
	Streaming id.Message `json:"streaming,omitempty"`
}

// Retry is a turn's wait before it tries a failed request again, so the
// chat can show "gemini is rate limited, retrying in 42 s" with *Retry now*
// and *Cancel* (8.3). The failed try's streamed text is dropped (see
// Status.Streaming).
type Retry struct {
	Provider string    `json:"provider"` // the connection's name
	Kind     string    `json:"kind"`     // rate_limited, overloaded or transport
	At       time.Time `json:"at"`       // when the next try starts, in UTC
}

// Waiting points at the card or form a turn waits for, for the bar above the
// composer and the chat list's badge (8.8).
type Waiting struct {
	Message id.Message `json:"message"`
	Index   int        `json:"index"` // the part's index in the message
	Kind    PartKind   `json:"kind"`  // approval or question
	Text    string     `json:"text"`  // what is asked, in one line
}
