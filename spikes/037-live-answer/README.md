# SPIKE-037 harness: a live answer, and earlier turns as answers only

Plays scripted chats on the app's real orchestrator (`internal/agent`) with
fake tools, and scores them. The ticket is
`Docs/Tickets/SPIKE-037-live-answer.md`; the results are in `results.md`.

```
go run . -part A -tests a_table -reps 1                             # smoke
go run . -part A -reps 3                                            # Part A: 8 tests × 2 conds × 3 models × 3 reps
go run . -part B -reps 3                                            # Part B: 2 chats × 3 conds × 3 models × 3 reps
go run . -resume results/<stamp>                                    # runs missing or failed
go run . -report results/<stamp>/runs.jsonl                         # results.md again
```

Keys come from `-env` (default `../../.env`) into memory and are redacted
from errors. `-lanes ollama-cloud=3,mistral=2` sets the runs at a time per
provider.

## Hooks in the app (spike-only)

- `Deps.LiveAnswer` (Part A): swaps item 4 of the prompt rule (8.3) for
  "write the whole answer again … It replaces your earlier answer". The
  request stays append-only. Where the answer goes in the user's view is
  computed by the scorer, as the design says Go would: a finish turn
  replaces its request's answer while no user message came since, else it
  goes below; a finish turn with no text leaves it.
- `Deps.Earlier` (Part B): a hook that gets a request's messages and the
  current turn and returns what `shape` gets of the turns before it. Nil
  is today's window. The harness sets it to answers only (the user's
  messages and the turn's last text part) or answers with a trace line
  per tool, written by Go (`earlier.go` here, tested in
  `earlier_test.go`). It gives each tool's main argument and what it made
  (`traceLine` in `world.go`).

The app's zero values keep today's loop; `go test ./internal/...` passes.
`spikes/` is outside `./...`, so build and test this one by its path:
`go test ./spikes/037-live-answer/`.

## What is faked

- **Finish notices.** The orchestrator doesn't start finish turns yet, so
  the harness sends `[task t_N finished: …]` as a user message once the
  chat is idle. Part A sends them in each test's order (tasks of one order
  together, in one notice); Part B sends a task's notice after a set
  scripted message.
- **Subagents** (`tasks.go`): `subagent` with `background: true` returns a
  task ID; its result comes from the test's rule for the source the task
  names first. `task_status` answers as 8.3 says, and lists the task IDs
  for a wrong one.
- **The web** (`world.go`): canned search results and pages. `fetch_page`
  stores the page in the run's bucket as the real tool does, so the real
  `read_ref` and `search_ref` read it. Tables live in memory and are in
  the project card; a wrong table name lists the tables.

## Files

- `parta.go`: Part A's 8 tests. `partb.go`: Part B's two chats and checks.
- `earlier.go`: Part B's conditions, set on the orchestrator through
  `Deps.Earlier`.
- `run.go`: one run: project, chat, orchestrator, the scheduler, and the
  record of turns, calls and requests (from the traces).
- `score.go`, `report.go`: the measures and `results.md` tables; a
  transcript per run in `results/<stamp>/transcripts/`.
