# TASK-014 — The agent offers features the app doesn't have
**Type:** Bug
**Status:** Open
**Gate:** 3
**Needs:** TASK-011

## Goal
The agent offers and tries only what its tools can do. When no tool can do what the user asks, it says so plainly. Found while testing the app on 2026-10-13.

## Today
- `base.md` says a project has "tables in a local database, pipelines that fetch data, and views" (`internal/agent/prompts/base.md:1`). Phase 1 has no tools for any of them: only `fetch_page`, the bucket tools, history search, `list_chats`, `load_skill` and `ask_user`.
- A test chat (Mother, gemini-3.5-flash-lite) went like this:
  - Its greeting offered to "set up tables, fetch data via pipelines, or build views".
  - Asked to build them, it saved `schemas/users.sql` and `pipelines/fetch_users.py` with `bucket_put`. The script writes to a `project.db` in whatever folder it runs from.
  - It then made up a "Tables section" and a "Views section", and told the user to run Python in a terminal.
- Nothing tells the model what to do when no tool fits.
- TASK-011 made the background rule and the delegation guidance follow the tools, but left `base.md` as it was: rewording it for every set cut gemma's L02 from 12/12 to 10/21.

## Scope
1. **Block 1 follows the tools.** `base.md` names tables, pipelines and views only when tools for them are registered. A tool says what it belongs to (for example a field in `tool.Spec`); the prompt builder doesn't list tool names. The scenario sets have fake tools for all three, so their prompt stays as it is.
2. **When no tool fits.** One line, the same for every model: when no tool can do what the user asks, say so and say what you can do instead. Don't describe screens, menus or steps you can't see.
3. **A scenario set with Phase 1's tools only.** It asks for a table, a pipeline and a view, plus a plain question. Checks:
   - No `bucket_put` of SQL or scripts posing as the setup.
   - The answer says the app can't do it yet.
   - The answer names no app sections or menus.

## Not in scope
- The table, pipeline and view tools themselves (Phase 2).

## Done when
- A test in `agent` checks block 1 with and without the table, pipeline and view tools.
- The new set passes on both reference models, with 3 reps.
- skill-loading and multi-part don't get worse (`-baseline`).
- `go test ./...` passes.
