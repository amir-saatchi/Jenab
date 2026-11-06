# TASK-039 report: agent-desktop 1.2.0

2026-11-06, Windows 11 Pro 26200, Go 1.26, branch `claude/admiring-mayer-e7998f`. Not committed.

## Summary

- **Pinned 1.2.0.** `agent-desktop-v1.2.0-x86_64-pc-windows-msvc.tar.gz` (1,759,368 bytes) matched `checksums.txt` (`a4199f741a441cedd349bf56c5eba01783b1261258b498efeb7ec9945220c3e8`). The archive holds only `agent-desktop.exe`, SHA-256 `d274667031322e50b9c2b1ab81c2ca46835d52cc932769a5c34bd4d481fd94c3`. No LICENSE in the archive, so `build/agent-desktop/LICENSE` (Apache-2.0) is unchanged; NOTICE and README are updated.
- **The envelope version changed from 2.5 to 2.6.** Jenab refuses any other version, so the pin alone would have broken every call. `envelopeVersion` is now 2.6. Side by side on the same windows, every answer Jenab reads has the same shape in 2.6 (list-windows, snapshot, click, set-value, find, get, press, the error envelopes). The only differences: `version`, and set-value with `--stdin` no longer echoes `post_state.value`.
- **set_value's text goes through standard input** (`set-value <ref> --stdin`), never in the arguments. `Runner` has a new `RunInput`. A text with a NUL character is refused before the call, since agent-desktop rejects NUL on stdin. Live check: Persian text, CR LF and a trailing newline all arrived exactly as sent.
- **New error code `AMBIGUOUS_TARGET`** (1.1.0's Windows refs) is mapped to "take a new snapshot". `ELEMENT_NOT_FOUND` now gets the same message as `STALE_REF`. Errors now carry `details.kind`. A snapshot with `identity_unproven: true` tells the model its ids may be refused.
- **Cursor overlay:** it is opt-in and draws nothing for Jenab. No flag is needed (details below).
- **Timing:** 1.2.0 is faster than 1.0.2 on every call measured. The extra live read that 1.1.0 added to each action costs less than what the release saved elsewhere.
- **Pick card:** when the agent's `app` matches no window, the card now says "Pick a window for the agent to use" and lists the windows (group `other`, unfolded by the UI). It doesn't repeat the agent's text.
- **Live:** see "Live runs" below.

## What changed in the code

| File | Change |
|---|---|
| `internal/desktop/desktop.go` | `Version` 1.2.0, `SHA256`, `envelopeVersion` 2.6; `Runner.RunInput`; `CodeAmbiguous`, `CodeElementNotFound`; `Error.Kind` from `details.kind` (read loosely: details that aren't an object still parse) |
| `internal/desktop/cli.go` | `RunInput`: the input goes to the process through a pipe (agent-desktop refuses a terminal); `Run` gives the null device |
| `internal/desktop/tools.go` | `set_value` → `set-value <ref> --stdin` with the text on stdin; NUL refused; plain card ask when no window matches; snapshot note for `identity_unproven` |
| `internal/desktop/session.go` | `fail`: `AMBIGUOUS_TARGET` → "not done … take a new snapshot …; if the same element is refused again, use another one"; `ELEMENT_NOT_FOUND` with the stale refs |
| `internal/desktop/snapshot.go` | `snapData.Unproven` (`identity_unproven`) |
| tests | fake runner: `RunInput`, and its `set-value` refuses a value in the arguments (like 1.2.0 with `--stdin`); `TestSetValueTextGoesByStdin` (no text in any call's arguments, empty text, NUL); `TestCLIStdin` (the real runner with the test binary as agent-desktop: stdin through a pipe, nothing in the arguments); `TestCLIEnvelope` (2.6, `AMBIGUOUS_TARGET` with its kind, details that aren't an object); `TestPickCardDoesNotRepeatTheAgentsApp`; `TestPickCardByApp` wording; `TestUnprovenSnapshot`; `TestErrorsReachTheModel` (`AMBIGUOUS_TARGET`, `ELEMENT_NOT_FOUND`) |
| `build/agent-desktop/NOTICE`, `README.md` | 1.2.0, the archive's and the program's SHA-256 |

**Text still in the arguments:** `select`'s option name and `desktop_wait`'s text (`find --text`). In 1.2.0 only `type`, `set-value` and `clipboard-set` take `--stdin`. Both are text from the screen (an option's name, text the agent waits for), not text the agent writes, so I left them as they are and say so in a comment.

## Envelope and error codes

Checked side by side (1.0.2 and 1.2.0, the same editor window), and against 1.2.0's bundled docs (`agent-desktop skills get desktop …`):

| Code | In 1.2.0 | Jenab |
|---|---|---|
| `WINDOW_NOT_FOUND` | same (live: `snapshot --window-id w-1`) | ends the session, as before |
| `APP_UNRESPONSIVE` | same | as before |
| `TIMEOUT` | same; new `details.kind`: `deadline`, `actionability_timeout`, `resolution_identity_unknown` (nothing sent, retry) | as before: "try again, or take a new snapshot" |
| `STALE_REF` | same; `details.kind: saved_ref_absent` | as before |
| `SNAPSHOT_NOT_FOUND` | same (live: a made-up snapshot id) | as before |
| `POLICY_DENIED` | same | as before |
| **`AMBIGUOUS_TARGET`** | **new for refs (1.1.0):** `identity_not_unique` (the ref wasn't proven unique when its snapshot was read: an identical element elsewhere in the window, or a snapshot cut short) or `saved_ref_ambiguous` (a second match appeared). Nothing is sent (`not_delivered`). | "not done: agent-desktop can't tell that element apart from another just like it (…). Take a new snapshot and use an id from it; if the same element is refused again, use another one." |
| `ELEMENT_NOT_FOUND` | listed in the docs | now with the stale-ref message (before: the raw code) |
| `ACTION_FAILED` + `post_action_verification` | a set-value the app reformats reads as failed, delivered, `retry: unsafe` | unchanged: the raw code plus "it may have reached the app: take a snapshot before trying again" |

**Live check of the new code:** a Calculator snapshot cut short (`--timeout-ms 150`) came back `ok`, `complete: false`, `truncated: true`, `identity_unproven: true`, 8 nodes, 4 refs. A click on one of its refs: `AMBIGUOUS_TARGET` / `identity_not_unique`. With `--timeout-ms` 1 or 50 the snapshot is `TIMEOUT` / `deadline` with no tree. Jenab asks for 12 s (27 s for the first snapshot), so this is rare. When it happens, the snapshot text now ends with "[the tree is incomplete: the app didn't answer in time, and its ids may be refused. Take a new snapshot before you act]".

**Partial snapshots (PR 372):** Jenab already read `complete` and already says when the tree is incomplete. A partial tree that keeps its refs usable (`complete: false` without `identity_unproven`) keeps the old note.

## Cursor overlay

- **What it is:** a separate renderer process paints a cursor, a ripple and a caret in a click-through, topmost, non-activating window (1.2.0 docs, `skills get windows references/shell-and-overlay.md`).
- **It is opt-in, per agent-desktop session:** it starts only with `session start --cursor` or `cursor-overlay enable` inside a session (`--session` or `AGENT_DESKTOP_SESSION`). Jenab never starts a session, and the runner already strips every `AGENT_DESKTOP_*` variable from the environment (`TestCLIEnvironment`), so a session set outside can't turn it on. The state folder is Jenab's own private one, so another program can't enable it there either.
- **Measured, 1.2.0, headless, no session:** 10 set-values and 10 clicks on the test editor, 10 clicks on Calculator, and set-value plus press on a background editor. No new visible top-level window appeared, the foreground window didn't change, no `agent-desktop.exe` process was left running after any call, and no window was left behind. The pointer position moved at a few sampled points with both 1.0.2 and 1.2.0, at different calls each run, so that was my own mouse on the machine in use, not agent-desktop. Headless actions are documented as never moving the cursor.
- **So there is no flag to set**, and none exists for the global namespace: `cursor-overlay disable` only works inside a session. If Jenab ever uses sessions (for traces, say), it must not pass `--cursor` and should run `cursor-overlay disable` in each one.
- Its cost, from the docs: about 355 ms per action when on.

## Timing

Same machine and windows, run one after the other, 10 calls each, median and p90 in ms. Each call starts a new process, as Jenab does. Both versions have their own fresh state folder. SPIKE-039's numbers (1.0.2, 2026-11-02, Calculator only) are given for reference; the machine was busier then.

| Call | SPIKE-039, 1.0.2 | 1.0.2 now | 1.2.0 now |
|---|---|---|---|
| **Test editor** | | | |
| `list-windows --app` | — | 85 / 96 | 72 / 81 |
| `snapshot --window-id -i` | — | 319 / 343 | 178 / 192 |
| `set-value` (1.0.2: text in the arguments; 1.2.0: `--stdin`) | — | 239 / 267 | 176 / 289 |
| `click` (Upper case) | — | 237 / 256 | 193 / 278 |
| `find --state focused --last` | — | 313 / 660 | 183 / 231 |
| `get --property states` | — | 205 / 429 | 137 / 253 |
| **Calculator** | | | |
| `list-windows --app CalculatorApp` | 134 / 163 | 84 / 106 | 78 / 105 |
| `snapshot --window-id -i` | 575 / 604 | 421 / 598 | 286 / 433 |
| `click` (Seven) | 326 / 408 | 292 / 378 | 254 / 377 |

- 1.1.0's per-action live read (one window read before each ref action) shows as a longer interaction lease (`lease_hold_ms` about 110 ms vs 50 ms in 1.0.2), but the call as a whole is faster: process start and UI Automation setup got cheaper.
- In Jenab during the live runs (1.2.0): a `desktop_snapshot` took 287–319 ms (with the window check). A `desktop_act` set_value took 690–707 ms in total: window check, `get states`, `set-value --stdin`, and the snapshot for the change line.
- Calculator after 10 clicks on Seven read 7,777,777,777 with both versions; set-value via stdin left exactly the text sent in the editor (checked with `WM_GETTEXT`), 10 of 10.

## Press to a background window

Two test editors (A and B, both started by the probe). Set A's text, then press Backspace to A while A was not in front. 1.0.2 and 1.2.0 both answered `ok` (`delivered_unverified`), and A's text was unchanged with both: `set-value` doesn't give the field focus. No change between the versions. The docs now say a headless press to an app that isn't in front fails closed on Windows. Here neither version returned an error, but the key didn't land either. Jenab's rule (a key goes only to the focused element of the locked window, checked first) stays as it is.

## Live runs

`TestDesktopLive` with `gemini-3.5-flash-lite`, `GEMINI_API_KEY`, the real 1.2.0 binary and SPIKE-039's test editor, at Standard. 57 requests in total, between 21:00 and 21:40 UTC on 2026-11-05.

**Final run: 3/3 and Stop passed** (`--- PASS: TestDesktopLive (133.93s)`):

| Run | OK | Requests | Tool calls | Time | Card ask |
|---|---|---|---|---|---|
| 1 | yes | 6 | load_skill, start, snapshot, act, end | 14.7 s | "Pick a window for the agent to use" (no app) |
| 2 | yes | 6 | same | 8.4 s | same |
| 3 | yes | 8 | same | 80.6 s | same |
| Stop | stopped after the pick, the session ended with the turn | 4 | load_skill, start, snapshot | 28.7 s | same |

**A run with the task naming "Notepad"** (`JENAB_DESKTOP_APP=Notepad`, the editor copied as `notepad.exe` in the scratchpad, never the real Notepad): 1/1, 5 requests, 9.3 s. The card asked "Let the agent use TASK035 Live Editor 1 — notepad?" (group `match`).

**The first attempt (before the final run) was 2/3, with Stop passing:**
- Run 1: OK, 6 requests, 18.3 s.
- Run 2: failed. The model passed `app: "notepad"`, which matched no window (the editor's process is `spike039`). The card read "Pick a window for the agent to use", the new wording, where 1.0.2's build said "No window of notepad is open: …". The test picked the editor from the `other` group. The model took one full snapshot in 319 ms, then made 6 more requests that returned no tool call and no text, until the test's 5-minute limit.
- Run 3: OK, but 4 min 15 s for 9 requests, while its desktop calls took under 1 s in total.

All desktop calls in both attempts succeeded and were fast: snapshots 287–319 ms, set_value 690–707 ms. So the stall was on the model's side. My best reading is failed or retried Gemini requests (free tier): the test counts every `Stream` call, but the agent doesn't log provider errors, so I couldn't see the cause, and this is not proven. The second attempt, a few minutes later, had one slow run (80 s, 2 extra requests) and no failure. I didn't change anything between the attempts except a temporary line in the test that printed the whole log, which showed nothing beyond the desktop lines; that line is reverted.

`TestCLILive` with the 1.2.0 binary: passed (hash check, `version` reads 1.2.0, the state folder it made is private).

## Proposed SPEC text

### 8.10 Desktop control

In **Picking the window**, replace the "None" bullet:

> - **None,** or an unknown or denied app: "Pick a window for the agent to use", with all the windows unfolded. The card doesn't repeat the agent's app, which is then often a window title or a description ("text editor"; TASK-039).

In **What the agent can do**, after "Actions run without taking the pointer or focus: …":

> - The text of a set value goes to agent-desktop on standard input (`--stdin`), never in its arguments, which other programs can read in the process list. A text with a NUL character is refused. An option's name for select, and the text of `desktop_wait`, are text from the screen and stay in the arguments: agent-desktop has no `--stdin` for them.
> - A ref acts only if agent-desktop proved it unique in the window when the snapshot was read, and exactly one live element matches it now (1.1.0). Otherwise nothing is sent (`AMBIGUOUS_TARGET`), and the model is told to take a new snapshot and, if the element is refused again, to use another one. A snapshot cut short says its ids may be refused.

In **How Jenab runs it**, after "The program ships in Jenab's installer, pinned to a version …":

> - Pinned to 1.2.0 (TASK-039), envelope version 2.6. Jenab never starts an agent-desktop session, so the cursor overlay (1.1.0, opt-in per session) never draws, and the runner drops every `AGENT_DESKTOP_*` variable, so one set outside can't start it. agent-desktop's own MCP server (`agent-desktop mcp`, 1.2.0) is not used: its tools have none of these rules (8.7).

### 8.7 MCP servers

In **Adding**, a new bullet:

> - **agent-desktop's own server** (`agent-desktop mcp`, 67 tools, one per command) would give the agent raw desktop tools without 8.10's rules: no window the user picks, no lock, no deny list, consent, password or CAPTCHA guards, no approvals, and text in the arguments. When the command (or an argument, such as `npx agent-desktop`) is agent-desktop, the server form warns: "This gives the agent control of every window on this computer, without the rules of Jenab's desktop control. Use *Settings → Let agents use other apps' windows* instead." Adding it stays possible after the warning; Jenab doesn't block it.

(The warning is not built in this task; 8.7's server form is Phase 5.)

## Checks

- `go build ./...`: OK. The worktree's `frontend/dist` (git-ignored) was copied from the main checkout's build so `go:embed` has something to embed. The frontend is not touched, so `bun` checks weren't needed.
- `go vet ./...`: OK.
- `go test -p 2 -short ./...` (`GOMEMLIMIT=1500MiB`, `GOPROXY=off`): all OK.
- `gofmt` on the changed Go files: clean.
- `go run ./cmd/jenab-scenarios check scenarios`: all OK.
- No new Go dependencies. `go.mod` and `go.sum` are unchanged.
- Downloads: only the 1.2.0 Windows x86_64 CLI archive and `checksums.txt`, into an empty scratchpad folder, outside the repo. The binary is not in git.
- Processes: only my own test editors and one Calculator (not running before) were started, each closed or killed by PID. Nothing is left running.

## Not done or not covered

- The 8.7 warning is only proposed (no server form yet).
- `AMBIGUOUS_TARGET` with real identical twins (`saved_ref_ambiguous`) wasn't produced live: the test editor and Calculator have none. `identity_not_unique` was produced live, from a snapshot cut short.
- Whether 1.1.0's exit-crash fix (PR 342) was TASK-035's one unexplained failure can't be shown; it didn't recur in these runs.
- The `mcp` command was not run.
