# Tickets

| ID | Title | Type | Status | Gate |
|---|---|---|---|---|
| [SPIKE-001](SPIKE-001-sqlite-driver.md) | SQLite driver capabilities | Spike | Done | 2 |
| [SPIKE-002](SPIKE-002-library-survey.md) | Library survey | Spike | Done | 2 |
| [SPIKE-003](SPIKE-003-wails-version.md) | Wails v2 vs v3 | Spike | Done | 2 |
| [SPIKE-004](SPIKE-004-sleep-and-catch-up.md) | Sleep/wake detection and catch-up | Spike | In progress (Windows decided; macOS open) | 2 |
| [SPIKE-005](SPIKE-005-expr-sandbox.md) | expr-lang sandbox | Spike | Done | 2 |
| [SPIKE-006](SPIKE-006-keychain-background.md) | Keychain in background mode | Spike | In progress (Windows decided; macOS open) | 2 |
| [SPIKE-007](SPIKE-007-sql-guard.md) | SQL guard without an authorizer | Spike | Done | 2 |
| [SPIKE-008](SPIKE-008-crash-safety.md) | Crash safety | Spike | Done | 2 |
| [SPIKE-009](SPIKE-009-change-log-cost.md) | Change log and undo cost | Spike | Done | 2 |
| [SPIKE-010](SPIKE-010-wal-and-large-tables.md) | WAL growth and large tables | Spike | Done | 2 |
| [SPIKE-011](SPIKE-011-fts5-languages.md) | FTS5 for German and Persian text | Spike | Done | 2 |
| [SPIKE-012](SPIKE-012-llm-sdk-agent-loop.md) | LLM SDKs and the agent loop | Spike | Done | 2 |
| [SPIKE-013](SPIKE-013-yaml-and-json-schema.md) | YAML and JSON Schema | Spike | Done | 2 |
| [SPIKE-014](SPIKE-014-cron-parsing.md) | Cron parsing | Spike | Done | 2 |
| [SPIKE-015](SPIKE-015-readable-text.md) | Readable text from web pages | Spike | Done | 2 |
| [SPIKE-016](SPIKE-016-starlark-sandbox.md) | Script sandbox for `script.starlark` | Spike | Done | 2 |
| [SPIKE-017](SPIKE-017-local-model-ollama.md) | Local model: Qwen 3.5 4B on Ollama | Spike | Done | 2 |
| [SPIKE-018](SPIKE-018-cloud-models.md) | Cloud models for development: Groq, Ollama Cloud, Gemini and Z.ai | Spike | Done | 2 |
| [SPIKE-019](SPIKE-019-self-update.md) | Self-update with the Wails updater | Spike | Done | 2 |
| [SPIKE-020](SPIKE-020-keyless-search.md) | Keyless web search | Spike | Done | 2 |
| [SPIKE-021](SPIKE-021-models-write-configs.md) | Can the models write valid configs? | Spike | Done | 2 |
| [SPIKE-022](SPIKE-022-frontend-under-load.md) | Frontend under load | Spike | Done | 2 |
| [SPIKE-023](SPIKE-023-multi-part-requests.md) | Requests with several parts, and background work | Spike | Done | 3 |
| [TASK-001](TASK-001-scenario-runner.md) | Scenario runner | Task | Open | 3 |
| [TASK-002](TASK-002-high-fidelity-design.md) | High-fidelity design with shadcn/ui | Task | Done | 2 |
| [SPIKE-024](SPIKE-024-mcp-client.md) | MCP client | Spike | Open (before Phase 5) | 2 |
| [SPIKE-025](SPIKE-025-skills.md) | Do models load the right skills? | Spike | Done | 2 |


The spikes ran under the working name Burrow, so their code, results and transcripts in `spikes/` still use it (`_burrow_` tables, `burrow.lock`). The tickets use the final name, Jenab.

## Template

```markdown
# SPIKE-001 — Title
**Type:** Spike | Task | Feature | Bug
**Status:** Open | In progress | Done
**Gate:** 2

## Question
## Done when
## Result
## Decision
```

Spike code lives in `spikes/<number>-<name>/` at the repository root. It is throwaway code, kept as evidence.
