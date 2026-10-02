# Spec v0.6 — Project Storage, Context, Views and Pipelines

> **v0.6:** API connections: saved APIs with the user's key, `connect_api`, `call_api` and `api.get`; secrets are used only through connections (6.5, 6.7, 6.9, 8.1); read-only workspace with `list_files`, `read_file` and `search_code`, and the plan for code tools later (8.5); connections and workspace in the project card (3.2) and `registry.db` (2.4); Mother chat, chat titles and roles, `list_chats`, `create_chat` and `send_to_chat` (2.3, 3.1, 8.1, 8.3, 8.6); MCP servers, draft (8.7); layout per chat, the open-page notice and `open_page` (5.9); page header and the `card`, `filter`, `button` and `image` blocks (5.9, 5.10); `set` and `bulk` row actions and the `badge` format (5.3); theme and tones (5.11); approval card, question form and `ask_user`, approval levels (8.8); skills (3.1, 8.1, 8.9), decided in SPIKE-025; *Refresh memory* replaces *Recheck memory*: it applies edits, writes a summary to the session notes and ends with a cut (3.1, 3.3–3.6); model catalog and first run without a wizard (3.9); pipeline page with Flow and Steps views, and *Pause schedule* (6.10); model picker per chat (3.9); app shell, project settings and window sizes (5.12); the waiting bar above the composer (8.8); starter prompts (3.9); Markdown styling with shadcn Typeset (5.8); right-to-left text (5.8); SPIKE-023 decisions: the prompt rule, status polling, `turn_max_requests`, messages during a turn and batched notices (7.6, 8.3), and Mother's delegation guidance (8.6); Gate-2 high-fidelity decisions: Mother's gradient colour (8.6), the folded sidebar rail, a resizable dock and Settings as a full view (5.12), `remark-gfm`, explicit list and table directions and the Vazirmatn font (5.8), *Settings → Usage*, *Other models*, *Open the example* and Ollama first when it runs (3.9), and *Inspect* in the turn footer (8.4); Phase 1 providers: Anthropic, OpenAI, Gemini, OpenAI-compatible and Ollama (3.9); web search with Tavily or SearXNG, Brave later (6.5); `thinking` message parts, stored with their signature so they can be sent back unchanged (2.3, 3.8); running background tasks in Mother's chat list (8.6); the Runtime panel in the developer tools (8.4); what *Stop* cancels, open tool calls closed as cancelled, and kept partial text (8.3); cancelled writes never commit (7.3); LLM stall timeouts (3.8); one source format for who did something, in the change log, chats, objects, links and approvals, with `app` for the Mother chat (2.3, 2.5, 4.2, 4.6, 8.8); subagents: one level, transcripts in the bucket, changes undone with the parent turn (8.1, 8.6); the app is named Jenab, so internal tables start with `_jenab_` and the lock file is `jenab.lock` (was the working name Burrow); Linux as a beta, shipped as an AppImage (2.8); user settings in `config.yaml` instead of `config.toml`, so one YAML library reads every config and comments survive saves (2.1); app files stay in `<user data dir>/Jenab` and only projects move with the data folder (2.1); limits split from provider pressure: chat calls never wait for a slot, `max_parallel_calls` 8 for background calls, a limit per provider, `max_subagents_per_chat` 5, limits change without a restart (7.6), provider error kinds, waits and one pause per provider (3.8), and who retries what (6.5, 8.3); planned limits for `run_command`: the agent states each command's memory need and the app checks it at the start, a hard stop at 500 MB, high count limits as a safety net, and below-normal priority (8.5, SPIKE-026); a rail separate from the left sidebar, with projects, *Waiting*, schedules and Settings, a bottom bar with work, commands, pipelines, context use, memory and provider problems, no tabs, and chat shortcuts (5.12).
>
> **v0.5:** model-neutral rule (1, 3.8); dependents in the project card (3.2); query options for view filters (5.2); page format and virtual rows for tables (5.3); chart size rules (5.4); frontend rules (5.8); exact `for_each` rules (6.3) and `flatten` (6.4); `feed.read`, search sources and search providers, no keyless web search (6.5, 8.1); `read_feed` and `read_config` tools (8.1); `op` field for migration steps and full dependent configs for the schema agent (8.2); turns and background work, draft (8.3); scenario runner and turn inspector (8.4); pages built from blocks, a block registry and where pages open (5.9, 5.10), pie charts (5.4) and `stat` views (5.10); corrected Bitcoin example with news from feeds, a news view and a new news key (9); repair loop (10).
>
> **v0.4:** agent SQL guard that does not depend on a driver's authorizer (2.2); rules for agent tables: explicit primary key, no triggers or SQL views (2.2); indexed row keys and per-row before-images in the change log, 90-day undo window (2.5, 2.6); structured migration steps instead of agent-written DDL, plus a schema guard before commit (7.5, 8.2); bucket write order for crash safety (4.3); file bytes kept for the undo window, with the cost shown (4.4); WAL checkpoint, cache and temp-file rules (7.2); expression sandbox rules (6.4); keychain rules (6.7); rules for long migrations and tries (7.5); warning for cloud-synced folders (2.1); crash recovery on open and the `interrupted` run status (2.7, 6.2); message-before-change order (2.3); safe-copy rule for `VACUUM INTO` (7.5); full-text search rules for English, German and Persian (2.3); cron syntax, day matching, DST and time-zone rules, and the wall-clock check with resume events (6.2); Starlark sandbox limits, with `db.query` through the parent and the memory cap on every OS (6.5); config parsing rules (1) and the schema check (10); readable-text pipeline for web pages (3.7); provider layer rules, with truncation, Ollama and OpenAI-compatible provider rules (3.8); app updates and the storage format version (2.8).
>
> **v0.3:** added project storage, change log and undo (section 2), context and memory (3), bucket and links (4), agent tools and the schema agent contract (8). Changed: table pagination and sorting, filter defaults, update forms, row actions, `upsert` replaces `replace`, `llm.select` returns original items, secret and network rules, catch-up for missed runs, re-validation instead of failing on `schema_changed`, migrations in one transaction, validation of view columns against query results.
>
> **v0.2:** added concurrency and database access: async data layer, short write transactions, batched inserts, write priorities, schema-version checks for running pipelines.

This spec defines:

- how a project is stored on disk (section 2)
- what the agent sees each turn and how memory works (3)
- the per-project bucket (4)
- the formats the LLM generates most often: **views and pages** (5) and **pipelines** (6)
- concurrency rules (7) and the agent's tools (8)

Rules of thumb: **views read, pipelines write. Tables hold data, memory holds intent.** Views never call the web or an LLM. Pipelines never render UI. Data never has to pass through the LLM's context to reach the database.

---

## 1. Conventions

- Every config has `version: 1`. The runner rejects unknown versions.
- Configs are stored as JSON internally and shown/edited as YAML. Both parse to the same structure.
- **Config parsing** (SPIKE-013):
  - **Library:** YAML is read with `go.yaml.in/yaml/v3` as a node tree. Jenab's own loader builds the value and records a line:column for every JSON path.
  - **Rejected:**
    - duplicate keys (in YAML and in JSON)
    - anchors, aliases, merge keys (`<<`) and tags
    - keys that are not strings
    - `NaN` and `Inf`
    - nesting deeper than 64
    - files over the size cap
  - **Values:**
    - `yes`, `no`, `on` and `off` stay strings.
    - Dates stay text.
    - Numbers with a leading zero, such as `0755`, are rejected; the fix is to quote them.
  - **Display:** JSON is shown as YAML with the node styles reset, and multi-line strings (SQL) are shown as `|` blocks. The round trip gives back the same JSON.
- IDs are `snake_case` and unique within a project.
- Every config is validated before it is saved (section 10). Invalid configs are never stored.
- Validation errors are returned to the LLM with a path, e.g. `steps[2].with.table: table "btc_price" not found (did you mean "btc_prices"?)`.
- Dates are stored as ISO 8601 text: dates as `YYYY-MM-DD`, timestamps as RFC 3339 in UTC (`2026-09-27T08:00:03Z`).
- Durations use Go syntax (`500ms`, `60s`, `5m`, `2h`). Longer periods in lifecycle rules, filter defaults and `feed.read` `since` use `d`, `m`, `y` (days, months, years).
- Internal tables start with `_jenab_`. The agent cannot see or query them (2.2).
- Agent tool names use underscores (`read_ref`), because LLM APIs do not allow dots in tool names. Pipeline step names use dots (`http.get`).
- **Model-neutral** (SPIKE-021):
  - Prompts, guides, tools, schemas, error messages and the repair loop are the same for every model.
  - Nothing above the provider layer (3.8) knows which model runs.
  - A guide or schema change is kept only if it helps, or is neutral, on every tested model.

---

## 2. Project storage

### 2.1 Layout

```
<user data dir>/Jenab/        app files, always here
  config.yaml                 user settings; secrets stay in the OS keychain
  registry.db                 project list and user memory
  logs/                       jenab.log, rotated (Q36)
  snapshots/                  registry.db backups before a format update (2.8)
<data folder>/                ~/Jenab unless changed in Settings → General (3.9)
  projects/
    <project_id>/             stable ULID; the display name is stored separately
      project.db              the project's state
      chats.db                the project's logs
      objects/ab/12/ab12f9…   bucket contents, named by SHA-256 (section 4)
      snapshots/              VACUUM INTO copies taken before destructive migrations
      tmp/                    dry-run copies and partial downloads
```

- `<user data dir>` is `%LOCALAPPDATA%` on Windows, `~/Library/Application Support` on macOS and `$XDG_DATA_HOME` or `~/.local/share` on Linux. Moving the data folder moves only the projects; the settings, registry and logs stay, so the app can always find them.
- `config.yaml` is read into typed settings with the same YAML library as the configs (10). Saving from *Settings* keeps comments the user wrote. An unknown key, a wrong type or a value out of range is logged with its line and replaced by the default, so the app still starts.
- A project folder is self-contained. Backup, export and delete work on the folder.
- A folder is never copied while the project is open, because the `-wal` file may hold committed data that is not yet checkpointed. Export uses `VACUUM INTO` (or the SQLite backup API) for both databases, then copies `objects/`. Every `VACUUM INTO` follows the safe-copy rule in 7.5.
- If `registry.db` is lost, it is rebuilt by scanning `projects/`. The project's name is also stored in `_jenab_meta`.
- SQLite in WAL mode needs a local disk. Jenab warns when the data folder is on a network drive or inside a folder synced by OneDrive, Dropbox, iCloud Drive or Google Drive, since syncing the `-wal` file separately can corrupt the database. Backups go through export instead.

### 2.2 project.db

Holds the tables the agent designs, plus internal tables:

| Table | Holds |
|---|---|
| `_jenab_meta` | Project ID, name, schema version, storage format version, bucket lifecycle rules |
| `_jenab_columns` | Column annotations, e.g. `kind: object_ref` (4.5) |
| `_jenab_views` | View configs with revision |
| `_jenab_pipelines` | Pipeline configs with revision, and the *Pause schedule* flag (6.10) |
| `_jenab_memory` | Project memory sections with revision (3.3) |
| `_jenab_changes`, `_jenab_change_rows` | Change log and the row keys each change touched (2.5) |
| `_jenab_runs`, `_jenab_run_steps` | Pipeline runs and step results |
| `_jenab_objects`, `_jenab_object_versions` | Bucket index (4.2) |
| `_jenab_links` | Saved links (4.6) |
| `_jenab_approvals` | Approved hosts, Starlark script hashes, destructive migrations and MCP tools, each with who approved it: the user, or the Auto level (8.8) |
| `_jenab_notifications` | Notifications from runs and `app.notify` |

Configs, memory and the change log live in the same file as the data, so a migration, the config updates it requires and its change-log entry commit in one transaction.

Internal tables are left out of the project card.

**Agent tables**

- Every table has an explicit primary key. Undo and the change log identify rows by primary-key value, never by `rowid`, which `VACUUM` may renumber.
- No triggers, SQL views or virtual tables in agent tables (v1). Views in the UI are view configs (section 5), not SQL views.
- Names do not start with `_jenab_` or `sqlite_`.

**Agent SQL guard**

Agent-written SQL is only ever a single read-only query: `query`, `db.query`, view queries and the `from` query of the `copy_data` migration step (8.2). All writes are built by Go from typed requests (7.3), with table and column names checked against the schema and quoted.

Every agent query passes three layers, in this order. None of them depends on a driver-specific authorizer, so any SQLite driver works. SPIKE-007 tested 52 attacks and 11 normal queries against them on two drivers.

1. **Text check, in Go, before SQLite sees the query.** A small lexer that follows SQLite's rules for strings, quoted names and comments. It rejects:
   - more than one statement (some drivers run every statement in a string)
   - anything that does not start with `SELECT`, `WITH` or `VALUES`
   - names starting with `_jenab_`, `sqlite_` or `pragma_`
   - virtual table modules other than `json_each` and `json_tree`
   - `load_extension`
   - NUL bytes

   This layer must come first. SQLite runs some PRAGMAs while *preparing* a statement: just preparing `PRAGMA query_only = 0`, or running `EXPLAIN` on it, turns read-only off. So no agent text is prepared, explained or inspected for columns before it passes this check.
2. **EXPLAIN check.** `EXPLAIN <query>` is compiled with NULL bound to every parameter, and its program is scanned. Tables opened by `OpenRead` are mapped to names through `sqlite_schema` (the map is cached by `schema_version`). The query is rejected if it:
   - opens a `_jenab_*` table, `sqlite_schema`, or the temp or an attached schema
   - opens a virtual table (`VOpen`), unless layer 1 found only `json_each` or `json_tree`
   - starts a write transaction, or uses `OpenWrite` or another schema- or file-changing opcode

   This catches access through CTEs, subqueries, `IN table` and index-only lookups. Row opcodes such as `Insert` are allowed, because CTEs and sorting use them on temporary tables.
3. **Guarded connection.** The query runs on a reader connection (the `copy_data` query runs on the writer, inside the migration):
   - `query_only`, which is set again each time the connection is taken from the pool
   - `SQLITE_LIMIT_ATTACHED = 0`. Without it, a read-only connection can attach and read any other file, such as another project's database
   - `SQLITE_LIMIT_LENGTH = 16 MiB`, so a query cannot build huge strings or blobs in memory
   - a `context` deadline: 2 s for views (5.7), 10 s for `query` and `db.query`. A cancelled query is interrupted.

No single layer is enough on its own: EXPLAIN does not see PRAGMAs, and the connection layer does not stop reads of internal tables. Only layer 3 stops runaway queries. The cost is about 2 µs for layer 1 and 70 µs for layer 2 per query. If the driver offers an authorizer callback, it can be installed as an extra layer.

### 2.3 chats.db

