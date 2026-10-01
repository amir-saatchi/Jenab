# SPIKE-024 — MCP client
**Type:** Spike
**Status:** Open (needed before Phase 5)
**Gate:** 2

## Question
Can Jenab use MCP servers safely and cheaply, as in SPEC 8.7?

1. **SDK:** the official `modelcontextprotocol/go-sdk` against `mark3labs/mcp-go`. Both without cgo, on Windows. Stdio and Streamable HTTP, errors, cancelling a call, progress messages.
2. **Real servers:** a Blender server, one Unity server and the GitHub server, plus a small fake server for the tests. For each: start, list tools, call, errors, crash and restart. Is the whole process tree stopped (launchers such as `uvx` and `npx` start children)?
3. **Token cost:** tool count and schema tokens per server.
4. **Tool style:** on the development models, with the same prompt for all (SPEC 1), about 10 tasks per server:
   - every MCP tool as a native tool
   - `mcp_describe` + `mcp_call` (SPEC 8.7)

   Success rate, tokens, prompt cache hits.
5. **Approval:** which servers declare `readOnlyHint`? Does the approval flow in 8.7 work in the scenario runner ([TASK-001](TASK-001-scenario-runner.md))?
6. **Prompt injection:** a tool result that contains instructions. Does the agent try to call a write tool, and does the approval stop it?

## Rules for the run
- Blender and Unity are installed by the user if wanted. The spike doesn't install apps; without them, it uses the fake server.
- MCP servers are started only from commands the user has approved for the spike.
- Only processes the spike started are stopped, by PID or by their Job Object.

## Done when
Each point has results, and there is a decision on the SDK, the tool style and the approval rules in SPEC 8.7.

## Result
_Pending._

## Decision
_Pending._
