# TASK-075 — Heading links in repeated text, and a cut number cell's tooltip
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-064, TASK-069

## Goal
Findings from the review of TASK-064 to TASK-070 (2026-11-17), in the kit. Both Low.

## Scope
1. **A `#heading` click can scroll the wrong message** (TASK-064). When two messages hold the same text, a heading link in one can scroll the other. The link scrolls only within the view it was clicked in.
2. **A cut number cell may get no tooltip** (TASK-069). `kit.Table` gives every cut cell a tooltip, but a number cell can miss it. Every cut cell has one, numbers included.

## Done when
- A test for each.
- `go build ./...`, `go vet ./...` and `go test ./...` pass.

## Result
Done.
1. `Markdown.ViewAs` and `StreamAs` take a key that names the view; a clicked `#heading` goes only to the view with that key. The chat passes each part's row key; `View` and `Stream` still match by text, for one view per `Markdown`.
2. `kit.Table` measures a numeric cell in tabular figures, as it is drawn, so a cut number gets its tooltip.

A test for each; both fail on the old code. Report: [reports/TASK-075-report.md](reports/TASK-075-report.md).
