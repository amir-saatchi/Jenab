# TASK-057 report: the project screens in the native app

**Base:** `main` at 1f581f4, with MyGo v0.9.1 pinned.
**Model requests:** none. The tests run on `statetest`'s fake services, and the live check needs no model.
**Downloads:** none.

## 1. What was ported

The screens are in a new package, `internal/native/projects`. It imports only `kit`, `state` and the Go side's types, never MyGo (`TestOnlyTheKitImportsMyGoUI` passes). The shell holds a `*projects.Screens` and calls it.

- **Welcome page** (`welcome.tsx`): it replaces the shell's "Choose a project". It has the folder icon, *Welcome to Jenab*, *Create project*, and *Open the example*, disabled with a tooltip as in the Wails app. Below them, "Saved in <folder> · change in Settings" and the folder warning when it applies.
- **New project** (`new-project.tsx`): a dialog with *Name*. The dialog is locked while the project is being created.
  - A name the service refuses shows under the field; other errors go to a toast.
  - On success, the dialog closes and the new project opens at its Mother chat.
  - The rail's *New project* opens it too.
- **Project settings** (`project-settings.tsx`, with TASK-050's changes): one scrolling page, titled with the project's name, with "Project settings · created <date>".
  - Beside it, *Back to chats* and a list of the sections. A click in the list brings that section into view.
  - **General:** the name, the folder and the folder warning (`folder-warning.tsx`).
  - **Approvals:** *Strict*, *Standard* and *Auto*, through `Actions.SetLevel`, as the composer's picker does. One change runs at a time.
  - **Private-network exceptions** (`private-hosts.tsx`): the hosts, each with *Remove*, and *Add a host*. The field checks the host before it is sent (`state.HostProblem`), and a host the service refuses shows under it.
  - **Export:** *Export…* opens the project export. SPEC 5.12 lists it, and the Wails page has no such section; it reaches the dialog the project menu opens.
  - **Memory:** the page shows a *Memory* section only when the hook `Screens.ProjectMemory` is set. The project's memory screen belongs to Settings (TASK-055).
  - **The frame:** the list and *Back to chats* are built in `projects` until TASK-055 has the frame both settings pages share.
- **Export project** (`export-project.tsx`, P2-12):
  - *Zip file* or *Folder*, and *Include approvals*, then the size estimate.
  - *Export…* asks for the place in the Windows save dialog, then shows the step, "done / total" and a progress bar, with *Stop*.
  - The result gives the size, the time, the path, any missing files, and the line on copying the export into the projects folder.
- **Export chat** (`export-chat.tsx`): *Markdown* or *JSON* with what each holds, then the save dialog. The result is "Exported N messages, size." and the path.
- **The chat menu** (`chat-menu.tsx`):
  - In the chat header: a ⋯ *Chat menu* with *Export chat…*.
  - On each chat row: a ⋯ *Chat menu: <title>*, shown in place of the badge on hover or focus.
  - The chat list's title is a *Project menu* with *Project settings* and *Export…*.
- **The rail:**
  - *Search*, disabled with "Search comes with a later version", as in the Wails app.
  - While the chat list is hidden, the chats move into the rail, below a divider. Each chat is a tile with its initials, and Mother's tile has the diamond. A bar marks the open chat, a spinner means working, and an amber dot means waiting. *New chat* follows.

## 2. The stores

Both are additive; `Stores` gained two fields.
- **`state.Hosts`** (`hosts.go`): each project's private-network exceptions.
  - `Load` and `Change`. A change starts from the stored list, and one change runs at a time.
  - `HostProblem`, `WithHost`, `WithoutHost` and `HostAdvice`, ported from `private-hosts.tsx` with its test.
- **`state.Exports`** (`exports.go`): the project and chat exports.
  - The estimate, then the save dialog (`Exports.Save`, set by `cmd/desktop-native` to `mygo.Dialog.Save` with the filter).
  - The run, its progress and *Stop*, and an export from before followed when the dialog opens again.
  - The file names and paths, ported from `export-project.tsx` and `export-chat.tsx` with their tests. A cancelled save dialog does nothing.
- **`statetest`:** the fake's project and export services (`fake_project.go`). `Admin().Gate` holds an export, so a test can see it running.

## 3. The kit

Every piece is in a new file, so `kit.go`, `theme.go` and the existing components are unchanged.

**Names kept apart from the parallel tickets:** TASK-055's work in progress has `Choice`, `Segmented` (in `segmented.go`) and `FactRow`, and TASK-056's has `IconFolder`. This ticket's equivalents are therefore named `Segment`, `SegmentGroup` (`segmentgroup.go`), `DetailRow` and `IconProjectFolder`, so the branches merge without a conflict or a duplicate name.
- Once all are merged, one of each pair can go.
- `icons/arrow-left.svg` and `icons/folder.svg` are byte for byte the files those branches add.

| File | Pieces |
|---|---|
| `form.go` | `Field` (label, advice or problem, `Invalid` for assistive technology), `CheckField` |
| `page.go` | `PageTitle`, `Section`, `Reveal`, `DetailRow`, `Rule` |
| `segmentgroup.go` | `Segment`, `SegmentGroup` (one Tab stop, a radio group for assistive technology) |
| `progress.go` | `Progress` |
| `itemrow.go` | `ItemRow` |
| `path.go` | `Path`, `PathInput` (mono, left to right, as paths and hosts are) |
| `textlink.go` | `TextLink` |
| `actionbutton.go` | `ActionButton` (icon or spinner, then the label; disabled while busy) |
| `dialogparts.go` | `DialogText`, `DialogFooter`, `Locked` (a dialog Escape and the backdrop can't close while it works) |
| `navrow.go` | `NavRow` (a chat row with its ⋯ menu), `MenuTitle` (the chat list's title as a menu), `TileSpinner` |
| `icons_project.go`, `icons/` | `IconProjectFolder`, `IconDownload`, `IconFileJSON`, `IconFileArchive`, `IconArrowLeft` (Lucide, ISC) |
| `tester_pages.go` | `InView` and `KeyBackspace` for the screens' tests |

## 4. Tests

- **Kit:** 12 tests in `project_components_test.go`. Each runs in light and dark, and checks the label, the keyboard and right-to-left text.
- **Stores:**
  - `hosts_test.go` has 3 tests and `exports_test.go` has 11.
  - They port `private-hosts.test.ts`, and the file-name and path parts of `export-project.test.ts` and `export-chat.test.ts`.
- **Screens:**
  - `screens_test.go` has 12 tests, on the shell with the fake services, each in light and dark. They cover the welcome page, the folder warning on it and in project settings, a new project opening its Mother, a refused name, the settings page (level, hosts), a refused host, the list bringing a section into view, a project export run and stopped, a chat export from the header and from a row's menu, and the chats in the rail.
  - `format_test.go` ports `percent` and `stepText` from `export-project.test.ts`, and `formatText` from `export-chat.test.ts`, then checks the numbers the dialogs show.
- **Screenshots:** with `JENAB_SHOTS=<folder>`, the screens' tests save 24 PNG files, 12 screens in light and dark. They are kept outside the repo.
- **Checks:**
  - `go vet ./...` passes.
  - `go test ./...` passes, except `internal/scenario`'s `TestRunTimeout/in_a_refresh's_merge` in the full run: "0 calls" after 4 s, under the full run's load. Run alone, it passes in 8 s. This ticket doesn't touch that package.

## 5. Live check

This used a bench build of `cmd/desktop-native` on a scratch user folder, as the bench does, driven through UI Automation. On 2026-11-16, in the dark theme:
1. The welcome page showed. *Create project* → "قیمت طلا" → *Create*: the project opened at its Mother chat.
2. *Project menu* → *Project settings*:
   - *Strict* was chosen, and the chat got its notice ("the approval level changed from standard to strict").
   - "127.0.0.1:8099" showed "Give the host without a port." `nas.local` was added and listed with *Remove*.
3. *Export…*:
   - The estimate showed, and *Zip file* went through the Windows save dialog: "Exported 7.6 KB in under a second." The zip holds the project folder with `project.db` and `chats.db`.
   - Then *Folder* with *Include approvals*: "Exported 180.2 KB in under a second." The folder holds the same.
4. *Chat menu* → *Export chat…* → *Markdown* → the save dialog: "Exported 1 message, 0.2 KB." The file starts `# Mother`, with the project's name in `<bdi>`.
5. The app quit through the bench's `/quit`.

The screenshots hold a user-name path, so they stay outside the repo.

**One slip:** the first save dialog was driven at the wrong control, and the zip went to the dialog's default folder. It was moved into the scratch folder at once, and the helper was fixed for the other two saves.

## 6. Differences and follow-ups

- **Row menu labels:** each chat row's ⋯ is labelled *Chat menu: <title>*, so a screen reader tells the rows apart. The Wails app labels every row *Chat menu*. The header's stays *Chat menu*.
- **Export in project settings:** the page has an *Export* section (SPEC 5.12); the Wails page doesn't.
- **Right-to-left input:** a Persian name typed into a `TextInput` is aligned left. That is the kit's `TextInput` now; the Wails input uses `dir="auto"`. It's left for a kit change, since `input.go` is shared.
- **Storage and the workspace folder:** SPEC 5.12 lists them on project settings, but neither app has them yet.
- **The settings frame:** the page's own frame should give way to TASK-055's when both are merged.
