package main

// The card plan: what the app cards for each follow-up, worked out
// offline as it does at the follow-up's first request. The project has
// all eight chats; the follow-up is stored as turn 23 of its chat before
// the cards are found, as in a run. It is deterministic, so the live
// runs skip the cards condition of a follow-up that gets no card: its
// requests are the same as today's.

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/scenario"
	"github.com/amir-saatchi/jenab/internal/store"
)

// setDir is the set the harness runs.
var setDir = filepath.Join("scenarios", "history-cards")

// chatIDs are the set's chats, in the order the harness writes them.
var chatIDs = []string{"C1", "C2", "C3", "C4", "C5", "C6", "C7", "C8"}

// plan is one follow-up's cards.
type plan struct {
	Scenario string
	Chat     id.Chat // in the plan's database
	Kind     string  // repeat, paraphrase or control
	Needed   int     // the turn the follow-up needs; 0 for none
	Text     string
	Cards    []int
	Scores   []agent.CardScore
	Shared   map[int][]string // the counted words each card's turn, and the needed turn, share with it
}

// past reports whether the needed turn is before the window.
func (p plan) past() bool { return p.Needed > 0 && p.Needed < followUp-historyMin }

// followUp is the follow-up's turn: every chat has 22 earlier turns.
const followUp = 23

var reNeeded = regexp.MustCompile(`^Turn (\d+),`)

// loadSet reads the set and every chat's turns.
func loadSet() (*scenario.Set, map[string][]seedTurn, error) {
	s, err := scenario.Load(setDir)
	if err != nil {
		return nil, nil, err
	}
	turns := map[string][]seedTurn{}
	for _, c := range chatIDs {
		if turns[c], err = loadChat(setDir, c); err != nil {
			return nil, nil, err
		}
		if len(turns[c]) != followUp-1 {
			return nil, nil, fmt.Errorf("%s has %d turns, want %d", c, len(turns[c]), followUp-1)
		}
	}
	return s, turns, nil
}

