# SPIKE-039 results: desktop control with agent-desktop

2026-11-02, Windows 11 Pro 26200, agent-desktop 1.0.2 (CLI and FFI, x64), Go 1.26.10, the
reference model `gemini-3.5-flash-lite` (free tier, native API). Raw output is in `out/`
(git-ignored); how to run it is in `README.md`.

Notepad was replaced by the spike's own test editor (a plain Win32 window with a menu, a
multi-line Edit box, two buttons and a character count): Notepad on this machine has saved tabs
it would reopen, and their text would have gone into snapshots and to the model.

## 1. How Jenab runs it: CLI or DLL

Times on Calculator (10 runs each; median, p90):

| Call | CLI (child process) | DLL (`syscall.LazyDLL`, no cgo, one locked thread) |
|---|---|---|
| Start | process start + `version`: 38, 64 ms | load + `ad_init` + adapter: 5 ms, once |
| List windows | `list-windows --app`: 134, 163 ms | `ad_list_windows_exact`: 90, 117 ms |
| Snapshot, interactive | `snapshot --window-id -i`: 575, 604 ms | `ad_snapshot(app)`: 275, 314 ms; `ad_get_tree_exact(window)`: 96, 105 ms |
| Click, headless | `click <ref>`: 326, 408 ms | `ad_execute_by_ref` (no auto-wait): 38, 44 ms; `ad_find_exact` + action: 106, 107 ms |

- The CLI pays ~250–300 ms per call over the DLL: a fresh process has to start UI Automation and
  load the ref map. Next to a model turn (1–3 s) that is small.
- Clicks landed both ways (the display read 7,777,777,777 after 10 CLI clicks on "Seven"), the
  foreground did not change in 10 of 10 clicks, and the pointer never moved.

**What each can target.** VISO's "the library can't target a window" is half true in 1.0.2:
- `ad_snapshot` takes an app name and reads that app's *focused* window. With two editor windows
  of one app open, it read B (in front) when A was wanted. No window id, no `--root`, no skeleton.
- But the library also has a window-pinned family: `ad_list_windows_exact` →
  `ad_get_tree_exact(window)` → `ad_find_exact(window, role, name)` →
  `ad_execute_action_with_policy(handle)`. It read and clicked the right window (A) here. Its tree
  has **no refs**: the caller must find elements again by role and name (and nth), so Jenab would
  rebuild agent-desktop's ref machinery (identity checks, stale-ref detection, actionability
  waits) itself.
- The CLI has all of it: `--window-id`, refs that pin a snapshot, `find --window-id`, `--root`,
  `--skeleton`, `wait`, and the JSON envelope with `disposition.delivery/retry`.

**When a call hangs.**
- A frozen window (the test editor blocked its UI thread for 40 s): agent-desktop noticed it
  itself. A snapshot failed with `APP_UNRESPONSIVE` after 3.2 s (0.4 s on later calls), and a click
  on a button of the frozen window gave up after its 5 s budget (`TIMEOUT`). The same through the
  DLL (`-16 APP_UNRESPONSIVE`, 5.0 s). Nothing hung.
- Edge at a cold start: the first snapshot (at +0.8 s) came back `complete: true` with an empty
  page ("Chrome Legacy Window" with no children); from the second (+2 s) the page was there
  (142 nodes). So `complete` does not mean the page has loaded: Jenab must check that the page
  document has children, and snapshot again if not.
- The 15–30 s hangs VISO saw (an Electron app's cold start) did not happen with these apps. The
  difference stays: a child process can be killed at any time; a DLL call cannot be stopped, and
  it holds the one thread all DLL calls must use until it returns.

## 2. Tokens

Tokens are Gemini's `countTokens`; Explorer's are estimated (~) from the others' ratio, since its
navigation pane shows the user's own folders and its text was not sent.

| App | Form | Bytes | Lines | Tokens |
|---|---|---|---|---|
| Test editor | JSON, full / interactive | 3,412 / 3,276 | | 1,029 / 953 |
| | compact, interactive | 599 | 15 | 252 |
| Calculator | JSON, full / interactive | 14,419 / 9,943 | | 4,067 / 2,935 |
| | compact, full / interactive | 2,330 / 1,486 | 80 / 46 | 972 / 640 |
| File Explorer (test folder) | JSON, full / interactive | 47,477 / 37,411 | | ~13,057 / ~11,264 |
| | compact, full / interactive | 10,165 / 8,014 | 217 / 165 | ~4,180 / ~3,436 |
| Edge, local test page | JSON, full / interactive | 85,402 / 50,823 | | 23,295 / 15,395 |
| | compact, full / interactive | 11,460 / 10,019 | 285 / 246 | 4,697 / 4,298 |
| | page only, compact, full / interactive | 5,805 / 4,849 | 161 / 132 | 2,702 / 2,405 |

- The compact form (`- button "Send" [3:e14] (disabled)`, indented, unnamed containers dropped,
  link URLs dropped) is **a quarter to a fifth** of the JSON (VISO had a third; the URLs and
  nesting are what go).
- Edge's frame (tabs, address bar, toolbar, favorites bar, side pane) is ~1.9k tokens compact on a
  fresh profile. It holds the address, the tab titles and, on a real profile, favorites, the
  profile name and extensions. Page-only is half the window.
- "Interactive only" keeps the Calculator display and the editor's text (both are elements with
  refs) but drops a page's plain text (headings, paragraphs, labels). A page needs the full form
  to be read; a form-filling step needs only the interactive one.

