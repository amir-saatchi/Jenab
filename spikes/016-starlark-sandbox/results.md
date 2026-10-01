# SPIKE-016 results

Go go1.26.0, windows/amd64, 12 CPUs, run on 2026-09-28.

## Candidates

| Engine | Module | Version (from the build) | Date | License |
|---|---|---|---|---|
| starlark | `go.starlark.net` | `v0.0.0-20260908191801-89a6a09411d5` | 2026-09-08 (commit 89a6a09411d5, no tags) | BSD-3-Clause |
| goja | `github.com/dop251/goja` | `v0.0.0-20260926152631-39ec2650adc9` | 2026-09-26 (commit 39ec2650adc9, no tags) | MIT |
| gopher-lua | `github.com/yuin/gopher-lua` | `v1.1.2` | 2026-04-01 | MIT |
| risor | `github.com/deepnoodle-ai/risor/v2` | `v2.2.0` | 2026-08-18 | Apache-2.0 |
| tengo | `github.com/d5/tengo/v2` | `v2.17.0` | 2024-02-25 | MIT |

Also: `modernc.org/sqlite` `v1.59.0`, `golang.org/x/sys` `v0.48.0`. `github.com/risor-io/risor/v2` now declares its path as `github.com/deepnoodle-ai/risor/v2`.

**Limits configured** (the best each library offers):
- starlark: `SetMaxExecutionSteps(10000000)`, `Thread.Cancel` on timeout, recursion off (default), `Load` nil
- goja: `Interrupt` on timeout, `SetMaxCallStackSize(1000)`; no step limit exists; `Date` and `Math.random` deleted
- gopher-lua: `SetContext` (cancel), `CallStackSize 1000`, `RegistryMaxSize 256K`; only base, table, string, math opened; `dofile`, `loadfile`, `load`, `loadstring`, `require`, `math.random` removed; no step limit exists
- risor: `WithMaxSteps(10000000)`, `WithMaxStackDepth(1000)`, context cancel; `risor.Builtins()` minus `rand`
- tengo: `SetMaxAllocs(10000000)` (counts objects, not bytes), `MaxStringLen`/`MaxBytesLen` = 16 MiB, `RunContext` cancel; no imports
- all: cancel after 2s. Hostile scripts run in a child process in a Job Object (safety cap 1 GB, hard kill after 10s).

## 1. Hostile scripts with the library limits

Each cell: what stopped the script · time from start until it stopped · peak committed memory of the child process (Job Object `PeakProcessMemoryUsed`, includes the Go runtime and all five engines). "n/a": the language cannot express it.

