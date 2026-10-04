package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
)

// History search (SPEC 2.3, SPIKE-011): one FTS5 index over text parts,
// with the text normalized in Go before it is indexed and the same way
// before it is searched.

// ftsTables is chats.db format step 2. The index is contentless: it holds
// no text, only tokens, and its rowid is the text part's rowid. The chat
// column holds the chat ID as one token, so a search in one chat stays
// inside the index; every hit in the chat has it, so it doesn't change the
// order. A trigger removes a
// part's tokens with the part, also when a chat is cleared or deleted. Text
// parts written before this step are indexed here.
func ftsTables(tx *sql.Tx) error {
	if err := execAll(
		`CREATE VIRTUAL TABLE messages_fts USING fts5 (
			text,
			chat,
			tokenize = 'unicode61 remove_diacritics 2',
			content = '',
			contentless_delete = 1
		)`,
		`CREATE TRIGGER message_parts_unindex AFTER DELETE ON message_parts WHEN old.type = 'text'
		BEGIN
			DELETE FROM messages_fts WHERE rowid = old.rowid;
		END`,
	)(tx); err != nil {
		return err
	}
	type row struct {
		rowid   int64
		content string
		chat    id.Chat
	}
	rows, err := queryTx(tx, `SELECT p.rowid, p.content, m.chat_id FROM message_parts p JOIN messages m ON m.id = p.message_id WHERE p.type = 'text'`, nil,
		func(r *sql.Rows) (row, error) {
			var x row
			return x, r.Scan(&x.rowid, &x.content, &x.chat)
		})
	if err != nil {
		return err
	}
	for _, r := range rows {
		var t chat.Text
		if err := json.Unmarshal([]byte(r.content), &t); err != nil {
			return fmt.Errorf("text part %d: %w", r.rowid, err)
		}
		if _, err := tx.Exec(`INSERT INTO messages_fts (rowid, text, chat) VALUES (?, ?, ?)`, r.rowid, Normalize(t.Text), r.chat); err != nil {
			return err
		}
	}
	return nil
}

const zwnj = '\u200c' // zero-width non-joiner, inside Persian words

// Normalize is the normalization of SPEC 2.3, for indexed text and queries:
// NFKC; ي ى → ی and ك → ک; tatweel and Arabic diacritics removed; Persian and
// Arabic-Indic digits → ASCII; ß → ss; ZWNJ removed. Case and Latin
// diacritics are left to the tokenizer.
func Normalize(s string) string { return normalize(s, false) }