| Table | Holds |
|---|---|
| `chats` | ID, kind (`mother` / `chat`), title, title_fixed, role, `skills` (8.9), `model` (3.9), default_page, created_by (a source as in 2.5: `app` for the Mother chat, `user`, or `message:<id>` of Mother's `create_chat` call), created_at, archived, `memory_reviewed_up_to` (3.5) (8.6); `ui_state` with the chat's layout (5.9) |
| `messages` | ID, chat_id, turn number, role (`user`, `assistant`, `tool`), model, token counts, created_at |
| `message_parts` | message_id, seq, type (`text`, `thinking`, `tool_call`, `tool_result`, `image`, `notice`, `approval`, `question` (8.8)), content, `ref` to a bucket object for large outputs, preview |
| `session_notes` | chat_id, content, revision, updated_at (3.4) |
| `review_chunks` | Results of *Refresh memory* per message range (3.5) |
| `messages_fts` | FTS5 index over text parts, used by `search_history`. See the full-text search rules below |

- A **turn** is one user message plus everything the agent does until it replies.
- Streamed text is kept in memory and written when a part completes. A crash loses at most the part in progress.
- The change log in `project.db` refers to messages by ID. These are plain IDs, not foreign keys, because they point into another file.
- **Full-text search** (SPIKE-011):
  - **Table:** `messages_fts` uses `tokenize = 'unicode61 remove_diacritics 2'`, `content = ''`, `contentless_delete = 1`. The rowid is the text part's rowid.
  - **Normalization:** Go normalizes the text before indexing, and queries the same way:
    - NFKC
    - `ي ى` → `ی`, `ك` → `ک`
    - remove tatweel and Arabic diacritics
    - Persian and Arabic-Indic digits → ASCII
    - `ß` → `ss`
    - remove ZWNJ
  - **Query builder:**
    - Split on spaces, quote each term (doubling `"`) and add `*`; join with `AND`.
    - A term with a ZWNJ becomes `("joined"* OR "split form"*)`.
    - An empty query does no search.
    - No other text reaches `MATCH`.
  - **Snippets** are built in Go from the stored text, because the index text differs from it.
  - **Inside words:** `search_history` with `substring: true` (and the UI toggle "Search inside words") runs a `LIKE` scan within the scope instead. That is about 0.15 s per 100,000 messages. It stops at 500 ms and returns what it found with a note.
  - **Measured on 100,000 messages:** the index is 0.4 × the text size; queries take 2 ms p50 and 25 ms p95.
- **Message before change:** message IDs are ULIDs made in Go. Before a tool call writes to `project.db`, the assistant message and its `tool_call` part are written to `chats.db`. A crash can then leave a message without a change, which is harmless, but never a change pointing to a missing message, which would break *Undo turn*. In SPIKE-008, the reverse order left a dangling entry in 11 of 20 crashes.

### 2.4 registry.db

| Table | Holds |
|---|---|
| `projects` | ID, name, folder path, workspace path (8.5), pinned, last_opened, created_at |
| `user_memory` | User memory sections with revision (3.3) |
| `connections` | Connection configs with revision (6.9); the keys are in the keychain |
| `mcp_servers` | MCP server configs with revision, and the projects each one is enabled in (8.7) |

Written rarely. One connection with `busy_timeout`, no writer goroutine needed.

### 2.5 Change log

Every write that changes project state adds one entry to `_jenab_changes`, in the same transaction as the write.

| Field | Meaning |
|---|---|
| `id`, `at` | Entry ID and time |
| `source` | `message:<id>`, `run:<id>`, `form:<view_id>`, `user`, `migration:<id>`, `refresh:<chat_id>`, `undo:<change_id>` (the source format below) |
| `target` | `table:<name>`, `view:<id>`, `pipeline:<id>`, `memory:<scope>/<section>`, `object:<key>`, `schema` |
| `op` | `insert`, `update`, `upsert`, `delete`, `save`, `migrate` |
| `before`, `after` | Previous revision for configs, memory and objects. Row changes keep their before-images in `_jenab_change_rows`; after cleanup, `after` holds the row counts per kind |

- One entry per write request, not per row. A 5,000-row insert is one entry.
- **Source format:** `<kind>` or `<kind>:<id>`. It is the one way Jenab records who did something: the change log, `chats.created_by` (2.3), `created_by` on objects and links (4.2, 4.6) and approvals (8.8). `app` means the app itself, e.g. for the Mother chat. A kind may gain an ID later (for example `user:<id>` if projects are ever shared), so readers accept both forms.
- The rows each entry touched are listed in `_jenab_change_rows(change_id, table_name, row_key, kind, before)`:
  - It is a rowid table with indexes on `(table_name, row_key, change_id)` and `(change_id)`.
  - `row_key` is the primary-key value as a JSON array, e.g. `["BTC","2026-09-27"]`. Configs, memory sections and objects use their target as the key.
  - `kind` is `i` for an inserted row, `u` for an updated one and `d` for a deleted one.
  - `before` is the row as a JSON object before the change; it is empty for inserted rows.
  - The index keeps the undo conflict check an index lookup instead of a scan of the log (8–10 ms at 1 million entries in SPIKE-009).
- **Cleanup after 90 days (configurable):** a daily job, working forward from a stored watermark, deletes the change's row entries and keeps only the change entry, with row counts per kind in `after`. In SPIKE-009 this kept the log at about 30% of a one-year project; keeping row entries without before-images left it at 55%.

### 2.6 Undo

- **Undo turn** reverts all changes whose source is a message of that turn, in reverse order. Inserted rows are deleted, updated and deleted rows are restored, and configs, memory and objects return to their previous revision. Changes from other chats, forms and pipelines are not touched.
- **Revert run** does the same for one pipeline run.
- **Conflicts:** if a later change touched the same row, config, memory section or object (found through `_jenab_change_rows`), undo stops and lists the conflicts. The user chooses to skip those items or cancel.
- **Migrations are not reverted by undo.** They are reversed by a new migration from the schema agent, or by restoring the snapshot taken before a destructive migration. Restoring a snapshot shows how many writes happened since and will be lost.
- An undo is itself recorded as a change, so it can be redone.
- **Undo window:** changes older than the cleanup limit (90 days) are history only and cannot be undone. File bytes are kept for the same window, and their size is shown to the user (4.4).
- Cost (SPIKE-009): undoing a 5,000-row run takes about 0.3 s, and a chat turn about 1 ms. A 5,000-row upsert with change log must stay under 1 s.

### 2.7 Connections and lifecycle

- `project.db` and `chats.db` each have their own writer goroutine and reader pool (section 7).
- Connections of a project that has not been used for 10 minutes are closed. The scheduler opens a project when a run starts.
- At startup, the scheduler reads `_jenab_pipelines` of every project to register triggers, then closes the connections again.

**Recovery after a crash.** Opening a project creates `jenab.lock` in its folder, and a clean close removes it. If the file is already there when the project is opened, the last session did not end cleanly, and before the project is used Jenab:

1. runs `PRAGMA quick_check` on both databases (about 2 ms per MB, so ~0.3 s for 150 MB in SPIKE-008). If it fails, the project opens read-only and offers the latest snapshot
2. marks runs still `running` as `interrupted`. Their committed chunks stay, and *Revert run* removes them if wanted
3. empties `tmp/` and deletes `snapshots/*.partial` and `*.partial-journal`
4. sweeps `objects/` for files that no index row points to (4.3)

SQLite itself needs no help: an interrupted transaction is discarded the next time the database is opened.

### 2.8 App updates (SPIKE-019)

- **Updater:** Wails `pkg/updater` with the endpoint provider, reading Jenab's own manifest. The app is installed per user (`%LOCALAPPDATA%\Programs\Jenab`), so updating needs no admin rights.
- **Platforms:** Windows 11 and macOS are supported. Linux x64 is a beta, shipped as an AppImage: one file the updater can replace, installed per user. On Linux the webview is WebKitGTK.
- **Fail closed:**
  - A release must carry an ed25519 signature that verifies against the public key built into the app.
  - The feed must use https.
  - Prerelease versions are ignored unless the user picks the beta channel.
- **Checks:**
  - At start, then every 6 hours, on Jenab's own timer (never `CheckInterval`). Each version is downloaded once.
  - The user chooses automatic, notify only, or off. A check sends only the version, platform and architecture.
- **Restart:**
  - Only when no run, migration or write is in progress.
  - `OnShutdown` has a 10 s deadline.
  - A detached watchdog relaunches the installed exe if no instance is running 60 s after `Restart`.
- **Cleanup on start:**
  - leftover `%TEMP%\wails-update-*` files and helper logs
  - old `.old.*` exes
  - all but the last 2 pre-update database backups
- **Storage format version:**
  - Each `.db` stores it in `PRAGMA user_version`.
  - A newer app backs the file up with `VACUUM INTO` into `snapshots/`, then migrates in one transaction.
  - An older app refuses a newer file without touching it and asks the user to update.

---

## 3. Context and memory

### 3.1 Context order

Every request of the main agent is assembled in this order, from most stable to most changing:

1. System prompt, plus the chat's role if it has one (8.6), its loaded skills and the list of available skills (8.9)
2. User memory
3. Project memory
4. Project card (3.2), plus the chat list in the Mother chat (8.6)
5. Session notes (3.4)
6. History window (3.6)
7. Current turn

Provider prompt caches reuse the longest unchanged beginning of a request. The rule that follows: **between window cuts, a request only grows at the end.** Blocks 2–6 are rewritten only at a cut (3.6); *Refresh memory* (3.5) also ends with one. There is one exception where correctness wins:

- A schema, view or pipeline change refreshes the project card immediately.

Anything else that changes in between (another chat edits memory, a pipeline run finishes, this chat edits memory) is added as a short notice to the current turn and folded into its block at the next cut:

```
[memory updated by another chat: preferences → prices in EUR]
[run daily_btc finished 08:00: success, 6 rows inserted]
[user opened page bitcoin: from_date = 2026-07-01]
```

For providers with explicit cache control (Anthropic), cache points are placed after block 5 and at the end of the latest message.

### 3.2 Project card

Generated by the app, never written by the agent. Contains:

- **Tables:** name, columns with types, primary key, object-reference columns, row count, min/max for date columns, and the views, pipelines and forms that use the table (SPIKE-021)
- **Views:** ID, title, type
- **Pages:** ID, title, the views they use
- **Pipelines:** ID, trigger, last run status and time
- **Connections:** ID, title, host, docs link (6.9)
- **MCP servers:** each enabled server with its tool names and one-line descriptions (8.7)
- **Workspace:** folder name and top-level entries, if a folder is linked (8.5)
- **Bucket:** top-level prefixes with object counts; number of saved links

Counts and run status are shown "as of" the time the card was built, since the card only refreshes at cuts or on schema and config changes. Target size is under 2,000 tokens. For larger projects, the card lists table names only, and the agent uses `describe_table`.

### 3.3 User memory and project memory

| | User memory | Project memory |
|---|---|---|
| Scope | All projects | All chats in one project |
| Stored in | `registry.db` | `project.db` |
| Size cap | 1,000 tokens | 2,000 tokens |

Both are organized in named sections. Default sections: `goals`, `preferences`, `decisions`, `conventions`. The agent may add sections.

**Content rule:** memory holds intent: goals, preferences, decisions with their reasons, conventions. It never holds values that live in tables, such as prices, counts or lists of tracked items.

**Editing** uses `update_memory(scope, section, content, expected_revision)`:

- Writes go through the writer queue as interactive requests and are recorded in the change log.
- If the section's revision has changed since the agent read it, the edit is rejected and the current content is returned, so the agent can merge.
- Empty content deletes the section. Edits that exceed the size cap are rejected.
- Each edit shows a chip in the chat: `Memory updated: preferences [undo] [edit]`.

**Who writes:** main agents in chats, the user, and *Refresh memory* (3.5). Subagents, pipelines and the schema agent do not.

### 3.4 Session notes

One per chat, stored in `chats.db`, capped at 2,500 tokens. This includes a `summary` section of at most 1,000 tokens, written by *Refresh memory* (3.5). They hold the current task, the plan, open questions and decisions not yet confirmed. The agent maintains them with `update_session_notes` (same revision rule as memory), and *Refresh memory* updates them. They are deleted with the chat.

Where information goes: *Would a new chat in this project need it?* Then project memory. *Only this conversation?* Then session notes.

### 3.5 Refresh memory

A button per chat, in the chat header. It makes sure memory and session notes hold everything important from the chat, then shortens the history window. It is Jenab's compaction, done on request: the summary goes into capped, structured notes, and the history itself is never rewritten.

1. **Split.** Messages after `memory_reviewed_up_to` are split at turn boundaries into chunks of up to 5 turns or 12,000 tokens. Tool outputs appear as stubs.
2. **Map.** One background LLM call per chunk, run by the app (model alias `fast`, background priority, within `max_parallel_calls`). Each gets its chunk plus the previous turn as read-only context and returns items:
   ```yaml
   - kind: preference        # goal | preference | decision | convention | open_question | progress
     text: Prices are shown in EUR
     scope: project          # user | project | chat
     source: [msg_210, msg_214]
   ```
3. **Merge and compare.** One LLM call merges the items in chat order (later items replace earlier items on the same subject, duplicates are removed, items that state data values are dropped) and compares them with the current user memory, project memory and session notes. Each item is labeled `present`, `missing`, `contradicts` or `outdated`. The same call rewrites the `summary` section of the session notes (3.4): what was done, what was decided, what is still open.
4. **Apply.** The edits are written as normal memory edits (3.3) with source `refresh:<chat_id>`, without asking. One chip shows them: `Memory refreshed: 3 added, 1 changed · view changes · undo`. *View changes* shows the diff per section; *undo* reverts the whole batch.
   - It asks first, with the diff in an approval card (8.8), when the project's approval level is Strict, or when an edit changes or deletes a section whose last edit was made by the user.
5. **Cut.** A cut of the history window follows (3.6): the chat keeps its last `history_min_turns` turns, and blocks 2–5 are rebuilt with the new memory, card and notes. Older turns stay reachable through `search_history`.
6. **Save progress.** `memory_reviewed_up_to` moves forward, and chunk results are saved in `review_chunks`, so the next refresh only reads new messages. *Full refresh* discards the saved chunks.

For long chats, the estimated token cost is shown before starting.

### 3.6 History window

There is no automatic LLM compaction; *Refresh memory* (3.5) is the one the user starts. The history window is configured in `config.yaml`:

```yaml
context:
  history_min_turns: 3        # turns kept after a cut
  history_max_turns: 8        # a cut happens when the window grows past this
  history_max_tokens: 24000   # ...or when the history block grows past this
  tool_preview_tokens: 1500   # maximum size of a tool result preview
  turn_trim_ratio: 0.5        # in-turn trimming starts at this share of the model's window
```

- The current turn is always complete.
- Previous turns in the window contain user and assistant text in full and tool results as previews (3.7).
- **Cut:** when the window passes `history_max_turns` or `history_max_tokens`, it is reduced to the last `history_min_turns` turns in one step. At the same cut, tool results of previous turns become stubs and blocks 2–5 are refreshed. *Refresh memory* also ends with a cut.
- If the chat was idle long enough for the provider's cache to expire, a due cut happens on the next turn, since the cache is lost anyway.
- After the first cut, the context includes a line such as: `This chat has 38 earlier turns. Use search_history or read_messages.`

These sizes are starting values, to be tuned with the benchmark.

### 3.7 Tool outputs and web pages

- A tool result larger than `tool_preview_tokens` is stored in the bucket under `cache/tool/<chat_id>/<message_id>-<seq>`.
- Web pages are fetched, converted to readable text in Go (HTML, menus and ads removed) and stored under `cache/pages/<host>/<hash>.txt`.
- **Readable text** (SPIKE-015), used by `fetch_page` and `html.extract`:
  1. **Charset:** Go decodes the page to UTF-8 first, taking the charset from the HTTP header, then the BOM, then `<meta>` (`x/net/html/charset`). The libraries guess charsets and turned every raw windows-1256 page into garbage.
  2. **Size:** the body is capped at 5 MB before parsing (6.6).
  3. **Main content:** `codeberg.org/readeck/go-readability/v2` finds the main content.
  4. **Markdown:** `html-to-markdown/v2` turns that content into Markdown, so tables and links stay readable. It runs with newlines in cells kept and spans mirrored; its defaults skip such tables.
  5. **Title:** taken from `<title>` or `og:title`, normalized to NFC, with the ZWNJ kept.
  6. **JavaScript-only pages:** if the text is under about 200 characters, the result has status `needs_javascript` instead of an empty success. JavaScript is not rendered in v1.
- The result enters the context as a **preview**; the `ref` is the bucket key:
  ```
  [page "Laptop X review" — example.com — 8,000 tokens, showing first 1,500 — ref: cache/pages/example.com/ab12f9.txt]
  …first part of the text…
  [use read_ref(ref, offset) or search_ref(ref, query) for more]
  ```
- At a cut, previews of previous turns become **stubs**:
  ```
  [page "Laptop X review" — example.com — ref: cache/pages/example.com/ab12f9.txt]
  ```
- **In-turn trimming:** when the current turn passes `turn_trim_ratio` of the model's window, all tool previews in the turn except the two most recent become stubs, in one step.
- `cache/` objects expire (4.4). `read_ref` on an expired ref returns `expired` with the source URL, so the agent can fetch it again.
- For heavy reading, the agent uses a subagent, which reads in its own context and returns only its result.
- Images from tools are not placed in the context unless the agent opens them with `bucket_read` on a vision-capable model.

### 3.8 Provider layer

The orchestrator owns the agent loop. LLM providers are reached through the official SDKs, `anthropics/anthropic-sdk-go` and `openai/openai-go/v3`, behind Jenab's own `Provider` interface. The interface streams text, tool calls, thinking, usage and the stop reason. Local models use the OpenAI SDK with a custom base URL. No agent framework is used (SPIKE-012).

- **Protocol only:** the provider layer handles each API's protocol details, such as message shapes, field names, error codes, stream ends and rate limits. It never changes what the agent is told or what it may do (1, model-neutral).
- **Tools:** the provider layer never runs tools. The assistant message and its `tool_call` parts are written to `chats.db` before any tool runs (2.3).
- **Complete streams only:**
  - A stream counts as complete only after `message_stop` (Anthropic) or a `finish_reason` (OpenAI). Otherwise it is a truncation error. The SDKs return no error for a stream cut off early.
  - `ctx.Err()` is checked after the stream, so a cancelled partial message never looks complete.
  - Tool-call arguments must be valid JSON when the call ends.
- **Timeouts:** a request fails as stalled if its first event doesn't arrive within 2 minutes (10 minutes for Ollama), or if 60 s pass between two events. There is no limit on the whole answer, so long answers are fine. Starting values, tuned with the benchmark.
- **Message shapes:**
  - Anthropic: all tool results of a turn go in one user message.
  - OpenAI: tool messages take text only, so tool images follow in one user message.
  - Thinking blocks are sent back unchanged, signature included, before their `tool_use`.
- **Caching and usage:**
  - Cache points follow 3.1.
  - Cache write and read tokens are stored with the message.
  - OpenAI streams set `include_usage`.
- **Errors and retries:**
  - Jenab sets the SDK retry count itself and caps `retry-after`, which the SDKs do not cap.
  - Errors map to a `ProviderError` with status, kind and `retry-after`. The kind decides what happens:

    | Kind | Examples | What happens |
    |---|---|---|
    | `rate_limited` | 429 "too many requests" | Wait, then retry |
    | `overloaded` | 503, Anthropic's 529 | Wait, then retry |
    | `transport` | lost connection, a cut-off or stalled stream | Wait, then retry |
    | `quota` | daily quota or credit used up, often sent as a 429 | No retry: the model stops until the user acts |
    | `too_large` | 413 | No retry as is: the context is shrunk |
    | `request` | 400, 401, 403, 404 | No retry: the error is shown |

  - **Waits:** the provider's own wait when it sends one (`retry-after`, or Gemini's `retryDelay` in the body). Otherwise 10 s, 20 s, 40 s, 80 s, then every 100 s. Up to 20 % is added at random, so calls don't all retry at the same moment.
  - **One pause per provider:** when a call is `rate_limited` or `overloaded`, every call to that provider waits until the pause ends, instead of each call trying again on its own. The provider's limit for background calls is also halved (at least 1), then raised by one after every 20 calls in a row that succeed, up to its setting (7.6). A key that another app also uses is handled this way, with nothing to set.
  - How long a caller keeps trying is up to the caller: chat turns and subagents (8.3), pipeline steps (6.5). Starting values, tuned with the benchmark.
