# TASK-055 — Settings in the native app
**Type:** Task
**Status:** Done
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
Done. The native app has Settings, with the pages of the Wails app; the report is [TASK-055-report.md](reports/TASK-055-report.md).
- **The pages:** General (data folder, approval level for new projects, other apps' windows, About), Memory, Appearance, Models (providers, their models, *Add provider*, the default and fast model, the limits), Usage, Desktop (Windows only) and Developer, in a page list beside one scrolling column, as in `settings-layout.tsx`. The open project's settings are in the list too, as in `settings-view.tsx`.
- **The kit:** the usage chart (`kit.BarChart` on `ui/chart`) and the number input (`kit.NumberInput`), plus Switch, Segmented, Meter, Table, Link and the settings layout pieces, each in its own file with headless tests in light and dark, for the keyboard, the label and right-to-left text.
- **The stores:** `state.Pages` reads and writes what the pages need beyond `state.Settings`: usage, desktop items, presets, a provider's models, the user's memory, the folder check and the version.
- **Tests:** the frontend's tests for these pages are ported (`format`, `provider-choice`, `memory`, `settings-view`), and every page has headless tests in `internal/native/settings`.
- **Screenshots:** every page in light and dark, the models dialog and *Add provider*, kept outside the repo.
- **Live check:** in the native Settings, on the real services, *Add provider* connected Google Gemini with the user's key, kept in memory, and `gemini-3.5-flash-lite` became the default and fast model.
