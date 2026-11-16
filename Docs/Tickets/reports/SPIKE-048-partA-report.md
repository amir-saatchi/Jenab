# SPIKE-048 Part A report: the native foundation on MyGo

**Branch:** `claude/spike-048-part-a-67972e`. Commits: fcae61f (spike doc), 05ec55f (A1, Go 1.27.3), 1f0a37e (A2, services without Wails), 507ea64 (A4, stores), 37e224b (A5, kit), and the last one with A3 (`cmd/desktop-native`), this report and the Result note. Nothing is pushed or merged.
**MyGo:** `github.com/egoist/mygo v0.7.4`, pinned. It is the one new UI dependency. It brings `go-text/typesetting`, `golang.org/x/image` and `ebitengine/purego` as indirect dependencies.
**Downloads:** the Geist 1.9.2 and Vazirmatn 33.003 release zips, with the user's OK. Versions and SHA-256 are in `internal/native/kit/fonts/README.md`.

## 1. What was built

1. **Go 1.27.3.** `go.mod` says `go 1.27.3`, and the `toolchain` line is gone. The Wails app builds, every Go test passes, and `wails3 task check:bindings` finds the bindings unchanged.
2. **Services without Wails.** The Wails wiring moved to `internal/webui`: the app, the event registrations and the dark-mode probe. The lock, the shutdown and the start-up error box moved from `cmd/desktop` to `internal/lifecycle`. `internal/app` and the `Publisher` build without Wails, and `go list -deps` confirms it. The Wails app behaves as before.
3. **`cmd/desktop-native`** (`main.go`, `view.go`, 342 lines):
   - **Start:** a query worker exits first, as in the Wails app. `mygo.App.RequestSingleInstanceLock` comes next, then the same `lifecycle.LockRoot` the Wails app takes, so only one of the two apps runs at a time. The services are built in the same order as in `cmd/desktop`.
   - **Events:** a native `Publisher` posts each event to `Stores.Apply` through a `state.Mailbox`. The mailbox keeps the events sent before the window exists, then hands them to `Window.Update`, which returns without waiting.
   - **The window:** one window with `StateKey: "main"`. Closing it hides it unless the app is quitting. `OnWindowAllClosed` has a listener, so the app keeps running. A second launch shows and focuses the window.
   - **Notifications:** sent with `mygo.NewNotification`. A click shows the window and sends `app:open`.
   - **Theme:** `Settings.OnTheme` sets the theme source (light, dark or System). `WindowActive` comes from the window, so a chat shown in an active window sends no notification.
   - **Shutdown:** `lifecycle.Shutdown`, as in the Wails app.
   - **The view:** a placeholder. A sidebar lists the projects; choosing one opens its chat through the stores. The main area shows the chat's title, its message count and its state. Toasts come from the stores, and a Quit button sits in the sidebar.
4. **Stores** (`internal/native/state`, 54 tests): the React stores in plain Go, used on the UI thread only. Their async work runs in goroutines and posts results back. Each store's tests were ported with it, and the stores import neither MyGo nor Wails.
5. **The kit, first part** (`internal/native/kit`: 796 lines, plus 623 lines of tests):
   - **Theme:** the `index.css` tokens for light and dark, the six tones of SPEC 5.11, a mapping to `ui.Theme`, and `SetThemeSource`.
   - **Fonts:** Geist, Geist Mono and Vazirmatn, embedded as variable TTFs and registered with `ui.RegisterFont`.
   - **Icons:** 16 Lucide icons, ISC-licensed, the same set the frontend uses.
   - **Components:**
     - `Button` with the five shadcn variants, `IconButton` and `Disable`;
     - `TextInput` and `TextArea`;
     - `Tooltip`, `MenuButton`, `MoreButton`, `Dialog`, `Heading` and `AlertDialog`;
     - `Badge` in a tone, `Spinner`, `Alert` and `Empty`;
     - `Sidebar`, `SidebarSection` and `SidebarItem`;
     - `Toast` and `Toasts`;
     - `Markdown`, with `View` and `Stream`.
   - **Layout helpers:** `Root`, which keeps the window left to right, plus `View`, `Row`, `Column`, `Box`, `Scroll`, `Text`, `Icon` and `Local`.
   - **Tests:** every component has a headless `ui.NewTester` test. Each one:
     - runs in a light and a dark window and checks the window's background;
     - finds the component by its label;
     - checks that Tab reaches it (or passes over it, if it takes no input);
     - shows Persian text in it.
   - **Other kit tests check that:**
     - the layout isn't mirrored when the window's direction is right to left, while Persian text still starts on the right;
     - the fonts draw weights 300 to 700 differently;
     - tabs draw as 4 spaces;
     - only the kit imports `github.com/egoist/mygo/ui` or its packages. The test walks the whole repository.
   - **`NOTICE`** names the fonts (OFL) and Lucide (ISC).

