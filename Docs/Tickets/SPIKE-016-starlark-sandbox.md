# SPIKE-016 — Script sandbox for `script.starlark`
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Can `go.starlark.net` run the `script.starlark` step safely (SPEC 6.5), or is another embedded language better?

1. Step limit and cancel stop runaway scripts
2. Memory: can a script exhaust memory (`[0] * 10**9`, long strings), and can we stop it?
3. Deep recursion and huge integers
4. Only the functions we provide: no files, network, time or randomness
5. A read-only `db.query` and the transform functions can be exposed
6. Speed for typical transforms over 10,000 rows

Candidates: `go.starlark.net`, `dop251/goja` (JavaScript), `yuin/gopher-lua`, `risor`, `tengo`.

## Done when
Each candidate passes or fails a set of hostile scripts, and one is chosen, along with any limits Go must add.

## Result
Run on 2026-09-28 on Windows. Code and full output: `spikes/016-starlark-sandbox/` (`results.md`). All five candidates are pure Go (no cgo).

| Engine | Version (date), license | Limits it offers | Escapes by default | Same output every run |
|---|---|---|---|---|
| `go.starlark.net` | commit 89a6a09411d5 (2026-09-08), BSD-3 | step limit, cancel, no recursion | none | yes |
| `dop251/goja` | commit 39ec2650adc9 (2026-09-26), MIT | interrupt, stack limit | time, random | yes |
| `yuin/gopher-lua` | v1.1.2 (2026-04-01), MIT | context cancel, stack limit | files, environment, clock, random, loading code | yes |
| `deepnoodle-ai/risor/v2` | v2.2.0 (2026-08-18), Apache-2.0 | step limit, cancel | random | yes |
| `d5/tengo/v2` | v2.17.0 (2024-02-25), MIT | object count, string size | none | **no** (20 key orders in 20 runs) |

**Hostile scripts**
- **No engine limits memory in bytes.** With only the library limits:
  - `[0] * 10**9` and a string-doubling loop reached the 1 GB safety cap in four or five engines.
  - `"x" * 10**9` finished in three of them, holding 1 GB.
- **An in-process heap watchdog (256 MB) is not enough.** A single large allocation is done before the watchdog can cancel.
- **A child process in a Windows Job Object capped at 256 MB stopped every memory bomb in every engine**, within 1.18 s. It costs about 28 ms per run.
  - Gotcha: a Go child that runs out of memory inside the job sometimes hangs. The parent must wait for the job's memory-limit message and terminate the job.
- **Starlark:**
  - The step limit stopped the endless loop in 264 ms and dict growth in 1 s.
  - Recursion is rejected at once.
  - Some builtins ignore cancel:
    - `str()` on a list nested 10^6 deep ran until the 10 s hard kill.
    - `sorted` and big-integer multiplication overran the 2 s cancel by about 0.5 s.
- **Read-only `db.query`** blocked writes, PRAGMA, ATTACH and two statements. The table kept its 10,000 rows. The 100 MB blob, the 10M-row result and the endless query were all stopped.
- **Typical transform over 10,000 rows:** 42 ms and 510,787 steps in Starlark (33–55 ms across engines).
- **Cost:** start-up takes 6 µs, and the binary grows by 1.2 MB (goja: 8.6 MB).

**Not tested:**
- how the child process reaches `project.db`
- a real WAL database with the full SPIKE-007 guard
- Linux and macOS, which have no Job Objects
- `debug.SetMemoryLimit`
- scripts running in parallel
- limits on result size and nesting depth

## Decision
**Decided (2026-09-28):** `go.starlark.net`, pinned to a commit, run in a child process.
- **Memory:** a Job Object capped at 256 MB. The parent terminates the job when the memory-limit message arrives.
- **Steps:** `SetMaxExecutionSteps(10_000_000)`, about 20 times the typical transform.
- **Time:** `Thread.Cancel` at the step timeout; the parent hard-kills the job shortly after, since some builtins ignore cancel.
- **Sandbox:**
  - recursion off
  - no `load()`
  - only `db.query` and the transform functions available
  - `print` captured into the run log
- **`db.query`:**
  - the full SPIKE-007 guard, with `query_only` on every call
  - at most 100,000 rows
  - `SQLITE_LIMIT_LENGTH` of 16 MiB
  - `SQLITE_LIMIT_ATTACHED` of 0
**Decided later (2026-09-28), for the two open points:**
- **How the child reads `project.db`: it asks the parent.**
  - The child never opens the database file. `db.query` sends the SQL and parameters over the child's stdin/stdout pipe.
  - The parent runs the query on its own guarded reader pool: the SPIKE-007 guard, `query_only`, timeouts and schema checks.
  - The parent streams the rows back, capped at 100,000.
  - The reasons:
    - the safety rules stay in one place
    - the process that runs agent-written code needs no file access, so it can be locked down further later
  - The copying cost is measured during implementation.
- **Memory cap on macOS and Linux: a soft limit plus the parent's check.**
  - The child sets `debug.SetMemoryLimit` to about 200 MB.
  - The parent reads the child's memory about every 100 ms and kills it above 256 MB.
  - On Linux, a generous `RLIMIT_AS` (1–2 GB) is added as a backstop. A tight limit breaks Go, which reserves virtual memory upfront.
  - Windows keeps the Job Object as the hard cap.
  - **Known gap:** on macOS, one large allocation can go over 256 MB for up to one check interval before the kill.
  - To test before Phase 5: Linux (possible in WSL on the dev machine) and macOS.
