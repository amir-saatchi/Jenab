# SPIKE-045 — Subscribe and publish between agents

What does subscribing mean, do models subscribe to what their job needs (and
not more), and do they publish when another chat needs the result (and not on
routine turns)? See [the ticket](../../Docs/Tickets/SPIKE-045-subscribe-and-publish.md)
and the findings in [`results.md`](results.md).

The harness plays one scripted day on the app's real orchestrator
(`internal/agent`) in SPEC 9's Bitcoin project. Six agents are set up as SPEC
8.6 says, each with a title, a role (its custom prompt) and its tool groups:

| Agent | Groups | Its job needs |
|---|---|---|
| Schema agent | Read, Change data | nothing (its own schema changes aren't delivered to it) |
| Docs agent | Read, Files | `schema_changed`, any table |
| Reviewer | Read, Build and run | `pipeline_run`, outcome = failed, any pipeline |
| Price watcher | Read, Web | nothing in Part 1 (daily_btc runs are allowed); publishes the 6.4% drop in Part 2 |
| Reporter | Read, Files | the `alerts` channel in Part 2 (weekly_summary runs are allowed) |
| Data entry | Read, Change data | nothing: any subscription is unneeded |

The scripted user sends 14 messages from 09:00 to 16:00 (`script.go`), one in
Persian. Each agent's first message is the turn it was created for. App
events come in between: daily_btc fails at 10:00 (r_101, HTTP 503), the user
runs weekly_summary at 10:15, daily_btc succeeds at 12:00 and adds today's
price (−6.4%). The schema agent's two changes (a column `volume`, a table
`eth_prices`) are real migrations, so the project card changes as in the app.

Everything the app doesn't have sits in the harness and reaches the app only
through `agent.Deps` hooks the app leaves nil:
- `Notices`: delivery. A matching event is queued for each subscribed chat
  (once per chat, never the chat that caused it, at most 1,000 characters)
  and written into the chat's next turn as a notice. A notice starts no turn.
- `Notes`: block 5 is the session notes, then the chat's subscriptions
  (conditions 2–5) and, in condition 5, the channels with their last 20
  titles. It is built at cuts.
- `Section` (added for this spike): condition 5's publish rule in block 1.
- `ChatTools`: each agent's groups, plus `subscribe` and `unsubscribe`
  (condition 3) or `publish` and `read_report` (conditions 4 and 5).

Channels are a fixed list made in the setup: `alerts`, `data` and `reviews`.
Publishing to any other name is refused (the error lists the channels) and
counted.

```
go run . -dry -reps 1                                                          # no model: checks every path of the harness
go run . -env D:/DEV/Burrow/.env -models gemini/gemini-3.5-flash-lite -reps 1   # one run per condition
go run . -resume results/<model>/<stamp> -reps 3 -env D:/DEV/Burrow/.env       # runs that are missing or failed, up to 3 reps
go run . -report results/<model>/<stamp>/runs.jsonl[,more/runs.jsonl]          # results.md again
```

Keys come from `../../.env` (`-env` points elsewhere, e.g. from a worktree,
which has no `.env`). A run whose turn a provider error stopped (a quota, a
402) counts as failed, so `-resume` runs it again. Each batch writes
`results/<model>/<stamp>/`: `runs.jsonl`, `results.md` and a transcript per
run.

## Conditions

- **Part 1, subscribe** (app events only, no publishing): 1 none, only the
  project card; 2 the key's subscriptions, as if set in the *Customize*
  panel; 3 agents subscribe themselves with `subscribe`.
- **Part 2, publish** (channels, with the key's subscriptions): 4 `publish`
  with a description that says when to use it; 5 the same plus the prompt
  section and the channel list in block 5.

## Files

| File | What |
|---|---|
| `agents.go` | The groups (SPEC 8.6), the six agents and their answer keys |
| `pubsub.go` | The subscription store, events, delivery, block 5, and the `subscribe`, `unsubscribe`, `publish` and `read_report` tools |
| `script.go` | The day: 14 messages, the pipeline runs and what each turn is to the publish and use measures |
| `world.go` | SPEC 9's Bitcoin project, the card and the fake tools (from SPIKE-044), with the scripted runs and schema changes |
| `run.go` | One run: a new project, the agents, the orchestrator with the hooks, the day |
| `score.go`, `report.go` | Scoring and `results.md`; a transcript per run |
| `dry.go` | `-dry`: a rule-based fake model |
