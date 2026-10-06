package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/limit"
)

// ChatsFormat is chats.db's storage format (SPEC 2.3).
var ChatsFormat = Format{Name: "chats", Steps: []func(*sql.Tx) error{
	execAll(
		// seq goes up with every change to the chat, for snapshot plus
		// sequence (Q32).
		`CREATE TABLE chats (
			id                    TEXT PRIMARY KEY,
			kind                  TEXT NOT NULL CHECK (kind IN ('mother', 'chat')),
			title                 TEXT NOT NULL,
			title_fixed           INTEGER NOT NULL DEFAULT 0,
			role                  TEXT NOT NULL DEFAULT '',
			skills                TEXT NOT NULL DEFAULT '[]',
			model                 TEXT NOT NULL,
			default_page          TEXT NOT NULL DEFAULT '',
			created_by            TEXT NOT NULL,
			created_at            TEXT NOT NULL,
			archived              INTEGER NOT NULL DEFAULT 0,
			memory_reviewed_up_to TEXT NOT NULL DEFAULT '',
			ui_state              TEXT NOT NULL DEFAULT '{}',
			seq                   INTEGER NOT NULL DEFAULT 0
		)`,
		// A chat has no title until one is generated after its first turn.
		`CREATE UNIQUE INDEX chats_title ON chats (title COLLATE NOCASE) WHERE title <> ''`,
		`CREATE UNIQUE INDEX chats_mother ON chats (kind) WHERE kind = 'mother'`,
		`CREATE TABLE messages (
			id                 TEXT PRIMARY KEY,
			chat_id            TEXT NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
			turn               INTEGER NOT NULL,
			role               TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'tool')),
			model              TEXT NOT NULL DEFAULT '',
			input_tokens       INTEGER NOT NULL DEFAULT 0,
			output_tokens      INTEGER NOT NULL DEFAULT 0,
			cache_read_tokens  INTEGER NOT NULL DEFAULT 0,
			cache_write_tokens INTEGER NOT NULL DEFAULT 0,
			created_at         TEXT NOT NULL
		)`,
		`CREATE INDEX messages_chat ON messages (chat_id, turn, id)`,
		// A rowid table: the FTS index (P1-07) uses the text part's rowid.
		// content is the part's own field as JSON; ref and preview repeat a
		// large output's bucket key and preview, so they can be found
		// without reading the JSON.
		`CREATE TABLE message_parts (
			message_id TEXT NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
			seq        INTEGER NOT NULL,
			type       TEXT NOT NULL,
			content    TEXT NOT NULL,
			ref        TEXT NOT NULL DEFAULT '',
			preview    TEXT NOT NULL DEFAULT '',
			UNIQUE (message_id, seq)
		)`,
		`CREATE TABLE session_notes (
			chat_id    TEXT PRIMARY KEY REFERENCES chats (id) ON DELETE CASCADE,
			content    TEXT NOT NULL,
			revision   INTEGER NOT NULL,
			updated_at TEXT NOT NULL
		) WITHOUT ROWID`,
		// Refresh memory's results per message range (SPEC 3.5), used in
		// Phase 2.
		`CREATE TABLE review_chunks (
			chat_id    TEXT NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
			first_id   TEXT NOT NULL,
			last_id    TEXT NOT NULL,
			result     TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY (chat_id, first_id)
		) WITHOUT ROWID`,
		// Role changes with their source (SPEC 8.6). Undo comes with the
		// change log (Phase 2).
		`CREATE TABLE role_changes (
			id      INTEGER PRIMARY KEY,
			chat_id TEXT NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
			before  TEXT NOT NULL,
			after   TEXT NOT NULL,
			source  TEXT NOT NULL,
			at      TEXT NOT NULL
		)`,
		`CREATE INDEX role_changes_chat ON role_changes (chat_id, id)`,
	),
	ftsTables, // P1-07
	// Each chat's newest message for Mother's chat list (P1-10).
	execAll(`CREATE INDEX messages_activity ON messages (chat_id, created_at)`),
}}

// Limits from SPEC 3.4 and 8.6. Tokens are estimated at 4 bytes each, as
// for requests.
const (
	MaxRoleTokens  = 500
	MaxNotesTokens = 2500
	MaxTitle       = 100 // characters
	MotherTitle    = "Mother"
	DefaultModel   = "default" // the alias new chats use (SPEC 3.9)
)