## 3. The reference model

`gemini-3.5-flash-lite`, goal only, 3 runs per task, 30 turns at most. The tools: `list_windows`
(names the one window the run was locked to), `snapshot` (the compact form, interactive only),
`act {id, action, text, key}` and `wait {text, gone, seconds}`. The prompt is generic: no
app-specific hints. Time is wall time; nearly all of it is the model (free tier, ~20–25 s a
request with 503 retries); the tools took 1–9 s per run.

| Task | Run | Result | Turns | Prompt / output tokens | Time |
|---|---|---|---|---|---|
| Test editor: "write this sentence: Meet me at the north gate at 7:30, and bring the blue folder." | 1–3 | done, text exact | 5 each | 4,265 / 86 each | 124, 107, 102 s |
| Calculator: "work out 4827 + 9356, and tell me the result" | 1 | stopped: lock lost | 6 | 6,394 / 74 | 131 s |
| | 2 | done: 14,183 | 23 | 98,101 / 364 | 447 s |
| | 3 | **wrong: 17,210** | 28 | 79,079 / 579 | 427 s |

- **Editor:** the same five turns every time: list windows, snapshot, one `set_value` on the text
  box, snapshot to check, answer.
- **Calculator 2:** ten clicks (4 8 2 7 + 9 3 5 6 =), with a snapshot after every click. Right,
  but each snapshot stays in the history: the prompt grows by ~700 tokens a turn and reaches
  98k for one sum. With older snapshots replaced by stubs it would be about a third (estimate).
- **Calculator 3:** it clicked Seven, Eight, Five, Four (7854) where it meant 4 8 2 7, sent all
  ten clicks without looking, saw 17,210, pressed Clear, sent the same ten clicks again and
  reported 17,210. It never compared the display with what it meant to enter. A small model
  mixes up ids in a long list of similar buttons; clicks that report only "delivered" give it
  nothing to notice the mistake with.
- **Calculator 1:** after the first click the lock check failed (the window was gone, or its
  owner had changed); the harness refused the next call, and the model stopped and said so, as
  told. Not reproduced: in five cold starts with the same click (`calcprobe`), the window, its
  owner and agent-desktop's process instance stayed the same for 65 s. Unexplained.
- An earlier Calculator batch failed for a harness bug, not the model: the lock compared the
  window's owner (`GetWindowThreadProcessId`) with the pid list-windows reports. For a UWP app the
  frame window belongs to **ApplicationFrameHost**, which hosts every UWP app's frame; the app's
  own process is `CalculatorApp`. So the lock has to use agent-desktop's `process_instance` (or
  the owner seen at the start), never "the owner pid equals the app's pid".
- No run pressed a key, so nothing took the foreground. 117 Gemini requests were used today in
  all (sizes, both agent batches).

## 4. Safety

The SPEC draft (8.10) is in the report to Main-Session. What the spike showed for it:
- The window lock worked as meant: in Calculator 1 the run stopped instead of acting on
  something else.
- The lock must not use the window owner's pid alone (UWP, above). Window id plus
  `process_instance` from agent-desktop works.
- A `file://` page puts the local path (with the Windows user name) into every snapshot: the
  document's value is the URL. The first Edge size measurement sent such a path to Gemini's
  `countTokens` once; the page is now served from `127.0.0.1`. Jenab's page-only form should
  keep the origin, not the full URL, in what it shows the model.
- agent-desktop refuses a state folder that others can read; Jenab should let it create its own.

## 5. Go or no-go

**Go, for a narrow v1**: one user-picked window, watched by the user, with the CLI as a child
process.
- The plumbing is sound: headless actions that don't take the foreground, window-pinned refs,
  hangs reported in 3–5 s instead of hanging, and a process Jenab can kill.
- Tokens are fine with the compact form (250 tokens for a small app, ~650 for Calculator,
  ~2.4k for a web page), if older snapshots become stubs.
- The reference model does a one-step task (fill a text box) reliably, but a ten-click task only
  1 in 3. v1 should not promise more than short tasks, and should give the model the feedback it
  lacked: after each action, a short line of what changed in the window ("Display is 4,827").

## 6. The ticket to build it

See the report: a TASK for desktop control v1 on Windows (tools, card, lock, compact form,
change line after actions, limits, log), with the CLI pinned by SHA-256.
