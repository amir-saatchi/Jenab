# SPIKE-039 — Desktop control with agent-desktop
**Type:** Spike
**Status:** Done (go, narrow v1)
**Gate:** 3
**Needs:** none

## Question
[agent-desktop](https://github.com/vercel-labs/agent-desktop) (Vercel Labs, Apache-2.0, Rust) lets an agent read desktop apps through the accessibility tree and act on their elements by reference, mostly without taking the pointer or focus. It ships as a command-line program with JSON answers, and as a C library. Can Jenab use it, how, and at what cost?

1. **How Jenab runs it,** on Windows: the command-line program as a child process, or the DLL loaded without cgo (`golang.org/x/sys/windows`). Speed, what each can target, and what happens when a call hangs.
2. **Tokens:** how big a snapshot is, as JSON and in a compact one-line-per-element form, for a small app and a busy one.
3. **Can the reference models do it?** A simple task, given as a goal with no steps, in Notepad and in Calculator.
4. **Safety rules:** a draft for the SPEC.

## What VISO found (USR-026, 2026-11-01 and 02)
VISO's spike drove ChatGPT and Gemini in Chrome with agent-desktop 1.0.2 and built a sidecar. Its findings, which this spike checks for Jenab rather than repeats:
- **Command-line program over the library:** the library's `ad_snapshot` targets only the focused window; window targeting, skeleton snapshots and `--root` are not in the C ABI. A child process can be killed when an accessibility call hangs (15–30 s at an app's cold start). Starting it took ~22 ms and a query ~170 ms.
- **Background actions:** click, set-value, expand and toggle work without focus or pointer. Paste and key presses need the window in front (Windows `SendInput`).
- **Cost:** a first-time task took 25–35 calls and 20–40k tokens of tool output; a browser's frame alone was ~6.5k tokens. One line per element cut snapshots to about a third.
- **Safety design:** off until the user turns it on; the user picks one window and every command is locked to it; element IDs only from the tool's own snapshots; a short key list; a time limit; a visible log with *Stop*; the clipboard put back; typing refused on fields with the `secure` state or a password-like name. Screen text is data, never instructions.
- **Quirks:** windows can vanish when an app restarts (`WINDOW_NOT_FOUND`); `find` on big pages needs a longer timeout; "a menu or dialog opened" is reported falsely after some clicks. The binary is unsigned.
- **Its code**, read only, in `D:\DEV\Visual-Image-Gen\viso\apps\desktop\src-tauri\src\mcp\desktop\`: `runner.rs` (the child process and its time limit), `tree.rs` (the compact snapshot), `handoff.rs` and `handoff/tests.rs` (the lock and the checks).

## Scope
1. **Get it:** agent-desktop's latest release from GitHub, the Windows x64 command-line and library archives, checked against `checksums.txt`. Ask the user before downloading, with the file names, source and sizes. Kept in the spike folder, not committed.
2. **Runner:** a small Go runner in `spikes/039-desktop-control/` for both ways, with a time limit that kills a hung call. Measure start, snapshot and action times, and note what the library can't target.
3. **Snapshots:** Notepad, Calculator, File Explorer and one browser window: raw JSON size, a compact form, and tokens.
4. **Model runs:** a few desktop tools (list windows, snapshot, act, wait) for a spike harness, not the app, locked to one window the test opens. Tasks: write a given sentence in Notepad; compute a given sum in Calculator and report it. 3 reps each on `gemini-3.5-flash-lite`. `gemma4:31b` waits for Ollama Cloud's quota (about 2026-11-22). Record steps, tokens, time and where it got stuck.
5. **Safety draft:** the rules Jenab would need, starting from VISO's, with what differs for Jenab: who turns it on, which apps, approval levels (8.8), untrusted screen text (3.7), and what goes to the model.

## Rules
- Only apps the test opens itself (Notepad, Calculator, a test folder in Explorer, a local test page). Never the user's own windows, documents, accounts or browser profile. Close what the spike opened.
- No password fields, sign-ins or real accounts. Nothing typed outside the locked window.
- Model calls only with `GEMINI_API_KEY`, under 400 requests a day; ask before using any other key.
- No new dependencies in Jenab's `go.mod`.

## Done when
- The ticket says which way Jenab should run it and why, with times and token sizes.
- The model runs are recorded, and the safety draft is written.
- A go or no-go, and if go, the ticket that would build it.

## Result
Done on 2026-11-02 with agent-desktop 1.0.2. Code and numbers in `spikes/039-desktop-control/` (`results.md`). **Go, for a narrow v1:** one window the user picks, the user watching, the command-line program as a child process.

- **Command-line program, not the DLL.** It is about 0.3 s slower a call (snapshot 575 ms vs 96–275 ms; click 326 vs 38–106 ms, medians on Calculator), but it has window-pinned refs, `find` and `wait`, and a hung call can be killed. The DLL's app-level snapshot reads the app's focused window (with two windows it read the wrong one), its window-pinned calls return trees without refs, and a stuck call blocks the one thread all its calls share. A frozen window came back `APP_UNRESPONSIVE` or `TIMEOUT` in 3–5 s either way. Headless clicks never took the foreground (10/10).
- **Tokens,** interactive elements, JSON → one line per element: test editor 953 → 252, Calculator 2,935 → 640, File Explorer about 11.3k → 3.4k, an Edge page 15.4k → 4.3k, or 2.4k for the page alone.
- **gemini-3.5-flash-lite, goal only, 3 reps:**
  - Writing a sentence into a test editor (Notepad would reopen the user's saved tabs): 3/3, 5 turns, 4.3k prompt tokens, about 110 s.
  - Calculator, 4827 + 9356: 1/3. One right in 23 turns and 98k tokens, each snapshot staying in the history; one typed 7854 without looking and reported 17,210; one stopped when its lock check failed once (not reproduced).
  - So v1 is for short tasks, with older snapshots as stubs and a line saying what changed after each action.
- **Found for the build:**
  - A UWP app's frame window belongs to ApplicationFrameHost, so the lock uses agent-desktop's process instance, not the window owner's process.
  - Edge's first snapshot at a cold start said complete with an empty page; wait until the page has children.
  - A `file://` page puts the local path, with the Windows user name, into every snapshot. Snapshots should show only the origin. One such path went to Gemini's token counter once, during a size measurement.
- **Safety rules:** written as SPEC 8.10, from VISO's design (USR-026): off by default; only in turns the user starts; the window picked in a card at every level, never terminals, Jenab or system tools; a lock on window and site; IDs only from Jenab's own snapshots; headless actions plus a short key list, the first key press asking; password fields refused; screen text untrusted; compact page-only snapshots; 10 minutes, *Stop* and a log that shows lengths, not text.
- **Decided:** the program ships in Jenab's installer, pinned to a version and SHA-256. TASK-035 builds v1.
- **Open:** gemma4:31b, when Ollama Cloud's quota resets.
- **Requests:** 117 Gemini (`GEMINI_API_KEY`).
