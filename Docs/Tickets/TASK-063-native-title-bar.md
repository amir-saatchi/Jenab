# TASK-063 — The native app's own title bar
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** TASK-061

## Goal
The native app shows the plain Windows title bar above its own headers, which wastes a row. The user chose (2026-11-17) to hide it and let the app's top row take its place, with Windows' own minimize, maximize and close buttons kept. The right sidebar's tabs (*Pages*, *Pipelines*, *Files*, *Links*) move out of the sidebar into that row as icons beside the window buttons. Their text tabs are cut off today ("Pipeli…").

## Scope
1. **Hidden title bar, system buttons kept:** the window opens with `TitleBarStyle: mygo.TitleBarHidden` (MyGo's [native UI windows](https://mygo.run/docs/ui/windows)). Keep the system buttons; don't use `HideWindowButtons`. The top row of each column (the chat list's project header, the chat's header, the right sidebar) lines up as one bar, at least as tall as `c.TitleBar().Height`. It leaves `c.TitleBar().Right` free for the system buttons, and `Left` on macOS.
2. **Dragging:** the bar's empty parts move the window with `DragWindow()`, and a double-click maximizes, as a title bar does. Check first whether buttons inside a `DragWindow` element still get clicks. If they don't, make only the empty parts draggable.
3. **The window buttons' theme:** `SetWindowButtonsTheme` follows the app's light or dark theme, and changes with it.
4. **The right sidebar's tabs as icons:** four icon buttons at the end of the bar, just before the system buttons, with tooltips and screen-reader names *Pages*, *Pipelines*, *Files* and *Links*. Use Lucide icons, as the kit does.
   - A click opens the right sidebar on that tab. A click on the open tab's icon hides the sidebar. The open tab's icon shows as selected.
   - The sidebar loses its tab row. Its content starts at the top, under a small title naming the tab.
   - Whether the sidebar is shown, and its tab, are kept as `state.Shell` keeps them now.
   - A `jenab://` link still opens *Files* (`state.OpenFile`), and shows the sidebar if it was hidden.
   - Check SPEC 5.12 and `right-sidebar.tsx` for hiding the right sidebar, and follow them if they say more.
5. **Narrow layout (under 900 px, TASK-062):** the icons stay in the bar and open the right overlay on their tab, in place of today's *Pages* button in the chat header. The ☰ button stays at the start.
6. **Other screens:** Settings, project settings, the developer tools and the file view use the same bar, so the window can be dragged and the buttons never cover content.
7. **Docs:** SPEC 5.12 and CODE-OUTLINE section 10.

## Rules
- Screens import only the kit and the stores. The bar and the drag region are kit pieces, with headless tests in light and dark for the keyboard, the labels and right-to-left text.
- The tab icons are ordinary kit buttons: focus ring, keyboard, tooltip and name, as everywhere else. Check them with Narrator.
- Windows 11 first. macOS and Linux (TASK-060) need only to keep working: the system bar there may stay until TASK-060.
- Live checks use a scratch profile and no real system dialogs.

## Done when
- The app has no system title bar on Windows. Its own top row drags, double-click maximizes, and Windows' buttons work, including snap layouts on maximize.
- The four icons open, switch and hide the right sidebar, at 1280 and at 800 px wide.
- Headless tests and screenshots in light and dark; a live check with the bench build, and a Narrator check of the icons.
- `go build ./...`, `go vet ./...` and `go test ./...` pass.

## Result