// seedProject writes the eight chats into a project's chats: own's turns
// into c, the chat under test, and each other chat into a new one.
func seedProject(ctx context.Context, db *store.ChatsDB, c id.Chat, own string, turns map[string][]seedTurn) error {
	for _, name := range chatIDs {
		ch := c
		if name != own {
			o, err := db.CreateChat(ctx, id.SourceUser, chat.Chat{Title: "Chat " + name})
			if err != nil {
				return err
			}
			ch = o.ID
		}
		if err := seed(ctx, db, ch, turns[name]); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// planCards works out every follow-up's cards, with databases in work.
func planCards(ctx context.Context, work string) ([]plan, error) {
	s, turns, err := loadSet()
	if err != nil {
		return nil, err
	}
	var out []plan
	for _, sc := range s.Scenarios {
		p := plan{Scenario: sc.ID, Text: sc.Messages[0].Text}
		if len(sc.Tests) > 0 {
			p.Kind = sc.Tests[0]
		}
		if m := reNeeded.FindStringSubmatch(sc.Why); m != nil {
			p.Needed, _ = strconv.Atoi(m[1])
		}
		dir := filepath.Join(work, sc.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		db, err := store.OpenChats(ctx, dir)
		if err != nil {
			return nil, err
		}
		err = func() error {
			defer db.Close(ctx)
			c, err := db.CreateChat(ctx, id.SourceUser, chat.Chat{Title: sc.ID})
			if err != nil {
				return err
			}
			p.Chat = c.ID
			if err := seedProject(ctx, db, c.ID, chatOf(sc.ID), turns); err != nil {
				return err
			}
			if _, _, err := db.AppendMessage(ctx, chat.Message{Chat: c.ID, Turn: followUp, Role: chat.RoleUser,
				Parts: []chat.Part{{Kind: chat.PartText, Text: &chat.Text{Text: p.Text}}}}); err != nil {
				return err
			}
			if p.Scores, err = agent.CardScores(ctx, db, c.ID, p.Text, followUp); err != nil {
				return err
			}
			p.Cards = agent.PickCards(p.Scores, followUp-historyMin)
			p.Shared, err = sharedWords(ctx, db, c.ID, p.Text, append(slices.Clone(p.Cards), p.Needed))
			return err
		}()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", sc.ID, err)
		}
		out = append(out, p)
	}
	return out, nil
}

// sharedWords are the words of text that count for each of turns, as
// agent.CardScores counts them: in fewer than 1/8 of the project's text
// parts (at least 160), and in at most 1/4 of the chat's earlier turns.
func sharedWords(ctx context.Context, db *store.ChatsDB, ch id.Chat, text string, turns []int) (map[int][]string, error) {
	parts, err := db.TextParts(ctx)
	if err != nil {
		return nil, err
	}
	limit := (max(parts, 160) + 7) / 8
	out := map[int][]string{}
	for _, w := range words(text) {
		n, err := db.PartsWith(ctx, w, limit)
		if err != nil {
			return nil, err
		}
		in, err := db.TurnsWith(ctx, ch, w, followUp)
		if err != nil {
			return nil, err
		}
		if n >= limit || len(in) == 0 || len(in)*4 > followUp-1 {
			continue
		}
		for _, t := range turns {
			if t > 0 && slices.Contains(in, t) && !slices.Contains(out[t], w) {
				out[t] = append(out[t], w)
			}
		}
	}
	return out, nil
}

// words are text's words as the app's cards read them (agent.cardWords).
func words(text string) []string {
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

// verdict is what a follow-up's cards mean for its test.
func (p plan) verdict() string {
	switch {
	case p.Kind == "control" && len(p.Cards) > 0:
		return "false card"
	case p.Kind == "control":
		return "no card (right)"
	case !p.past():
		return "needed turn in the window"
	case slices.Contains(p.Cards, p.Needed) && len(p.Cards) == 1:
		return "needed turn carded"
	case slices.Contains(p.Cards, p.Needed):
		return "needed turn carded, with another"
	case len(p.Cards) > 0:
		return "wrong card"
	}
	return "no card"
}

// writePlan writes the plan as Markdown.
func writePlan(w io.Writer, ps []plan) {
	fmt.Fprintf(w, "# TASK-027: the history cards of the set's follow-ups\n\n")
	fmt.Fprintf(w, "Worked out offline as the app does at the follow-up's first request: the project has all eight chats, the follow-up is turn %d, the first request cuts and the window starts at turn %d. Thresholds as frozen on SPIKE-028's chats (min score 5.0, margin 1.25).\n\n", followUp, followUp-historyMin)
	fmt.Fprintf(w, "| follow-up | test | needs | cards | verdict | best scores (turn score/words) | counted words shared |\n|---|---|---|---|---|---|---|\n")
	count := map[string]int{}
	for _, p := range ps {
		needs := "none"
		if p.Needed > 0 {
			needs = fmt.Sprintf("turn %d", p.Needed)
		}
		cards := "none"
		if len(p.Cards) > 0 {
			cards = strings.Trim(strings.Join(strings.Fields(fmt.Sprint(p.Cards)), ", "), "[]")
		}
		top := slices.Clone(p.Scores)
		slices.SortFunc(top, func(a, b agent.CardScore) int {
			switch {
			case a.Score > b.Score:
				return -1
			case a.Score < b.Score:
				return 1
			}
			return a.Turn - b.Turn
		})
		var best []string
		for _, s := range top[:min(4, len(top))] {
			best = append(best, fmt.Sprintf("t%d %.1f/%d", s.Turn, s.Score, s.Words))
		}
		var shared []string
		for _, t := range slices.Sorted(maps.Keys(p.Shared)) {
			shared = append(shared, fmt.Sprintf("t%d: %s", t, strings.Join(p.Shared[t], " ")))
		}
		v := p.verdict()
		count[p.Kind+": "+v]++
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s | %s |\n", p.Scenario, p.Kind, needs, cards, v, strings.Join(best, ", "), strings.Join(shared, "; "))
	}
	keys := make([]string, 0, len(count))
	for k := range count {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	fmt.Fprintf(w, "\nTotals:\n\n")
	for _, k := range keys {
		fmt.Fprintf(w, "- %s: %d\n", k, count[k])
	}
}
