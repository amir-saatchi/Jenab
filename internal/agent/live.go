package agent

// SPIKE-037 Part A (a prototype): a live answer. Spike-only: the spike's
// harness turns it on through Deps.LiveAnswer to compare the conditions;
// the app's zero value keeps today's loop. Part B (earlier turns as
// answers only) was dropped in TASK-024; TASK-027 took out its hook.

import "strings"

// liveRule is item 4 of the prompt rule with a live answer (SPIKE-037):
// the finish turn's answer replaces the earlier one, so it is written
// whole. The other items stay as they are.
const liveRule = "4. In the finish turn, write the whole answer again with what arrived: keep what still holds from your earlier answer, add the new results and correct what changed. It replaces your earlier answer."

// rule is the prompt rule for requests with several parts (8.3), with
// item 4 reversed when the answer is live.
func (o *Orchestrator) rule() string {
	r := strings.TrimSpace(rulePrompt)
	if !o.d.LiveAnswer {
		return r
	}
	lines := strings.Split(r, "\n")
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "4.") {
			lines[i] = liveRule
		}
	}
	return strings.Join(lines, "\n")
}
