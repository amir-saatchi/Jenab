# TASK-035 — Desktop control v1 (Windows)
**Type:** Task
**Status:** Open
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
