# P1-14 — Frontend base and app shell
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 1)
**Needs:** P1-13
**Requirements:** R-115, R-118, R-121, N-07, N-52, N-53

## Goal
The real frontend, starting from the high-fidelity mockups (TASK-002, SPEC 5.11, 5.12).

## Scope
- **Carry-over:** decide what moves from `mockups/src` (CODE-OUTLINE open point 2): shadcn/ui on Radix, Tailwind, the fonts including Vazirmatn.
- **State:** Zustand stores fed by snapshots and events.
- **Shell (5.12):**
  - the chat list with Mother pinned and its gradient
  - the rail with projects, *Waiting*, schedules and Settings; the sidebar that hides while a page is open; overlays below 900 px
  - project settings from the project's menu
  - the bottom bar, and *Ctrl+Tab* and back and forward
  - Settings as a full view
- **First run (3.9):**
  - *Create project*, which opens the Mother chat
  - the data folder banner from 2.1
  - *Open the example* waits for Phases 3 and 4
- **Theme:** Light, Dark and System (5.11).
- **Errors:** shown with *Copy details* (`UIError`).

## Done when
- The shell matches the mockups in both themes.
- Text passes the contrast check (N-53).
- On the built app, cold start is under 1.5 s (N-07) and idle memory under 300 MB (N-52).

## Result
- **Carry-over (CODE-OUTLINE open point 2):** the shadcn components (radix-nova), `index.css` without `typeset.css`, `components.json`, the fonts (Geist, Geist Mono, Vazirmatn) and the settings layout. The screens and fake data stay in `mockups/`.
- **Packages,** pinned: Tailwind v4 with its Vite plugin, tw-animate-css, radix-ui, class-variance-authority, cn, lucide-react, sonner, the fontsource fonts, zustand; shadcn as a dev dependency.
- **State:** Zustand stores in `frontend/src/state` (nav, projects, chats, settings, ui), loaded by service calls and kept up to date by `chat:status` and `project:notice`.
- **Go additions:**
  - `Orchestrator.State` and `Orchestrator.Waits`; `ChatItem.State`; `ChatService.Waiting` for *Waiting* in the rail.
  - `internal/sysmem` and `SystemService.Memory` for the bottom bar.
  - The window background comes from the saved theme, so the first paint has no flash; System reads Windows' setting.
- **Shell (5.12):**
  - The rail: project tiles, *New project*, Search (disabled until it has a UI), *Waiting* with its count, Schedules (disabled), Settings. With the chat list hidden, the rail also shows the chats.
  - The chat list: Mother pinned with its gradient, then the newest; *Working…* and *Waiting for you* on each row.
  - The right sidebar: Pages, Pipelines, Files (from the bucket) and Links.
  - The bottom bar: working and waiting counts, provider pauses, the chat's model, free memory. Polled every 2 s while the window is visible.
  - Below 900 px the rail and both sidebars are sheets, and the bottom bar is hidden.
  - *Ctrl+Tab* through the chats used last; Alt+← and Alt+→ (⌘[ and ⌘] on macOS) for back and forward.
  - Settings as a full view (General, Appearance) and project settings from the project's menu (General, Approvals).
- **First run (3.9):** the welcome screen; *Create project* opens the Mother chat; the data folder banner; *Open the example* is disabled with a tooltip.
- **Theme:** Light, Dark and System; applied at once, saved, and kept in `localStorage` for the first paint.
- **Errors:** a toast with *Copy details*, from the UIError in the error's cause.
- **Done-when checks:**
  - Compared with the mockups in a browser against the real Go side (server mode, scratch data): screens 01 and 08 in light, 01 in dark.
  - Contrast (N-53): `contrast.test.ts` checks every text pair and the tones in both themes, from the tokens in `index.css`. All pass.
  - Cold start (N-07), `bin/jenab.exe` from process start to the first paint of the shell, on Windows 11: 0.80–1.07 s on repeat starts, 1.10 s on the very first start with an empty data folder. With a project to open, 1.02–1.12 s, and 1.63 s once, when the project first had to recover from a killed session.
  - Idle memory (N-52), 15 s after the start, private bytes over the app and its 6 WebView2 processes: 245–274 MB. Working set is 431–464 MB because it counts shared pages once per process (as in SPIKE-022).
- **Tests:** `bun test` (the stores, the formatting, the shortcuts, the contrast check) and Go tests for State, Waits, Waiting, Memory and the window theme. CI now runs `bun test`. DEVELOPMENT.md has the server mode for UI checks.
- **Not done:**
  - Saving each chat's UI state (`chats.ui_state`), commands and pipelines in the bottom bar, the context fill.
  - The Windows frame colour doesn't follow a theme change until the next start (Wails can't change it at run time).
  - The Search UI, schedules, the chat drawer and the activity dot.
  - Hiding the sidebar on its own when a page opens: only the toggle exists, as there are no pages yet.
  - Workspace, storage, hosts and Export in project settings; the Settings pages after General and Appearance (P1-16).
  - Opening a project that needs recovery can take the first paint past 1.5 s, as the shell waits for the project. Showing the shell first is left for P1-18, if the acceptance run needs it.
