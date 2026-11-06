# TASK-035 — Desktop control v1 (Windows)
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** SPIKE-039

## Goal
Build SPEC 8.10: an agent reads and uses one window of another app the user picks, through agent-desktop's command-line program, with the rules SPIKE-039 drafted.

## Scope
1. **Runner** (`internal/desktop`): starts the pinned agent-desktop program with its SHA-256 checked first, with no console window, kills a call after 15 s (30 s for a first snapshot), parses its JSON envelope (version 2.5) and maps its errors (`WINDOW_NOT_FOUND`, `APP_UNRESPONSIVE`, `TIMEOUT`). The state folder gets a private ACL. A fake runner for tests.
2. **Shipping:** the program and its NOTICE in the installer, with the version and SHA-256 in one place in the code. No download at run time.
3. **Tools:** `desktop_start`, `desktop_snapshot`, `desktop_act`, `desktop_wait`, `desktop_end`, offered only when the Settings switch is on and the turn was started by the user in a chat.
4. **The pick card** with the deny list, the lock (window id, process instance, the browser's origin, a blank page taking its first site; a window agent-desktop stops listing ends the session) and the approval rows of 8.10, including the first-key card.
5. **Snapshots:** the compact one-line form, interactive by default, at most 400 lines; for browsers the page to a depth of 40 plus the address bar as an origin; wait until the page has children; older snapshots as stubs; a change line after each action.
6. **Safety:** password fields refused; accepting consent refused (English, German, Persian); a CAPTCHA ends the session; before a key press, the focused element checked to be in the locked window; screen text marked untrusted; the 10-minute limit, *Stop*, and a log with lengths only.
7. **Settings:** the switch "Let agents use other apps' windows", off by default.
8. **Help:** a `desktop` skill that comes with the desktop tools (`load_with`, 8.9), the same for every model:
   - use the other tools first; desktop control is for what no tool can do;
   - when no search provider is set up or search fails, ask for a browser window and search there, saying why in `desktop_start`;
   - look before acting, and pick elements by their names, not their place in the list;
   - decline consent; stop and tell the user at a CAPTCHA.
   Macros and recipes come after SPIKE-040.

## Not in scope
- macOS and Linux, the library (DLL), screenshots, the clipboard, key combinations.
- Pipelines, scheduled runs, subagents.

## Rules
- Tests use the fake runner. Live checks only on windows the test opens itself (Calculator, a test editor), never the user's own windows or accounts.
- No new Go dependencies without asking.

## Done when
- Go tests cover the runner (hash check, kill, envelope), the lock, the deny list, the approval rows, password and consent refusal, the focus check, stubs and the time limit.
- Live on `gemini-3.5-flash-lite`: "write this sentence" in a test editor passes 3/3, with the pick card and *Stop* checked by hand.
- `gemma4:31b` when Ollama Cloud's quota allows.

## Result
Done on 2026-11-05 with agent-desktop 1.0.2. SPEC 8.10 and 8.9 describe it.

- **Built:** `internal/desktop`:
  - a runner that checks the program's SHA-256 and holds the file open until it starts, with no console, a clean environment, a time limit and a private state folder;
  - the five tools, offered only while the Settings switch is on and only in turns the user started;
  - the window card (the agent names an app, the user picks), the deny list (password managers also by title), and the lock on window, process instance and site;
  - compact snapshots, stubs for older ones, a change line after each action;
  - password, consent, CAPTCHA and focus guards; approval rows; a log with lengths only.
  - Also: the `desktop` help skill (`load_with` the tools), the switch in *Settings → General* (off by default, Windows only), tool chips that show lengths instead of text, and a window card that holds still while it waits.
- **Tested:** the runner with a fake agent-desktop (envelope, hash, kill, cancel, state folder), the tools with a fake runner (deny list, card by app, lock, site lock, guards, approvals, stubs, session ends), the agent wiring, and frontend tests for the card, the chips and the move guard.
- **Live** (the real orchestrator and agent-desktop, only windows the test opened):
  - gemini-3.5-flash-lite, "write this sentence" in a test editor: 19/20 runs, mostly 5–6 requests and 20–80 s. The one failure (two pick cards, nothing written, 26 requests) wasn't saved and didn't come back.
  - With the new card: 3/3 with the editor named by title. Then 3/3 "In Notepad" (a copy of the test editor named `notepad.exe`), where every run named `app: notepad` and got the one-window card (4–5 requests).
  - ministral-14b: 3/3, but it skipped the skill and `desktop_end`, and once called a tool that doesn't exist.
  - gemma-4-26b-a4b-it on the Gemini API, instead of gemma4:31b (no Ollama quota): 3/3, 63–113 s, never loaded the skill.
  - Stop ended the session with the turn, nothing reached the editor.
  - Prompts were 12–17k tokens a run, against 4.3k in SPIKE-039: Jenab's system prompt, tool list and skill. The snapshot itself was about 600 characters.
- **By hand in the app** (a server build in a browser, scratch data folders): the switch, the card, a write and Stop all worked.
  - Found and fixed: a Chrome tab on Google Password Manager was offered; it is now denied by its title.
  - Found and fixed: the chat auto-scrolled under the card, so a click picked the user's own Chrome window, and about 9,800 characters of it went to Mistral. The model saw the wrong window and ended the session; the chat was deleted. The card now holds still, and a click right after it moves is ignored.
  - Decided: apps that run commands (Claude, VS Code, Cursor) stay pickable.
  - Recorded: ministral-14b claimed a write worked after a "session ended" error.
- **Open:** in server mode the switch is hidden (no Windows check in a plain browser). Key presses fail in plain Win32 windows that aren't in front (SPIKE-039). Prompt cache breaks where a snapshot becomes a stub. Turns the app starts will need to leave the tools out (8.3).
- **Requests:** Gemini (`GEMINI_API_KEY`) 87 on 2026-11-03 and about 138 on 2026-11-05; Mistral (`MISTRAL_API_KEY`) 17 in the live test, plus the check by hand.

The session's report: [reports/TASK-035-report.md](reports/TASK-035-report.md).