func normalize(s string, keepZWNJ bool) string {
	s = norm.NFKC.String(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == zwnj:
			if keepZWNJ {
				b.WriteRune(r)
			}
		case r == 'ي' || r == 'ى':
			b.WriteRune('ی')
		case r == 'ك':
			b.WriteRune('ک')
		case r == 'ـ', r >= 0x064B && r <= 0x065F, r == 0x0670: // tatweel, harakat, superscript alef
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + (r - '۰'))
		case r >= '٠' && r <= '٩':
			b.WriteRune('0' + (r - '٠'))
		case r == 'ß':
			b.WriteString("ss")
		case r == 'ẞ':
			b.WriteString("SS")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// MaxSearchTerms bounds a query's words.
const MaxSearchTerms = 32

var ErrBadQuery = errors.New("store: a search has at most 32 words")

// queryTerm is one word of a query, normalized with its ZWNJ kept. A word
// with a ZWNJ also matches its split form: بیت‌کوین finds "بیتکوین" and
// "بیت کوین".
type queryTerm struct {
	joined string   // ZWNJ removed
	split  []string // the parts around each ZWNJ; nil without one
}

func queryTerms(q string) ([]queryTerm, error) {
	// Control characters separate words too: the tokenizer drops them, and
	// a NUL would end the query for FTS5.
	words := strings.FieldsFunc(normalize(q, true), func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
	if len(words) > MaxSearchTerms {
		return nil, ErrBadQuery
	}
	var out []queryTerm
	for _, w := range words {
		t := queryTerm{joined: strings.ReplaceAll(w, string(zwnj), "")}
		if t.joined == "" {
			continue // only ZWNJs
		}
		if parts := strings.FieldsFunc(w, func(r rune) bool { return r == zwnj }); len(parts) > 1 {
			t.split = parts
		}
		out = append(out, t)
	}
	return out, nil
}

// ftsQuery builds the MATCH expression: every term quoted (a " doubled) with
// a prefix star, joined with AND, in the text column. Nothing else from the
// query reaches MATCH, so FTS5 operators and syntax in it are plain text. A
// chat adds its ID, in the chat column.
func ftsQuery(terms []queryTerm, ch id.Chat) string {
	quote := func(t string) string { return `"` + strings.ReplaceAll(t, `"`, `""`) + `"` }
	parts := make([]string, len(terms))
	for i, t := range terms {
		parts[i] = quote(t.joined) + "*"
		if t.split != nil {
			// FTS5 has no implicit AND next to a group, so AND is explicit.
			parts[i] = "(" + parts[i] + " OR " + quote(strings.Join(t.split, " ")) + "*)"
		}
	}
	q := "text : (" + strings.Join(parts, " AND ") + ")"
	if ch != "" {
		q = "chat : " + quote(string(ch)) + " AND " + q
	}
	return q
}

// rankedMatches bounds ranking: bm25 orders the newest rankedMatches
// matches. Ranking costs about 5 µs a match, so a common word in 100,000
// messages would take over 100 ms (N-09). Tests change it.
var rankedMatches = 2000

// SearchReq is a history search (search_history, SPEC 8.1).
type SearchReq struct {
	Query     string
	Chat      id.Chat // "" searches every chat of the project
	Substring bool    // also find words inside words: a slower scan (2.3)
	Limit     int     // 0 means 20; at most 100
}

// Hit is one message that matched, with a snippet of its text.
type Hit struct {
	Chat      id.Chat    `json:"chat"`
	Message   id.Message `json:"message"`
	Turn      int        `json:"turn"`
	Role      chat.Role  `json:"role"`
	CreatedAt time.Time  `json:"created_at"`
	Snippet   string     `json:"snippet"`
}

// SearchResult is the hits, best first for a word search and newest first
// for a substring scan.
type SearchResult struct {
	Hits []Hit `json:"hits"`
	// Stopped is set when a substring scan reached its time limit; older
	// messages may match too.
	Stopped bool `json:"stopped"`
}

// substringBudget is how long a substring scan may run (SPEC 2.3). Tests
// change it.
var substringBudget = 500 * time.Millisecond

// Search finds messages whose text parts have every word of the query. An
// empty query finds nothing.
func (c *ChatsDB) Search(ctx context.Context, s SearchReq) (SearchResult, error) {
	switch {
	case s.Limit <= 0:
		s.Limit = 20
	case s.Limit > 100:
		s.Limit = 100
	}
	terms, err := queryTerms(s.Query)
	if err != nil || len(terms) == 0 {
		return SearchResult{Hits: []Hit{}}, err
	}
	if s.Chat != "" {
		if _, err := c.Seq(ctx, s.Chat); err != nil {
			return SearchResult{}, err
		}
	}
	if s.Substring {
		return c.scan(ctx, s, terms)
	}
	return c.match(ctx, s, terms)
}

const hitCols = `p.message_id, p.content, m.chat_id, m.turn, m.role, m.created_at`

type hitRow struct {
	Hit
	content string
}

func scanHit(r scanner) (hitRow, error) {
	var h hitRow
	var created string
	if err := r.Scan(&h.Message, &h.content, &h.Chat, &h.Turn, &h.Role, &created); err != nil {
		return h, err
	}
	var err error
	h.CreatedAt, err = parseTime(created)
	return h, err
}

// match uses the index: the best of the newest matches by bm25, one hit per
// message.
func (c *ChatsDB) match(ctx context.Context, s SearchReq, terms []queryTerm) (SearchResult, error) {
	// A message can match with several text parts, so a few more rows are
	// read than hits are returned.
	rows, err := Query(ctx, c.DB, `WITH newest AS MATERIALIZED (
			SELECT rowid, rank FROM messages_fts WHERE messages_fts MATCH ?1 ORDER BY rowid DESC LIMIT ?2
		), best AS MATERIALIZED (
			SELECT rowid, rank FROM newest ORDER BY rank, rowid DESC LIMIT ?3
		)
		SELECT `+hitCols+`
		FROM best f
		JOIN message_parts p ON p.rowid = f.rowid
		JOIN messages m ON m.id = p.message_id
		ORDER BY f.rank, f.rowid DESC`, []any{ftsQuery(terms, s.Chat), rankedMatches, s.Limit * 3},
		func(r *sql.Rows) (hitRow, error) { return scanHit(r) })
	if err != nil {
		return SearchResult{}, err
	}
	res := SearchResult{Hits: []Hit{}}
	seen := map[id.Message]bool{}
	for _, h := range rows {
		if seen[h.Message] {
			continue
		}
		seen[h.Message] = true
		text, err := textOf(h.content)
		if err != nil {
			return SearchResult{}, err
		}
		h.Snippet = snippet(text, terms)
		res.Hits = append(res.Hits, h.Hit)
		if len(res.Hits) == s.Limit {
			break
		}
	}
	return res, nil
}

// scan reads text parts newest first and keeps those that contain every
// term anywhere, compared with fold. After substringBudget it returns what
// it found, with Stopped set.
func (c *ChatsDB) scan(ctx context.Context, s SearchReq, terms []queryTerm) (SearchResult, error) {
	limited, cancel := context.WithTimeout(ctx, substringBudget)
	defer cancel()
	hits, err := c.scanParts(limited, s, terms)
	res := SearchResult{Hits: hits}
	if err != nil {
		// An error caused by the budget alone ends the scan.
		if ctx.Err() != nil || limited.Err() == nil {
			return SearchResult{}, err
		}
		res.Stopped = true
	}
	if res.Hits, err = c.addDetails(ctx, res.Hits); err != nil {
		return SearchResult{}, err
	}
	return res, nil
}

// scanParts returns the hits with their message and snippet. It reads only
// message_parts: joining messages for every part made the scan half again
// as slow.
func (c *ChatsDB) scanParts(ctx context.Context, s SearchReq, terms []queryTerm) ([]Hit, error) {
	hits := []Hit{}
	conn, err := c.reader(ctx)
	if err != nil {
		return hits, err
	}
	defer conn.Close()
	rows, err := conn.QueryContext(ctx, `SELECT message_id, content FROM message_parts
		WHERE type = 'text' AND (?1 = '' OR message_id IN (SELECT id FROM messages WHERE chat_id = ?1))
		ORDER BY rowid DESC`, s.Chat)
	if err != nil {
		return hits, err
	}
	defer rows.Close()
	want := foldedTerms(terms)
	var folded []byte
	seen := map[id.Message]bool{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return hits, err
		}
		var m id.Message
		var content string
		if err := rows.Scan(&m, &content); err != nil {
			return hits, err
		}
		if seen[m] {
			continue
		}
		text, err := textOf(content)
		if err != nil {
			return []Hit{}, err
		}
		folded = foldBytes(folded[:0], text)
		if !containsAll(folded, want) {
			continue
		}
		seen[m] = true
		hits = append(hits, Hit{Message: m, Snippet: snippetAt(text, terms, true)})
		if len(hits) == s.Limit {
			return hits, nil
		}
	}
	return hits, rows.Err()
}

// addDetails fills in each hit's chat, turn, role and time. A message
// deleted since the scan read it is left out.
func (c *ChatsDB) addDetails(ctx context.Context, hits []Hit) ([]Hit, error) {
	if len(hits) == 0 {
		return hits, nil
	}
	ids := make([]id.Message, len(hits))
	for i, h := range hits {
		ids[i] = h.Message
	}
	list, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	rows, err := Query(ctx, c.DB, `SELECT id, chat_id, turn, role, created_at FROM messages WHERE id IN (SELECT value FROM json_each(?))`, []any{string(list)},
		func(r *sql.Rows) (Hit, error) {
			var h Hit
			var created string
			if err := r.Scan(&h.Message, &h.Chat, &h.Turn, &h.Role, &created); err != nil {
				return h, err
			}
			var err error
			h.CreatedAt, err = parseTime(created)
			return h, err
		})
	if err != nil {
		return nil, err
	}
	byID := make(map[id.Message]Hit, len(rows))
	for _, r := range rows {
		byID[r.Message] = r
	}
	out := hits[:0]
	for _, h := range hits {
		if d, ok := byID[h.Message]; ok {
			d.Snippet = h.Snippet
			out = append(out, d)
		}
	}
	return out, nil
}

// foldedTerms is each term's forms, folded.
func foldedTerms(terms []queryTerm) [][][]byte {
	want := make([][][]byte, len(terms))
	for i, t := range terms {
		want[i] = [][]byte{foldBytes(nil, t.joined)}
		if t.split != nil {
			want[i] = append(want[i], foldBytes(nil, strings.Join(t.split, " ")))
		}
	}
	return want
}

func containsAll(text []byte, want [][][]byte) bool {
	for _, forms := range want {
		found := false
		for _, f := range forms {
			if bytes.Contains(text, f) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func textOf(content string) (string, error) {
	// Most text has nothing escaped: then the JSON is the text in quotes.
	const prefix = `{"text":"`
	if rest, ok := strings.CutPrefix(content, prefix); ok {
		if i := strings.IndexAny(rest, `"\`); i >= 0 && rest[i] == '"' && (rest[i+1:] == "}" || strings.HasPrefix(rest[i+1:], ",")) {
			return rest[:i], nil
		}
	}
	var t chat.Text
	if err := json.Unmarshal([]byte(content), &t); err != nil {
		return "", fmt.Errorf("store: a text part: %w", err)
	}
	return t.Text, nil
}

// fold is the normalization for comparing in Go: the SPEC 2.3 rules, with
// ZWNJ removed, plus what the tokenizer does: lower case, and Latin, Greek
// and Cyrillic letters without their diacritics. Compatibility forms such
// as Arabic presentation forms and full-width letters become plain ones. If
// offs is not nil, it gets the byte offset in s that each folded rune
// comes from.
func fold(s string, offs *[]int) []rune {
	out := make([]rune, 0, len(s))
	for i, r := range s {
		n := len(out)
		out = foldRune(out, r)
		if offs != nil {
			for range len(out) - n {
				*offs = append(*offs, i)
			}
		}
	}
	return out
}

// foldBytes is fold as UTF-8, appended to dst, for the substring scan.
func foldBytes(dst []byte, s string) []byte {
	var buf [4]rune
	for _, r := range s {
		if r < utf8.RuneSelf {
			if 'A' <= r && r <= 'Z' {
				r += 'a' - 'A'
			}
			dst = append(dst, byte(r))
			continue
		}
		for _, f := range foldRune(buf[:0], r) {
			dst = utf8.AppendRune(dst, f)
		}
	}
	return dst
}

// lowFold holds foldRune's result for the runes below U+0800 (Latin,
// Greek, Cyrillic, Hebrew, Arabic), worked out once: the scan folds every
// message.
var lowFold = func() (t [0x0800][]rune) {
	for r := range rune(len(t)) {
		t[r] = foldSlow(nil, r)
	}
	return t
}()

func foldRune(dst []rune, r rune) []rune {
	if r < utf8.RuneSelf {
		if 'A' <= r && r <= 'Z' {
			r += 'a' - 'A'
		}
		return append(dst, r)
	}
	if r < rune(len(lowFold)) {
		return append(dst, lowFold[r]...)
	}
	return foldSlow(dst, r)
}

func foldSlow(dst []rune, r rune) []rune {
	switch {
	case 'A' <= r && r <= 'Z':
		return append(dst, r+'a'-'A')
	case r < utf8.RuneSelf:
		return append(dst, r)
	case r == zwnj, r == 'ـ', r >= 0x064B && r <= 0x065F, r == 0x0670:
		return dst
	case r == 'ي' || r == 'ى':
		return append(dst, 'ی')
	case r == 'ك':
		return append(dst, 'ک')
	case r >= '۰' && r <= '۹':
		return append(dst, '0'+(r-'۰'))
	case r >= '٠' && r <= '٩':
		return append(dst, '0'+(r-'٠'))
	case r == 'ß', r == 'ẞ':
		return append(dst, 's', 's')
	}
	// Decompose Latin, Greek and Cyrillic letters, to drop their marks, and
	// compatibility forms. Other scripts keep their letters as they are:
	// آ is not ا.
	if r < 0x0590 || r >= 0x1E00 && r < 0x2000 || r >= 0xF900 {
		var b [utf8.UTFMax]byte
		n := utf8.EncodeRune(b[:], r)
		if !norm.NFKD.IsNormal(b[:n]) {
			for _, d := range norm.NFKD.String(string(b[:n])) {
				dst = foldSlow(dst, d) // drops the marks
			}
			return dst
		}
		if unicode.Is(unicode.Mn, r) {
			return dst
		}
	}
	return append(dst, unicode.ToLower(r))
}

// Snippet sizes, in characters.
const (
	snippetBefore = 60
	snippetLen    = 200
)

// snippet is about snippetLen characters of the stored text around the
// first word that starts with a query term, with … where it was cut. It is
// built from the text itself, since the index holds only tokens.
func snippet(text string, terms []queryTerm) string { return snippetAt(text, terms, false) }

func snippetAt(text string, terms []queryTerm, inside bool) string {
	var offs []int
	f := fold(text, &offs)
	pos := -1
	for _, t := range terms {
		forms := [][]rune{fold(t.joined, nil)}
		if t.split != nil {
			forms = append(forms, fold(strings.Join(t.split, " "), nil))
		}
		for _, form := range forms {
			if p := find(f, form, inside); p >= 0 && (pos < 0 || p < pos) {
				pos = p
			}
		}
	}
	start := 0 // a byte offset in text
	if pos > snippetBefore {
		start = offs[pos-snippetBefore]
	}
	end, n := len(text), 0
	for i := range text[start:] {
		if n == snippetLen {
			end = start + i
			break
		}
		n++
	}
	cut := text[start:end]
	// Don't start or end in the middle of a word.
	if before, _ := utf8.DecodeLastRuneInString(text[:start]); start > 0 && !unicode.IsSpace(before) {
		if i := strings.IndexFunc(cut, unicode.IsSpace); i >= 0 && i < 20 {
			cut = cut[i:]
		}
	}
	if end < len(text) {
		if i := strings.LastIndexFunc(cut, unicode.IsSpace); i >= 0 && len(cut)-i < 20*utf8.UTFMax {
			cut = cut[:i]
		}
	}
	out := strings.Join(strings.Fields(cut), " ")
	if start > 0 {
		out = "…" + out
	}
	if end < len(text) {
		out += "…"
	}
	return out
}

// find returns the index in text of the first word that starts with term,
// or of term anywhere when inside is set; -1 if there is none.
func find(text, term []rune, inside bool) int {
	if len(term) == 0 {
		return -1
	}
	for i := 0; i+len(term) <= len(text); i++ {
		if !inside && i > 0 && isWord(text[i-1]) {
			continue
		}
		if slices.Equal(text[i:i+len(term)], term) {
			return i
		}
	}
	return -1
}

func isWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r)
}

// TextHit is one line of a text that has every word of a query.
type TextHit struct {
	Offset  int // the line's byte offset in the text
	Line    int // from 1
	Snippet string
}

// SearchText finds the lines of text with a word that starts with each
// word of query, compared as history search compares (SPEC 2.3). It
// returns the first max hits and how many lines match in all. search_ref
// uses it on stored pages and tool outputs.
func SearchText(text, query string, max int) ([]TextHit, int, error) {
	terms, err := queryTerms(query)
	if err != nil || len(terms) == 0 {
		return nil, 0, err
	}
	want := foldedTerms(terms)
	forms := make([][][]rune, len(terms))
	for i, w := range want {
		for _, f := range w {
			forms[i] = append(forms[i], []rune(string(f)))
		}
	}
	var hits []TextHit
	total, off, n := 0, 0, 0
	var folded []byte
	for line := range strings.Lines(text) {
		n++
		start := off
		off += len(line)
		// The byte check is fast and finds every match; the word check
		// then drops matches inside words.
		if folded = foldBytes(folded[:0], line); !containsAll(folded, want) || !startsWords(fold(line, nil), forms) {
			continue
		}
		total++
		if len(hits) < max {
			hits = append(hits, TextHit{Offset: start, Line: n, Snippet: snippet(line, terms)})
		}
	}
	return hits, total, nil
}

// startsWords reports whether every term has a form that starts a word in
// text.
func startsWords(text []rune, forms [][][]rune) bool {
	for _, fs := range forms {
		if !slices.ContainsFunc(fs, func(f []rune) bool { return find(text, f, false) >= 0 }) {
			return false
		}
	}
	return true
}