| Script | starlark | goja | gopher-lua | risor | tengo |
|---|---|---|---|---|---|
| baseline: trivial script | finished → `2` · 296 µs · 47 MB | finished → `2` · 2 ms · 47 MB | finished → `2` · 1 ms · 48 MB | finished → `2` · 633 µs · 47 MB | finished → `2` · 326 µs · 47 MB |
| infinite loop | step limit · 264 ms · 47 MB | timeout cancel · 2.00 s · 47 MB | timeout cancel · 2.00 s · 48 MB | timeout cancel · 2.00 s · 55 MB | timeout cancel · 2.00 s · 47 MB |
| infinite loop, step limit off | timeout cancel · 2.00 s · 47 MB | timeout cancel · 2.00 s · 47 MB | timeout cancel · 2.00 s · 48 MB | timeout cancel · 2.00 s · 55 MB | timeout cancel · 2.00 s · 47 MB |
| `[0] * 10**9` | **job cap 1024 MB** · 24 ms · 1022 MB | **job cap 1024 MB** · 4.77 s · 992 MB | timeout cancel · 2.00 s · 822 MB | **job cap 1024 MB** · 1.86 s · 1005 MB | **job cap 1024 MB** · 1.65 s · 1013 MB |
| `"x" * 10**9` | finished → `1000000000` · 332 ms · 1005 MB | timeout cancel · 3.91 s · 1005 MB | finished → `1000000000` · 318 ms · 1005 MB | finished → `1000000000` · 831 ms · 1005 MB | size limit · 101 µs · 47 MB |
| string doubling, 40 times | **job cap 1024 MB** · 181 ms · 820 MB | **job cap 1024 MB** · 193 ms · 820 MB | **job cap 1024 MB** · 196 ms · 822 MB | **job cap 1024 MB** · 191 ms · 820 MB | size limit · 16 ms · 81 MB |
| dict growth, 10**9 keys | step limit · 959 ms · 207 MB | timeout cancel · 2.00 s · 370 MB | timeout cancel · 2.00 s · 375 MB | step limit · 1.01 s · 210 MB | timeout cancel · 2.00 s · 366 MB |
| sort 2,000,000 items, 100 times | timeout cancel · 2.45 s · 236 MB | timeout cancel · 2.27 s · 334 MB | timeout cancel · 2.00 s · 336 MB | timeout cancel · 2.38 s · 257 MB | n/a |
| deep recursion | recursion rejected · 107 µs · 47 MB | stack limit · 924 µs · 48 MB | stack limit · 8 ms · 48 MB | stack limit · 6 ms · 55 MB | stack limit (recovered Go panic) · 294 µs · 48 MB |
| list nested 10**6 deep, then `str()` | **hard kill at 10s** · 10.03 s · 234 MB | **hard kill at 10s** · 10.08 s · 733 MB | n/a | **job cap 1024 MB** · 770 ms · 882 MB | **hard kill at 10s** · 10.08 s · 679 MB |
| huge integer `2**10**7` | finished → `big int (8388609 bits)` · 20 ms · 56 MB | finished → `big int (10000001 bits)` · 24 ms · 56 MB | finished → `+Inf` · 219 µs · 47 MB | finished → `-9223372036854775808` · 200 µs · 47 MB | finished → `0` · 275 µs · 47 MB |
| integer squared 34 times | timeout cancel · 2.16 s · 482 MB | timeout cancel · 3.14 s · 730 MB | finished → `+Inf` · 257 µs · 48 MB | finished → `0` · 582 µs · 48 MB | finished → `0` · 165 µs · 47 MB |

## 2a. Memory fallback: in-process heap watchdog

A goroutine reads `/memory/classes/heap/objects:bytes` every 1ms and cancels the script above 256 MB. Job safety cap 1 GB.

| Script | starlark | goja | gopher-lua | risor | tengo |
|---|---|---|---|---|---|
| baseline: trivial script | finished → `2` · 52 µs · 47 MB | finished → `2` · 135 µs · 47 MB | finished → `2` · 208 µs · 48 MB | finished → `2` · 274 µs · 47 MB | finished → `2` · 189 µs · 47 MB |
| `[0] * 10**9` | **job cap 1024 MB** · 52 ms · 1022 MB | **job cap 1024 MB** · 4.55 s · 965 MB | watchdog cancel · 788 ms · 374 MB | **job cap 1024 MB** · 1.92 s · 1024 MB | **job cap 1024 MB** · 1.49 s · 1024 MB |
| `"x" * 10**9` | watchdog cancel · 319 ms · 1005 MB | watchdog cancel · 3.85 s · 1005 MB | watchdog cancel · 258 ms · 1006 MB | watchdog cancel · 271 ms · 1006 MB | size limit · 142 µs · 47 MB |
| string doubling, 40 times | watchdog cancel · 171 ms · 564 MB | watchdog cancel · 164 ms · 563 MB | watchdog cancel · 175 ms · 563 MB | watchdog cancel · 92 ms · 307 MB | size limit · 18 ms · 82 MB |
| dict growth, 10**9 keys | step limit · 837 ms · 206 MB | watchdog cancel · 1.82 s · 320 MB | watchdog cancel · 1.50 s · 318 MB | step limit · 1.09 s · 212 MB | timeout cancel · 2.00 s · 317 MB |
| sort 2,000,000 items, 100 times | timeout cancel · 2.13 s · 206 MB | timeout cancel · 2.12 s · 361 MB | timeout cancel · 2.00 s · 307 MB | timeout cancel · 3.30 s · 269 MB | n/a |
| list nested 10**6 deep, then `str()` | **hard kill at 10s** · 10.02 s · 233 MB | watchdog cancel · 371 ms · 325 MB | n/a | **job cap 1024 MB** · 575 ms · 883 MB | **hard kill at 10s** · 10.04 s · 676 MB |
| integer squared 34 times | watchdog cancel · 663 ms · 484 MB | watchdog cancel · 1.64 s · 730 MB | finished → `+Inf` · 199 µs · 48 MB | finished → `0` · 361 µs · 48 MB | finished → `0` · 104 µs · 47 MB |

