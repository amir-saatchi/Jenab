package scenario

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/amir-saatchi/jenab/internal/store"
)

// historyTools are the tools that read earlier turns (3.6).
var historyTools = []string{"search_history", "read_messages"}

// fromHistory checks that the match came back from beyond the turn's
// window: on a history card Go added (3.6), or in a search_history or
// read_messages result in the turn. It fails when the window already holds
// the match, since then the turn tests nothing about history.
func (rec *record) fromHistory(a Assert) (string, string, float64) {
	re := regexp.MustCompile(a.Match)
	in := rec.inTurns(a.Turn)
	seen := false
	for t := 1; t <= rec.turns(); t++ {
		if !in(t) {
			continue
		}
		if _, ok := rec.window[t]; !ok {
			continue
		}
		seen = true
		if m := re.FindString(rec.window[t]); m != "" {
			return Fail, "the window of turn " + strconv.Itoa(t) + " already holds " + clip(m, 80), 0
		}
		if m := re.FindString(rec.cards[t]); m != "" {
			return Pass, "a history card brought back " + clip(m, 80), 0
		}
		for _, e := range rec.events {
			if e.turn == t && e.kind == evResult && slices.Contains(historyTools, e.tool) {
				if m := re.FindString(e.text); m != "" {
					return Pass, e.tool + " brought back " + clip(m, 80), 0
				}
			}
		}
	}
	if !seen {
		return Skip, "no request of the turn was recorded", 0
	}
	return Fail, "neither a history card nor " + strings.Join(historyTools, " or ") + " brought back " + a.Match, 0
}

// memoryContains checks user and project memory at the end (3.5): a
// convention may go to either.
func (rec *record) memoryContains(a Assert) (string, string, float64) {
	re := regexp.MustCompile(a.Match)
	for _, l := range []struct {
		name string
		ms   []store.MemorySection
	}{{"user", rec.userMemory}, {"project", rec.projectMemory}} {
		for _, m := range l.ms {
			if txt := m.Section + ": " + m.Content; re.MatchString(txt) {
				return Pass, l.name + " memory: " + clip(txt, 120), 0
			}
		}
	}
	return Fail, "neither user nor project memory matches " + a.Match, 0
}
