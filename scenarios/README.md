# Scenarios

Scripted chats that test the agent on real models (SPEC 8.4, TASK-001). Each run builds a new project and uses the real orchestrator, system prompt and skills. Tools the app doesn't have yet are fakes from the set's files.

## Run

1. Copy `models.example.yaml` to `models.yaml` and list your providers. The file names the environment variable for each key, never the key.
2. Run:

```bash
go run ./cmd/jenab-scenarios run -env .env scenarios
```

- `-model ollama-cloud/gemma4:31b,zai/glm-4.5-flash` picks models. Without it, every model of a provider that isn't `paid` runs.
- `-scenario L01_news_view,L04_sql_fails` runs some scenarios only. `-reps 5` runs each one 5 times.
- `-scale 0.1` makes tool delays and message times shorter.
- `-baseline <report>.json` compares with that report instead of the newest one.
- `-model scripted:scenarios/skill-loading/_offline.yaml` plays scripted answers instead of a model, with no keys. It tests the runner and the app, not a model; its reports go to `<set>/results/scripted`.
- `check scenarios` loads every set without models, and `list scenarios` lists the scenarios.

Each set's report goes to `<set>/results/<time>.md`, with a JSON copy and one transcript per run. The report shows the changes since the last run, the tokens and the share read from the provider's cache. With `-reps 3` or more, it also shows the least and the most of a rep's pass share. Keys never appear in it.

## Sets

| Set | From | What it checks |
|---|---|---|
| `multi-part` | SPIKE-023 | Requests with several parts, messages during a turn, and Mother's delegation. Scenarios that need background work (8.3) are skipped until the app has it. |
| `skill-loading` | SPIKE-025 T-load | The chat loads the right skill before it acts, and no others. |
| `role-skills` | SPIKE-025 T-roles | Mother gives a new chat the skills its role needs. |
| `acceptance` | P1-18 | Phase 1's "done when": a chat of 25 turns with tool calls survives a restart, and turn 25 finds turn 3 with `search_history`. |

SPIKE-021's config tasks (T1–T4) come with Phase 2, when the app can check configs.

## Files

A set is a folder:

- `_set.yaml`: the project title, its card and fixture database, the fixture's day (`today`, shown as the date in block 1), stored configs, skills, other chats, the fake tools and their rules. `settings` holds `context` and `llm` keys of config.yaml, such as `llm: {turn_max_requests: 4}`. A set in another language gives `false_running`: regexes for text that says work goes on, for `no_false_running`.
- One YAML file per scenario: the context (`main` or `mother`), the messages (one with `at` is sent during a turn; one with `restart: true` comes after the app closes and opens again), `settings` over the set's, and the asserts. An assert's `turn` is `first`, `last`, `finish`, `any` or a turn number. `after_message` takes `when: during` or `between`, and fails when the message lands elsewhere.
- A file that starts with `_`, such as `_offline.yaml`, isn't a scenario. A script lists each scenario's replies: `text`, `calls` (`name`, `args`) and an optional `usage`.

A fake tool answers with the first rule whose `match` regex fits the call's arguments as JSON. The scenario's own rules come first. Tools of kind `sql`, `describe` and `config` read the fixture. An assert with `test: info` is reported but never fails a run.

The assert types are listed in `internal/scenario/assert.go`. `max_requests`, `no_turn_failed` and `cache_read_share` (`min`, a share from 0 to 1; `test: info` unless given) check the requests. A check that comes with a later feature, such as `background_started`, needs that feature in `needs`; every type is checked, even in a skipped scenario.