- **Silent truncation:**
  - Jenab compares the reported `prompt_tokens` with its own estimate for the request.
  - A large shortfall is a truncation error, not a normal reply. Ollama drops old messages without an error (SPIKE-017).
- **Ollama** (SPIKE-017):
  - `/v1` ignores `num_ctx` and uses a small default context (4,096 tokens without a GPU). Jenab uses a derived model with `num_ctx` and `num_thread` set, or the native `/api/chat`.
  - Send `max_tokens`, not `max_completion_tokens`.
  - Turn thinking off with `reasoning_effort: "none"`.
  - Background calls go one at a time (`provider_max_parallel_calls`, 7.6). Chat calls never wait (7.6), so Ollama queues them itself; the 10-minute first-event timeout covers that queue.
  - Set keep-alive through the native API.
- **OpenAI-compatible providers** (SPIKE-018):
  - The stream is read to its end. Usage comes either in its own chunk after `finish_reason` or in the same chunk.
  - Errors, mapped to the kinds above:
    - **413** means the request is too big for the provider's limit (`too_large`).
    - Daily or quota errors are `quota`. Examples: Gemini `PerDay`, and Z.ai codes 1113, 1308 and 1310. They often come as a 429, so the body decides the kind, not the status.
    - Transport errors and cut-off streams are `transport`.
  - Waits follow the rules above. Ollama Cloud and Z.ai send no rate-limit headers, so the pause per provider is their only pacing.
  - Clients are built with explicit options and an allow-list of outgoing headers. The SDK's `OPENAI_*` environment variables are ignored, so they never reach another host.
  - Gemini:
    - Tool calls carry `extra_content` (a thought signature), which is sent back unchanged. Without it, Gemini returns a 400.
    - Error bodies are JSON arrays.
    - In a stream, a new tool-call id starts a new call even if the index repeats.
    - No `reasoning_effort` for Gemma models.


### 3.9 Models and first run

**Model catalog.** Jenab ships a list of known models, `models.json`, built into the app.
- One entry per model: provider, model ID, display name, context window, output limit, whether it takes tools, images and thinking, and the price per million tokens (for the usage shown in the app).
- It holds facts about limits and prices only, never prompts or rules for a model (1).
- An entry can be the provider's suggested `default` or `fast` model.
- New models arrive with app updates (2.8). A release can also mark a model as retired: it stays usable while the provider serves it, and the model picker suggests a replacement.

**Providers and keys.**
- Anthropic, OpenAI and Google Gemini, plus *OpenAI-compatible* (any base URL, e.g. Z.ai, Groq or Ollama Cloud) and Ollama (3.8). All five come in Phase 1 (decided 2026-09-29). Anthropic uses the Anthropic SDK; the others use the OpenAI SDK (3.8).
- *Connect* stores the key in the OS keychain (6.7), reads the provider's model list and turns on every catalog model the key can use. They appear in the model picker at once.
- For Anthropic, OpenAI and Gemini, catalog models are shown first. Models the provider lists but the catalog doesn't know yet appear under *Other models*, off by default; turning one on asks for its context window. For OpenAI-compatible providers and Ollama, the models come from the provider's list; the context window is read from the provider when it reports one (Ollama `/api/show`), otherwise the user enters it.
- `llm.models` in `config.yaml` maps the aliases `default` and `fast` (used by `llm.*` steps, 6.5) to models. With the first provider they are set from the catalog's suggestions; the user can change them in *Settings → Models*.
- **Model picker:** a chip in the composer shows the chat's model by its catalog name, e.g. "Gemma 4 31B". Its menu lists the models that are on, grouped by provider, and marks which ones are `default` and `fast`. Providers without a key are listed with *Add key*.
  - The choice applies to that chat and is stored in `chats.model`. New chats use `default`.
  - Changing the model changes nothing else: same prompts, tools and limits (1).
- **Usage:** *Settings → Usage* shows tokens per day, per chat and per model, with pipeline steps as their own line. The cost comes from the catalog prices; models without a price show tokens only.

**First run.** There is no setup wizard.
- The app opens to its main window with *Create project* and *Open the example*, which creates the Bitcoin project from section 9 with sample data. Its pipelines run once a provider is set up. The data folder defaults to `~/Jenab` and can be changed in *Settings → General*; the warning from 2.1 shows as a banner if it applies.
- *Create project* asks for a name and opens the project's Mother chat. It starts with a short greeting and three starter prompts, e.g. "Track a price every day". A starter prompt fills the composer; it is sent only when the user sends it. They are built in and the same for every model (1).
- A message sent while no provider is set up is kept. A card in the chat asks for a provider: one button per provider and a key field, the same form as *Settings → Models*. If Ollama is running on this machine (`localhost:11434`), it is listed first, with no key needed. Once connected, the kept message is sent.
- Everything else is asked when it is first needed: a search provider at the first web search (6.5), and start at login when the first scheduled pipeline is saved.

---

## 4. Bucket and links

### 4.1 Model

Each project has one bucket: an S3-style store that maps keys to objects. Keys are what users, the agent and tables see; underneath, content is stored by hash.

```
Key                                         Content (immutable)
images/btc/2026-09-27.png        ───────►   objects/ab/12/ab12f9…
reports/weekly.pdf               ───────►   objects/c3/7e/c37e01…
cache/pages/example.com/9d04.txt ───────►   objects/9d/04/9d04aa…
```

- **Keys** are segments of `[A-Za-z0-9._-]` separated by `/`, at most 512 characters, with no leading `/`, no empty segments and no `..`. Prefixes act as folders.
- **Content** is immutable. The same bytes are stored once (deduplication), and a file's name is its checksum. Original file names are metadata and are never used as paths.
- Overwriting a key creates a new version; the old one is kept according to lifecycle rules.

### 4.2 Index tables

```
_jenab_objects(key PRIMARY KEY, sha256, mime, size, metadata JSON,
                source, source_url, created_by, updated_at)
_jenab_object_versions(key, version, sha256, size, created_by, created_at)
```

`source` is `upload`, `http`, `pipeline`, `agent` or `tool`. `created_by` is a source (2.5), e.g. `message:<id>`, `run:<id>` or `user`.

### 4.3 Operations

| Operation | Meaning |
|---|---|
| `put` | Store content under a key (new version if the key exists) |
| `get`, `head` | Read content or metadata |
| `list` | List by prefix, with `/` as delimiter |
| `delete` | Remove the key; bytes stay until the sweep (4.4) |
| `move` | Rename a key; object-reference columns are updated in the same transaction |

All operations are recorded in the change log.

**Write order (crash safety):**

1. `put` streams the bytes into `tmp/`, computing the SHA-256 on the way.
2. The file is flushed to disk (`fsync`) and renamed into `objects/`. If a file with that hash already exists, the temporary file is removed.
3. The index row, the version row and the change-log entry are written in one transaction.

A crash leaves at most a file that no index row points to. The sweep (4.4) removes it. `delete` removes only the index row; the bytes are removed later by the sweep. So an index row never points to a missing file (SPIKE-008).

### 4.4 Lifecycle

Rules are stored in `_jenab_meta` and editable in project settings. Defaults:

```yaml
lifecycle:
  - { prefix: "cache/", expire_after: 30d }
  - { prefix: "", keep_versions: 5 }
cache_max_size: 500MB      # cache/ is trimmed least-recently-read first
```

- Objects referenced by an object-reference column are never expired.
- **Daily sweep:** it removes bytes only when nothing points to them: no current version, no kept version, and no change still inside the undo window (2.6). So file undo reaches back as far as row undo.
  - When the change-log cleanup drops before-images older than 90 days (2.5), the next sweep frees their bytes.
  - `cache/` keeps its own `expire_after` (30 days): cached pages can be fetched again.
  - Files in `objects/` with no index row at all (left by a crash) and files in `tmp/` older than a day are removed too.
- **The cost is visible:** project settings and the project card show **"Kept for undo: N files, X MB"**, meaning bytes that only the undo window still holds.
  - **Free now** removes them at once, after a confirmation that names the size. Undo then stops working for those files and says so.
  - The same line appears in the bucket's size total, so the user sees why a project is larger than its visible files.

### 4.5 Links and references

- Every object has a link: `jenab://<project_id>/<key>`. Links are used in chat, in the UI and for "copy link". Clicking one opens the object in the Files panel.
- **In tables, object-reference columns store the key only**, not the full link, so data stays valid when a project is imported under a new ID. The schema agent marks such columns in `_jenab_columns` with `kind: object_ref`. A write with a key that does not exist fails.
- The frontend loads objects through a Wails asset handler route, `/objects/<project_id>/<key>`, never through `file://` paths.

### 4.6 Saved links

```
_jenab_links(id, url UNIQUE, title, note, tags, created_by, created_at)
```

`created_by` is a source (2.5).

Shown next to the bucket in the **Resources** panel. The agent saves sources with `save_link`; the user can add, edit and delete links.

### 4.7 Serving untrusted files

- The app sets `Content-Type` from the MIME type detected when the object was stored, not from the source, and sends `X-Content-Type-Options: nosniff`.
- HTML is never rendered inline. SVG is shown only through `<img>`, since SVG can contain scripts.
- Files are never executed. Opening one in an external app requires an explicit user click.
- Size limits: 100 MB per upload, 50 MB per `http.download` by default.

### 4.8 Backend

The bucket sits behind an `ObjectStore` interface (`put`, `get`, `head`, `list`, `delete`). v1 uses the local folder. Later, a project's bucket can use an S3-compatible backend (S3, R2, MinIO) with credentials in the keychain, and presigned links for sharing.

---

## 5. Views and pages

- A **view** is one block: a table, chart, `stat` or form. It is saved once and can be used on several pages.
- A **page** arranges views and static blocks in rows (5.9). Pages are what the user opens.
- The LLM writes only configs, never HTML, CSS or code.

### 5.1 Common fields

| Field | Required | Description |
|---|---|---|
| `version` | yes | Always `1` |
| `id` | yes | Unique view ID |
| `title` | yes | Label shown above the block |
| `type` | yes | `table`, `chart`, `stat` (5.10) or `form` |
| `description` | no | Short text shown under the title |
| `query` | table, chart, stat | A single read-only SQL `SELECT` (or `WITH ... SELECT`) |
| `params` | no | Fixed values for named parameters that are not bound to a filter |
| `filters` | no | UI controls bound to query parameters (5.2) |
| `actions` | no | Buttons, e.g. run a pipeline (5.6) |

A parameter name may appear in `params` or in `filters`, not both.

### 5.2 Filters

A filter is a UI control whose value becomes a named query parameter.

```yaml
filters:
  - { param: from_date, control: date, label: "From", default: "today-90d" }
  - { param: coin, control: select, label: "Coin", options: [BTC, ETH] }
```

- Controls: `text`, `number`, `date`, `select`, `checkbox`.
- `required: true` prevents the user from clearing the filter.
- Date defaults accept an ISO date or a relative form: `today`, `today-7d`, `today-3m`, `today-1y`, `start_of_month`, `start_of_year`.
- A cleared optional filter binds `NULL`. Queries handle this explicitly: `(:coin IS NULL OR coin = :coin)`.
- `select` options can be a static list or `{ query: "SELECT ..." }`, as in forms (5.5). The query returns one column.

### 5.3 Table

| Field | Required | Description |
|---|---|---|
| `columns` | yes | List of `{ field, label, format, format_options, align }` |
| `default_sort` | yes | `{ field, direction }`, the initial sort; the user can re-sort by clicking a column |
| `page_size` | no | Rows per page, default 50 |
| `rows_from` | for row actions | `{ table, key }`: the base table and its key column; the query must select the key |
| `row_actions` | no | Buttons per row (below) |

**Pagination and sorting.** The query does not contain `ORDER BY`, `LIMIT` or `OFFSET`. The runtime wraps it:

```sql
SELECT * FROM (<query>) ORDER BY <sort> LIMIT ? OFFSET ?
SELECT count(*) FROM (<query>)
```

An `ORDER BY` inside a table query is ignored and produces a validation warning. If the count query hits the timeout, the table shows "many rows" instead of a total.

**Transfer and rendering** (SPIKE-022):
- Pages go from Go to the frontend as column names plus array rows, not objects. That is 36 % smaller: a 500-row page is about 83 KB and takes about 10 ms.
- The table renders virtual rows. It scrolled 10,000 rows × 12 columns at 60 fps.

**Formats:** `text`, `number`, `currency`, `percent`, `date`, `datetime`, `url`, `boolean`, `badge`, `image`, `file`. `image` and `file` expect a bucket key and show a thumbnail or a download link. Format options go in `format_options`, e.g. `{ currency: USD, decimals: 2 }`.

**`badge`** shows a value as a coloured label, e.g. a status. `format_options: { tones: { new: blue, read: neutral, failed: red } }` maps values to tones (5.11); other values are `neutral`.

**Row actions:**

```yaml
rows_from: { table: watch_keywords, key: id }
row_actions:
  - { label: "Edit", open_form: edit_keyword }
  - { label: "Delete", delete: true, confirm: true }
```

- **`set`** updates fields of that row without a form, e.g. `{ label: "Mark read", set: { read: true }, bulk: true }`.
  - The fields must be columns of `rows_from.table`. The key and `_run_id` / `_fetched_at` can't be set.
  - Values are literals, `now()` or `today()`, checked against the column types.
- **`bulk: true`** (for `set` and `delete`): the table gets a checkbox column, and the action also works on the selected rows of the loaded page, at most 500. It is one write request and one change-log entry.
- **Writes:** `set` and `delete` go through the writer queue, are recorded in the change log and can be undone.

### 5.4 Chart

| Field | Required | Description |
|---|---|---|
| `chart_type` | yes | `line`, `bar`, `scatter` or `pie` |
| `x` | yes | `{ field, label, format }` |
| `y` | yes | List of series: `{ field, label }` |
| `series_by` | no | Field used to split rows into series (e.g. `coin`); `y` then has exactly one entry |

- The runtime wraps the query as `SELECT * FROM (<query>) LIMIT 10001`. If more than 10,000 rows come back, the first 10,000 are shown with a notice to aggregate in the query.
- Line and scatter charts are sorted by `x`.
- **Pie charts:**
  - `x` is the label field and `y` has exactly one entry, the value. `series_by` isn't allowed.
  - At most 12 slices are drawn, largest first; the rest are summed into "Other".
  - Negative values are an error shown on the block.
- **Size rules** (SPIKE-022). The runtime applies these from the data size; they aren't config fields.
  - **Up to 1,000 points:** Recharts' normal animation and dots.
  - **Above 1,000 points:** no animation and no dots. With the defaults, 10,000 points took 1.0–2.2 s to draw and up to 4.7 s to resize.
  - **Line and scatter above 2,000 points:** downsampled in Go to 2,000 points with LTTB before sending. This draws in about 130 ms. Scatter wasn't measured, so it follows the line rule until it is.
  - **Bar charts above 1,000 bars:** the first 1,000 are shown, with a notice to aggregate in the query, like the 10,000-row notice. 10,000 SVG bars stay over 1.5 s even without animation.
  - If exact 10,000-point charts are needed later, uPlot (canvas) drew them in about 100 ms.

### 5.5 Form

Forms write only through their `submit` action. Their writes go through the project writer queue and are recorded in the change log like any other data change.

| Field | Required | Description |
|---|---|---|
| `fields` | yes | List of `{ field, label, control, required, default, options, prefix }` |
| `submit` | yes | What happens on submit (below) |
| `load` | for update | Query that fills the fields; receives the row key as `:key` |

- Controls: `text`, `textarea`, `number`, `date`, `select`, `checkbox`, `file`.
- `options` can be a static list or `{ query: "SELECT ..." }`.
- A `file` control uploads into the bucket under `prefix` (e.g. `uploads/`) and stores the key.

```yaml
submit:
  action: insert          # insert | update | run_pipeline
  table: watch_keywords   # for insert / update
  key: id                 # for update
  pipeline: daily_btc     # for run_pipeline; form fields become pipeline inputs
```

- **Update forms** are opened from a row action (`open_form`), which passes the row's key. `load` fills the fields:
  ```yaml
  load: "SELECT keyword FROM watch_keywords WHERE id = :key"
  submit: { action: update, table: watch_keywords, key: id }
  ```
- Values are validated in Go against the table's column types before writing.
- Constraint errors are shown on the field, e.g. `keyword: a row with this value already exists`.
- For `run_pipeline`, if the pipeline is already running, the form shows "already running since 08:00" and does not queue a second run.

### 5.6 Actions

```yaml
actions:
  - { label: "Refresh now", run_pipeline: daily_btc }
```

The same "already running" rule as in 5.5 applies.

### 5.7 Runtime rules for views

- Queries run on a **read-only connection** from the project's reader pool (`PRAGMA query_only = ON`). They never wait for the writer (section 7).
- Views load asynchronously: each block shows its own loading state and fills in when its rows arrive. The blocks of a page query in parallel on the reader pool, so one slow block doesn't hold up the others.
- Only named parameters (`:name`). No string building.
- Query timeout: 2 seconds per block. Chart result limit: 10,000 rows. Tables are always paginated.
- No LLM call and no network access when a view opens.
- View queries cannot reference `_jenab_*` tables.
- After a migration, open views re-validate their queries on next load. A view that no longer validates shows an error state and is marked in the project card. This should be rare, since the schema agent updates dependents in the same migration.

### 5.8 Frontend rules (SPIKE-022)

Measured in WebView2 on a mid-range laptop.

