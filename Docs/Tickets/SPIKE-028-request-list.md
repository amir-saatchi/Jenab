# SPIKE-028 — Loading tools and history when needed
**Type:** Spike
**Status:** Done
**Gate:** 3

## Question
Does it pay to send the model only the tools and history a request needs? Today every request carries the full schema of every tool in 8.1, whether the request uses it or not.

**Narrowed on 2026-10-27.** The ticket first also held a request list, a `plan(requests)` tool the app checks, and a *Plan first* switch. Those moved to [SPIKE-036](SPIKE-036-plan-feature.md), the plan spike for later; SPIKE-035 dropped *Plan first* as an approval check for v1. The link to SPIKE-029's subjects went with its deferral.

The pattern:
- **Tool index, details when needed:** the start of the prompt never changes, so the provider's cache holds.
  - The core tools (data, memory, `load_skill`) stay native tools with their full schemas.
  - Every other tool is in a short index in the system prompt: its name and one line.
  - `describe_tool(name)` returns a tool's schema as a tool result, so the details come after the cached part, never in the tool list. `call_tool(name, args)` runs it; Jenab checks the arguments against the schema first.
  - This is SPEC 8.7's pattern for MCP tools (`mcp_describe`, `mcp_call`, SPIKE-024), used for Jenab's own rare tools.
- **History cards:** older related parts, found through `search_history` and `read_messages`, added to the recent turns, never replacing them.

## Conditions
1. **Today:** every tool native with its full schema, and today's history window (3.6).
2. **Index:** the tool index with `describe_tool` and `call_tool`.
3. **Index and cards:** the index, plus history cards.

## Tests
- SPIKE-023's `multi-part` scenarios and SPIKE-025's T-load scenarios (`skill-loading`), to see how often a needed tool or skill is missed.
- Scenarios that need a rare tool, called through `call_tool`, against the same calls made natively.
- Long chats where a follow-up ("do the same for ETH") needs an earlier turn past a cut, for the history cards.

## Measures
- Tasks done, against condition 1.
- Tokens per run, and the share read from the provider's cache where the provider reports it.
- The size of the index against the full schemas of the same tools.
- Wrong arguments through `call_tool` against the same tools called natively.
- Needed tools or history that were missed, and how often the model loaded them itself afterwards.

Models: the reference models, `gemini-3.5-flash-lite` and `ollama-cloud/gemma4:31b`, with the same prompts for all (SPEC 1). The harness extends the scenario runner (TASK-001, TASK-013).

## Done when
Each condition has results per model, with at least 3 reps per cell. There is a decision on the tool index (8.1) and on history cards (3.6).

## Result
Run on 2026-10-27 in two rounds (commit 143aec2) on `gemma4:31b` (Ollama Cloud) and `ministral-14b-latest` (Mistral), 175–177 runs per model a round. Gemini was not run, to save its shared quota. Details are in `spikes/028-tool-index/results.md`.

- **Conditions:** 1 today; 2 the tool index; 3 the index and history cards; 4 today's tools and history cards (added in round 2).
- **Round 1 was unfair to the index,** as a review found. The app hides a skill whose `load_with` tools are missing from the tool list (8.9), so config-guide, pipelines and web-research vanished under the index; part of the token saving was that missing skill text; a native call of an indexed tool got "no such tool". Round 2 fixed all three and kept `search_history` and `read_messages` native.
- **The index (2 against 1), round 2:**
  - The index lines plus `describe_tool` and `call_tool` are 548–601 tokens, against 1,525–2,002 for the full schemas of the same tools.
  - Tasks done stay level: gemma 90→91 of 100, ministral 70→71.
  - Prompt tokens per run are mixed: multi-part −27% and −37%, skill-loading +48% and +16% (more requests), rare tools −12% and +1%, long chats −3% and −27%.
  - New mistakes: wrong arguments in 16 of 92 calls (gemma) and 8 of 78 (ministral), calls without describing first (48 and 28), and 39 native calls of indexed or made-up names from ministral, which the redirect recovers.
- **History cards (4 against 1), long chats, 5 reps:**
  - Tasks done: gemma 19→25 of 25, ministral 10→17. Prompt tokens per run −42% and −49% (about −30% without H5's long runs), requests per run 4.8→3.1 and 5.4→2.9, history calls 52→17 and 39→5.
  - Where a card hit the right turn, the follow-up was done and used about half the tokens. Where no card was made, it cost nothing.
  - **Weak spots:** most of the gain is one chat (H4). Cards are the same in every rep, so 4 chats are 4 samples. The one true paraphrase ("early warning" for "drop alert") got no card. The two other paraphrase hits matched on small words ("it", "in", "as"), which a longer chat would not count, and were written after the scorer could be run offline. A fixture left a lure in H5.
- **Cache:** Mistral's cache shares moved by chance (the same 4,817-token prompt read 512 to 4,736 from cache), so no cache effect is claimed. Ollama reports none.

## Decision
**Decided (2026-10-27):**
- **No tool index for Jenab's own tools (8.1).** All stay native with their full schemas. The index gave no task gain, mixed tokens and new argument mistakes, for tools that take 1.2–1.7k tokens.
- **MCP keeps its index (8.7),** with two fixes from this spike: a skill whose `load_with` names an MCP tool stays in the skill list and loads on `mcp_call`, and a native call of an MCP tool's name gets a result pointing to `mcp_call`.
- **History cards: yes in principle (3.6),** built in TASK-027, which must show they work first: small words don't count, and the test chats are longer and written without running the scorer.
- `Deps.ToolIndex` and the scenario runner's `Options.Deps`, `Call` and `Setup` stay as spike-only hooks.

Doc changes: SPEC 3.6, 8.1 and 8.7.
