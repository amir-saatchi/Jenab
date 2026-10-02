# P1-18 — Phase 1 acceptance
**Type:** Task
**Status:** Open
**Gate:** 3 (Phase 1)
**Needs:** P1-15, P1-16, TASK-001
**Requirements:** N-01, N-02, N-07, N-09, N-52, N-54

## Goal
Prove Phase 1's "done when" (PROPOSAL 9): a user can hold a normal tool-using chat that survives restarts, and the agent finds something said 20 turns earlier through history search.

## Scope
- **Scenario:** a TASK-001 scenario of 25 turns with tool calls and a restart after turn 12. At turn 25 it asks about something from turn 3, and asserts `tool_called: search_history` and `answer_contains`.
- **Restart test:** the same restart as a Go integration test with `provider/fake`, so it also runs in CI.
- **Performance:** the Phase 1 numbers (N-01, N-02, N-07, N-09, N-52), measured on the built app on Windows, and on macOS and Linux when available.
- **Memory under load (N-54):** the app's memory with 8 background LLM calls streaming from `provider/fake` and two busy chats. Pipelines and scripts come later, so the full test, with 4 runs each holding a script near its 256 MB cap, is added to Gate 4 with them. The results confirm or change the 4 GB minimum.
- **Gate 4:** the Phase 1 acceptance tests in Gate 4 are written from this ticket.

## Done when
The scenario passes on two development models, the restart test passes in CI, and the measured numbers are recorded here.
