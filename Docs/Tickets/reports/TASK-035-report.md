# TASK-035 report — Desktop control v1 (Windows)

Branch `claude/relaxed-keller-163953`. Not committed: this file.

## What I built

### Runner and shipping (`internal/desktop`)
- `desktop.go`: the pin in one place: `Version = "1.0.2"`, `SHA256` of `agent-desktop.exe` (from the v1.0.2 Windows x64 archive, checked against the release's `checksums.txt`), `Program`. Envelope version `2.5`, call limits 15 s and 30 s (a session's first snapshot). The `Runner` interface; `Error` with code, message, suggestion and disposition (delivery, retry).
- `cli.go`, `cli_windows.go`: `CLI.Run`:
  - prepares the state folder (makes it with a private DACL, user + SYSTEM, protected; refuses one that inherits or grants anyone else);
  - opens the program with `FILE_SHARE_READ` only, checks its SHA-256 and keeps the handle until the process has started, so the file can't be swapped between the check and the start;
  - starts it with `CREATE_NO_WINDOW`, an environment without `AGENT_DESKTOP_*` plus `AGENT_DESKTOP_HOME`, output capped (16 MB / 4 KB);
  - kills it at the limit (`ErrKilled`); a cancelled turn returns its cause;
  - parses the envelope and maps failures.
- `cli_other.go`: other systems get `ErrUnsupported`; `main.go` adds the tools on Windows only.
- `build/agent-desktop/`: `LICENSE` (Apache-2.0, from the repository), `NOTICE` (version, release, SHA-256), `README.md` (where the binary goes: next to `jenab.exe`; in `bin/` for development). Root `NOTICE` names it. The binary is not in git (`*.exe`, `bin/` are ignored).
- The program's path is the folder of Jenab's executable + `agent-desktop.exe`; the state folder is `<Paths.Root>\agent-desktop` (see SPEC changes).

### Tools and sessions
- `desktop_start {why}`, `desktop_snapshot {full?}`, `desktop_act {id, action, text?, key?}`, `desktop_wait {text, gone?, seconds?}`, `desktop_end`.
- All have `Spec.On` (the switch) and `Spec.UserTurns`; all but `desktop_end` are `Untrusted`.
- One session per chat, for one turn. It ends at `desktop_end`, the turn's end (a new `Env.AtTurnEnd` hook, run by a defer in the turn, so *Stop* and errors end it too), 10 minutes, a closed or unlisted window, an app restart (process instance changed), a site change, a CAPTCHA, or a password field after an action. The next call gets why it ended.
- **Pick card** (`desktop_window`): one option per window that can be picked, labelled `title — app`, with the grant `pick:<window id>`, plus Deny. The model gets only the picked window's title. Deny with a note passes the note on.
- **Deny list**: Jenab's own pid and `jenab*`; terminals and shells (Windows Terminal, cmd, PowerShell, pwsh, conhost, OpenConsole, wsl, bash, …); regedit, mmc, taskschd, taskmgr, SystemSettings, control; password managers (1Password, KeePass, KeePassXC, Bitwarden, Dashlane, Enpass, NordPass, Proton Pass, LastPass, RoboForm); consent, LogonUI, CredentialUIBroker, LockApp; `explorer.exe` windows of class `#32770` (the Run box); invisible windows; and (added 2026-11-05, after the hand check) any window whose title names a password manager, such as a browser on Google Password Manager. A snapshot whose title turns into one ends the session.
- **Lock**: window id + agent-desktop's process instance, checked with `list-windows --app` before every command.
- **Browser** (chrome, msedge, firefox, brave, opera, vivaldi, chromium): snapshots with `--max-depth 40 --compact`; the roots sent are the address bar (value as its origin, no children) and the page; the page is retried until it has children (up to 8 s); the site lock takes the first non-blank origin and is checked before each action (a snapshot before acting). Every document value is reduced to its origin; `file://…` becomes `file://`.
- **Snapshots**: the compact form, `-i` unless `full`, at most 400 lines with a "[N more elements left out…]" line. Jenab's ids `n:eK` map to agent-desktop's refs; ids work only from this session's snapshots. Results are `Whole`, with `Series: "desktop_snapshot"` and a `Stub`.
- **Stubs**: `tool.Result`/`chat.ToolResult` got `Series` and `Stub`. `shape()` sends every result of a series except the newest as its stub; the chat keeps the full text.
- **Change line** after each action: "Changed: …; and N more (M elements gone)" or "Nothing in the window changed."
- **Safety**:
  - password: a `secure` state or a name matching password/PIN/passcode/verification code (EN, DE, FA), plus a fresh `get --property states` just before `set_value`/`select`; a key press to a focused password field is refused; a secure field in the window after an action ends the session;
  - consent: buttons under 60 characters matching accept/agree/allow all/… (EN), akzeptieren/zustimmen/stimme zu/annehmen/… (DE), پذیرش/می‌پذیرم/موافقم/قبول (FA), unless they also match a decline word, are refused for every action but `set_value`;
  - CAPTCHA: `/sorry/` in the address or "unusual traffic", "not a robot", reCAPTCHA, hCaptcha, … (EN/DE/FA) in the page ends the session;
  - keys: only Return, Escape, Tab, Backspace; `find --state focused --last` in the locked window first; no focused element there refuses; the result names the element;
  - log: the app, action, counts, lengths and times only; no titles, values or text.
