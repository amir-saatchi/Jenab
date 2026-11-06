package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

// Rewind (TASK-018): the user takes a chat back to an earlier turn. The
// later turns leave the chat, and with them their words in the history
// index, the session notes and loaded skills they changed, and Refresh
// memory's progress over them. They are kept in rewound_messages, like
// other deletions inside the undo window (SPEC 4.4), so the rewind can be
// undone until the chat goes on or the window ends. There are no
// branches. The project data the turns changed stays; their sources
// still name the messages, so Undo turn can revert them from the
// rewind's mark (SPEC 2.6).

var (
	// ErrNothingToRewind: the chat has no turn after the one asked for.
	ErrNothingToRewind = errors.New("store: there are no later turns to rewind")
	// ErrRewindGone: the rewind was undone already, or is past the undo
	// window.
	ErrRewindGone = errors.New("store: the rewind was undone already, or is past the undo window")
	// ErrRewindMoved: the chat went on after the rewind, or was rewound
	// again, so the turns can't come back.
	ErrRewindMoved = errors.New("store: the chat went on after the rewind")
)

// The kinds of turn_state rows: the chat's session notes and its loaded
// skills as they were before the first change in a turn.
const (
	stateNotes  = "notes"
	stateSkills = "skills"
)

// notesState is the notes in a turn_state row and a rewind's state.
type notesState struct {
	Content string `json:"content"`
	Summary string `json:"summary"`
}

// rewindTables is chats.db format step 6 (TASK-018).
//   - rewinds: each rewind with what it changed besides the messages, so
//     it can be undone.
//   - rewound_messages: the messages that left, with their parts as JSON.
//     Their tokens still count in Usage.
//   - turn_state: the notes and skills before the first change in each
//     turn, keyed by the turn the chat was at (its running turn, or its
//     last one between turns). A rewind to turn N puts back the row of
//     the first turn after N.
func rewindTables(tx *sql.Tx) error {
	return execAll(
		`CREATE TABLE rewinds (
			id        INTEGER PRIMARY KEY,
			chat_id   TEXT NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
			turn      INTEGER NOT NULL,
			last_turn INTEGER NOT NULL,
			notice    TEXT NOT NULL DEFAULT '',
			state     TEXT NOT NULL,
			at        TEXT NOT NULL
		)`,
		`CREATE INDEX rewinds_chat ON rewinds (chat_id, id)`,
		`CREATE INDEX rewinds_at ON rewinds (at)`,
		`CREATE TABLE rewound_messages (
			rewind_id          INTEGER NOT NULL REFERENCES rewinds (id) ON DELETE CASCADE,
			id                 TEXT NOT NULL,
			chat_id            TEXT NOT NULL,
			turn               INTEGER NOT NULL,
			role               TEXT NOT NULL,
			model              TEXT NOT NULL,
			input_tokens       INTEGER NOT NULL,
			output_tokens      INTEGER NOT NULL,
			cache_read_tokens  INTEGER NOT NULL,
			cache_write_tokens INTEGER NOT NULL,
			created_at         TEXT NOT NULL,
			parts              TEXT NOT NULL
		)`,
		`CREATE INDEX rewound_messages_rewind ON rewound_messages (rewind_id, turn, id)`,
		`CREATE INDEX rewound_messages_id ON rewound_messages (id)`,
		`CREATE TABLE turn_state (
			chat_id TEXT NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
			turn    INTEGER NOT NULL,
			kind    TEXT NOT NULL,
			state   TEXT NOT NULL,
			PRIMARY KEY (chat_id, turn, kind)
		) WITHOUT ROWID`,
	)(tx)
}

