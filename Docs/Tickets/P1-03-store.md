# P1-03 — Store: writers, readers and migrations
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 1)
**Needs:** P1-02
**Requirements:** R-06, N-30, N-32

## Goal
Safe access to every SQLite file: one writer goroutine and a reader pool per file (SPEC 7), and storage format versions (2.8).

## Scope
- **Writer:** one goroutine per file, `Do[T]`, two priorities with the 10:1 rule, short transactions (7.3, 7.4). A cancelled write that is still queued never commits.
- **Readers:** a pool per file and `Query[T]` (7.2), with the connection settings from 7.2.
- **Versions:** `PRAGMA user_version` per file. A newer app backs up with `VACUUM INTO` into `snapshots/` (the safe-copy rule, 7.5), then migrates in one transaction. An older app refuses a newer file without touching it.
- **`registry.db`:** one connection with `busy_timeout`, tables from 2.4 (`user_memory`, `connections` and `mcp_servers` are created now and used later).
- **`project.db` in Phase 1:** only the internal tables Phase 1 needs, `_jenab_meta` and `_jenab_approvals`.
- WAL checkpoints as in 7.2.

## Done when
- Integration tests on real SQLite in `t.TempDir()`; `synctest` for the 10:1 rule with fake jobs (Q35).
- SPIKE-008's crash tests run as permanent tests and pass.
- A file with a newer version is refused and stays byte-for-byte the same.

## Result
- `internal/store` on `modernc.org/sqlite` v1.59.0: `Open`, `Do[T]`, `Query[T]`, `Checkpoint`, `Snapshot`, `Close` and `Stats`; `OpenProject` with `Meta` and `SetMeta`; `OpenRegistry` with the project list. 24 tests and 3 crash tests; `go vet` and `go test ./...` pass on Windows.
- **Format versions:** each file has a list of steps, and its version is the number of steps. The check reads `user_version` on a read-only connection. A test showed why: a read-write open of a newer file that crashed copies its WAL into the file on close.
- **Pre-update backups** use the safe-copy rule. Project files keep theirs in `snapshots/`; `registry.db` keeps its own in `<user data dir>/Jenab/snapshots/` (SPEC 2.1). The last 2 per file are kept. Half-written copies are deleted when the file is opened.
- **Readers:** `query_only`, `SQLITE_LIMIT_ATTACHED = 0` and `SQLITE_LIMIT_LENGTH = 16 MiB` are set again each time a connection is taken from the pool, so a connection someone switched back cannot be used. The writer and the registry also have `SQLITE_LIMIT_ATTACHED = 0`.
- **Crash tests:** the store's SPIKE-008 scenarios, killed 10 times each: chunked writes (whole chunks only), snapshots (8 of 10 kills hit a copy in progress; every copy left is complete) and a format update with a table rebuild (the old version or the new one). The bucket, chats and recovery scenarios move to P1-04 to P1-06. `-short` skips them; `JENAB_CRASH_KILLS` sets the count.
- `project.db` has `_jenab_meta` (key and value) and `_jenab_approvals`; P1-11 may change the approvals columns in step 1 until the first release.
- Not in this ticket: the SQL guard and the typed project writes (Phases 2 and 3), `chats.db`'s tables (P1-06), idle close (P1-04).
