# P1-14 — Frontend base and app shell
**Type:** Feature
**Status:** Open
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
  - the folding sidebar, and overlays below 900 px
  - the project switcher and project settings
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
