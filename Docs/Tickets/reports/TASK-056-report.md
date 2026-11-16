# TASK-056 report: the files panel, the file view and the dock in the native app

**Base:** `main` at 1f581f4, with MyGo v0.9.1 pinned.
**Model requests:** none. The tests use `statetest`'s fake services. The live check's only provider was a local Ollama on a closed loopback port, and no message was sent.
**Downloads:** none.

## 1. Reading the bucket without a route

The native app has no webview, so it has no `/objects` route (SPEC 4.5). Screens read files through the services:
- **`app.NativeBucket`** (`internal/app/bucket_read.go`) wraps `BucketService` and adds `Read(ctx, p, key, max)`, which leases the project and returns the head and at most `max` bytes, with `Cut` when there is more. It is a type of its own, so Wails doesn't bind a `Read`. Its test is `TestNativeBucketRead`.
- **`state.BucketAPI`** is what the stores ask for: List, Head, Versions, Read, Delete, UndoDelete and OpenWithSystem. `cmd/desktop-native` passes `app.NativeBucket{b.Bucket}`.

## 2. The stores (`internal/native/state`)

Ported from `frontend/src/shell` and `frontend/src/lib/files.ts`, with their tests:
- **`files.go`:**
  - The `Files` store holds one folder per project and prefix. It loads 100 entries a page (`More`), reloads when `Shell.Rev` changes, and keeps `cache/` out of the top folder (`HasCache`).
  - A file's view holds its head, its versions and at most `TextLimit` (1 MB) of a text file or a picture.
  - The helpers are `Crumbs`, `KindOf`, `TypeName`, `FormatSize`, `WhoText`, `NameOf`, `ParentOf`, `LinkOf` and `ParseLink`.
- **`openfile.go`:**
  - `OpenFile` docks a file in the open chat and shows its folder. `OpenHere` does the same for a stored tool output.
  - `OpenLink` checks a `jenab://` link with `Head`. A link to another project or a missing file says so in a toast and opens nothing.
  - `DeleteFile` shows a toast with *Undo*; for `cache/`, its detail is "From the app cache."
  - The rest are `OpenWithSystem` and `ListHidden`/`SetListHidden`. While something is docked, the chat list hides and the project's chats move to the rail, until the user shows the list again.
- **`links.go`:**
  - `MarkLinks` wraps bare `jenab://` links in `<…>`, since MyGo's GFM autolinks cover only http, www and mail. It leaves fenced code, code spans, reference definitions and links already written as links alone.
  - `CodeLink` makes inline code that holds just a link a link.
  - `SplitLinks` finds the bare links in the user's own text.
- **`bar.go`:** the bottom bar reads the memory and the providers at once, then every 2 s while the window shows (`Bar.Visible`). `WorkText`, `WorkChat`, `MemoryText`, `MemoryTip` and `ProblemText` are `format.ts`'s texts.
- **`statetest`:** `FakeBucket` adds files, versions, deletes with undo, opens with the system app, and memory and provider readings. It also has `AddProjectAs`, for a ULID project ID that links need.
- **Tests:** `TestFolderNavigation`, `TestALongFolderLoadsPageByPage`, `TestCacheIsHiddenFromTheTopFolderOnly`, `TestLinks`, `TestKindsSizesAndWho`, `TestOpeningALinkOpensTheFileNextToTheChatAndShowsItsFolder`, `TestALinkToAnotherProjectOrAMissingFileOpensNothing`, `TestTheDockOpenCloseAndADeletedFileCloses`, `TestTheChatListHidesWhileSomethingIsDocked`, `TestTheFileViewReadsTheHeadVersionsAndText`, `TestMarkLinks`, `TestWorkText`, `TestMemoryText`, `TestProblemText` and `TestTheBarReadsWhileTheWindowShows`.

## 3. The kit

