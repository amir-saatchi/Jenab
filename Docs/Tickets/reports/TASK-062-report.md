# TASK-062 report: the native app's leftovers before the switch

**Base:** `main` at 4c3365b, with MyGo v0.9.1 pinned.
**Model requests:** none. The tests use `statetest`'s fake services, and the live checks use the bench build's fake model.
**Downloads:** none.
**Real data:** every live run used a scratch profile. The bench build keeps keys in memory. The update check wrote the Apps entry under `HKCU` for the test install, and its uninstaller removed it. No real Jenab was installed on this PC.

## 1. One settings frame

Project settings now uses TASK-055's frame, `kit.SettingsLayout`, as `settings-view.tsx` does. The list on the start edge has the project's pages, and the body uses `SettingsTitle`, `SettingsSection`, `FactRow` and `SettingRow`. `kit/page.go` (`PageTitle`, `Section`, `DetailRow`, `Rule`) is gone; nothing else used it.

- Project settings also has the project's **Memory** section, as the Wails app does. It uses the same view as Settings › Memory (`settings.ProjectMemory`), on a new store, `state.ProjectMemory`. A memory chip in a chat opens it.
- The list is 224 px wide, and 176 px below 900 px, as Tailwind's `w-56` and `w-44` in `settings-layout.tsx`. This holds for both Settings and project settings.

Tests: `TestProjectSettingsShareSettingsFrame`, `TestAProjectMemoryChipOpensItsSection`, `TestTheSettingsListNarrows` (in `internal/native/settings`, so the shared frame is checked in one place).

## 2. Right-to-left text in inputs

`TextInput`, `TextArea` and the composer's text take their direction from the first strong letter, as `dir="auto"` does (`kit.RightToLeft`). A Persian name is aligned right and an English one left. The layout isn't mirrored: the label, the buttons and the field's place stay where they are. A path field stays left to right.

Tests: `TestTextFieldsTakeTheirTextsDirection` checks the name field, a text area and the composer. Each starts empty, then gets Persian that starts with a digit, then Latin text. The label stays before the field. `TestPathInputStaysLeftToRight` checks the path field.

## 3. The narrow layout, under 900 px

As `frontend/src/shell` and `use-mobile.ts` do. New in the kit: `Narrow` and `Sheet` (`kit/narrow.go`), the icons `menu` and `panel-right`, and the role "dialog" in the tester. New in the shell: `shell/narrow.go`.

- The rail, the chat list, the right sidebar and the bottom bar are hidden.
- ☰ (*Open the sidebar*) in the chat header opens the **left overlay**: the rail and, with a chat open, the chat list, across the full height on the start edge. Choosing a project, a chat, Settings or the developer tools closes it.
- *Pages* in the chat header opens the **right overlay**: the right sidebar (Pages, Pipelines, Files, Links) on the end edge.
- A **file** opened from the Files tab fills the main area. *Chat* opens the chat as a **drawer**, 448 px wide (`w-[28rem]`) or the window's width if less, on the end edge. The swap and hide buttons are not shown, as in the Wails app.
- A **page without a chat** (Settings, project settings, the developer tools, the welcome page) gets a header with ☰ and the page's title.
- Escape or a click on the dimmed window closes an overlay or the drawer. Above 900 px, the frame is as before.

