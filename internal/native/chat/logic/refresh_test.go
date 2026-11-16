package logic

// Ported from the React app's chat/refresh.test.ts.

import (
	"strings"
	"testing"

	"github.com/amir-saatchi/jenab/internal/agent"
)

func TestALongRefreshsDialogSaysWhatItCosts(t *testing.T) {
	e := agent.RefreshEstimate{Turns: 30, Chunks: 6, Saved: 2, Calls: 5, Tokens: 24000, Model: "fast", Long: true}
	if got := EstimateTitle(e, false); got != "Refresh memory: 30 turns?" {
		t.Errorf("title: %q", got)
	}
	one := e
	one.Turns = 1
	if got := EstimateTitle(one, true); got != "Review the whole chat again: 1 turn?" {
		t.Errorf("full title: %q", got)
	}
	want := "This makes 5 calls to Gemma, with about 24,000 tokens of input. 2 parts of 6 reviewed before are kept." +
		" Memory changes only when it ends, and Stop cancels it."
	if got := EstimateText(e, "Gemma"); got != want {
		t.Errorf("text: %q", got)
	}
	fresh := e
	fresh.Saved, fresh.Calls = 0, 1
	if got := EstimateText(fresh, "Gemma"); !strings.HasPrefix(got, "This makes 1 call to Gemma, with about 24,000 tokens of input. Memory") {
		t.Errorf("text, none saved: %q", got)
	}
}

func TestABatchNamesItsSectionsByMemory(t *testing.T) {
	cases := map[string]string{
		"memory:user/preferences": "User memory · preferences",
		"memory:project/a/b":      "Project memory · a/b",
	}
	for in, want := range cases {
		if got := MemoryTitle(in); got != want {
			t.Errorf("MemoryTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAnUndoThatDidNothingNamesTheSectionsThatChanged(t *testing.T) {
	if got := UndoChangedText([]string{"user memory, section preferences"}); got != "User memory, section preferences changed since, so nothing was undone. Edit memory in the settings." {
		t.Errorf("got %q", got)
	}
	if got := UndoChangedText(nil); !strings.HasPrefix(got, "A section changed since") {
		t.Errorf("got %q", got)
	}
}

func TestThousands(t *testing.T) {
	cases := map[int]string{0: "0", 999: "999", 1000: "1,000", 24000: "24,000", 1234567: "1,234,567", -1500: "-1,500"}
	for n, want := range cases {
		if got := thousands(n); got != want {
			t.Errorf("thousands(%d) = %q, want %q", n, got, want)
		}
	}
}
