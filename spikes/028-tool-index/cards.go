package main

// History cards (SPEC 3.6, condition 3): in a turn after a cut, Go looks
// for older turns of the chat that share rare words with the user's
// message, and adds the best one or two to that message as a card: the
// turn's request, its calls and its answer. They are added to the recent
// turns, never in their place. The search is the store's own message
// search (search_history's), one word at a time, scored by idf over the
// chat's turns; a word that half the turns have says nothing and is left
// out. A card's turn must score better than every turn in the window, and
// a second card at least half the first. No word list,
// so it works the same in every language the tokenizer splits.

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/project"
	"github.com/amir-saatchi/jenab/internal/store"
)

const (
	maxCards     = 2   // cards on one message
	minWords     = 2   // a turn shares at least this many of the message's words
	minScore     = 4.0 // and their idf adds up to this: three words that one or two turns have, say
	cardArgsClip = 700 // bytes of a call's arguments in a card
	cardTextClip = 600 // bytes of the request or the answer in a card
)

// cards gives a run's requests their history cards through Deps.Earlier.
// A turn's cards are found once, at its first request, and kept, so the
// turn's requests stay append-only and the cache holds.
type cards struct {
	pm  *project.Manager
	pid id.Project // set once the project exists

	mu   sync.Mutex
	memo map[cardKey]chat.Part // a turn's card part; Kind "" for none
	got  map[int][]int         // the earlier turns carded, by turn
}

type cardKey struct {
	chat id.Chat
	turn int
}

func newCards(pm *project.Manager) *cards {
	return &cards{pm: pm, memo: map[cardKey]chat.Part{}, got: map[int][]int{}}
}

// earlier is Deps.Earlier: ms with the card added to turn cur's first user
// message, when the window starts after turn 1. The store is not changed.
func (c *cards) earlier(ms []chat.Message, cur int) []chat.Message {
	start, at := cur, -1
	for i, m := range ms {
		start = min(start, m.Turn)
		if at < 0 && m.Turn == cur && m.Role == chat.RoleUser {
			at = i
		}
	}
	if at < 0 || start <= 1 {
		return ms
	}
	k := cardKey{ms[at].Chat, cur}
	c.mu.Lock()
	p, ok := c.memo[k]
	c.mu.Unlock()
	if !ok {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		turns, card := c.find(ctx, ms[at].Chat, textOf(ms[at]), start, cur)
		cancel()
		if card != "" {
			p = chat.Part{Kind: chat.PartText, Text: &chat.Text{Text: card}}
		}
		c.mu.Lock()
		c.memo[k], c.got[cur] = p, turns
		c.mu.Unlock()
	}
	if p.Kind == "" {
		return ms
	}
	out := slices.Clone(ms)
	m := out[at]
	m.Parts = append(slices.Clone(m.Parts), p)
	out[at] = m
	return out
}

// carded is the earlier turns carded at turn n.
func (c *cards) carded(n int) []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.got[n]
}

// find scores the turns before start by turn cur's words and writes
// the card of the best ones.
func (c *cards) find(ctx context.Context, ch id.Chat, text string, start, cur int) ([]int, string) {
	if c.pid == "" {
		return nil, ""
	}
	p, err := c.pm.Open(ctx, c.pid)
	if err != nil {
		return nil, ""
	}
	defer p.Release()
	all := float64(cur - 1) // the chat's earlier turns, in the window too
	score := map[int]float64{}
	shared := map[int]int{}
	for _, w := range words(text) {
		res, err := p.Chats.Search(ctx, store.SearchReq{Query: w, Chat: ch, Limit: 100})
		if err != nil {
			continue
		}
		in := map[int]bool{}
		for _, h := range res.Hits {
			if h.Turn < cur {
				in[h.Turn] = true
			}
		}
		df := float64(len(in))
		if df == 0 || df > all/2 {
			continue
		}
		idf := math.Log(all / df)
		for t := range in {
			score[t] += idf
			shared[t]++
		}
	}
	// The best turn in the window: when it matches as well as any older
	// turn, the model has what it needs and no card is added.
	inWindow := 0.0
	for t, s := range score {
		if t >= start && shared[t] >= minWords {
			inWindow = max(inWindow, s)
		}
	}
	var best []int
	top := 0.0
	for t, s := range score {
		if t < start && shared[t] >= minWords && s >= minScore && s > inWindow {
			best = append(best, t)
			top = max(top, s)
		}
	}
	best = slices.DeleteFunc(best, func(t int) bool { return score[t] < top/2 })
	slices.SortFunc(best, func(a, b int) int {
		if score[a] != score[b] {
			if score[a] > score[b] {
				return -1
			}
			return 1
		}
		return b - a // the newer first
	})
	if len(best) > maxCards {
		best = best[:maxCards]
	}
	slices.Sort(best)
	var b strings.Builder
	for _, t := range best {
		ms, _, err := p.Chats.Messages(ctx, ch, t, t)
		if err != nil || len(ms) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(card(t, ms))
	}
	return best, b.String()
}

// card is one earlier turn as the model sees it in a card. The calls show
// the tool they ran, also through call_tool.
func card(n int, ms []chat.Message) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[From turn %d of this chat, before the recent turns:", n)
	answer := ""
	for _, m := range ms {
		switch m.Role {
		case chat.RoleUser:
			if t := textOf(m); t != "" {
				b.WriteString("\nUser: " + clip(t, cardTextClip))
			}
		case chat.RoleAssistant:
			for _, p := range m.Parts {
				switch {
				case p.ToolCall != nil:
					name, args := unwrap(p.ToolCall.Name, string(p.ToolCall.Args))
					b.WriteString("\nCall: " + name + " " + clip(args, cardArgsClip))
				case p.Text != nil && strings.TrimSpace(p.Text.Text) != "":
					answer = p.Text.Text
				}
			}
		}
	}
	if answer != "" {
		b.WriteString("\nAnswer: " + clip(answer, cardTextClip))
	}
	b.WriteString("]")
	return b.String()
}

func textOf(m chat.Message) string {
	var parts []string
	for _, p := range m.Parts {
		if p.Text != nil && strings.TrimSpace(p.Text.Text) != "" {
			parts = append(parts, p.Text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// words are the message's words as the store's tokenizer splits them:
// letters and digits, lower case, once each. One-letter words are left out.
func words(text string) []string {
	var out []string
	for f := range strings.FieldsFuncSeq(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.Is(unicode.Mn, r)
	}) {
		if len([]rune(f)) > 1 && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}
