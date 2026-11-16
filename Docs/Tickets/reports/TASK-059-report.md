# TASK-059 report: updates and the installer for the native app

**Base:** `main` at d70d0c3, with MyGo v0.9.1 pinned.
**Model requests:** none.
**Downloads:** NSIS 3.13, once, by `mygo build`, after the user said yes. MyGo pins it: `nsis-3.13.zip` from MyGo's GitHub release `nsis-3.13` (SourceForge as the fallback), SHA-256 `ba63dffc4410ee89193e1cb5a41989991bd77c61068da17e3156d136b7b0b3d8`, which MyGo checks. It unpacks into `%LOCALAPPDATA%\mygo\nsis-3.13`; `makensis.exe` there has SHA-256 `4EC390D0F96F9728507FBC2F4E91311DD90BFC6C8DDFE2159357B3651AE151C0`. Nothing else was downloaded: the `mygo` command and its modules were in the module cache.
**Keys:** the test signing keys were made with `mygo keygen` in a scratch folder outside the repo. No private key is committed.
**Real data:** every live run used a scratch profile (`LOCALAPPDATA`, `APPDATA`, `USERPROFILE`), and the bench build keeps keys in memory. The user's own Jenab folders and keychain were not touched.

## What was built

### Updates

- **`internal/update`** (new, no MyGo): the `Service` runs Jenab's rules from SPEC 2.8 over an `Updater` interface. It checks at start and every 6 hours, as the user chose (automatic, notify, off), ignores prereleases on the stable channel, installs each version once, and does not install a refused version again on its own. In automatic mode it restarts into an installed version when `CanRestart` says so, checked every minute. Its status (off, unavailable, idle, checking, up-to-date, available, downloading, ready, failed) goes to the UI. Tests use a fake updater.
- **`cmd/desktop-native/updater.go`:** `mygo.Updater` behind that interface. `relaunch` is `mygo.App.Relaunch("--after-update")`.
- **The restart order:** MyGo starts the new copy inside `App.Run`, after the window closes but before Jenab's `lifecycle.Shutdown` and before the app lock is let go. So the new copy, seeing `--after-update`, waits up to a minute for the lock (`lifecycle.LockRootWait`, `lockApp`).
- **Settings › General:** an *Updates* row with the mode (Automatic, Notify me, Off), *Beta versions*, and a status line with one button (Check now, Install, Restart now) and a progress bar while it downloads. A build that can't update says "This build can't update itself". A new version and an installed one also show a toast once.
- **Can restart:** no chat is working or waiting (`Stores.Busy`) and the window is not shown with the focus.

### The installer and `task build:native`

- **`mygo.json`** at the repo root: name Jenab, identifier `io.github.amir-saatchi.jenab` (as `build/config.yml`), version from `VERSION`, publisher and company "Jenab", copyright and license Apache-2.0, `build/appicon.png`, main `./cmd/desktop-native`, output `bin/native`. No person's name, account or path. `go test ./internal/version` now checks its version, and that its publisher is the `CompanyName` of `build/windows/info.json`.
- **`wails3 task build:native`** runs `go tool mygo build` (MyGo's command, pinned as a `tool` in `go.mod`), with `MYGO_LDFLAGS` set to the version, commit and modified flags of `internal/version`. MyGo builds with `-s -w -trimpath -H=windowsgui`, puts the icon, the version info and the manifest in through a `.syso` it removes after, and makes the NSIS installer. Output: `bin/native/windows-amd64/Jenab.exe` (41 MB) and `jenab-setup-0.1.0-dev-amd64.exe` (10.7 MB).
- **The installer:** per user into `%LOCALAPPDATA%\Programs\Jenab`, no admin prompt, a Start menu entry `Jenab.lnk`, an Apps entry under `HKCU\…\Uninstall\io.github.amir-saatchi.jenab` (name, version, publisher Jenab, icon, uninstall and quiet-uninstall commands). It closes a running Jenab first. The uninstaller removes the folder, the shortcut and the entry, and leaves the data.
- **The release key:** `mygo.json` has no `updates` block, so release builds can't update themselves yet (the user's choice). With `updates` and `MYGO_UPDATER_PRIVATE_KEY`, the same task writes the signed archive and the manifest.

### Notifications

MyGo shows a notification on Windows as a balloon of Jenab's tray icon, which Windows 11 shows as a toast. The click comes back to the app as the tray icon's `NIN_BALLOONUSERCLICK` message, and `notifier` shows the window and opens the chat. So there is no app id (AUMID) to register, and the installer needs no shortcut property for it.

### The bench build

`bench_update.go` (bench tag only): `JENAB_UPDATE_FEED` points the updater at a local folder or a loopback URL, so tests never reach a real host; MyGo's checks are unchanged. New routes: `POST /update?do=check|install|restart`, `POST /hide`, `POST /notify`. `GET /state` adds the version, the projects, whether the window is shown and focused, and the updater's state. `DEVELOPMENT.md` has the steps.

## Checked against SPEC 2.8 and SPIKE-019

