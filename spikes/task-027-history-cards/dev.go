package main

// The dev set the card thresholds are set on: SPIKE-028's five long chats
// (H1-H5), each in a project of its own, as that spike ran them. The
// history-cards set is held out: it is scored only with the thresholds
// frozen.

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/amir-saatchi/jenab/internal/agent"
	"github.com/amir-saatchi/jenab/internal/chat"
	"github.com/amir-saatchi/jenab/internal/id"
	"github.com/amir-saatchi/jenab/internal/scenario"
	"github.com/amir-saatchi/jenab/internal/store"
)

var (
	devHistory = filepath.Join("..", "028-tool-index", "scenarios", "long-history")
	devSet     = filepath.Join("..", "028-tool-index", "scenarios", "long-chats")
)

// historyMin is the app's default history_min_turns: the turns a cut
// keeps before the current one.
const historyMin = 3

// devChat is one of SPIKE-028's long chats: its turns, the turn its
// follow-up needs and the follow-up.
type devChat struct {
	id    string
	turns []seedTurn
	at    int
	text  string
}

func loadDev() ([]devChat, error) {
	var filler struct {
		Turns []seedTurn `yaml:"turns"`
	}
	if err := readYAML(filepath.Join(devHistory, "_filler.yaml"), &filler); err != nil {
		return nil, err
	}
	s, err := scenario.Load(devSet)
	if err != nil {
		return nil, err
	}
	var out []devChat
	for _, sc := range s.Scenarios {
		var own struct {
			At   int      `yaml:"at"`
			Turn seedTurn `yaml:"turn"`
		}
		if err := readYAML(filepath.Join(devHistory, sc.ID+".yaml"), &own); err != nil {
			return nil, err
		}
		d := devChat{id: sc.ID, at: own.At, text: sc.Messages[0].Text}
		d.turns = append(d.turns, filler.Turns[:own.At-1]...)
		d.turns = append(d.turns, own.Turn)
		d.turns = append(d.turns, filler.Turns[own.At-1:]...)
		out = append(out, d)
	}
	return out, nil
}

func readYAML(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// params are the thresholds the dev sweep tries.
type params struct{ minScore, margin float64 }

// pick is agent.PickCards with other thresholds (cardMinWords and
// maxCards as the app's).
func pick(scores []agent.CardScore, start int, p params) []int {
	inWindow := 0.0
	for _, s := range scores {
		if s.Turn >= start && s.Words >= 2 {
			inWindow = max(inWindow, s.Score)
		}
	}
	var best []agent.CardScore
	for _, s := range scores {
		if s.Turn < start && s.Words >= 2 && s.Score >= p.minScore && s.Score > p.margin*inWindow {
			best = append(best, s)
		}
	}
	slices.SortFunc(best, func(a, b agent.CardScore) int {
		switch {
		case a.Score > b.Score:
			return -1
		case a.Score < b.Score:
			return 1
		}
		return b.Turn - a.Turn
	})
	var out []int
	for _, s := range best {
		if len(out) == 2 || s.Score < best[0].Score/2 {
			break
		}
		out = append(out, s.Turn)
	}
	slices.Sort(out)
	return out
}

// runDev scores the dev chats and writes the sweep. work holds the
// chats' databases.
func runDev(ctx context.Context, work string, w io.Writer) error {
	chats, err := loadDev()
	if err != nil {
		return err
	}
	type scored struct {
		devChat
		scores []agent.CardScore
		start  int
	}
	var all []scored
	for _, d := range chats {
		dir := filepath.Join(work, d.id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		db, err := store.OpenChats(ctx, dir)
		if err != nil {
			return err
		}
		c, err := db.CreateChat(ctx, id.SourceUser, chat.Chat{Title: d.id})
		if err == nil {
			err = seed(ctx, db, c.ID, d.turns)
		}
		var scores []agent.CardScore
		cur := len(d.turns) + 1
		if err == nil {
			scores, err = agent.CardScores(ctx, db, c.ID, d.text, cur)
		}
		db.Close(ctx)
		if err != nil {
			return fmt.Errorf("%s: %w", d.id, err)
		}
		all = append(all, scored{d, scores, cur - historyMin})
	}
	fmt.Fprintf(w, "# TASK-027 dev sweep: SPIKE-028's long chats\n\n")
	fmt.Fprintf(w, "Each chat has %d earlier turns; the follow-up's first request cuts, so the window starts at turn %d.\n\n", len(all[0].turns), all[0].start)
	for _, s := range all {
		where := "past the cut"
		if s.at >= s.start {
			where = "in the window"
		}
		fmt.Fprintf(w, "- %s (needs turn %d, %s): %q\n  scores:", s.id, s.at, where, s.text)
		for _, cs := range s.scores {
			fmt.Fprintf(w, " t%d %.2f/%dw", cs.Turn, cs.Score, cs.Words)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "\n| min score | margin | needed turn carded | wrong cards | cards on the control |\n|---|---|---|---|---|\n")
	for _, ms := range []float64{2, 3, 4, 5, 6, 8} {
		for _, mg := range []float64{1, 1.25, 1.5, 2} {
			p := params{ms, mg}
			hits, past, wrong, ctrl := 0, 0, 0, 0
			for _, s := range all {
				got := pick(s.scores, s.start, p)
				if s.at >= s.start {
					ctrl += len(got)
					continue
				}
				past++
				if slices.Contains(got, s.at) {
					hits++
				}
				wrong += len(slices.DeleteFunc(slices.Clone(got), func(t int) bool { return t == s.at }))
			}
			fmt.Fprintf(w, "| %.1f | %.2f | %d/%d | %d | %d |\n", ms, mg, hits, past, wrong, ctrl)
		}
	}
	// The app's own thresholds give what the sweep's row does.
	var app []string
	for _, s := range all {
		app = append(app, fmt.Sprintf("%s %v", s.id, agent.PickCards(s.scores, s.start)))
	}
	fmt.Fprintf(w, "\nThe app's PickCards: %s.\n", strings.Join(app, "; "))
	return nil
}
