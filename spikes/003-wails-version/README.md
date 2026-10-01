# SPIKE-003 — Wails v2 vs v3

Throwaway code for [SPIKE-003](../../Docs/Tickets/SPIKE-003-wails-version.md).

```bash
sh build.sh          # app-v2.exe (v2.16.0), app-v2.12.exe, app-v3.exe (beta.26), app-v3a104.exe (alpha2.104); no Wails CLI, no npm
sh run-input.sh      # paint + input test on five setups, about 30 minutes; don't use the machine meanwhile
./app-v3.exe --manual                       # or: ./app-v2.exe --tray --manual; stays open for a hands-on check
```

App flags: `--frameless`, `--alpha1` (background alpha 1, as in Nord-Agent), `--tray` (v2 only: add `fyne.io/systray`), `--long N` (only the 2-minute minimise, N times), `--manual`, and a `*.json` path for the result.

- `common/probe.go`: the self-test, shared by the apps (`build.sh` copies it in). It:
  - serves `/objects/<project>/<key>`
  - minimises, restores, hides and shows the window
  - after each restore, checks with a screen capture that the page repainted in its new colour
  - clicks the window and presses a key, which the page must report back
  - on failure, saves a screenshot and records whether our window was in front
- `common/frontend/index.html`: a plain page.
  - It tests the `/objects` route (text, image, `Range`, 5 MB, 404).
  - It polls `/state` every 100 ms and fills the window with that colour.
  - It reports clicks and key presses to `/input`.
- `v2app/`: Wails v2 with `HideWindowOnClose`, `SingleInstanceLock` and `OnSuspend`/`OnResume`. `v2app-212/` is the same code on v2.12.0.
- `v3app/`: Wails v3 with a native tray, close-to-hide, single instance and APM events. `v3app-a104/` is the same code on alpha2.104.
- `driver/`: minimises and restores any window by title and compares captures with the one taken before (used on the Nord-Agent build).
- `result-*.json`, `r-*.json`: output of the runs.
