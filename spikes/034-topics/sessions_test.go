package main

import (
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/id"
)

func TestSessions(t *testing.T) {
	sc, _ := scenarioByID("sqlite")
	for _, cond := range []int{condNotice, condSummary} {
		th := newThreads(sc, "m", "parent", cond)
		s1 := th.Add("c1", "Users table", "users", 0)
		s1.Summary = "users has a unique email"
		th.started["c1"] = 0
		th.at("parent", 3)

		line := th.index()
		if !strings.HasPrefix(line, "s1 · Users table · nothing open · chat c1") {
			t.Fatalf("cond %d: list line %q", cond, line)
		}
		if got := strings.Contains(line, s1.Summary); got != (cond == condSummary) {
			t.Errorf("cond %d: summary in the list line = %v", cond, got)
		}

		th.up("c1", 0, strings.Repeat("é", noticeChars+50)) // the finish notice
		th.up("c1", 1, "the user's own turn")
		th.up("parent", 2, "the parent's turn sends nothing")
		want := 1
		if cond == condNotice {
			want = 2
		}
		ns := th.notices("parent")
		if len(ns) != want {
			t.Fatalf("cond %d: %d notices, want %d", cond, len(ns), want)
		}
		if n := len([]rune(ns[0].Text)); n != len("Session s1 · Users table: ")-1+noticeChars+1 {
			t.Errorf("cond %d: the finish notice has %d characters", cond, n)
		}
		if len(th.notices("parent")) != 0 || len(th.notices(id.Chat("c1"))) != 0 {
			t.Errorf("cond %d: notices given twice, or to a sub-session", cond)
		}
		if th.ups[0].Delivered != 3 || !th.ups[0].Finish {
			t.Errorf("cond %d: first notice %+v", cond, th.ups[0])
		}
	}
}
