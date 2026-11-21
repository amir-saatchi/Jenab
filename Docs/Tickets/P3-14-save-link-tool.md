# P3-14 — The `save_link` tool
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 3)
**Needs:** P3-11
**Requirements:** R-72

## Goal
The agent saves a source it used with `save_link(url, title, note, tags)`, and the link shows in the Links tab at once (SPEC 4.6, 8.1). P3-11 built the tab and the store; nothing calls the store's merge save yet.

## Scope
- **The tool** in `internal/tool`, next to `fetch_page`: it calls the store's `SaveLink` with `Merge` and the source `message:<id>`, so saving a URL that is there already updates it instead of failing. An empty title, note or tags keeps the old value.
- **Its result:** the link's ID and whether it was new or updated, short, as the other tools answer.
- **Undo:** the save is in the change log and undone with its turn; no approval card, since it can be undone.
- **Tool groups (8.6):** in the Web group, with `fetch_page`. Every model gets the same tool and description; check the tool list's size with the tool-index tests.
- **The project card:** "Saved links: N." already shows when N > 0 (P3-11).
- **Scenario:** a turn that fetches a page and saves it as a link, in an existing scenario set or a new one.

## Rules
- Only http, https and mailto URLs are saved by the agent; others are refused with a message, so the model can fix the call.
- No network call in the tool itself.

## Done when
- Tool tests: a new link, the same URL again (merge keeps the user's note), an unsafe URL refused, and undo of the turn.
- The scenario passes on the reference models with 3 reps, or the failures are recorded.
- The report lists the SPEC lines to change (8.1, 8.6).