## 2b. Memory fallback: child process in a Job Object with a 256 MB cap

Library limits only. When the child passes the cap, Windows refuses the allocation and posts a memory-limit message to the job's completion port; the parent then terminates the job. (Without that, a Go child that hit "fatal error: out of memory" sometimes hung until the hard kill.)

| Script | starlark | goja | gopher-lua | risor | tengo |
|---|---|---|---|---|---|
| baseline: trivial script | finished → `2` · 42 µs · 47 MB | finished → `2` · 97 µs · 47 MB | finished → `2` · 113 µs · 47 MB | finished → `2` · 326 µs · 47 MB | finished → `2` · 70 µs · 47 MB |
| `[0] * 10**9` | **job cap 256 MB** · 20 ms · 183 MB | **job cap 256 MB** · 520 ms · 256 MB | **job cap 256 MB** · 406 ms · 256 MB | **job cap 256 MB** · 254 ms · 256 MB | **job cap 256 MB** · 216 ms · 256 MB |
| `"x" * 10**9` | **job cap 256 MB** · 15 ms · 169 MB | **job cap 256 MB** · 16 ms · 168 MB | **job cap 256 MB** · 15 ms · 171 MB | **job cap 256 MB** · 18 ms · 168 MB | size limit · 100 µs · 47 MB |
| string doubling, 40 times | **job cap 256 MB** · 58 ms · 244 MB | **job cap 256 MB** · 57 ms · 254 MB | **job cap 256 MB** · 55 ms · 250 MB | **job cap 256 MB** · 53 ms · 251 MB | size limit · 11 ms · 81 MB |
| dict growth, 10**9 keys | step limit · 793 ms · 207 MB | **job cap 256 MB** · 979 ms · 256 MB | **job cap 256 MB** · 1.16 s · 256 MB | step limit · 1.03 s · 209 MB | **job cap 256 MB** · 1.18 s · 256 MB |
| sort 2,000,000 items, 100 times | timeout cancel · 2.12 s · 236 MB | **job cap 256 MB** · 948 ms · 255 MB | **job cap 256 MB** · 436 ms · 256 MB | **job cap 256 MB** · 1.77 s · 256 MB | n/a |
| list nested 10**6 deep, then `str()` | **hard kill at 10s** · 10.03 s · 232 MB | **job cap 256 MB** · 436 ms · 256 MB | n/a | **job cap 256 MB** · 610 ms · 256 MB | **job cap 256 MB** · 371 ms · 255 MB |
| integer squared 34 times | **job cap 256 MB** · 445 ms · 256 MB | **job cap 256 MB** · 464 ms · 256 MB | finished → `+Inf` · 170 µs · 47 MB | finished → `0` · 366 µs · 47 MB | finished → `0` · 121 µs · 47 MB |

## 3. Reaching outside the sandbox

Each probe runs in-process twice: with the library's default setup ("default") and with the Burrow setup above ("sandbox"). **reached** means the script got a value; values are not shown. Otherwise the cell shows the start of the error.

