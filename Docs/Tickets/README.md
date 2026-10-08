# Tickets

## Spikes and tasks

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
| [TASK-001](TASK-001-scenario-runner.md) | Scenario runner (Phase 1; needs P1-10) | Task | Done | 3 |
| [TASK-002](TASK-002-high-fidelity-design.md) | High-fidelity design with shadcn/ui | Task | Done | 2 |
| [SPIKE-024](SPIKE-024-mcp-client.md) | MCP client | Spike | Open (before Phase 5) | 2 |
| [SPIKE-025](SPIKE-025-skills.md) | Do models load the right skills? | Spike | Done | 2 |
| [SPIKE-026](SPIKE-026-command-limits.md) | How many commands can run at once? | Spike | Done on Windows | 3 |
| [SPIKE-027](SPIKE-027-decision-models.md) | Can a decision model make Jenab's small decisions? | Spike | Done | 3 |
| [SPIKE-028](SPIKE-028-request-list.md) | A request list, and loading tools and history when needed | Spike | Open (before Phase 2) | 3 |
| [SPIKE-029](SPIKE-029-subjects.md) | Subjects: a chat's work, kept as objects | Spike | Open (before Phase 2) | 3 |
| [SPIKE-030](SPIKE-030-structured-output.md) | Structured output: JSON that matches a schema, from any model | Spike | Done | 3 |
| [TASK-003](TASK-003-broken-responses.md) | Broken responses: the same handling for every model | Task | Open | 3 |
| [TASK-004](TASK-004-gemini-native.md) | Gemini through its native API | Task | Open | 3 |

## Phase 1

Phase 1 tickets are named `P1-<nn>`. *Needs* lists the tickets that must be done first. The frontend tickets (P1-14 to P1-16) can start earlier against fake services, and P1-19 and then P1-08 can run alongside the store work. P1-19 was added later, so its number doesn't follow the build order.

| ID | Title | Needs | Status |
|---|---|---|---|
| [P1-01](P1-01-repository-and-ci.md) | Repository skeleton and CI | – | In progress |
| [P1-02](P1-02-shared-packages.md) | Shared packages: id, config, secret, limit, logfile | P1-01 | Done |
| [P1-03](P1-03-store.md) | Store: writers, readers and migrations | P1-02 | Done |
| [P1-04](P1-04-projects.md) | Projects: folders, lock and recovery | P1-03 | Done |
| [P1-05](P1-05-bucket.md) | Bucket | P1-04 | Done |
| [P1-06](P1-06-chats.md) | Chats: storage, Mother chat, titles and roles | P1-04, P1-19 | Done |
| [P1-07](P1-07-history-search.md) | History search | P1-06 | Done |
| [P1-08](P1-08-providers.md) | Providers and the model catalog | P1-02, P1-19 | Done |
| [P1-09](P1-09-tools.md) | Tools, web pages and refs | P1-05, P1-07 | Done |
| [P1-10](P1-10-agent-loop.md) | Agent loop | P1-06, P1-08, P1-09 | Done |
| [P1-11](P1-11-approvals.md) | Approvals and questions | P1-10 | Done |
| [P1-12](P1-12-skills.md) | Skills | P1-10 | Done |
| [P1-13](P1-13-wails-layer.md) | Wails layer | P1-10, P1-11 | Done |
| [P1-14](P1-14-app-shell.md) | Frontend base and app shell | P1-13 | Done |
| [P1-15](P1-15-chat-ui.md) | Chat UI | P1-14, P1-11 | Done |
| [P1-16](P1-16-settings.md) | Settings: models, keys and usage | P1-14, P1-08 | Done |
| [P1-17](P1-17-developer-tools.md) | Turn inspector and runtime panel | P1-15 | Done |
| [P1-18](P1-18-phase-1-acceptance.md) | Phase 1 acceptance | P1-15, P1-16, TASK-001 | Open |
| [P1-19](P1-19-chat-types.md) | Chat types | P1-02 | Done |

## Notes

The spikes ran under the working name Burrow, so their code, results and transcripts in `spikes/` still use it (`_burrow_` tables, `burrow.lock`). The tickets use the final name, Jenab.

## Template

Spikes:

```markdown
# SPIKE-001 — Title
**Type:** Spike
**Status:** Open | In progress | Done
**Gate:** 2

## Question
## Done when
## Result
## Decision
```

Features and tasks:

```markdown
# P1-01 — Title
**Type:** Feature | Task | Bug
**Status:** Open | In progress | Done
**Gate:** 3 (Phase 1)
**Needs:** other tickets
**Requirements:** R-xx, N-xx (Gate 1)

## Goal
## Scope
## Done when
## Result
```

Spike code lives in `spikes/<number>-<name>/` at the repository root. It is throwaway code, kept as evidence.
