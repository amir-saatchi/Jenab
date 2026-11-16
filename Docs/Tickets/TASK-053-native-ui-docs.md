# TASK-053 — The SPEC and design docs for the native UI
**Type:** Task
**Status:** Open
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