**Checks run:**
- `go vet ./...` and `go test ./...` pass. The kit and state tests also pass with `-count=3`. `-race` needs cgo, which this machine doesn't have.
- One package failed once: `internal/scenario` `TestRunTimeout` ("0 calls"). That run shared the machine with the smoke run and `bun test`, and the test gives a scenario 2 seconds. It passed alone twice, and so did its whole package. The test doesn't touch this spike's code, but it's timing-sensitive under load.
- `wails3 task check:bindings` passes.
- In `frontend`: `bun run typecheck` is clean and `bun test` passes 136 tests.
- **Smoke run** of `cmd/desktop-native` (Windows 11), with `LOCALAPPDATA` and `APPDATA` pointed at a temporary folder:
  - the app starts, opens its window and writes its log;
  - a second launch exits at once, with code 0, and the first keeps running;
  - the working set with the placeholder view was 114 MB.

  The default data folder isn't under those variables, so the start-up scan added the two projects in the real data folder to the temporary registry. The scan only reads each project's metadata (`store.ReadProjectMeta`); no project was opened. Part B's runs should set the data folder too.

**Not checked live** (no UI automation here; these belong in Part B's checks):
- whether closing the window hides it;
- whether the size and place come back after a restart;
- whether a notification click opens the chat;
- how DirectWrite renders the fonts in a real window (the weight test runs on the headless renderer).

## 2. What MyGo lacked or got wrong

1. **Tabs draw as nothing.**
   - **Where:** in `ui.Text` and in Markdown code blocks, so Go code indented with tabs loses its indentation. A headless test shows the frame with a tab is identical to the frame without it.
   - **Upstream:** a tab size for text came in [PR #1217](https://github.com/egoist/mygo/pull/1217), for [issue #1182](https://github.com/egoist/mygo/issues/1182), merged on 2026-11-04, after v0.7.4. It is in v0.8.1 and later.
   - **Kit fix:** `ExpandTabs` turns tabs into spaces to stops of 4. This keeps the block structure that CommonMark gives tabs. Text that grows by appending still expands to text that grows by appending, so streaming works.
   - **Copying:** the kit's code blocks copy the code with its tabs. To get it, they read block *i* again from the unexpanded source. A selection copied by hand still gets spaces.
   - **The fix can go:** `TestMyGoDropsTabs` fails once MyGo draws tabs.
2. **The sidebar's selected item can't be themed separately.** While the sidebar has the focus, `SidebarItem` fills the selected item with `Theme.Accent` (as AppKit does). Without the focus it uses `SurfacePressed`. The kit puts the sidebar in its own `ui.Themed` theme: the unfocused selected item and hover use the sidebar-accent gray, as in shadcn. The focused selected item stays in the primary color, unlike the web app.
3. **One surface for every control.** `ui.Theme` has one `Surface` and one `Border`. shadcn has separate input, border and ring tokens, plus `dark:bg-input/30`. The kit maps them as closely as one theme allows and styles each component after it is built.
4. **`ButtonBase` has no disabled look,** and `IsDisabled` can't be read while the button is built, because `Disabled` is set later. The kit adds `Disable(e, d)`, which also sets opacity 50%.
5. **The tester doesn't see field text.** `Tester.Texts` leaves out the values and placeholders of text fields, so the input tests check the bound string instead.
6. **The pace of releases.** MyGo went from v0.7.4 (2026-11-01) to v0.9.1 (2026-11-15) during the spike. A pinned version falls behind fast, and fixes such as #1217 land after it.

## 3. Upstream issues worth filing

None was filed: posting to a public tracker needs the user's OK.

1. **The sidebar's selected-item colors.** A theme field or a style hook, so an app can show the focused selected item as it likes rather than always in `Accent`.
2. **Text-field text in the tester.** `Tester.Texts` and `Find` could include text-field values and placeholders, or a `Tester.Value(label)` could read them.
3. **Tabs in Markdown code blocks**, only if the move to v0.8.1 or later shows they still draw as nothing. #1217 covers `ui.Text`. Whether Markdown's code body uses the tab size has to be checked on that version.

## 4. Hours

About 0.8 hour of agent time from the first commit to this report, plus the user's answer on the font download. MyGo's headless tester made the component tests cheap to write. Most of the kit's time went into reading MyGo's sources for what its docs leave out: the sidebar's colors, the code block's copy button, and what the tester sees.

## 5. What Part B needs

1. **Decide the MyGo version first.** Moving to v0.9.1 brings the tab fix (#1217) and two weeks of changes. It needs a download, so ask the user. After the move, run `TestMyGoDropsTabs` and drop `ExpandTabs` if it fails.
2. **Kit pieces for the chat slice:**
   - the rail;
   - a virtualized message list (`ui.List`), to open a 200-message chat;
   - thinking parts (collapsible);
   - tool and approval cards;
   - the `ask_user` form (radio, checkbox, text);
   - marker rows;
   - the composer: a growing text area with *Send* and *Stop*, the quote chip, attachments, and the model picker (select or combobox).
3. **Live checks still open from Part A:**
   - closing the window hides it;
   - the size and place come back after a restart;
   - a notification click opens the chat;
   - DirectWrite renders Geist and Vazirmatn with variable weights.
4. **The measurements in item 9** need a fake provider streaming at a fixed rate, the SPIKE-022 laptop, and the Wails app built from the same commit.
5. **A way to quit** other than the placeholder's Quit button, such as a tray or a menu. Closing the native window only hides it.
