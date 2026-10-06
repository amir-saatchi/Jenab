# SPIKE-028 — A request list, and loading tools and history when needed
**Type:** Spike
**Status:** Open (before Phase 2)
**Gate:** 3

## Question
In SPIKE-023, the finish turn gave every part of a multi-part request in 88% of runs with the prompt rule (SPEC 8.3). The rest relied on the model remembering. Can a request list that the app checks close that gap? And does it pay to send the model only the tools, skills and history a request needs?

The pattern (proposed on 2026-10-03):
- **First call:** the model sees the prompt, recent history, the system prompt, the tool index and the skill list (8.9). It either answers directly, which ends the turn, or answers what it can and calls `plan(requests)`.
- **Request list:** each request has `id`, `ask`, `status` (`open`, `running`, `done`, `blocked`, `dropped`), `note`, `after` (the requests it needs first) and `result` (a ref). It goes back into every call, and it shows in the chat as a checklist.
- **The app enforces it:** a turn can't end while a request is `open` or `running`. `blocked` and `dropped` need a reason. If the model stops early, the app sends one more request naming the open items.
- **Tool index, details when needed:** the start of the prompt never changes, so the provider's cache holds.
  - The core tools (data, memory, `plan`, `load_skill`) stay native tools with their full schemas.
  - Every other tool is in a short index in the system prompt: its name and one line.
  - `describe_tool(name)` returns a tool's schema as a tool result, so the details are added after the cached part, never in the tool list. `call_tool(name, args)` runs it; Jenab checks the arguments against the schema first.
  - This is SPEC 8.7's pattern for MCP tools (`mcp_describe`, `mcp_call`), used for Jenab's own rare tools.
- **History cards:** older related parts, through `search_history` and `read_messages`, added to the recent turns, never replacing them.
- **Subagents per request** only for heavy, independent work; the model decides. A result comes back as a short note plus a ref.
- **Notes:** in the session notes (3.4), which already hold the plan.
- **Subjects:** [SPIKE-029](SPIKE-029-subjects.md) keeps the chat's work across turns as subjects. A request that ends `done`, `blocked` or `dropped` may update or create a subject. The two designs are decided together.

## Conditions
1. **Today:** the SPEC 8.3 loop with the prompt rule.
2. **List:** today's loop plus `plan` and the app's check.
3. **Full:** the list, the tool index with details when needed, and history cards.
4. **Index only:** today's loop with the tool index, to see its effect apart from the list.

## Tests
- SPIKE-023's 20 scenarios, plus new ones with 3–5 requests, one request that depends on another (`after`), and one that should be `dropped` with a reason.
- SPIKE-025's T-load scenarios, to see how often a needed tool or skill is missed under conditions 3 and 4.
- Long chats where a follow-up ("do the same for ETH") needs an earlier turn, for the history cards.
- No false plans: a simple question must not get a `plan` call.

## Measures
- Every part answered, and nothing repeated or started twice.
- Tokens per run, and the share read from the provider's cache where the provider reports it.
- The size of the index against the full schemas of the same tools.
- Wrong arguments through `call_tool` against the same tools called natively.
- Time to the first answer, and to the last.
- Needed tools, skills or history that were missed, and how often the model loaded them itself afterwards.

Models: the SPIKE-018 development models, with the same prompts for all (SPEC 1). The harness extends SPIKE-023's scenario runner.

## Done when
Each condition has results per model, with at least 3 reps per cell. There is a decision on `plan` and the app's check (8.3), on the tool index (8.1), on history cards (3.6), and on a `Plan` message part (P1-19).