// keepBefore records state as the chat's notes or skills before the first
// change in the turn the chat is at. A later change in the same turn
// keeps the first record. A chat with no turn yet records nothing.
func keepBefore(tx *sql.Tx, ch id.Chat, kind string, state any) error {
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT OR IGNORE INTO turn_state (chat_id, turn, kind, state)
		SELECT ?1, t, ?2, ?3 FROM (SELECT COALESCE(MAX(turn), 0) AS t FROM messages WHERE chat_id = ?1) WHERE t > 0`,
		ch, kind, string(b))
	return err
}

// turnStateRow is a row of turn_state.
type turnStateRow struct {
	Turn  int             `json:"turn"`
	Kind  string          `json:"kind"`
	State json.RawMessage `json:"state"`
}

// storedPart is a part as message_parts holds it, for rewound_messages.
type storedPart struct {
	Type    chat.PartKind `json:"type"`
	Content string        `json:"content"`
	Ref     string        `json:"ref,omitempty"`
	Preview string        `json:"preview,omitempty"`
}

// rewindState is what a rewind changed besides the messages, as it was
// before, so its undo can put it back.
type rewindState struct {
	Notes        *notesState     `json:"notes,omitempty"`  // set when the rewind changed the notes
	Skills       json.RawMessage `json:"skills,omitempty"` // set when it changed the skills
	ReviewedUpTo *id.Message     `json:"reviewed_up_to,omitempty"`
	Chunks       []ReviewChunk   `json:"chunks,omitempty"`
	TurnState    []turnStateRow  `json:"turn_state,omitempty"`
}

// RewindPreview is what a rewind to Turn would do, for its dialog.
type RewindPreview struct {
	Turn     int          `json:"turn"`      // the chat would end with this turn
	LastTurn int          `json:"last_turn"` // the last turn that would leave
	Messages []id.Message `json:"-"`         // the messages that would leave; their sources name the changes they made
	Notes    bool         `json:"notes"`     // the session notes would go back
	Unload   []string     `json:"unload"`    // skills that would be unloaded
	Load     []string     `json:"load"`      // skills that would be loaded again
}

// Rewound is what Rewind did.
type Rewound struct {
	ID       int64
	Turn     int
	LastTurn int
	Messages []id.Message // the messages that left
	Notes    bool         // the session notes went back
	Skills   []string     // the skills now, nil if they didn't change
	Notice   id.Message   // the notice written with the rewind, or ""
	Seq      uint64
}

// RewindInfo is a chat's rewind, for its mark in the chat (TASK-018).
type RewindInfo struct {
	ID       int64     `json:"id"`
	Turn     int       `json:"turn"`      // the chat ended with this turn
	LastTurn int       `json:"last_turn"` // the last turn that left
	At       time.Time `json:"at"`
	// Undoable: it is the chat's newest rewind and the chat has not gone
	// on since, so Undo rewind can bring the turns back.
	Undoable bool `json:"undoable"`
}

// lastTurnTx is the chat's last turn, 0 without messages.
func lastTurnTx(tx *sql.Tx, ch id.Chat) (int, error) {
	var t int
	err := tx.QueryRow(`SELECT COALESCE(MAX(turn), 0) FROM messages WHERE chat_id = ?`, ch).Scan(&t)
	return t, err
}

// targets reads the turn_state rows after turn, oldest first, and the
// first of each kind: the notes and skills as they were after turn.
func targets(tx *sql.Tx, ch id.Chat, turn int) ([]turnStateRow, map[string]json.RawMessage, error) {
	rows, err := queryTx(tx, `SELECT turn, kind, state FROM turn_state WHERE chat_id = ? AND turn > ? ORDER BY turn, kind`, []any{ch, turn},
		func(r *sql.Rows) (turnStateRow, error) {
			var x turnStateRow
			var s string
			err := r.Scan(&x.Turn, &x.Kind, &s)
			x.State = json.RawMessage(s)
			return x, err
		})
	if err != nil {
		return nil, nil, err
	}
	first := map[string]json.RawMessage{}
	for _, x := range rows {
		if _, ok := first[x.Kind]; !ok {
			first[x.Kind] = x.State
		}
	}
	return rows, first, nil
}

// currentNotes reads the chat's notes; revision 0 without a row.
func currentNotes(tx *sql.Tx, ch id.Chat) (notesState, int, error) {
	var n notesState
	var rev int
	err := tx.QueryRow(`SELECT content, summary, revision FROM session_notes WHERE chat_id = ?`, ch).Scan(&n.Content, &n.Summary, &rev)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return n, rev, err
}

// putNotes writes the notes with the next revision, so an edit made from
// the revision before is refused (SPEC 3.4).
func putNotes(tx *sql.Tx, ch id.Chat, n notesState, rev int) error {
	_, err := tx.Exec(`INSERT INTO session_notes (chat_id, content, summary, revision, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (chat_id) DO UPDATE SET content = excluded.content, summary = excluded.summary, revision = excluded.revision, updated_at = excluded.updated_at`,
		ch, n.Content, n.Summary, rev+1, formatTime(now()))
	return err
}

func currentSkills(tx *sql.Tx, ch id.Chat) ([]string, error) {
	var s string
	if err := tx.QueryRow(`SELECT skills FROM chats WHERE id = ?`, ch).Scan(&s); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = fmt.Errorf("store: chat %s: %w", ch, ErrNotFound)
		}
		return nil, err
	}
	var out []string
	return out, json.Unmarshal([]byte(s), &out)
}

// preview is RewindPreview inside a transaction.
func preview(tx *sql.Tx, ch id.Chat, turn int) (RewindPreview, map[string]json.RawMessage, []turnStateRow, error) {
	p := RewindPreview{Turn: turn, Unload: []string{}, Load: []string{}}
	skills, err := currentSkills(tx, ch)
	if err != nil {
		return p, nil, nil, err
	}
	last, err := lastTurnTx(tx, ch)
	if err != nil {
		return p, nil, nil, err
	}
	if turn < 0 || turn >= last {
		return p, nil, nil, ErrNothingToRewind
	}
	p.LastTurn = last
	p.Messages, err = queryTx(tx, `SELECT id FROM messages WHERE chat_id = ? AND turn > ? ORDER BY turn, id`, []any{ch, turn},
		func(r *sql.Rows) (id.Message, error) {
			var m id.Message
			return m, r.Scan(&m)
		})
	if err != nil {
		return p, nil, nil, err
	}
	rows, first, err := targets(tx, ch, turn)
	if err != nil {
		return p, nil, nil, err
	}
	if b, ok := first[stateNotes]; ok {
		var want notesState
		if err := json.Unmarshal(b, &want); err != nil {
			return p, nil, nil, err
		}
		cur, _, err := currentNotes(tx, ch)
		if err != nil {
			return p, nil, nil, err
		}
		p.Notes = cur != want
	}
	if b, ok := first[stateSkills]; ok {
		var want []string
		if err := json.Unmarshal(b, &want); err != nil {
			return p, nil, nil, err
		}
		for _, s := range skills {
			if !slices.Contains(want, s) {
				p.Unload = append(p.Unload, s)
			}
		}
		for _, s := range want {
			if !slices.Contains(skills, s) {
				p.Load = append(p.Load, s)
			}
		}
	}
	return p, first, rows, nil
}

// RewindPreview says what a rewind of the chat to turn would do. It is
// ErrNothingToRewind when the chat has no turn after it.
func (c *ChatsDB) RewindPreview(ctx context.Context, ch id.Chat, turn int) (RewindPreview, error) {
	return readTx(ctx, c.DB, func(tx *sql.Tx) (RewindPreview, error) {
		p, _, _, err := preview(tx, ch, turn)
		return p, err
	})
}

// Rewind takes the chat back to the end of turn: the messages of later
// turns move out of the chat and the history index, and the session
// notes, loaded skills and Refresh memory's progress go back to how they
// were after turn. notice, if not nil, is written in turn after the
// rewind, in the same transaction. It is ErrNothingToRewind when the
// chat has no turn after turn.
func (c *ChatsDB) Rewind(ctx context.Context, ch id.Chat, turn int, notice *chat.Message) (Rewound, error) {
	return Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (Rewound, error) {
		p, first, rows, err := preview(tx, ch, turn)
		if err != nil {
			return Rewound{}, err
		}
		out := Rewound{Turn: turn, LastTurn: p.LastTurn, Messages: p.Messages}
		var st rewindState
		at := now()
		if err := tx.QueryRow(`INSERT INTO rewinds (chat_id, turn, last_turn, state, at) VALUES (?, ?, ?, '{}', ?) RETURNING id`,
			ch, turn, p.LastTurn, formatTime(at)).Scan(&out.ID); err != nil {
			return out, err
		}
		if err := moveOut(tx, out.ID, ch, turn); err != nil {
			return out, err
		}

		// The notes and skills as they were after turn.
		if p.Notes {
			var want notesState
			if err := json.Unmarshal(first[stateNotes], &want); err != nil {
				return out, err
			}
			cur, rev, err := currentNotes(tx, ch)
			if err != nil {
				return out, err
			}
			st.Notes = &cur
			if err := putNotes(tx, ch, want, rev); err != nil {
				return out, err
			}
			out.Notes = true
		}
		if len(p.Unload) > 0 || len(p.Load) > 0 {
			var cur string
			if err := tx.QueryRow(`SELECT skills FROM chats WHERE id = ?`, ch).Scan(&cur); err != nil {
				return out, err
			}
			st.Skills = json.RawMessage(cur)
			if _, err := tx.Exec(`UPDATE chats SET skills = ? WHERE id = ?`, string(first[stateSkills]), ch); err != nil {
				return out, err
			}
			if err := json.Unmarshal(first[stateSkills], &out.Skills); err != nil {
				return out, err
			}
			if out.Skills == nil {
				out.Skills = []string{}
			}
		}
		st.TurnState = rows
		if _, err := tx.Exec(`DELETE FROM turn_state WHERE chat_id = ? AND turn > ?`, ch, turn); err != nil {
			return out, err
		}

		// Refresh memory's progress: a review of turns that left goes back
		// to the last message kept, and its saved chunks over them leave.
		var upTo, kept id.Message
		if err := tx.QueryRow(`SELECT memory_reviewed_up_to, COALESCE((SELECT MAX(id) FROM messages WHERE chat_id = ?1), '') FROM chats WHERE id = ?1`, ch).
			Scan(&upTo, &kept); err != nil {
			return out, err
		}
		if upTo != "" && slices.Contains(p.Messages, upTo) {
			st.ReviewedUpTo = &upTo
			if _, err := tx.Exec(`UPDATE chats SET memory_reviewed_up_to = ? WHERE id = ?`, kept, ch); err != nil {
				return out, err
			}
		}
		st.Chunks, err = queryTx(tx, `SELECT first_id, last_id, result, created_at FROM review_chunks WHERE chat_id = ? AND last_id > ? ORDER BY first_id`,
			[]any{ch, kept}, func(r *sql.Rows) (ReviewChunk, error) {
				var k ReviewChunk
				var at string
				if err := r.Scan(&k.First, &k.Last, &k.Result, &at); err != nil {
					return k, err
				}
				var err error
				k.CreatedAt, err = parseTime(at)
				return k, err
			})
		if err != nil {
			return out, err
		}
		if _, err := tx.Exec(`DELETE FROM review_chunks WHERE chat_id = ? AND last_id > ?`, ch, kept); err != nil {
			return out, err
		}

		if notice != nil {
			if notice.Turn != turn || notice.Chat != ch {
				return out, fmt.Errorf("store: a rewind's notice goes in turn %d of chat %s", turn, ch)
			}
			if err := insertMessage(tx, notice); err != nil {
				return out, err
			}
			out.Notice = notice.ID
		}
		b, err := json.Marshal(st)
		if err != nil {
			return out, err
		}
		if _, err := tx.Exec(`UPDATE rewinds SET state = ?, notice = ? WHERE id = ?`, string(b), out.Notice, out.ID); err != nil {
			return out, err
		}
		out.Seq, err = bump(tx, ch)
		return out, err
	})
}

// moveOut moves the chat's messages after turn into rewound_messages.
// Deleting their parts takes their words out of the history index.
func moveOut(tx *sql.Tx, rid int64, ch id.Chat, turn int) error {
	type part struct {
		m id.Message
		p storedPart
	}
	parts, err := queryTx(tx, `SELECT p.message_id, p.type, p.content, p.ref, p.preview FROM message_parts p JOIN messages m ON m.id = p.message_id
		WHERE m.chat_id = ? AND m.turn > ? ORDER BY p.message_id, p.seq`, []any{ch, turn},
		func(r *sql.Rows) (part, error) {
			var x part
			return x, r.Scan(&x.m, &x.p.Type, &x.p.Content, &x.p.Ref, &x.p.Preview)
		})
	if err != nil {
		return err
	}
	byMsg := map[id.Message][]storedPart{}
	for _, x := range parts {
		byMsg[x.m] = append(byMsg[x.m], x.p)
	}
	ids, err := queryTx(tx, `SELECT id FROM messages WHERE chat_id = ? AND turn > ?`, []any{ch, turn}, func(r *sql.Rows) (id.Message, error) {
		var m id.Message
		return m, r.Scan(&m)
	})
	if err != nil {
		return err
	}
	for _, m := range ids {
		ps := byMsg[m]
		if ps == nil {
			ps = []storedPart{}
		}
		b, err := json.Marshal(ps)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO rewound_messages (rewind_id, id, chat_id, turn, role, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, created_at, parts)
			SELECT ?, id, chat_id, turn, role, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, created_at, ? FROM messages WHERE id = ?`,
			rid, string(b), m); err != nil {
			return err
		}
	}
	for _, q := range []string{
		`DELETE FROM message_parts WHERE message_id IN (SELECT id FROM messages WHERE chat_id = ? AND turn > ?)`,
		`DELETE FROM messages WHERE chat_id = ? AND turn > ?`,
	} {
		if _, err := tx.Exec(q, ch, turn); err != nil {
			return err
		}
	}
	return nil
}

