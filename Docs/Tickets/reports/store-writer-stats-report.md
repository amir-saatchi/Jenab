# Report: the hang in TestCancelledWriteNeverCommits

Commit `16d8ed3` (store: answer a write only after the writer's stats are updated), 2026-11-06. No ticket: found during the TASK-018, TASK-038, TASK-040 and TASK-041 test merge, where the test hung for 10 minutes once under load.

## Cause

A bug in the writer that showed up as a race in the test.

- The writer loop sent a request's answer (inside `job.run`) before it cleared `current` (Busy) and counted the request as done. So right after `mustExec("CREATE TABLE")` returned, `Stats` could still say Busy.
- `holdWriter`'s `waitBusy` took that leftover Busy for the held write and returned before the held `Do` was even queued.
- Under load, the test's second `Do` then reached the idle writer first, was taken at once and committed (nil into `errc`), and the held write ran after it. `Stats().Interactive` never went above 0, and the wait loop had no exit.
- This matches the goroutine dump: the held transaction open, no goroutine left for the second `Do`. The exact hang at `store_test.go:207` was reproduced by adding temporary delays to the old code.

## Fix (`internal/store` only)

- `writer.go`: `job.run` returns a reply func, which the loop calls after `current.Store(nil)` and `completed.Add(1)`. The panic path returns `j.fail(pe)` the same way. A caller that has its answer no longer sees its request running in `Stats`, and it is already counted as done. The reply channel has room for one, so the writer never waits on a caller that left.
- `store_test.go`: `holdWriter` learns that the held write is running from the transaction itself (a `started` channel) instead of `Stats().Busy`; `waitBusy` is gone. A new `waitQueued(t, db, errc, cond)` polls `Stats`, also selects on `errc`, stops after 10 s and fails with the error. It replaces the bare loop in `TestCancelledWriteNeverCommits` and the same pattern in the Close test.
- `writer_test.go`: the new `TestStatsAreCurrentWhenTheAnswerArrives` runs 500 requests and checks Busy is false and Done is i+1 right after each answer. It fails on the old writer at every `-cpu` setting ("after answer 84: Busy true") and passes on the new one.

## Checks

`GOPROXY=off`, `GOMEMLIMIT=1500MiB`, `-p 2`.

- `-count=200 -cpu 1,2,4 -run 'TestCancelledWriteNeverCommits$'` while a full `go test -p 2 -short ./...` ran alongside: 600/600 pass.
- Heavier: two stress runs at once, each `-count=200 -cpu 1,2,4` over both tests, while another full `-short ./...` ran: 1200/1200 pass in each.
- Full `go test -p 2 -short ./...`: pass. `go vet ./internal/store`: ok. gofmt (CRs stripped): clean.
- Main-Session's review: vet, the store, agent and project packages, and `-count=100 -cpu 1,4` of both tests: pass.
