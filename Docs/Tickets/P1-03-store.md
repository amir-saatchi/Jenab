# P1-03 — Store: writers, readers and migrations
**Type:** Feature
**Status:** Open
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