| Probe | Setup | starlark | goja | gopher-lua | risor | tengo |
|---|---|---|---|---|---|---|
| read a file | default | blocked: script.star:1:10: undefined: open | blocked: ReferenceError: require is not defined at <eval>… | **reached** | blocked: compile error: undefined variable "os" | blocked: Compile Error: module 'os' not found |
| read a file | sandbox | blocked: script.star:1:10: undefined: open | blocked: ReferenceError: require is not defined at <eval>… | blocked: <string>:1: attempt to index a non-table object(… | blocked: compile error: undefined variable "os" | blocked: Compile Error: module 'os' not found |
| network (127.0.0.1:9) | default | blocked: script.star:1:10: undefined: http | blocked: ReferenceError: fetch is not defined at <eval>:1… | blocked: <string>:1: module socket not found: | blocked: compile error: undefined variable "http" | blocked: Compile Error: module 'http' not found |
| network (127.0.0.1:9) | sandbox | blocked: script.star:1:10: undefined: http | blocked: ReferenceError: fetch is not defined at <eval>:1… | blocked: <string>:1: attempt to call a non-function objec… | blocked: compile error: undefined variable "http" | blocked: Compile Error: module 'http' not found |
| environment variable | default | blocked: script.star:1:10: undefined: os | blocked: ReferenceError: process is not defined at <eval>… | **reached** | blocked: compile error: undefined variable "os" | blocked: Compile Error: module 'os' not found |
| environment variable | sandbox | blocked: script.star:1:10: undefined: os | blocked: ReferenceError: process is not defined at <eval>… | blocked: <string>:1: attempt to index a non-table object(… | blocked: compile error: undefined variable "os" | blocked: Compile Error: module 'os' not found |
| current time | default | blocked: script.star:1:10: undefined: time | **reached** | **reached** | blocked: compile error: undefined variable "time" | blocked: Compile Error: module 'times' not found |
| current time | sandbox | blocked: script.star:1:10: undefined: time | blocked: ReferenceError: Date is not defined at <eval>:1:… | blocked: <string>:1: attempt to index a non-table object(… | blocked: compile error: undefined variable "time" | blocked: Compile Error: module 'times' not found |
| random number | default | blocked: script.star:1:10: undefined: random | **reached** | **reached** | **reached** | blocked: Compile Error: module 'rand' not found |
| random number | sandbox | blocked: script.star:1:10: undefined: random | blocked: TypeError: Object has no member 'random' at <eva… | blocked: <string>:1: attempt to call a non-function objec… | blocked: compile error: undefined variable "rand" | blocked: Compile Error: module 'rand' not found |
| load code from a file | default | blocked: load not implemented by this application | blocked: ReferenceError: load is not defined at <eval>:1:… | **reached** | blocked: compile error: undefined variable "load" | blocked: Compile Error: module './win' not found |
| load code from a file | sandbox | blocked: load not implemented by this application | blocked: ReferenceError: load is not defined at <eval>:1:… | blocked: <string>:1: attempt to call a non-function objec… | blocked: compile error: undefined variable "load" | blocked: Compile Error: module './win' not found |
| import a module | default | blocked: script.star:1:1: got import, want primary expres… | blocked: ReferenceError: require is not defined at <eval>… | **reached** | blocked: parse error: unexpected token "os" following sta… | blocked: Compile Error: module 'os' not found |
| import a module | sandbox | blocked: script.star:1:1: got import, want primary expres… | blocked: ReferenceError: require is not defined at <eval>… | blocked: <string>:1: attempt to call a non-function objec… | blocked: parse error: unexpected token "os" following sta… | blocked: Compile Error: module 'os' not found |

## 4. Read-only `db.query` (Starlark, in-memory modernc database)

The reader connection has `query_only`, `SQLITE_LIMIT_ATTACHED = 0` and `SQLITE_LIMIT_LENGTH = 16 MiB`. `db.query` accepts one `SELECT`/`WITH` statement (a simplified SPIKE-007 text check), resets `query_only` before each call, returns at most 100000 rows, and uses the script's context (cancel after 2s).

| Call | Result | Time |
|---|---|---|
| `read 10,000 rows` | ok: 10000 | 29 ms |
| `DELETE` | error: db.query: only SELECT or WITH | 53 µs |
| `PRAGMA query_only = 0` | error: db.query: only SELECT or WITH | 18 µs |
| `WITH … DELETE` | error: attempt to write a readonly database (8) | 167 µs |
| `two statements` | error: db.query: one statement only | 26 µs |
| `ATTACH` | error: db.query: only SELECT or WITH | 13 µs |
| `100 MB blob` | error: string or blob too big (18) | 80 µs |
| `10,000,000 rows` | error: db.query: more than 100000 rows | 125 ms |
| `endless query` | error: context deadline exceeded | 2.00 s |

Rows in `prices` afterwards: 10000.

## 5. Typical transform over 10,000 rows

`db.query` returns 10,000 rows (10 symbols × 1,000 days); the script groups by symbol, averages the price with `round`, and builds a label with `add_days` and `format_date`. Time is for the whole run: new runtime, compile, query, conversion into script values, transform, conversion of the result. 20 runs each.

| Engine | Median | Min | Same output as starlark | First item |
|---|---|---|---|---|
| starlark | 42 ms | 39 ms | yes | `{"avg":105.13,"days":1000,"label":"SYM00: 105.13 until 28.09.2023","symbol":"SYM00"}` |
| goja | 55 ms | 51 ms | yes | `{"avg":105.13,"days":1000,"label":"SYM00: 105.13 until 28.09.2023","symbol":"SYM00"}` |
| gopher-lua | 49 ms | 46 ms | yes | `{"avg":105.13,"days":1000,"label":"SYM00: 105.13 until 28.09.2023","symbol":"SYM00"}` |
| risor | 52 ms | 43 ms | yes | `{"avg":105.13,"days":1000,"label":"SYM00: 105.13 until 28.09.2023","symbol":"SYM00"}` |
| tengo | 43 ms | 33 ms | yes | `{"avg":105.13,"days":1000,"label":"SYM00: 105.13 until 28.09.2023","symbol":"SYM00"}` |

The query alone from Go (scan into Go values): median 20 ms.
Starlark steps for one transform run: 510787 (step limit 10000000).

## 6. Determinism

A map gets 1,000 keys in a scrambled order, then the script lists its keys. 20 runs, each in a fresh runtime. The transform from section 5 also runs 20 times.

| Engine | Distinct key orders | Key order starts with | Distinct transform outputs |
|---|---|---|---|
| starlark | 1 | `[k0 k919 k838 k757 k676 …` | 1 |
| goja | 1 | `[k0 k919 k838 k757 k676 …` | 1 |
| gopher-lua | 1 | `[k0 k919 k838 k757 k676 …` | 1 |
| risor | 1 | `[k0 k1 k10 k100 k101 k10…` | 1 |
| tengo | 20 | `[k266 k57 k608 k215 k439…` | 1 |

## 7. Cost

**Startup per run** (in-process, Burrow setup, new runtime + compile + run `1 + 1` + result; 200 runs):

| Engine | Median | Min |
|---|---|---|
| starlark | 6 µs | 5 µs |
| goja | 8 µs | 6 µs |
| gopher-lua | 94 µs | 53 µs |
| risor | 60 µs | 43 µs |
| tengo | 38 µs | 28 µs |

**Memory limit overhead** (Starlark):

| Setup | Median | Min |
|---|---|---|
| trivial script in a child process + Job Object (start to exit) | 28 ms | 25 ms |
| transform (section 5), no watchdog | 43 ms | 38 ms |
| transform (section 5), watchdog every 1ms | 43 ms | 39 ms |
| one heap sample (`runtime/metrics.Read`) | 1 µs | 1 µs |

**Binary size added** (`go build -trimpath -ldflags="-s -w"` of a program that runs `1 + 1`, minus the same program without an engine):

| Engine | Size | Added |
|---|---|---|
| (none) | 1.7 MB | |
| starlark | 2.9 MB | 1.2 MB |
| goja | 10.3 MB | 8.6 MB |
| gopher-lua | 2.9 MB | 1.3 MB |
| risor | 4.9 MB | 3.3 MB |
| tengo | 2.4 MB | 0.8 MB |

