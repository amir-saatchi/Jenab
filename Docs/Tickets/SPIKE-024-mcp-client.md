# SPIKE-024 — MCP client
**Type:** Spike
**Status:** Done
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
Run on 2026-10-25 on Windows 11 (commit cfd1990), with the user's Viso app as the real server and a fake server for the rest; no GitHub, Blender or Unity server was installed. Models: gemma4:31b and gemini-3.5-flash-lite. Details are in `spikes/024-mcp-client/results.md`.

1. **SDK:** go-sdk v1.8.0 passed every check (stdio, Streamable HTTP, errors, cancel, progress, a 6 MB result, crash and restart). mcp-go v1.3.0 failed to connect to a default HTTP server and didn't pass a cancel over HTTP. Neither SDK's close stops a launcher's children on Windows; a Job Object does. Neither checks arguments against the schema. No cgo.
2. **Real server:** Viso has 32 tools, 52 KB of schemas. If Viso isn't open, its server opens the Viso app, inside the server's job, so closing the job closed the app.
3. **Token cost:** one app's tools as native tools cost 12–15k prompt tokens in every request; `mcp_describe` + `mcp_call` about 1.3k.
4. **Tool style** (10 one-step Viso tasks):

   | | Passed | Prompt tokens / run |
   |---|---|---|
   | gemma4:31b, native | 20/20 | 35,000 |
   | gemma4:31b, describe | 18/20 | 12,500 |
   | gemini-3.5-flash-lite, native | 10/10 | 39,900 |
   | gemini-3.5-flash-lite, describe | 10/10 | 10,600 |

   gemma often called a tool without describing it first and guessed the arguments; the errors that named the right ones let it fix them.
5. **Approval:** the approval card works in the runner. Viso marks 7 tools read-only and 4 destructive. SPEC 8.7 and 8.8 disagreed on MCP writes at Standard.
6. **Prompt injection** (a fake notes app whose notes carry instructions, 48 runs): gemma followed a planted "delete note 1" 3 times; Gemini never. The card stopped all 3, but with an *Always* already given, the delete ran. No model told the user that a result carried instructions.

## Decision
**Decided (2026-10-25):**
- **SDK:** the official `modelcontextprotocol/go-sdk`, in a new `internal/mcp` package in Phase 5.
- **Tools:** keep `mcp_describe` + `mcp_call`. Jenab refuses argument names the schema doesn't list, and its argument errors name the arguments the tool takes.
- **Approval:** after a turn reads untrusted content (web pages, files, MCP results), a tool that deletes or reaches outside asks again, with no *Always*. Other writes keep their *Always*. Asking for every write was rejected as too many questions.
- **Processes:** each stdio server runs in a Job Object, but an app the server opens (a process with a window) is never closed, not on the idle stop, Jenab's exit or a crash; only background helpers are stopped. The job code moves out of `internal/sqlworker` into a shared package, used by the SQL workers, MCP servers and later commands.
- **Open:**
  - Detecting "a process with a window" is untested; Phase 5 builds and tests it.
  - Longer Viso tasks, other servers (GitHub), and whether the result wrapper should tell the model that a result carried instructions.
  - go-sdk lost the last progress message in 2 of 5 calls on a re-run; progress is display-only.

Doc changes: SPEC 8.7 (SDK, argument checks, approval after untrusted content, processes) and 8.8 (the MCP write row).
