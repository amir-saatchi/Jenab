# SPIKE-008 — Crash safety
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Does a project stay consistent when the process is killed in the middle of work (SPEC 2.5, 4.3, 7.5)?

Kill the process (hard kill, no shutdown) during:
1. A chunked 100k-row pipeline write
2. A migration with a table rebuild
3. A bucket `put` of a 50 MB file
4. A `VACUUM INTO` snapshot
5. A `chats.db` append while `project.db` is writing

## Done when
Each case is repeated 20 times at random moments. After each restart:
- `PRAGMA integrity_check` and `foreign_key_check` are clean on both databases
- Every committed data write has its change-log entry, and no entry exists without its write
- A migration is fully applied or fully absent
- No index row points to a missing object file; orphan files are found by the sweep
- A half-written snapshot is detected and removed

## Result
Run on 2026-09-27, Windows 11, Go 1.26.0, modernc v1.59.0, WAL with `synchronous = NORMAL`.
- **Method:** a worker process is killed at a random moment with TerminateProcess, 20 times per scenario. After each kill the project is reopened, recovered and checked.
- **Code and full output:** [spikes/008-crash-safety](../../spikes/008-crash-safety/) (`results.md`).

| Scenario | Killed during | Recovery actions | Problems |
|---|---|---|---|
| S1: chunked pipeline write, 500-row transactions | write 17, commit 3 | 20 runs marked `interrupted` | **0** |
| S2: migration with a 200k-row table rebuild | copy 14, commit 5, drop 1 | none needed | **0** |
| S3: bucket `put` of 50 MB files, with deletes | fsync 17, streaming 3 | 20 half-written `tmp/` files removed | **0** |
| S3c: crash exactly after the rename into `objects/`, before the index row | index 20 | 20 orphan files swept | **0** |
| S4a: `VACUUM INTO` straight to the final name (demo) | vacuum 20 | — | **20 broken snapshots** |
| S4b: `VACUUM INTO` a `.partial` file, fsync, rename | vacuum 18, fsync 2 | 20 `.partial` files and 18 `-journal` files removed | **0** |
| S5a: chat message written before the project change | message 9, change 11 | none needed | **0** |
| S5b: project change written before the message (demo) | message 6, change 14 | — | **11 dangling change entries** |

What each scenario checked after every crash:
- **S1:** row count, `rows_total` in meta, run row counts and change rows all agree, and the row count is a whole number of 500-row chunks.
- **S2:** table shape, `schema_version`, the dependent view config and the change log always match. There is never a `prices_new` left behind, and all 200k rows are present.
- **S3:** every index row has a file of the right size, and every file's content matches its SHA-256 name.

**Findings**
- **SQLite's own transactions held in every case.** Chunks, migrations and multi-table writes were always fully there or fully absent.
- **An interrupted `VACUUM INTO` is silently dangerous.** It leaves a file with a hot `-journal`. When the file is opened, SQLite rolls it back to an **empty but valid database** (0 tables, 0 MB), which a restore would accept. The `.partial` + fsync + rename rule fixed it.
- **Order across the two database files matters.** Writing the project change first left a change pointing to a missing message in 11 of 20 crashes, which would break *Undo turn*. Writing the message first gave 0.
- **Recovery is fast:** open + `quick_check` + recovery took 20–70 ms for 1–20 MB and ~0.3 s for a 147 MB database (about 2 ms per MB).
- **Random kills rarely hit very short windows.** The rename-to-index gap in S3 was never hit, so S3c crashes there on purpose.

**Limits:** a process kill is not a power cut, since the OS still writes out its buffers. With `synchronous = NORMAL`, a power cut can lose the last commits but should not corrupt the database. This is not tested here.

## Decision
**Decided (2026-09-27):** keep `synchronous = NORMAL`, and add these rules to SPEC v0.4:
- **Safe-copy rule for every `VACUUM INTO`** (snapshots, export): write to `.partial`, fsync, then rename (7.5).
- **Message before change:** the assistant message and its `tool_call` part go to `chats.db` before any project write that refers to them. Message IDs are ULIDs made in Go (2.3).
- **Crash recovery on open, using `jenab.lock`:**
  1. `quick_check`
  2. mark runs still `running` as `interrupted`
  3. empty `tmp/` and remove `.partial` files
  4. sweep orphan object files (2.7)
- **New run status `interrupted`** (6.2).
