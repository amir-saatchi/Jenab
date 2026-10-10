package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The prompts are the same for every model (SPEC 1).

const systemPrompt = `You build a graph of thoughts to work out how to do a task. Each thought is one node: a kind, a short title, its text, and a weight. Write titles and text in the language of the task.

Kinds:
- problem: what the task really is: the goal, the facts and limits given, what is unknown, and what a good result must achieve. Only the first thought, #1.
- solution: one way to reach the goal. Solutions under the same thought are different approaches, not parts of one approach.
- step: the next concrete step inside a solution or step: what to do and how.
- critique: a risk, gap or mistake in the thought it is under, and what to do about it.
- merge: one approach that combines the thought it is under with other thoughts; list their numbers in merge_with.

Weight: how much the thought helps solve the problem as #1 defines it, from 0 to 1.
0.2 = probably won't work, or doesn't matter. 0.5 = workable, with clear downsides. 0.8 = strong. 1.0 = clearly solves it.
Weigh new thoughts against their siblings, and use the whole range.

Status of each new thought:
- open: worth working out further
- done: complete enough to act on; nothing more is needed under it
- dead_end: not worth more work

Titles: at most 8 words that say what the thought is, so the index alone tells the thoughts apart. Not "Option A" or "Step 2".

You see the index of the whole graph (titles only) and the full text of the path to the thought you expand. Don't add a thought the index already has: build on it, critique it, or merge it instead.

Answer only by calling record_thoughts.`

const toolName = "record_thoughts"

// toolSchema allows up to k thoughts per call.
func toolSchema(k int) json.RawMessage {
	s := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"thoughts": map[string]any{
				"type": "array", "minItems": 1, "maxItems": k,
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"kind":       map[string]any{"type": "string", "enum": []string{"problem", "solution", "step", "critique", "merge"}},
						"title":      map[string]any{"type": "string", "minLength": 1, "maxLength": 120, "description": "at most 8 words"},
						"content":    map[string]any{"type": "string", "minLength": 1},
						"weight":     map[string]any{"type": "number", "minimum": 0, "maximum": 1},
						"rationale":  map[string]any{"type": "string", "description": "why this weight, in one sentence"},
						"status":     map[string]any{"type": "string", "enum": []string{"open", "done", "dead_end"}},
						"merge_with": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "merge only: the numbers of the other thoughts it combines"},
					},
					"required": []string{"kind", "title", "content", "weight", "rationale", "status"},
				},
			},
			"reweight": map[string]any{
				"type": "array", "description": "new weights for thoughts already in the index",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":     map[string]any{"type": "integer"},
						"weight": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
					},
					"required": []string{"id", "weight"},
				},
			},
			"next": map[string]any{"type": "string", "description": `the thought to expand next, as "#7", or "finish"`},
		},
		"required": []string{"thoughts", "next"},
	}
	b, _ := json.Marshal(s)
	return b
}

const toolDescription = "Record new thoughts in the graph, change weights of thoughts in the index, and say which thought to expand next."

func defineMsg(p problem) string {
	return fmt.Sprintf(`Task:
%s

The graph is empty. Add exactly one thought of kind problem, #1: restate the task as a clear problem: the goal, the facts and limits given, what is unknown, and what a good result must achieve. Give it weight 1 and status open. Set next to "#1".`, p.Text)
}

func expandMsg(g *graph, focus, k, room int) string {
	add := fmt.Sprintf("add 1 to %d new thoughts under it", k)
	if k == 1 {
		add = "add one new thought under it"
	}
	return fmt.Sprintf(`Task:
%s

Index of the graph (%d thoughts):
%s

Path to #%d, in full:
%s

Expand #%d: %s.
- Under the problem, add solutions. Under a solution or a step, add next steps, critiques, or a merge of it with other thoughts.
- In reweight, give new weights to thoughts in the index that look different now. Leave it empty if none do.
- In next, name the thought to expand after this one, as "#7": an open or expanded thought, maybe one you add now (new thoughts get #%d onward, in order). Or "finish" if the graph already holds enough for a good, complete answer.
Room left: %d more thoughts. Thoughts at depth %d (the problem is depth 0) can't be expanded.`,
		g.Problem.Text, len(g.nodes), g.index(), focus, g.pathText(focus), focus, add, len(g.nodes)+1, room, g.MaxDepth)
}

func concludeMsg(g *graph) string {
	return fmt.Sprintf(`Task:
%s

Index of the graph (%d thoughts):
%s

The strongest thoughts, in full:
%s

Write the final answer to the task for the user, from the graph: the approach to take and its steps in order, and the risks to watch. Write in the language of the task, and don't mention the graph or its numbers in the answer. After the answer, add one last line: "Used: #3, #7, ..." with the thoughts the answer is built on.`,
		g.Problem.Text, len(g.nodes), g.index(), full(g.strongest()))
}

const concludeSystem = "You turn a graph of thoughts about a task into the final answer for the user."

const baselineSystem = "Answer the user's task: explain how to do it. Write in the language of the task."

// usedLine splits the Used line off the answer.
func usedLine(answer string) (string, string) {
	lines := strings.Split(strings.TrimRight(answer, "\n "), "\n")
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-3; i-- {
		l := strings.Trim(strings.TrimSpace(lines[i]), "*_")
		if rest, ok := strings.CutPrefix(l, "Used:"); ok {
			return strings.TrimSpace(strings.Join(lines[:i], "\n")), strings.TrimSpace(rest)
		}
	}
	return strings.TrimSpace(answer), ""
}
