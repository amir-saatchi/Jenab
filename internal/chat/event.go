package chat

import "github.com/amir-saatchi/jenab/internal/id"

// Event payloads (Q32): chat:delta, chat:part and chat:status. Each carries
// its IDs and the chat's sequence number, which goes up with every change.
// The frontend drops events older than its snapshot and asks for a new
// snapshot when it sees a gap.

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
}

// PartDone is a finished part, as it was written to chats.db.
type PartDone struct {
	Project id.Project `json:"project"`
	Chat    id.Chat    `json:"chat"`
	Seq     uint64     `json:"seq"`
	Message id.Message `json:"message"`
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
}

// Waiting points at the card or form a turn waits for, for the bar above the
// composer and the chat list's badge (8.8).
type Waiting struct {
	Message id.Message `json:"message"`
	Index   int        `json:"index"` // the part's index in the message
	Kind    PartKind   `json:"kind"`  // approval or question
	Text    string     `json:"text"`  // what is asked, in one line
}
