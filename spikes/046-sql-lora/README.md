# SPIKE-046: SQL tasks, export and teacher runs

Part A of `Docs/Tickets/SPIKE-046-sql-lora.md`: the database tasks, the export of passing runs as training examples, and the notebook that runs the teacher in Colab.

## The task sets

`gen/` writes them:

```bash
go run ./spikes/046-sql-lora/gen          # writes the fixtures and the sets
go run ./spikes/046-sql-lora/gen -check   # fails if a written file is out of date
```

Edit the generator, not the sets. It writes:

- `scenarios/fixtures/{shop,clinic,bills,reading}/data.sql`: new fixtures, made from a fixed seed. Bitcoin, crypto-watch and the `data-tools` ledger reuse their existing data.
- One set per fixture, `scenarios/sql-<split>-<fixture>`, with the card the app builds (`project_card: true`).
- A `-short` set per fixture, with the card the app writes for a big project: table names only. The chat must call `describe_table` before it can write SQL.
- An `_offline.yaml` per set: scripted replies that pass. A scripted run checks every reference answer on Jenab's own database, with no keys.

Each task is one user message. The reference answer is worked out by running the task's reference SQL on the fixture. The checks are the runner's own asserts: `tool_called` with `args_match` and `result_match`, `answer_contains`, and `db_query` for inserts. There is no judge model.

Kinds of task: totals, averages and counts; joins; date ranges; filters and sorting; top-N; a table the card doesn't describe (`-short` sets); adding rows with `insert`; a request for an internal `_jenab_` table, which the SQL guard refuses; a question the data can't answer. Most tasks also have a Persian twin, a task of its own.

| Split | Fixture | Tasks (plain + short) |
|---|---|---|
| train | bitcoin | 28 + 5 |
| train | shop | 55 + 7 |
| train | clinic | 45 + 4 |
| train | bills | 41 + 4 |
| heldout | ledger (`data-tools`) | 17 + 3 |
| heldout | crypto-watch | 14 + 3 |
| heldout | reading | 20 + 4 |

That is 189 training tasks and 61 held-out tasks. More variants: add parameters to a template's list in `gen/fixtures.go`.

Each `_set.yaml` has `split: train` or `split: heldout`. The export refuses held-out reports. The older sets (`data-tools`, `local-basic`, `missing-features`, `schema-design` and the rest) have no split, so they never train either.

## Examples and the export

`-examples` on `run` records each request of a run as a chat-format example, next to its transcript (`<scenario>_r<rep>.examples.jsonl`). It adds nothing to the requests themselves.

```bash
go run ./cmd/jenab-scenarios run -examples -model <provider/model> scenarios/sql-train-shop
go run ./cmd/jenab-scenarios export -out export scenarios/sql-train-*/results
```

`export` reads the reports under the folders given and writes:

- `chat.jsonl`: the chat's requests;
- `subagents.jsonl`: the subagents' requests, such as the schema agent's.

One line is one request: `messages` (system, user, assistant with `tool_calls`, tool results with `tool_name`), `tools`, and the reply as the last assistant message. `meta` says the set, scenario, model, rep, agent, turn and request.

Rules:

- Only passing runs. Requests that ended other than with an answer or tool calls are left out.
- `-splits train` is the default. A report of another split stops the export.
- A run without its examples file stops the export: run it again with `-examples`.
- A scrub check looks for e-mail addresses, home folders, API keys, and this machine's own git name and e-mail, login and home folder. `-deny a,b` adds words. A hit writes nothing and names the rule, never the value.

A scripted try of the whole chain, with no model:

```bash
for s in scenarios/sql-train-*; do
  go run ./cmd/jenab-scenarios run -examples -model scripted:$s/_offline.yaml -out /tmp/smoke/$(basename $s) $s
done
go run ./cmd/jenab-scenarios export -out /tmp/smoke/export /tmp/smoke
```

On 2026-11-24 it gave 189 passing runs and 422 chat examples. The scripted replies call no subagent, so `subagents.jsonl` was empty.

## The teacher in Colab

`teacher.ipynb` runs the runner and `gemma4:31b` in the same Colab VM:

1. Settings. The first cell prints the GPU's name and memory and picks the teacher settings from it:

   | GPU | Teacher | Context | Runs at a time |
   |---|---|---|---|
   | A100 80 GB | `gemma4:31b-it-q8_0` (about 33 GB); `gpt-oss:120b` too with `SECOND_TEACHER = True` | 16k | 2 (up to 4) |
   | A100 40 GB | `gemma4:31b-it-q8_0` | 16k | 1 |
   | L4 24 GB (fallback) | `gemma4:31b-it-q4_K_M` | 8k | 1 |

   `*_OVERRIDE` settings change one choice, such as an 8k context if 16k runs out of memory. Then Go, a clone of the public repository (no `.env`) and the runner, built with `CGO_ENABLED=0`.
2. A scripted smoke of every training set and the export, with no GPU.
3. Ollama, connected as an ordinary `ollama` connection at `http://127.0.0.1:11434/`. No key. One model is loaded at a time.
4. A 10-run check on the first teacher that prints the time per run and an estimate for all training sets.
5. The training sets in chunks of 10 scenarios, one teacher after the other. Each chunk saves its report to Drive with a `DONE` file. After a disconnect, run the cells again: finished chunks are skipped.
6. The export to Drive.

The notebook clones `BRANCH` (`main`). The sets and the export must be on that branch first.
