# TASK-063 report: the native app's own title bar

**Base:** `main` at e2dc0b9, with MyGo v0.9.1 pinned.
**Model requests:** none. The tests use `statetest`'s fake services, and the live check uses the bench build's fake model.
**Downloads:** none.
**Real data:** the live check used a scratch profile, deleted after the run. No system dialogs were opened.

## 1. Hidden title bar, system buttons kept

On Windows the window opens with `TitleBarStyle: mygo.TitleBarHidden` (`titleBarStyle` in `cmd/desktop/main.go`). Windows' minimize, maximize and close buttons stay. `HideWindowButtons` isn't used. macOS and Linux keep the system's title bar until TASK-060.

The top row of each column lines up as one bar:
- the chat list's head, with the project's name and menu (`kit.HeadedSidePanel`, `kit.PanelHead`);
- the chat's or the file's header (`kit.Bar`);
- the right sidebar's head (`kit.PanelHead`).

The bar is 56 px high, or `c.TitleBar().Height` if that is taller. The bar at the window's end edge ends with `kit.ControlsRoom`, which keeps `c.TitleBar().Right` free for the buttons. Without system buttons it takes no room. It doesn't handle `Left` yet, which only macOS needs (TASK-060).

The shell decides which bar is at the end edge (`shell/titlebar.go`, `chatEnds`):
- the right sidebar's head, while it shows;
- otherwise the chat's header, or the file's header when the chat is hidden or docked on the start side.

## 2. Dragging

`kit.Bar` and `kit.PanelHead` call `DragWindow()`. MyGo gives a press to the deepest interactive element under the pointer (`ui/input.go`). So buttons inside the bar still get their clicks, and only the empty parts move the window. A double-click there maximizes, and another one restores.

## 3. The window buttons' theme

`SetWindowButtonsTheme` gets the app's theme when the window opens, and again on each change through `Settings.OnTheme`. `kit.ThemeSourceOf` maps the setting to light, dark or system for both the app and the buttons.

## 4. The right sidebar's tabs as icons

There are four toggle buttons at the end of the bar, 8 px before the window's buttons: *Pages* (`layout-dashboard`), *Pipelines* (`workflow`), *Files* (`folder`) and *Links* (`link`). They are Lucide icons, with tooltips and names, in a group named *Project sidebar* (`kit.BarToggle`, `kit.BarGroup`).
- A click opens the sidebar on that tab. A click on the open tab's icon hides the sidebar. The open tab's icon is pressed (accent background).
- The icons sit in the sidebar's head while it shows. While it is hidden, they sit at the end of the chat's or the file's header, in the same place. So a second click lands on the same icon.
- The sidebar has a small title naming its tab (`kit.PanelTitle`) instead of the tab row. *Files* has no title, because its path bar already starts with *Files*.
- `state.Shell` has a new field, `RightHidden`, kept in memory like the tab. `RightOpen` and `ToggleRight` work the same in wide and narrow windows. `state.OpenFile` opens *Files* and shows the sidebar.
- SPEC 5.12 and `right-sidebar.tsx` say nothing more about hiding the sidebar.

## 5. Narrow layout, under 900 px

The icons stay in the chat's header. They open the right overlay on their tab and close it the same way. They replace the old *Pages* button, and the `panel-right` icon is gone. ☰ stays at the start. Back at 1280 px, the sidebar shows again.

## 6. Other screens

Settings, project settings and pages without a chat (the welcome page, titled "Jenab") get the page header in a wide window too: the title, then `ControlsRoom`. Before, they had it only in a narrow window. The developer tools' own bar ends with `ControlsRoom`. A file view's bar is the file's header (section 1).

## Choices made

- **Icons:** `layout-dashboard` for Pages and `workflow` for Pipelines, next to the kit's `folder` and `link`. They are the plainest Lucide matches.
- **The bar is 56 px**, the existing header height, and grows only if the system's buttons are taller.
- **The selected icon** uses the accent background.
- **Enter doesn't press a toggle; Space does.** This is MyGo's convention for toggle buttons (`ui.ToggleBase`).
- **A narrow developer tools page has two bars:** the ☰ header and the tools' own bar. This was already true before this ticket.

## Tests (headless, light and dark)

- `kit/titlebar_test.go`:
  - `TestBarKeepsClearOfTheWindowButtons`: nothing focusable under the buttons, a right-to-left title, the group's name, clicks and Space on the toggles, their pressed state, and a double-click on the empty bar that clicks nothing.
  - `TestControlsRoomIsEmptyWithoutWindowButtons`
  - `TestPanelHeadLinesUpWithTheBar`: titles centered, and the rules on one line for no title bar, Windows' and a taller one.
  - `TestPanelTitle`, `TestThemeSourceOf`
- `shell/titlebar_test.go`:
  - `TestTheTabIconsOpenSwitchAndHideTheSidebar`
  - `TestTheTabIconsEndTheDockedBars`: the chat docked on the end side, on the start side, and hidden.
  - `TestTheTabIconsOpenTheNarrowOverlay`
  - `TestOtherPlacesHaveTheBar`: Settings, project settings and the developer tools.
  - `TestToggleRight`
- The test helpers `kit.WindowsButtons`, `kit.Pressed` and `kit.UnderWindowButtons` set Windows' button room (32 × 138 DIPs) in the tester.

Screenshots in light and dark are kept outside the repo. Headless shots don't draw the system's buttons.

## Live check (Windows 11, 125 % scale, bench build, scratch profile)