var (
	ErrMother     = errors.New("store: the Mother chat can't be archived or deleted, or have a role")
	ErrTitleTaken = errors.New("store: another chat has this title")
	ErrBadTitle   = errors.New("store: a title needs 1 to 100 characters on one line")
	ErrTooLong    = errors.New("store: the text is over its token limit")
	ErrNotPending = errors.New("store: the card was already answered or closed")
)

func estimateTokens(s string) int { return len(s) / 4 }

// ChatsDB is a project's chats.db.
type ChatsDB struct {
	*DB
}

// OpenChats opens dir/chats.db, creating it if needed.
func OpenChats(ctx context.Context, dir string) (*ChatsDB, error) {
	db, err := Open(ctx, filepath.Join(dir, "chats.db"), Options{
		Format:    ChatsFormat,
		Snapshots: filepath.Join(dir, "snapshots"),
	})
	if err != nil {
		return nil, err
	}
	return &ChatsDB{DB: db}, nil
}

// OpenChatsReadOnly opens dir/chats.db for reading only, for a damaged
// project (SPEC 2.7).
func OpenChatsReadOnly(ctx context.Context, dir string) (*ChatsDB, error) {
	db, err := Open(ctx, filepath.Join(dir, "chats.db"), Options{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return &ChatsDB{DB: db}, nil
}

const chatCols = `id, kind, title, title_fixed, role, skills, model, default_page, created_by, created_at, archived`

func scanChat(r scanner) (chat.Chat, error) {
	var c chat.Chat
	var skills, created string
	if err := r.Scan(&c.ID, &c.Kind, &c.Title, &c.TitleFixed, &c.Role, &skills, &c.Model, &c.DefaultPage, &c.CreatedBy, &created, &c.Archived); err != nil {
		return c, err
	}
	if err := json.Unmarshal([]byte(skills), &c.Skills); err != nil {
		return c, fmt.Errorf("store: chat %s: skills: %w", c.ID, err)
	}
	var err error
	c.CreatedAt, err = parseTime(created)
	return c, err
}

// EnsureMother creates the Mother chat if the project has none (SPEC 8.6).
// It runs when a project is created and again at every open, so a crash
// between the two files can't leave a project without one.
func (c *ChatsDB) EnsureMother(ctx context.Context) (chat.Chat, error) {
	return Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (chat.Chat, error) {
		m, err := scanChat(tx.QueryRow(`SELECT ` + chatCols + ` FROM chats WHERE kind = 'mother'`))
		if !errors.Is(err, sql.ErrNoRows) {
			return m, err
		}
		title, err := freeTitle(tx, "", MotherTitle) // "Mother 2" if a chat has the name
		if err != nil {
			return m, err
		}
		m = chat.Chat{ID: id.Chat(id.New()), Kind: chat.KindMother, Title: title, TitleFixed: true,
			Skills: []string{}, Model: DefaultModel, CreatedBy: id.SourceApp, CreatedAt: now()}
		return m, insertChat(tx, m)
	})
}

// CreateChat adds a chat with no title yet; one is generated after the
// first turn (P1-10). Role, skills, model and default page are taken from
// c; the model defaults to DefaultModel.
func (c *ChatsDB) CreateChat(ctx context.Context, src id.Source, ch chat.Chat) (chat.Chat, error) {
	ch.ID, ch.Kind, ch.CreatedBy, ch.CreatedAt, ch.Archived = id.Chat(id.New()), chat.KindChat, src, now(), false
	if ch.Model == "" {
		ch.Model = DefaultModel
	}
	if ch.Skills == nil {
		ch.Skills = []string{}
	}
	if estimateTokens(ch.Role) > MaxRoleTokens {
		return chat.Chat{}, fmt.Errorf("%w: a role has at most %d tokens", ErrTooLong, MaxRoleTokens)
	}
	if ch.Title != "" {
		t, err := checkTitle(ch.Title)
		if err != nil {
			return chat.Chat{}, err
		}
		ch.Title, ch.TitleFixed = t, true
	}
	return Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (chat.Chat, error) {
		if err := insertChat(tx, ch); err != nil {
			return ch, err
		}
		if ch.Role != "" {
			return ch, recordRole(tx, ch.ID, "", ch.Role, src)
		}
		return ch, nil
	})
}

