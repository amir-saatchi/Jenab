# P3-13 — Phase 3 acceptance
**Type:** Task
**Status:** Open
**Gate:** 3 (Phase 3)
**Needs:** P3-01 to P3-12, P3-14
**Requirements:** R-40 to R-49, R-53, R-72, R-110 to R-114

## Goal
Check Phase 3's "done when" in the built app (PROPOSAL 9): the Bitcoin page opens without any LLM involvement, and a page with six blocks paints in under 1.5 s.

## Scope
- **Scenario, as P2-14 did for Phase 2,** on the reference models, starting from the Phase 2 Bitcoin project (tables and some days of data):
  1. The user asks for a page with the price, a chart, today's price and the news. The agent saves the views and the page (some failed saves fixed in the repair loop) and opens it.
  2. The user adds a keyword with the form and deletes one with a row action.
  3. The app restarts. The page opens from the Pages tab with its filter value, with no provider call and no network request.
  4. *Undo turn* on the turn that saved the page reverts it.
  5. A migration renames a column a view uses; the view keeps working.
- **"Refresh now"** (`run_pipeline`) waits for Phase 4, so the page leaves it out; P4's acceptance adds it.
- **Numbers:** P3-12's six-block page.
- **Built app:** the same steps by hand in the native build on a scratch data folder, with screenshots of the page, the form, a row action's undo and the narrow layout, kept outside the repo.

## Done when
- The scenario passes on the reference models with 3 reps, and the manual run in the built app is recorded.