The bench build ran with `-hold`. Its port set the size, and a new route, `POST /theme?s=`, sets the theme as Settings does. The mouse was moved with `SetCursorPos` and `mouse_event`, and the names were read with UI Automation.

| Check | Result |
|---|---|
| 1280 px | no system title bar; the four icons in the sidebar's head, before Windows' buttons |
| Drag the chat's header | the window moved by the drag, 120 × 60 px |
| Drag the sidebar's head (under the buttons), the narrow header, the overlay's head | the window moved |
| Drag in the chat list | the window stayed |
| A drag that starts on a button (an icon, the project's name) | acts as a click on that button, and the window stays |
| Double-click the bar | maximized, then restored; a double-click in the chat does nothing |
| Hover on maximize | Windows' snap layouts open |
| Theme light, dark (`/theme`) | the buttons' glyphs dark on light, light on dark, changing at once |
| Files (open) clicked | the sidebar hides; the icons stay in place in the chat's header |
| Links clicked | the sidebar shows *Links* under the title "Links" |
| 800 px | ☰, the title, ⋯ and the four icons, clear of the buttons |
| Pipelines at 800 px | the right overlay on *Pipelines*; the icons stay in place; clicked again, it closes |
| UI Automation | a group "Project sidebar" with four buttons named Pages, Pipelines, Files and Links, each with a toggle state (On for the open tab) and its tooltip as help text |
| Log | no warnings or errors |

## Not done, and gaps

- **Narrator by ear:** the icons were checked through UI Automation, which is what Narrator reads. A spoken pass with Narrator wasn't done, since it needs someone to listen.
- **macOS:** `ControlsRoom` handles only the end side. macOS needs `Left` on the start side (TASK-060).
- **The rail's top** didn't move the window. The follow-up below adds a strip that does.

## Checks

`go build ./...`, `go vet ./...` and `go test ./...` pass, and so does `go vet -tags bench ./cmd/desktop`. The changed Go files are gofmt-clean.

## Follow-up (2026-11-17): compact bar

The user saw that the bar was much taller than Windows' buttons (56 DIPs against 32), so the tab icons sat lower than them, and the bar looked heavy.

**Height.** Every column's top bar is now one line as high as the window's buttons, `c.TitleBar().Height`, with its 1 DIP rule under them. On Windows 11 that is 33 DIPs (`barHeight`). The rule sits under the buttons, so the row they share is exactly theirs. Where the system keeps its title bar (macOS and Linux until TASK-060), the bar is 40 DIPs (`kit.BarHeight`).

**Buttons.** The bar's buttons are 28 DIPs with 16 DIP icons: the tab toggles, ⋯, the hide-list button, ☰, the dock's buttons, the file's buttons and the developer tools' two tabs (`kit.BarButton`, `kit.BarIconButton`). Their focus ring is on their edge (`FocusRingOffset(0)`), so its 2 DIPs fit in the 32 and aren't cut at the window's top. A headless shot of the focused icon showed the whole ring.

**What moved where:**
- The chat list's head shows the project's name and the chevron on one line, 28 DIPs high. The folder is the name's tooltip, which screen readers read as its description. The project menu doesn't show the folder, so the tooltip is where it is now.
- The chat's header has no second line. Last active, Waiting for you, Working…, the role and Project home stay on the chat's row in the chat list. The title is one line, cut with an ellipsis, with all of it as its tooltip. Mother's diamond stays.
- The file view's header lost its second line too, since two lines don't fit in 32 DIPs. The file's title is the name's tooltip, or its path under Files.
- Page headers (Settings, project settings, pages without a chat), the developer tools' bar, the right sidebar's head and the narrow bar with ☰ all use the same height.
- The rail's top is an empty strip as high as the bar, which moves the window and maximizes it on a double-click. The tiles start under it, about 29 DIPs lower than before. This only happens where the window has its own title bar.

**Tests** (headless, light and dark):
- `TestPanelHeadLinesUpWithTheBar` now checks that the titles and a toggle are centered at the buttons' middle, with the rule under them, for no buttons, Windows' and taller ones.
- `TestBarKeepsClearOfTheWindowButtons` checks the toggles are 28 DIPs and centered.
- `TestMenuTitleAndTileSpinner` checks the folder is the description and not a line, and the title is 28 DIPs.
- The shell's `clear` helper now also checks that every control in the bar is centered with the window's buttons (`kit.OffTheWindowButtonsMiddle`), in every place the TASK-063 tests visit.
- New `TestTheBarIsOneLine`: the folder as the project menu's description, no second line in the chat's header (neither Project home nor Last active), a Persian title with itself as its description and centered at 16, and the rail's tiles under the bar.

**Live check** (bench build, scratch profile, 125 % scale), with UI Automation:
- At 1280 px, the caption buttons' pane is 40 px high from the client's top, so its centre is at 20 px (16 DIPs). The four icons, ⋯, the hide-list button, the project menu are centered at 19.5 px, within 0.5 px of it, and the title text at 19 px.
- At 800 px, ☰, ⋯ and the four icons are centered at 19.5 px, and the overlay opened by Files keeps its icons in the same place.
- Dragging the chat's header, the rail's strip and the sidebar's head before its icons moved the window. A double-click on the rail's strip maximized it, and one on the chat's header restored it. Under the buttons the bar has no room left, since the buttons fill the full height.
- In light and dark, the buttons' glyphs and the icons sit on one line. The app log has no warnings or errors.

`go build ./...`, `go vet ./...`, `go vet -tags bench ./cmd/desktop` and `go test ./...` pass. Screenshots in light and dark are kept outside the repo.
