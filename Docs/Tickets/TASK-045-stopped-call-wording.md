# TASK-045 — Say when a stopped call may have run
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** —

## Goal
After *Stop*, every open call gets "cancelled by user" (8.3). For a call that was already running, that can be wrong. A `desktop_act` may have reached the app before agent-desktop was killed, and later MCP calls and commands will be the same. A model that reads "cancelled" and the user's "go on" may repeat the action. Tell the model what is known.

## Scope
1. **List the tools** that can have an effect before they return: desktop actions and replays, and any write whose result is lost after *Stop*. Check what an `insert` taken by the writer returns after *Stop* (7.3 says it finishes).
2. **Three wordings:**
   - not started: "cancelled by user before it ran";
   - running, no effect outside Jenab's undo: "cancelled by user";
   - running, may have acted: "cancelled by user while it ran; it may have partly run. Check before repeating it." For a desktop step, also name the window and the step.
3. **Mark** such tools in `tool.Spec` (or reuse `Effects`), so MCP and commands get the same wording later.
4. **SPEC 8.3:** the three wordings, in the *Stop* list.

## Rules
- The same wording for every model (SPEC 1).
- Keep the result short; it stays in the window.

## Done when
- Go tests for each wording, including a desktop action stopped mid-call (with a fake runner).
- SPEC 8.3 has the rule.
