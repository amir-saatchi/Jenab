# TASK-039 — agent-desktop 1.2.0
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-035

## Goal
Jenab pins agent-desktop 1.0.2 (SPEC 8.10). Two releases followed: [1.1.0](https://github.com/vercel-labs/agent-desktop/releases/tag/v1.1.0) (2026-11-03) and [1.2.0](https://github.com/vercel-labs/agent-desktop/releases/tag/v1.2.0) (2026-11-05). Move to 1.2.0 for the changes that make desktop control safer, and check that nothing Jenab relies on changed.

What changed that matters here:
- **Text through standard input** (1.2.0, PR 373): `type`, `set-value` and `clipboard-set` take `--stdin`. Jenab now passes the text of `set_value` on the command line, where other programs on the computer can read it in the process list.
- **Windows refs** (1.1.0, PR 442): a ref acts only if one window read proved it unique and exactly one live element matches it now; otherwise it is refused. No more acting on the row that moved into a removed row's place. Each action reads the window again, so it is slower.
- **Windows crash at exit fixed** (1.1.0, PR 342). Maybe the cause of TASK-035's one unexplained failure; not known.
- **Partial snapshots kept** when the observation time runs out (1.2.0, PR 372).
- **Cursor overlay on Windows** (1.1.0, PRs 343–350, 347): a drawn cursor and caret for headless actions. It shouldn't move the real pointer, but it may draw on screen.
- **`agent-desktop mcp`** (1.2.0, PR 519): an MCP server with 67 tools, one per command. Jenab doesn't use it: the raw tools have none of 8.10's rules.

## Scope
1. **Pin 1.2.0:** download `agent-desktop-v1.2.0-x86_64-pc-windows-msvc.tar.gz` and `checksums.txt` from the GitHub release, check the archive against it, and update the version and SHA-256 in `internal/desktop/desktop.go` and `build/agent-desktop/` (NOTICE, README, LICENSE if it changed).
2. **Text by stdin:** `set_value` (and any other text) goes through `--stdin`, never in the arguments. A test checks the fake runner sees no text in the arguments.
3. **Envelope and errors:** check the envelope version and the error codes Jenab maps (`WINDOW_NOT_FOUND`, `APP_UNRESPONSIVE`, `TIMEOUT`, stale refs, policy). Note any new code, such as a refused ref that isn't unique, and map it to a clear message ("take a new snapshot").
4. **Cursor overlay:** does a headless `click` or `set-value` draw anything on screen, take the pointer or focus, or leave a window behind? If it draws, find the flag that turns it off and use it; if there is none, say so in the report.
5. **Timing:** snapshot, click and set-value times on the test editor and Calculator, next to SPIKE-039's 1.0.2 numbers.
6. **The card when no window matches:** when the agent's `app` matches no pickable window, the card says "Pick a window for the agent to use" and lists the windows, without repeating the agent's text. In a live run gemma-4-26b put a window title or "text editor" in `app`, and the card read "No window of task035 live editor 2 is open". A Go test for the wording.
7. **SPEC 8.7 note** (proposed text in the report): adding agent-desktop's own MCP server would give the agent raw desktop tools without 8.10's rules; the server form warns when the command is agent-desktop and points to the built-in desktop control.

## Rules
- Downloading 1.2.0 from the release page is approved; nothing else.
- Live checks only on windows the test opens (the test editor, Calculator); never the user's windows.
- `gemini-3.5-flash-lite` with `GEMINI_API_KEY` for live runs, under 400 requests a day.
- No new Go dependencies.

## Done when
- Go tests pass with the new pin, including the stdin test.
- Live: the editor task 3/3 on `gemini-3.5-flash-lite`, and Stop once.
- The report says what the overlay does, the timing next to 1.0.2's, any new error codes, and the SPEC 8.10 and 8.7 text.

## Result
Done on 2026-11-05. SPEC 8.7 and 8.10 describe it.

- **Pin:** agent-desktop 1.2.0; the archive matched the release's `checksums.txt`. Its envelope went from 2.5 to 2.6, so the pin alone would have broken every call; the answers Jenab reads have the same shape.
- **Text by stdin:** `set_value` sends its text with `set-value <ref> --stdin` (a new `Runner.RunInput`); a text with NUL is refused. Select's option and `desktop_wait`'s text stay in the arguments: 1.2.0 has no `--stdin` for them.
- **Errors:** the new `AMBIGUOUS_TARGET` and `ELEMENT_NOT_FOUND` tell the model to take a new snapshot; a snapshot with `identity_unproven` says its ids may be refused. Seen live on Calculator from a snapshot cut short; identical twins weren't produced.
- **Cursor overlay:** opt-in per agent-desktop session, and Jenab starts none and drops `AGENT_DESKTOP_*`, so nothing draws. Measured: no new windows, no foreground change, no leftover processes.
- **Faster than 1.0.2** (medians): editor snapshot 178 vs 319 ms, set value 176 vs 239 ms; Calculator snapshot 286 vs 421 ms, click 254 vs 292 ms.
- **Card:** an app that matches no window gets "Pick a window for the agent to use".
- **Live,** gemini-3.5-flash-lite: 3/3 and Stop, and 1/1 with "In Notepad" (the one-window card). A first attempt was 2/3: one run made 6 requests with no reply and no tool call until the 5-minute limit, while its desktop calls were fast. Most likely failed, retried or empty provider answers; the live test prints only the desktop log lines, not the provider's (`provider: call failed`), so it isn't proven. 57 requests.
- **Not done:** the 8.7 warning (the server form is Phase 5); whether the exit-crash fix in 1.1.0 explains TASK-035's one failure.

The session's report: [reports/TASK-039-report.md](reports/TASK-039-report.md).
