# TASK-033 — Less fixed context per request
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** P2-13

## Goal
In P2-13's benchmark, Jenab used 1.9-3.8 times the prompt tokens per turn of a plain chat harness, with the same accuracy on the reference models, so N-42 doesn't hold. Most of the gap is fixed: about 4,900 tokens of tool definitions and block 1 in every request (plain: 900), and 2.9-4.1 requests a turn (plain: 1.5-1.6). Cut it without losing accuracy, the same for every model.

## Scope
Measure each change on `scenarios/bench-bitcoin` (`-harness jenab,plain`) and keep what helps:
1. **Tools when needed:** the data tools only once the project has tables, or another rule that shrinks the tool list. The list changes only at a cut, so requests stay append-only (3.1).
2. **Skills:** a loaded skill's text leaves block 1 at a cut once it is no longer used.
3. **Fewer rounds** in a data turn, for example a write that returns what the answer needs.
4. **Standing instructions:** the session notes take a change to one ("add Ethereum to the usual"); in P2-13 they missed it twice.
5. **A longer set:** 3-4 weeks of the same project, written and committed before any scored run, to find the turn where plain gets dearer than Jenab.

## Done when
- On the reference models, 3 reps: Jenab's prompt tokens per turn on `bench-bitcoin` are at least 30 % lower than in P2-13, with checks as good.
- The longer set shows where Jenab gets cheaper than plain, or that it doesn't within 4 weeks.

## Result
Done on 2026-11-01 for Gemini; gemma4:31b on the final code waits for Ollama Cloud's quota (about 2026-11-22). SPEC 3.1, 3.3, 8.1, 8.3 and 8.9 describe it.

- **What changed:**
  1. **Answer with a write:** `insert` carries the turn's reply in a required, may-be-empty `answer` argument; when every call succeeds the turn ends without another request. Optional, gemma used it unevenly (−22 to −25 %); required, it made 2.1–2.2 requests a turn.
  2. **Unused skills** leave block 1 at a cut: block 1 went from about 1,400 to 750–900 tokens.
  3. **Tools when needed:** data, history, chat-list and bucket tools join once there is something for them, saving 100–200 tokens a request on the bench and more on new projects. Gating `insert` lost day 1's insert, so it stays.
  4. **Standing instructions:** a line in `update_memory` and block 1. gemma still left Ethereum out of "the usual" in 2 of about 20 runs.
  5. **Fewer rounds:** a block 1 line for independent calls in one message, and a note that the schema agent loads its own skills.
- **`bench-bitcoin`, 3 reps, prompt tokens a turn, Jenab vs plain (checks; requests a turn):**
  - gemini-3.5-flash-lite (final code): 19,981 (49/51; 2.73) vs 9,760 (51/51; 1.65). P2-13 was 37,466 (51/51), so **−46.7 %**. The 2 missed checks are one answer in rep 3 that ended with no text after two history calls, most likely a thinking-only completion; Jenab showed the empty-answer notice (8.3). Reps 1 and 2 answered it.
  - gemma4:31b: 15,494 (51/51; 2.13) vs 12,499 (51/51) on 4b3504b, **−34.9 %**, before the fix that keeps the answer off `update_memory`. The final code is unmeasured: every Ollama key is out of quota.
  - ministral-14b (extra, as is): 20,505 (40/51) vs 19,082 (47/51); P2-13 was 25,790 (40/51).
- **`bench-bitcoin-long`** (three weeks, 30 turns, committed before any scored run):
  - Gemini: 18,898 (83/84; 2.62) vs 13,050 (84/84; 1.57). Jenab's first request is smaller (7,115 vs 8,727; peak 13,033 vs 17,094), and per turn it is first cheaper at turn 14, then on 6 of the update turns. Summed, it **isn't cheaper within the three weeks** (567k vs 391k): each tool round sends the context again, and plain answers questions from its window without tools. The miss is a wrong average (64,004.40 for 64,000.40).
  - gemma (13e68df): 17,320 (84/84; 2.48) vs 17,452 (84/84; 1.50); cheaper per turn from turn 12 on the update turns, and summed at turn 30 (519.6k vs 523.6k).
  - ministral: 20,152 (63/84) vs 29,051 (70/84); one plain rep looped at 50k a turn.
- **Done when:** Gemini meets the token target with one model-side miss; gemma reached it on 4b3504b but not yet on the final code. The longer set shows where Jenab gets cheaper per turn and that summed it doesn't for Gemini.
- **Open:** gemma4:31b on the final code, both sets, when Ollama Cloud's quota resets.
- **Also:** `bench-bitcoin`'s date checks accept "October 12th" (gemma's 3 earlier misses); no expected answer changed. On Google's API, `gemma-4-31b-it` gives HTTP 500 whenever tools are sent ([MODELS](../MODELS.md)).
- **Requests:** Ollama Cloud 2,892 (the first key 257, `_II` 2,515, `_III` 120, all rejected for credit), Mistral 704, Gemini 714 (the first key 270, `_III` 253, `_IV` 191).
