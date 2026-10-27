# TASK-024 — Move SPIKE-037's Part B out of the app
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** SPIKE-037

## Goal
SPIKE-037 merged its prototype into `internal/agent` (7f10c24). Part B, earlier turns as answers only or with a trace, was not adopted ("today's window stays"), but its code is still in the app: about 200 of the 244 lines in `live.go`, run on every request. Move it into the spike, so the app keeps only one hook.

## Today
- `internal/agent/live.go`: `EarlierMode`, `EarlierModes` (used nowhere), `EarlierTurns`, `earlier`, `answerOnly`, `traceLines`, `firstString` and `TraceArg`.
- `Deps.EarlierTurns` in `agent.go`, and `r.o.earlier(...)` in `cutIfDue` and `request` (`context.go`).
- `TestEarlierTurns` in `live_test.go`.
- Part A, `Deps.LiveAnswer` with `liveRule` and `rule()`, was adopted. It stays until the live-answer feature puts the rule into `prompts/rule.md`.

## Scope
1. Replace `Deps.EarlierTurns` with one hook, for example `Earlier func(ms []chat.Message, cur int) []chat.Message`. Nil keeps today's window. Call it where `earlier` is called today.
2. Move the Part B types, functions and their test into `spikes/037-live-answer/`. The spike sets the hook, and `go run` still gives the same conditions.
3. Drop `EarlierModes`, and don't export what only the spike uses.
4. Keep `LiveAnswer`, `liveRule`, `rule()` and `TestLiveRule` in the app.

## Done when
- `internal/agent` has no Part B code, only the hook; with it nil the requests are as before.
- The app's build, vet and tests pass; the spike builds and vets, and its tests pass.
- The SPIKE-037 ticket and the spike's README point to the new place.

## Result
Done on 2026-10-27 (commit b49054b).

- **The hook:** `Deps.Earlier func(ms []chat.Message, cur int) []chat.Message` replaces `Deps.EarlierTurns`. `earlier()` in `live.go` returns the messages as they are when it is nil, and `context.go` is unchanged.
- **Moved:** `answerOnly`, `traceLines`, `firstString` and `traceArg` are in `spikes/037-live-answer/earlier.go`, unexported, with their logic unchanged. `run.go` sets the hook for the answers and trace conditions. `EarlierModes` is dropped.
- **Tests:** `TestEarlierTurns` moved to the spike and checks the hook's output directly. A new `TestEarlierHook` in the app checks that the request carries what the hook returns and that the store is untouched. `TestLiveRule` stays.
- **Not Part B:** the ticket first listed `TestEarlierTurnsAndFirstLine`. It tests P1-10's stub line, so it stays.
