# Gate-4 — Tests
**Status:** Not started

## Purpose
How we prove each phase works.

## Content
To be written. Phase 1 so far:

### Phase 1 acceptance (P1-18)

| Test | How it runs | Proves |
|---|---|---|
| `P1_restart_history` (`scenarios/acceptance`) | `jenab-scenarios` on the reference models | 25 turns with tools survive a restart after turn 12, and turn 25 finds turn 3 through `search_history` |
| `TestAcceptanceRestart` (`internal/scenario`) | `go test`, scripted model | the same run without a network: the restart cuts the window, and the answer comes from the search |
| `TestSearchSpeed` (`internal/store`) | `go test` with `JENAB_PERF_TEST=1` | N-09 |
| `wails3 task bench` (`cmd/jenab-bench`) | Windows, the built app | N-01, N-02, N-07, N-52, and N-54 for LLM calls and chats |

## Exit criteria
- [ ] Test levels defined (unit, integration against real SQLite, end-to-end)
- [ ] Acceptance tests for each phase's "done when" ([PROPOSAL §9](../PROPOSAL.md#9-roadmap))
- [ ] Token and accuracy benchmark defined ([PROPOSAL §11](../PROPOSAL.md#11-success-measures))
- [ ] Tests run in CI

## Open items
- Memory under load with 4 pipeline runs and their scripts (N-54), once pipelines exist. Phase 1 measures LLM calls and chats only (P1-18).