- **Streaming chat:**
  - Go coalesces tokens into at most one Wails event per stream every ~16 ms. Wails `Emit` never blocks, so a faster producer would only grow memory and latency.
  - The frontend buffers tokens outside React and renders once per animation frame.
  - Finished Markdown blocks are memoised; only the last block is parsed again.
  - With these rules the chat held 60 fps at 100 and at 1,000 tokens/s. One React update per token dropped to 32 fps at 100 tokens/s, and typing lagged up to 2.6 s.
  - Markdown uses `react-markdown` + `remark-gfm` (tables, strikethrough, task lists) + `rehype-highlight`, which is safe by default. `marked` + DOMPurify is the fallback if memory or bundle size matters.
  - **Markdown styling:** shadcn Typeset, one CSS file kept in the repo. It never restyles blocks already on screen when a new one arrives, and it takes colours from the theme tokens (5.11).
    - The `typeset-chat` preset is used for messages, and a roomier preset for page text blocks (5.9).
    - Tool chips, approval cards and question forms inside a message use `not-typeset`.
    - Wide Markdown tables are wrapped in `typeset-scroll` and scroll sideways.
- **Opening a chat** must not block: the history renders virtualised or with `content-visibility: auto`. 200 plain messages took 265–750 ms.
- **Loading:** Recharts and the Markdown renderer load lazily. The shell alone is about 89 KB gzip, and everything together is about 318 KB.
- **Channels:** tokens, progress and notices use Wails events; they stayed in order with no loss up to about 10,000 per second. High-rate channels such as logs may use Wails GoStream, which has backpressure.
- **Perf checks** use frame times and Long Animation Frames, not the `longtask` API, which missed all of the streaming jank.
- **Right-to-left text:**
  - Every message, Markdown block, input, table cell and page text block gets `dir="auto"`, so Persian, Arabic or Hebrew text runs right to left, also next to English.
  - Text inside these elements aligns with `text-align: start`, so right-to-left text starts on the right.
  - Markdown lists, list items and tables get an explicit `dir` from their first strong character, so an item that differs from its list keeps its bullet. `dir="auto"` can't do this, and CSS `:dir()` is rewritten by the build (Lightning CSS).
  - **Font:** Geist has no Arabic-script glyphs, so Vazirmatn (OFL) is bundled right after it. Persian then looks the same on every OS; Latin text stays Geist.
  - The UI itself stays English and left to right. Only the text is handled.

### 5.9 Pages

A page is what the user opens. It arranges blocks in rows on a 12-column grid.

```yaml
version: 1
id: bitcoin
title: Bitcoin
description: Daily price and the news most likely to move it
filters:
  - { param: from_date, control: date, label: "From", default: "today-90d", required: true }
actions:
  - { label: "Refresh now", run_pipeline: daily_btc }
rows:
  - [ { view: btc_price_chart, span: 8 }, { view: btc_today, span: 4 } ]
  - [ { view: btc_price_table, span: 5 }, { view: btc_news_table, span: 7 } ]
  - [ { card: "Keywords", rows: [ [ { view: keyword_list }, { view: add_keyword } ] ] } ]
```

| Field | Required | Description |
|---|---|---|
| `version`, `id`, `title`, `description` | as in 5.1 | |
| `filters` | no | Page filters (5.2). Each one binds to the parameter of the same name in every block that has it. A block's own filter for that parameter is hidden on the page |
| `actions` | no | Buttons in the page header, as in 5.6, plus `open_page` and `open_form` |
| `rows` | yes | A list of rows; each row is a list of 1–4 blocks |

**Blocks:**

| Block | Fields | Notes |
|---|---|---|
| `view` | `view` (a view ID), `span` | Data blocks always point to a saved view, so a view has one definition however many pages use it |
| `heading` | `heading` (text), `span` | A section title |
| `text` | `text` (Markdown), `span` | Static text, rendered like chat Markdown, with no raw HTML |
| `card` | `card` (title), `rows`, `span` | A titled frame around rows of blocks, e.g. a stat and a chart. Its rows follow the same rules, on a 12-column grid inside the card. A card can't contain a card |
| `filter` | `filter` (the `param` of a page filter), `span` | Shows that page filter here, e.g. a search box above a table, instead of in the header |
| `button` | `button` (label), one of `run_pipeline`, `open_page`, `open_form`; `tone`, `confirm`, `span` | The "already running" rule of 5.5 applies |
| `image` | `image` (a bucket key), `alt`, `span` | Shown through `<img>`, as in 4.7 |

- **Page header:** the page's `title`, `description`, filters and `actions` are drawn at the top, so pages don't need a heading block for their title.
- **Writing data** stays with `form` views and row actions; `filter` blocks only set query parameters.

- **Span:** 1–12 columns. The default is 12 divided by the number of blocks in the row. The spans of a row add up to at most 12.
- **Narrow windows:** below about 900 px, rows stack into one column, and the chat becomes a drawer over the page (5.12).
- **No other layout:** no CSS and no positions. Simple shapes are what models write reliably (SPIKE-021).
- **Where pages open:**
  - The right sidebar has four tabs: Pages, Pipelines (6.10), Files and Links. Saved views aren't listed; they are building blocks.
  - Clicking a page opens it in the main area. The chat moves to a side dock, on the right by default.
  - The user can swap the dock to the left, hide it, or close the page to get the full chat back.
  - The agent puts views on a page even when a project needs only one.
- **Layout per chat:** pages belong to the project, and any chat can open any page. Each chat remembers its own layout:
  - Saved: the open page or pipeline, its filter values, the dock side, and the right-sidebar tab. They are stored in `chats.ui_state`. This is UI state, not project state, so it isn't in the change log.
  - Switching chats restores that chat's layout.
  - Opening a project opens its last active chat; the first time, that is Mother (8.6). So a page is always shown next to a chat.
  - A new chat starts full width, or with its `default_page` (8.6). The dock side defaults to the project's last choice.
  - The same page can be open in several chats, each with its own filter values.
  - A page changed by another chat refreshes live. A deleted page closes, and the chat goes back to full width.
- **The agent knows the open page:** opening a page or changing its filters adds a notice to the chat (3.1), e.g. `[user opened page bitcoin: from_date = 2026-07-01]`, so questions like "why did this chart drop?" work.
- **`open_page(id, filters)`:** the agent can open a page in its own chat, e.g. after building or changing it (8.1).

### 5.10 Block registry and `stat`

**Block registry:**
- Each block type is one unit: a React component, a JSON Schema and a Go validator.
  - Data blocks (saved views): `table`, `chart`, `stat`, `form`.
  - Page blocks: `heading`, `text`, `card`, `filter`, `button`, `image`.
- The frontend looks the type up in a registry. A new block type is added by registering one unit, with no change to pages.
- More types are expected later, e.g. kanban, tabs and lists. Each one uses tones (5.11), not colours.

**`stat`** shows one number:

| Field | Required | Description |
|---|---|---|
| `value` | yes | `{ field, format, format_options }` (5.3 formats) |
| `change` | no | `{ field, format }`, e.g. the change against yesterday, shown green or red |
| `caption` | no | Short text under the number |

- The query must return exactly one row. Any other result shows an error on the block.
- `ORDER BY` and `LIMIT` are allowed in a `stat` query, unlike in table queries. Example: `SELECT price, price / LAG(price) OVER (ORDER BY date) - 1 AS change FROM btc_prices ORDER BY date DESC LIMIT 1`.

### 5.11 Theme

- **Setting:** Light, Dark or System (the default, following the OS). Set in *Settings → Appearance* and stored in `config.yaml` (`ui.theme`).
- **How it works:**
  - shadcn/ui colours are CSS variables, switched by a `dark` class on the root. A change applies at once, without a reload. With System, an OS change applies at once too.
  - Blocks, charts (Recharts reads the same variables), code highlighting and the chat all use these variables.
  - The window background is set from the saved theme before the first paint, so there is no white flash at start. The OS window frame follows the theme where the platform allows it.
- **Tones:** configs never contain colours or CSS. They can only name a tone: `neutral`, `blue`, `green`, `amber`, `red` or `purple`.
  - Each tone has a light and a dark value, and text in each tone meets WCAG AA contrast (4.5:1) in both themes.
  - Validation rejects anything else, such as hex colours.
  - So every page the agent builds looks right in both themes.
- **Charts:** series take the tones in a fixed order. Changes (`stat.change`) are green or red.
- **Later:** accent colours, custom themes, high contrast.

### 5.12 App shell

Agreed in the Gate-2 low-fidelity review (screens 01, 02, 08 and 11).

