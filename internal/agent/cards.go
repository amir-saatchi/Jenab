package agent

// History cards (SPEC 3.6, TASK-027): after a cut, a turn whose message
// shares rare words with a turn before the window gets that turn as a
// card: its request, its calls and its answer, added to the message in
// the request and never stored. The words are weighed by idf over the
// chat's earlier turns. A word common in the project's chats, or in the
// chat's own turns, counts for nothing, so no word list is needed and it
// works the same in every language. A turn's cards are found once, at its
// first request, and kept while the turn is in the window, so requests
// stay append-only (3.1).

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

const (
	maxCards     = 2    // cards on one message
	cardMinWords = 2    // a carded turn shares at least this many counted words
	cardMinScore = 5.0  // and their idf adds up to this
	cardMargin   = 1.25 // times the best turn in the window
	// The two thresholds are the middle of the range that cards the
	// needed turn of SPIKE-028's long chats with no wrong card (TASK-027,
	// spikes/task-027-history-cards/results/dev-sweep.md).
	commonShare  = 8    // a word in 1/8 of the project's text parts is common
	commonFloor  = 160  // counted against at least this many parts, so a young project has a floor
	chatShare    = 4    // a word in more than 1/4 of the chat's earlier turns is common
	cardTextClip = 1000 // bytes of the request or the answer in a card
	cardArgsClip = 2000 // bytes of a call's arguments
	cardClip     = 4000 // bytes of one card
	cardsClip    = 6000 // bytes of a message's cards
	// cardsTimeout bounds the search for a turn's cards: a slower database
	// leaves the turn without cards.
	cardsTimeout = 300 * time.Millisecond
	// A project's count of text parts is a full scan, so it is reused for
	// partsFresh, and counted with partsTimeout of its own.
	partsFresh   = 10 * time.Minute
	partsTimeout = 2 * time.Second
)

// historyTools are the calls a card leaves out: the history tools, which
// only looked for what the card shows.
var historyTools = []string{"search_history", "read_messages"}

// CardScore is an earlier turn's score for a message: the idf of the
// counted words it shares, and how many there are.
type CardScore struct {
	Turn  int
	Score float64
	Words int
}

// CardScores scores chat ch's turns before cur by the words of text.
// Harnesses use it to report the scores; the app's cards come from
// PickCards.
func CardScores(ctx context.Context, c *store.ChatsDB, ch id.Chat, text string, cur int) ([]CardScore, error) {
	parts, err := c.TextParts(ctx)
	if err != nil {
		return nil, err
	}
	return cardScores(ctx, c, parts, ch, text, cur)
}

// cardScores is CardScores in a project with parts text parts.
func cardScores(ctx context.Context, c *store.ChatsDB, parts int, ch id.Chat, text string, cur int) ([]CardScore, error) {
	all := cur - 1
	if all < 1 {
		return nil, nil
	}
	limit := (max(parts, commonFloor) + commonShare - 1) / commonShare
	by := map[int]*CardScore{}
	for _, w := range cardWords(text) {
		n, err := c.PartsWith(ctx, w, limit)
		if err != nil {
			return nil, err
		}
		if n >= limit {
			continue
		}
		turns, err := c.TurnsWith(ctx, ch, w, cur)
		if err != nil {
			return nil, err
		}
		df := len(turns)
		if df == 0 || df*chatShare > all {
			continue
		}
		idf := math.Log(float64(all) / float64(df))
		for _, t := range turns {
			s := by[t]
			if s == nil {
				s = &CardScore{Turn: t}
				by[t] = s
			}
			s.Score += idf
			s.Words++
		}
	}
	out := make([]CardScore, 0, len(by))
	for _, s := range by {
		out = append(out, *s)
	}
	slices.SortFunc(out, func(a, b CardScore) int { return a.Turn - b.Turn })
	return out, nil
}

// PickCards is the turns before start that get cards, oldest first. A
// card's turn shares cardMinWords counted words, scores cardMinScore, and
// beats the best turn in the window by cardMargin: when a turn there
// matches as well, the model has what it needs. A second card scores at
// least half the first.
func PickCards(scores []CardScore, start int) []int {
	inWindow := 0.0
	for _, s := range scores {
		if s.Turn >= start && s.Words >= cardMinWords {
			inWindow = max(inWindow, s.Score)
		}
	}
	var best []CardScore
	for _, s := range scores {
		if s.Turn < start && s.Words >= cardMinWords && s.Score >= cardMinScore && s.Score > cardMargin*inWindow {
			best = append(best, s)
		}
	}
	slices.SortFunc(best, func(a, b CardScore) int {
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		return b.Turn - a.Turn // the newer first
	})
	var out []int
	for _, s := range best {
		if len(out) == maxCards || s.Score < best[0].Score/2 {
			break
		}
		out = append(out, s.Turn)
	}
	slices.Sort(out)
	return out
}

