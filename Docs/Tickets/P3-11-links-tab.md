# P3-11 — The Links tab
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 3)
**Needs:** P1-05
**Requirements:** R-72

## Goal
Saved links show in the right sidebar's Links tab, where the user can add, edit and delete them (SPEC 4.6, 5.12).

## Scope
- **The list:** `_jenab_links` (title, URL's host, note, tags, who saved it and when), newest first, searchable by title, host and tag, live as the agent saves more.
- **Actions:** open in the system (only http, https and mailto, by `kit.SafeLink`, 4.5), copy, add, edit and delete. A delete is a change by `user` with undo from its notice, as file deletes are.
- **Text:** titles and notes take their own direction (5.8).
- SPEC 4.6 still says the links show "in the Resources panel"; Main-Session updates it to the Links tab.

## Done when
- Headless tests for the list, search, each action, undo of a delete, a right-to-left title and an unsafe URL that doesn't open.
