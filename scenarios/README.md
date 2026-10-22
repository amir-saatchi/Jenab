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
- `-grounding mark -k 5` turns on SPIKE-032's number check for every run, over the sets' and scenarios' own: `mark`, `same_turn`, `next_turn`, or `off`. `-k` is how many numbers without a source make it fire; 0 is the default, 3.
- `check scenarios` loads every set without models, and `list scenarios` lists the scenarios.

Each set's report goes to `<set>/results/<time>.md`, with a JSON copy and one transcript per run. The report shows the changes since the last run, the tokens and the share read from the provider's cache. With `-reps 3` or more, it also shows the least and the most of a rep's pass share. Keys never appear in it.

## Sets

| Set | From | What it checks |
|---|---|---|
| `multi-part` | SPIKE-023 | Requests with several parts, messages during a turn, and Mother's delegation. Scenarios that need background work (8.3) are skipped until the app has it. |
| `skill-loading` | SPIKE-025 T-load | The chat loads the right skill before it acts, and no others. |
| `role-skills` | SPIKE-025 T-roles | Mother gives a new chat the skills its role needs. |
| `acceptance` | P1-18 | Phase 1's "done when": a chat of 25 turns with tool calls survives a restart, and turn 25 finds turn 3 with `search_history`. |
| `data-tools` | P2-05 | The real `query`, `describe_table` and `insert` on a ledger in the project's database: answers from the tables, a query the SQL guard refuses (an internal table) that the chat fixes, and a row added by hand. |
| `grounding` | SPIKE-032 | Numbers no source gave. The web gives part of what is asked (3 of 7 days, the close but not the open); a made-up answer fills in the rest. Honest answers with computed numbers, an example and Persian check the number check leaves them alone. |
| `missing-features` | TASK-014, P2-06 | A new project on an empty database: the data tools read tables and add rows, and `request_schema_change` makes tables through the schema agent, but there are no pipelines or views. Asked for a pipeline or a view, the agent says it can't do that yet: no SQL or scripts saved as the setup, no other place named where it could be done, no app screens. Asked for a table, it makes one with the columns asked for. A plain question and a note it can save check it doesn't refuse those either. |
| `project-card` | P2-08 | The real project card, built from the run's database. In one turn the chat makes a table through the schema agent and uses it at once, in a query or an insert, without `describe_table`: the card is rebuilt right after the migration. |

SPIKE-021's config tasks (T1–T4) come with Phase 2, when the app can check configs.

Runs from before P2-06 aren't comparable with later ones for `data-tools`, `grounding-honest` and `missing-features`: those sets now have the real `request_schema_change`, whose cards the runner approves, so the schema agent runs. Nor for `grounding` and `multi-part`: their chats have no `request_schema_change`, so `database-design`, which needs it, is no longer in their skills.

## Files

A set is a folder:

- `_set.yaml`: the project title, its card (or `project_card: true`, for the card the app builds from the run's project) and fixture database, `data` (a SQL script run on each run's own project database, for the real data tools), the fixture's day (`today`, shown as the date in block 1), stored configs, skills, other chats, the fake tools and their rules. `settings` holds `context` and `llm` keys of config.yaml, such as `llm: {turn_max_requests: 4}`. A set in another language gives `false_running`: regexes for text that says work goes on, for `no_false_running`. `grounding: {mode: mark, k: 3}` turns on SPIKE-032's number check (spike-only; the app has no setting for it); a scenario's `grounding` replaces the set's.
- One YAML file per scenario: the context (`main` or `mother`), the messages (one with `at` is sent during a turn; one with `restart: true` comes after the app closes and opens again), `settings` over the set's, and the asserts. An assert's `turn` is `first`, `last`, `finish`, `any` or a turn number. `after_message` takes `when: during` or `between`, and fails when the message lands elsewhere.
- A file that starts with `_`, such as `_offline.yaml`, isn't a scenario. A script lists each scenario's replies: `text`, `calls` (`name`, `args`) and an optional `usage`.

A fake tool's `areas` (`tables`, `pipelines`, `views`) are the parts of a project it works on; block 1 names only the parts some tool has. A fake of a real tool with no `description` or `schema`, such as `fetch_page`, gets the real one's. A fake tool answers with the first rule whose `match` regex fits the call's arguments as JSON. The scenario's own rules come first. Tools of kind `sql`, `describe` and `config` read the fixture. A set that fakes `query`, `describe_table` or `insert` gets none of the real ones. A set's skill replaces the built-in one of its name. An assert with `test: info` is reported but never fails a run.

The assert types are listed in `internal/scenario/assert.go`. `answer_lacks` fails when the answer matches `match`. `grounding_fired` and `grounding_quiet` pass when the number check fired, or didn't, on an answer of the selected turns before any notice; they are skipped with the check off, and a run's transcript and JSON have each verdict. `max_requests`, `no_turn_failed` and `cache_read_share` (`min`, a share from 0 to 1; `test: info` unless given) check the requests. `tool_called`, `tool_not_called` and `max_calls` take `result_match`, a regex the call's result must match too, and `result_line_all`, a list of regexes that must all match one line of that result, such as the five columns of one table in an applied schema change. A check that comes with a later feature, such as `background_started`, needs that feature in `needs`; every type is checked, even in a skipped scenario.
