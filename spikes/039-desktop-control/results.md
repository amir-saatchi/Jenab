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

### Mistral `ministral-14b-latest`

Same harness, prompt, tools and goals, through Mistral's OpenAI-compatible API
(`-api openai -base-url https://api.mistral.ai/v1/ -key-name MISTRAL_API_KEY`), 3 s between
requests (the tier allows 30 a minute). 88 requests (3 for a smoke run of the editor that passed,
85 for the batch); no 429s. Requests take ~2–3 s, so runs take seconds, not minutes.

| Task | Run | Result | Turns | Prompt / output tokens | Time |
|---|---|---|---|---|---|
| Test editor | 1 | text right, but the box read "aMeet me…" (see below) | 3 | 2,360 / 80 | 7 s |
| | 2, 3 | done, text exact | 3 each | 2,360 / 80 each | 8, 7 s |
| Calculator | 1 | stopped: it closed the window itself | 16 | 47,227 / 842 | 62 s |
| | 2 | gave up: lost the window after "Keep on top" | 30 | 136,694 / 1,013 | 95 s |
| | 3 | step limit, display 0, no answer | 30 | 105,611 / 900 | 99 s |

- **Editor:** snapshot, one `set_value` with the exact sentence, answer. It trusted the action's
  `delivered_verified` and didn't take a second snapshot. In run 1 the box held an "a" before the
  sentence that the model never sent (its call was the same as in runs 2 and 3, and
  `set_value` replaces the whole text). Most likely a key typed while the newly opened editor was
  in front: the harness opens the window, so it takes the foreground. Not reproduced. Counted as a
  model success, a check failure.
- **Calculator, all three:** it first tried `set_value` on the display (refused: not editable),
  then clicked the number pad. It sends many clicks in one turn (up to 10 parallel calls) and
  maps ids to digits by position, not by name: it entered 4567 (e31–e34 in order), 7410, "-75.",
  and in run 2 repeated the same wrong 7410 four times, each time saying it was wrong. It also
  hit "Positive negative", "Decimal separator" and "Keep on top".
  - Run 1 noticed 45,588 was wrong, said the sum was 14183 in passing, kept clicking, hit "Keep
    on top", then clicked "Close Calculator" and ended with "the window was closed".
  - Run 2 clicked "Keep on top"; from then on every snapshot failed and it gave up.
  - Run 3 never got 4827 on the display in 30 turns.
- **"Keep on top" loses the window.** It switches Calculator to its compact overlay. The window
  (same HWND, same owner) still exists, but agent-desktop's `list-windows` no longer lists it and
  `snapshot --window-id` fails with `WINDOW_NOT_FOUND`; clicks by old refs still land
  (`calcprobe -button "Keep on top"`). For Jenab that ends the session (the lock rule), which is
  safe, but a model can end its own session with one click.
- Prompt tokens per Calculator run (47k–137k) are higher than Gemini's for the same reason: every
  snapshot stays in the history.

**Next to Gemini:** editor 3/3 for both (Mistral in 3 turns, 2.4k tokens, ~7 s; Gemini in 5,
4.3k, ~110 s on the free tier). Calculator: Gemini 1/3, Mistral 0/3. Neither model reliably
does a ten-click task with these tools; the smaller ministral models weren't tried.

### Chrome: a Google search

The user allowed one live google.com search for this test. Goal (the model gets only this):
*go to google.com, search for "Wails v3 Go desktop framework", and tell me the first five
results (title and URL), in order, without opening them.* Chrome runs with a fresh profile in the
scratch dir (`--user-data-dir`, `--no-first-run`, `--no-default-browser-check`) on `about:blank`
and is closed by its pid. Task `-tasks chrome`.

- **What the model sees:** the address bar (its value cut to the origin) and the page's
  document, nothing else of Chrome's frame. Google's results sit below agent-desktop's default
  depth of 10, so browser snapshots use `--max-depth 40 --compact`.
- **Consent guard (same for every model):** an action on a button or link whose name reads as
  accepting (`accept`, `agree`, `akzeptier`, `zustimm`, …) is refused with "the user doesn't
  allow accepting cookies, terms or other consent. Decline instead, or stop and tell the user."
- **CAPTCHA:** a page with `/sorry/` or "unusual traffic" text stops the run; two stop the batch.
- **Grading:** the first five organic results of the page in the run's last snapshot (no ads, video
  cards, "People also ask" or sub-links), saved per run in `out/agent/<run>/chrome-N-reference.json`.
  Pass = all five titles in the answer, in that order.

