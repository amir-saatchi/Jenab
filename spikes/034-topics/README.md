# SPIKE-034 — Topics

Does the agent propose a topic (a thread one level below a chat, with its
own notes and history) at the right moments, and do topics beat one long
chat with cuts and `search_history`? Condition 4 runs SPIKE-029's combined
subjects design on the same chats. See
[the ticket](../../Docs/Tickets/SPIKE-034-topics.md), the findings in
[`results.md`](results.md), and the full design of the harness, chats and
scoring in [`PLAN.md`](PLAN.md).

The harness plays two scripted chats on the app's real orchestrator
(`internal/agent`), with the real `search_history`, `read_messages` and
`update_memory`, and fakes for the tools the app doesn't have yet. Each topic
is a chat in the same project; the harness keeps the parent link.

```
go run . -dry -reps 1                                                        # no model: checks every path of the harness
go run . -env D:/DEV/Burrow/.env -models ollama-cloud/gemma4:31b -scenarios sqlite -reps 1   # smoke run
go run . -env D:/DEV/Burrow/.env -models ollama-cloud/gemma4:31b -reps 3     # a model's 24 runs
go run . -resume results/<stamp>                                             # runs that are missing or failed
go run . -report results/<stamp>/runs.jsonl[,more/runs.jsonl]                # results.md again
```

Keys come from `../../.env` (`-env` points elsewhere, e.g. from a worktree,
which has no `.env`). `models.yaml` lists the reference models and the
stand-ins; it uses no reserve key.

## Files

| File | What |
|---|---|
| `scenarios.go` | Two chats of 19 messages, `sqlite` (SQLite expert) and `jobs` (job search). Each message has a label (its subject, or `main`), a kind and its checks |
| `world.go` | The fake project from SPIKE-029, with `alter_table`, column defaults, references and unique keys, and the project card built from it |
| `threads.go` | Topics: the main thread and its topics, their notes, the index, `update_notes`, `merge_topics`, `propose_topic`, and block 5 for each thread |
| `subjects.go` | Condition 4: `update_subject` (open items, no status), `get_subject`, `merge_subjects`, the rule and the index |
| `run.go` | One run: a new project, the orchestrator, the scripted user, the parallel step, and what each message's checks read |
| `score.go`, `report.go` | Scoring and `results.md`; a transcript per run |
| `dry.go` | `-dry`: a rule-based fake model, so the harness runs without a key |

## Setup

- **Conditions:** 1 one chat; 2 topics the scripted user opens at a subject's first message; 3 topics the agent proposes with `propose_topic`, accepted when the proposal matches the message's label; 4 SPIKE-029's subjects.
- **Block 5:** the app hook `agent.Deps.Notes` (spike-only; the app leaves it nil) gives each chat its own block after the project card, built at cuts like blocks 2–4. In the main thread it holds the rule, the main thread's notes and the topic index; in a topic, the main thread's notes (read-only) and the topic's own.
- **History window:** `history_min_turns` 2 and `history_max_turns` 4, so cuts come every few turns.
- **The parallel step:** two messages sent at once to two topics, both saving a fact to project memory (conditions 2 and 3 when both topics exist; otherwise one after the other).
- **Proposals:** the scripted user answers `propose_topic` inside the call. On an accept, the turn that proposed ends with one line and the message is sent again in the topic; both turns count.
