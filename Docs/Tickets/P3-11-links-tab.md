# P3-11 — The Links tab
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 3)
**Needs:** P1-05
**Requirements:** R-72

## Goal
Saved links show in the right sidebar's Links tab, where the user can add, edit and delete them (SPEC 4.6, 5.12).

## Scope
- **The list:** `_jenab_links` (title, URL's host, note, tags, who saved it and when), newest first, searchable by title, host and tag, live as the agent saves more.
- **Actions:** open in the system (only http, https and mailto, by `kit.SafeLink`, 4.5), copy, add, edit and delete. A delete is a change by `user` with undo from its notice, as file deletes are.
- **Text:** titles and notes take their own direction (5.8).

## Done when
- Headless tests for the list, search, each action, undo of a delete, a right-to-left title and an unsafe URL that doesn't open.

## Result
Done on 2026-11-21. Report: [P3-11-report](reports/P3-11-report.md).

- **Store:** `project.db` format step 7 adds `_jenab_links`. Saves and deletes go through the writer and the change log, with undo, redo and conflicts. A URL is stored normalized, and a merge save keeps the fields it leaves empty, ready for `save_link`.
- **Tab:** the right sidebar's Links tab lists the links newest first, with search that folds Persian spellings. Its row menu has *Open* (only through SafeLink), *Copy link*, *Edit* and *Delete* with *Undo*. An add and edit dialog closes with its tab or project.
- **Events:** `project:links` keeps the tab live. `app.LinkService` serves the screen.
- **Not done:** the agent's `save_link` tool was never built; it is [P3-14](P3-14-save-link-tool.md).
- **Review:** two Medium and eight Low points are fixed. The dialog no longer outlives its tab, and undo conflicts name the link. Tags refuse commas, URLs are normalized, and direction characters are refused or isolated.
