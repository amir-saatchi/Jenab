# Tickets

The reports the ticket sessions wrote are in [reports/](reports/), linked from each ticket's result. What Jenab can learn from Codex CLI's source, with proposed tickets kept there until Phase 6 is closer: [reports/CODEX-LESSONS-report.md](reports/CODEX-LESSONS-report.md).

## Spikes and tasks

| ID | Title | Type | Status | Gate |
|---|---|---|---|---|
| [SPIKE-001](SPIKE-001-sqlite-driver.md) | SQLite driver capabilities | Spike | Done | 2 |
| [SPIKE-002](SPIKE-002-library-survey.md) | Library survey | Spike | Done | 2 |
| [SPIKE-003](SPIKE-003-wails-version.md) | Wails v2 vs v3 | Spike | Done | 2 |
| [SPIKE-004](SPIKE-004-sleep-and-catch-up.md) | Sleep/wake detection and catch-up | Spike | Done (Windows decided; macOS needs a Mac) | 2 |
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
| [SPIKE-024](SPIKE-024-mcp-client.md) | MCP client | Spike | Done | 2 |
| [SPIKE-025](SPIKE-025-skills.md) | Do models load the right skills? | Spike | Done | 2 |
| [SPIKE-026](SPIKE-026-command-limits.md) | How many commands can run at once? | Spike | Done on Windows | 3 |
| [SPIKE-027](SPIKE-027-decision-models.md) | Can a decision model make Jenab's small decisions? | Spike | Done | 3 |
| [SPIKE-028](SPIKE-028-request-list.md) | Loading tools and history when needed | Spike | Done (no index; history cards) | 3 |
| [SPIKE-029](SPIKE-029-subjects.md) | Subjects: a chat's work, kept as objects | Spike | Deferred (rounds 1–3 done) | 3 |
| [SPIKE-030](SPIKE-030-structured-output.md) | Structured output: JSON that matches a schema, from any model | Spike | Done | 3 |
| [SPIKE-031](SPIKE-031-thoughts-graph.md) | Thoughts-Graph: a how-to task as a graph of model calls | Spike | Done | 3 |
| [SPIKE-032](SPIKE-032-made-up-numbers.md) | Catch made-up numbers and let the model correct them | Spike | Done | 3 |
| [SPIKE-033](SPIKE-033-semantic-recall.md) | Semantic recall: find earlier messages and file passages by meaning | Spike | Done | 3 |
| [SPIKE-034](SPIKE-034-topics.md) | Topics: threads inside a chat, with their own notes and history | Spike | Deferred (rounds 1–2 done on gemma, ministral and Gemini) | 3 |
| [SPIKE-035](SPIKE-035-plan-first.md) | Plan first, as an approval check | Spike | Done (dropped for v1) | 3 |
| [SPIKE-036](SPIKE-036-plan-feature.md) | A plan the agent keeps, when the task needs one | Spike | Open (later) | 3 |
| [SPIKE-037](SPIKE-037-live-answer.md) | A live answer, and earlier turns as answers only | Spike | Done (live answer yes; window kept) | 3 |
| [SPIKE-038](SPIKE-038-local-models.md) | Local models on the user's PC | Spike | Done | 3 |
| [SPIKE-039](SPIKE-039-desktop-control.md) | Desktop control with agent-desktop | Spike | Done (go, narrow v1) | 3 |
| [SPIKE-040](SPIKE-040-desktop-macros.md) | Desktop macros and recipes | Spike | Done (go, Jenab records) | 3 |
| [SPIKE-041](SPIKE-041-edit-tool.md) | Edit tool: matching, shape and protected paths | Spike | Open (Phase 6) | 3 |
| [SPIKE-042](SPIKE-042-command-patterns.md) | Commands: patterns, approvals and output | Spike | Open (Phase 6) | 3 |
| [SPIKE-043](SPIKE-043-windows-write-sandbox.md) | Windows write sandbox without admin rights | Spike | Open (Phase 6) | 3 |
| [SPIKE-044](SPIKE-044-specialized-agents.md) | Specialized agents: tool groups and a custom prompt | Spike | Done (groups, decided 2026-11-14) | 3 |
| [SPIKE-045](SPIKE-045-subscribe-and-publish.md) | Subscribe and publish between agents | Spike | Done (decided 2026-11-15) | 3 |
| [SPIKE-046](SPIKE-046-sql-lora.md) | A LoRA for the database agent on lfm3:1.7b | Spike | Open (Part A and the pilot now; Part B after SPIKE-044) | 3 |
| [SPIKE-047](SPIKE-047-headless-browser.md) | A headless browser with agent-browser | Spike | Done (no-go for now) | 3 |
| [SPIKE-048](SPIKE-048-native-ui-mygo.md) | A native UI with MyGo | Spike | Done (go, decided 2026-11-16) | 3 |
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
| [TASK-015](TASK-015-files-panel.md) | The Files panel: browse and open the project's files | Task | Done | 3 |
| [TASK-016](TASK-016-export-chat.md) | Export a chat | Task | Done | 3 |
| [TASK-017](TASK-017-installer.md) | Install the app | Task | Open | 3 |
| [TASK-018](TASK-018-rewind-chat.md) | Rewind a chat to an earlier message | Task | Done | 3 |
| [TASK-019](TASK-019-reply-to-part.md) | Reply to part of a message | Task | Done | 3 |
| [TASK-020](TASK-020-tool-call-errors.md) | Tool-call errors: measure them, then check more | Task | Open (later) | 3 |
| [TASK-021](TASK-021-word-files.md) | Read Word files: text from .docx | Task | Done | 3 |
| [TASK-022](TASK-022-retry-skipped-undo.md) | Undo the skipped items later | Task | Done | 3 |
| [TASK-023](TASK-023-wails-3-stable.md) | Move to Wails v3.0.0 and Go 1.26.10 | Task | Done | 3 |
| [TASK-024](TASK-024-spike-037-part-b-out.md) | Move SPIKE-037's Part B out of the app | Task | Done | 3 |
| [TASK-025](TASK-025-live-answer.md) | A live answer | Task | Open (needs finish turns) | 3 |
| [TASK-026](TASK-026-memory-edit-after-untrusted.md) | Ask before a memory edit after untrusted content | Task | Done | 3 |
| [TASK-027](TASK-027-history-cards.md) | History cards after a cut | Task | Done | 3 |
| [TASK-028](TASK-028-memory-mark-in-window.md) | Keep the untrusted mark for memory edits while the content is in the window | Task | Done | 3 |
| [TASK-029](TASK-029-provider-json-mode.md) | Each provider's JSON mode for schema answers | Task | Done | 3 |
| [TASK-030](TASK-030-memory-saves-without-asking.md) | Memory saves without asking | Task | Done | 3 |
| [TASK-031](TASK-031-history-cards-paraphrases.md) | History cards for paraphrased follow-ups | Task | Open (later) | 3 |
| [TASK-032](TASK-032-truncation-check-without-tools.md) | Truncation check without tool definitions | Task | Done | 3 |
| [TASK-033](TASK-033-less-fixed-context.md) | Less fixed context per request | Task | Done | 3 |
| [TASK-034](TASK-034-fake-price-server.md) | A fake price site for manual runs | Task | Done | 3 |
| [TASK-035](TASK-035-desktop-control.md) | Desktop control v1 (Windows) | Task | Done | 3 |
| [TASK-036](TASK-036-private-network-exceptions.md) | Ask before reaching this computer or the local network | Task | Done | 3 |
| [TASK-037](TASK-037-app-version.md) | One version number for the app | Task | Done | 3 |
| [TASK-038](TASK-038-desktop-macros.md) | Desktop macros and recipes | Task | Done | 3 |
| [TASK-039](TASK-039-agent-desktop-1-2.md) | agent-desktop 1.2.0 | Task | Done | 3 |
| [TASK-040](TASK-040-chat-bubble-fixes.md) | Chat bubble fixes: links in the user's message, wide answers | Task | Done | 3 |
| [TASK-041](TASK-041-one-card-private-host.md) | One card for a new private host | Task | Done | 3 |
| [TASK-042](TASK-042-shared-scenario-failures.md) | Scenario failures every model shares | Task | Done | 3 |
| [TASK-043](TASK-043-more-shared-failures.md) | More scenario failures both reference models share | Task | Done | 3 |
| [TASK-044](TASK-044-parallel-read-calls.md) | Run read-only tool calls of one answer together | Task | Open | 3 |
| [TASK-045](TASK-045-stopped-call-wording.md) | Say when a stopped call may have run | Task | Open | 3 |
| [TASK-046](TASK-046-thinking-binding.md) | Thinking blocks after the context changes | Task | Done | 3 |
| [TASK-047](TASK-047-steady-prefix.md) | Keep the start of each request the same | Task | Done | 3 |
| [TASK-048](TASK-048-runner-quota-stop.md) | The scenario runner stops a model when its quota is gone | Task | Done | 3 |
| [TASK-049](TASK-049-bucket-search-words.md) | Search the project's files by words (`bucket_search`, R-128) | Task | Open | 3 |
| [TASK-050](TASK-050-settings-findings.md) | Find Project settings and the project's memory | Task | Done | 3 |
| [TASK-051](TASK-051-query-wording.md) | The `query` tool points to the card, not `sqlite_master` | Task | Done | 3 |
| [TASK-052](TASK-052-provider-select.md) | Pick the provider from a list in *Add provider* | Task | Done | 3 |
| [TASK-053](TASK-053-native-ui-docs.md) | The SPEC and design docs for the native UI | Task | Done | 3 |
| [TASK-054](TASK-054-native-chat-gaps.md) | Close the native chat's gaps | Task | Done | 3 |
| [TASK-055](TASK-055-native-settings.md) | Settings in the native app | Task | Done | 3 |
| [TASK-056](TASK-056-native-files-and-dock.md) | The files panel, the file view and the dock in the native app | Task | Done | 3 |
| [TASK-057](TASK-057-native-project-screens.md) | Project screens in the native app | Task | Done | 3 |
| [TASK-058](TASK-058-native-developer-tools.md) | Developer tools in the native app | Task | Done | 3 |
| [TASK-059](TASK-059-native-updater-installer.md) | Updates and the installer for the native app | Task | Done | 3 |
| [TASK-060](TASK-060-native-macos-linux.md) | The native app on macOS and Linux | Task | Open (waits for CI and a Mac) | 3 |
| [TASK-061](TASK-061-remove-wails.md) | Switch to the native app and remove Wails | Task | Done | 3 |
| [TASK-062](TASK-062-native-leftovers.md) | The native app's leftovers before the switch | Task | Done | 3 |
| [TASK-063](TASK-063-native-title-bar.md) | The native app's own title bar | Task | Done | 3 |
| [TASK-064](TASK-064-native-link-safety.md) | Links in Markdown open only safe addresses | Task | Done | 3 |
| [TASK-065](TASK-065-native-chat-parity.md) | The native chat's memory chips, *Refresh memory* and Connect card | Task | Done | 3 |
| [TASK-066](TASK-066-native-state-fixes.md) | The native stores: lost updates and stale reads | Task | Done | 3 |
| [TASK-067](TASK-067-native-crash-and-updates.md) | Crash log, and updates that wait for all work | Task | Done | 3 |
| [TASK-068](TASK-068-native-settings-fixes.md) | Settings forms: keys, dialogs and the data folder | Task | Done | 3 |
| [TASK-069](TASK-069-native-polish.md) | Small native fixes: digits, text direction, tooltips, old comments | Task | Done | 3 |
| [TASK-070](TASK-070-wails-leftovers.md) | Remove the code and text the Wails app left behind | Task | Done | 3 |
| [TASK-071](TASK-071-connect-card-and-chips.md) | The Connect card keeps the message; memory chips' Undo and Redo | Task | Done | 3 |
| [TASK-072](TASK-072-stores-stale-reads-2.md) | The native stores: the hosts' counter, the rewind dialog, Ctrl+Tab and the Dev page | Task | Done | 3 |
| [TASK-073](TASK-073-crash-and-updates-2.md) | Updates during a download, and the crash log's raw file | Task | Done | 3 |
| [TASK-074](TASK-074-local-provider-token.md) | Replace the token of a provider on this computer | Task | Done | 3 |
| [TASK-075](TASK-075-heading-links-and-cell-tooltips.md) | Heading links in repeated text, and a cut number cell's tooltip | Task | Done | 3 |
| [TASK-076](TASK-076-review-leftovers.md) | Three small leftovers from the review: crash redaction, Undo turn's state, number fields | Task | Done | 3 |
| [TASK-077](TASK-077-connect-retry-and-chip-reads.md) | The Connect card's refused retry, and a chip's read during its own undo | Task | Done | 3 |
| [TASK-078](TASK-078-shapes-crash-start-and-hosts.md) | Key shapes in escaped JSON, the crash redactor at start, and overlapping host changes | Task | Done | 3 |
| [TASK-079](TASK-079-host-edits-under-the-lock.md) | A card's host lost to a page change | Task | Done | 3 |
| [TASK-080](TASK-080-retried-turn-notice.md) | What the model reads of a failed turn's notice | Task | Done | 3 |
| [TASK-081](TASK-081-prune-hooks-stamp.md) | Pruning keeps what came after the chat list's read | Task | Done | 3 |
| [TASK-082](TASK-082-mygo-0-10.md) | Move to MyGo v0.10.0 | Task | Done | 3 |
| [TASK-083](TASK-083-file-view-player.md) | Play audio and video in the file view | Task | Open | 3 |

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

