package main

// Candidate rules for a revision of the card rule, scored offline on the
// history-cards set (now seen: the dev set of a revision) and on
// SPIKE-028's long chats. Nothing here is the app's: each rule is worked
// out from each word's turns in the chat and its text parts in the
// project, as the store counts them.

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/store"
)

// wstat is one word of a follow-up: the chat's earlier turns with it and
// the project's text parts with it.
type wstat struct {
	w     string
	turns []int
	parts int
}

// probe is one follow-up with its words' counts.
type probe struct {
	id, kind string
	needed   int // 0 for none
	start    int // the window's first turn
	all      int // the chat's earlier turns
	parts    int // the project's text parts
	words    []wstat
}

func (p probe) past() bool { return p.needed > 0 && p.needed < p.start }

func stats(ctx context.Context, db *store.ChatsDB, ch id.Chat, text string, cur int) ([]wstat, int, error) {
	parts, err := db.TextParts(ctx)
	if err != nil {
		return nil, 0, err
	}
	var out []wstat
	for _, w := range words(text) {
		n, err := db.PartsWith(ctx, w, 1<<30)
		if err != nil {
			return nil, 0, err
		}
		ts, err := db.TurnsWith(ctx, ch, w, cur)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, wstat{w, ts, n})
	}
	return out, parts, nil
}

// probeSet is the history-cards set's follow-ups.
func probeSet(ctx context.Context, work string) ([]probe, error) {
	ps, err := planCards(ctx, work) // writes each follow-up's project
	if err != nil {
		return nil, err
	}
	var out []probe
	for _, p := range ps {
		db, err := store.OpenChats(ctx, filepath.Join(work, p.Scenario))
		if err != nil {
			return nil, err
		}
		ws, parts, err := stats(ctx, db, p.Chat, p.Text, followUp)
		db.Close(ctx)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.Scenario, err)
		}
		out = append(out, probe{p.Scenario, p.Kind, p.Needed, followUp - historyMin, followUp - 1, parts, ws})
	}
	return out, nil
}

// probeDev is SPIKE-028's long chats, each in a project of its own.
func probeDev(ctx context.Context, work string) ([]probe, error) {
	chats, err := loadDev()
	if err != nil {
		return nil, err
	}
	var out []probe
	for _, d := range chats {
		dir := filepath.Join(work, d.id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		db, err := store.OpenChats(ctx, dir)
		if err != nil {
			return nil, err
		}
		cur := len(d.turns) + 1
		c, err := db.CreateChat(ctx, id.SourceUser, chat.Chat{Title: d.id})
		var ws []wstat
		var parts int
		if err == nil {
			err = seed(ctx, db, c.ID, d.turns)
		}
		if err == nil {
			ws, parts, err = stats(ctx, db, c.ID, d.text, cur)
		}
		db.Close(ctx)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", d.id, err)
		}
		kind := "repeat"
		if d.at >= cur-historyMin {
			kind = "control"
		}
		out = append(out, probe{d.id, kind, d.at, cur - historyMin, cur - 1, parts, ws})
	}
	return out, nil
}

// rule is one candidate.
type rule struct {
	name     string
	idf      string // "chat": log(turns/df) over the chat's turns; "project": log(parts/df) over the project's parts
	chatOff  bool   // no chat-common cut
	anchor   int    // > 0: a card's turn shares a word in at most this many of the chat's turns
	minWords int
	winWords int // a window turn counts against a card with this many words; 0 is minWords
	minScore float64
	margin   float64
	stop     []string // words never counted
}

var stopEN = strings.Fields(`a an the and or but if of to in on at by for with from as into about
	that this these those it its is are was were be been do does did have has had will would can could
	should what which who when where why how there here then than so not no yes all any some more most
	each per too also just again same other my me i you your we our us he she they them their his her
	make made show give get let one two`)

