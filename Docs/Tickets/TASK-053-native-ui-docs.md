# TASK-053 — The SPEC and design docs for the native UI
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** SPIKE-048

## Goal
SPIKE-048 decided to move Jenab's desktop UI from React in WebView2 (Wails v3) to MyGo (go, 2026-11-16). The SPEC and the design docs still describe a React frontend. From now on new UI work goes into the native app first, so the docs must say how it is built before the port tickets (TASK-054 to TASK-059) start.

## Scope
1. **SPEC:**
   - **2.8 App updates:** MyGo's signed updater (`mygo.Updater`) replaces Wails `pkg/updater`. Say what TASK-059 decides; until then, mark the section as moving.
   - **4.5 Links and references:** the `/objects` route served to the webview. Say what replaces it in a native window (opening a ref in the file view or the system app).
   - **5.8 Frontend rules:** rewrite for the native app: the kit and the stores, screens import only those, right-to-left text with no mirrored layout, the Publisher coalescing deltas, `markdown.Stream` while streaming.
   - **5.10 Block registry:** blocks become kit components in Go, not React components.
   - **5.11 Theme** and **5.12 App shell:** the tokens and the frame as the kit has them.
2. **Gate-2's technical design:** the UI layer, with SPIKE-048's architecture diagram.
3. **`DEVELOPMENT.md`:** building and running `cmd/desktop-native`, the headless kit tests (`ui.NewTester`), the bench's `-native` mode. Keep the Wails and Bun steps while the Wails app still ships, marked as going away with TASK-061.
4. **`CODE-OUTLINE.md`:** `internal/webui`, `internal/lifecycle`, `internal/native/{state,kit,chat,shell}`, `cmd/desktop-native`.
5. **The rule while both apps exist:** the React frontend gets bug fixes only. A new screen or feature is built in the native app.

## Rules
- Plain wording, section numbers as they are. Change only what the move changes.
- No code changes.

## Done when
- Every section above matches the code on `main`.
- The README's ticket table and SPIKE-048's result link this ticket.

## Result

Done on 2026-11-16, checked against the code on `main` (a050de7). Report: [reports/TASK-053-report.md](reports/TASK-053-report.md). No code changed and no model requests were made.

- **SPEC:**
  - **2.8:** marked as moving. MyGo's `mygo.Updater` replaces Wails `pkg/updater` in the native app, and TASK-059 rewrites the section. The native app has no updater yet.
  - **4.5:** the native app has no `/objects` route; screens read objects through the services. A link opens the file view, or *Open with the system app* for other types (TASK-056). The route stays for the Wails app.
  - **5.8:** rewritten for the native app. It covers the two apps until TASK-061 and the React bug-fixes-only rule. It also covers the kit and the stores, the screens' import rule and its test, a headless test for every component, events through the Publisher and the mailbox, `markdown.Stream` and the virtual thread, and right-to-left text with no mirrored layout. The React rules stay, under their own heading, until TASK-061.
  - **5.10:** blocks are kit components in Go.
  - **5.11:** the kit's tokens and tones, with `SetThemeSource`. The Wails way stays until TASK-061.
  - **5.12:** the kit's frame pieces, what the native shell has now, and hiding on close with the tray.
- **Gate-2:** a *Desktop UI* row (MyGo, with SPIKE-048's numbers), notes on the Wails, updater, frontend and frontend-load rows, and a *UI layer* section with SPIKE-048's architecture diagram.
- **DEVELOPMENT.md:** building and running `cmd/desktop-native`, the native UI's headless tests, and the bench's `-native` mode. Every Wails and Bun step is marked as going with TASK-061.
- **CODE-OUTLINE:** `cmd/desktop-native` and `internal/lifecycle` (section 2), `webui` in the start sketch and the object route, and section 10 as *The UI: `native` and `frontend`*, with `internal/native/{state,kit,shell,chat}`.
