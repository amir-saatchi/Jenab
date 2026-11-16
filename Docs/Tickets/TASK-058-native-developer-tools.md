# TASK-058 — Developer tools in the native app
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** TASK-053

## Goal
Port the developer tools (SPEC 8.4) from `frontend/src/dev` to the native app, after SPIKE-048's go: the turn inspector and *Runtime*. About 900 lines of React.

## Scope
1. **The turn inspector** (`inspector.tsx`): each request and answer of a turn, the tool calls and the token counts.
2. ***Runtime*** (`runtime.tsx`).
3. **The dev page** (`dev-page.tsx`) and its formatting helpers (`format.ts`).

## Rules
- Screens import only the kit and the stores. Long request bodies use the kit's virtual list, so a large turn stays smooth.
- Port the frontend's tests for these parts to Go.

## Done when
- Both tools work as in the Wails app, with headless tests and screenshots in light and dark.
- A live check on a turn with tool calls, using the fake model.

## Result
Done. The turn inspector and *Runtime* are in `internal/native/dev`, on the store `state.Dev`, with `format.ts` ported and its tests. They open from the rail's *Developer tools* while the tools are on, and *Inspect* in a turn's footer opens the inspector at that turn. Request bodies open in the kit's new `LinesView`, a virtual list of lines. New kit pieces, each with a headless test: `DataTable`, `Picker`, `TimeAxis` and `Lane`, `TitledCard`, `ShareBar`, `LinesView`, `LinkButton`. Headless tests in light and dark cover both tools, the rail and *Inspect*; `go vet ./...` and `go test ./...` pass. The live check used the bench's fake model with the new `-dev-tools`, which makes a message with "use a tool" call `query` twice: the inspector showed the turn's two requests, its tool calls and context blocks, and *Runtime* showed a running turn. No model requests, no downloads. Report: [reports/TASK-058-report.md](reports/TASK-058-report.md).