var stopFA = strings.Fields(`و در به از که را رو با برای این آن اون هم یا تا هر چه چی چطور کدام کدوم
	است هست بود شد می من تو ما شما او اونها آنها خود دارد داره دارم بکن بکنم کن بساز بده نشان همین همان اینجا
	یک دو هست نیست بله نه اگر اما ولی پس هنوز بعد قبل`)

func (r rule) score(p probe) map[int][2]float64 {
	limit := (max(p.parts, commonFloorH) + 7) / 8
	out := map[int][2]float64{} // score, words
	anchored := map[int]bool{}
	for _, w := range p.words {
		df := len(w.turns)
		if df == 0 || w.parts >= limit || slices.Contains(r.stop, w.w) || (!r.chatOff && df*4 > p.all) {
			continue
		}
		idf := math.Log(float64(p.all) / float64(df))
		if r.idf == "project" {
			idf = math.Log(float64(max(p.parts, commonFloorH)) / float64(w.parts))
		}
		for _, t := range w.turns {
			s := out[t]
			out[t] = [2]float64{s[0] + idf, s[1] + 1}
			if df <= r.anchor {
				anchored[t] = true
			}
		}
	}
	if r.anchor > 0 {
		for t := range out {
			if !anchored[t] {
				s := out[t]
				out[t] = [2]float64{s[0], -s[1]} // marked: no rare word
			}
		}
	}
	return out
}

const commonFloorH = 160

func (r rule) pick(p probe) []int {
	sc := r.score(p)
	inWindow := 0.0
	for t, s := range sc {
		if t >= p.start && math.Abs(s[1]) >= float64(cmp.Or(r.winWords, r.minWords)) {
			inWindow = max(inWindow, s[0])
		}
	}
	type ts struct {
		t int
		s float64
	}
	var best []ts
	for t, s := range sc {
		if t < p.start && s[1] >= float64(r.minWords) && s[0] >= r.minScore && s[0] > r.margin*inWindow {
			best = append(best, ts{t, s[0]})
		}
	}
	slices.SortFunc(best, func(a, b ts) int {
		switch {
		case a.s > b.s:
			return -1
		case a.s < b.s:
			return 1
		}
		return b.t - a.t
	})
	var out []int
	for _, b := range best {
		if len(out) == 2 || b.s < best[0].s/2 {
			break
		}
		out = append(out, b.t)
	}
	slices.Sort(out)
	return out
}

// tally is a rule's result on a set.
type tally struct {
	hits, past, extra, wrong, missed int // past: follow-ups whose needed turn is past the cut
	ctrl, falseCtrl                  int
	byKind                           map[string][2]int // hits, past
	falseIDs, wrongIDs               []string
}

func (r rule) run(ps []probe) tally {
	t := tally{byKind: map[string][2]int{}}
	for _, p := range ps {
		got := r.pick(p)
		if !p.past() {
			t.ctrl++
			if len(got) > 0 {
				t.falseCtrl++
				t.falseIDs = append(t.falseIDs, fmt.Sprintf("%s%v", p.id, got))
			}
			continue
		}
		t.past++
		k := t.byKind[p.kind]
		k[1]++
		switch {
		case slices.Contains(got, p.needed):
			t.hits++
			k[0]++
			if len(got) > 1 {
				t.extra++
			}
		case len(got) > 0:
			t.wrong++
			t.wrongIDs = append(t.wrongIDs, fmt.Sprintf("%s%v", p.id, got))
		default:
			t.missed++
		}
		t.byKind[p.kind] = k
	}
	return t
}