Tests: `TestSheet`, `TestNarrow` (899 is narrow, 900 isn't), `TestTheNarrowWindow` (at 800 px: the overlays, the file and the drawer, then back at 1280 px), `TestANarrowPageHasAHeader`, `TestPageTitle`. All of them run in light and dark.

## 4. The composer in a narrow dock

The picker row checks its own width (`kit.Narrower`). Below 432 px, the model's label is cut at 96 px, and the approval level shows only its icon (`kit.MenuPicker` with an empty label), as the Wails composer does with `@container` and `max-w-24`. The picker keeps its full name for assistive technology, for example "Approvals: Standard". The row now fits in a 320 px dock.

Test: `TestTheComposerFitsANarrowDock`. It uses a 370 px dock: the level picker is 64 px wide or less, no buttons overlap, and *Send* ends inside the window.

## 5. The Apps entry's version after an update

MyGo's NSIS installer writes `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\io.github.amir-saatchi.jenab` with `DisplayVersion`. Its updater never changes that value. On Linux it calls `update.RefreshDesktopEntry`, but on Windows it does nothing.

The fix is in Jenab (`cmd/desktop-native/appsentry*.go`):

- At each start on Windows, the app reads the entry.
- It writes the running version into `DisplayVersion` only if the entry exists, its `InstallLocation` is the app's folder, and the version differs.
- It never creates the entry, and it skips a `dev` build, so a copy run from elsewhere leaves the installed one's entry alone.
- `appinfo.Identifier` holds the identifier, and `version_test.go` checks it matches `mygo.json`.

Test: `TestTheAppsEntryTakesTheVersionAfterAnUpdate`, with a fake entry. It covers a write; the folder in another case, quoted, with a trailing separator; the same version; another copy, such as a build in the repo; a dev build; and an entry with no folder. On other systems it builds to a no-op (`GOOS=linux go vet` is clean).

**MyGo issue, drafted and not filed.** I checked MyGo v0.9.1, v0.9.2 (2026-11-16) and main at 6fbc3c5. `DisplayVersion` appears only in `cmd/mygo/nsis.go`. The draft:

> **The updater doesn't refresh the Windows Apps entry's version**
>
> The NSIS installer writes `DisplayVersion` under `HKCU\…\Uninstall\<identifier>`. After `Updater.Install`, the files are the new version, but Settings › Apps still shows the installed one. On Linux the updater calls `update.RefreshDesktopEntry`; Windows has no counterpart. Could `Install` (or the first start after it) set `DisplayVersion` when the entry's `InstallLocation` is the running app's folder? Jenab does this itself at start for now.

## Live checks (Windows 11, 125 % scale, scratch profile)

**Items 1 to 4** were checked with the bench build (`-hold`, dark theme), driven through its port. A new route was added to the bench, `POST /file?path=`. It uploads a file into the open chat's project and opens it with the chat in the dock. `/size` set the width to 1280 and then 800.

| Check | Result |
|---|---|
| Project settings, 1280 | the shared frame, with the list (224 px) and the Memory section |
| Project settings, 800 | the ☰ header "Project settings", with the list at 176 px |
| Left overlay, 800 | the rail at x = 0 across the full height; with a chat open, the chat list beside it; choosing the project closed it; Escape closed it |
| Right overlay, 800 | on the end edge across the full height; Escape closed it |
| Persian draft (`/open?draft=`) | right-aligned in the composer, with the layout unchanged |
| Composer in the dock, 1280 | dock 320 px; the model label cut ("bench-m…"); the level picker icon only, 62 px, named "Approvals: Standard" in UI Automation |
| File and drawer, 800 | the file fills the area; *Chat* opens the drawer on the end edge; the picker row fits; Escape closes it |
| Back to 1280 | the rail, the list, the dock and the right sidebar return |
| Log | three lines, no warnings or errors |

**Item 5** was checked with a local update, as in TASK-059. 0.1.1 (with its installer) and 0.1.2 were built with the bench tag and a test key.

| Check | Result |
|---|---|
| Silent install of 0.1.1 into a scratch folder | exit 0; Apps entry `DisplayVersion` 0.1.1, `InstallLocation` the scratch folder |
| Start, then `/update` install and restart | 0.1.2 found, installed and restarted; `/state` shows 0.1.2 |
| Apps entry after the restart | `DisplayVersion` 0.1.2; the log has `app: apps entry version=0.1.2` |
| A second start | no new write (one log line in all) |
| `Uninstall.exe /S` | the entry and the folder are gone |

## Not done, and gaps

- **The focus when an overlay opens.** MyGo gives the focus to the first control in a modal, and v0.9.1 can't turn that off. When the drawer opens, ☰ shows its focus ring and tooltip. The Wails app keeps the focus off its overlays. Fixing this needs a MyGo option.
- **A dot on the chat button for new activity** (SPEC 5.12): the Wails app has none either, so it isn't ported.
- **The chat row's ⋯ always shown below 900 px**: still shown only on hover, as above 900 px.
- **The narrow layout on pages and pipelines** (rows stacking, 5.9) comes with those screens.

## Checks

`go build ./...`, `go vet ./...` and `go test ./...` pass. The changed Go files are gofmt-clean. Screenshots are kept outside the repo.
