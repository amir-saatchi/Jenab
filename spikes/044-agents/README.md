# SPIKE-044 — Specialized agents

Can Mother set an agent up well: pick its tool groups and write its custom
prompt (at most 500 tokens, in the role's slot in block 1), when the user asks
for an agent and when nobody asks? And does a chat that lacks a tool say so,
rather than pretend or work around it? See
[the ticket](../../Docs/Tickets/SPIKE-044-specialized-agents.md) and the
findings in [`results.md`](results.md).

The harness plays seven scenarios on the app's real orchestrator
(`internal/agent`) in SPEC 9's Bitcoin project, with the real `query`,
`describe_table`, `insert`, bucket tools, memory and history tools, and fakes
for the tools the app doesn't have yet (views, pipelines, the web, the linked
folder, the schema agent). A chat's tools come from the spike-only hook
`agent.Deps.ChatTools` (the app leaves it nil): the chosen groups plus the
always-on tools, narrowed further by the usual gating (TASK-033), and a call
to any other tool is refused with a message that names its group.

```
go run . -dry -reps 1 -conds 0,1,2,3,4                                      # no model: checks every path of the harness
go run . -env D:/DEV/Burrow/.env -models gemini/gemini-3.5-flash-lite -scenarios entry -reps 1   # smoke run
go run . -env D:/DEV/Burrow/.env -reps 1                                    # every reference model, 28 runs each
go run . -resume results/<stamp> -reps 3                                    # runs that are missing or failed, up to 3 reps
go run . -report results/<stamp>/runs.jsonl[,more/runs.jsonl]               # results.md again
```

Keys come from `../../.env` (`-env` points elsewhere, e.g. from a worktree,
which has no `.env`). `models.yaml` lists the models of the runs: Gemini,
NVIDIA, and Anthropic Haiku 5.5 in gemma4:31b's place (`OLLAMA_API_KEY`
returned 402, and the user chose Haiku for this spike). It uses no reserve
key. To run Gemini on another key, point `-models-file` at a copy with a
different `key_env` and the same provider name, so `-resume` matches the
jobs. A run whose turn a provider error stopped (a quota, a 402) counts as
failed, so `-resume` runs it again.

Each batch writes `results/<model>/<stamp>/`: `runs.jsonl`, `results.md` and a
transcript per run. `results/all.md` is the report over every model's
`runs.jsonl`.

## Files

| File | What |
|---|---|
| `groups.go` | The six tool groups, the always-on tools, Mother's tools, the refusal and `create_chat`'s schema (groups, or single tools for condition 3) |
| `scenarios.go` | The seven scenarios: the request (asked and unasked), the answer key (title, role, groups), the needed and forbidden groups, the follow-up task and its check |
| `world.go` | SPEC 9's Bitcoin project: the tables and rows, the pipelines, views and page, the project card, and the fake tools |
| `mother.go` | `create_chat` and `send_to_chat`: Mother's calls start the agent's turn after hers ends |
| `run.go` | One run: a new project, the orchestrator with the hook, Mother's turns, the agent's turns and the end state |
| `score.go`, `report.go` | Scoring and `results.md`; a transcript per run |
| `dry.go` | `-dry`: a rule-based fake model, so the harness runs without a key |

## Setup

- **Conditions:** 1 the key: the scenario's agent exists with the key's
  groups and role, and the user sends the task to it. 2 asked, groups: the
  user asks Mother for the agent; `create_chat` takes groups. 3 asked, single
  tools: the same, with single tools in the schema. 4 unasked: the user tells
  Mother about the job without asking for an agent ("plain" message), then
  gives her the job; Mother decides whether to make one. 0 (optional) every
  group: the key's role with every group, as a token control.
- **Mother** has in every condition: read data, files, memory,
  `create_chat` and `send_to_chat`, and the always-on tools. No change data,
  no views or pipelines, no web.
- **Scenario 7** asks Mother for a write while no agent exists: does she
  delegate it, or pretend?
- **The follow-up task** goes to the agent the request made (the last one
  created), or to Mother when there is none; its check reads the project's
  end state.
