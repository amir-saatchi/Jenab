# P1-13 — Wails layer
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 1)
**Needs:** P1-10, P1-11

## Goal
The thin `app` package between Go and React (CODE-OUTLINE 9, Q30–Q33).

## Scope
- **Services:** `ProjectService`, `ChatService`, `SettingsService`, `BucketService`, and `DevService` (bound only with the developer tools on).
- **Events:** registered with `RegisterEvent[T]` in the `init()` in `events.go`.
- **Errors:** `UIError` with `MarshalJSON`.
- **Publisher:** `publisher` implements the `Publisher` seams of `agent` and `project`.
- **Snapshots:** snapshot plus sequence (Q32).
- **Objects:** the handler for `/objects/<project_id>/<key>`.
- **Shutdown:** the Q30 order within 10 s.
- **Tooling:**
  - the Wails CLI, the Taskfile and `build/`
  - generated bindings in `frontend/bindings`
  - `wails3 dev` with hot reload
  - DEVELOPMENT.md updated

## Done when
- A Go test drives the services without Wails.
- CI regenerates the bindings and fails if they differ from the committed ones.
- The app quits within 10 s while a turn is running, and the next start needs no recovery.

## Result
- **Services (`internal/app`):**
  - `ProjectService`: List, Create, Open (with the Mother chat and the level), Activity, FolderWarning.
  - `ChatService`: List, Create, Snapshot, Messages, Send, Stop, Retry, Answer, Clear, Delete, Rename, SetRole, SetModel, Archive, SetLevel, Search.
  - `SettingsService`: Get, Save (written to config.yaml and applied at once), Providers.
  - `BucketService`: List (100 a page, with folders), Versions.
  - `DevService`: Providers, Activity, Log (redacted). Bound only when the developer tools are on at the start.
  - `NewServices` builds them; tests call them without Wails.
- **Errors:** `UIError{kind, message, details}`. Known errors get a short message; provider errors keep theirs; anything else is logged once and is "Something went wrong." Every method ends with `guard`, which also recovers a panic.
- **Events:** `chat:delta`, `chat:part`, `chat:status`, `project:notice`, `project:activity`, registered in `events.go`. `Publisher` implements the agent and project seams. `run:status` comes with pipelines.
- **Snapshots (Q32):** the last 30 turns, the sequence number and the live answer, read first. Changes from the services call `Orchestrator.Wrote`, so a rename during a turn doesn't make the frontend drop that turn's later events.
- **Settings:** `app.Settings` holds them while the app runs. A save reads config.yaml back, so problems come with their defaults, then resizes the call gate and applies provider changes. The config structs got `json` tags matching their `yaml` tags.
- **Objects:** `/objects/<project_id>/<key>` through `bucket.Handler`; the project is leased per request and released when the body closes.
- **Shutdown:** `app.Shutdown` runs the Q30 order: refuse, cancel, wait up to 5 s, then close the projects with the rest of the 10 s. The window hides first.
- **App root:** `cmd/desktop` now wires everything: the tools, `agent.Tools()` and `skill.Builtin()` (the wiring left from P1-12) included.
- **Tooling:**
  - the Wails CLI pinned to `v3.0.0-beta.26`, and `@wailsio/runtime` pinned to the same version
  - `Taskfile.yml`: build, dev, bindings, check:bindings, test
  - `build/`: the dev-mode config, the Windows icon, manifest and version info
  - `frontend/bindings`, generated and committed with LF line endings
  - DEVELOPMENT.md updated
- **Windows:** WebView2 keeps its data in `<app folder>\webview`, not `%APPDATA%\jenab.exe`. Wails' own log goes to the log file at Warn and above only (it logged every asset request).
- **Placeholder UI:** shows the project count from `ProjectService.List`, so a build shows the services are reachable.
- **Done-when checks:**
  - The services are tested without Wails (`internal/app/app_test.go`).
  - CI installs the CLI and runs `wails3 task check:bindings` on Linux. Locally it fails on stale bindings and passes on fresh ones; CI itself can't run yet (billing lock).
  - `TestShutdownDuringTurn`: a quit during a hanging turn takes well under 10 s, leaves no lock file, and the next start opens the project without recovery. The real app also quit cleanly when its window closed (under 0.2 s, exit 0).
  - `wails3 task dev` ran: the app started in 45 s, and an edit to a Go file restarted it in 8 s.
- **Tests:** project create and open, the chat calls and their error kinds, the snapshot window and older turns, settings save with a problem, provider status, bucket pages and versions, the objects route (both path forms, not found, no lease kept), the dev log, the UIError shape, panics, event names, the shutdown order and its deadline.
- **Mutation checks:** 27 on `internal/app`; 21 made the tests fail at first. Of the other 6:
  - 5 got tests (an error that is already a UIError, the snapshot window, the fixed title on rename, `Wrote` during a turn, running calls).
  - 1 was code that did nothing (the bounds on `Messages`, which the store handles), now removed.
- **Not done:**
  - Files in `Send`, `UndoTurn`, connecting providers and usage (P1-14 to P1-16).
  - Page and pipeline services, `run:status`, the scheduler's `OnStart` (later phases).
  - The real app icon (the Wails logo is a placeholder in `build/`).