func candidates() []rule {
	var out []rule
	for _, ms := range []float64{4, 5, 6} {
		out = append(out,
			rule{name: "frozen", idf: "chat", minWords: 2, minScore: ms, margin: 1.25},
			rule{name: "anchor<=2", idf: "chat", anchor: 2, minWords: 2, minScore: ms, margin: 1.25},
			rule{name: "anchor<=1", idf: "chat", anchor: 1, minWords: 2, minScore: ms, margin: 1.25},
			rule{name: "anchor<=2, 3 words", idf: "chat", anchor: 2, minWords: 3, minScore: ms, margin: 1.25},
			rule{name: "project idf", idf: "project", minWords: 2, minScore: ms, margin: 1.25},
			rule{name: "project idf, anchor<=2", idf: "project", anchor: 2, minWords: 2, minScore: ms, margin: 1.25},
			rule{name: "stop lists", idf: "chat", stop: append(slices.Clone(stopEN), stopFA...), minWords: 2, minScore: ms, margin: 1.25},
			rule{name: "stop lists, anchor<=2", idf: "chat", anchor: 2, stop: append(slices.Clone(stopEN), stopFA...), minWords: 2, minScore: ms, margin: 1.25},
			rule{name: "window 1 word", idf: "chat", minWords: 2, winWords: 1, minScore: ms, margin: 1.25},
			rule{name: "window 1 word, anchor<=1", idf: "chat", anchor: 1, minWords: 2, winWords: 1, minScore: ms, margin: 1.25},
			rule{name: "window 1 word, margin 2", idf: "chat", minWords: 2, winWords: 1, minScore: ms, margin: 2},
			rule{name: "window 1 word, anchor<=1, margin 2", idf: "chat", anchor: 1, minWords: 2, winWords: 1, minScore: ms, margin: 2},
			rule{name: "stop lists, window 1 word", idf: "chat", stop: append(slices.Clone(stopEN), stopFA...), minWords: 2, winWords: 1, minScore: ms, margin: 1.25},
		)
	}
	return out
}

// runRules writes each candidate's result on both sets.
func runRules(ctx context.Context, work string, w io.Writer) error {
	set, err := probeSet(ctx, filepath.Join(work, "set"))
	if err != nil {
		return err
	}
	dev, err := probeDev(ctx, filepath.Join(work, "dev"))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "# TASK-027: candidate card rules, offline\n\nThe history-cards set (24 follow-ups, 8 chats in one project) and SPIKE-028's long chats (H1-H5, one chat a project). Needed: the follow-ups whose needed turn is past the cut. Margin 1.25 unless named.\n\n")
	fmt.Fprintf(w, "| rule | min score | set: repeats | set: paraphrases | set: wrong only | set: extra card | set: false on controls | dev: needed | dev: wrong | dev: false on controls |\n|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range candidates() {
		s, d := r.run(set), r.run(dev)
		fmt.Fprintf(w, "| %s | %.0f | %d/%d | %d/%d | %d | %d | %d/%d | %d/%d | %d | %d/%d |\n", r.name, r.minScore,
			s.byKind["repeat"][0], s.byKind["repeat"][1], s.byKind["paraphrase"][0], s.byKind["paraphrase"][1],
			s.wrong, s.extra, s.falseCtrl, s.ctrl, d.hits, d.past, d.wrong, d.falseCtrl, d.ctrl)
	}
	fmt.Fprintf(w, "\n## Details at min score 5\n\n")
	for _, r := range candidates() {
		if r.minScore != 5 {
			continue
		}
		s := r.run(set)
		fmt.Fprintf(w, "- %s: false cards %s; wrong cards %s\n", r.name, strings.Join(s.falseIDs, " "), strings.Join(s.wrongIDs, " "))
	}
	fmt.Fprintf(w, "\n## Words of the set's follow-ups\n\nEach counted word (not project-common): the chat's earlier turns with it. Project parts in brackets.\n\n")
	for _, p := range set {
		var ws []string
		for _, x := range p.words {
			ws = append(ws, fmt.Sprintf("%s %v[%d]", x.w, x.turns, x.parts))
		}
		fmt.Fprintf(w, "- %s (needs %d): %s\n", p.id, p.needed, strings.Join(ws, ", "))
	}
	return nil
}
