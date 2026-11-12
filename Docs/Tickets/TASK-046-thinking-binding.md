# TASK-046 — Thinking blocks after the context changes
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** —

## Goal
Newer Claude models (Haiku 5.5, Sonnet 5.5, Opus 5.5, Fable 5.1 and 5.2) tie each thinking block to everything sent before it: the system prompt, the tools and the earlier messages. If any of that changed and the block is sent back, the API answers 400: "The block is bound to a different conversation". This is on by default for accounts made on or after 2026-08-31, and for every account on Fable 5.2. Retrying doesn't help.

Jenab changes that prefix often, then sends the old blocks back unchanged (3.8). In the Haiku 5.5 pass, 102 of 391 requests failed this way and 11 of 82 runs broke ([reports/HAIKU-PASS-report.md](reports/HAIKU-PASS-report.md)). Any app user on these models will hit it in longer chats.

## Scope
1. **List the changes.** Find each place a chat's system blocks, tools or earlier messages change between requests, and how often each happens. The report names `refreshCard`, `refreshTools` and `memoryNotices` within a turn, `cutIfDue`, session notes and memory between turns, and the plain harness's date (`plainWindow`).
2. **No failed request.** Fix it in the anthropic provider:
   - With adaptive thinking, send `thinking.block_binding.prefix_mismatch_behavior: "drop_block"` and the `thinking-binding-controls-2026-08-01` beta. The API then drops the stale blocks, and the ones after them, and goes on. `block_binding` works only with `thinking: {type: "adaptive"}`, so a `thinking_budget` model (Haiku 4.5, which doesn't run the check) gets none.
   - Read the response's `input_transformations` and count the dropped blocks in the usage or the trace.
   - A model not in the catalog gets no thinking setting today, so it can't use `drop_block`. Decide what it gets, for example earlier thinking blocks left out when no thinking setting is sent. Don't match on the error's text.
3. **Measure what is lost.** Count the dropped blocks per run on Haiku 5.5. Propose how Jenab could change the prefix less, such as a system prompt that stays the same within a turn, with card and tool changes sent as new messages. That also helps the prompt cache on every provider. If it is large, write it up for its own ticket.
4. **Catalog:**
   - Add `claude-haiku-5-5` (the report's entry), and move `fast` from Haiku 4.5 to it.
   - Fix Sonnet 5.5's `cache_read` to 0.10.
   - Check the other Claude entries against `/v1/models` and Anthropic's prices.
5. **Rerun** the 11 broken runs on Haiku 5.5 (the paid key). Ask Main-Session first, and stop at $0.50.
6. **SPEC 3.8:** the rule, next to "Thinking blocks are sent back unchanged".

## Rules
- Provider layer only. The prompts and the loop stay the same for every model (SPEC 1).
- No URL or model-name guessing: the catalog's `thinking` settings decide.

## Done when
- Go tests: the request has `block_binding` with adaptive thinking and none with a budget or without a thinking setting, and dropped blocks are counted.
- The 11 runs rerun with no 400.
- SPEC 3.8 has the rule, and the catalog has Haiku 5.5.