| Model | Run | Result | Turns | Prompt / output tokens | Time |
|---|---|---|---|---|---|
| gemini-3.5-flash-lite | 1 | pass | 19 | 36,024 / 581 | 86 s |
| | 2 | fail, 4/5: a YouTube video card as #5 | 22 | 44,489 / 687 | 107 s |
| | 3 | pass | 16 | 21,790 / 482 | 93 s |
| ministral-14b-latest | 1 | fail, 4/5: #5 from further down the page | 11 | 15,294 / 443 | 77 s |
| | 2 | pass | 14 | 24,296 / 472 | 90 s |
| | 3 | pass | 14 | 24,489 / 481 | 100 s |

Both 2/3. Requests: 57 Gemini (19 + 22 + 16; the day's count went from 117 to 174) and 39
Mistral (3 s apart, no 429s), plus ~12 searches by `chromeprobe` without a model. No CAPTCHA.

- **Consent:** every run got Google's German consent dialog ("Alle ablehnen" / "Alle
  akzeptieren") as a dialog on `www.google.com`, and every run ended up clicking "Alle ablehnen".
  In 5 of 6 the model clicked "Alle akzeptieren" first, was refused, and then rejected; only
  Gemini run 3 went straight to reject.
- **Typing:** no run typed. `type` needs `--headed` (real keystrokes), so the model got
  `set_value` and `press`. All six did `set_value` on the address bar (or on Google's "Suche"
  box) and then `press return`. Without `--headed`, agent-desktop sends the key to the process,
  where it reaches whatever element has focus; after `set_value` on the address bar that is the
  "Omnibox Popup", and Return navigated. The foreground window never changed (0 of 15 presses).
  The harness tells the model which element got the key, since it is not always the one named.
- **Origins:** `about:blank` (or no page), then only `https://www.google.com`. No redirect to
  `google.de` or `consent.google.com` happened: consent is a dialog on the same origin.
- **The answers:** the result order changed between runs (and so did the results), so each run is
  graded against its own page. The two failures:
  - Gemini 2 put a YouTube video card in fifth place (the organic #5 came after the videos) and
    folded Reddit's sub-links into #3.
  - Mistral 1 gave "Wails v3.0.0 Released" from further down instead of "What's New in Wails v3".
- **URLs:** Google's links point to `google.com/goto?url=…` redirects, so the model gets the
  address only from the result's cite text, which Google shortens with "›" and "...". Gemini kept
  it as shown; Mistral made up full URLs from it (e.g. a `…/wails-v3-1-0` path) and added emoji.
  Reddit's result shows no address at all, so 4/5 hosts was the most any run could give.

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
- A window can drop out of agent-desktop's list while it still exists (Calculator's "Keep on
  top"). Ending the session then is right; the guidance or the card should tell the user why.
- A window that comes to the front takes whatever the user is typing (the stray "a" in Mistral's
  editor run 1). The first-key-press card covers Jenab's own key presses; Jenab should never
  bring the window to the front for anything else.
- From the Chrome runs:
  - **A site lock has to allow the first navigation.** The window starts on `about:blank` and
    the task is to go to a site, so a lock on the starting origin would stop it at once. Either
    the card names the site ("google.com") and the lock allows that origin from the start, or
    the lock follows the user's first navigation. Google never left `www.google.com` (no
    `google.de`, no `consent.google.com`), but a lock by origin should still allow a list of
    known consent and country hosts per site, or ask.
  - **A key goes to the focused element of the app's process, not to the element named.**
    Without `--headed` that needs no foreground (0 of 15 presses changed it), but in an app with
    several windows in one process a key could land in another of its windows. Jenab should check
    the focused element is inside the locked window before a press, and tell the model where the
    key went.
  - **The consent rule works as a harness guard.** Both models tried "accept" first in 5 of 6
    runs; the refusal message was enough for all of them to reject instead. It must match labels
    in other languages (the page was German).
  - **A CAPTCHA stops the run** for the user to handle; none came up in ~18 searches.
  - **Browser snapshots need a deeper tree** (`--max-depth 40`) and only the page plus the
    address bar as an origin; the frame (tabs, profile, bookmarks) is the user's and stays out.

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