Phase 2 is structured memory (PROPOSAL 9). The data layer comes first (P2-01 to P2-05). SPIKE-028 (loading tools and history when needed) can be decided while it is built. The request list moved to SPIKE-036 (later), and SPIKE-029 (subjects) and SPIKE-034 (topics) are deferred, so P2-10 waits for none of them. P2-07 holds SPIKE-021's and SPIKE-025's table checks that were due before Phase 2; their pipeline, view and page tasks move to Phases 3 and 4.

| ID | Title | Needs | Status |
|---|---|---|---|
| [P2-01](P2-01-sql-guard.md) | SQL guard | P1-03 | Done |
| [P2-02](P2-02-change-log.md) | Change log and typed writes | P1-03, P1-05 | Done |
| [P2-03](P2-03-undo-turn.md) | Undo turn | P2-02 | Done |
| [P2-04](P2-04-migrations.md) | Migrations | P2-01, P2-02 | Done |
| [P2-05](P2-05-data-tools.md) | Data tools: query, describe_table, insert | P2-01, P2-02 | Done |
| [P2-06](P2-06-schema-agent.md) | Schema agent | P2-04, P2-05 | Done |
| [P2-07](P2-07-schema-skills-and-checks.md) | Schema skills and model checks | P2-06, TASK-013 | Done |
| [P2-08](P2-08-project-card.md) | Project card | P2-04 | Done |
| [P2-09](P2-09-memory.md) | User and project memory | P2-02, P2-03 | Done |
| [P2-10](P2-10-session-notes.md) | Session notes | P1-06, P2-09 | Done |
| [P2-11](P2-11-refresh-memory.md) | Refresh memory | P2-09, P2-10 | Done |
| [P2-12](P2-12-export.md) | Export a project | P1-04 | Done |
| [P2-13](P2-13-benchmark.md) | Token and accuracy benchmark | P2-05, P2-09, P2-10, P2-11, TASK-013 | Done |
| [P2-14](P2-14-phase-2-acceptance.md) | Phase 2 acceptance | P2-01 to P2-13 | Done |

