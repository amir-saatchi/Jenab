# Claude Haiku 5.5: one pass of the scenario suite

**Date:** 2026-11-12, 10:02–10:16 UTC.
**Model:** `anthropic/claude-haiku-5-5` through Jenab's native `anthropic` provider, paid key `ANTHROPIC_API_KEY`.
**Code:** this branch (main at 4451143 plus the runner's `-usage-log`, below). Nothing in prompts, skills or the provider was changed.

## Summary

- **66 of 82 runs passed (80 %). Real cost: $0.109**, against an estimate of $0.49.
- **11 runs failed because of one Jenab bug.** Anthropic refused 102 requests with a 400: a thinking block Jenab sent back was "bound to a different conversation". Jenab changes the system prompt, the tools or earlier messages, then replays the old thinking blocks. Haiku 5.5 checks for this (below). Without those 11 runs, 66 of 71 passed (93 %).
- The other 5 failures: 2 the check, 2 shared with both reference models, 1 the number check on a worked example.
- **A Sonnet 5.5 pass would cost about $2.0 with these tokens, about $3.0 for a pass without the bug, and up to about $3.6 if Sonnet writes more.** It would hit the same bug, so fix it first.

## Setup

- `scenarios/models.yaml` (git-ignored, not committed): provider `anthropic`, kind `anthropic`, `key_env: ANTHROPIC_API_KEY`, `paid: true`, model `{id: claude-haiku-5-5, context: 1000000}`.
- Each set ran as `jenab-scenarios run -env <.env> -model anthropic/claude-haiku-5-5 -usage-log <file> [flags] <set>`, reps 1, one run at a time. acceptance had `-timeout 40m`, refresh-memory `-scenario R1_garden_chat`, the two bench sets `-harness jenab,plain`. local-refresh ran with JSON mode on (the default).
- After each set, a small script added up the usage log and the cost. It would stop before a set at $1.50 and on any rate-limit or quota error. Neither happened. There was no 402 and no 429.
- One mistake of mine: the first driver put `-timeout` after the set folder, so acceptance didn't start ("no _set.yaml"). No request was sent. I fixed the order and went on.

## Prices used

From Anthropic's pricing page (read on 2026-11-12), the same as Main-Session's figures:

| Model | Input | Cache write (5 min) | Cache read | Output |
|---|---|---|---|---|
| Haiku 5.5, prompt up to 100K | $0.10 | $0.125 | $0.01 | $0.50 |
| Haiku 5.5, prompt over 100K | $0.50 | $0.625 | $0.05 | $2.50 |
| Sonnet 5.5 | $2.00 | $2.50 | $0.10 | $10.00 |

Per million tokens. Haiku is priced per request by its prompt (input + cache read + cache write). No request came near 100K: the largest prompt was 21,744 tokens. So every request was at the low price. Jenab's cache marker is the 5-minute kind.

## Results by set

Requests are every request the provider got, including subagents', titles and Refresh memory, from the new usage log. "Refused" requests got the 400 and cost nothing. Prompt = input + cache read + cache write. Time is run time.

| Set | Passed | Requests | Prompt | Cache read | Cache write | Output | Time | Cost |
|---|---|---|---|---|---|---|---|---|
| grounding | 15/16 | 44 | 195,619 | 180,543 | 14,950 | 13,380 | 118 s | $0.0104 |
| multi-part | 8/9 | 17 | 96,428 | 74,000 | 22,370 | 4,023 | 77 s | $0.0056 |
| role-skills | 4/6 | 12 | 89,623 | 42,728 | 46,859 | 4,669 | 34 s | $0.0086 |
| acceptance | 1/1 | 38 (9 refused) | 174,674 | 151,708 | 22,876 | 5,375 | 54 s | $0.0071 |
| session-notes | 0/1 | 10 (2 refused) | 40,677 | 34,670 | 5,983 | 2,018 | 15 s | $0.0021 |
| memory-untrusted | 2/2 | 8 | 35,158 | 29,427 | 5,709 | 1,404 | 14 s | $0.0017 |
| refresh-memory R1 | 1/1 | 10 | 21,905 | 9,336 | 12,529 | 5,295 | 16 s | $0.0043 |
| local-refresh | 1/1 | 3 | 2,985 | 0 | 2,973 | 2,379 | 12 s | $0.0016 |
| schema-design | 0/1 | 5 (1 refused) | 19,478 | 8,846 | 10,618 | 2,187 | 12 s | $0.0025 |
| schema-widen | 0/1 | 4 (1 refused) | 17,259 | 9,260 | 7,989 | 2,399 | 12 s | $0.0023 |
| schema-widen-check | 0/1 | 3 (1 refused) | 9,812 | 4,554 | 5,250 | 1,860 | 9 s | $0.0016 |
| bench-bitcoin (jenab + plain) | 0/2 | 55 (31 refused) | 218,382 | 172,138 | 46,174 | 10,137 | 68 s | $0.0126 |
| bench-bitcoin-long (jenab + plain) | 0/2 | 86 (54 refused) | 284,500 | 208,760 | 75,652 | 19,394 | 114 s | $0.0212 |
| local-basic | 3/3 | 7 | 32,775 | 27,100 | 5,655 | 802 | 11 s | $0.0014 |
| skill-loading | 11/12 | 31 | 193,252 | 166,720 | 26,446 | 6,703 | 56 s | $0.0083 |
| data-tools | 4/4 | 11 | 50,836 | 44,085 | 6,721 | 1,453 | 13 s | $0.0020 |
| missing-features | 5/6 | 12 (1 refused) | 47,478 | 36,747 | 10,695 | 2,826 | 21 s | $0.0031 |
| memory | 2/2 | 6 | 24,748 | 11,383 | 13,345 | 1,780 | 13 s | $0.0027 |
| project-card | 0/2 | 7 (2 refused) | 23,999 | 18,236 | 5,745 | 1,743 | 11 s | $0.0018 |
| grounding-honest | 9/9 | 22 | 107,423 | 93,948 | 13,413 | 10,860 | 58 s | $0.0081 |
| **All 20 sets** | **66/82** | **391 (102 refused)** | **1,687,011** | **1,324,189** | **361,952** | **100,687** | **736 s** | **$0.1089** |

Uncached input was only 870 tokens in all: every prompt was either read from the cache (78 %) or written to it (21 %).

**Thinking tokens:** not known. Anthropic's usage has no separate thinking count; thinking is inside the output tokens. Jenab sends no `thinking` field for this model (see models.json below), so the API's default display is "omitted" and the thinking text comes back empty: not even its length can be counted. Haiku 5.5 at its default effort (medium) seems to think little: "What is 2+2?" took 3 output tokens, and output was about 1,230 tokens a run.

## Cost: $0.109 against the $0.49 estimate

1. **The bug cut the biggest sets short.** The bench sets are the largest in the suite. 85 of their 141 requests were refused, so they used about 500K prompt tokens instead of about 1.6M (mistral's full runs). acceptance lost 9 of its turns the same way.
2. **Refused requests are free.** 102 requests got the 400 before any work.
3. **No long prompts.** The largest prompt was 21,744 tokens, so the over-100K price never applied.
4. **Caching works well.** Almost nothing was sent uncached. Cache reads (1.32M tokens) cost $0.013. Cache writes (362K tokens) cost $0.045, the biggest part of the prompt cost.
5. **Short answers.** 100,687 output tokens cost $0.050, under half the total.

**A pass without the bug** would add about 1.25M prompt tokens and about 22K output (the missing bench turns, acceptance's 9 turns, the other broken runs). At the same mix that is about $0.05 more: **about $0.16 for a clean pass**, still well under the estimate.

## Sonnet 5.5 estimate

Same tokens at Sonnet 5.5 prices ($2 in, $10 out, $0.10 cache read, $2.50 cache write):

| | Prompt cost | Output cost | Total |
|---|---|---|---|
| This pass's tokens as measured | $1.04 (of which cache write $0.90) | $1.01 | **$2.05** |
| A clean pass (+1.25M prompt, +22K output) | $1.81 | $1.23 | **about $3.0** |
| A clean pass, output 1.5 times Haiku's | $1.81 | $1.85 | **about $3.6** |

Sonnet 5.5's default effort is high, where Haiku's is medium, so it may think and write more; the last row allows for that. Sonnet runs the same history check, so **a Sonnet pass today would break the same 11 runs**.

## Failures

### Jenab: thinking blocks replayed after the context changed (11 runs, 102 requests)

The error, the same each time:

> messages.1.content.0: Invalid `signature` in `thinking` block. The block is bound to a different conversation. … The `system` prompt, the `tools` list and the earlier `messages` all differ from when this block was created.

Claude Haiku 5.5 (like Sonnet 5.5, Opus 5.5 and Fable 5.1) binds each thinking block to everything sent before it: the system prompt, the tools and the earlier messages. If any of these changes and the block is sent again, the API answers 400. This is enforced by default for accounts created on or after 2026-08-31, so this workspace is enforced. Retrying the same request fails the same way.

Jenab changes that prefix in several places, then sends the old blocks back:

- **Inside a turn** (Jenab harness): `request()` calls `refreshCard`, `refreshTools` and `memoryNotices` before each request. Loading a skill or making a table changes the system blocks or the tools, so the next request in the same turn fails. Examples: bench-bitcoin turn 1 after `load_skill` and `request_schema_change`, schema-widen turn 1, project-card C1 and C2 turn 1, missing-features F2.
- **Between turns**: a cut (`cutIfDue`) builds the system blocks again and drops old turns, and session notes and memory change the context. Examples: session-notes turns 5–6, schema-design turn 2, acceptance turns 10–12 and 19–24.
- **Plain harness too**: `plainWindow` rebuilds the system prompt when the date changes. The bench scenarios move the clock every turn, so plain failed from turn 2 on. In the app this would happen to any chat that goes past midnight.

Runs it broke: session-notes N1, schema-design T1, schema-widen T4, schema-widen-check T4, bench-bitcoin (both harnesses), bench-bitcoin-long (both harnesses), missing-features F2, project-card C1 and C2. **acceptance passed anyway**, with 9 of 25 turns refused: its checks don't need every turn to succeed. That pass says little.

This is a protocol rule, the same for every model that runs the check. Possible fixes, for Main-Session to decide:

1. In the anthropic provider, send the `thinking-binding-controls-2026-08-01` beta with `thinking.block_binding.prefix_mismatch_behavior: "drop_block"`. The API then drops the stale blocks and the request goes on. It works only with `thinking: {type: "adaptive"}` sent explicitly, so the model needs a catalog entry first.
2. Or have Jenab leave out earlier thinking blocks from a request whose system blocks, tools or history it changed since those blocks were made.

### The check (2 runs)

- **multi-part t3_d_range_7_days**: "make that the last 7 days" arrived during the 30-day query. Haiku answered correctly from the rows it already had: high 65,230.80 on 09-26, low 62,980.10 on 09-22. It made no new 7-day call, so `after_message` failed. The answer was right.
- **grounding H4_ma_example**: the number check fired on a worked example of a moving average (3 of 4 numbers, such as 104.14, made up to illustrate). flash-lite failed H4 the same way; mistral passed.

### Shared with both reference models (3 runs)

- **role-skills R3_gold_news_daily**: Mother gave `web-research` but not `pipelines`. Both reference models are still 0/3 on R3 after TASK-043.
- **role-skills R6_exchange_trades**: Mother gave `database-design`, `pipelines` and `config-guide` but not `migrations`. The reference models are at 0/3 and 1/3 on R6.
- **skill-loading L04_sql_fails**: answered without loading `sql-queries` first. TASK-043 left this open as the model's.

Info-only failures, not counted: `grounding_fired` on grounding (the check stayed quiet because Haiku made up no numbers), `tool_called` info on G2, G8 and memory-untrusted, `grounding_quiet` info on grounding-honest R5 and R7.

## Haiku 5.5 next to the reference models

Runs passed. Reference numbers are the latest in TASK-043-report.md ("after", where it ran the set) and REFERENCE-SUITE-report.md (2026-11-10). Their reps differ, and some are older code: the number check changed in TASK-042.

| Set | Haiku 5.5 | flash-lite | mistral-large-4 |
|---|---|---|---|
| grounding | 15/16 | 12/14 | 11/16 |
| multi-part | 8/9 | 9/9 | 9/9 |
| role-skills | 4/6 | 11/18 | 7/18 |
| acceptance | 1/1 (9 turns refused) | 1/1 | 1/1 |
| session-notes | 0/1 (bug) | 0/3 | 3/3 |
| memory-untrusted | 2/2 | 2/2 | 6/6 |
| refresh-memory R1 | 1/1 | 2/3 | 1/1 |
| local-refresh (JSON on) | 1/1 | 1/1 | 3/3 |
| schema-design | 0/1 (bug) | 1/1 | 3/3 |
| schema-widen | 0/1 (bug) | 0/1 | 3/3 |
| schema-widen-check | 0/1 (bug) | 2/3 | 2/3 |
| bench-bitcoin | 0/2 (bug) | 5/6 | 2/2 |
| bench-bitcoin-long | 0/2 (bug) | 2/2 | 2/2 |
| local-basic | 3/3 | 3/3 | 3/3 |
| skill-loading | 11/12 | 12/12 | 12/12 |
| data-tools | 4/4 | 16/16 | 13/16 |
| missing-features | 5/6 (1 bug) | 5/6 | 6/6 |
| memory | 2/2 | 4/8 | 7/8 |
| project-card | 0/2 (bug) | 6/8 | 4/8 |
| grounding-honest | 9/9 | 4/9 | 5/9 |

- Where the bug didn't hit, Haiku 5.5 is at least as good as both reference models. It is clearly better on grounding-honest (9/9; both references fail R1, R4, R5 and R7), grounding (it made up no missing days), role-skills R2 and R5, and memory.
- It is fast: 1–12 s a run on most sets, about 2 s a request.
- The sets the bug broke can't be compared until it is fixed.

## models.json: no entry for claude-haiku-5-5

`internal/provider/models.json` has Claude Sonnet 5.5 and `claude-haiku-4-5-20251001`, but no `claude-haiku-5-5`. What the missing entry changes:

- **Context:** none in the runner (models.yaml gave 1,000,000). In the app, the provider's model list (`max_input_tokens`) or the user's setting must give it.
- **Thinking request:** `req.Known` is nil, so Jenab sends no `thinking` field. Haiku 5.5 thinks anyway (adaptive by default), but with the display "omitted": the thinking text is empty, so the app shows no thinking. With an entry (`thinking: true`, no `thinking_budget`) Jenab would send `{type: "adaptive", display: "summarized"}`. The `drop_block` fix above also needs that explicit adaptive setting.
- **Output cap:** none. Jenab's anthropic backend always uses its own default `max_tokens` (32,000) when the request sets none; the catalog's `output` isn't read for it.
- **Price:** the app can't show the cost (`priceOf` reads the catalog).
- **Fast suggestion:** the anthropic kind's suggested fast model is still Haiku 4.5.

Suggested entry (Main-Session's call):

```json
{"kind": "anthropic", "id": "claude-haiku-5-5", "name": "Claude Haiku 5.5", "context": 1000000, "output": 128000, "tools": true, "images": true, "thinking": true,
 "price": {"input": 0.1, "output": 0.5, "cache_read": 0.01, "cache_write": 0.125}, "fast": true}
```

The price format has one tier only; prompts over 100K cost 5 times as much ($0.50 / $2.50 / $0.05 / $0.625). Two more small points: the Sonnet 5.5 entry says `cache_read: 0.2`, but the pricing page says $0.10. And Haiku 4.5's `fast: true` should move to Haiku 5.5.

## Runner change: `-usage-log`

The per-run report counts only the chat's requests. Subagents', titles' and Refresh memory's tokens are left out. So I added a flag to `cmd/jenab-scenarios`:

- `-usage-log <file>` wraps every backend and writes one JSON line per provider request: time, model, seconds, input, cache read, cache write, output, thinking (when the provider reports it), stop reason and error.
- After each set it prints the set's totals, such as `usage: 22 requests, input 62, cache read 93948, cache write 13413, output 10860`.
- The wrapper keeps the backend's JSON mode (`SendsSchema`), so the Registry treats it the same.
- Test: `TestUsageWatch` in `cmd/jenab-scenarios/main_test.go`. `go test -p 2 ./cmd/jenab-scenarios/` passes; gofmt is clean.
- One limit: an error is clipped at 120 characters in the log. The full text is in the run's report.

## Files

- Reports: `scenarios/<set>/results/20261112-*` (git-ignored, in this worktree).
- Usage logs and the cost script: in the session's scratch folder, not committed.
