# P3-09 — The page screen
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 3)
**Needs:** P3-05, P3-06, P3-07, P3-08
**Requirements:** R-45, R-47, R-110, R-111, R-112

## Goal
A page draws its header and its rows of blocks on a 12-column grid, each block loading on its own, with the block types looked up in the registry (SPEC 5.9, 5.10).

## Scope
- **Header:** title, description, page filters and actions (`run_pipeline`, `open_page`, `open_form`).
- **Rows:** 1–4 blocks a row, spans with the default of 12 divided by the count, `card` with its own rows, and the page blocks `heading`, `text` (Markdown as in the chat, no raw HTML, links by 4.5's rules), `filter` (a page filter shown in place), `button` (with `tone` and `confirm`) and `image` (a bucket key, read through the services as the file view does; SPEC 5.9's "`<img>`" is from the WebView and Main-Session updates it).
- **The registry:** the screen draws each block by looking up its unit (P3-02), so a new type needs no change here (R-47).
- **Filters:** a page filter binds to the same parameter in every block that has it and hides the block's own filter; changing one re-runs only the blocks that use it.
- **Loading:** each block shows its own loading, error and empty state; a slow block doesn't hold the others (5.7).
- **Live:** a page changed by another chat refreshes; a deleted view shows an error on its block.
- **Narrow windows:** below about 900 px, rows stack into one column.
- **Store:** a `Pages` store in `internal/native/state` (plain Go, UI thread only, tested without a window) holds the open pages, their filter values and their blocks' results.

## Done when
- Headless tests: the Bitcoin page from fixtures, every page block, a card in a row, filter binding and hiding, one slow block, a live change, the narrow layout.
- Store tests without a window for filter changes, cancellation and a live change.
