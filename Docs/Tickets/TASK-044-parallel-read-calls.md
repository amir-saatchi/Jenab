# TASK-044 — Run read-only tool calls of one answer together
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** —

## Goal
Block 1 asks the model to make calls that don't need each other's results in the same message (`prompts/base.md`, TASK-033). `runTools` (`internal/agent/turn.go`) still runs them one after another. So the saved requests don't save wall time. Measure how much time that costs, and if it is worth it, run read-only calls of one message together.

## Scope
1. **Measure first.** From the reference suite's traces (`ToolTrace.Took`), count the messages with two or more read-only calls and the tool time they spend. Go on only if tool time is a real share of turn time, for example in `fetch_page` or `web_search` runs.
2. **A flag:** `Parallel bool` on `tool.Spec`. Candidates: `query`, `describe_table`, `read_ref`, `search_ref`, `search_history`, `read_messages`, `fetch_page`, `web_search`, `read_feed`, `call_api`, `bucket_read`, `bucket_list`, `list_files`, `read_file`, `search_code`, `mcp_describe`. Not flagged: anything that writes, `load_skill`, `ask_user`, `subagent`, and the `desktop_*` tools.
3. **Order:**
   - Checks and approval cards run first, one call at a time, in call order. A call's check counts the untrusted mark of every earlier call in the message as already set (from `Effects`), so the Auto rule (8.8) asks exactly as it does today.
   - Then the approved flagged calls run together, up to a small cap (start with 4). Other calls run alone, in their place.
   - Results are stored in call order, so the history and the cache stay the same as today.
4. **Stop and limits:** *Stop* cancels them all; each call keeps its own timeout; the runtime panel shows each call.
5. **SPEC 8.3:** a short paragraph with the rule.

## Rules
- The same tools, prompts and order for every model (SPEC 1). No change to what the model sees.
- No loosening of approvals: a message runs the same cards in the same order as now.
- No streaming dispatch: tools still run only after a complete answer (8.3).

## Done when
- The measurement is in the ticket's result, with the decision to build or not.
- If built: Go tests for call-order results, a card in the middle of a message, the untrusted mark across siblings, *Stop* during parallel calls, and a panic in one call.
- Before and after wall time on the scenarios with several fetches, on both reference models, and `jenab-scenarios check scenarios` passes.
