# SPIKE-023: requests with several parts, background work and Mother's delegation

Throwaway code for [SPIKE-023](../../Docs/Tickets/SPIKE-023-multi-part-requests.md). It is a first small version of the scenario runner ([TASK-001](../../Docs/Tickets/TASK-001-scenario-runner.md)): an orchestrator loop that follows the SPEC 8.3 draft, fake tools, YAML scenarios and assertions, run on the development models with and without the SPEC 8.3 prompt rule.

```bash
go test ./...                               # offline checks of the orchestrator and assertions (scripted model, no network)
go run . -list                              # scenarios and the four system prompts, no network
go run . -reps 1,2 -budget 2100000          # all scenarios x both conditions x all models, reps 1 and 2
go run . -scen t6_ -conds rule -reps 1 -models gemma4:31b   # a subset (IDs or ID prefixes)
go run . -report                            # rewrite the measured part of results.md from results/runs.jsonl
go run . -scan                              # secret scan of this folder (prints file names only)
```

Runs append to `results/runs.jsonl`. A combination (model, scenario, condition, rep) that is already there without an error is skipped, so an interrupted run can be restarted with the same command. The prompt-token budget counts everything in `results/runs.jsonl`, `results/smoke.jsonl` and `results/superseded.jsonl`; the program stops about 15k tokens below `-budget`.

## Keys and safety

- `OLLAMA_API_KEY` and `Z_API_KEY` come from the repo-root `.env` (git-ignored). Only `loadEnv` (`env.go`, from SPIKE-018/021) reads it; values stay in memory.
- Keys go only into the `Authorization` header, and only to `ollama.com` and `api.z.ai`. The client middleware refuses any other host and strips headers that are not on an allow-list; `OPENAI_*` variables are removed from the process environment.
- Everything written (runs, transcripts, logs) goes through the redaction in `env.go`. `-scan` checks the folder for key values.
- Only synthetic prompts are sent: the scenario texts, the fixture project card and chat list, and fake tool results. The fake tools never touch the network or the disk (the project database is in-memory SQLite).

## How it works

- **Orchestrator (`orch.go`):** a turn is a list of parts (text, tool calls, tool results, user messages, notices). Time is virtual: each model request costs 4 s, each tool call its configured delay, so a 30 s pipeline run takes no real time but ordering is real.
  - `run_pipeline`, `subagent(background: true)`, `create_chat` and `send_to_chat` return an ID at once and finish at start + delay (at most 3 at once per chat).
  - A finished task starts a new **system-started turn** with a notice part (`[run r_1 finished 12:00:34: ...]`). If a turn is running, the notice waits until it ends; notices that are due together share one turn. A task whose final status the agent already read with `run_status`/`task_status` gets no notice.
  - A scripted user message with `at` is added to the running turn at the next step boundary: after the current tool calls (OpenAI message order does not allow a user message between a tool call and its result). If the model's response has no tool calls, the turn ends and the message starts a new turn. Messages without `at` are sent when the chat is idle.
  - Caps: 8 model requests per turn, 8 turns per scenario, max output 8,192 tokens per request in user turns and 4,096 in system-started turns.
- **Fixture (`fixture.go`, `prompts/card.md`, `prompts/chats.md`):** the "Crypto watch" project: `prices` (60 BTC rows) and `news` (12 rows) in in-memory SQLite, the pipelines `daily_prices`, `backfill_prices`, `fear_greed` (new, deliberately without `on_error`), a `coingecko` connection, and for Mother a chat list with *Reviewer*, *ETH tracker* and *Project setup*.
- **Tools (`tools.go`):** `query`, `describe_table`, `read_config`, `run_pipeline`, `run_status`, `web_search`, `fetch_page`, `call_api`, `subagent`, `task_status`; Mother also gets `list_chats`, `create_chat`, `send_to_chat`. Results of `web_search`, `fetch_page`, `subagent`, `send_to_chat` and `create_chat` come from regex rules in `scenarios/_defaults.yaml` (a scenario can override them and the delays).
- **Scenarios (`scenarios/*.yaml`):** `t12_*` (tests 1 and 2), `t3_*`, `t4_*`, `t6_*` (Mother). Each has scripted messages (optional `at`), optional delays/rules, and assertions with the test they belong to (`info` = reported only). Every scenario also gets `max_prompt_tokens` 8,000 (test 5).
- **Assertions (`assert.go`):** `quick_before_finish`, `slow_started`, `text_before_tool`, `background_started`, `mentions_running`, `answer_contains`, `no_repeat`, `no_restart`, `after_message`, `tool_called`, `tool_not_called`, `max_calls`, `no_background`, `max_turns`, `no_false_running`, `max_prompt_tokens`, `right_target`, `created_chat_only_for_lasting_job`, `used_subagent_for_one_off`, `no_delegation_when_simple`, `no_resend_after_notice`. `results.md` defines each one.
- **Conditions:** `base` = base system prompt (`prompts/base.md`, plus `prompts/mother.md` in Mother); `rule` = the same plus the SPEC 8.3 rule (`prompts/rule.md`) and, in Mother, the delegation guidance (`prompts/delegation.md`). `rule2` (follow-up, Mother only) = `rule` with `prompts/delegation_v2.md` instead of `prompts/delegation.md` (`-conds rule2`). Everything else is identical, and identical for every model.
- **Providers (`providers.go`, `stream.go`, `env.go`):** copied from SPIKE-021 unchanged in behaviour (Ollama Cloud and Z.ai, temperature 0.2, provider-default reasoning effort, our own retry loop). Ollama models run one after another (free plan: one request at a time); Z.ai runs in its own lane.

## Output

- `results/runs.jsonl`: one record per run: all turns with their parts and virtual times, per-request tokens and real seconds, background tasks, assertion results, per-test status.
- `results/transcripts/<scenario>_<cond>_<model>_r<rep>.txt`: readable transcripts (the last run of each combination).
- `results/smoke.jsonl`: the first smoke runs (counted in the budget, not in the results).
- `results/superseded.jsonl`: the 12 t3_d runs with the first fixture (v1), replaced after a fixture fix (see results.md); counted in the budget, not in the tables.
- `results/phase*.log`: console logs.
- `results.md`: hand-written findings above the marker, generated tables below it.
