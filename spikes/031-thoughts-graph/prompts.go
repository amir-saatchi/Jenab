package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The prompts are the same for every model (SPEC 1). The answer is one JSON
// object whose schema is in the prompt, as SPIKE-030 decided.

const kindsText = `You build a graph of thoughts to work out how to do a task. Each thought is one node: a kind, a short title, its text, and a weight. Write titles and text in the language of the task.

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

Titles: at most 8 words that say what the thought is, so they tell the thoughts apart. Not "Option A" or "Step 2".`

const indexText = `

You see the index of the whole graph (titles only) and the full text of the path to the thought you expand. Don't add a thought the index already has: build on it, critique it, or merge it instead.`

const pathText = `

You see the full text of the path from #1 to the thought you expand.`

// systemPrompt ends with the schema of the answer.
func systemPrompt(index bool, schema json.RawMessage) string {
	s := kindsText
	if index {
		s += indexText
	} else {
		s += pathText
	}
	return s + "\n\nAnswer with exactly one JSON object that matches this JSON Schema, and nothing else:\n" + string(schema)
}

// thoughtsSchema allows up to k thoughts. With next, the model also says
// which thought to expand next.
func thoughtsSchema(k int, next bool) json.RawMessage {
	props := map[string]any{
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
	}
	required := []string{"thoughts"}
	if next {
		props["reweight"] = map[string]any{
			"type": "array", "description": "new weights for thoughts already in the index",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":     map[string]any{"type": "integer"},
					"weight": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
				},
				"required": []string{"id", "weight"},
			},
		}
		props["next"] = map[string]any{"type": "string", "description": `the thought to expand next, as "#7", or "finish"`}
		required = append(required, "next")
	}
	b, _ := json.Marshal(map[string]any{"type": "object", "properties": props, "required": required})
	return b
}

func defineMsg(p problem) string {
	return fmt.Sprintf(`Task:
%s

The graph is empty. Add exactly one thought of kind problem, #1: restate the task as a clear problem: the goal, the facts and limits given, what is unknown, and what a good result must achieve. Give it weight 1 and status open.`, p.Text)
}

func expandMsg(g *graph, focus, k, room int, index bool) string {
	add := fmt.Sprintf("add 1 to %d new thoughts under it", k)
	if k == 1 {
		add = "add one new thought under it"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Task:\n%s\n\n", g.Problem.Text)
	if index {
		fmt.Fprintf(&b, "Index of the graph (%d thoughts):\n%s\n\n", len(g.nodes), g.index())
	}
	fmt.Fprintf(&b, "Path to #%d, in full:\n%s\n\n", focus, g.pathText(focus))
	fmt.Fprintf(&b, "Expand #%d: %s.\n- Under the problem, add solutions. Under a solution or a step, add next steps, critiques", focus, add)
	if index {
		b.WriteString(", or a merge of it with other thoughts")
	}
	b.WriteString(".\n")
	if index {
		fmt.Fprintf(&b, `- In reweight, give new weights to thoughts in the index that look different now. Leave it empty if none do.
- In next, name the thought to expand after this one, as "#7": an open or expanded thought, maybe one you add now (new thoughts get #%d onward, in order). Before you go deeper on one branch, consider whether an open solution that hasn't been expanded could be better. Or say "finish" if the graph already holds enough for a good, complete answer.
Room left: %d more thoughts. Thoughts at depth %d (the problem is depth 0) can't be expanded.`, len(g.nodes)+1, room, g.MaxDepth)
	} else {
		fmt.Fprintf(&b, "Room left: %d more thoughts.", room)
	}
	return b.String()
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
