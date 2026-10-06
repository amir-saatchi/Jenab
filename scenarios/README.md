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
- `check scenarios` loads every set without models, and `list scenarios` lists the scenarios.

Each set's report goes to `<set>/results/<time>.md`, with a JSON copy and one transcript per run. The report shows the changes since the last run. Keys never appear in it.

## Sets

| Set | From | What it checks |
|---|---|---|
| `multi-part` | SPIKE-023 | Requests with several parts, messages during a turn, and Mother's delegation. Scenarios that need background work (8.3) are skipped until the app has it. |
| `skill-loading` | SPIKE-025 T-load | The chat loads the right skill before it acts, and no others. |
| `role-skills` | SPIKE-025 T-roles | Mother gives a new chat the skills its role needs. |

SPIKE-021's config tasks (T1–T4) come with Phase 2, when the app can check configs.

## Files

A set is a folder:

- `_set.yaml`: the project title, its card and fixture database, stored configs, skills, other chats, the fake tools and their rules.
- One YAML file per scenario: the context (`main` or `mother`), the messages (one with `at` is sent during a turn), and the asserts.

A fake tool answers with the first rule whose `match` regex fits the call's arguments as JSON. The scenario's own rules come first. Tools of kind `sql`, `describe` and `config` read the fixture. An assert with `test: info` is reported but never fails a run.

The assert types are listed in `internal/scenario/assert.go`.
