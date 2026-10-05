# P1-17 — Turn inspector and runtime panel
**Type:** Feature
**Status:** Done
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

## Result
- **Recording:** `agent.Traces` keeps what each turn sent and got back: one record per try of a request, and one per tool call.
  - It runs only when the developer tools were on at the start, and lives in memory only, so a restart loses it.
  - It keeps the last 100 turns and 32 MiB of request text. Past that, the oldest requests lose their texts; their numbers stay.
- **Turn inspector,** from *Inspect* in the turn footer or *Settings → Developer*:
  - Pickers for the chat and the turn. While the turn runs, it reads again every second.
  - A timeline with lanes for Model, Tools and Parts. The Background lane comes with runs.
  - The requests, with the provider's token counts. The tool calls, with time, result size and ref.
  - The chosen request's context blocks, with estimated tokens, cache points and cache hits. A block opens its text.
  - Cache hits are worked out: the estimates are scaled to the provider's prompt count and compared with its cache read.
- **Runtime panel:**
  - The LLM-call slots and each provider.
  - Per open project: its work and the two database writers.
  - *Stuck* when work has not moved within its limit, or a write has run over 30 s. The limit is the provider's first-event limit during a request and the tool's timeout during a tool call.
  - It reads again every second.
- **Go additions:**
  - `project.Manager.Activities`.
  - `Status.Title` and `Status.Limit`, and `Activity.Name`.
  - `provider.Registry.Calls` and `FirstEvent`.
  - `Block.Name` (not sent).
- **Done-when checks:**
  - `TestDevScreensShowWhatWasSentWithoutSecrets` covers both:
    - Every block's text equals what `provider/fake` received, redacted.
    - With the fake key in the user's message, a provider error and a streaming answer, the JSON of every screen has no key.
  - `TestTracesRecordWhatWasSent` compares the record with the fake's calls.
  - The screens were checked in server mode against a local fake provider, during a turn and after it.
- **Not done:**
  - Token counts per block are estimates (4 bytes a token).
  - Writers show their queue, whether they are busy and for how long, but not what the current request is.
  - There is no run gate yet, so the panel shows only the LLM-call slots.
