# P3-03 — Agent tools for views and pages
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 3)
**Needs:** P3-01, P3-02
**Requirements:** R-53, R-49

## Goal
The agent builds and changes views and pages with tools, fixes failed saves in the repair loop, and opens a page in its chat (SPEC 8.1, 10, 5.9).

## Scope
- **Tools (8.1):** `save_view(config)`, `save_page(config)`, `validate(config)` (no save), `read_config(id)` (the stored YAML, for editing), `open_page(id, filters)`. Their `Areas` put them in block 1 like the data tools. `open_page` here records the request; the shell shows the page (P3-10).
- **Deleting (decided 2026-11-20):** `delete_config(id)` deletes a view or a page; pipelines join in Phase 4.
  - It refuses a view that a page or a row action's `open_form` still uses, and names them, so the agent takes it off those first.
  - The delete is in the change log (op `delete`) and is undone with its turn like any other change. It needs no approval card, since it can be undone.
  - An open page that is deleted closes in every chat (5.9).
- **Repair loop (10):** at most 4 more attempts per config in a turn, then stop and show the user the remaining errors. The early-stop notice when a turn ends right after a failed save that wasn't retried, at most once a turn. The same for every model.
- **Project card (3.2):** Views (ID, title, type) and Pages (ID, title, the views they use), and per table the views and forms that use it. The card rebuilds on a config change.
- **The `config-guide` skill** (`internal/skill/builtin/config-guide`, 8.9): views, filters, formats, actions, row actions, page rows and blocks, with examples, checked by a test that validates every example in it. A short section on removing: taking a view off a page doesn't delete it; delete only when the user asked to remove it and the card shows nothing else uses it; a page is deleted with its own views only if no other page uses them. `delete_config` loads the skill too (`load_with`).
- **Chat:** a saved view or page shows as a chip in the chat with *Open* for a page, like the memory chip; undoing it follows *Undo turn*.
- **Scenarios:** `scenarios/views-and-pages`: build the Bitcoin views and page on a fixture database, change one, fix a validation error, open the page, and remove a view the user no longer wants. Run on the reference models (gemini-3.5-flash-lite and mistral-large-4) with 3 reps. Weak-model failures are recorded, not worked around.

## Done when
- Tool tests for each tool, the repair limit, the early-stop notice and the card lines, and for `delete_config`: a refused delete that names the users, a delete and its undo.
- The scenario set passes on both reference models with 3 reps, or the failures are recorded with their causes.

## Result
Done on 2026-11-24. Report: [P3-03-report](reports/P3-03-report.md).

- **Tools:** `save_view`, `save_page`, `validate`, `read_config`, `open_page` and `delete_config`, in block 1 with the data tools. Saves validate first and return every error with its line. They take the revision the model read, and check their references again in the write.
- **Repair loop:** 4 more tries per config in a turn or subagent run, then the model is told to stop. The early-stop notice comes when a turn ends after a failed save that wasn't retried.
- **Deletes:** `delete_config` refuses, in its write, a view, form or page other configs use, naming them. A delete is undone with its turn, and tables are never touched.
- **Card and chips:** the project card lists views, pages and, per table, the views and forms that use it, and is rebuilt at the next turn after a config change. Saved and deleted views and pages show as chips; a page's chip has *Open*, which docks the page (drawn from P3-10). A deleted page closes everywhere.
- **config-guide:** five tested examples and a section on removing views.
- **Scenarios:** `scenarios/views-and-pages` passed 15/15 on gemini-3.5-flash-lite and on mistral-large-4. The first version of its remove scenario led both models to drop the news table too; the message now names the view, and config-guide says removing views leaves tables and data alone.
- **Review:** two Medium and seven Low points are fixed. The reference checks run in the write, saves take a `revision`, subagents get the repair limit, and `open_page` checks its values.
