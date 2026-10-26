# SPIKE-037 — A live answer, and earlier turns as answers only
**Type:** Spike
**Status:** Done
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
5. Does a short trace built in Go, one line per tool call with names, refs and IDs but no results, win back what was lost, at little cost?

## The design to test
Proposed on 2026-10-26.

- **The chain** is stored as it happens and never edited, as today.
- **The answer** is the turn's last text part. Text between tool calls belongs to the chain.
- **In the chat,** the answer is shown in full. The chain stays visible above it, folded and greyed, and the user can unfold it to check any step: tool calls with their arguments, results, thinking and text between calls.
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
  - **Nothing is deleted.** The full chain stays in `chats.db`. `read_messages` already returns tool calls and results, and `search_history` finds them, so the model can get any detail it needs.
  - **Trace variant:** one line per tool call, written by Go, never by the model: the tool, its main argument, and what the call made (a ref, a table, a task ID), without the result. A turn's trace ends with where the full steps are. For example:
    ```
    [fetch_page jobs.example.com/123 → ref cache/pages/jobs.example.com/ab12.txt]
    [create_table applications]
    [subagent "search LinkedIn" → task t_4]
    [full steps: read_messages(from: 12, to: 12)]
    ```
    Each tool gives its own line, as tools now give their stubs (3.7). Many calls of one tool in a turn merge into one line, e.g. `fetch_page ×12` with their refs.
- **What the model made, after its turn leaves the request:**
  - **Project objects** (tables, pipelines, views) are already in the project card (3.2), so their names don't depend on history.
  - **Refs and task IDs** are in the trace line.
  - **A wrong name** gets an error that lists what exists, e.g. "no table `job`; tables: `jobs`, `applications`", so the model can correct it on its next request (8.3).
  - Asking the model to list what it made in its answer is not tested: it depends on the model following the prompt, which differed between the reference models in SPIKE-035, and it puts bookkeeping into the user's answer.

## Conditions
**Part A:**
1. **Stacked (baseline):** today's finish turns and prompt rule.
2. **Live answer:** as above, with the reversed prompt rule item.

**Part B:**
1. **Today's window:** full text and previews, stubs after a cut.
2. **Answers only.**
3. **Answers and trace:** answers only, plus the trace line per call.

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
  - Extra reads: `search_history`, `read_messages` and `read_ref` calls per run, and whether the model reads when a follow-up needs a detail that isn't in the answer, or answers without reading.
  - Repeated work: tool calls that redo an earlier turn's call.
  - Wrong names: calls that use a ref, table or task ID that doesn't exist.
  - Tool use: tool calls per turn in later turns, against today's window, to catch a model that stops using tools once it doesn't see its earlier calls.
  - Fake calls: trace-like lines written in an answer instead of a real tool call.
  - Wrong claims about earlier work, e.g. a source that wasn't used.
  - The trace's size: tokens per turn.

## Models
Chosen by the user on 2026-10-26 ([MODELS](../MODELS.md)), since Gemini's free quota was used up that day:
- Ollama Cloud `gemma4:31b` (reference model), `OLLAMA_API_KEY`
- `gpt-oss-120b`: Ollama Cloud `gpt-oss:120b`, or Groq `openai/gpt-oss-120b` if Ollama's one-request-at-a-time plan is too slow
- Mistral `ministral-14b-latest`, `MISTRAL_API_KEY` (30 requests a minute)

Rules:
- The prompts and tools are the same for every model (SPEC 1).
- No reserve keys.
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
Each condition has results per model, with 3 reps per cell: Part A 2 conditions × 3 models × 3 reps per test, Part B 2 chats × 3 conditions × 3 models × 3 reps = 54 runs. There is a decision on:
- the live answer: yes or no, and the rules above (in place only while latest, revisions, an empty finish turn)
- 8.3's prompt rule: the whole answer or the rest
- earlier turns: today's window, answers only, or answers and trace, and from which turn on
- the trace line's format, if it is kept
- whether 3.6's window sizes change

## Result
Run on 2026-10-26 (commit 68e3dfc) on the app's real orchestrator: 144 Part A runs (8 tests × 2 conditions × 3 models × 3 reps) and 54 Part B runs, on `gemma4:31b`, `gpt-oss:120b` (both Ollama Cloud) and `ministral-14b-latest`. There were no errors and no quota stops. Details are in `spikes/037-live-answer/results.md`.

