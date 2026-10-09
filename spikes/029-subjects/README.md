# SPIKE-029 — Subjects

Does the agent keep a list of subjects (one object per piece of the chat's
work, with its status and outcome) when a prompt rule tells it to, and does
the list help it after the history window is cut? See
[the ticket](../../Docs/Tickets/SPIKE-029-subjects.md).

The harness plays long scripted chats on the app's real orchestrator
(`internal/agent`), with the real `search_history` and `read_messages`, and
fakes for the tools the app doesn't have yet. It changes no app code.

```
go run . -models ollama-cloud/gemma4:31b -scenarios crypto -conds 3 -reps 1   # smoke run
go run . -models ollama-cloud/gemma4:31b,ollama-cloud/gpt-oss:120b -reps 3    # main run
go run . -resume results/<stamp>                                               # runs that are missing or failed
go run . -report results/<stamp>/runs.jsonl                                    # results.md again
go run . -rule v1 ...                                                          # round 1's prompt rule (v2 is the default)
```

Keys come from `../../.env`. `models.yaml` uses the reserve Ollama Cloud key,
`OLLAMA_API_KEY_II`.

## Files

| File | What |
|---|---|
| `scenarios.go` | Two chats of 14 messages: `crypto` and `jobs`. Each message has a kind (work, change, question, follow-up), a topic and its checks |
| `world.go` | The fake project: `create_table`, `insert_rows`, `update_rows`, `describe_table`, `save_pipeline`, `run_pipeline`, `get_config`, `save_view`, `update_memory`, `web_search`, and the project card built from them |
| `subjects.go` | The subject store, `update_subject`, `get_subject` and the prompt rule |
| `run.go` | One run: a new project and chat, the orchestrator, the messages one by one, and the app's check in condition 4 |
| `score.go`, `report.go` | Scoring and `results.md`; a transcript per run |

## Setup

- **Conditions:** 1 baseline; 2 `update_subject`, `get_subject` and the rule; 3 condition 2 plus the index in the context; 4 condition 3 plus the app's check.
- **History window:** `history_min_turns` 2 and `history_max_turns` 4, so the window is cut at turns 6, 9 and 12. The follow-ups and the changes late in each chat ask about turns that are out of the window by then.
- **The rule and the index** are in block 4, after the project card, through `agent.Deps.Card`. The agent asks for that block only at a cut, so the index is as of the last cut, as SPEC 3.1 has it for blocks 2–5.
- **Project memory** is shown in the card in every condition, as blocks 2–3 will show it (3.3). A decision the agent saves to memory is visible without subjects, so the checks that need subjects are about status, open items and dropped work.

## What differs from the ticket

- **Block 4, not block 5.** Session notes are not in the context yet, and the card hook is the one place a spike can add text without changing the app. The cache behaviour is the same: the block changes only at cuts.
- **`get_subject` without `id` lists every subject.** This is the "tool to get all the subjects" from the first proposal, so condition 2 tests that idea and condition 3 tests the index in the context.
- **No `expected_revision`.** Only the agent writes subjects in these runs, so the revision rule would never apply.
- **The app's check is a message, not a request.** Without changing the agent loop, the harness sends `[app check: …]` as the next message, which starts a turn. In the app it would be one more request inside the turn. Condition 4 therefore has extra turns, which move its cuts.
- **The card isn't refreshed after a schema change**, only at cuts. The app refreshes it at once (3.1).
- **The fake tools have no approvals.** Their effect is `reads_db`, so no approval card stops a run.
