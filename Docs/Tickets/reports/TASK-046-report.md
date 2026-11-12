# TASK-046 report: thinking blocks after the context changes

Date: 2026-11-12. Model for all runs: `anthropic/claude-haiku-5-5`.

## 1. Where the prefix changes

A thinking block is bound to everything before it: system blocks, tools and
earlier messages. These are the places where Jenab changes that prefix.

**Inside a turn (Jenab harness)**

| Change | Code | How often |
|---|---|---|
| Project card block rebuilt | `refreshCard` | After a migration that changes the schema version. A few times in schema runs, never in chat-only turns. |
| Tool list re-offered | `refreshTools` | When a migration adds a table that a waiting tool needs. Rare: schema runs only. |
| Current turn's tool results stubbed | `trim` | When the turn passes `turn_trim_ratio` (0.5) of the window. Long turns only. |
| Older result in a series stubbed | series stubs | Each new result in a series (for example each new desktop snapshot). |
| Last-request user message | `lastRequest` | Sent only on the final request at the round cap and not stored, so the next turn's thinking follows a message that is gone. Rare. |
| User turns a tool on or off | 8.10 | Applies at the next request. Rare. |

Not prefix changes (they only add at the end): memory notices (new user
message), history cards (stable after the turn's first request), `load_skill`
text (part of a tool result), `tool_choice: none`, cache markers.

**Between turns (Jenab harness)**

`cutIfDue` rebuilds all system blocks (date, role, skills, memory, card, chat
list, notes, "Earlier turns"), moves the window start, stubs older tool
results, re-offers tools and drops loaded skills. A cut happens when:

- the window has more than `history_max_turns` (8) turns: about every 5 turns
  after a cut back to 3;
- the window passes `history_max_tokens` (24,000);
- the cache was idle more than 5 minutes (any pause);
- it is the first request in the process (restart, Clear);
- after Refresh memory.

Session notes and memory reach the system prompt only at a cut.

**Plain harness**

`plainWindow` rebuilds block 1 when its text changes (the date each day, loaded
skills, role) and drops the oldest turns near 80% of the window.

## 2. What changed

- `internal/provider/anthropic/anthropic.go`
  - Adaptive thinking now always sends
    `"block_binding": {"prefix_mismatch_behavior": "drop_block"}` and the
    header `anthropic-beta: thinking-binding-controls-2026-08-01` (SDK
    constant `AnthropicBetaThinkingBindingControls2026_08_01`). The Guard
    allows `anthropic-beta`.
  - Budget thinking (Haiku 4.5) sends neither: block_binding works only with
    adaptive, and budget models don't check.
  - A request without a thinking setting (model not in the catalog, thinking
    off, no room for a budget) sends **no earlier thinking blocks**. Leaving
    all of them out is always valid. Such a request can't ask for drop_block,
    and a newer model thinks by default, so replaying would 400. An older
    uncatalogued model doesn't think by default, so little is lost. The
    catalog's `thinking` and `thinking_budget` decide; no names, URLs or
    error text.
  - `message_start` is read for `input_transformations`. Each
    `thinking_dropped` entry with reason `prefix_binding_mismatch`,
    `model_binding_mismatch` or `organization_binding_mismatch` counts as one
    dropped block. Other types and reasons are ignored.
- `internal/chat/chat.go`: `Usage.ThinkingDropped` (in the trace, not stored
  in the database).
- Scenario runner: `-usage-log` lines carry `thinking_dropped`; the tally adds
  "thinking blocks dropped N in M requests"; scenario results carry
  `thinking_dropped` and `thinking_drop_requests`.
- Tests: adaptive request has block_binding and the beta header (checked on
  the wire); budget and no-setting requests have neither; earlier thinking is
  sent back for adaptive and budget and left out otherwise (tool calls and
  text kept); dropped count from a mixed `input_transformations` list is 3,
  and 0 for an empty list.
- `internal/provider/models.json`: see section 5.

`go build ./...`, `go vet ./...`, `go test -p 2 -short ./...` pass; gofmt clean.

## 3. Dropped blocks per run

Counts are `thinking_dropped` entries. A stale block is counted again in every
later request that still holds it, so these are "blocks left out", not
"breaks".

| Run | Requests | Dropped entries | Requests with drops |
|---|---|---|---|
| session-notes N1 | 11 | 3 | 3 |
| schema-design T1 | 6 | 6 | 2 |
| schema-widen T4 | 4 | 2 | 1 |
| schema-widen-check T4 | 4 | 2 | 1 |
| missing-features F2 | 3 | 1 | 1 |
| project-card C1 + C2 | 8 | 3 | 3 |
| bench-bitcoin B1, jenab (chat) | 48 | 248 | 46 |
| bench-bitcoin B1, plain | 30 | 282 | 28 |
| bench-bitcoin-long L1, jenab (chat) | 68 | 347 | 66 |
| bench-bitcoin-long L1, plain | 45 | 666 | 43 |
| acceptance P1 | 42 | 42 | 25 |
| **All** | **274** | **1,602** | **219** |

The bench runs move the clock a day each turn, so the date in the system
prompt changes every turn: a cut every turn (Jenab) and a new block 1 every
turn (plain). Nearly every request then drops all earlier thinking.

## 4. Reruns: before and after

Before = HAIKU-PASS report (all 11 runs broke on the binding 400; 102 of 391
requests refused). After = this branch.

| Run | Before | After | Cost after |
|---|---|---|---|
| session-notes N1 | fail (400) | **fail** (model) | $0.0041 |
| schema-design T1 | fail (400) | pass | $0.0042 |
| schema-widen T4 | fail (400) | **fail** (model) | $0.0033 |
| schema-widen-check T4 | fail (400) | pass | $0.0034 |
| missing-features F2 | fail (400) | pass | $0.0020 |
| project-card C1 | fail (400) | pass | $0.0044 (C1+C2) |
| project-card C2 | fail (400) | pass | |
| bench-bitcoin B1 jenab | fail (400) | pass (3m12s) | $0.0632 (both) |
| bench-bitcoin B1 plain | fail (400) | pass | |
| bench-bitcoin-long L1 jenab | fail (400) | pass (4m2s) | $0.1022 (both) |
| bench-bitcoin-long L1 plain | fail (400) | pass | |
| acceptance P1 | pass, 9 of 25 turns refused | pass, none refused | $0.0102 |

No binding 400 and no other error in 274 requests. 9 of 11 pass now.

The two failures are model behaviour, not the binding:

- **N1**: on turn 6 the model wrote the answer ("imaginary city", "Mina")
  before its `update_session_notes` call; the answer after the tool was empty
  (1 output token), so `answer_contains` failed twice.
- **T4 schema-widen**: the db check expected 2 `watch_keywords` rows and found
  3. Gemini 3.5 Flash-Lite also failed this run in an earlier report.

**Cost**: $0.197 in total (cap $0.50). Input 846, cache read 2,251,787, cache
write 938,110, output 114,180 tokens. No prompt passed 100K, so the low Haiku
5.5 price tier applied throughout.

I can't say how much the drops raise output tokens: the "before" runs stopped
early, so there is no baseline without drops.

### Probe: a tool round sent back with no thinking setting

The reruns always sent adaptive thinking, so the "no thinking setting" path
was not used. Haiku 5.5 thinks by default even without a `thinking` field, so
I checked that leaving its blocks out of a tool round is accepted.

Live probe through Jenab's anthropic backend on `claude-haiku-5-5`: one
synthetic tool (`lookup_code`); a prompt that needs some reasoning first;
the first request ends in `tool_use`; the second sends the assistant turn as
it came back (thinking parts kept in the history) plus the `tool_result`.

| Case | Request 1 | Request 2 body | Result |
|---|---|---|---|
| Model not in the catalog (`Known: nil`) | no thinking field; answer had 1 thinking block (signature only, no text) | no thinking field, no thinking block: `assistant: tool_use` | answered, no 400 |
| Thinking off (`Thinking: false`) | same | same | answered, no 400 |
| Adaptive first, then `Known: nil` | adaptive; 1 summarized thinking block | no thinking field, no thinking block: `assistant: text tool_use` | answered, no 400 |

So the API does not need the thinking block before a `tool_use` when the
request has no thinking setting. No fix needed. The probe was a build-tagged
test file, not committed. Cost: about $0.0015 (12 requests in two attempts;
the first prompt was too easy and the model didn't think). Total spent on
the key for TASK-046: about $0.199 of $0.50.

Side finding: `Thinking: false` sends no field, and Haiku 5.5 then thinks
anyway (default on). Turning it really off would need an explicit setting;
I didn't add one.

## 5. Catalog

Changes in `models.json`:

- New `claude-haiku-5-5`: 1M context, 128K output, tools, images, adaptive
  thinking, $0.10 in, $0.50 out, $0.01 cache read, $0.125 cache write.
  `fast: true` moved here from Haiku 4.5.
- Sonnet 5.5 `cache_read` 0.2 → 0.10.

Checked against `GET /v1/models` (one free call) and the pricing page:

- Context and output match for Opus 5.5, Fable 5.1, Sonnet 5.5, Haiku 5.5
  (1M / 128K) and Haiku 4.5 (200K / 64K).
- Prices match for Opus 5.5 (4 / 20 / 0.20 / 5), Fable 5.1 (10 / 50 / 0.25 /
  12.5), Sonnet 5.5 (2 / 10 / 0.10 / 2.5, after the fix), Haiku 4.5 (1 / 5 /
  0.10 / 1.25).
- The API lists models not in the catalog: `claude-fable-5-2` (the current
  Fable: $8 in, $40 out, $0.20 cache read, $10 cache write),
  `claude-opus-5`, `claude-sonnet-5`, `claude-fable-5`, `claude-opus-4-8`,
  `claude-opus-4-7`, `claude-sonnet-4-6`, `claude-opus-4-6`,
  `claude-opus-4-5-20251101`.
- Haiku 5.5 has a second price tier over 100K prompt tokens ($0.50 / $2.50 /
  $0.05 / $0.625). The catalog has one tier, so long prompts are priced too low.
- The `_note` still says "Checked on 2026-10-03".

## 6. Proposed SPEC 3.8 text

Under *Message shapes*:

> Thinking blocks are sent back unchanged, signature included, before their
> tool_use, when the request has a thinking setting (below).

Added to the Anthropic thinking bullet:

> Newer Claude models (Haiku 5.5, Sonnet 5.5, Opus 5.5, Fable 5.1 and 5.2)
> refuse a thinking block whose system prompt, tools or earlier messages
> changed since it was made (400). Jenab changes them at cuts, when a
> migration rebuilds the card or the tool list (3.1, 3.6), and when it trims
> the current turn. So adaptive thinking always carries
> `block_binding.prefix_mismatch_behavior: "drop_block"` with the
> `thinking-binding-controls-2026-08-01` beta: the API leaves out the first
> stale block and every thinking block after it, for that request only, and
> goes on. block_binding works only with adaptive; budget models don't check
> and get neither. A request without a thinking setting (unknown model,
> thinking off, no room for a budget) sends no earlier thinking: leaving all
> of it out is always valid. Jenab never removes a block from the middle
> itself. `input_transformations` in `message_start` lists each drop; Jenab
> counts the `thinking_dropped` entries (prefix, model and organization
> reasons) as the request's dropped thinking blocks (in the trace and the
> scenario runner, not stored); other types and reasons are ignored. No error
> text is matched. A dropped block is lost reasoning and can cost more output
> tokens, so the prefix should change as little as possible.

## 7. Proposal: change the prefix less (not built)

- Keep system blocks fixed between cuts. Send card and tool-list changes as a
  new notice message at the end, like memory notices.
- Keep the full tool list from the cut; a tool that needs a missing table
  says so when called, instead of being re-offered later.
- For a cut caused only by idle time, keep the window start and system blocks
  when nothing in them changed. When a cut does happen, leave out the thinking
  of turns before the cut (a model-neutral request field or a shape rule), so
  stale blocks are not sent and dropped again in every request.
- Plain harness: send the date change as a message, not a new block 1.
- Count breaks, not just entries: record the first dropped block's path per
  request, so a repeat isn't counted twice.

This is big enough for its own ticket.

## 8. Open points

- N1 and T4 widen fail on model behaviour (section 4).
- Output-token cost of the drops is unmeasured (no baseline).
- `ThinkingDropped` is not stored in the database, only in the trace and
  runner output.
- A `message_delta` that might carry later transformations is not read; only
  `message_start` is.
- Thinking off doesn't stop Haiku 5.5 from thinking (probe above).
- Catalog: add Fable 5.2? Two price tiers for Haiku 5.5? Update the `_note`
  date?