**How the spike differs from the design:**
- Finish turns aren't built yet (8.3), so the harness sent the finish notices as user messages once the chat was idle.
- Part A's hook changes only the prompt rule. The scorer works out where each answer lands under the design's rules.
- The fake pages were 30–200 tokens, against previews of up to 1,500 in real use. This understates what Part B saves.
- Ollama reports no cache reads, so cache figures are Mistral's only.

**Part A, the live answer:**
- **The whole answer:** 86–91% of finish turns rewrote it, keeping 94–99% of the earlier facts. Under today's rule, 67–79% of finish turns were whole anyway, so the stacked view repeated the growing answer: 4.1 answers to read, against 1.4. In one table test that was five near-identical tables. Where models did give only the rest, the last stacked answer lacked earlier facts (ministral: 83% right, against 97% live).
- **Cost:** output tokens per update +9% (gemma), +12% (gpt-oss), +27% (ministral). Requests per update stayed the same. Mistral's cache reads went from 35% to 31%.
- **Edge cases:**
  - A failed task was reported in 17 of 17 runs.
  - An empty result was folded into a whole answer; no finish turn was empty.
  - Two tasks in one notice: both results were in one answer, 9 of 9 runs.
  - A late user message was taken into account in 8 of 9 runs.
  - Persian held in every revision.
- **Risk:** a whole rewrite can add detail no result gave. Ministral added a morning temperature and a train time once.
- **"In place only while latest":** after a user message, every later arrival went below. One test was left with three copies of one table.

**Part B, earlier turns:**
- **Answers only** shrank the history part of a request by 50–85%. But gemma and gpt-oss read it back, with 11–24 `read_messages` and `search_history` calls a run against 3–4, and redid work. Their prompt tokens per run went up 35–120%. Ministral saved 17–22% per run, but guessed URLs, refs and task IDs, and fell to 33% of follow-ups right on the trip chat (67% with the window).
- **Answers and trace** brought reads and repeated work back to about window levels, and removed most of ministral's guessed names. But the models copied the trace: gemma wrote `[read_ref …]` as text instead of calling the tool, and ministral wrote `[subagent …]` instead of starting a task. That was 20 fake calls in 8 of 18 runs, on 2 of 3 models, the reference model included. Follow-ups right fell below the window's for ministral (48% against 59–67%) and for gemma on the jobs chat (70% against 78%).
- **Tool use** didn't drop in any condition.
- **Cache (Mistral):** reshaping the just-finished turn didn't hurt the cached prefix (29–30% read with the window, 33–36% otherwise).
- **A shared failure, in every condition:** after a cut, models forgot their background tasks. Asked for the full list of sources, they left out LinkedIn, StepStone and the flight search.

## Decision
**Decided (2026-10-26):**
- **A live answer: yes.** A finish turn's answer becomes the next revision of the answer of the turn that started the task. The model's request stays append-only.
- **8.3's prompt rule, item 4, is reversed:** in the finish turn, write the whole answer again with what arrived. It replaces the earlier answer. The wording is `liveRule` in the spike's `internal/agent/live.go`.
- **In place only while latest, refined:** after a user message, the first arrival answers below. Later arrivals for the same request update *that* answer while it is the chat's latest.
- **Revisions are kept**, and the user can see earlier ones. A whole rewrite sometimes drops a fact or adds an unsupported detail, and an earlier revision is how the user checks.
- **A finish turn with no text** leaves the answer as it is.
- **Earlier turns: today's window stays,** and 3.6's sizes don't change. Neither answers only nor answers and trace is adopted. If it is tried again:
  - apply it only to turns behind a cut, in place of today's stubs
  - put the trace in its own system block, worded as a record, not in tool-call syntax
  - test with real page sizes
- **Not decided here:** a rule for a very short finish answer, which would replace a full one (proposed: it goes below as a note). It never happened in the runs.

**Still to do:**
- **SPEC 8.3:** finish turns, the reversed rule item, the live answer and its revisions.
- **Building it** needs real finish turns in the orchestrator and a mockup of the chat (the folded chain, revisions, "updating").
- **Forgotten background tasks:** models forgetting their background tasks after a cut is a separate problem, for the background-task work.