| Rule | Wails `pkg/updater` (SPIKE-019) | `mygo.Updater` |
|---|---|---|
| Signature | optional: an unsigned release went through unless Jenab wrapped it | always required; a manifest without one is refused at the check |
| Size | not checked | must match the manifest |
| Feed | https | https; plain http only on loopback |
| Prereleases | Jenab's rule | Jenab's rule; MyGo is told which (`SetPrereleases`) |
| Check timer | `CheckInterval` not used | Jenab's own timer |
| Swap | a helper process copies the new exe after the app exits, with a backup copy | in place while the app runs: each file renamed aside to `.<name>.old-<pid>`, the new one moved in; undone on failure |
| Failed swap | helper exit codes 14 and 17; a watchdog relaunches | the app is unchanged and keeps running; no helper, no watchdog |
| Downtime | the helper's copy, then the start | the restart only: under a second |
| Cleanup | `%TEMP%\wails-update-*`, helper logs, `.old.*` | MyGo removes `.old-<pid>` at the next start; the work folder `.Jenab.update-*` is removed on failure |
| Restart order | the helper starts the app after the old one exits | MyGo starts the new copy before Jenab's shutdown, so it waits for the lock (`--after-update`) |
| A new version that crashes at start | the watchdog relaunched the installed exe | not handled: the old files are deleted at the next start, so there is no way back |

## Live tests (Windows 11, scratch profile, local feed)

Five versions were built from this branch with the bench tag: 0.1.1 (with installer), 0.1.2 and 0.1.5 signed with the test key, 0.1.3 signed with another key, 0.1.4-beta.1. The feed was a scratch folder.

| Check | Result |
|---|---|
| Silent install of 0.1.1 (`/S /D=…`) | exit 0; `Jenab.exe`, `Uninstall.exe`, `.mygo-files`; Start menu shortcut to the exe; Apps entry with version 0.1.1 and publisher Jenab; exe version info: Jenab, 0.1.1, Apache-2.0 |
| 0.1.3, signed with another key | refused: "the update is not signed with the app's key"; files unchanged, app running |
| 0.1.2 with one bit flipped (same size) | refused: not signed with the app's key; files unchanged |
| 0.1.2 cut short by 1000 bytes | refused: size differs; files unchanged |
| 0.1.2 manifest without signature | refused at the check: "incomplete manifest" |
| 0.1.4-beta.1 on stable | ignored: up to date |
| Swap fails half-way (`Jenab.exe` held open without delete sharing) | refused: rename failed; every file as before, no leftovers, app running |
| Good update 0.1.1 → 0.1.2, then *Restart* | installed while running; exe matches 0.1.2; old copy gone after 0.23 s, 0.1.2 answering after 0.75 s; `.Jenab.exe.old-*` removed at start |
| Automatic mode, 0.1.2 → 0.1.5 | installed by itself at start; restarted into 0.1.5 at the next minute's check, once the window was no longer in front |
| Installer over an installed 0.1.5 | back to 0.1.1, Apps entry 0.1.1 |
| Notification click | with the window hidden, `POST /notify` showed the notification; the tray click message brought the window back; a click with no notification does nothing |
| Uninstall (`Uninstall.exe /S`) | folder, shortcut and Apps entry gone; data folder kept |
| Wails → native | see below |

**A bug the live run found:** the first restart took 36 s, and the second never came up. The restarted copy took the lock with `LockRoot`, then again with `LockRootWait`; when the first worked, its open file blocked the second until the garbage collector closed it. Fixed (`lockApp` locks once), with a test, and the restart then took under a second.

**Wails → native:** the Wails release exe (`wails3 task build`) ran on a fresh scratch profile and made its data. A project "From Wails" with a chat and one message was added with the same `project` code (a scratch program, not committed), and `config.yaml` set `updates.mode: off`, `channel: beta` and a context setting. The Wails app, started again, showed the project (read through UI Automation). Then the native installer ran over the Wails exe in the same folder: it replaced `jenab.exe` and added the uninstaller. The native app started on the same data: the project, the chat with its message, and the updater "off" from the settings; `config.yaml` unchanged. Keys: both apps use the keychain service `jenab` (`secret.OSKeyring`), so they stay; this was read from the code, not tried, as the test must not write to the real Credential Manager.

## Not checked, and gaps

- **A real toast click.** On this PC the toast never showed on screen (UI Automation found no toast window; Windows may hold it in the notification center). The click was sent as the shell sends it, to the app's tray window. A click on a real toast is a check by hand.
- **Not restarting while the window is in front.** The bench could not keep the window in front while the PC was in use (it lost the focus in 108 of 117 samples), so this rests on the unit test.
- **A new version that crashes at start** is not rolled back (the table above).
- **The Apps entry's version** stays at the installed version after an update: MyGo's updater doesn't write it.
- **The Wails app has no updater**, so the first native version reaches Wails users through the installer, not as an update. Same data, so nothing is lost.

## Follow-ups

1. Before the first public release (Phase 5): a release key and feed in `mygo.json`, the private key in the release build's secrets, and code signing (SmartScreen).
2. A start check after an update: if the new version doesn't reach its window, put the old files back (needs MyGo to keep them until then, or Jenab's own copy).
3. Update the Apps entry's version after an update (MyGo or Jenab).
4. TASK-017 (the Wails installer) overlaps: this installer covers its Windows scope except "asks before removing data", which MyGo's uninstaller doesn't do. TASK-061 can close it.
5. CI builds `task build:native` with TASK-060.
