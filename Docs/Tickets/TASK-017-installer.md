# TASK-017 — Install the app
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** SPIKE-019

## Goal
Jenab installs like a desktop app instead of running from `bin/`. Found while testing the app on 2026-10-13.

## Today
- The app is a single exe that `task build` writes to `bin/`. There is no installer, Start menu entry or uninstall.
- SPEC 2.8 already decides a per-user install in `%LOCALAPPDATA%\Programs\Jenab`, with no admin prompt. The updater relies on that location.
- The PROPOSAL puts "installer builds" in Phase 5. SPIKE-019 left installers, code signing and SmartScreen open.

## Scope
Windows first:
1. **Installer:** per user, into `%LOCALAPPDATA%\Programs\Jenab`, with no admin prompt. It adds a Start menu entry and an entry in *Installed apps* that uninstalls.
2. **Uninstall:** removes the app, not the user's data (`<user data dir>/Jenab` and the projects). It asks before removing data.
3. **Update path:** the installed app is what the updater replaces (2.8). Installing over an older version keeps the settings and projects.
4. **Build:** `task package` builds the installer. Which tool (Wails v3's NSIS or MSIX packaging, or another) is decided here. A tool that needs a download waits for approval.
5. **Signing:** decide how releases are code-signed, and what an unsigned build shows (SmartScreen).

Later, in the same ticket or a new one: a macOS `.dmg` and the Linux AppImage (2.8).

## Not in scope
- The update feed and release signing, already decided in SPIKE-019.

## Done when
- On Windows 11, a new user installs, starts, updates and uninstalls the app without admin rights, and their data stays.
- SPEC 2.8 names the installer and the signing decision.
