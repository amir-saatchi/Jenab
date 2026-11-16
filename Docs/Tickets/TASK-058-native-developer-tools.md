# TASK-058 — Developer tools in the native app
**Type:** Task
**Status:** Open
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
