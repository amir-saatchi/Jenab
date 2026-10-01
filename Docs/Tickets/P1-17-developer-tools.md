# P1-17 — Turn inspector and runtime panel
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 1)
**Needs:** P1-15
**Requirements:** R-94

## Goal
See what the agent sent and what is running (SPEC 8.4). R-94 is a Should item.

## Scope
- **Turn inspector:**
  - the parts timeline
  - tool calls with timing and result size
  - the context blocks with token counts and cache hits
  - *Inspect* in the turn footer
- **Runtime panel:**
  - chats in a turn
  - each writer's queue and current request
  - the LLM-call slots
  - anything stuck past its limit, flagged
- **Access:** both are read-only, behind *Settings → Developer*.

## Done when
- The inspector shows the same blocks `provider/fake` received for a recorded turn.
- A test with a fake key finds no secret on either screen.