// UndoRewind brings back the turns the rewind took out of the chat, with
// their place in the history index, and the notes, skills and Refresh
// memory's progress as they were before it. The rewind's notice leaves.
// Only the chat's newest rewind can be undone, and only while the chat
// has not gone on: ErrRewindMoved. A rewind undone already or past the
// undo window is ErrRewindGone.
func (c *ChatsDB) UndoRewind(ctx context.Context, ch id.Chat, rid int64) (Rewound, error) {
	return Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (Rewound, error) {
		out := Rewound{ID: rid}
		var state string
		err := tx.QueryRow(`SELECT turn, last_turn, notice, state FROM rewinds WHERE id = ? AND chat_id = ?`, rid, ch).
			Scan(&out.Turn, &out.LastTurn, &out.Notice, &state)
		if errors.Is(err, sql.ErrNoRows) {
			return out, ErrRewindGone
		}
		if err != nil {
			return out, err
		}
		var newer bool
		if err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM rewinds WHERE chat_id = ? AND id > ?)`, ch, rid).Scan(&newer); err != nil {
			return out, err
		}
		last, err := lastTurnTx(tx, ch)
		if err != nil {
			return out, err
		}
		if newer || last != out.Turn {
			return out, ErrRewindMoved
		}
		var st rewindState
		if err := json.Unmarshal([]byte(state), &st); err != nil {
			return out, fmt.Errorf("store: rewind %d: %w", rid, err)
		}
		if out.Notice != "" {
			for _, q := range []string{`DELETE FROM message_parts WHERE message_id = ?`, `DELETE FROM messages WHERE id = ?`} {
				if _, err := tx.Exec(q, out.Notice); err != nil {
					return out, err
				}
			}
		}
		ms, err := rewoundMessages(tx, rid)
		if err != nil {
			return out, err
		}
		for i := range ms {
			if err := insertMessage(tx, &ms[i]); err != nil {
				return out, err
			}
			out.Messages = append(out.Messages, ms[i].ID)
		}
		if st.Notes != nil {
			_, rev, err := currentNotes(tx, ch)
			if err != nil {
				return out, err
			}
			if err := putNotes(tx, ch, *st.Notes, rev); err != nil {
				return out, err
			}
			out.Notes = true
		}
		if st.Skills != nil {
			if _, err := tx.Exec(`UPDATE chats SET skills = ? WHERE id = ?`, string(st.Skills), ch); err != nil {
				return out, err
			}
			if err := json.Unmarshal(st.Skills, &out.Skills); err != nil {
				return out, err
			}
		}
		if st.ReviewedUpTo != nil {
			if _, err := tx.Exec(`UPDATE chats SET memory_reviewed_up_to = ? WHERE id = ?`, *st.ReviewedUpTo, ch); err != nil {
				return out, err
			}
		}
		for _, k := range st.Chunks {
			if _, err := tx.Exec(`INSERT OR REPLACE INTO review_chunks (chat_id, first_id, last_id, result, created_at) VALUES (?, ?, ?, ?, ?)`,
				ch, k.First, k.Last, k.Result, formatTime(k.CreatedAt)); err != nil {
				return out, err
			}
		}
		for _, x := range st.TurnState {
			if _, err := tx.Exec(`INSERT OR REPLACE INTO turn_state (chat_id, turn, kind, state) VALUES (?, ?, ?, ?)`,
				ch, x.Turn, x.Kind, string(x.State)); err != nil {
				return out, err
			}
		}
		if _, err := tx.Exec(`DELETE FROM rewinds WHERE id = ?`, rid); err != nil {
			return out, err
		}
		out.Seq, err = bump(tx, ch)
		return out, err
	})
}

// rewoundMessages reads a rewind's messages with their parts, in turn
// order.
func rewoundMessages(tx *sql.Tx, rid int64) ([]chat.Message, error) {
	return queryTx(tx, `SELECT id, chat_id, turn, role, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, created_at, parts
		FROM rewound_messages WHERE rewind_id = ? ORDER BY turn, id`, []any{rid}, func(r *sql.Rows) (chat.Message, error) {
		var m chat.Message
		var created, parts string
		u := &m.Usage
		if err := r.Scan(&m.ID, &m.Chat, &m.Turn, &m.Role, &m.Model, &u.Input, &u.Output, &u.CacheRead, &u.CacheWrite, &created, &parts); err != nil {
			return m, err
		}
		var err error
		if m.CreatedAt, err = parseTime(created); err != nil {
			return m, err
		}
		var ps []storedPart
		if err := json.Unmarshal([]byte(parts), &ps); err != nil {
			return m, fmt.Errorf("store: rewound message %s: %w", m.ID, err)
		}
		m.Parts = make([]chat.Part, 0, len(ps))
		for _, sp := range ps {
			p, err := decodePart(sp.Type, sp.Content)
			if err != nil {
				return m, fmt.Errorf("store: rewound message %s: %w", m.ID, err)
			}
			m.Parts = append(m.Parts, p)
		}
		return m, nil
	})
}

// Rewinds lists the chat's rewinds, the oldest first.
func (c *ChatsDB) Rewinds(ctx context.Context, ch id.Chat) ([]RewindInfo, error) {
	return readTx(ctx, c.DB, func(tx *sql.Tx) ([]RewindInfo, error) {
		last, err := lastTurnTx(tx, ch)
		if err != nil {
			return nil, err
		}
		rs, err := queryTx(tx, `SELECT id, turn, last_turn, at FROM rewinds WHERE chat_id = ? ORDER BY id`, []any{ch},
			func(r *sql.Rows) (RewindInfo, error) {
				var x RewindInfo
				var at string
				if err := r.Scan(&x.ID, &x.Turn, &x.LastTurn, &at); err != nil {
					return x, err
				}
				var err error
				x.At, err = parseTime(at)
				return x, err
			})
		if err != nil {
			return nil, err
		}
		if n := len(rs); n > 0 && rs[n-1].Turn == last {
			rs[n-1].Undoable = true
		}
		return rs, nil
	})
}

// RewindMessages returns the IDs of the messages a rewind of the chat took
// out, oldest first. The changes they made have the sources message:<id>
// of them (SPEC 2.6). A rewind undone already or purged is ErrRewindGone.
func (c *ChatsDB) RewindMessages(ctx context.Context, ch id.Chat, rid int64) ([]id.Message, error) {
	return readTx(ctx, c.DB, func(tx *sql.Tx) ([]id.Message, error) {
		var ok bool
		if err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM rewinds WHERE id = ? AND chat_id = ?)`, rid, ch).Scan(&ok); err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrRewindGone
		}
		return queryTx(tx, `SELECT id FROM rewound_messages WHERE rewind_id = ? ORDER BY turn, id`, []any{rid}, func(r *sql.Rows) (id.Message, error) {
			var m id.Message
			return m, r.Scan(&m)
		})
	})
}

// PurgeRewinds drops the rewinds made before before, with their messages:
// they are past the undo window (SPEC 2.6, 4.4). It returns how many it
// dropped.
func (c *ChatsDB) PurgeRewinds(ctx context.Context, before time.Time) (int64, error) {
	return Do(ctx, c.DB, limit.Background, func(tx *sql.Tx) (int64, error) {
		res, err := tx.Exec(`DELETE FROM rewinds WHERE at < ?`, formatTime(before))
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	})
}