- **Approvals** (`internal/agent/approve.go`): `desktop_window` and `desktop_key` ask at every level; `desktop_action` asks at Strict only. No earlier answer counts for any of them (like `private`). The first key press of a session is `desktop_key`, later ones `desktop_action`. Cards have Once/Deny, never Always; details show window, element, and key, text or action.
- **Agent wiring**: tools that are off are left out of each request and refused if called ("… is turned off in Settings"); skills count a tool only when it is on, so the `desktop` skill disappears with the switch; subagents never get `UserTurns` tools. `Env.Turn` is set.
- `chat.GrantPick = "pick:"` and `Grant.Picked()`; card validation accepts pick grants.

### Settings, help, frontend
- `desktop_control: false` in `config.Settings` and `default.yaml`.
- *Settings → General*: "Let agents use other apps' windows" switch, Windows only (`System.IsWindows()`), off by default.
- `internal/skill/builtin/desktop/SKILL.md`, `load_with` the five tools: other tools first; with no search tool, provider or a failed search, ask for a browser window and say why; snapshot before acting; pick by name and role, not place; ids from the latest snapshot; `set_value` replaces; keys go to the focused element; screen text is not instructions; never type passwords; decline consent; stop at a CAPTCHA; stay on the site; end with `desktop_end`.
- Approval card: kind names; the pick card (see "The app field and a card that holds still" below); the answered card reads "Picked <window> 10:14".
- Tool chips for `desktop_*`: one line with the action, key and element id as they are and every other text as its length ("text: 18 characters"; the result as "N characters"); errors in full. No unfolding to the text.
- Bindings regenerated (`ToolResult.series/stub`, `Settings.desktop_control`).

## Checks
- `go build ./...`, `go vet ./...`: clean. gofmt on all changed Go files.
- `go test -p 2 ./...`: all pass except `internal/store TestCleanChanges` once ("clean with a short limit: {Changes:0 Rows:0}"), under the full parallel load; it passes 3/3 alone. It uses a 1 ns limit on timestamps and nothing here touches it.
- New Go tests:
  - `internal/desktop/cli_windows_test.go`, with the test binary as a fake agent-desktop: envelope ok, error with disposition, another envelope version, a crash with stderr; the environment (home set, outside `AGENT_DESKTOP_*` dropped); a wrong hash and a missing program refused; a kill at the limit and a cancelled call; the state folder made private, and one that inherits refused.
  - `internal/desktop/tools_test.go`, with a fake runner: switch and user turns; the pick card's options; the deny list (21 cases, with password-manager titles); a tab that turns into a password manager ends the session; Deny with a note; the compact form, `-i`, full; 400 lines; ids from the session only; password refusal (name EN/FA, `secure` read again, key to a focused password field); a secure field after an action ends it; consent refusal EN/DE/FA and decline allowed; the focus check and where the key went; the approval rows (Standard: window + first key only; Strict: every action, Deny with the note); the change line; the lock on window and instance, a window that is gone; the browser site lock and a blank page taking its first site; the wait for a page with children and depth 40; a `file://` path scrubbed; CAPTCHA; the session's ends (turn end, another turn, 10 minutes, `desktop_end`, a second start); error mapping; wait; the stub fields; `origin`.
  - `internal/desktop/cli_live_test.go`: with `JENAB_AGENT_DESKTOP` set, the real program's `version` and the folder it made.
  - `internal/agent/desktop_test.go`: an off tool isn't offered and is refused; on, it is offered from the next request; turn-end hooks run once per call; the older snapshot of a series goes as its stub and the chat keeps the full text; subagents get no user-turn tool; the approval rows per level.
  - `internal/chat`: pick grants validate; an empty pick and an unknown grant don't; a group only on pick options, and only `match` or `other`.
  - `TestPickCardByApp`: one window of the app, several (`chrome.exe` with the .exe and capitals), none, a denied app named (never offered; the card names it), no app (no groups).
