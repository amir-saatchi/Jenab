# TASK-025 — A live answer
**Type:** Task
**Status:** Open (needs finish turns)
**Gate:** 3
**Needs:** SPIKE-037, background tasks and finish turns (Phases 4 and 5, P1-10)

## Goal
SPIKE-037 decided on a live answer: when background work finishes, the finish turn writes the whole answer again, and it replaces the earlier answer in the chat. Today the user reads one answer per finish turn, stacked, often repeating the same table (4.1 answers to read against 1.4).

## Today
- The prompt rule (8.3, `internal/agent/prompts/rule.md`) item 4 says "give the rest, without repeating". `Deps.LiveAnswer` swaps in the reversed item, `liveRule` in `internal/agent/live.go`; only the spike sets it.
- The orchestrator doesn't start finish turns yet. `NoticeTaskFinished` exists, but background tasks come in Phases 4 and 5 (P1-10).

## Scope
1. **SPEC 8.3 first:** finish turns, the reversed rule item, the live answer and its revisions, as SPIKE-037's Decision has them.
2. **The rule:** put `liveRule`'s wording into `prompts/rule.md` as item 4, and remove `Deps.LiveAnswer`, `liveRule`, `rule()` and the flag in the spike.
3. **The answer:** a finish turn's answer becomes the next revision of the answer of the turn that started the task. The model's request stays append-only.
   - In place only while it is the chat's latest. After a user message, the first arrival answers below, and later arrivals for the same request update that answer while it is latest.
   - A finish turn with no text leaves the answer as it is.
4. **The chat:** a mockup first. The folded chain, "updating" while a finish turn runs, and the earlier revisions, which the user can open.
5. **Open:** a very short finish answer that would replace a full one. SPIKE-037 proposed it goes below as a note; it never happened in the runs.

## Done when
- SPEC 8.3 describes it.
- Tests: revisions in place, below after a user message, an empty finish turn, two tasks in one notice, and Persian text.
- A scenario with two background tasks gives one answer to read, holding both results.

## Result