- **Layout:** from the start edge: the rail, the left sidebar (this project's chats), the main area (a chat, or a page or pipeline with the chat in the dock, 5.9), and the right sidebar (Pages, Pipelines, Files, Links). The bottom bar runs under all of them.
- **Rail:** always shown, 56 px wide. It holds what belongs to the whole app:
  - At the top, one tile per project with its initial, then *New project*. A bar on the start edge marks the open project. Switching is one click.
  - *Search*, and *Waiting*: the approvals and questions from every project (8.8), with a count. Clicking one opens its chat.
  - At the bottom, the schedules (which keep running in the tray) and Settings.
- **Left sidebar:** the project's name, a menu with *Project settings*, then *New chat* and the chat list with Mother pinned at the top (8.6).
- **Page open:** while a page or pipeline is open, the left sidebar hides and its chats move into the rail, below a divider: one tile per chat, then *New chat*. The user can show the sidebar again; the choice is saved in `chats.ui_state`.
  - A chat's tile shows its initials; Mother's tile is filled with its gradient (8.6).
  - A bar on the start edge marks the open chat, a spinner means working, and an amber dot means waiting. Every tile has a tooltip with the title and status.
- **Bottom bar:** about 24 px high. It shows only what changes and what the user can act on. Clicking an item opens it.
  - Start side: this project's work ("2 working · 1 waiting", opens the chat), its commands ("1 command running" or "Waiting for memory", a list with *Stop*, 8.5), and its pipelines (the running one with its time, or the next scheduled run; opens the Pipelines tab).
  - End side: the open chat's model and how full its context is, e.g. `gemma4 · 41% of context` (3.1); memory, e.g. `RAM 2.1 / 16 GB`, amber below 1 GB and red below the 500 MB hard stop; and provider problems while they last, e.g. "Z.ai rate-limited, retry in 30 s" (3.8).
  - Memory is the same free-memory reading as 8.5, read about every 2 s while the window is visible. There is no CPU reading: nothing in the app acts on it.
- **No tabs:** the main area shows one chat, with its page if one is open. Each chat keeps its own layout (5.9), and the agent is told which page is open, so tabs would add a second, competing idea of what is open.
  - *Ctrl+Tab* goes to the last chat used, with its page; pressing *Tab* again while holding *Ctrl* goes further back.
  - *Alt+←* and *Alt+→* (*⌘[* and *⌘]* on macOS) go back and forward through the chats and pages opened.
  - Two pages side by side is a later option (a split view, R-122).
- **Dock:** the dock can be resized. It starts at about a third of the main area, is at least 320 px wide, and its width is saved with the dock side.
- **Settings:** a full view in the main area, like the project settings: a page list on the left, *Back to chats*, and one scrolling page.
- **Project settings:** one scrolling page with the cloud-sync warning (2.1) when it applies, the workspace folder (8.5), storage ("Kept for undo" with *Free now*, the cache and lifecycle rules, 4.4), the approval level (8.8), allowed hosts and private-network exceptions (6.7), and *Export* (2.1).
- **Window size:** at least 640 × 480 px.
- **Narrow windows:** below about 900 px, page rows stack (5.9), the rail and both sidebars open as overlays from ☰, the chat opens as a drawer over the page, and the bottom bar is hidden. A dot on the chat button shows new activity.

---

## 6. Pipelines

### 6.1 Top-level fields

| Field | Required | Description |
|---|---|---|
| `version` | yes | Always `1` |
| `id` | yes | Unique pipeline ID |
| `description` | no | What the pipeline does |
| `trigger` | yes | `{ schedule, timezone, catch_up }` and/or `manual: true` |
| `inputs` | no | Named inputs with type and default, e.g. `{ coin: { type: string, default: BTC } }`; types: `string`, `number`, `boolean`, `date` |
| `steps` | yes | Ordered list of steps (6.3) |
| `on_error` | no | `stop` (default) or `continue` |
| `timeout` | no | Whole-run limit, default `5m` |

`timezone` is an IANA name, defaulting to the user's local time zone. Date helpers in expressions use it (6.4).

### 6.2 Runs and triggers

- `schedule` is a 5-field cron expression, evaluated in `timezone` (SPIKE-014):
  - **Syntax:**
    - Allowed: numbers, ranges, lists, steps on `*` or a range, three-letter month and day names, `7` as Sunday, and `@yearly`/`@annually`/`@monthly`/`@weekly`/`@daily`/`@midnight`/`@hourly`.
    - Rejected: 6 or 7 fields, `L`, `W`, `#`, `?`, `H`, backwards ranges, `@reboot`, `@every`, `TZ=` prefixes.
    - An expression with no run in the next 9 years is rejected.
  - **Day matching** is classic cron: if day-of-month and day-of-week are both restricted, either one matches. A day field starting with `*` makes it AND.
  - **DST:**
    - A fixed-time job whose time is skipped runs once, at the first minute after the gap.
    - A fixed-time job whose time repeats runs only the first time.
    - Jobs with `*` in minute or hour follow the real clock.
  - **Time zones:** the app embeds `time/tzdata`, so zones work on machines without Go.
- **Wall-clock check** (SPIKE-004):
  - The scheduler compares the wall clock with the job times every minute. Job times are never long timers, since those don't follow clock changes.
  - On Windows the minute check fires within 1.5 s of waking. The process does not run during Modern Standby.
  - It also checks on resume events from `PowerRegisterSuspendResumeNotification`. Wails' sleep and wake events are not used, because they don't arrive on Modern Standby.
  - Suspend and resume times are logged, so a late run can show when the laptop was asleep.
- **Catch-up:** `catch_up: once` (default) means that if one or more scheduled times were missed in the last 24 hours (app closed, laptop asleep), the pipeline runs once as soon as possible, with trigger `catch_up`. With `catch_up: skip`, missed times are recorded as `skipped`.
- **One run per pipeline at a time.** A scheduled or catch-up run that starts while the previous one is still running is recorded as `skipped`. A manual or form-triggered run is rejected with a message and not queued. Different pipelines in the same project can run in parallel; only their writes are serialized (section 7).
- When `max_parallel_runs` is reached, runs wait in a FIFO queue. A run that waits longer than its pipeline timeout is recorded as `skipped`.
- Run status: `success`, `failed`, `partial` (some steps failed with `continue_on_error` or `on_error: continue`), `skipped`, `interrupted` (the app stopped during the run, 2.7).
- **Failure notifications:** when a scheduled or catch-up run ends `failed` or `partial`, the app shows a desktop notification and adds an entry to `_jenab_notifications`. After three failures in a row, the pipeline is flagged in the sidebar and the project card; its schedule keeps running.

### 6.3 Step fields

| Field | Required | Description |
|---|---|---|
| `id` | yes | Unique within the pipeline |
| `use` | yes | Step name from the catalog, e.g. `http.get` |
| `with` | depends | Inputs for the step, checked against its input schema |
| `if` | no | Expression; the step is skipped when false |
| `for_each` | no | Expression returning a list; the step runs once per item, available as `item` |
| `retry` | no | `{ max: 3, backoff: 10s }` |
| `timeout` | no | Per-step limit, default `60s` |
| `continue_on_error` | no | Default `false` |

Steps run in order. (`depends_on` for parallel branches is planned for a later version.)

**`for_each`** (SPIKE-021; misuse was the most common error the models made):
- Use it only to run a step once per element of a real list, such as one price request per coin. A step that already takes a list, such as `llm.select`, `transform.map` or `db.insert_many`, doesn't need it.
- `item` is available only in:
  - the `with` and `if` of a step that has `for_each`
  - `transform.map` fields and `transform.filter` conditions
- In any other place, `item` is a validation error.
- The step's output is `each`, a list of `{ item, ...outputs }` in item order, plus `count`. Later steps read it as `steps.<id>.each`, e.g. `steps.pages.each[0].text`. They never use `item` for it.
- To turn `each` into rows, use `transform.map` over `steps.<id>.each`. To merge lists made per item, use `flatten` (6.4).
- `if` is checked once per item, before the step runs for that item. A skipped item has no entry in `each`.

### 6.4 Expressions

Expressions are written as `${{ ... }}` and evaluated with a sandboxed expression engine (`expr-lang/expr`).

- If a value is **only** an expression, the result keeps its type (list, object, number).
- If an expression is **inside** a string, the result is converted to text.
- Referencing a missing field is an error. Use `??` for a default: `${{ steps.price.json.close ?? 0 }}`.

Available in expressions:

| Name | Meaning |
|---|---|
| `steps.<id>.<output>` | Output of an earlier step |
| `inputs.<name>` | Pipeline input |
| `item` | Current item inside `for_each` and per-item expressions |
| `today()` | Current date as `YYYY-MM-DD` in the pipeline's timezone |
| `now()` | Current time as RFC 3339 in UTC |
| `date(s)`, `format_date(d, layout)`, `add_days(d, n)` | Date helpers |
| `map`, `filter`, `all`, `any`, `len`, `lower`, `upper`, `trim`, `split`, `join`, `round`, `int`, `float`, `string`, `flatten` | Allowed functions; other `expr` builtins are disabled. `flatten(list)` joins a list of lists into one list, one level deep |

References may only point to **earlier** steps.

Per-item expressions (`transform.map` fields and `transform.filter` conditions) are not evaluated when the step starts. The step evaluates them once for each item, with `item` set to that item.

**Sandbox rules** (SPIKE-005):
- **Syntax:**
  - Allowed: literals, field and index access, arithmetic, comparisons, `and`/`or`/`not`, `in`, `contains`/`startsWith`/`endsWith`, `??`, `? :`, `if/else`, list and map literals, and pipes (`x | upper()`).
  - Rejected when the pipeline is saved: `let`, `matches`, `..`, `$env`, method calls, and names starting with `__`.
- **Strict fields:** a missing field or list index is an error, except on the left of `??`. The engine alone returns `nil`, so Jenab rewrites every field access into a strict lookup.
- **Limits per evaluation:**
  - source at most 4 KB
  - at most 500 syntax nodes
  - list functions may not nest inside a predicate (`filter(a, any(b, …))` is rejected), so cost stays linear in the list size
  - at most 1 MB of text built, counted across `+` and all text functions
  - the result must be a finite number (no `Inf` or `NaN`)
- **Timeouts:** expressions evaluated at step start get a 1 s watchdog. Per-item expressions run inside the step timeout. The engine cannot be cancelled, so these timeouts only stop waiting; the limits above bound the actual cost.
- **Environment:** only JSON-like values (objects, lists, text, numbers, booleans, null), never Go values with methods.
- **Numbers:**
  - Whole JSON numbers that fit in int64 are decoded as integers, so `%` works and SQLite stores them as `INTEGER`.
  - Integer `+`, `-` and `*` report overflow.
- **Map literal as a predicate body:** it needs parentheses: `map(list, ({symbol: .id, value: .price}))`.
- **Engine upgrades:** the sandbox depends on the engine's syntax tree types, so the SPIKE-005 cases run in CI.

### 6.5 Step catalog v1

**Network**

| Step | Inputs | Outputs |
|---|---|---|
| `http.get` | `url`, `query`, `headers`, `fail_on_status` (default `true`: a non-2xx status fails the step). The time limit is the step `timeout` (6.3) | `status`, `headers`, `body`, `json` |
| `api.get` | `connection`, `path`, `query`, `headers` (the connection adds the key), `fail_on_status`, as in `http.get` (6.9) | as `http.get` |
| `http.download` | `url` (or `connection` and `path`), `key`, `headers`, `max_size` (default 50 MB) | `key`, `size`, `mime` |
| `web.search` | `query`, `limit`, `freshness` (`day` / `week` / `month` / `any`), `source` (`web` / `news` / `wikipedia` / `hn`, default `web`) | `results[]` — `{ title, url, snippet, published }` |
| `feed.read` | `urls` (RSS or Atom feeds), `match` (optional keywords: keeps items whose title or summary contains one, ignoring case; an empty list keeps all), `since` (default `7d`), `limit` (default 50) | `items[]` — `{ title, url, summary, published, feed }`, newest first |
| `html.extract` | `url` or `html`, `mode` (`readable` / `selector`), `selector` | `title`, `text` (Markdown in `readable` mode), `links[]`, `status`. `readable` follows 3.7; an invalid `selector` is a validation error (CSS parsed with cascadia) |

`http.download` saves the response into the bucket under `key` (which may contain expressions, e.g. `images/btc/${{ today() }}.png`). Allowed MIME types are set in `config.yaml`.

**Search sources** (SPIKE-020). Jenab never scrapes search result pages. There is no keyless general web search: none that is allowed worked.
- **`web`:** the search provider set up by the user.
  - Tavily with the user's key; it has a free monthly tier.
  - Or the URL of a SearXNG instance the user runs.
  - **Later:** Brave. Its terms limit storing results, which matters when a pipeline saves them, so the settings page will say so.
  - If no provider is set up, `web_search` returns "no search provider set up", and a pipeline that uses `source: web` fails validation with the same hint.
- **Keyless sources:**

  | `source` | Service | Notes |
  |---|---|---|
  | `news` | GDELT DOC 2.0 | Best effort: only 12 of 44 attempts got through in the test. 60 s timeout, 30 s TLS handshake timeout, backoff on 429, results cached for a day. Queried in English with `sourcelang:`; words shorter than 3 characters are dropped |
  | `wikipedia` | MediaWiki search | The wiki is picked from the query language (`de.`, `fa.`, …). No publication dates, so `freshness` is ignored with a warning |
  | `hn` | Hacker News (Algolia) | Uses `search_by_date` |

- **Rules for every source:**
  - Every request sends a User-Agent with the app name, version and project URL (Wikimedia requires contact info).
  - The tool descriptions ask for 2–4 keywords with no filler words: HN matches every word, and GDELT rejects short ones.
- **Feeds** (`feed.read`) are the most reliable keyless source for trackers: all items are dated and fresh. The user picks the feeds, and their hosts need approval like any literal host (6.7). Jenab ships no publisher content.

**Transform (no LLM, no network)**

| Step | Inputs | Outputs |
|---|---|---|
| `json.extract` | `input`, `path` (JSONPath) | `value` |
| `transform.map` | `items`, `fields` (map of name → expression using `item`) | `items[]` |
| `transform.filter` | `items`, `condition` (expression using `item`) | `items[]` |

**Database**

| Step | Inputs | Outputs |
|---|---|---|
| `db.query` | `sql`, `params` | `rows[]`, `count` |
| `db.insert` | `table`, `row`, `on_conflict` (`error` / `ignore` / `upsert`), `conflict_key` | `inserted`, `updated`, `id` |
| `db.insert_many` | `table`, `rows`, `on_conflict`, `conflict_key` | `inserted`, `updated`, `skipped` |
| `db.update` | `table`, `set`, `where`, `params` | `updated` |

- `upsert` means `INSERT … ON CONFLICT(<conflict_key>) DO UPDATE SET` for the columns given in the row only; other columns are untouched. `conflict_key` defaults to the primary key.
- There is no `replace`. SQLite's `REPLACE` deletes and re-inserts the row, which clears columns not given and can trigger foreign-key delete actions.
- `db.query` is read-only. `db.update` requires a non-empty `where`. Schema changes are **not** possible from pipelines; they go through the schema agent.

**LLM**

| Step | Inputs | Outputs |
|---|---|---|
| `llm.select` | `items`, `instruction`, `count`, `add` (fields the LLM adds per item, e.g. `{ reason: string }`), `model`, `max_input_tokens` (default 20,000) | `items[]`, `indexes[]` |
| `llm.extract` | `input`, `instruction`, `output` (field schema), `model` | `data` |

- `llm.select` asks the LLM only for the indexes of the chosen items and the `add` fields. The runtime returns the **original** items with the added fields, in ranked order, so the LLM cannot change URLs or titles. If there are fewer items than `count`, all are returned, ranked. Items beyond `max_input_tokens` are dropped, with a warning in the run log.
- `model` is an alias from `config.yaml` (`llm.models`, e.g. `default`, `fast`). Pipeline steps default to `fast`.
- LLM output is validated against its schema. Invalid output is retried once, then the step fails. Token usage is recorded per step.
- A call that fails with `rate_limited`, `overloaded` or `transport` waits as in 3.8, and the step's `retry` decides how often it tries again. Time spent waiting for an LLM slot or a provider pause doesn't count toward the step's `timeout`; the pipeline's `timeout` still does. `quota` and `request` errors fail the step at once.

**Bucket**

| Step | Inputs | Outputs |
|---|---|---|
| `bucket.put` | `key`, `content` (text or JSON), `mime` | `key`, `size` |
| `bucket.list` | `prefix` | `objects[]` — `{ key, size, mime, updated_at }` |

Pipelines cannot delete objects. Deletion is manual or through lifecycle rules.

**App**

| Step | Inputs | Outputs |
|---|---|---|
| `app.notify` | `title`, `message`, `level` (`info` / `warning`) | — |

**Escape hatch**

| Step | Inputs | Outputs |
|---|---|---|
| `script.starlark` | `code`, `inputs` | whatever the script returns |

- Runs sandboxed Starlark with access only to the transform functions and a read-only `db.query`. It has no network access, no secrets, and cannot write to the database or the bucket. Its result is written by later `db.*` steps, so every write still goes through validated steps.
- Needs user approval, tied to the SHA-256 of the code. Changed code needs a new approval.
- **Sandbox** (SPIKE-016): the engine is `go.starlark.net`, pinned to a commit. It runs in a child process, because no engine limits memory in bytes.
  - **Memory:**
    - **Windows:** the child runs in a Job Object capped at 256 MB. The parent terminates the job when the memory-limit message arrives.
    - **Every OS:** the child sets `debug.SetMemoryLimit` to about 200 MB. The parent reads the child's memory about every 100 ms and kills it above 256 MB.
    - **Linux:** `RLIMIT_AS` of 1–2 GB as a backstop.
    - On macOS, a single large allocation can go over 256 MB for up to one check interval.
  - **Steps and time:** the step limit is 10,000,000. `Thread.Cancel` runs at the step timeout, and the parent hard-kills the child shortly after, because some builtins ignore cancel.
  - **Available:** recursion is off and `load()` is not available. The script sees only `db.query` and the transform functions, and `print` goes to the run log.
  - **`db.query`:**
    - The child never opens the database. It sends the SQL and parameters to the parent over its stdin/stdout pipe.
    - The parent runs the query on the project's reader pool, with the SQL guard (2.2), `query_only`, the reader timeouts, `SQLITE_LIMIT_LENGTH` of 16 MiB and `SQLITE_LIMIT_ATTACHED` of 0.
    - The parent streams back at most 100,000 rows.
  - **Cost:** about 28 ms to start the child, and about 40 ms for a typical transform over 10,000 rows.

### 6.6 Runtime rules for pipelines

- All writes go through the project writer queue as background-priority requests (section 7).
- Each `db.*` write step is its own short transaction. No transaction is ever held open across a network call, an LLM call or a wait for user approval.
- `db.insert_many` writes in chunks of 500 rows, one transaction per chunk. Use `on_conflict: ignore` or `upsert` so a retried step is safe to repeat.
- A run records the project's schema version when it starts. If the version has changed at a write step, the runner re-validates the remaining steps against the new schema. If they are still valid, the run continues. If not, the step fails with `schema_changed` and the validation errors.
- Each run is recorded in `_jenab_runs` with the pipeline revision, trigger, schema version and token usage. Each step result is recorded in `_jenab_run_steps` with a preview truncated to 2 KB, with secrets redacted.
- Retention: step previews 90 days, run records 1 year (configurable).
- HTTP response limit for `http.get` and `html.extract`: 5 MB.
- If a target table has `_run_id` and `_fetched_at` columns, `db.insert` and `db.insert_many` fill them automatically; `upsert` updates them. The schema agent adds these columns to tables it creates for pipeline data.

### 6.7 Network and secret rules

The agent writes pipelines after reading untrusted web content, and pipelines run unattended. So:

- **Secrets are used only through connections** (6.9):
  - Each key is bound to its connection's host.
  - Chats and pipelines never see or write a key; there is no `secrets.` expression.
  - This is checked at validation and again at runtime, including on redirects.
- LLM and search provider keys are used by the providers themselves and never appear in pipelines.
- **Host approval:** every literal host in a pipeline must be approved for the project once, in an approval card (8.8), or by the Auto level when the turn hasn't read untrusted content. A connection's host is approved when the connection is made. Approvals are stored in `_jenab_approvals`. Requests whose host comes from an expression (e.g. URLs from search results) are allowed, but cannot carry secrets or custom headers.
- **Private network block:** requests to loopback, private (RFC 1918), link-local and unique-local addresses are blocked. This is checked after DNS resolution and on every redirect. Exceptions are set per host in project settings.
- Only `http` and `https` URLs are allowed.
- **Redaction:** secret values are replaced with `[secret:NAME]` in previews, errors, logs and anything sent to an LLM.
- **Keychain** (SPIKE-006, Windows):
  - **Storage:** secrets are stored with `zalando/go-keyring` under the service `jenab`, as `jenab:conn:<id>` for connections and `jenab:provider:<id>` for LLM and search providers. On Windows this is a Generic credential in Credential Manager with "Local machine" persistence, so it doesn't roam.
  - **Reading:** a secret is read when a step needs it (about 0.1 ms) and isn't kept in memory.
  - **Size:** values must be 1–2,560 bytes.
  - **Background use:** reads from the app in tray mode need no prompt and no setup. Schedules run only in the app's process in the user's session, never as a Windows service or a logged-out scheduled task.
  - **Protection:** the keychain keeps secrets away from other OS users and out of the project folder. It does **not** protect them from other programs running as the same user.

The agent's own `fetch_page` tool (section 8) follows the same network rules.

### 6.8 Dry runs

- A dry run executes the pipeline against a `VACUUM INTO` copy in `tmp/`. Writes are discarded with the copy.
- Network and LLM calls are real. Their token usage is recorded and shown. New hosts still need approval first.
- `app.notify` and `bucket.put` are simulated: they appear in the result but are not delivered or stored. `http.download` writes to a temporary area that is discarded.
- The agent dry-runs every new pipeline before saving it.

### 6.9 Connections

A connection is a saved API with the user's key. The auth is written once, when the connection is made. Chats and pipelines then give only the connection, a path and parameters, so a model never has to put a secret, a host and an auth header together itself.

```yaml
version: 1
id: openweather
title: OpenWeather
base_url: https://api.openweathermap.org
auth: { in: query, name: appid }
docs: https://openweathermap.org/api
rate_limit: 60/min
```

| Field | Required | Description |
|---|---|---|
| `version`, `id`, `title` | yes | As in 5.1 |
| `base_url` | yes | An `https` URL with a literal host; paths are added to it |
| `auth` | no | Where the key goes: `in` (`header` / `query`), `name` (e.g. `Authorization`, `appid`), optional `prefix` (e.g. `"Bearer "`). No `auth` means a keyless API |
| `headers` | no | Fixed headers sent with every request, e.g. `Accept` |
| `docs` | no | Link to the API docs, shown in the project card |
| `rate_limit` | no | e.g. `60/min`; requests wait for a free slot |

- **Making one:**
  1. The agent reads the API docs and calls `connect_api` (8.1) with the fields filled in.
  2. The chat shows a form: title, host, where the key goes, and a key field.
  3. The user pastes the key and confirms, in an approval card (8.8). The key goes straight to the keychain and never passes through the LLM. The same confirmation approves the host.
  4. The agent tests the connection with one `call_api`.
- **Scope:** connections belong to the user and are stored in `registry.db`, so every project can use them. The project card lists them.
- **Use:** `call_api` in chat, `api.get` in pipelines (6.5).
  - Paths start with `/` and stay on the connection's host. A full URL or `//` in a path is a validation error.
  - A redirect to another host drops the key.
- **Errors:**
  - 401 and 403 become "OpenWeather rejected the key", with a button to enter the key again.
  - 429 waits and retries once, respecting `Retry-After`.
- **Editing:** the user changes or removes connections in *Settings → Connections*. Pipelines that use a removed connection fail validation with a hint.
- **Later:** `method: post`, paging and OAuth fit into connections without changing the pipelines that use them.
- **MCP servers** are stored next to connections, but are used as agent tools, not in pipelines (8.7).

### 6.10 Pipeline page

Agreed in the Gate-2 low-fidelity review (screens 06 and 06-A).

- **Where:** the right sidebar has a **Pipelines** tab, next to Pages, Files and Links (5.9). It lists every pipeline with its last run: ✓, ● running with the time so far, or ⚑ flagged after three failures (6.2). Clicking one opens it in the main area like a page, with the chat in the dock.
- **Header:** the ID, schedule, time zone and next run, with *Run now*, *Pause schedule*, *Dry run* (6.8) and *View YAML* (read-only).
  - *Pause schedule* stops scheduled and catch-up runs until the user resumes it; manual runs still work. The flag is stored with the pipeline in `_jenab_pipelines`, is recorded in the change log, and shows in the project card.
- **Two views**, switched with a *Flow / Steps* toggle. Flow is the default.
  - **Flow:** a read-only diagram drawn from the saved YAML, top to bottom: the trigger, one box per step (type, name and one key detail), `for_each` as a frame around its steps with the item count, and an end marker. `llm.*` steps are marked with their model alias.
  - **Touches:** beside the flow, the hosts, connections and tables each step uses, and the pages that show those tables.
  - **Steps:** the same steps as a table.
  - Both views show the selected run's result on each step: a count and time, warnings (e.g. items dropped by `llm.select`), or the failure.
- **Runs:** the run list from `_jenab_runs` (start, trigger, status, time). Picking a run shows its results on the steps.
  - The selected step shows its settings, its output preview with secrets redacted (6.6) and a ref to the full output.
  - *Revert run* undoes one run's changes (2.6).
- **Read-only:** the page never edits a pipeline. Changes go through the agent, as with pages.
- **Build:** plain React and CSS in v1, because v1 pipelines have no branches. A diagram library (React Flow) comes only with parallel branches (§11).
- **Later:** the same flow in the approval card of a scheduled pipeline.

---

## 7. Concurrency and database access

A project is used by many things at once: several chats, subagents, the schema agent, scheduled pipelines, forms and view refreshes. The rules below keep all of them responsive.

### 7.1 Core rule

**Nothing that must stay responsive waits on the database.** The UI, the agent loop and the scheduler never call SQLite directly. They send requests to the project's data layer and receive results asynchronously (Go channels internally, Wails events to the frontend).

### 7.2 Connections per database file

`project.db` and `chats.db` each have:

- **One writer goroutine** that owns the only write connection and processes write requests one at a time from a queue.
- **A reader pool** of read-only connections. With WAL mode, reads run in parallel and are never blocked by the writer.

`registry.db` is written rarely and uses a single connection.

SQLite settings on open:

```
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
-- reader connections only:
PRAGMA query_only = ON;
-- plus, through the driver: sqlite3_limit(SQLITE_LIMIT_ATTACHED, 0) on all connections,
-- and sqlite3_limit(SQLITE_LIMIT_LENGTH, 16 MiB) on readers (2.2)
```

**WAL checkpoints.** A reader that holds a read transaction open stops the WAL file from being reset, so it keeps growing. Rules:

- Readers read all rows they need and close the statement right away. No open cursor is kept across a page change, an LLM call or a UI event; each page of a table is a new query.
- No read transaction lasts longer than its query timeout (2.2).
- SQLite's automatic checkpoint stays on (default 1,000 pages). Under constant use (4 readers, a 500-row pipeline chunk every 2 s, and a chat write every 0.5 s), the WAL stayed at 7.8 MB for an hour. With one reader holding a transaction for 60 s at a time, it grew to 267 MB in 10 minutes (SPIKE-010).
- The `project.db` writer also sets `PRAGMA journal_size_limit = 67108864` (64 MB), so after a large write the file shrinks back at the next checkpoint that finishes.
- The `project.db` writer sets `PRAGMA cache_size = -65536` (64 MB). With the 2 MB default, rebuilds of 1M rows were up to 3× slower, and chunked inserts into large indexed tables were up to 2× slower.
- `PRAGMA wal_checkpoint(TRUNCATE)` runs when a project goes idle (2.7), when it is closed, and after a long migration (7.5). It takes 11 ms to 1.1 s. While a reader holds a transaction, it waits the full `busy_timeout` and then fails, blocking the writer. So it runs with `busy_timeout = 100` ms; if it is busy, it is retried at the next idle.
- SQLite writes sort and index temp files to the OS temp folder (on Windows, `%TMP%`, usually on C:), not to the project folder.

Projects are fully independent: separate folders, separate writers, separate pools. Idle projects are closed (2.7).

### 7.3 Write requests

Every write is a typed request (`insert`, `insert_many`, `update`, `delete`, `form_submit`, `config_save`, `memory_update`, `object_put`, `migration`, `undo`) carrying a Go `context` for cancellation and deadlines. The caller gets the result back asynchronously.

**Cancelled means not done.** While a request waits in the queue, its caller can give up, and the writer then skips it. Once the writer has taken it, it runs to the end and the caller waits for the result, even if its context is cancelled; the transaction is short (7.4). The switch between waiting, taken and given up is atomic, so a cancelled request never commits.

The `project.db` writer queue has two priorities:

- **Interactive**: chat turns, form submits, memory edits, schema agent migrations, undo.
- **Background**: scheduled and manually triggered pipelines, *Refresh memory*.

Interactive requests go first, but after 10 interactive requests in a row the writer takes one background request, so background work is never starved.

The `chats.db` writer has a single queue, since its writes are small appends.

### 7.4 Short transactions

A write transaction contains **only** database work. The required pattern is:

1. Fetch (HTTP, search, LLM) — outside any transaction.
2. Transform in memory.
3. Send one write request.

Large writes are split into chunks (500 rows per transaction) so one big import never blocks other writers for long.

### 7.5 Migrations

- A migration is a list of **structured steps** from the schema agent (8.2). Go generates all DDL from these steps; the agent never writes DDL.
- A migration is one interactive write request running **one transaction**: the DDL, any data copy, the updates to dependent views, pipelines and forms, the change-log entry and the schema version bump. The dependents are validated inside the transaction. If any check fails, everything is rolled back. SQLite's DDL is transactional, so nothing remains.
- **Schema guard before commit.** Inside the transaction, after all steps, Go checks `sqlite_schema`: the `_jenab_*` entries are exactly as before, there are no triggers or SQL views, and every agent table has an explicit primary key. `PRAGMA foreign_key_check` must also be clean. If any check fails, the migration is rolled back (SPIKE-007).
- **Long migrations** (SPIKE-010). A migration is long when its steps rewrite or scan more than 500,000 rows in total; Go estimates this from row counts.
  - Measured on 1M / 5M rows:
    - table rebuild 4–13 s / 20–56 s
    - `DROP COLUMN` 1.3 s / 20 s
    - `CREATE INDEX` 1–1.4 s / 5–7 s
    - `copy_data` with `GROUP BY` 6 s / 54 s
    - `ADD COLUMN` without `CHECK`, and `RENAME COLUMN`: about 1 ms at any size
  - **Before a long migration:**
    - The approval card (8.8) shows the estimated time.
    - Go checks free space: at least 2× the size of the rewritten tables on the project drive, and 0.5× on the temp folder's drive (7.2). If there isn't enough, the migration is refused with that reason.
  - **During a long migration:**
    - The project shows a progress notice.
    - Reads continue. In the test there were no errors, and read p99 stayed under 250 ms.
    - Project writes wait in the queue. Background runs pause at their next write step, and the time they spend waiting doesn't count toward step timeouts.
    - Chat keeps working, since `chats.db` has its own writer.
  - **After a long migration:** the WAL is about 1.2× the rewritten data, so the writer runs `wal_checkpoint(TRUNCATE)` (under 0.1 s).
- The schema agent can **try** a migration: the same transaction, always rolled back, returning the full validation report. This holds the writer only for local database work; no network or LLM call happens inside the transaction. A try costs as much as the migration itself. A **long** try therefore runs on a `VACUUM INTO` copy on its own connection, so it doesn't hold the writer; the copy is deleted afterwards.
- Table rebuilds (changing a primary key, dropping constrained columns) follow SQLite's documented procedure: `PRAGMA foreign_keys = OFF` before the transaction, `PRAGMA foreign_key_check` before commit, foreign keys back on after.
- Before an approved destructive migration, a snapshot is taken with `VACUUM INTO` into `snapshots/`. It runs on a separate plain connection outside the reader pool, because `query_only` connections reject `VACUUM INTO` (SPIKE-001). It blocks neither readers nor the writer. The last 5 snapshots are kept.
- **Safe-copy rule** for every `VACUUM INTO` (snapshots, export): write to `<name>.partial`, `fsync` it, then rename it to the final name. An interrupted `VACUUM INTO` leaves a file with a hot `-journal`. When that file is opened, SQLite rolls it back to an **empty but valid database**, so a restore from it would silently wipe the project. This happened in all 20 crashes in SPIKE-008. With the rule, only complete snapshots ever carry a final name.
- After a migration, the schema version increases. Running pipelines re-validate at their next write step (6.6), open views re-validate on next load, and the project card refreshes.

### 7.6 Global limits

Two things are limited, separately:
1. **How much Jenab starts at once.** The user sets this, so parallel work can't overload the machine or spend too much. These are the limits below.
2. **How much a provider accepts.** Jenab can't know this ahead of time: plans differ, limits change, and another app may use the same key. So it reacts to the provider's answers instead, with the error kinds, waits and the pause per provider in 3.8.

```yaml
scheduler:
  max_parallel_runs: 4                # pipeline runs across all projects

llm:
  max_parallel_calls: 8               # background LLM calls; chat calls count but never wait
  provider_max_parallel_calls:        # per provider, for background calls (3.8)
    ollama: 1                         # local models answer one request at a time
  max_background_tasks_per_chat: 3    # 8.3
  max_subagents_per_chat: 5           # subagents running at once in one chat (8.3)
  turn_max_requests: 25               # model requests per user turn (8.3)
  system_turn_max_requests: 8         # model requests per turn the app starts (8.3)
  system_turn_max_tokens: 20000       # token cap for turns started by a finish notice (8.3)
```

- **Chat calls never wait for a slot.** A turn the user started, including its foreground subagents, sends its calls at once, so a busy app never freezes a chat. These calls count as in use, so background work gets fewer slots while chats are busy. There are few of them, since each turn starts with a message the user sends.
- **Background calls wait** in one first-come queue, for `max_parallel_calls` and for their provider's limit: pipeline steps, background tasks and subagents, *Refresh memory* (3.5), turns the app starts and Mother's delegations (8.6). Time in the queue doesn't count toward step timeouts (6.5).
- **Pipeline runs** wait for `max_parallel_runs` in their own first-come queue (6.2). This limit also bounds memory: each run can start a script process of up to 256 MB (6.5), and the project it writes to holds a 64 MB write cache (7.2).
- **Changing a limit:** in *Settings → Models → Limits* or in `config.yaml`. A change applies at once. More slots start waiting work right away; fewer slots let running work finish and start nothing new until the count is below the limit.
- The 10:1 rule (7.3) is for writes only, since chat calls never queue.

### 7.7 Scope

This design assumes one machine and one folder per project. Multi-machine or team access needs a sync layer or a server database and is out of scope for v1.

---

## 8. Agent tools

### 8.1 Main agent tools (v1)

Large results follow the preview-and-ref rules in 3.7.

**Data**

| Tool | Purpose |
|---|---|
| `query(sql, params)` | Read-only query; returns up to 200 rows or 4,000 tokens, plus the total count |
| `describe_table(name)` | Columns, types, keys, annotations, row count |
| `insert(table, rows)` | Small manual entries, at most 50 rows per call; recurring or bulk data goes through pipelines |
| `request_schema_change(goal)` | Hands a schema request to the schema agent (8.2) |

**Views and pipelines**

| Tool | Purpose |
|---|---|
| `save_view(config)`, `save_page(config)`, `save_pipeline(config)` | Validate and save; return all errors together |
| `read_config(id)` | The stored view, form, page or pipeline as YAML, for editing (SPIKE-021) |
| `open_page(id, filters)` | Open a page in this chat, e.g. after building or changing it (5.9) |
| `validate(config)` | Validate without saving |
| `dry_run(pipeline, inputs)` | Dry run (6.8) |
| `run_pipeline(id, inputs)` | Start a run; returns the run ID |
| `run_status(run_id)` | Status and step summaries; a finish notice follows, so there is no need to poll (8.3) |

**Memory and history**

| Tool | Purpose |
|---|---|
| `update_memory(scope, section, content, expected_revision)` | Edit user or project memory (3.3) |
| `update_session_notes(content, expected_revision)` | Edit this chat's session notes (3.4) |
| `search_history(query, scope, substring?)` | Full-text search over this chat or all chats in the project; returns snippets with message IDs. `substring: true` also finds words inside words, more slowly (2.3) |
| `read_messages(chat_id, from, to)` | Read specific messages |
| `read_ref(ref, offset)`, `search_ref(ref, query)` | Read or search a stored tool output or page |

**Web**

| Tool | Purpose |
|---|---|
| `web_search(query, limit, freshness, source)` | Search through the configured provider (`source: web`, the default) or a keyless source: `news`, `wikipedia`, `hn` (6.5) |
| `read_feed(url, match)` | Read an RSS or Atom feed; returns dated items (6.5) |
| `fetch_page(url)` | Fetch a page as readable text into `cache/pages/`; returns a preview and ref; follows 6.7 |

**Connections** (6.9)

| Tool | Purpose |
|---|---|
| `connect_api(id, title, base_url, auth, docs, rate_limit)` | Propose a connection; the user adds the key and confirms in a chat form |
| `call_api(connection, path, query)` | One GET through a connection; the response is stored as a ref with a preview, like `fetch_page` |

**MCP** (8.7, Phase 5)

| Tool | Purpose |
|---|---|
| `mcp_describe(server, tool)` | Input schema of an MCP tool |
| `mcp_call(server, tool, arguments)` | Call an MCP tool, with approval; the result is untrusted data |

**Workspace** (8.5, read-only)

| Tool | Purpose |
|---|---|
| `list_files(path, pattern)` | Files and folders under a path, with sizes |
| `read_file(path, from, to)` | Lines of a text file; large files as preview and ref |
| `search_code(pattern, path, glob)` | Regex search; returns file, line and a snippet per match |

**Bucket and links**

| Tool | Purpose |
|---|---|
| `bucket_list(prefix)` | List objects |
| `bucket_read(key)` | Text as preview and ref; images to vision-capable models |
| `bucket_put(key, content or from_ref)` | Store content, or keep a cached object under a permanent key |
| `bucket_delete(key)` | Delete a key (bytes kept until the sweep) |
| `save_link(url, title, note, tags)` | Save a link to the Resources panel |

**Other**

| Tool | Purpose |
|---|---|
| `subagent(task, inputs, background, skills)` | Run a task in an isolated context, optionally with skills (8.9); returns only the result. Subagents cannot change memory or the schema, or start subagents. With `background: true` it returns a task ID right away (8.3) |
| `task_status(id)` | Status of a background task; a finish notice follows, so there is no need to poll (8.3) |
| `ask_user(questions)` | Ask the user 1–4 questions with options; the turn waits for the answer (8.8) |
| `list_chats()` | IDs, titles, roles and status of the project's chats (8.6) |
| `create_chat(title, role, message, skills)`, `send_to_chat(chat_id, message)` | Mother chat only: start or continue a chat; returns a task ID (8.6) |
| `load_skill(name, file)` | Load a skill, or one of its extra files, into this chat (8.9) |

### 8.2 Schema agent

**Instructions:** the `database-design` and `migrations` skills (8.9), so the main chat can use the same text when the user discusses a design.

**Input:** the goal, the current schema with annotations, the dependents (views, pipelines and forms, with the columns they use), and row counts. Pages only point to views, so they rarely need changes. It can read each dependent's full config with `read_config` (8.1); it needs the full configs to rewrite them.

**Output:**

| Result | Contents |
|---|---|
| `applied` | Migration ID, new schema version, dependents updated |
| `rejected` | Reason |
| `needs_approval` | Plan in plain language, destructive operations, affected dependents, affected row counts |

**Migration steps.** The schema agent returns a list of steps, not SQL. Go validates each step, chooses the SQL (for example `ALTER TABLE … DROP COLUMN` or a full rebuild) and runs them in one transaction (7.5). Each step names its type in `op`, e.g. `{ op: add_column, table: news, column: { name: coin, type: TEXT } }`.

| `op` | Fields | Destructive |
|---|---|---|
| `create_table` | `table`, `columns` (name, type, `not_null`, `default`, `check`), `primary_key`, `unique` (a list of column lists), `foreign_keys` | no |
| `add_column` | `table`, `column` | no |
| `rename_table` | `from`, `to` | no |
| `rename_column` | `table`, `from`, `to` | no |
| `create_index` | `table`, `columns`, `name`, `unique` (`true` / `false`) | no |
| `drop_index` | `name` | no |
| `annotate_column` | `table`, `column`, `kind` (e.g. `object_ref`) | no |
| `insert_rows` | `table`, `rows` (seed data, at most 50 rows) | no |
| `copy_data` | `into`, `columns`, `from` (one read-only query, checked by the SQL guard, 2.2) | no |
| `rebuild_table` | `table`, the new definition as in `create_table`, and a column mapping | yes, if columns or rows are dropped |
| `drop_column` | `table`, `column` | yes |
| `drop_table` | `table` | yes |

Dependent updates (views, pipelines, forms) are sent in the same request as full configs and validated as in section 10.

**Guidelines:**

- Design for the goal as stated. When the user widens it (e.g. a second coin), migrate to a general table such as `prices(coin, …)` instead of adding parallel tables like `eth_prices`.
- Every table has an explicit primary key (2.2).
- Tables for pipeline data get `_run_id` and `_fetched_at`. Copied rows keep both.
- Existing rows get new values explicitly, through `copy_data` or the `rebuild_table` mapping. A column `default` is never used to label old rows, because it would also silently label new rows that lack a value.
- A key must fit the widened goal: when news is tracked per coin, the key is `(coin, url)`, not `url`.
- Columns that hold bucket keys are marked as `object_ref`.
- Never drop tables or columns, or change keys, without approval.
- Update all dependents in the same migration (7.5).

### 8.3 Turns and background work

Decided in SPIKE-023.

- **Parts:** a turn is a list of parts (2.3). The agent may write text before, between and after tool calls, e.g. answer the quick part of a request, then call a tool for the rest.
- **Background tasks:**
  - `subagent(task, inputs, background: true)`, `run_pipeline`, `create_chat` and `send_to_chat` (8.6) return an ID right away. `task_status(id)` and `run_status(run_id)` report progress.
  - At most 3 background tasks run per chat. They share `max_parallel_calls` (7.6).
- **Subagents:** at most `max_subagents_per_chat` (5) run at once in one chat, foreground and background together. A `subagent` call beyond that waits until one ends. The `subagent` tool's description states the limit, filled in from the setting, so the agent can plan around it. The text is the same for every model (1).
- **When a provider fails** (3.8):
  - **Turns** retry `rate_limited`, `overloaded` and `transport` errors by themselves. The chat shows the wait, e.g. "Gemini is rate limited, retrying in 42 s", with *Retry now* and *Cancel*, so it never looks frozen. After 10 minutes of failed tries, the turn stops with an error card and *Retry*. A retry is safe: tools run only after a complete answer (3.8), and text from a cut-off try is replaced.
  - **Subagents** try up to 3 times within 2 minutes, then fail. The parent agent gets the error with its kind and wait, e.g. `rate_limited, retry after 60 s`, and decides whether to wait, try again or do the work itself.
  - `quota` and `request` errors are never retried. The chat says what the user can do, e.g. add credit or pick another model.
- **Finish notice:**
  - When a background task the agent started finishes or fails, the orchestrator starts a new turn in that chat with a notice part: `[task t_12 finished: …]`.
  - If the chat is in a turn, the notice waits until that turn ends.
  - Notices that are due together share one turn.
  - The notice is dropped only if the agent has already answered after seeing the final status.
  - Scheduled runs don't start turns; they only add the notice described in 3.1.
- **Polling:** from the second `task_status` or `run_status` call for the same ID in a turn, the orchestrator answers "still running; a notice will follow" without checking again. In SPIKE-023, polling was the main failure: models polled until the request cap, and the user never got the rest.
- **Request cap:** a turn makes at most `turn_max_requests` model requests (`system_turn_max_requests` for turns the app starts; 7.6). When a turn reaches the cap, one last request goes out without tools, so the user always gets an answer. The starting values are tuned with the benchmark.
- **System-started turns:**
  - Such a turn is marked as started by the app and has its own *Undo turn*.
  - It has lower caps (`system_turn_max_tokens`, `system_turn_max_requests`).
  - Changes that need approval wait for the user.
- **User messages during a turn:**
  - The user can write while the agent works.
  - The message is added to the running turn at the next step boundary, after the tool results of the current model response, and the agent sees it on its next request. It can't go earlier: the OpenAI message order has no place for a user message between a tool call and its result.
  - *Stop* cancels the turn and the subagents it started. Pipeline runs and turns in other chats that it started keep going; each has its own *Stop*.
  - Every tool call still open gets a `tool_result` "cancelled by user", so the history stays valid for every provider (both reject a tool call without a result).
  - Text streamed before *Stop* is kept and marked as stopped (3.8).
  - A write the writer has already taken finishes; one still waiting in the queue is skipped (7.3).
  - *Undo turn* covers everything in the turn, including after the new message.
- **Prompt rule for requests with several parts:**
  1. Answer the parts that need no tools first.
  2. Start slow work in the background.
  3. Say what is still running.
  4. Give the rest in the finish turn, without repeating what was already answered.
  - The rule is the same for every model (1). In SPIKE-023 it raised the finish-turn pass rate from 65 % to 88 % and cut the mean prompt tokens per run by 20 %.

### 8.4 Developer tools

The scenario runner and the turn inspector use the real orchestrator, context builder and validation, not a separate playground, so they can't drift from the app.

- **Scenario runner** (headless, from Phase 1):
  - A scenario is a YAML file with:
    - a fixture project
    - scripted user messages, with an optional `at` time for messages sent during a turn
    - fake or recorded tool results, with delays
    - assertions, e.g. `text_before_tool`, `tool_called`, `config_valid`, `max_prompt_tokens`
  - It runs every scenario on the test model set: the free development models, plus any frontier model the developer has a key for. It reports pass rates, repair rounds, tokens and time per model.
  - The SPIKE-021 config tasks and the Phase 2 benchmark (PROPOSAL 11) are scenario sets for it.
- **Turn inspector** (in the app, behind *Settings → Developer*):
  - For each turn it shows:
    - the parts timeline
    - tool calls with timing and result size
    - background tasks
    - the context blocks the builder sent (3.1), with token counts and cache hits
  - It is read-only. Secrets are redacted as in 6.7.
  - When the developer tools are on, each turn's footer has *Inspect*, which opens the inspector at that turn.
- **Runtime panel** (in the app, behind *Settings → Developer*): everything running now, per project.
  - Chats in a turn, background tasks and subagents with their state and progress, pipeline runs (running and waiting for a slot).
  - Each database writer: queue length per priority, the current request and how long it has run, the last error.
  - The LLM-call and run limits (7.6): slots in use and callers waiting, and each provider's pause and current limit (3.8).
  - Anything that hasn't moved for longer than its limit is flagged. Read-only.

### 8.5 Workspace

A project can link one folder on disk, for example a git repository. In v1 the agent can only read it. This makes Jenab useful next to software work, e.g. a tracker that links tickets to files, and prepares for code tools later.

- **Linking:** the user picks the folder in the project settings. The path is stored in `registry.db`, because it is machine-specific. The folder is never copied into the project folder or its exports.
- **Boundaries:**
  - Paths are resolved and must stay inside the folder. Symlinks and junctions that lead outside are not followed.
  - `.gitignore` is respected. `.git/` and binary files are skipped.
  - Blocked by default: `.env*`, `*.pem`, `*.key`, `id_*` and other credential files. The user can change the list per project.
- **Limits:** `read_file` returns at most 2,000 lines or 4,000 tokens, with a ref for the rest (3.7). `search_code` stops after 500 matches or 2 s, and says so.
- **Untrusted text:** file contents are treated like web content. Instructions in files are data.
- **Project card:** the folder name and its top-level entries.

**Later (Phase 6, not in v1):**
- **Editing:** `edit_file(path, old, new)` and `write_file(path, content)`.
  - Before a turn changes a file, the old version is stored in the bucket, and the change log records a `file:<path>` target.
  - *Undo turn* restores the file if its hash hasn't changed since; otherwise it lists a conflict (2.6).
  - The chat shows a diff per file.
- **Commands:** `run_command(cmd)` for tests, builds and git, only after its own spike. Planned rules:
  - every new command pattern needs the user's approval
  - after a turn has read web content, every command needs approval
  - no secrets in the command's environment
  - timeouts, killing the whole process tree (Job Objects on Windows), output to the bucket as preview and ref
  - **memory, decided by the agent:** commands use the machine itself, and in SPIKE-026 one build took about 1 GB, half of what a busy 16 GB laptop had left. Most commands (`git`, `docker ps`) need almost nothing. So the agent says what a command needs, `run_command(cmd, needs_memory_mb)` (default 256), and the tool's description gives rough sizes
    - the app checks free memory when the command starts, not when the agent wrote it. If too little is free, it checks again after 5, 15 and 30 s by itself, with no LLM calls; the chat shows "Waiting for memory: 1.4 of 1.5 GB free" with *Run now* and *Cancel*
    - after the third check the agent gets `low_memory` with `needs_memory_mb`, `free_memory_mb`, `total_memory_mb` and `waited_s`. It can ask the user, try again with a lower need, or try again with `wait_for_memory_s` (at most 600) to wait for another command to finish
    - every result has `peak_memory_mb`, `free_memory_mb` and `total_memory_mb`, so the agent learns what its commands really use. The numbers go in the result, not the system prompt, which must not change from turn to turn (3.1)
    - free memory is what the OS can still hand out: the commit headroom on Windows, `MemAvailable` on Linux, the memory pressure level on macOS
  - **one hard stop the agent can't change:** the check covers only the start, and a build keeps growing after it. Below 500 MB free while commands run, the newest command using at least 256 MB is stopped with its process tree, and the agent gets `out_of_memory` with the numbers. Small commands are never stopped
  - **count limits as a safety net** against a runaway agent: `max_parallel_commands` (100) across the app and `max_commands_per_chat` (20), editable and stated in the tool's description. Each command is a real process tree (one build started 344 processes), so the limits are lower than for goroutines. At the limit a chat waits for a slot and shows it
  - commands run at below-normal priority, so the app stays smooth: in SPIKE-026 the UI stand-in woke at most 16 ms late, against up to 133 ms at normal priority
  - N-54's minimum system doesn't cover builds, which need about 1 GB each on top
  - pipelines never run commands
- **Not planned:** parity with dedicated coding agents (language servers, worktrees, hooks).

### 8.6 Mother chat and chat roles

Every project has one **Mother chat**: the project's home, which directs the other chats. Other chats can have a **role**: a lasting job such as *Reviewer* or *ETH tracker*.

**Titles and roles**
- **Title:** one field.
  - It is generated at first, and stays fixed once the user or Mother sets it.
  - Titles are unique within a project, so `@Reviewer` works in the UI. Tools use chat IDs.
- **Role:** optional text, at most 500 tokens, added after the system prompt (3.1, block 1).
  - It says what the chat is for and how it works, e.g. "Review every new pipeline for cost and missing error handling."
  - The user or Mother writes it. A change is recorded like a memory edit, with undo.
- **Default page:** optional. A role chat can open with a page, e.g. the *ETH tracker* with the ETH page (5.9).

**Mother chat**
- **Place:** created with the project, pinned at the top of the chat list in its own colour.
  - The colour is a gradient of the `purple`, `blue` and `green` tones (5.11): a border on Mother's chat row and composer, and a filled tile in the rail while a page is open (5.12).
  - The gradient turns slowly while Mother is in a turn, and stays still when the OS asks for reduced motion. It can't be archived or deleted, only cleared.
- **Context:** the project card is followed by a **chat list** (3.1, block 4):
  - ID, title, the first line of the role, status (idle / in a turn / waiting for the user), the number of its background tasks still running (8.3), last activity, and the first lines of the session notes.
  - Example row: `c_07  ETH tracker  in a turn · 2 tasks running · last activity 14:02`.
  - It is refreshed at cuts, like the project card; changes in between arrive as notices.
- **Notices:** scheduled-run notices and failure notifications are also shown in Mother.
- **Tools** (only Mother has them):
  - `create_chat(title, role, message, skills)`: a new chat with a role, optionally the skills it needs (8.9), and a first message
  - `send_to_chat(chat_id, message)`: continue an existing chat
  - Both return a task ID right away, like a background subagent (8.3). When the target chat's turn ends, Mother gets the finish notice with its last reply.

**Chat, role chat or subagent**

| | Mother chat | Chat (with or without a role) | Subagent |
|---|---|---|---|
| Created by | The app, with the project | The user, or Mother | Any chat, during a turn |
| In the chat list | Pinned at the top | Yes | No; shown inside the parent turn |
| The user can write in it | Yes | Yes | No |
| Lives | As long as the project | Until archived | One task |
| Context | Project card, memory, chat list, own history | Role, project card, memory, own history | The task and its inputs |
| Sends to other chats | Yes | No | No |
| Changes memory or schema | Yes | Yes | No |
| Starts subagents | Yes | Yes | No |
| Use it for | Overview and directing work | A lasting job, or work the user wants to follow | One-off work, e.g. reading 20 pages |

**Rules**
- **Only Mother sends.** Other chats answer only through the finish notice. They can't send to other chats, so messages can't loop.
- **Subagents** (not chats, so they have no row in `chats`):
  - **One level:** a subagent can't start another subagent.
  - **Transcript:** a subagent's own messages are not written to `chats.db`, so `search_history` doesn't find them. When it ends, its transcript is stored in the bucket as JSON, and the parent's `tool_result` points to it with `ref` (3.7). The parent turn shows its steps from there, and so does the turn inspector (8.4).
  - **Undo:** the subagent's changes use the parent's message as their source (`message:<id>` of the `subagent` call), so *Undo turn* on the parent turn reverts them, even for a background subagent that finishes after the turn.
- **Visible:** a message from Mother appears in the target chat marked *from Mother*. The user can read and write in that chat at any time; a user message during a turn follows 8.3.
- **Limits:** turns started by Mother are system-started turns (8.3). They count against Mother's 3 background tasks and `system_turn_max_tokens`, and changes that need approval wait for the user.
- **Busy chats:** if the target chat is in a turn, the message waits until the turn ends.
- **Reading:** every chat can still read the others with `list_chats`, `search_history(scope: project)` and `read_messages`.
- **Model-neutral:** the guidance on when to delegate is part of Mother's system prompt and is the same for every model (1).
- **Delegation guidance v1** (SPIKE-023):
  - Answer yourself when the project card, the chat list or a quick tool call is enough.
  - If an existing chat's role covers the job, send it there with `send_to_chat`, with the context it needs: what to do, which pipeline, view or data, and what to report back.
  - Create a chat with a role only for a lasting job: something done again and again, or that the user wants to follow.
  - For long one-off work, e.g. summarising many pages, use a background subagent.
  - When a reply arrives in a finish notice, pass on what matters in your own words. Don't send the work again or repeat what you already said.
  - Known gap: some models do a role chat's job themselves instead of sending it. That costs tokens but is safe. It is rechecked before Phase 5 with a frontier model and each chat's full role in the chat list.
- **Later idea, not planned:** a read-only `list_activity()` tool with the details of all running work in the project. It is added only if the Phase 5 scenario runs show that Mother chooses worse without it, since every tool costs tokens in each request and invites polling (SPIKE-023).

**Phases:** titles, roles, and the pinned Mother chat with its chat list in Phase 1; `create_chat` and `send_to_chat` in Phase 5, with the subagent tool.

### 8.7 MCP servers

Draft, for Phase 5. SPIKE-024 tests it first.

The user can add MCP servers, e.g. for Blender, Unity or GitHub. Their tools become tools of the agent, so a project can keep structured data about work done in those apps: items and stats, levels, scenes, assets, bugs.

```yaml
version: 1
id: blender
title: Blender
transport: stdio              # or http
command: uvx                  # stdio: the command and arguments the user gives
args: [blender-mcp]
env: { }                      # values may come from the keychain
# url: https://mcp.example.com/mcp                               (http)
# auth: { in: header, name: Authorization, prefix: "Bearer " }   (http)
```

- **Adding:**
  - Only the user adds servers, in *Settings → MCP servers*. The agent may suggest one in chat, but can't add it.
  - Jenab never downloads or installs a server. The full command is shown before the first start.
  - Servers are stored with the connections in `registry.db` and enabled per project. Keys go to the keychain as `jenab:mcp:<id>`, and the process gets only the `env` listed.
- **Tools in the context:**
  - The project card lists each enabled server with its tool names and one-line descriptions.
  - `mcp_describe(server, tool)` returns a tool's input schema, and `mcp_call(server, tool, arguments)` calls it. Jenab checks the arguments against the schema first, with errors as in 10.
  - This keeps the tool list and the prompt cache stable, however many servers are enabled. SPIKE-024 compares it with giving each MCP tool as a native tool.
- **Approval** (approval card, 8.8):
  - Each tool needs approval on first use: *once*, *always in this project*, or *deny*. This is the Standard level; Strict and Auto change it (8.8).
  - A tool that doesn't declare `readOnlyHint: true` asks every time, unless the user allows it always in the settings. The hint is only trusted this far because the user chose the server.
  - After a turn has read web content, every tool that isn't read-only asks again, as for commands (8.5).
- **Results:** untrusted data, like web pages. Text over `tool_preview_tokens` becomes a preview and ref (3.7); images go to vision-capable models; results over 5 MB are cut.
- **No undo:** changes made in Blender or Unity are outside Jenab. The tool chip says "can't be undone", and *Undo turn* skips them.
- **Processes:**
  - A stdio server starts on first use, one process per project. It stops when the project closes, or after 10 minutes without calls.
  - It is stopped with its whole process tree (a Job Object on Windows), since launchers such as `uvx` and `npx` start child processes.
  - Its stderr goes to a log in *Settings → Developer*. After a crash, the next call restarts it once.
- **Time limit:** 60 s per call by default, configurable per server. Progress messages are shown on the tool chip.
- **Not in pipelines yet:** a later `mcp.call` step (v1.1) may use only the tools the user marks as safe for pipelines.
- **Not yet:** MCP resources, prompts and sampling; Jenab as an MCP server, so that other agents can query a project's tables.
- **Model-neutral:** the same tools and descriptions for every model (1).

### 8.8 Approvals and questions

Two chat parts ask the user something and make the turn wait: the **approval card** and the **question form**.

**Approval card** (`approval` part). Every approval in Jenab uses it:

| Asked for | Options | Details shown |
|---|---|---|
| A new host (6.7) | Allow for this project, Deny | The host, and the pipeline or tool that needs it |
| A destructive migration (7.5, 8.2) | Approve, Deny | The plan in plain language, destructive steps, affected dependents and row counts; the estimated time for long ones |
| Starlark code (6.5) | Approve, Deny | The code and its hash |
| A connection (6.9) | Save, Cancel | Title, host, where the key goes, and the key field |
| An MCP tool (8.7) | Once, Always in this project, Deny | Server, tool and arguments |
| A command (Phase 6, 8.5) | Once, Always for this pattern, Deny | The command and the folder |

- **Content:** what is asked, why (one line from the agent) and the risk. Details expand.
- **Options:** *Always* appears only where the rules allow it. *Deny* can carry a short note, which goes back to the agent with the result.
- **Waiting:**
  - The turn waits. No transaction is held while waiting (7.4), and other chats, pipelines and forms keep working.
  - If the chat isn't on screen, it gets a badge in the chat list, and a desktop notification is shown.
  - If the card is scrolled out of view, a bar above the composer shows what is waiting, with a button that scrolls to it.
  - The card stays open until the user answers or presses *Stop*. If the user writes a message instead, the card is closed as *Deny*, with the message as the note.
- **Record:** decisions are stored in `_jenab_approvals` with what, when, the answer and who gave it, as a source (2.5): `user`, or `auto` for the Auto level. The card then shows the result, e.g. "Approved 10:14".
- **Turns started by the app** (8.3) wait the same way. Scheduled runs never ask: everything they need is approved when the pipeline is saved (10).

**Approval levels.** The user picks how much is asked: **Strict**, **Standard** (the default) or **Auto**.
- **Where:** per project, stored in `_jenab_meta`. The default for new projects is in `config.yaml` (`approvals.default_level`).
- **Shown:** a chip in the composer, "Approvals: Standard ▾", next to the model picker. A change applies from the next tool call and adds a notice to the chat.
- **Scope:** every chat of the project uses it, including role chats working for Mother and turns started by the app.

| Asked for | Strict | Standard | Auto |
|---|---|---|---|
| A new host | Ask | Ask | Auto* |
| A schema change that deletes nothing | Ask | No ask | No ask |
| A destructive migration | Ask | Ask | Ask |
| Saving a pipeline with a schedule | Ask | No ask | No ask |
| A *Refresh memory* edit (3.5) | Ask | No ask; asks if it changes a section the user wrote | Same as Standard |
| Starlark code | Ask | Ask | Auto |
| An MCP tool that only reads | Ask on first use | Ask on first use; *Always* allowed | Auto |
| An MCP tool that changes something | Ask every time; no *Always* | Ask; *Always* allowed | Auto* |
| A command (Phase 6) | Ask | Ask for each new pattern | Ask for each new pattern |

\* Only if the turn hasn't read untrusted content (web pages, files, MCP results) since the user's last message; otherwise it asks. This keeps a web page from steering the agent into sending data somewhere.

- **The same at every level:**
  - Connections: the user pastes the key in the card.
  - Private network addresses stay blocked, unless the user adds an exception in project settings (6.7).
  - A snapshot before every destructive migration (7.5), and the migration always asks.
  - Scheduled runs never ask.
- **Auto-approved** actions show a chip in the chat, e.g. `Auto-approved: api.coingecko.com · revoke`, and are recorded in `_jenab_approvals` like manual ones.
- **Model-neutral:** the level changes what the orchestrator asks, never the prompts or tools (1).

**Question form** (`question` part), from `ask_user(questions)`:

```yaml
questions:
  - header: Currency
    question: Which currency should prices use?
    options:
      - { label: EUR, description: "Converted from USD every day", recommended: true }
      - { label: USD, description: "As the price API returns them" }
    multi: false
```

- **Shape:** 1–4 questions, each with a short header and 2–4 options. An *Other* field for free text is always added. With `multi: true`, several options can be picked.
- **Answer:** it returns as the tool result, e.g. `{ "Currency": "EUR" }`, with any note.
- **Waiting:** as for the approval card. A message written instead is the answer.
- **Text:** questions and options are shown as plain text, with no links or HTML.
- **Guidance** (the same for every model, 1):
  - Ask only about choices that are the user's to make and change what the agent does next.
  - Don't ask about what the agent can find out, or decide with a sensible default; then decide and say so.
  - Don't ask to confirm a plan the user has already given.
- **Who can ask:** chats, including role chats working for Mother; Mother's chat list then shows "waiting for the user". Subagents can't ask; they return the open question in their result. Pipelines never ask.


### 8.9 Skills

Decided in SPIKE-025.

A **skill** is a Markdown file with instructions for one kind of work, e.g. designing tables or writing a pipeline. Chats load a skill only when they need it, so every request stays short and provider caches stay valid.

```markdown
---
name: migrations
description: "Creating and changing tables: migration steps, destructive changes, updating dependents. Load before request_schema_change."
load_with: [request_schema_change]
---
Instructions and examples.
```

| Field | Required | Description |
|---|---|---|
| `name` | yes | Unique; lowercase letters, digits and `-` |
| `description` | yes | One line, at most 200 characters. The agent picks skills by it, so it says what the skill covers and when to load it: "Load before `<tool>`" or "Load to …" |
| `load_with` | no | Tools whose first call in a chat also loads the skill, if it isn't loaded yet |
| body | yes | At most 3,000 tokens. Longer material goes in extra files next to it, read with `load_skill(name, file)` |

- **Where skills come from:**
  - **Built-in:** written and tested by us with the scenario runner (8.4), the same for every model (1).
  - **User skills:** in *Settings → Skills*, for all projects.
  - **Project skills:** in `skills/` in the project folder, so they travel with exports.
  - Names are unique. A user or project skill can't replace a built-in one.
- **Trust:** skills are instructions, so only built-in skills and skills the user wrote or accepted are used.
  - Jenab never downloads skills. Importing a file shows it in an approval card (8.8) first.
  - Later, the agent may propose a skill, e.g. from *Refresh memory*. It is saved only when the user accepts it.
- **How a chat gets a skill:**
  1. **Skill list:** block 1 (3.1) ends with a `## Skills` list, one line per skill: `- name: description` (about 45 tokens each). It shows only the skills this chat can use, so `delegation` is in Mother only. It changes only when skills are added or removed.
  2. **`load_skill(name)`:** the agent loads a skill when it needs one. The text arrives as the tool result.
  3. **`load_with`:** the first call to one of those tools in a chat also loads the skill, if it isn't loaded yet, returned with the tool's result. It is a safety net for models that don't load a skill on their own, and costs nothing when they do.
     - When `request_schema_change` touches dependent pipelines or views, it also loads `pipelines` and `config-guide`.
  4. **Roles:** a chat can start with skills. Mother passes them in `create_chat(title, role, message, skills)` (8.6), and the user can add them from the chat header. `skills` is optional: the new chat has its own skill list, so it can load a skill Mother missed.
- **Staying loaded:**
  - The chat's loaded skills are stored in `chats.skills`.
  - At the next cut of the history window (3.6), they move into block 1, after the role, so they survive cuts. Block 1 changes only at cuts, which keeps the cache valid in between.
  - Starting limit: 6 skills or 10,000 tokens per chat. Beyond that, `load_skill` returns an error that lists what is loaded.
- **Subagents:** `subagent(task, inputs, background, skills)` starts with those skills loaded. The schema agent (8.2) is a subagent with `database-design` and `migrations`.
- **Shown:** a loaded skill is a chip in the chat, `Skill loaded: migrations`.
- **Skills and memory:** memory holds facts and intent and is always in the context (3.3). Skills hold procedures and are loaded when needed.

**Built-in skills (v1):**

| Skill | Content | `load_with` |
|---|---|---|
| `config-guide` | Views, pages and forms (5), with the recipes from SPIKE-021 | `save_view`, `save_page` |
| `pipelines` | Steps, expressions, `for_each`, schedules, dry runs (6). The description says to load it for anything that runs on a schedule | `save_pipeline`, `dry_run` |
| `database-design` | Tables, keys, annotations, when to widen a table (8.2 guidelines) | – |
| `migrations` | Migration steps, destructive changes, updating dependents (7.5, 8.2) | `request_schema_change` |
| `sql-queries` | The SQLite dialect, window functions, the SQL guard rules (2.2) | – |
| `web-research` | Search sources, feeds, untrusted content (3.7, 6.5) | `web_search`, `read_feed` |
| `delegation` | Mother's delegation guidance v1 (8.6) | – (always loaded in Mother) |

**SPIKE-025 results** (the SPIKE-021 guide v2 split into skills, on the three development models):
- Models loaded the right skill before every first write (60/60 runs) and never loaded one they didn't need.
- No model loaded `web-research` before searching (0/12), and only one loaded `sql-queries` to explain a failed query. `load_with` covered the web case in 6/6 runs.
- Configs were as valid as with the whole guide on nemotron and glm. gemma did worse on the "add ETH" migration task (valid 0/2 against 2/2); the scenario runner (8.4) rechecks this with at least 5 reps before Phase 2.
- The first request of a turn drops from 4.3–4.7k tokens to 1.7–2.2k. Chats that need no skill cost 54–61% less.
- Mother picked the right skills for single-purpose roles (12/12) but missed one skill for combined roles (e.g. `pipelines` for "watch gold news daily"). The descriptions above were reworded for this; the scenario runner rechecks them.

**Phases:** the skill format, the skill list, `load_skill`, `load_with` and `config-guide` in Phase 1; the schema agent on skills in Phase 2; skills in `create_chat` and `subagent`, and user and project skills, in Phase 5.

---

## 9. Full example — Bitcoin project

### 9.1 Tables (created by the schema agent)

```
btc_prices(date TEXT PRIMARY KEY, price REAL, _run_id TEXT, _fetched_at TEXT)
btc_news(url TEXT PRIMARY KEY, date TEXT, title TEXT, reason TEXT, _run_id TEXT, _fetched_at TEXT)
watch_keywords(id INTEGER PRIMARY KEY, keyword TEXT NOT NULL UNIQUE)
```

### 9.2 Pipeline

The price URL and the feed URLs are placeholders. The published example project uses a keyless public price API and suggests 2–3 crypto news feeds, which the user can change. News comes from feeds, not web search, so the example works without any key except the LLM's (SPIKE-020). `web.search` with `source: news` (GDELT) can be added as a second, best-effort source.

```yaml
version: 1
id: daily_btc
description: Daily BTC price and the 5 news items most likely to affect it
trigger:
  schedule: "0 8 * * *"
  timezone: UTC
  catch_up: once
  manual: true
steps:
  - id: price
    use: http.get
    with:
      url: https://api.example.com/btc/daily

  - id: save_price
    use: db.insert
    with:
      table: btc_prices
      on_conflict: upsert
      row:
        date: ${{ today() }}
        price: ${{ steps.price.json.close }}

  - id: keywords
    use: db.query
    with:
      sql: SELECT keyword FROM watch_keywords

  - id: news
    use: feed.read
    with:
      urls:
        - https://news.example.com/crypto/rss
        - https://markets.example.org/feed.xml
      match: ${{ map(steps.keywords.rows, .keyword) }}
      since: 1d
      limit: 40

  - id: top5
    use: llm.select
    with:
      items: ${{ steps.news.items }}
      count: 5
      instruction: Pick the items most likely to affect the Bitcoin price today.
      add: { reason: string }

  - id: news_rows
    use: transform.map
    with:
      items: ${{ steps.top5.items }}
      fields:
        url: ${{ item.url }}
        title: ${{ item.title }}
        reason: ${{ item.reason }}
        date: ${{ today() }}

  - id: save_news
    use: db.insert_many
    with:
      table: btc_news
      on_conflict: ignore
      rows: ${{ steps.news_rows.items }}
```

### 9.3 Views

```yaml
version: 1
id: btc_price_table
title: Bitcoin price
type: table
query: |
  SELECT date, price FROM btc_prices
  WHERE date >= :from_date
filters:
  - { param: from_date, control: date, label: "From", default: "today-90d", required: true }
default_sort: { field: date, direction: desc }
columns:
  - { field: date, label: "Date", format: date }
  - { field: price, label: "Price", format: currency, format_options: { currency: USD } }
actions:
  - { label: "Refresh now", run_pipeline: daily_btc }
```

```yaml
version: 1
id: btc_price_chart
title: Bitcoin price chart
type: chart
chart_type: line
query: SELECT date, price FROM btc_prices
x: { field: date, label: "Date", format: date }
y:
  - { field: price, label: "Price (USD)" }
```

```yaml
version: 1
id: btc_news_table
title: Bitcoin news
type: table
query: SELECT date, title, reason, url FROM btc_news
default_sort: { field: date, direction: desc }
columns:
  - { field: date, label: "Date", format: date }
  - { field: title, label: "Title", format: text }
  - { field: reason, label: "Why it matters", format: text }
  - { field: url, label: "Link", format: url }
```

```yaml
version: 1
id: btc_today
title: Today
type: stat
query: |
  SELECT price, price / LAG(price) OVER (ORDER BY date) - 1 AS change
  FROM btc_prices ORDER BY date DESC LIMIT 1
value: { field: price, format: currency, format_options: { currency: USD } }
change: { field: change, format: percent }
```

```yaml
version: 1
id: keyword_list
title: News keywords
type: table
query: SELECT id, keyword FROM watch_keywords
default_sort: { field: keyword, direction: asc }
columns:
  - { field: keyword, label: "Keyword", format: text }
rows_from: { table: watch_keywords, key: id }
row_actions:
  - { label: "Delete", delete: true, confirm: true }
```

```yaml
version: 1
id: add_keyword
title: Add news keyword
type: form
fields:
  - { field: keyword, label: "Keyword", control: text, required: true }
submit:
  action: insert
  table: watch_keywords
```

All six views are shown on the `bitcoin` page in 5.9. Its `from_date` filter binds to `btc_price_table`, whose own filter is then hidden.

### 9.4 Project memory after setup

```yaml
goals: Track the daily Bitcoin price and the news most likely to move it.
preferences: Update every morning at 08:00 UTC. Prices in USD.
```

The list of keywords is not in memory; it is data and lives in `watch_keywords`.

### 9.5 Adding Ethereum

The user says *"also track Ethereum."* The schema agent returns `needs_approval` with this plan:

1. Create `coins(symbol TEXT PRIMARY KEY)` with `BTC` and `ETH`.
2. Create `prices(coin TEXT, date TEXT, price REAL, _run_id TEXT, _fetched_at TEXT, PRIMARY KEY (coin, date))` and copy all `btc_prices` rows with `coin = 'BTC'`, keeping `_run_id` and `_fetched_at`.
3. Rebuild `btc_news` with `coin TEXT NOT NULL` and the key `(coin, url)`, setting `coin = 'BTC'` for existing rows in the mapping, then rename it to `news`. The new key lets one article be stored for both coins. There is no column default; the pipeline sets `coin` on every row.
4. Update the pipeline:
   - The price step runs once per coin, with `for_each` over the rows of a `db.query` on `coins`.
   - The feeds' `match` also includes the coin names.
   - `llm.select` adds a `coin` field next to `reason`, and `transform.map` puts it in each row.
5. Update the views:
   - The price table and chart read from `prices`, with a coin filter (`options: { query: "SELECT symbol FROM coins" }`). The chart uses `series_by: coin`.
   - The news table reads from `news` and shows the coin.
   - `btc_today` reads from `prices` for one coin.
   - The `bitcoin` page gets a page filter `coin` (`options: { query: "SELECT symbol FROM coins" }`), which binds to every block that has `:coin`. Its rows don't change, because it only points to views.
6. Drop `btc_prices` (destructive, hence the approval).

As migration steps (8.2), items 1–3 and 6 are seven steps: `create_table` + `insert_rows`, `create_table` + `copy_data`, `rebuild_table` + `rename_table`, and `drop_table`. Items 4 and 5 are dependent updates (one pipeline and three views), sent in the same request.

After approval, a snapshot is taken, and the seven steps and the dependent updates run in one transaction (7.5). Nothing already collected is lost.

---

## 10. Validation before saving

Every view and pipeline passes these checks, in order:

1. **Parse** YAML/JSON and check it against the format's JSON Schema. For **pages**, also check that:
   - every `view` exists
   - every row has 1–4 blocks, with spans adding up to at most 12, also inside cards; no card inside a card
   - every page filter binds to a parameter of at least one block
   - every `filter` block names a page filter, and each page filter is shown at most once
   - every `button` and action target exists
   - tones are from the list in 5.11
   - a missing `image` key is a warning, since a pipeline may create it later
2. **Step check** — every `use` exists in the catalog, and every `with` matches that step's input schema.
3. **Reference check** — every `steps.<id>` points to an earlier step; every `inputs.<name>` is declared; every `connection` exists and has its key in the keychain; every `open_form` and `run_pipeline` target exists; form fields for `run_pipeline` match the pipeline's declared inputs.
4. **Schema check** — every table and column used by `db.*` steps, forms and `rows_from` exists in the project database; no `_jenab_*` table is referenced; every `NOT NULL` column without a default is covered by a required form field; `image` and `file` formats and `file` controls use object-reference columns.
5. **SQL check** — every query first passes the text check (2.2, layer 1), and only then is prepared with SQLite (without running it) to catch syntax errors and unknown columns and to run the EXPLAIN check (layer 2). View `columns[].field`, `x`, `y`, `series_by` and the `stat` fields are checked against the **result columns of the prepared query**, not against table columns, so aliases like `AVG(price) AS avg_price` work.
6. **Expression check** — every expression compiles with the allowed functions; type errors are reported where types are known.
7. **Trigger check** — cron expressions parse, and time zones are valid IANA names.
8. **Network check** — literal hosts are listed for approval; `api.get` paths stay on the connection's host; only `http` and `https` URLs.
9. **Optional dry run** — see 6.8.

All errors are returned together, each with its path, so the LLM can fix everything in one attempt.

**Repair loop** (SPIKE-021). It is the same for every model (1).
- **Rounds:** after a failed save, the agent gets at most 4 more attempts per config in the turn. Then it stops and shows the user the remaining errors.
- **Measured:** 50 % of configs from the free test models were valid on the first try, and 81 % after repairs. Every run that hit the limit was repeating the same error.
- **Hints:** YAML errors carry a fix hint, e.g. "quote this value: it contains `: `".
- **Early stop:** if a turn ends right after a failed save that wasn't retried, the orchestrator adds one notice: "save_pipeline daily_btc failed validation (3 errors) and was not saved. Fix and save it, or tell the user why not." This happens at most once per turn.
- **Network check:** a `web.search` with `source: web` fails validation when no search provider is set up (6.5).

**Schema check** (SPIKE-013):
- **Validator:** JSON Schema 2020-12 with `santhosh-tekuri/jsonschema/v6`, `AssertFormat()` on, and custom formats `cron`, `iana-tz`, `go-duration` and `http-url`.
- **Which errors:** only the leaf errors are kept, and a failed `anyOf` counts as one error.
- **Where each error points:**
  - Unknown fields, missing fields and wrong whole objects point at the key.
  - All other errors point at the value.
- **Message form:** `line 4:13 trigger.schedule: '0 8 * *' is not valid cron: needs 5 fields`.
- **Checked in Go instead:** unique step IDs, and `propertyNames`, which v6.0.3 reports with a wrong path. Warnings (for example, `ORDER BY` in a table query) are returned too but do not block saving.

---

## 11. Not in v1

- A built-in keyless web search (SPIKE-020: none that is allowed worked)
- UI code written by the LLM (HTML, CSS, React); pages are configs only (5.9)
- A drag-and-drop page editor (the user asks the agent to change a page)
- A translated UI or a mirrored right-to-left layout; only right-to-left text is supported (5.8)
- `http.post`, and connection auth other than a key in a header or query (e.g. OAuth)
- Parallel branches (`depends_on`) and a visual pipeline editor
- Event triggers (e.g. run when a table changes)
- SQLite triggers, SQL views and virtual tables in agent tables
- Agent-written DDL (migrations are structured steps, 8.2)
- Schema graph view (planned for v1.1)
- Pipelines that read or write across projects
- Team or multi-machine access to the same project
- S3-compatible remote bucket backends and presigned links
- Editing files in the workspace and running commands (Phase 6, 8.5)
- MCP tools in pipelines (planned for v1.1), MCP resources, prompts and sampling, and Jenab as an MCP server (8.7)
- A gallery view for images
- Automatic *Refresh memory*, e.g. when a chat goes idle or at a cut once enough unread turns build up
