# SPIKE-003 — Wails v2 vs v3
**Type:** Spike
**Status:** Done
**Gate:** 2

## Question
Which Wails version supports what v1 needs?
1. System tray and running with the window closed (background mode)
2. A custom asset handler route for bucket objects (`/objects/<project_id>/<key>`, SPEC 4.5)
3. OS sleep/wake events (with SPIKE-004)
4. Stability and release status of v3
5. Added: can the freeze after minimise/restore, seen on this machine with v3 in June, be reproduced?

## Done when
A minimal app with a tray icon and an asset route runs on Windows; the version is chosen.

## Result
Run on 2026-09-27/28 on Windows 11 Pro 26200 (WebView2 / Edge 153) with Go 1.26.0. Neither version needs cgo or the Wails CLI on Windows.
- **Code and full output:** [spikes/003-wails-version](../../spikes/003-wails-version/) (`result-*.json`, `r-*.json`).
- **Setup:**
  - Each app tests itself.
  - The page fills the window with a colour that Go changes while the window is minimised or hidden.
  - After each restore, a screen capture checks that the new colour is on screen. So a blank, frozen or stale window counts as a failure.

| Check | v3.0.0-beta.26 | v2.16.0 | v2.16.0 + `fyne.io/systray` v1.12.2 |
|---|---|---|---|
| Tray icon with menu | yes, built in | **no** | yes (separate library in the same process) |
| Close hides the window, the app keeps running | yes (`WindowClosing` hook) | yes (`HideWindowOnClose`) | yes |
| Background work while hidden | ok (50 of 50 ticks in 5 s) | ok | ok |
| Second launch shows the running window | yes; second exits in 60–100 ms | yes | yes |
| `/objects/...` route: text, image, `Range` (206), 5 MB, 404 | all ok; 5 MB in 178 ms | all ok; 131 ms | all ok; 97 ms |
| Repaint after minimise/restore (20 like the taskbar, 10 through the API) | **30 of 30** at 300 ms and at 1.5 s | 30 of 30 | 30 of 30 |
| Repaint after hide/show (20) | 20 of 20 | 20 of 20 | 20 of 20 |
| Repaint after 30 s and 2 min minimised | 2 of 2 | 2 of 2 | 2 of 2 |
| Notification | sent without error | sent without error | sent without error |
| Sleep/wake events | `APMSuspend`, `APMResumeAutomatic` | `OnSuspend`, `OnResume` | same |
| Start at login | built in (`autostart`) | not built in (Run key) | not built in |
| Release status | beta; betas come every few days | stable, still maintained | two stable libraries |

**The bug seen earlier** (Nord-Agent, June 2026):
- **Setup:** Wails v3, most likely `v3.0.0-alpha2.104` (18 June; the betas only started in August), with `Frameless: true` and a React title bar.
- **What happened:** after minimising from the taskbar and restoring, the window showed old content and nothing responded.
- **When:** every time. It didn't happen with a normal OS frame.
- **After:** moving the app to Wails v2.12.0 fixed it. That app still sets `BackgroundColour` alpha to `1` of 255, which is probably a typo for `255`.

**Reproduction attempts**
- Besides painting, each restore now also checks input: a real mouse click in the middle of the window and a key press, which the page must report back.
- Each failure saves a screenshot and records whether our window was in front.

| Setup | Paint after restore | Click and key after restore |
|---|---|---|
| v3 alpha2.104, frameless (the June setup) | 106 of 106 | 52 of 52 |
| v3 alpha2.104, with frame | 106 of 106 | 52 of 52 |
| v3 beta.26, frameless | 106 of 106 | 52 of 52 |
| v3 beta.26, with frame | 106 of 106 | 52 of 52 |
| v2.12.0, frameless, alpha 1 | 106 of 106 | 52 of 52 |
| v3 beta.26, frameless, alpha 1, five 2-minute minimises (paint only) | 11 of 15; in every failure **another window was in front** (the screenshot shows the Claude app), so these failures come from the test setup | — |
| v2.12.0, frameless, alpha 1, five 2-minute minimises (paint only) | 15 of 15 | — |
| The Nord-Agent build itself (v2.12.0), idle: 12 rounds, the last two minimised for 30 s and 2 min | 36 of 36 captures identical to the one before | — |

(Per setup: 20 taskbar-style rounds, 10 through the Wails API and 20 hide/show rounds, each checked at 300 ms and 1.5 s; plus one 30 s and one 2 min minimise, checked at 300 ms, 1.5 s and 5 s. Input is checked once per round.)

**Findings**
- **The June freeze did not reproduce,** not even with alpha2.104 frameless. The minimal test page kept painting and taking input in every Wails version. So the trigger was probably something in that app together with v3 frameless: its title bar, drag regions, CSS or window code. That code no longer exists, so this can't be settled here.
- **v3 has had no failure in any test** that wasn't another window in front.
- **v2 plus `fyne.io/systray` works on Windows,** in the same process as Wails. The tray runs its own message loop on its own thread. On macOS both need the main thread, so this setup may not work there; that needs a Mac test.
- **Not yet checked by hand:** the tray icons weren't clicked. `--manual` mode is there for that.
- **Sleep/wake events:** both versions offer them. They are tested with real sleep in SPIKE-004.

## Decision
**Decided (2026-09-28):**
- **Shell:** Wails **v3**, pinned to an exact tag (now `v3.0.0-beta.26`). It has the built-in tray, autostart, notifications, single instance and APM events on all three OSes.
- **Window:** v1 uses the **normal OS frame, not a frameless window,** since the only freeze seen was with frameless. Making it frameless is a separate decision later.
- **Upgrade check:** the SPIKE-003 self-test (paint and input after minimise/restore and hide/show, `/objects` route, second instance) runs before every Wails upgrade. It stays in the repo as a smoke test.
- **Fallback:** if a freeze shows up with v3, switch to Wails v2.16 + `fyne.io/systray`. This works on Windows; macOS is unchecked.