// cardWords are text's words as history search matches them: normalized
// (SPEC 2.3), lower case, letters, digits and marks, once each, at most
// store.MaxSearchTerms. One-letter words are left out.
func cardWords(text string) []string {
	var out []string
	for f := range strings.FieldsFuncSeq(strings.ToLower(store.Normalize(text)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.Is(unicode.Mn, r)
	}) {
		if utf8.RuneCountInString(f) > 1 && !slices.Contains(out, f) {
			if out = append(out, f); len(out) == store.MaxSearchTerms {
				break
			}
		}
	}
	return out
}

// findCards finds turn t's cards once, at its first request after a cut
// has left turns out of the window, from its first user message in ms. A
// turn a card in the window already shows gets no card again, and neither
// does a turn that was undone, whose answer may name what is gone. A
// carded turn that read outside data, or whose own cards showed such a
// turn, marks this turn as if it read it too: the card's answer can repeat
// a planted line (8.8, 3.3). The mark is also stored, as a notice nobody
// reads, so it outlives a restart. A failure, or a search slower than
// cardsTimeout, is logged and leaves the turn without cards.
func (r *runner) findCards(ctx context.Context, t *turn, ms []chat.Message) {
	w := &r.cs.win
	if r.o.d.NoHistoryCards || w.start <= 1 {
		return
	}
	if _, ok := w.cards[t.n]; ok {
		return
	}
	if w.cards == nil {
		w.cards = map[int]chat.Part{}
	}
	w.cards[t.n] = chat.Part{} // none, until found
	i := slices.IndexFunc(ms, func(m chat.Message) bool { return m.Turn == t.n && m.Role == chat.RoleUser })
	if i < 0 {
		return
	}
	text := messageText(ms[i])
	if text == "" {
		return
	}
	parts, err := r.o.textParts(ctx, r.cs.key.p, r.p.Chats)
	if err != nil {
		r.cardsFailed(ctx, err)
		return
	}
	qctx, cancel := context.WithTimeout(ctx, cardsTimeout)
	defer cancel()
	scores, err := cardScores(qctx, r.p.Chats, parts, t.ch.ID, text, t.n)
	if err != nil {
		r.cardsFailed(ctx, err)
		return
	}
	var b strings.Builder
	var shown []int
	outside := false
	for _, n := range PickCards(scores, w.start) {
		if w.shows(n) {
			continue
		}
		old, _, err := r.p.Chats.Messages(qctx, t.ch.ID, n, n)
		undone := false
		if err == nil && len(old) > 0 {
			undone, err = r.undone(qctx, t.ch.ID, n)
		}
		if err != nil {
			r.cardsFailed(ctx, err)
			return
		}
		room := min(cardClip, cardsClip-b.Len()-2)
		if len(old) == 0 || undone || room < 200 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(historyCard(t.ch.ID, n, old, room))
		shown = append(shown, n)
		outside = outside || r.o.untrustedTurn(old) > 0
	}
	if b.Len() == 0 {
		return
	}
	w.cards[t.n] = chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: b.String()}}
	if w.carded == nil {
		w.carded = map[int][]int{}
	}
	w.carded[t.n] = shown
	if outside {
		r.readOutside(t.n)
		text := fmt.Sprintf("[turn %d's history cards show turns that read outside data]", t.n)
		if err := r.notice(ctx, t, chat.NoticeCardOutside, text); err != nil {
			r.o.d.Log.Warn("agent: the mark of history cards with outside data", "project", r.cs.key.p, "chat", r.cs.key.c, "turn", t.n, "err", err)
		}
	}
}

// shows reports whether a card in the window shows turn n.
func (w *window) shows(n int) bool {
	for _, ts := range w.carded {
		if slices.Contains(ts, n) {
			return true
		}
	}
	return false
}

// undone reports whether turn n of chat ch is undone, or partly undone:
// Redo turn would put something back.
func (r *runner) undone(ctx context.Context, ch id.Chat, n int) (bool, error) {
	srcs, err := turnSources(ctx, r.p, ch, n)
	if errors.Is(err, store.ErrNothingToUndo) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	s, err := turnState(ctx, r.p, n, srcs)
	return s.Redo.Changes > 0, err
}

// cardMark reports whether p is history cards' mark of outside data.
func cardMark(p chat.Part) bool { return p.Notice != nil && p.Notice.Kind == chat.NoticeCardOutside }

// partsCount is a project's count of text parts, and when it was counted.
type partsCount struct {
	n  int
	at time.Time
}

