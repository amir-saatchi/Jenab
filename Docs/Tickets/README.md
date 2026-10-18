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
| [SPIKE-029](SPIKE-029-subjects.md) | Subjects: a chat's work, kept as objects | Spike | In progress (rounds 1–3 done; decisions proposed) | 3 |
| [SPIKE-030](SPIKE-030-structured-output.md) | Structured output: JSON that matches a schema, from any model | Spike | Done | 3 |
| [SPIKE-031](SPIKE-031-thoughts-graph.md) | Thoughts-Graph: a how-to task as a graph of model calls | Spike | Done | 3 |
| [SPIKE-032](SPIKE-032-made-up-numbers.md) | Catch made-up numbers and let the model correct them | Spike | In progress (round 1 done; round 2 later) | 3 |
| [TASK-003](TASK-003-broken-responses.md) | Broken responses: the same handling for every model | Task | Done | 3 |
| [TASK-004](TASK-004-gemini-native.md) | Gemini through its native API | Task | Done | 3 |
| [TASK-005](TASK-005-open-chat-memory.md) | What an open chat holds in memory | Task | Done | 3 |
| [TASK-006](TASK-006-stream-watchdog.md) | Long tool calls look like a stalled stream | Bug | Done | 3 |
| [TASK-007](TASK-007-tool-call-ids.md) | Tool-call IDs and arguments that every API accepts | Bug | Done | 3 |
| [TASK-008](TASK-008-thinking-refusals-overflow.md) | Thinking, refusals and context overflow in the provider layer | Bug | Done | 3 |
| [TASK-009](TASK-009-skill-text-twice.md) | Skill text is sent twice after a cut | Bug | Done | 3 |
| [TASK-010](TASK-010-request-cap-answer.md) | A turn at its request cap still answers | Bug | Done | 3 |
| [TASK-011](TASK-011-prompts-match-tools.md) | Prompts that match what the app can do | Task | Done | 3 |
| [TASK-012](TASK-012-untrusted-mark.md) | The untrusted mark and messages that join a turn | Task | Done | 3 |
| [TASK-013](TASK-013-scenario-runner-harness.md) | Scenario runner: measure the harness, not only the model | Task | Done | 3 |
| [TASK-014](TASK-014-agent-offers-missing-features.md) | The agent offers features the app doesn't have | Bug | Done | 3 |
| [TASK-015](TASK-015-files-panel.md) | The Files panel: browse and open the project's files | Task | Open | 3 |
| [TASK-016](TASK-016-export-chat.md) | Export a chat | Task | Open | 3 |
| [TASK-017](TASK-017-installer.md) | Install the app | Task | Open | 3 |
| [TASK-018](TASK-018-rewind-chat.md) | Rewind a chat to an earlier message | Task | Open | 3 |
| [TASK-019](TASK-019-reply-to-part.md) | Reply to part of a message | Task | Open | 3 |
| [TASK-020](TASK-020-tool-call-errors.md) | Tool-call errors: measure them, then check more | Task | Open (later) | 3 |

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
| [P1-18](P1-18-phase-1-acceptance.md) | Phase 1 acceptance | P1-15, P1-16, TASK-001 | In progress |
| [P1-19](P1-19-chat-types.md) | Chat types | P1-02 | Done |

## Phase 2

Phase 2 is structured memory (PROPOSAL 9). The data layer comes first (P2-01 to P2-05). It doesn't depend on SPIKE-028 or SPIKE-029, so those can be decided while it is built; their decisions shape P2-10. P2-07 holds SPIKE-021's and SPIKE-025's table checks that were due before Phase 2; their pipeline, view and page tasks move to Phases 3 and 4.

| ID | Title | Needs | Status |
|---|---|---|---|
| [P2-01](P2-01-sql-guard.md) | SQL guard | P1-03 | Done |
| [P2-02](P2-02-change-log.md) | Change log and typed writes | P1-03, P1-05 | Done |
| [P2-03](P2-03-undo-turn.md) | Undo turn | P2-02 | Open |
| [P2-04](P2-04-migrations.md) | Migrations | P2-01, P2-02 | Open |
| [P2-05](P2-05-data-tools.md) | Data tools: query, describe_table, insert | P2-01, P2-02 | Open |
| [P2-06](P2-06-schema-agent.md) | Schema agent | P2-04, P2-05 | Open |
| [P2-07](P2-07-schema-skills-and-checks.md) | Schema skills and model checks | P2-06, TASK-013 | Open |
| [P2-08](P2-08-project-card.md) | Project card | P2-04 | Open |
| [P2-09](P2-09-memory.md) | User and project memory | P2-02, P2-03 | Open |
| [P2-10](P2-10-session-notes.md) | Session notes | P1-06, P2-09, SPIKE-028, SPIKE-029 | Open |
| [P2-11](P2-11-refresh-memory.md) | Refresh memory | P2-09, P2-10 | Open |
| [P2-12](P2-12-export.md) | Export a project | P1-04 | Done |
| [P2-13](P2-13-benchmark.md) | Token and accuracy benchmark | P2-05, P2-09, P2-10, P2-11, TASK-013 | Open |
| [P2-14](P2-14-phase-2-acceptance.md) | Phase 2 acceptance | P2-01 to P2-13 | Open |

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
