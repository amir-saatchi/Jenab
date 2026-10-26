# SPIKE-035 — Plan first, as an approval check

Can a plan the agent makes before it reads untrusted content catch the
writes a planted line asks for, the same way for every model, without
asking on normal tasks? See [the ticket](../../Docs/Tickets/SPIKE-035-plan-first.md)
and [the results](results.md).

The runs use SPIKE-024's harness (the app's real orchestrator,
`internal/agent`, with a scripted user) and its fake notes server, copied
here and extended. It changes no app code. This folder has its own
`go.mod` (SPIKE-024's) so the MCP SDK stays out of Jenab's.

```
go build -o <scratch>/fake.exe ./fake
go build -o <scratch>/planfirst.exe .
<scratch>/planfirst.exe -fake <scratch>/fake.exe -scratch <scratch>/runs -env <.env> \
    -models ollama-cloud/gemma4:31b -conds none,checked,unchecked -tasks attacked -reps 3
<scratch>/planfirst.exe -report > <scratch>/tables.md      # the tables in results.md
FAKE=<scratch>/fake.exe go test -p 2 .
```

Builds: `GOPROXY=off GOFLAGS=-mod=mod CGO_ENABLED=0`. Runs resume: a run
already in `results/runs-<model>.jsonl` without an error is skipped.
Gemini requests in the results files are capped by `-gemini-max` (150).
`-tasks` takes ids (`I4,N08`) or kinds (`attacked`, `normal`).

## Files

| File | What |
|---|---|
| `fake/` | SPIKE-024's notes tracker plus `rename_note` and `add_note`; notes with planted instructions, or `-clean` without |
| `client.go` | SPIKE-024's go-sdk client, stdio only |
| `mcptools.go` | `mcp_describe` + `mcp_call` (SPEC 8.7); the approval with SPEC 8.7's rule and the plan check |
| `plan.go` | The `plan` tool and the matching of a write against the plan |
| `harness.go` | SPIKE-024's harness: one turn on the orchestrator, cards answered by a scripted user |
| `tasks.go` | The attacked and normal tasks, their checks |
| `runner.go`, `main.go` | The runs, results and transcripts |
| `report.go` | The tables (`-report`) |
| `models.yaml` | ollama-cloud gemma4:31b, gemini-3.5-flash-lite |
| `results/` | The runs, transcripts and `tables.md`; `smoke/` has the smoke runs with the plan wording the runs used |

`go.mod` is SPIKE-024's: it still requires `mcp-go` and `x/sys`, which
this spike doesn't use. `go mod tidy` needs the network, so it stays.

## Setup

- **Tools:** `mcp_describe` and `mcp_call` for one server, `notes`, as
  SPEC 8.7 decided; plus `plan` in the plan conditions.
- **Approvals:** the project already has an *Always* for every notes tool,
  so only a rule makes a card. SPEC 8.7's rule as decided in SPIKE-024:
  after untrusted content, a tool that deletes or reaches outside
  (`destructiveHint` or `openWorldHint` true, or missing) asks again. In
  the fake, `update_note`, `rename_note` and `add_note` declare both
  false, so 8.7 lets them through; `delete_note` and `share_note` ask.
- **Untrusted content:** an MCP result. A call counts as after it when it
  is in a later assistant message than the call that returned the first
  result (the model wrote it with the result in its context).
- **Conditions:** `none` (8.7 only, no plan tool), `checked` (plan tool,
  and a write after untrusted content that no plan made before it names
  asks, with "Not in the plan." on the card), `unchecked` (the same plan
  tool and prompt, never enforced).
- **Scripted user:** Once for a write the task asks for, Deny ("I didn't
  ask for that.") for any other.