func insertChat(tx *sql.Tx, c chat.Chat) error {
	skills, err := json.Marshal(c.Skills)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO chats (`+chatCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Kind, c.Title, c.TitleFixed, c.Role, string(skills), c.Model, c.DefaultPage, c.CreatedBy, formatTime(c.CreatedAt), c.Archived)
	if isUnique(err) {
		return ErrTitleTaken
	}
	return err
}

// Chat returns one chat.
func (c *ChatsDB) Chat(ctx context.Context, ch id.Chat) (chat.Chat, error) {
	rows, err := Query(ctx, c.DB, `SELECT `+chatCols+` FROM chats WHERE id = ?`, []any{ch}, func(r *sql.Rows) (chat.Chat, error) { return scanChat(r) })
	if err != nil {
		return chat.Chat{}, err
	}
	if len(rows) == 0 {
		return chat.Chat{}, fmt.Errorf("store: chat %s: %w", ch, ErrNotFound)
	}
	return rows[0], nil
}

// Chats returns every chat, archived ones too: Mother first, then the
// oldest first.
func (c *ChatsDB) Chats(ctx context.Context) ([]chat.Chat, error) {
	return Query(ctx, c.DB, `SELECT `+chatCols+` FROM chats ORDER BY kind <> 'mother', created_at, id`, nil,
		func(r *sql.Rows) (chat.Chat, error) { return scanChat(r) })
}

// Seq is the chat's sequence number now (Q32).
func (c *ChatsDB) Seq(ctx context.Context, ch id.Chat) (uint64, error) {
	rows, err := Query(ctx, c.DB, `SELECT seq FROM chats WHERE id = ?`, []any{ch}, func(r *sql.Rows) (uint64, error) {
		var s uint64
		return s, r.Scan(&s)
	})
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("store: chat %s: %w", ch, ErrNotFound)
	}
	return rows[0], nil
}

// bump adds one to the chat's sequence number and returns it. Every write
// to a chat calls it in its transaction.
func bump(tx *sql.Tx, ch id.Chat) (uint64, error) {
	var s uint64
	err := tx.QueryRow(`UPDATE chats SET seq = seq + 1 WHERE id = ? RETURNING seq`, ch).Scan(&s)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("store: chat %s: %w", ch, ErrNotFound)
	}
	return s, err
}

// checkTitle trims t and checks its length and that it is one line.
func checkTitle(t string) (string, error) {
	t = strings.TrimSpace(t)
	if t == "" || utf8.RuneCountInString(t) > MaxTitle || strings.ContainsAny(t, "\r\n") {
		return "", ErrBadTitle
	}
	return t, nil
}

// SetTitle sets a chat's title. A title set by the user or Mother (fixed)
// stays; a generated one is ignored once the title is fixed, and gets a
// number when another chat has it. It returns the title the chat has now.
func (c *ChatsDB) SetTitle(ctx context.Context, ch id.Chat, title string, fixed bool) (string, uint64, error) {
	title, err := checkTitle(title)
	if err != nil {
		return "", 0, err
	}
	type res struct {
		title string
		seq   uint64
	}
	r, err := Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (res, error) {
		var cur string
		var wasFixed bool
		if err := tx.QueryRow(`SELECT title, title_fixed FROM chats WHERE id = ?`, ch).Scan(&cur, &wasFixed); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				err = fmt.Errorf("store: chat %s: %w", ch, ErrNotFound)
			}
			return res{}, err
		}
		if wasFixed && !fixed {
			return res{title: cur}, nil
		}
		if !fixed {
			var err error
			if title, err = freeTitle(tx, ch, title); err != nil {
				return res{}, err
			}
		}
		if _, err := tx.Exec(`UPDATE chats SET title = ?, title_fixed = title_fixed OR ? WHERE id = ?`, title, fixed, ch); err != nil {
			if isUnique(err) {
				err = ErrTitleTaken
			}
			return res{}, err
		}
		s, err := bump(tx, ch)
		return res{title, s}, err
	})
	return r.title, r.seq, err
}

// freeTitle returns title, or title with " 2", " 3"… when another chat
// has it.
func freeTitle(tx *sql.Tx, self id.Chat, title string) (string, error) {
	for n := 1; ; n++ {
		t := title
		if n > 1 {
			sfx := " " + strconv.Itoa(n)
			t = truncateRunes(title, MaxTitle-utf8.RuneCountInString(sfx)) + sfx
		}
		var taken bool
		if err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM chats WHERE title = ? COLLATE NOCASE AND id <> ?)`, t, self).Scan(&taken); err != nil {
			return "", err
		}
		if !taken {
			return t, nil
		}
	}
}