// textParts is project p's count of text parts, counted again once it is
// partsFresh old. The count only sets how common a word must be to count
// for nothing, so a count a few minutes old does.
func (o *Orchestrator) textParts(ctx context.Context, p id.Project, c *store.ChatsDB) (int, error) {
	o.mu.Lock()
	pc, ok := o.parts[p]
	o.mu.Unlock()
	if ok && time.Since(pc.at) < partsFresh {
		return pc.n, nil
	}
	ctx, cancel := context.WithTimeout(ctx, partsTimeout)
	defer cancel()
	n, err := c.TextParts(ctx)
	if err != nil {
		return 0, err
	}
	o.mu.Lock()
	if o.parts == nil {
		o.parts = map[id.Project]partsCount{}
	}
	o.parts[p] = partsCount{n, time.Now()}
	o.mu.Unlock()
	return n, nil
}

func (r *runner) cardsFailed(ctx context.Context, err error) {
	switch {
	case ctx.Err() != nil:
	case errors.Is(err, context.DeadlineExceeded):
		r.o.d.Log.Warn("agent: no history cards: the search was too slow", "project", r.cs.key.p, "chat", r.cs.key.c, "err", err)
	default:
		r.o.d.Log.Error("agent: finding history cards", "project", r.cs.key.p, "chat", r.cs.key.c, "err", err)
	}
}

// withCards is ms with each turn's cards added to its first user message.
// ms is not changed.
func withCards(ms []chat.Message, cards map[int]chat.Part) []chat.Message {
	if len(cards) == 0 {
		return ms
	}
	var out []chat.Message
	done := map[int]bool{}
	for i, m := range ms {
		p := cards[m.Turn]
		if m.Role != chat.RoleUser || p.Text == nil || done[m.Turn] {
			continue
		}
		done[m.Turn] = true
		if out == nil {
			out = slices.Clone(ms)
		}
		m.Parts = append(slices.Clone(m.Parts), p)
		out[i] = m
	}
	if out == nil {
		return ms
	}
	return out
}

// dropCards forgets the cards of turns before the window, at a cut.
func (w *window) dropCards() {
	for n := range w.cards {
		if n < w.start {
			delete(w.cards, n)
			delete(w.carded, n)
		}
	}
}

// historyCard is turn n of chat ch, from its messages, as a card of at
// most room bytes: the user's text, the calls without their results, and
// the answer. Calls that don't fit are left out, with a line that says so.
// The card points to the turn with read_messages' exact arguments, the
// chat's id among them.
func historyCard(ch id.Chat, n int, ms []chat.Message, room int) string {
	var user []string
	var calls []string
	answer := ""
	for _, m := range ms {
		switch m.Role {
		case chat.RoleUser:
			if s := messageText(m); s != "" {
				user = append(user, s)
			}
		case chat.RoleAssistant:
			for _, p := range m.Parts {
				switch {
				case p.ToolCall != nil:
					if !slices.Contains(historyTools, p.ToolCall.Name) {
						calls = append(calls, "Call "+p.ToolCall.Name+" "+clipBytes(string(p.ToolCall.Args), cardArgsClip))
					}
				case p.Text != nil && strings.TrimSpace(p.Text.Text) != "":
					answer = strings.TrimSpace(p.Text.Text)
				}
			}
		}
	}
	head := fmt.Sprintf("[Turn %d of this chat, before the recent turns. read_messages(chat_id: %s, from: %d) has its tool results.", n, ch, n)
	const more = "\nCall … (more calls: read_messages has them)"
	// The request and the answer share what a small room leaves, after
	// the line that says calls are left out.
	fixed := len(head) + len("\nUser: \nAnswer: ]")
	if len(calls) > 0 {
		fixed += len(more)
	}
	text := min(cardTextClip, (room-fixed)/2)
	tail := "]"
	if answer != "" {
		tail = "\nAnswer: " + clipBytes(answer, text) + "]"
	}
	var b strings.Builder
	b.WriteString(head)
	if len(user) > 0 {
		b.WriteString("\nUser: " + clipBytes(strings.Join(user, "\n"), text))
	}
	for i, c := range calls {
		left := room - b.Len() - len(tail) - 1 // 1 for the line break
		if i == len(calls)-1 && len(c) <= left {
			b.WriteString("\n" + c)
			break
		}
		if left -= len(more); len(c) <= left {
			b.WriteString("\n" + c)
			continue
		}
		if left > 100 {
			b.WriteString("\n" + clipBytes(c, left))
		}
		b.WriteString(more)
		break
	}
	b.WriteString(tail)
	return b.String()
}

// messageText is m's text parts, joined.
func messageText(m chat.Message) string {
	var parts []string
	for _, p := range m.Parts {
		if p.Text != nil && strings.TrimSpace(p.Text.Text) != "" {
			parts = append(parts, strings.TrimSpace(p.Text.Text))
		}
	}
	return strings.Join(parts, "\n")
}

// clipBytes cuts s to at most n bytes, at a character, ending in "…" when
// it cut.
func clipBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	const dots = "…"
	if n < len(dots) {
		return ""
	}
	k := n - len(dots)
	for k > 0 && !utf8.RuneStart(s[k]) {
		k--
	}
	return s[:k] + dots
}
