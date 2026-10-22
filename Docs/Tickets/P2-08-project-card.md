# P2-08 — Project card
**Type:** Feature
**Status:** Done
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
Done on 2026-10-22 (commit 0d1238f), after two review rounds.

- **Built:**
  - `ProjectDB.Card` in `internal/store/card.go`, read in one transaction. Each table gets one line: rows, columns with types, NOT NULL, `object_ref`, the key, unique constraints, foreign keys, and the min and max of date columns. Then come the bucket's top-level prefixes with counts, and the header with the project name and "as of" with the UTC offset.
  - A date column is a TEXT, untyped or DATE/TIME column whose min and max are both dates. Tables over 100k rows read only columns that lead an index.
  - At 2,000 tokens or more (bytes/4, like `provider.EstimateTokens`) the card lists table names only and says to use `describe_table`. The bucket line has its own budget, and the project name is cut to 100 runes.
  - `agent.Deps.Card` takes the open project, and the desktop app wires in `p.DB.Card`.
  - The runner keeps the schema version the card was built at. Before each request it reads the version and rebuilds the card when it changed, after a migration by this chat or another. Earlier traces keep the old card, and the cache point stays on the last block.
  - The scenario runner keeps the fixture's card. A set with `project_card: true` gets the real one.
- **Tests:** an empty project, the Bitcoin tables, a project over the cap, a Persian name with ZWNJ, long names and prefixes, a NULL first date, a table of 1,200 columns; the card rebuilt right after a migration and not after an insert, a migration in another chat, a card at a cut, and a failing card.
- **Scenarios:** the new `project-card` set on gemma4:31b, 3 runs each: C1 (make a table, then query with it) 3/3 and C2 (make a table, then insert) 3/3, with no `describe_table`. Gemini wasn't run.
- **Decided here:**
  - **The card is best effort.** If it fails, the turn goes on with the old card, or none at a cut. It is tried again once in the next turn, and the error is logged once per project until a card works again.
- **Changes from the ticket:**
  - Saved links are counted only once their table exists, so today the line never shows.
  - The table lines also list unique constraints, foreign keys and NOT NULL.
  - The header has the project name, which SPEC 3.2 doesn't list.
  - The turn inspector needed no change: the card was already its own block with its size.
- **Open points:**
  - Each rebuild counts every table's rows with `count(*)`. That isn't timed on large projects.
  - The scenarios can't tell the rebuilt card from the schema agent's result, which already lists the tables. `TestCardAfterMigration` is the proof.
