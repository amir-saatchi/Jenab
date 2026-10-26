# SPIKE-037 — A live answer, and earlier turns as answers only
**Type:** Spike
**Status:** Open
**Gate:** 3
**Needs:** SPIKE-023

## Question
A turn is two things mixed in one list of parts (SPEC 2.3): the **chain** of what happened (thinking, tool calls, results, notices, text between calls) and the **answer** the user reads. This spike tests what changes when the two are kept apart.

**Part A: a live answer.** Today a background task's finish notice starts a new turn, and its answer goes below the first one with "the rest" (8.3). For a request such as "find jobs from five sources", the user gets one answer per arrival. Instead, the finish turn could update the first answer in place, so the user always reads one current answer.

1. Does the model write the whole updated answer in a finish turn, keeping what was already there and adding what arrived, on every reference model?
2. Is one live answer clearer than answers stacked below each other, and at what cost?

**Part B: earlier turns as answers only.** Earlier turns now carry their full text, tool calls and previews until a cut, then stubs (3.6, 3.7). If an earlier turn entered the request as only the user's message and the turn's answer, the history would be much smaller.

3. How much smaller, per request and per run?
4. What does the model lose? Follow-ups that need a tool result's detail, a source, or a name the turn created, and work it repeats because it doesn't see that it was done.
5. Does a short trace line built in Go (tools used, with names, refs and IDs, no results) win back what was lost, at little cost?

## The design to test
Proposed on 2026-10-26.

- **The chain** is stored as it happens and never edited, as today.
- **The answer** is the turn's last text part. Text between tool calls belongs to the chain. In the chat, the chain folds into the step list, and the answer is shown in full.
- **One answer per turn can change, and nothing else.** No sections, no names, no tool: the model just answers, and Go decides where the text goes. This keeps the model from naming the wrong section and keeps the user from seeing several places change.
- **Finish turns (Part A):**
  - A finish turn's answer becomes the next revision of the answer of the turn that started the task. It isn't added below.
  - The model writes the whole updated answer, not only the new part. This reverses item 4 of 8.3's prompt rule for requests with several parts ("give the rest… without repeating"), which raised SPIKE-023's finish-turn pass rate from 65 % to 88 %. The rule is measured again here.
  - An answer is updated in place only while its turn is the chat's latest. If the user has sent a message since, the finish turn answers below, as today.
  - A finish turn with no text leaves the answer as it is.
  - Every revision is kept; the chat can show earlier ones. While a new revision streams, the old one stays, marked as updating, and is replaced once the new one is complete (answers are stored only when complete, 8.3).
  - The model's request stays append-only: the first answer is still there as written, and the finish turn comes after it. Only the user's view replaces it.
- **Earlier turns (Part B):**
  - When a turn ends, its chain leaves the request: from the next turn on, it enters as the user's message and the latest revision of its answer. Thinking, tool calls, results and text between calls are dropped, always together, so every provider accepts the history.
  - The cached prefix still holds up to the turn before: only the just-finished turn changes shape, and it gets smaller (3.1).
  - `search_history` and `read_messages` still reach the full chain.
  - **Trace variant:** one line per turn, written by Go, e.g. `[turn 12 used: fetch_page ×3 (refs: cache/pages/…), create_table orders, subagent t_4]`.

## Conditions
**Part A:**
1. **Stacked (baseline):** today's finish turns and prompt rule.
2. **Live answer:** as above, with the reversed prompt rule item.

**Part B:**
1. **Today's window:** full text and previews, stubs after a cut.
2. **Answers only.**
3. **Answers and trace.**

## Tests
- **Part A:** requests that start 3 to 5 background subagents whose results arrive at different times, using the `multi-part` set's fake tools.
  - **Answer kinds:** a text answer that a later result refines, a table that gains rows (jobs from several sources), and a mix.
  - **Edge cases:** a task that fails; a task whose result adds nothing; two tasks that finish together (one finish turn, 8.3); a user message sent before the last task finishes.
  - **A non-English request**, Persian, to check the language holds across revisions.
- **Part B:** two scripted chats of about 15 turns with tool use in most turns. SPIKE-029's jobs chat can be reused. Run with today's window sizes and once with a low `history_max_turns`, so cuts happen in both.
  - **Follow-ups that need:** a fact in an earlier answer (should still work); a detail only in a tool result ("what was the salary on the second listing?"); the source ("where did you read that?"); a name the turn created (a table, a ref, a task ID); more of the same work ("add two more sources"), to catch pages fetched again.

## Measures
- **Part A:**
  - Kept content: each task's facts get a checklist; count what each revision keeps and what it drops.
  - Rule-following: finish turns that wrote the whole answer, only the new part, or the old answer plus an appended block.
  - Final answer right, against the stacked baseline.
  - Output tokens and requests per update, and cache reads.
  - Edge cases handled: the failed task said, the empty result left alone, the new message answered below.
- **Part B:**
  - Prompt tokens per request and per run, cache reads, and cuts per run.
  - Follow-ups answered right, per kind.
  - Extra reads: `search_history`, `read_messages` and `read_ref` calls per run.
  - Repeated work: tool calls that redo an earlier turn's call.
  - Wrong claims about earlier work, e.g. a source that wasn't used.

## Models
The reference models (DEVELOPMENT.md): Gemini `gemini-3.5-flash-lite` and Ollama Cloud `gemma4:31b`.
- The prompts and tools are the same for every model (SPEC 1).
- Which keys to use, including whether to use the reserve keys, is confirmed with the user before any run.
- A free tier's failures (rate limits, quota) are recorded and that model stops; there are no workarounds for them.

## Setup
- **Harness:** `spikes/037-live-answer/`, running the app's real orchestrator (`internal/agent`) with the `multi-part` fake tools and a fake world for the jobs chat. A transcript per run.
- **App code:** both parts change what the loop does: where a finish turn's answer goes, and what `shape` keeps of earlier turns. Like SPIKE-032's `Grounding`, they need spike-only hooks in `Deps`. They are agreed with the user before the harness is written.
- **Part A's UI** (the folded chain, revisions, "updating") is not built here. The spike measures the model's side; a mockup can follow if the result is a yes.
- **Safety:** keys come from `.env` and stay in memory. Only synthetic text is sent.

## Relation to other work
- **SPIKE-023** decided finish turns and the prompt rule this spike changes.
- **SPIKE-036** (a plan the agent keeps) is also something in the chat that updates in place. If both are adopted, they should look and behave alike.
- **3.6's window sizes** were set with today's turn sizes. If Part B wins, `history_max_turns` and `history_max_tokens` are tuned again.

## Done when
Each condition has results per model, with 3 reps per cell: Part A 2 conditions × 2 models × 3 reps per test, Part B 2 chats × 3 conditions × 2 models × 3 reps = 36 runs. There is a decision on:
- the live answer: yes or no, and the rules above (in place only while latest, revisions, an empty finish turn)
- 8.3's prompt rule: the whole answer or the rest
- earlier turns: today's window, answers only, or answers and trace, and from which turn on
- whether 3.6's window sizes change

## Result
_Pending._

## Decision
_Pending._