func truncateRunes(s string, n int) string {
	for i := range s {
		if n == 0 {
			return strings.TrimSpace(s[:i])
		}
		n--
	}
	return s
}

// SetRole changes a chat's role and records the change with its source
// (SPEC 8.6). The Mother chat has no role.
func (c *ChatsDB) SetRole(ctx context.Context, ch id.Chat, role string, src id.Source) (uint64, error) {
	role = strings.TrimSpace(role)
	if estimateTokens(role) > MaxRoleTokens {
		return 0, fmt.Errorf("%w: a role has at most %d tokens", ErrTooLong, MaxRoleTokens)
	}
	return Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (uint64, error) {
		var kind chat.Kind
		var before string
		if err := tx.QueryRow(`SELECT kind, role FROM chats WHERE id = ?`, ch).Scan(&kind, &before); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				err = fmt.Errorf("store: chat %s: %w", ch, ErrNotFound)
			}
			return 0, err
		}
		if kind == chat.KindMother {
			return 0, ErrMother
		}
		if before == role {
			return bump(tx, ch)
		}
		if _, err := tx.Exec(`UPDATE chats SET role = ? WHERE id = ?`, role, ch); err != nil {
			return 0, err
		}
		if err := recordRole(tx, ch, before, role, src); err != nil {
			return 0, err
		}
		return bump(tx, ch)
	})
}

func recordRole(tx *sql.Tx, ch id.Chat, before, after string, src id.Source) error {
	_, err := tx.Exec(`INSERT INTO role_changes (chat_id, before, after, source, at) VALUES (?, ?, ?, ?, ?)`,
		ch, before, after, src, formatTime(now()))
	return err
}

// RoleChange is one recorded change of a chat's role.
type RoleChange struct {
	Before, After string
	Source        id.Source
	At            time.Time
}

// RoleChanges returns a chat's role changes, the oldest first.
func (c *ChatsDB) RoleChanges(ctx context.Context, ch id.Chat) ([]RoleChange, error) {
	return Query(ctx, c.DB, `SELECT before, after, source, at FROM role_changes WHERE chat_id = ? ORDER BY id`, []any{ch},
		func(r *sql.Rows) (RoleChange, error) {
			var x RoleChange
			var at string
			if err := r.Scan(&x.Before, &x.After, &x.Source, &at); err != nil {
				return x, err
			}
			var err error
			x.At, err = parseTime(at)
			return x, err
		})
}

// SetModel sets the model alias or model a chat uses (SPEC 3.9).
func (c *ChatsDB) SetModel(ctx context.Context, ch id.Chat, model string) (uint64, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return 0, errors.New("store: a chat needs a model")
	}
	return c.update(ctx, ch, `UPDATE chats SET model = ? WHERE id = ?`, model, ch)
}

// SetSkills sets the skills loaded in a chat (8.9); they move into block 1
// at the next cut.
func (c *ChatsDB) SetSkills(ctx context.Context, ch id.Chat, skills []string) (uint64, error) {
	if skills == nil {
		skills = []string{}
	}
	b, err := json.Marshal(skills)
	if err != nil {
		return 0, err
	}
	return c.update(ctx, ch, `UPDATE chats SET skills = ? WHERE id = ?`, string(b), ch)
}

// Archive archives or restores a chat. The Mother chat can't be archived.
func (c *ChatsDB) Archive(ctx context.Context, ch id.Chat, archived bool) (uint64, error) {
	return c.update(ctx, ch, `UPDATE chats SET archived = ? WHERE id = ? AND kind <> 'mother'`, archived, ch)
}

// update runs one UPDATE of the chat and bumps its sequence number. No row
// changed means a missing chat, or the Mother chat where it is excluded.
func (c *ChatsDB) update(ctx context.Context, ch id.Chat, q string, args ...any) (uint64, error) {
	return Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (uint64, error) {
		res, err := tx.Exec(q, args...)
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return 0, missingOrMother(tx, ch)
		}
		return bump(tx, ch)
	})
}

func missingOrMother(tx *sql.Tx, ch id.Chat) error {
	var kind chat.Kind
	err := tx.QueryRow(`SELECT kind FROM chats WHERE id = ?`, ch).Scan(&kind)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("store: chat %s: %w", ch, ErrNotFound)
	case err != nil:
		return err
	case kind == chat.KindMother:
		return ErrMother
	}
	return fmt.Errorf("store: chat %s was not changed", ch)
}

