# TASK-055 — Settings in the native app
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** TASK-053

## Goal
Port *Settings* from `frontend/src/settings` to the native app, after SPIKE-048's go. About 2,000 lines of React.

## Scope
1. **The pages:** General, Models (with the provider list from TASK-052 and the provider's models), Memory (with TASK-050's changes), Usage, Desktop, Developer and About, in the settings layout and view from `frontend/src/shell` (`settings-layout.tsx`, `settings-view.tsx`).
2. **The usage chart** (`usage-chart.tsx`) on MyGo's `ui/chart`, through a kit component.
3. **The number input** (`number-input.tsx`) as a kit component.
4. **`state.Settings`** and its save queue already exist from SPIKE-048 Part A. Extend them where a page needs more.

## Rules
- Screens import only the kit and the stores. New components go into the kit with headless tests (light and dark, keyboard, label, right-to-left text).
- Port the frontend's tests for these pages to Go as the spec.
- The React pages get bug fixes only from now on (TASK-053).

## Done when
- Every settings page works in the native app, as in the Wails app.
- Headless tests for each page; screenshots of each page in light and dark, kept outside the repo.
- A live check: connect a provider and pick a model.

## Result
