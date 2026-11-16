# TASK-053 report: the SPEC and design docs for the native UI

**Base:** `main` at a050de7, with SPIKE-048's code merged and MyGo v0.9.1 pinned.
**Code changes:** none.
**Model requests:** none.
**Downloads:** none.

Each section was checked against the code, not against the spike's plan: `cmd/desktop-native/main.go`, `internal/native/{kit,state,shell,chat}`, `internal/webui`, `internal/lifecycle`, `internal/agent/coalesce.go`, `cmd/jenab-bench` and `go.mod`.

## What changed

**SPEC**
- **2.8 App updates.** A *Moving* note at the top says three things. MyGo's signed updater (`mygo.Updater`) replaces Wails `pkg/updater` in the native app. TASK-059 checks it against the section's rules and rewrites the section. Until then the rules stand, and the native app has no updater yet. The *Updater* bullet points to the note. Nothing else in 2.8 changed.
- **4.5 Links and references.** The last bullet is now *Loading objects*, one part for each app:
  - **Native:** there is no webview and no route. A screen reads an object through the services, which lease the project. A link opens the file view with the chat in the dock. A type the file view doesn't show offers *Open with the system app* (4.7).
  - **Pending:** the file view, the dock and the service call that reads the bytes come with TASK-056. Until then the native chat doesn't handle `jenab://` links itself.
  - **Wails:** the `/objects/<project_id>/<key>` route, until TASK-061. 4.7's headers apply to it.
