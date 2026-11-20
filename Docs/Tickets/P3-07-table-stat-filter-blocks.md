# P3-07 — Kit blocks: table, stat and filters
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 3)
**Needs:** –
**Requirements:** R-40, R-42, R-44, R-113, R-114, R-116

## Goal
The kit components for the table and `stat` blocks and the filter controls, built from the kit with headless tests (SPEC 5.2, 5.3, 5.8, 5.10, 5.11). They take plain Go data in the kit's own types, so this needs no other Phase 3 ticket; P3-09 maps view configs onto them.

## Scope
- **Table:** a virtual list (`kit.List`) with sortable column headers, pagination with the total or "many rows", each format (5.3) including `badge` tones, `url` as a safe link (`kit.SafeLink`), `image` as a thumbnail and `file` as a link that opens the file view, a checkbox column for `bulk` actions, and row action buttons. Each cell takes the direction of its first strong character (5.8). Cut cells have a tooltip. The existing `kit.Table` and `kit.DataTable` are a start, not a limit.
- **Stat:** the value, the change in green or red, the caption.
- **Filters:** `text`, `number`, `date` (a date picker, with the relative default shown as a date), `select` and `checkbox`, with `required`.
- **States:** loading, empty, an error on the block (a failed query, a view that no longer validates), and the "10,000 rows" style notices.
- **Theme:** colours only from the tokens and tones (5.11); no colour names in configs or screens.

## Rules
- Only the kit imports MyGo's `ui` (5.8). Every component has a headless test: light and dark, reached by the keyboard, found by its accessible label, with right-to-left text.

## Done when
- Headless tests for each component and state, including a 10,000-row table that scrolls by keyboard and a Persian table whose columns run right to left.
- Screenshots in light and dark, kept outside the repo.
