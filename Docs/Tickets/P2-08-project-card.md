# P2-08 — Project card
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 2)
**Needs:** P2-04
**Requirements:** R-20, R-23

## Goal
The app builds the project card from the project's state, as block 4 (SPEC 3.1, 3.2).

## Scope
- **`ProjectDB.Card`:** tables with columns, types, the primary key, object-reference columns, row counts and min and max of date columns; the bucket's top-level prefixes with counts and the number of saved links. Views, pages, pipelines, connections, MCP servers and the workspace join in their phases; the format leaves room for them.
- **"As of":** counts carry the time the card was built.
- **Size:** under 2,000 tokens. A larger project lists table names only, and the agent uses `describe_table`.
- **When it changes:** at a cut, like blocks 2–5, and at once after a migration: the one exception to "only grows at the end" (3.1).
- **Wiring:** `agent.Deps.Card` gets the real function; the scenario runner keeps its fixture's card.
- **Turn inspector:** the card is its own block with its size.

## Done when
- Tests: an empty project, the Bitcoin tables, a project over the size cap, a card rebuilt right after a migration and not after a plain insert.
- A scenario: after a migration in the same turn, the model uses the new table without calling `describe_table`.

## Result