- Frontend: `bun run typecheck`, `bun test` (127 pass; new: the desktop chip shows lengths, not text; the pick card lists windows then Deny; an answered one says which; the single-window card, the folded "Other windows", no window of the app; `pickView`, `pickWaiting` and the move guard), `bun run build`.
- `go run ./cmd/jenab-scenarios check scenarios`: ok.
- `wails3 task check:bindings`: no difference.

## Live results
`internal/agent/desktop_live_test.go` (Windows; runs only when `JENAB_DESKTOP_LIVE`, `JENAB_AGENT_DESKTOP` and `JENAB_TEST_EDITOR` are set):
- It runs the real orchestrator with the built-in tools and skills, the desktop tools with the real agent-desktop 1.0.2, a scratch project in a temp folder, and the key in memory only.
- For each run it starts SPIKE-039's test editor with its own title and kills that process by PID at the end.
- It answers the real pick card with the editor's option only (Deny if that is missing) and checks that the card offers no terminal, Jenab or the test binary. Other cards get Once. At Standard there are no other cards: only the window card asked.
- The task is SPIKE-039's: `In the open text editor "TASK035 Live Editor N", write this sentence: Meet me at the north gate at 7:30, and bring the blue folder.` It passes when the editor's text has the sentence.
- Requests are model requests (the spike's "turns"). Tokens are prompt (+ cache read) / output. Time is wall time for the turn.

**gemini-3.5-flash-lite**: 19 of 20 runs passed, in 5 batches: 9 of 10 on 2026-11-03 (87 requests) and 10 of 10 on 2026-11-05 (64 requests).

| Batch | Run | Result | Requests | Prompt (+cached) / output tokens | Time |
|---|---|---|---|---|---|
| 1 | 1 | done, text exact | 6 | 16,882 (+3,033) / 352 | 79 s |
| 1 | 2 | done | 6 | 17,583 (+4,535) / 383 | 74 s |
| 1 | 3 | done | 6 | 16,832 (+6,210) / 633 | 52 s |
| 2 | 1 | **failed**: two pick cards, the editor without the sentence | 26 | — | — |
| 3 | 1 | done | 6 | 12,513 (+7,505) / 388 | 45 s |
| 3 | 2 | done | 6 | 14,944 (+7,743) / 616 | 40 s |
| 3 | 3 | done | 5 | 13,923 (+4,602) / 556 | 39 s |
| 4 | 1 | done | 6 | 12,493 (+7,512) / 391 | 20 s |
| 4 | 2 | done | 6 | 14,383 (+7,681) / 369 | 29 s |
| 4 | 3 | done | 5 | 12,236 (+6,202) / 437 | 36 s |
| 5 | 1 | done | 6 | 14,271 (+5,967) / 411 | 34 s |
| 5 | 2 | done | 5 | 14,135 (+4,625) / 589 | 31 s |
| 5 | 3 | done | 4 | 11,736 (+3,004) / 378 | 29 s |
| 5 | 4 | done, after 3 writes | 11 | 31,626 (+12,053) / 882 | 26 s |
| 5 | 5 | done | 7 | 19,108 (+7,643) / 771 | 35 s |
| 5 | 6 | done | 6 | 16,171 (+6,093) / 426 | 28 s |
| 5 | 7 | done | 6 | 16,238 (+6,115) / 503 | 24 s |
| 5 | 8 | done | 6 | 16,042 (+6,086) / 409 | 22 s |
| 5 | 9 | done | 7 | 15,910 (+6,078) / 352 | 51 s |
| 5 | 10 | done | 6 | 13,811 (+4,593) / 420 | 26 s |

- **Every pass took the same steps:** `load_skill desktop`, `desktop_start`, `desktop_snapshot`, one `set_value` on the text box, `desktop_end`, then the answer.
  - The snapshot was 16 lines and 592 characters. `set_value` took 1.3–1.8 s, the snapshot about 0.8 s.
  - Every `why` on the card was one plain line, such as "to write the sentence into TASK035 Live Editor 3".
- **The failure (batch 2, run 1):**
  - It took 26 requests and 24 tool calls: snapshots and acts alternating, then a second `desktop_start`, so the first session had ended.
  - I don't have its steps: that batch didn't save its output. I added step logging for failures and ran 16 more runs (6 on 11-03, 10 on 11-05), and it didn't happen again.
  - The likely cause is the session ending in the middle of the task. Possible reasons are an unlisted window or a lock check that failed. Not confirmed.
- **Batch 5, run 4** passed after three writes. The first `set_value` gave a longer result than usual (176 characters against 145), and the next full snapshot was 80 characters longer. The model then clicked an element, wrote again, looked again and ended. The steps weren't printed because the run passed. My guess is that the first write didn't replace the text cleanly and the model fixed it. It ended with the sentence in the editor.
- In batch 5 Gemini asked for `full: true` snapshots (17 lines, 622 characters). Earlier batches used the default.
- **Prompt size:** about 12–17k tokens a run, against SPIKE-039's 4.3k. Jenab sends its whole system prompt, the tool list and the skill. The snapshot itself is small.
- **Stop** (two runs):
  - The harness picked the window, waited 1 s, then called `Orchestrator.Stop`. Each time the session ended with the turn: the log says "session ended … why=the turn ended snapshots=0".
  - Nothing reached the editor, and the turn ended in 9–12 s.
- **Pick card:** 14–15 windows each time, never a terminal, Jenab or the test binary. The model only ever saw the picked editor's title.
- **Log:** lines carry only the app, counts, lengths and times, for example `desktop: act app=spike039.exe action=set_value text_len=61 result_len=145 took=1800`. No text or titles.
- **Cleanup:** every editor the test started was killed by its PID, and no `spike039.exe` was left running. The probe editor from development, also started by me, was closed by its PID.

**ministral-14b-latest** (OpenAI-compatible API at `https://api.mistral.ai/v1/`): 3/3 passed. All 17 requests went within about 2 minutes, under 30 a minute.

| Run | Result | Requests | Prompt (+cached) / output tokens | Time |
|---|---|---|---|---|
| 1 | done | 8 | 17,948 (+4,480) / 170 | 66 s |
| 2 | done | 4 | 19,169 / 115 | 14 s |
| 3 | done | 5 | 17,152 (+2,048) / 132 | 25 s |

- It never loaded the `desktop` skill. It went straight to `desktop_start`, `desktop_snapshot` and `desktop_act set_value`, and never called `desktop_end`, so the session ended with the turn.
- In run 1 it called a tool that doesn't exist (`desktop_set_value`), got "there is no tool named …", and then used `desktop_act`.
- Its `why` lines repeated the whole sentence ("Write the sentence '…' in the open text editor …"). That is fine for the card, but longer than needed.

**The real app, by hand** (2026-11-05):
- **Setup:**
  - I used the app's server mode (`go build -tags server ./cmd/desktop`) at `http://127.0.0.1:9310`, driven in a browser, with `LOCALAPPDATA`, `APPDATA` and `USERPROFILE` pointed at a scratch folder. agent-desktop sat next to the executable.
  - The model came through a scratch proxy on `127.0.0.1:8731`. It speaks Ollama's API to Jenab and forwards each request to Mistral's OpenAI-compatible API with the key from `.env`, sent only in the auth header and paced at one request per 2.1 s.
  - Jenab saw a local Ollama with no key, so nothing went into the Credential Manager and no key was typed into the app. The model was ministral-14b-latest.
- **Settings switch:** it was off on a new install. Turning it on saved `desktop_control: true` to `config.yaml`, and the next chat offered the tools.
  - In server mode the switch is hidden, because a plain browser has no `window._wails.environment`, so `System.IsWindows()` is false. WebView2 sets it. I set that one field from the console to reach the switch. Only a dev-mode issue, but the switch could instead ask the Go side whether desktop control exists.
- **Pick card:** it listed 12–15 windows as `title — app`, with Deny and a note field, and no terminal or Jenab. Picking the test editor recorded "Picked TASK035 Hand Editor — spike039 08:17". The model loaded the skill, took a snapshot, called `set_value`, and the editor had the sentence exactly (checked with agent-desktop). The session ended with the turn.
- **Stop, twice:**
  - Pressing Stop right after the pick: `desktop_start` showed "cancelled by user", the card kept "Picked …", and no session started (no log line).
  - Pressing Stop while the card waited: the card read "Stopped without an answer" and `desktop_start` showed "cancelled by user".
  - The editor didn't change either time.
- **A second write** in a new chat ("replace the text with: The meeting moved to Friday.") worked after I picked the right window. The editor read exactly that.
- **What the check found:**
  1. **A browser tab on a password manager was offered**: "Google Password Manager - Google Chrome". The deny list went by program only. **Fixed**: a window whose title names a password manager (Google Password Manager, Firefox's "Passwords", 1Password, Bitwarden, KeePass, LastPass, Dashlane, NordPass, Proton Pass, Enpass, RoboForm; EN/DE/FA) can't be picked. A snapshot whose title turns into one ends the session, since a browser's title follows its tab. There are tests for both. After a rebuild the card no longer offered it.
  2. **The Claude desktop app was offered** ("Claude — claude"). It is an agent app that can run commands, so letting an agent drive it gets around the reason terminals are denied. The same goes for editors with a built-in terminal (VS Code, Cursor). The user decided on 2026-11-05 to leave them pickable (see Open points).
  3. **The card moves while it waits.** About 60 px of auto-scroll between my look and my click moved the options, and **I picked the wrong window**: the user's own Chrome window on the Gemini site. Its full snapshot, about 9,800 characters, went to Mistral. The model saw it was the wrong window, called `desktop_end` and asked again, which is what the skill says. A person can make the same slip. Proposal: don't auto-scroll a chat while a window card waits, or hold the list still under the pointer.
  4. **Ministral claimed success after an error.** In a follow-up turn of the first chat it reused the ended session's ids without calling `desktop_start`. It got "the desktop session ended: the turn ended. If the user still wants it, call desktop_start…", and still answered "The text in the editor has been replaced". The editor was unchanged. This is a model failure, recorded and not worked around. Gemini asked again in the same situation.
- My first chat message went in doubled (I typed into the box twice), so that turn's prompt read oddly. The model still wrote the right sentence.
- Afterwards I killed the test editor by its PID, stopped the proxy and the app, and deleted the scratch data. That deleted the chat with the wrong window's snapshot.

**gemma4:31b on Ollama Cloud:** not run. On 2026-11-05 the first key and `_II` answered 429 (monthly limit) and `_III` answered 402 (no credit), one tiny request each. Instead, at the user's word, I ran **gemma-4-26b-a4b-it on the Gemini API** with `GEMINI_API_KEY`: 3/3 passed, and Stop passed.

| Run | Result | Requests | Prompt (+cached) / output tokens | Time |
|---|---|---|---|---|
| 1 | done | 6 | 13,232 (+2,889) / 531 | 67 s |
| 2 | done | 6 | 13,350 (+4,464) / 478 | 63 s |
| 3 | done | 8 | 7,267 (+10,551) / 456 | 113 s |
| Stop | session ended with the turn, editor untouched | 2 | 1,482 (+2,006) / 225 | 7 s |

- Every run took the same four steps: `desktop_start`, a `full` snapshot (17 lines, about 620 characters), one `set_value`, then `desktop_end`. It never loaded the `desktop` skill.
- It is slow: each session lasted about 55–105 s from start to end for one write. Run 3 took 8 requests for 4 calls; the extra requests were answers without a call, and the steps weren't printed because it passed.
- Its `why` lines were one plain line each, such as "To write the requested sentence in the "TASK035 Live Editor 2" text editor."
- 20 Gemini-key requests in all.

## The app field and a card that holds still (2026-11-05)
The user's decisions on 2026-11-05, on top of `98fdac3`.

**The model names an app, the user picks.**
- `desktop_start` has `app` (optional, up to 100 characters): the program by its process name, without .exe ("chrome", "msedge", "notepad", "calc"), never a window title. `why` is still required.
- Jenab matches `app` to agent-desktop's process name without case, spaces at the ends or ".exe". Each pick option gets a `group`: `match` for that app's windows, `other` for the rest. With no `app`, options have no group and the card is the full list, as before.
- The card, by the number of matching windows:
  - **one:** "Let the agent use <title — app>?" with Allow (the pick grant), Deny (+ note) and "Show all windows". That opens the full list in the same card; it is not a new request, and the answer is still one pick grant.
  - **several:** "Pick a window of <app> for the agent to use"; that app's windows first, the rest folded under "Other windows (N)".
  - **none:** "No window of <app> is open: pick another for the agent to use"; the other windows, unfolded. A denied app named (a terminal, say) lands here: its windows are never offered.
- The window list scrolls inside the card (at most half the window's height, 24 rem).
- Unchanged: the deny list; the model sees only the picked window's title; the approval rows; the lock. `chat.Approval` validation rejects a group on any option that isn't a pick, and any group but `match` and `other`.
- The `desktop` skill says to name the app in `desktop_start(app, why)`, by process name, and to leave it out when unsure.

**The card holds still.**
- While a window card waits unanswered, the chat stops following new content (`MessageScrollerProvider autoScroll` is off; `pickWaiting`). It follows again once the card is answered or stopped. The user can still scroll.
- A click on a window (a list button or Allow) is ignored if the card moved in the last 500 ms. The check is the layout, not a timer after render: a `ResizeObserver` on the card and every element around it up to the body, scroll and resize listeners, and one more reading of the card's place at the click. A change of place or of the card's own size counts; so does the card appearing. Deny and "Show all windows" aren't guarded: they pick no window.

**Live, gemini-3.5-flash-lite, 3 runs with the new card** (GEMINI_API_KEY; `live-gemini-6.txt` in the scratchpad):

| Run | Result | Requests | Tokens in (+cached) / out | Time | Card |
|---|---|---|---|---|---|
| 1 | done | 6 | 15,424 (+5,973) / 636 | 11 s | no `app`: the full list (11 windows) |
| 2 | done | 7 | 17,372 (+6,087) / 666 | 43 s | no `app`: the full list (11 windows) |
| 3 | done | 7 | 18,456 (+4,549) / 539 | 29 s | `app: notepad`: "No window of notepad is open", the test picked from the other windows |

- 3/3 done, 87 s for the batch. Every run: `load_skill`, `desktop_start`, a snapshot, one `set_value`, `desktop_end`. It loaded the skill each time.
- The single-window card wasn't reached live. The task names a window title ("In the open text editor "TASK035 Live Editor 1", write …"), not a program. The test editor's process is `spike039`, which the model can't guess, so it left `app` out twice and guessed `notepad` once. The skill says to leave `app` out when unsure. Each card did what its case says: no app gave the full list; an app with no window said so and listed the rest unfolded.
- The live harness now logs the card's Ask and the picked option's group. The `why` lines were plain. In two runs they name the test editor's title, which the user gave in the task.
- 20 Gemini-key requests in all; about 124 today.

**Live, "In Notepad", 3 runs** (GEMINI_API_KEY; `live-gemini-7.txt`). The real Notepad wasn't used: Windows 11 Notepad restores its last tabs, which can hold the user's text. The test ran SPIKE-039's editor copied to a scratch folder as `notepad.exe`, so its process name is `notepad`. The task: "In Notepad, write this sentence: Meet me at the north gate at 7:30, and bring the blue folder." No real Notepad was open, so the card never showed "several". `JENAB_DESKTOP_APP=Notepad` sets this task in the live test.

| Run | Result | Requests | Tokens in (+cached) / out | Time | Card |
|---|---|---|---|---|---|
| 1 | done | 5 | 10,866 (+2,903) / 526 | 42 s | `app: notepad`, one window: "Let the agent use TASK035 Live Editor 1 — notepad?", Allow |
| 2 | done | 4 | 12,217 (+2,981) / 553 | 7 s | the same, Editor 2 |
| 3 | done | 5 | 13,105 (+6,029) / 594 | 9 s | the same, Editor 3 |

- 3/3 done. The model named `app: notepad` each time and got the one-window card; the test answered with its own window's pick (group `match`). 62 s for the batch, 14 requests, so about 138 today.
- Its `why` lines: "to write the meeting sentence in Notepad", "I need to type the message into Notepad.", and "I need to open Notepad to write the sentence as requested." The last says "open", though Notepad was already open.
- Calculator's process is `CalculatorApp` (SPIKE-039 and SPIKE-040). The examples in the skill and in the `app` field's description now say `calculatorapp`, not `calc`. There is no alias table.

## Open points
- **The pick card and Stop by hand:** done in server mode through a browser (see Live results). Not yet done in the WebView2 window of a built `jenab.exe`, which would need a key in the Credential Manager. The frontend is the same.
- **Agent apps and editors with terminals** (Claude, VS Code, Cursor, …) stay pickable: the user decided on 2026-11-05. The deny list covers terminals, system tools, password managers and sign-in screens only. Every pick is the user's own choice on the card, so 8.10 could say that apps which can run commands are left to the user.
- **The window card and auto-scroll:** done (finding 3; see "The app field and a card that holds still").
- **Password-manager titles** are now denied (the fix above). The list of names is a best effort, like the program list.
- **Keys in plain Win32 apps.** SPIKE-039 found that agent-desktop's headless press fails in plain Win32 windows that aren't in front ("exact focus could not be confirmed"). The model gets the mapped error and the skill says to try a button instead. Presses work in Chrome.
- **State folder ACL with Administrators.** Jenab makes the folder with user + SYSTEM only and accepts a folder that also grants Administrators. agent-desktop accepted Jenab's folder in the live runs.
- **Turns the app starts** don't exist yet: every chat turn comes from Send, so today `UserTurns` only keeps the tools from subagents. When app-started turns come (8.3), they must leave these tools out.
- **Prompt cache.** Each new snapshot turns the previous one into a stub, which changes an earlier message, so the cached prefix ends there. That is the SPEC's choice; the stub saves more than the cache would here.
- **The card shows every pickable window's title**, including the user's other windows. The titles stay in the chat's card and are not sent to the model; only the picked one's title is.
- The `desktop` skill's search line changed after batch 1 had started (wording only, about search providers). Batches 2–4 and Mistral used the new text.
- **The one Gemini failure** (see Live results) is not explained. If it is a session that ended mid-task, the model did the right thing and asked again. It still didn't finish, though, and a log line with the reason would show why. The log already has the reason; the harness now prints the steps of a failed run.

## Proposed SPEC changes (8.10)
- **State folder:** "Its state folder is `agent-desktop` under Jenab's fixed app folder (`%LOCALAPPDATA%\Jenab`, next to the settings and logs), not under the data folder: the data folder can be moved or synced, and the ACL must stay private. Jenab makes it with access for the user and SYSTEM only and refuses to start agent-desktop if the folder inherits access or grants anyone else (Administrators allowed)."
- **Stubs:** "Older snapshots become stubs: `[snapshot 3 of "Calculator" — replaced by a newer snapshot]`." (The stub is written with the result, before the newer one exists.)
- **Chat log:** "The chat shows each step as a tool chip with the action, the element id and the lengths of any text, not the text. The app log keeps the same: lengths and times only. The chat's history keeps the full results, which the model needs."
- **Pick card** (replaces the pick card text; changed by the user on 2026-11-05):
  > `desktop_start` takes `app`, the program the agent wants by its process name without .exe (such as `chrome` or `notepad`), never a window title, and `why`. Jenab matches `app` to each window's process name without case or ".exe", and offers the windows the deny list allows, each as `title — app`. When exactly one window of that app can be picked, the card asks "Let the agent use <title — app>?" with Allow, Deny (with an optional note) and "Show all windows", which opens the full list in the same card. When several or none match, the card lists that app's windows first and the rest under "Other windows", folded; with none (an unknown app too), the card says no window of the app is open and lists every other window, unfolded. With no `app`, the card is the full list. The list scrolls inside the card. The answer is the window picked (a pick grant, `pick:<window id>`) or Deny; the model sees only the picked window's title.
  >
  > While the card waits, the chat doesn't follow new content by itself, so the card holds still; the user can still scroll. A click on a window is ignored if the card moved or changed size in the last 500 ms.
- **Keys:** add "A key press asks at Strict like any action; the first one in a session asks at every level." (The table has it; the text could say the later ones are actions.)
- **Password names:** add "code" names only as "verification code", "one-time code" and "security code" (a plain "Code" field is allowed unless it is `secure`).
- **Consent:** "A button whose name also declines ("Don't accept", "Nicht akzeptieren") is allowed; long names (60 characters or more) are text, not consent buttons."
- **Help skill:** 8.9's list gains `desktop` (load_with the five tools): other tools first; ask for a browser window when there is no search provider or search fails, saying why; look first and pick by name; decline consent; stop at a CAPTCHA.
