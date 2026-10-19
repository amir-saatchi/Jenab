package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

// topN is how deep each list goes: the FTS and vector lists that hybrid
// merges, and the rank past which a target counts as lost.
const topN = 50

// messageFTS is today's search_history: the app's own ChatsDB.Search over a
// chats.db built from the corpus with the app's store.
type messageFTS struct {
	db    *store.ChatsDB
	toID  map[id.Message]string // the store's ID → the corpus ID
	bytes int64                 // text bytes indexed
}

func buildMessageFTS(ctx context.Context, dir string, c *corpus) (*messageFTS, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	db, err := store.OpenChats(ctx, dir)
	if err != nil {
		return nil, err
	}
	f := &messageFTS{db: db, toID: map[id.Message]string{}}
	at := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	for _, ch := range c.Chats {
		title := ch.Title
		if title == "" {
			title = ch.ID
		}
		sc, err := db.CreateChat(ctx, id.SourceUser, chat.Chat{Title: clip(title, store.MaxTitle-1)})
		if err != nil {
			db.Close(ctx)
			return nil, fmt.Errorf("chat %s: %w", ch.ID, err)
		}
		turn := 0
		for _, m := range ch.Messages {
			role := chat.Role(m.Role)
			if role == chat.RoleUser || turn == 0 {
				turn++
			}
			at = at.Add(time.Minute)
			sm, _, err := db.AppendMessage(ctx, chat.Message{Chat: sc.ID, Turn: turn, Role: role, CreatedAt: at,
				Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: m.Text}}}})
			if err != nil {
				db.Close(ctx)
				return nil, fmt.Errorf("message %s: %w", m.ID, err)
			}
			f.toID[sm.ID] = m.ID
			f.bytes += int64(len(m.Text))
		}
	}
	return f, nil
}

func (f *messageFTS) Close() error { return f.db.Close(context.Background()) }

// search returns the corpus IDs, best first. ChatsDB.Search gives no score,
// so the score is 0.
func (f *messageFTS) search(ctx context.Context, q string) ([]scored, error) {
	res, err := f.db.Search(ctx, store.SearchReq{Query: q, Limit: topN})
	if err != nil {
		return nil, err
	}
	out := make([]scored, 0, len(res.Hits))
	for _, h := range res.Hits {
		out = append(out, scored{ID: f.toID[h.Message]})
	}
	return out, nil
}

// chunkFTS is a lexical bucket_search: an FTS5 table over one chunking's
// chunks, with the store's normalization and a copy of its query builder.
type chunkFTS struct {
	db *sql.DB
}

func buildChunkFTS(ctx context.Context, path string, chunks []chunk) (*chunkFTS, error) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	f := &chunkFTS{db: db}
	if err := f.fill(ctx, chunks); err != nil {
		db.Close()
		return nil, err
	}
	return f, nil
}

func (f *chunkFTS) fill(ctx context.Context, chunks []chunk) error {
	// The same tokenizer as messages_fts (SPEC 2.3); the chunk ID is kept
	// unindexed beside the text.
	if _, err := f.db.ExecContext(ctx, `CREATE VIRTUAL TABLE chunks_fts USING fts5 (
		text, id UNINDEXED, tokenize = 'unicode61 remove_diacritics 2')`); err != nil {
		return err
	}
	tx, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range chunks {
		if _, err := tx.Exec(`INSERT INTO chunks_fts (text, id) VALUES (?, ?)`, store.Normalize(c.Text), c.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (f *chunkFTS) Close() error { return f.db.Close() }

// search returns chunk IDs by bm25, best first. The score is -bm25, so
// higher is better.
func (f *chunkFTS) search(ctx context.Context, q string) ([]scored, error) {
	terms, err := queryTerms(q)
	if err != nil || len(terms) == 0 {
		return nil, err
	}
	rows, err := f.db.QueryContext(ctx, `SELECT id, rank FROM chunks_fts WHERE chunks_fts MATCH ? ORDER BY rank LIMIT ?`, ftsQuery(terms), topN)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []scored
	for rows.Next() {
		var s scored
		if err := rows.Scan(&s.ID, &s.Score); err != nil {
			return nil, err
		}
		s.Score = -s.Score
		out = append(out, s)
	}
	return out, rows.Err()
}

// The query builder below is a copy of the store's unexported queryTerms
// and ftsQuery (internal/store/search.go), without the chat column. The
// store normalizes a query with its ZWNJ kept; store.Normalize drops it, so
// each ZWNJ is swapped for a private-use character around the call.

const (
	zwnj        = '‌'
	zwnjStandIn = ''
)

type queryTerm struct {
	joined string   // ZWNJ removed
	split  []string // the parts around each ZWNJ; nil without one
}

func normalizeKeepZWNJ(s string) string {
	s = strings.ReplaceAll(s, string(zwnj), string(zwnjStandIn))
	return strings.ReplaceAll(store.Normalize(s), string(zwnjStandIn), string(zwnj))
}

func queryTerms(q string) ([]queryTerm, error) {
	words := strings.FieldsFunc(normalizeKeepZWNJ(q), func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
	if len(words) > store.MaxSearchTerms {
		return nil, store.ErrBadQuery
	}
	var out []queryTerm
	for _, w := range words {
		t := queryTerm{joined: strings.ReplaceAll(w, string(zwnj), "")}
		if t.joined == "" {
			continue
		}
		if parts := strings.FieldsFunc(w, func(r rune) bool { return r == zwnj }); len(parts) > 1 {
			t.split = parts
		}
		out = append(out, t)
	}
	return out, nil
}

func ftsQuery(terms []queryTerm) string {
	quote := func(t string) string { return `"` + strings.ReplaceAll(t, `"`, `""`) + `"` }
	parts := make([]string, len(terms))
	for i, t := range terms {
		parts[i] = quote(t.joined) + "*"
		if t.split != nil {
			parts[i] = "(" + parts[i] + " OR " + quote(strings.Join(t.split, " ")) + "*)"
		}
	}
	return "text : (" + strings.Join(parts, " AND ") + ")"
}