## Phase 3

Phase 3 is views and pages (PROPOSAL 9, SPEC 5). The Go side comes first: storage (P3-01), validation and the block registry (P3-02), then the tools (P3-03), migrations (P3-04), running views (P3-05) and their writes (P3-06). The kit blocks (P3-07, P3-08) take plain Go data, so they can start at once. The page screen (P3-09) and the shell (P3-10) join them, and the Links tab (P3-11) needs nothing else; the agent's `save_link` tool (P3-14) follows it. `run_pipeline` actions wait for Phase 4.

| ID | Title | Needs | Status |
|---|---|---|---|
| [P3-01](P3-01-config-storage.md) | Views and pages: storage, change log and undo | P2-02, P2-03, P2-04 | Done |
| [P3-02](P3-02-config-validation.md) | Validation of views and pages, and the block registry | P2-01, P3-01 | Done |
| [P3-03](P3-03-config-tools.md) | Agent tools for views and pages | P3-01, P3-02 | Done |
| [P3-04](P3-04-migrations-and-dependents.md) | Migrations keep views and forms working | P3-01, P3-02, P2-06 | Done |
| [P3-05](P3-05-view-runtime.md) | Running views: queries, filters, tables, charts and stats | P3-01, P3-02 | Done |
| [P3-06](P3-06-view-writes.md) | Forms and row actions write data | P3-05 | Open |
| [P3-07](P3-07-table-stat-filter-blocks.md) | Kit blocks: table, stat and filters | – | Done |
| [P3-08](P3-08-chart-form-blocks.md) | Kit blocks: chart and form | – | Done |
| [P3-09](P3-09-page-screen.md) | The page screen | P3-05 to P3-08 | Open |
| [P3-10](P3-10-pages-in-the-shell.md) | Pages in the shell: the Pages tab, the dock and the layout per chat | P3-09, P3-03 | Open |
| [P3-11](P3-11-links-tab.md) | The Links tab | P1-05 | Done |
| [P3-12](P3-12-page-speed.md) | Page speed: six blocks in under 1.5 s | P3-09 | Open |
| [P3-13](P3-13-phase-3-acceptance.md) | Phase 3 acceptance | P3-01 to P3-12, P3-14 | Open |
| [P3-14](P3-14-save-link-tool.md) | The `save_link` tool | P3-11 | Open |

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