- **5.8 Frontend rules.** Rewritten for the native app, with the React rules kept under *The Wails app (React), until TASK-061*. The native part covers:
  - **Two apps:** `cmd/desktop-native` and `cmd/desktop`, with the same services and the same lock. The React frontend gets bug fixes only. A new screen or feature goes into the native app, and pages and pipelines are native only.
  - **The kit and the stores:**
    - the kit holds the theme, fonts, right-to-left defaults and MyGo fixes;
    - the stores are used on the UI thread only, with `state.Post` from `Window.Update`;
    - screens use the kit, the stores and the Go side's types, never MyGo's `ui`. `TestOnlyTheKitImportsMyGoUI` checks this;
    - a MyGo bug is fixed in the kit and reported upstream;
    - every component has a headless `ui.NewTester` test.
  - **Events:** the `Publisher` posts to the stores through `Window.Update`, which returns without waiting. A mailbox keeps the events sent before the window exists.
  - **Streaming:**
    - the agent coalesces deltas every ~16 ms, for both apps;
    - each delta is applied as it comes;
    - `markdown.Stream` shows a streaming part and `markdown.View` a finished one;
    - the thread is a virtual list that follows its end;
    - one selection is shared across the thread;
    - SPIKE-048's numbers.
  - **Right-to-left text, no mirrored layout:**
    - `kit.Root` keeps the window left to right;
    - direction comes from the first strong character (`kit.RightToLeft`);
    - lists and tables take their own direction;
    - code blocks stay left to right with tab stops every 2;
    - the fonts are embedded as TTF files.
  - **Known gap:** harakat (egoist/mygo#1925), which TASK-054 picks up.
- **5.10 Block registry.** A block's unit is a kit component in Go, a JSON Schema and a Go validator. The page screen looks the type up. Blocks have no React components, since pages are native only.
- **5.11 Theme.** *How it works* now has two parts:
  - **Native:** `kit.Light` and `kit.Dark` from `index.css`, `kit.Theme` mapped to MyGo's theme, `kit.SetThemeSource` for Light, Dark and System, and the theme set before the window opens.
  - **Wails:** as before, until TASK-061. The kit's tokens and `index.css` must stay the same while both apps ship.
  - **Tones:** in the kit, a tone is a text colour and a soft fill (`kit.ToneColors`).
- **5.12 App shell.** A paragraph *In the native app* comes before the rules:
  - **Kit pieces:** `Root`, `Rail` (56 px), `RailTile`, `SidePanel` (256 px), `Bar` (56 px), `Main`, `List`, `Dialog` and `Toasts`.
  - **What the shell has now:** the rail, the chat list and the main area. Which ticket brings the dock, the right sidebar, the files, the bottom bar, Settings and the project screens.
  - **Window:** close hides the window, the tray shows it and quits, a second launch shows it, and the size and place are saved.

  The section's rules are unchanged and hold for both apps.

**Gate-2 technical design**
- **New *Desktop UI* row:** MyGo v0.9.1, the only UI library. It carries SPIKE-048's measured results against the Wails app and the go decision of 2026-11-16.
- **Notes on four rows:** *Desktop shell* (Wails ships until TASK-061), *App updates* (`mygo.Updater`, TASK-059), *Frontend* (bug fixes only until TASK-061) and *Frontend load* (`markdown.Stream` and virtual lists).
- **New *UI layer (SPIKE-048)* section:**
  - SPIKE-048's architecture diagram, with `internal/webui` and `internal/lifecycle` added;
  - the rules: screens use only the kit and the stores, the stores are plain Go, right-to-left text with no mirrored layout, the same repository and `main`, and React bug fixes only;
  - the MyGo pin.

**DEVELOPMENT.md**
- **Intro:** the two apps until TASK-061, and the *(Wails)* mark, which means "goes with TASK-061".
- **Tools:** Bun, the Wails CLI, and the macOS and Linux build needs are marked *(Wails)*. MyGo comes through `go.mod`, and the native app is Windows only until TASK-060.
- **Build and run: *The native app*:**
  - `go run ./cmd/desktop-native`;
  - the shared data folder, settings, keychain and lock;
  - close hides the window, and the tray quits;
  - a release-like `go build` with `-H windowsgui` and the version;
  - a scratch user folder. `USERPROFILE` matters, since the default data folder is `%USERPROFILE%\Jenab`.

  The Wails table follows, marked *(Wails)*.
- **Tests: *The native UI's tests*:**
  - `go test ./internal/native/...`;
  - what each kit component's headless test checks;
  - the kit's own checks, including the import test;
  - the stores and the chat logic as ported TypeScript tests;
  - the screens against `statetest.Fake`;
  - the tester sees text, not pixels.

  The Bun tests and the server mode are marked *(Wails)*.
- **The bench:**
  - build `cmd/desktop-native` with `-tags bench`, then run `go run ./cmd/jenab-bench -native <exe>`;
  - what the bench build adds;
  - `-samples`, which needs the Wails bench app too, and `-hold` with `-gemini`;
  - don't commit the screenshots.
- **CI:** the Bun and bindings steps and the `cmd/desktop` build are marked *(Wails)*. `go test` covers the native tests. CI doesn't build `cmd/desktop-native` yet (TASK-060).
- **Commits:** a MyGo pin note: a version move is its own change, touches only the kit and `cmd/desktop-native`, and needs the user's OK to download. A Wails upgrade now touches `internal/webui`, not `internal/app`.

**CODE-OUTLINE**
- **Section 2:**
  - the start sketch now calls `webui.New`;
  - a paragraph on `internal/lifecycle`;
  - a new subsection *`cmd/desktop-native`: the MyGo app*, with a sketch of `main` and `run`, notifications, uploads through the in-process route, what it doesn't have yet, and the bench build.
- **Section 1:** a note on the updater and on the `native` import rule.
- **Section 4:** the object handler is in `webui`; the native app has no route.
- **Section 9:** `Notify` names both notification paths.
- **Section 10** is now *The UI: `native` and `frontend`*, with the same number:
  - the two-UI rule;
  - a `native` tree of `state`, `kit`, `shell`, `chat` and `chat/logic`, file by file, with notes on threads, screens, tests and what's coming;
  - the old `frontend` part under *`frontend` (until TASK-061)*, unchanged apart from the bug-fixes-only line.

**Tickets:** this ticket's status and Result, and its README row set to Done. SPIKE-048's Result already linked this ticket.

## Checks

- **Build commands:** both native builds in DEVELOPMENT.md were run (the release-like build and the `-tags bench` build). Both built, into a scratch folder that was deleted afterwards.
- **Claims checked against the code:**
  - the screens' imports (`go list`): `shell` and `chat` import `kit`, `state`, `app`, `chat`, `id`, and `chat` also imports `agent` and `store`;
  - the import test's walk;
  - the 16 ms coalescer in `agent`;
  - `kit.List` and `FollowEnd`;
  - the tone values against `index.css`;
  - the data folder default (`os.UserHomeDir()/Jenab`);
  - the keyring name, the same in both apps;
  - that neither app runs the scheduler or the updater yet;
  - the bench's flags.
- **Personal data:** the changes name no person, account ID or user-name path. The build line takes the module path from `go list -m` rather than writing it out.

## Found on the way (not changed here)

1. **The kit has no contrast check.** `kit.ToneColors` copies the tones of `index.css`, which `frontend/src/contrast.test.ts` checks for WCAG AA. The kit's own test only checks that text and fill differ. When TASK-061 removes `frontend/`, the check goes with it. SPEC 5.11 now says a kit check must come first. That fits TASK-055 (Settings → Appearance) or TASK-061.
2. **SPEC 5.9's `image` block** says "Shown through `<img>`, as in 4.7". Pages are native only now, so that wording is the webview's. 5.9 wasn't in this ticket's list, so it stays for the ticket that builds pages.
3. **No Taskfile task for the native app.** DEVELOPMENT.md gives the `go build` line by hand. TASK-059, with the installer, is where a `build:native` task fits.
4. **CI doesn't build `cmd/desktop-native`.** `go test ./...` compiles its packages, but the binary isn't built. TASK-060 covers macOS and Linux; a Windows build step could come sooner, but CI is blocked anyway.
