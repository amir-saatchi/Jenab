package agent

import (
	"strings"
	"testing"
)

func TestLiveRule(t *testing.T) {
	if got := New(Deps{}).rule(); got != strings.TrimSpace(rulePrompt) {
		t.Errorf("rule without a live answer:\n%s", got)
	}
	got := New(Deps{LiveAnswer: true}).rule()
	if !strings.Contains(got, liveRule) || strings.Contains(got, "without repeating") || !strings.Contains(got, "3. Say what is still running.") {
		t.Errorf("live rule:\n%s", got)
	}
}
