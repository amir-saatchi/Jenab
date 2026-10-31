package main

import (
	"testing"

	"github.com/amir-saatchi/jenab/internal/scenario"
)

// TestSet: the set loads, every scenario follows up a chat of at least 20
// earlier turns, and each chat has a repeat, a paraphrase and a control.
func TestSet(t *testing.T) {
	s, err := scenario.Load(setDir)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]map[string]bool{}
	for _, sc := range s.Scenarios {
		c := chatOf(sc.ID)
		turns, err := loadChat(setDir, c)
		if err != nil {
			t.Fatal(err)
		}
		if len(turns) < 20 {
			t.Errorf("%s: %d earlier turns, want at least 20", c, len(turns))
		}
		if kinds[c] == nil {
			kinds[c] = map[string]bool{}
		}
		for _, k := range sc.Tests {
			kinds[c][k] = true
		}
	}
	if len(kinds) < 8 {
		t.Errorf("%d chats, want at least 8", len(kinds))
	}
	for c, ks := range kinds {
		for _, k := range []string{"repeat", "paraphrase", "control"} {
			if !ks[k] {
				t.Errorf("%s has no %s follow-up", c, k)
			}
		}
	}
}