Each new component is in a file of its own. Each has a headless test in `kit/files_test.go`, in light and dark, covering the keyboard, its label and right-to-left text.
- **`Tree`** (`tree.go`, on MyGo's `TreeBase`): rows with an active or hover fill, a chevron for branches, a context menu, and a *More* button on hover that isn't a tab stop. `TestTree`.
- **`Split`** (`split.go`, on MyGo's `ui.Split`): the side pane starts at a third. It stays at least `min` wide and leaves the main pane at least `min`. It goes on the start or the end side, and the arrow keys move the divider the way it looks. `TestSplit`.
- **The rest:**
  - `Tabs`, `PathBar` (MyGo's `Breadcrumbs`, named "Path"), `StatusBar` with `StatusText` and `StatusButton`;
  - `EndPanel` (the right sidebar, 288 px), `Picture` (decoded once per key);
  - `LinkText` and `LinkBubble` (links that run a function, not the browser), `Item`, `ToastAction` and `CopyText`;
  - eight Lucide icons (ISC).
- **Small edits to shared files:**
  - `markdown.go` gets `Own`, for links the app opens itself while the others still open in the browser, and `CodeLink`.
  - `toast.go` shows a toast's detail.
  - `tester.go`'s `HasRole` knows five more roles.

## 4. The screens

- **`files.Screen.Panel`, the Files tab:**
  - A path bar and *Refresh* sit over a tree of one folder. A folder opens in place. A file opens in the main area, with an icon by kind and its original name under its key.
  - Its menu has *Copy link*, *Open with the system app* (for types that open outside the app) and *Delete*.
  - More entries load as the end comes into view.
  - The top folder also lists *App cache*.
- **`files.Screen.View`, the file view:**
  - The header has the name, the title or folder, *Copy link*, *Open with the system app*, *Delete* and the dock's buttons.
  - Markdown shows as in the chat, with its own `jenab://` links. Other text shows in a box, and a cut file says "Only the first 1.0 MB is shown".
  - A picture is up to 480 px high. Other types show their type, their size and *Open with the system app*, or say why they don't open.
  - The versions follow, newest first.
- **`shell/dock.go`:**
  - **The dock:** with a file open, the file fills the main area and the chat sits in a `kit.Split` on the project's side, its width saved in `state.Shell`. *Move the chat to the left/right*, *Hide the chat* or *Show chat*, and *Close the file*.
  - **The right sidebar:** Pages, Pipelines, Files and Links. Only Files has content yet.
  - **The bottom bar:** the chats at work (a click opens the one waiting), provider pauses with the last problem as the tip, and memory, amber when low and red when critical.
  - **The rail:** it shows the project's chats while the chat list is hidden.
- **`chat`:**
  - Answers go through `MarkLinks` and `CodeLink`. A `jenab://` link opens the file next to the chat, and other links open in the browser.
  - In the user's message, a bare link is a link (`LinkBubble`).
  - "The full output is stored as …" opens the stored output.
- **`shell/files_test.go`,** in light and dark: `TestTheFilesTabOpensAFileNextToTheChat`, `TestALinkInAnAnswerOpensTheFile`, `TestABareLinkInTheUsersMessageOpensTheFile`, `TestAFileIsDeletedAndPutBack`, `TestTheWordFileAndThePicture` and `TestTheBottomBar`.
  - With `JENAB_SHOTS=<folder>`, they save the window. The ten pictures are in [TASK-056/](TASK-056/): `files-tab`, `file-docked`, `word-file`, `picture` and `deleted`, each `-light` and `-dark`.
  - They hold only made-up data.

## 5. The live check

- **The scratch folder:** `cmd/desktop-native` was built and run with `LOCALAPPDATA`, `APPDATA`, `USERPROFILE` and `TEMP` pointing to a scratch folder.
- **The seed:** a throwaway seed, not committed, made a project "Garden" with one chat. It uploaded three made-up files through the app's own upload handler:
  - `یادداشت‌های باغ.md`, Persian and English Markdown with a code block;
  - `report.docx`, a minimal Word file with one made-up sentence and no document properties;
  - `photo.png`, a gradient.
- **The messages:** the chat held a user message with a bare link and an answer with a link and a link in inline code.
- **The steps,** driven with Windows UI Automation:
  - The Files tab listed `uploads/` and *App cache*.
  - The folder opened in place.
  - The answer's link opened the Persian-named note next to the chat. The rail showed the project's chats, and the bottom bar showed the memory.
  - The picture and the Word file opened. The Word file showed its type, its size and *Open with the system app*.
  - *Delete* showed "Deleted photo.png" with *Undo*. Deleting `report.docx`, then *Undo*, showed "report.docx is back" and listed it again.
  - *Quit Jenab* closed the app. The log had no warnings or errors.
- **Not run live:**
  - **The tree's context menu:** the native menu closed when the script brought the window to the front.
  - ***Open with the system app*:** it would open Word on this computer.
  - Both are covered by the headless tests.
- **The live screenshots stay uncommitted:** the chat list's header shows the scratch project's folder, which holds a user-name path.
- **Keys and names:** keys are ASCII (SPEC 4.1), so the Persian name was stored as `uploads/file.md`, with the name as its title. The tree and the view show the title under the key, as the Wails app does.

## 6. Gaps

- **Audio and video:** MyGo v0.9.1 has no player, so the native app offers *Open with the system app* for them. SPEC 4.5 says so.
- **The divider's name:** MyGo names it "Divider" for assistive technology, and the kit can't rename it in v0.9.1. SPEC 5.12 says so.
- **The narrow layout (under 900 px):** the drawer and the overlays aren't done, since the native frame has no narrow layout yet. SPEC 5.12 says so.
- **The bottom bar's commands, pipelines and context use:** they come with their screens, as in the Wails app.
- **The composer in a narrow dock:** in a dock about 370 px wide, its row of pickers is cut off at *Approvals: Standard*. It is the chat's composer (TASK-054's area), not the dock.

## 7. Checks

- `go vet ./...` passes.
- `go test ./...` passes. `cmd/desktop` and `frontend` need `frontend/dist`, which isn't built in this worktree. They were checked with the main checkout's built copy, then it was removed.
- `TestOnlyTheKitImportsMyGoUI` passes: the new screens import only the kit and the stores.
