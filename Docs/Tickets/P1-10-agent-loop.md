# P1-10 — Agent loop
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 1)
**Needs:** P1-06, P1-08, P1-09
**Requirements:** R-14, R-20, R-21, N-40, N-43, N-44

## Goal
The orchestrator: turns, steps, the context builder and the history window (CODE-OUTLINE 7, SPEC 3.1, 3.6, 8.3).

## Scope
- **Orchestrator:** a runner per chat, turns and steps, the coalescer, and the `Publisher` for `chat:delta`, `chat:part` and `chat:status`.
- **Context (3.1):** the blocks that exist in Phase 1: the system prompt, the role, the skill list, and the chat list in Mother. Memory, the project card and session notes come in Phase 2.
- **History window (3.6):** cuts, stubs, in-turn trimming, and the "earlier turns" line.
- **During a turn (8.3):**
  - user messages join at the next step
  - *Stop* closes open tool calls as cancelled and keeps the text so far
  - `turn_max_requests`, with a last request without tools
  - provider failures: retries with the wait shown in the chat, *Retry now* and *Cancel*, an error card after 10 minutes, and a clear message for `quota` and `request` errors (8.3)
  - the prompt rule for requests with several parts
- **Titles:** generated after the first turn with the `fast` model.
- **Later:** background tasks and finish notices come in Phases 4 and 5 (R-15), and subagents with `max_subagents_per_chat` and the limit in the tool description in Phase 5. The runner leaves room for them.

## Done when
- Integration tests with `provider/fake` and a real store:
  - a 30-turn chat cuts as configured
  - a message sent during a turn joins at the next step
  - *Stop* leaves a history both protocol shapes accept
  - a restart mid-chat keeps the history
  - a chat turn starts while every background slot is taken
  - a rate-limited turn shows the wait and finishes after it
- The coalescer and the stall handling are tested with `synctest`.
