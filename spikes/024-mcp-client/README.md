# SPIKE-024 — MCP client

Which Go MCP client SDK should Jenab use, what do an app's MCP tools cost
in tokens, should the model see every tool or a describe/call pair, and do
the approval rules of SPEC 8.7/8.8 hold up against a tool result that
carries instructions? See [the ticket](../../Docs/Tickets/SPIKE-024-mcp-client.md).

The servers are Viso (installed `Viso.exe --mcp`) and a small fake server
(`fake/`). The agent runs are on the app's real orchestrator
(`internal/agent`), as in SPIKE-029. It changes no app code. This folder
has its own `go.mod` so the two SDKs stay out of Jenab's.

```
go build -o <scratch>/fake.exe ./fake
go run . -sdk -fake <scratch>/fake.exe                         # SDK checks → results/sdk-checks.md
go run . -viso-list                                           # Viso's tools → results/local/viso-tools.json (Viso may be closed)
go run . -calib -env <.env>                                   # tool cost in prompt tokens → results/calib.jsonl
go run . -inject -env <.env> -fake <scratch>/fake.exe -reps 2 # injection runs → results/inject.jsonl
go run . -probe                                               # Viso: create the test project (Viso open, signed in)
go run . -try 'tool {json}' -project <id>                     # harness calls on the test project, then get_scene
go run . -bench -env <.env> -project <id>                     # tool-style bench → results/bench.jsonl
FAKE=<scratch>/fake.exe go test .                             # closeOnly and the watch leave a launched child running; the bench row trim
```

Builds: `GOPROXY=off GOFLAGS=-mod=mod CGO_ENABLED=0`. Runs resume: a run
already in the jsonl without an error is skipped. Gemini requests across
all results files are capped by `-gemini-max` (150). `-viso` sets Viso's
exe (default `%LOCALAPPDATA%\Viso\Viso.exe`). Viso's own texts (tool
definitions, instructions) go to `results/local/`, which git ignores.

## Files

| File | What |
|---|---|
| `client.go` | One client interface over both SDKs (stdio and Streamable HTTP) |
| `job_windows.go` | Job Object: start suspended, assign, resume, kill on close; `watch` clears kill-on-close once the job holds more than the server; `closeOnly` then stops only the server; process snapshots checked by start time |
| `job_other.go`, `procs.go` | Stubs so other systems build; the process type |
| `sdk.go` | The SDK checks |
| `fake/` | The fake server: echo, fail, slow (progress), crash, big, and a notes tracker with planted instructions |
| `mcptools.go` | MCP tools as Jenab tools: native (`<server>_<tool>`) or `mcp_describe` + `mcp_call`; approval cards; the block-4 card |
| `harness.go` | One agent turn on the orchestrator, cards answered by a scripted user |
| `inject.go` | The injection tasks and conditions |
| `viso.go`, `bench.go`, `scene.go` | Viso: the tools offered, the guard, the bench tasks, their checks, the `get_scene` parser; full bench rows go to `results/local/`, committed ones without Viso's texts |
| `models.yaml` | ollama-cloud gemma4:31b, gemini-3.5-flash-lite |

## Setup

- **Approval in the spike:** kind `mcp`, target `<server>/<tool>`. A tool that declares `readOnlyHint` gets Once / Always; any other tool too, unless the 8.7 rule applies. **The 8.7 rule** (an option): after the turn has read an MCP result, a tool that isn't read-only asks again, even with an Always, and its card has no Always.
- **Scripted user:** read-only tools Always; the write the task asks for Once; anything else Deny ("I didn't ask for that.").
- **Viso tools offered:** 24 of 32. Never offered: `list_projects`, `rename_project`, `duplicate_project`, `load_local_model`, `get_credit_balance`, `generate_image`, `preview_generation`, `get_generation`. `open_project` works only for the spike's test project.
- **Block 4:** a short card per server; in describe style it lists each tool with its first sentence. The bench puts Viso's server instructions in block 4 in both styles.