// DeleteChat removes a chat with its messages, notes and review results.
// The Mother chat can't be deleted; Clear empties it instead.
func (c *ChatsDB) DeleteChat(ctx context.Context, ch id.Chat) error {
	_, err := Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (struct{}, error) {
		res, err := tx.Exec(`DELETE FROM chats WHERE id = ? AND kind <> 'mother'`, ch)
		if err != nil {
			return struct{}{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return struct{}{}, missingOrMother(tx, ch)
		}
		return struct{}{}, nil
	})
	return err
}

// Clear removes a chat's messages, session notes and review results, and
// keeps the chat with its title, role and model. It works on every chat,
// the Mother chat too.
func (c *ChatsDB) Clear(ctx context.Context, ch id.Chat) (uint64, error) {
	return Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (uint64, error) {
		for _, q := range []string{
			`DELETE FROM messages WHERE chat_id = ?`,
			`DELETE FROM session_notes WHERE chat_id = ?`,
			`DELETE FROM review_chunks WHERE chat_id = ?`,
			`UPDATE chats SET memory_reviewed_up_to = '' WHERE id = ?`,
		} {
			if _, err := tx.Exec(q, ch); err != nil {
				return 0, err
			}
		}
		return bump(tx, ch)
	})
}

// AppendMessage writes a message with the parts it has so far, in one
// transaction. An assistant message and its tool_call part are written
// before the tool runs (SPEC 2.3). The ID and time are set if empty.
func (c *ChatsDB) AppendMessage(ctx context.Context, m chat.Message) (chat.Message, uint64, error) {
	if m.ID == "" {
		m.ID = id.Message(id.New())
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now()
	}
	if m.Turn < 1 {
		return m, 0, fmt.Errorf("store: message %s: turn %d, turns start at 1", m.ID, m.Turn)
	}
	if err := m.Validate(); err != nil {
		return m, 0, err
	}
	rows, err := encodeParts(m.Parts)
	if err != nil {
		return m, 0, err
	}
	s, err := Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (uint64, error) {
		u := m.Usage
		if _, err := tx.Exec(`INSERT INTO messages (id, chat_id, turn, role, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.ID, m.Chat, m.Turn, m.Role, m.Model, u.Input, u.Output, u.CacheRead, u.CacheWrite, formatTime(m.CreatedAt)); err != nil {
			if isForeignKey(err) {
				err = fmt.Errorf("store: chat %s: %w", m.Chat, ErrNotFound)
			}
			return 0, err
		}
		for i, r := range rows {
			if err := insertPart(tx, m.ID, i, r); err != nil {
				return 0, err
			}
		}
		return bump(tx, m.Chat)
	})
	return m, s, err
}

// AppendPart adds a finished part to the end of a message. Streamed text is
// written only here, when the part is complete (SPEC 2.3). It returns the
// part's index.
func (c *ChatsDB) AppendPart(ctx context.Context, m id.Message, p chat.Part) (int, uint64, error) {
	if err := p.Validate(); err != nil {
		return 0, 0, err
	}
	rows, err := encodeParts([]chat.Part{p})
	if err != nil {
		return 0, 0, err
	}
	type res struct {
		i int
		s uint64
	}
	r, err := Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (res, error) {
		var ch id.Chat
		var i int
		err := tx.QueryRow(`SELECT chat_id, (SELECT count(*) FROM message_parts WHERE message_id = m.id) FROM messages m WHERE id = ?`, m).Scan(&ch, &i)
		if errors.Is(err, sql.ErrNoRows) {
			return res{}, fmt.Errorf("store: message %s: %w", m, ErrNotFound)
		}
		if err != nil {
			return res{}, err
		}
		if err := insertPart(tx, m, i, rows[0]); err != nil {
			return res{}, err
		}
		s, err := bump(tx, ch)
		return res{i, s}, err
	})
	return r.i, r.s, err
}

// SetPart replaces an approval card or a question form with its answered
// or closed form (SPEC 8.8). Other parts don't change once written, and
// neither does a card that was already answered or closed: that is
// ErrNotPending.
func (c *ChatsDB) SetPart(ctx context.Context, m id.Message, i int, p chat.Part) (uint64, error) {
	if p.Kind != chat.PartApproval && p.Kind != chat.PartQuestion {
		return 0, fmt.Errorf("store: a %s part can't be changed", p.Kind)
	}
	if err := p.Validate(); err != nil {
		return 0, err
	}
	rows, err := encodeParts([]chat.Part{p})
	if err != nil {
		return 0, err
	}
	return Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (uint64, error) {
		var ch id.Chat
		var content string
		err := tx.QueryRow(`SELECT m.chat_id, p.content FROM message_parts p JOIN messages m ON m.id = p.message_id
			WHERE p.message_id = ? AND p.seq = ? AND p.type = ?`, m, i, p.Kind).Scan(&ch, &content)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("store: message %s has no %s part %d: %w", m, p.Kind, i, ErrNotFound)
		}
		if err != nil {
			return 0, err
		}
		old, err := decodePart(p.Kind, content)
		if err != nil {
			return 0, err
		}
		if !pendingCard(old) {
			return 0, fmt.Errorf("store: message %s, %s part %d: %w", m, p.Kind, i, ErrNotPending)
		}
		if _, err := tx.Exec(`UPDATE message_parts SET content = ? WHERE message_id = ? AND seq = ?`, rows[0].content, m, i); err != nil {
			return 0, err
		}
		return bump(tx, ch)
	})
}

// pendingCard tells whether an approval card or question form still waits
// for its answer.
func pendingCard(p chat.Part) bool {
	switch {
	case p.Approval != nil:
		return p.Approval.Answer == "" && p.Approval.AnsweredAt == nil && !p.Approval.Stopped
	case p.Question != nil:
		return p.Question.AnsweredAt == nil && !p.Question.Stopped
	}
	return false
}

// SetUsage saves an assistant message's token counts once its response has
// ended.
func (c *ChatsDB) SetUsage(ctx context.Context, m id.Message, u chat.Usage) (uint64, error) {
	return Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (uint64, error) {
		var ch id.Chat
		err := tx.QueryRow(`UPDATE messages SET input_tokens = ?, output_tokens = ?, cache_read_tokens = ?, cache_write_tokens = ?
			WHERE id = ? RETURNING chat_id`, u.Input, u.Output, u.CacheRead, u.CacheWrite, m).Scan(&ch)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("store: message %s: %w", m, ErrNotFound)
		}
		if err != nil {
			return 0, err
		}
		return bump(tx, ch)
	})
}

// partRow is a part as stored.
type partRow struct {
	kind                  chat.PartKind
	content, ref, preview string
	index                 string // the normalized text for messages_fts; "" for parts that aren't indexed
}

func encodeParts(ps []chat.Part) ([]partRow, error) {
	out := make([]partRow, len(ps))
	for i, p := range ps {
		var v any
		r := partRow{kind: p.Kind}
		switch p.Kind {
		case chat.PartText:
			v, r.index = p.Text, Normalize(p.Text.Text)
		case chat.PartThinking:
			v = p.Thinking
		case chat.PartToolCall:
			v = p.ToolCall
		case chat.PartToolResult:
			v = p.ToolResult
			if p.ToolResult.Ref != "" {
				r.ref, r.preview = p.ToolResult.Ref, p.ToolResult.Text
			}
		case chat.PartImage:
			v, r.ref = p.Image, p.Image.Ref
		case chat.PartNotice:
			v = p.Notice
		case chat.PartApproval:
			v = p.Approval
		case chat.PartQuestion:
			v = p.Question
		}
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("store: part %d: %w", i, err)
		}
		r.content = string(b)
		out[i] = r
	}
	return out, nil
}

// insertPart writes a part, and indexes a text part in the same
// transaction.
func insertPart(tx *sql.Tx, m id.Message, i int, r partRow) error {
	var rowid int64
	if err := tx.QueryRow(`INSERT INTO message_parts (message_id, seq, type, content, ref, preview) VALUES (?, ?, ?, ?, ?, ?) RETURNING rowid`,
		m, i, r.kind, r.content, r.ref, r.preview).Scan(&rowid); err != nil {
		return err
	}
	if r.index == "" {
		return nil
	}
	_, err := tx.Exec(`INSERT INTO messages_fts (rowid, text, chat) SELECT ?, ?, chat_id FROM messages WHERE id = ?`, rowid, r.index, m)
	return err
}

func decodePart(kind chat.PartKind, content string) (chat.Part, error) {
	p := chat.Part{Kind: kind}
	var v any
	switch kind {
	case chat.PartText:
		p.Text = &chat.Text{}
		v = p.Text
	case chat.PartThinking:
		p.Thinking = &chat.Thinking{}
		v = p.Thinking
	case chat.PartToolCall:
		p.ToolCall = &chat.ToolCall{}
		v = p.ToolCall
	case chat.PartToolResult:
		p.ToolResult = &chat.ToolResult{}
		v = p.ToolResult
	case chat.PartImage:
		p.Image = &chat.Image{}
		v = p.Image
	case chat.PartNotice:
		p.Notice = &chat.Notice{}
		v = p.Notice
	case chat.PartApproval:
		p.Approval = &chat.Approval{}
		v = p.Approval
	case chat.PartQuestion:
		p.Question = &chat.Question{}
		v = p.Question
	default:
		return p, fmt.Errorf("store: unknown part type %q", kind)
	}
	return p, json.Unmarshal([]byte(content), v)
}

// Messages returns a chat's messages of turns from through to, with their
// parts, and the sequence number they are current at (Q32). to = 0 means
// up to the last turn.
func (c *ChatsDB) Messages(ctx context.Context, ch id.Chat, from, to int) ([]chat.Message, uint64, error) {
	if to <= 0 {
		to = int(^uint32(0) >> 1)
	}
	type out struct {
		ms  []chat.Message
		seq uint64
	}
	r, err := readTx(ctx, c.DB, func(tx *sql.Tx) (out, error) {
		var o out
		if err := tx.QueryRow(`SELECT seq FROM chats WHERE id = ?`, ch).Scan(&o.seq); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				err = fmt.Errorf("store: chat %s: %w", ch, ErrNotFound)
			}
			return o, err
		}
		var err error
		o.ms, err = queryTx(tx, `SELECT id, chat_id, turn, role, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, created_at
			FROM messages WHERE chat_id = ? AND turn BETWEEN ? AND ? ORDER BY turn, id`, []any{ch, from, to},
			func(r *sql.Rows) (chat.Message, error) {
				var m chat.Message
				var created string
				u := &m.Usage
				if err := r.Scan(&m.ID, &m.Chat, &m.Turn, &m.Role, &m.Model, &u.Input, &u.Output, &u.CacheRead, &u.CacheWrite, &created); err != nil {
					return m, err
				}
				m.Parts = []chat.Part{}
				var err error
				m.CreatedAt, err = parseTime(created)
				return m, err
			})
		if err != nil || len(o.ms) == 0 {
			return o, err
		}
		at := make(map[id.Message]int, len(o.ms))
		for i, m := range o.ms {
			at[m.ID] = i
		}
		type part struct {
			m id.Message
			p chat.Part
		}
		parts, err := queryTx(tx, `SELECT p.message_id, p.type, p.content FROM message_parts p JOIN messages m ON m.id = p.message_id
			WHERE m.chat_id = ? AND m.turn BETWEEN ? AND ? ORDER BY p.message_id, p.seq`, []any{ch, from, to},
			func(r *sql.Rows) (part, error) {
				var x part
				var kind chat.PartKind
				var content string
				if err := r.Scan(&x.m, &kind, &content); err != nil {
					return x, err
				}
				var err error
				x.p, err = decodePart(kind, content)
				return x, err
			})
		for _, x := range parts {
			i := at[x.m]
			o.ms[i].Parts = append(o.ms[i].Parts, x.p)
		}
		return o, err
	})
	return r.ms, r.seq, err
}

// LastTurn is the chat's last turn, 0 if it has no messages.
func (c *ChatsDB) LastTurn(ctx context.Context, ch id.Chat) (int, error) {
	rows, err := Query(ctx, c.DB, `SELECT COALESCE(MAX(turn), 0) FROM messages WHERE chat_id = ?`, []any{ch}, func(r *sql.Rows) (int, error) {
		var t int
		return t, r.Scan(&t)
	})
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	return rows[0], nil
}

// LastActivity is the time of each chat's newest message, for Mother's
// chat list (SPEC 8.6). Chats without messages are left out.
func (c *ChatsDB) LastActivity(ctx context.Context) (map[id.Chat]time.Time, error) {
	type row struct {
		ch id.Chat
		at time.Time
	}
	// One index lookup per chat, not a scan of every message.
	rows, err := Query(ctx, c.DB, `SELECT id, at FROM (SELECT id, (SELECT MAX(created_at) FROM messages WHERE chat_id = chats.id) AS at FROM chats) WHERE at IS NOT NULL`, nil, func(r *sql.Rows) (row, error) {
		var x row
		var at string
		if err := r.Scan(&x.ch, &at); err != nil {
			return x, err
		}
		var err error
		x.at, err = parseTime(at)
		return x, err
	})
	out := make(map[id.Chat]time.Time, len(rows))
	for _, x := range rows {
		out[x.ch] = x.at
	}
	return out, err
}

// Notes returns a chat's session notes; revision 0 when it has none yet.
func (c *ChatsDB) Notes(ctx context.Context, ch id.Chat) (chat.SessionNote, error) {
	rows, err := Query(ctx, c.DB, `SELECT content, revision, updated_at FROM session_notes WHERE chat_id = ?`, []any{ch},
		func(r *sql.Rows) (chat.SessionNote, error) {
			n := chat.SessionNote{Chat: ch}
			var at string
			if err := r.Scan(&n.Content, &n.Revision, &at); err != nil {
				return n, err
			}
			var err error
			n.UpdatedAt, err = parseTime(at)
			return n, err
		})
	if err != nil || len(rows) == 0 {
		return chat.SessionNote{Chat: ch}, err
	}
	return rows[0], nil
}

// SaveNotes replaces a chat's session notes. n.Revision is the revision the
// writer read (0 for none); if the notes changed since, nothing is written
// and ErrConflict is returned, so the writer can read them again and merge
// (SPEC 3.3, 3.4). It returns the new revision.
func (c *ChatsDB) SaveNotes(ctx context.Context, n chat.SessionNote) (int, error) {
	if estimateTokens(n.Content) > MaxNotesTokens {
		return 0, fmt.Errorf("%w: session notes have at most %d tokens", ErrTooLong, MaxNotesTokens)
	}
	return Do(ctx, c.DB, limit.Interactive, func(tx *sql.Tx) (int, error) {
		var cur int
		err := tx.QueryRow(`SELECT revision FROM session_notes WHERE chat_id = ?`, n.Chat).Scan(&cur)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		if cur != n.Revision {
			return 0, fmt.Errorf("store: session notes are at revision %d, not %d: %w", cur, n.Revision, ErrConflict)
		}
		if _, err := tx.Exec(`INSERT INTO session_notes (chat_id, content, revision, updated_at) VALUES (?, ?, ?, ?)
			ON CONFLICT (chat_id) DO UPDATE SET content = excluded.content, revision = excluded.revision, updated_at = excluded.updated_at`,
			n.Chat, n.Content, cur+1, formatTime(now())); err != nil {
			if isForeignKey(err) {
				err = fmt.Errorf("store: chat %s: %w", n.Chat, ErrNotFound)
			}
			return 0, err
		}
		if _, err := bump(tx, n.Chat); err != nil {
			return 0, err
		}
		return cur + 1, nil
	})
}

func now() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }

// UsageRow is the tokens one chat used with one model on one day.
type UsageRow struct {
	Day   string // YYYY-MM-DD in the caller's time zone
	Chat  id.Chat
	Model string // "provider/model"
	Usage chat.Usage
}

// Usage sums the tokens of the messages since from, by day, chat and
// model (SPEC 3.9). offset is the caller's UTC offset, which sets the day.
func (c *ChatsDB) Usage(ctx context.Context, from time.Time, offset time.Duration) ([]UsageRow, error) {
	shift := fmt.Sprintf("%+d minutes", int(offset.Minutes()))
	return Query(ctx, c.DB, `SELECT date(created_at, ?) AS day, chat_id, model,
			SUM(input_tokens), SUM(output_tokens), SUM(cache_read_tokens), SUM(cache_write_tokens)
		FROM messages
		WHERE created_at >= ? AND input_tokens + output_tokens + cache_read_tokens + cache_write_tokens > 0
		GROUP BY day, chat_id, model ORDER BY day, chat_id, model`, []any{shift, formatTime(from)}, func(r *sql.Rows) (UsageRow, error) {
		var u UsageRow
		return u, r.Scan(&u.Day, &u.Chat, &u.Model, &u.Usage.Input, &u.Usage.Output, &u.Usage.CacheRead, &u.Usage.CacheWrite)
	})
}
