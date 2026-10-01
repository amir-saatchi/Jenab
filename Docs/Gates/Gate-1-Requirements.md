# Gate-1 — Requirements
**Status:** Draft (ready for review)

## Purpose
A numbered list of what v1 must do, so design, tickets and tests can refer to it.

## How to read it
- **ID:** `R-xx` for features, `N-xx` for non-functional requirements. IDs never change; removed ones are struck through, not reused.
- **Priority:**
  - **Must:** v1 isn't released without it.
  - **Should:** planned for v1; can move to v1.1 if time runs short.
  - **Later:** after v1, listed so the design leaves room for it.
- **Phase:** the PROPOSAL §9 phase that delivers it.
- **Source:** the PROPOSAL or SPEC section with the details.

## Functional requirements

**Projects and storage**

| ID | Requirement | Priority | Phase | Source |
|---|---|---|---|---|
| R-01 | Each project lives in one folder with `project.db`, `chats.db` and a bucket; backup, export and delete work on the folder | Must | 1 | SPEC 2.1 |
| R-02 | A registry lists projects and holds user memory and connections; it can be rebuilt from the project folders | Must | 1 | SPEC 2.4 |
| R-03 | Export makes a consistent copy of an open project (`VACUUM INTO` with the safe-copy rule) | Must | 2 | SPEC 2.1, 7.5 |
| R-04 | Jenab warns when the data folder is on a network drive or in a cloud-synced folder | Should | 1 | SPEC 2.1 |
| R-05 | After a crash, opening a project checks both databases, marks interrupted runs and cleans temp files | Must | 1 | SPEC 2.7 |
| R-06 | Each database has a storage format version; newer apps migrate with a backup, older apps refuse newer files | Must | 1 | SPEC 2.8 |

**Chats and turns**

| ID | Requirement | Priority | Phase | Source |
|---|---|---|---|---|
| R-10 | Chats are stored with messages and parts; a crash loses at most the part being streamed | Must | 1 | SPEC 2.3 |
| R-11 | Every project has a Mother chat, pinned at the top, with a list of the project's chats in its context | Must | 1 | SPEC 8.6 |
| R-12 | Chats have one editable title, unique within the project, and an optional role added to their prompt | Must | 1 | SPEC 8.6 |
| R-13 | Mother can start and continue chats (`create_chat`, `send_to_chat`); only Mother sends, and replies come back as finish notices | Must | 5 | SPEC 8.6 |
| R-14 | A turn can mix text and tool calls; the user can write during a turn, and the message joins at the next step | Must | 1 | SPEC 8.3 |
| R-15 | Background tasks (subagents, pipeline runs, chat tasks) return an ID at once; their finish starts a marked, capped turn | Must | 4–5 | SPEC 8.3 |
| R-16 | *Stop* cancels a turn; *Undo turn* reverts everything the turn changed | Must | 2 | SPEC 2.6, 8.3 |
| R-17 | History search over one chat or all chats of a project, for any language that puts spaces between words (tested on English, German and Persian) | Must | 1 | SPEC 2.3 |
| R-18 | One approval card in the chat for every approval (hosts, migrations, scripts, connections, MCP tools); the turn waits, with a badge and a notification when the chat isn't on screen, and a bar above the composer when the card is out of view | Must | 1 | SPEC 8.8 |
| R-19 | `ask_user`: a question form with 1–4 questions, options and an *Other* field; the answer returns to the agent | Must | 1 | SPEC 8.8 |
| R-117 | Approval levels per project: Strict, Standard (default) and Auto, picked in the composer; connections, private addresses, destructive migrations and scheduled runs behave the same at every level | Must | 1 | SPEC 8.8 |

**Context and memory**

| ID | Requirement | Priority | Phase | Source |
|---|---|---|---|---|
| R-20 | Each request is built in a fixed order (system prompt and role, memory, project card, notes, history window, turn) so provider caches stay valid | Must | 1–2 | SPEC 3.1 |
| R-21 | A short history window with cuts replaces automatic LLM compaction; older turns are reached through search | Must | 1 | SPEC 3.6 |
| R-22 | Large tool outputs and web pages are stored in the bucket and enter the context as previews with refs | Must | 1 | SPEC 3.7 |
| R-23 | The app generates a project card: tables, views, pages, pipelines, connections, MCP servers, workspace, bucket | Must | 2 | SPEC 3.2 |
| R-24 | User memory and project memory in sections, with size caps, revision checks, chips and undo | Must | 2 | SPEC 3.3 |
| R-25 | Session notes per chat | Must | 2 | SPEC 3.4 |
| R-26 | *Refresh memory*: a button per chat; background calls read new turns in chunks, update memory and a summary in the session notes, then cut the history window. Edits apply with a chip, diff and undo; they ask first at the Strict level or when changing a section the user wrote | Must | 2 | SPEC 3.5 |

**Data, schema and undo**

| ID | Requirement | Priority | Phase | Source |
|---|---|---|---|---|
| R-30 | A schema agent designs and changes tables through structured migration steps; Go writes all DDL | Must | 2 | SPEC 7.5, 8.2 |
| R-31 | A migration and the updates to its dependent views, pipelines and forms commit in one transaction, or not at all | Must | 2 | SPEC 7.5 |
| R-32 | Destructive migrations need approval and take a snapshot first; long ones show the estimated time and check free space | Must | 2 | SPEC 7.5 |
| R-33 | The agent reads data with `query` (read-only) and adds small manual entries with `insert` | Must | 2 | SPEC 8.1 |
| R-34 | Every state change is in a change log tied to the message, run, form or migration that caused it | Must | 2 | SPEC 2.5 |
| R-35 | *Undo turn* and *Revert run* revert only their own changes, stop on conflicts, and can be redone; the window is 90 days | Must | 2 | SPEC 2.6 |

**Views and pages**

| ID | Requirement | Priority | Phase | Source |
|---|---|---|---|---|
| R-40 | Table views with SQL pagination and sorting, formats, image and file columns, and row actions | Must | 3 | SPEC 5.3 |
| R-41 | Chart views: line, bar, scatter and pie, with the size rules | Must | 3 | SPEC 5.4 |
| R-42 | `stat` views that show one number with an optional change | Must | 3 | SPEC 5.10 |
| R-43 | Forms that insert, update or run a pipeline, with checks against column types | Must | 3 | SPEC 5.5 |
| R-44 | Filters bound to query parameters, with relative date defaults and query options | Must | 3 | SPEC 5.2 |
| R-45 | Pages arrange views, headings and text in rows on a 12-column grid, with page filters | Must | 3 | SPEC 5.9 |
| R-46 | Pages are listed in the right sidebar and open in the main area with the chat docked left or right | Must | 3 | SPEC 5.9 |
| R-47 | Block types are units in a registry (component, schema, validator); new types need no page changes | Must | 3 | SPEC 5.10 |
| R-48 | Opening a view or page makes no LLM call and no network request | Must | 3 | SPEC 5.7 |
| R-49 | Each chat remembers its layout (open page, filter values, dock side, right-sidebar tab); switching chats restores it; opening a project opens its last active chat, or Mother the first time; the agent is told which page is open and can open one with `open_page` | Must | 3 | SPEC 5.9 |
| R-110 | Pages have a header with the title, description, filters and buttons | Must | 3 | SPEC 5.9 |
| R-111 | Page blocks `card`, `filter` and `button` | Must | 3 | SPEC 5.9 |
| R-112 | `image` page block | Should | 3 | SPEC 5.9 |
| R-113 | Row action `set` (e.g. mark as read), with `bulk` for selected rows; undoable | Must | 3 | SPEC 5.3 |
| R-114 | `badge` format for status values | Must | 3 | SPEC 5.3 |
| R-115 | Light, Dark and System theme for the whole app; configs use named tones, never colours | Must | 1, 3 | SPEC 5.11 |
| R-116 | Right-to-left text (Persian, Arabic, Hebrew) shows correctly in the chat, inputs, tables and pages, also mixed with English; the UI stays English | Must | 1 | SPEC 5.8 |

**Pipelines and scheduling**

| ID | Requirement | Priority | Phase | Source |
|---|---|---|---|---|
| R-50 | YAML pipelines with the v1 step catalog, inputs, `if`, `for_each`, retries and timeouts | Must | 4 | SPEC 6.1–6.5 |
| R-51 | Sandboxed expressions in pipelines | Must | 4 | SPEC 6.4 |
| R-52 | `script.starlark` as an escape hatch, approved by the user per code hash | Should | 4 | SPEC 6.5 |
| R-53 | Every view, page, form and pipeline is validated before saving, with all errors and their paths returned together | Must | 3–4 | SPEC 10 |
| R-54 | The repair loop: up to 4 more attempts per config, YAML fix hints, one notice after an unfixed failed save | Must | 2–4 | SPEC 10 |
| R-55 | Dry runs on a copy of the project database | Must | 4 | SPEC 6.8 |
| R-56 | Cron schedules with time zones, DST rules, the wall-clock check and catch-up after missed runs | Must | 4 | SPEC 6.2 |
| R-57 | A run log per run and step, with secrets redacted; failure notifications; flag after three failures in a row | Must | 4 | SPEC 6.2, 6.6 |
| R-58 | Background/tray mode keeps schedules running with the window closed | Must | 4 | PROPOSAL 8 |
| R-120 | Pipeline page from a Pipelines tab: a read-only flow (the default) or a step table, run results per step, *Run now*, *Pause schedule*, *Dry run*, *Revert run* | Must | 4 | SPEC 6.10 |

**Network, connections and search**

| ID | Requirement | Priority | Phase | Source |
|---|---|---|---|---|
| R-60 | API connections: the agent fills in the details, the user pastes the key into a chat form, and the key goes to the keychain | Must | 4 | SPEC 6.9 |
| R-61 | `call_api` in chat and `api.get` in pipelines use a connection by name; secrets are used nowhere else | Must | 4 | SPEC 6.7, 6.9 |
| R-62 | Every literal host needs the user's approval once per project; private network addresses are blocked | Must | 4 | SPEC 6.7 |
| R-63 | `fetch_page` and `html.extract` turn web pages into readable text | Must | 1 | SPEC 3.7 |
| R-64 | Web search through Tavily or Brave with the user's key, or the user's SearXNG; a clear error when none is set up | Must | 4 | SPEC 6.5 |
| R-65 | Keyless sources: RSS/Atom feeds (`feed.read`, `read_feed`), Wikipedia, Hacker News | Must | 4 | SPEC 6.5 |
| R-66 | GDELT news as a best-effort keyless source | Should | 4 | SPEC 6.5 |

**Bucket and links**

| ID | Requirement | Priority | Phase | Source |
|---|---|---|---|---|
| R-70 | A per-project bucket with keys, deduplicated content, versions and `jenab://` links | Must | 1 | SPEC 4.1–4.5 |
| R-71 | Lifecycle rules, a cache size cap, and the "kept for undo" size shown with *Free now* | Must | 3 | SPEC 4.4 |
| R-72 | Saved links, shown with Files in the right sidebar | Must | 3 | SPEC 4.6 |

**Agents and tools**

| ID | Requirement | Priority | Phase | Source |
|---|---|---|---|---|
| R-80 | One subagent tool for heavy reading; subagents can't change memory or the schema | Must | 5 | SPEC 8.1 |
| R-81 | A read-only workspace: one linked folder with `list_files`, `read_file` and `search_code`, blocked credential files | Should | 4 | SPEC 8.5 |
| R-82 | MCP client: servers the user adds, tools through `mcp_describe` and `mcp_call`, approval per tool | Should | 5 | SPEC 8.7 |
| R-83 | Skills: Markdown instructions that a chat loads when needed (`load_skill`), or automatically with a tool (`load_with`); built-in skills for configs, pipelines, database design, migrations, SQL, web research and delegation | Must | 1–2 | SPEC 8.9 |
| R-84 | Chats and subagents can start with skills; Mother picks them in `create_chat`; the schema agent is built on skills | Must | 2, 5 | SPEC 8.9 |
| R-85 | User and project skills, written or accepted by the user; never downloaded | Should | 5 | SPEC 8.9 |

**Providers and app**

| ID | Requirement | Priority | Phase | Source |
|---|---|---|---|---|
| R-90 | One OpenAI-compatible provider in Phase 1; Anthropic and local models (Ollama) by the release | Must | 1, 5 | SPEC 3.8 |
| R-118 | No setup wizard: the app opens with *Create project*; a new Mother chat has starter prompts; a provider is asked for in the chat when a message is sent without one, and the message is kept | Must | 1 | SPEC 3.9 |
| R-119 | A built-in model catalog (limits and prices, no prompts), updated with app releases; adding a provider key turns on its models in the model picker | Must | 1 | SPEC 3.9 |
| R-121 | App shell: the left sidebar folds to icons while a page is open; project settings from the project switcher; a minimum window of 640 × 480; below 900 px, sidebars and the chat become overlays | Must | 1, 3 | SPEC 5.12 |
| R-91 | Settings in `config.toml`; all keys in the OS keychain | Must | 1 | SPEC 2.1, 6.7 |
| R-92 | Self-update with signed releases, a user choice of automatic, notify only or off | Must | 5 | SPEC 2.8 |
| R-93 | Scenario runner: the same agent scenarios on every test model, with a report | Must | 1 | SPEC 8.4, TASK-001 |
| R-94 | Turn inspector behind *Settings → Developer* | Should | 1 | SPEC 8.4 |
| R-95 | Installer builds for Windows and macOS | Must | 5 | PROPOSAL 9 |

**Later (not v1)**

| ID | Requirement | Priority | Phase | Source |
|---|---|---|---|---|
| R-100 | Editing workspace files with diffs and per-turn undo | Later | 6 | SPEC 8.5 |
| R-101 | Running commands, with approvals, after its own spike | Later | 6 | SPEC 8.5 |
| R-102 | MCP tools in pipelines, for tools the user marks as safe | Later | v1.1 | SPEC 8.7 |
| R-103 | Schema graph view | Later | v1.1 | SPEC 11 |
| R-104 | `http.post`, paging and OAuth in connections | Later | – | SPEC 6.9, 11 |
| R-105 | Kanban block, gallery view, remote bucket backends, Jenab as an MCP server | Later | – | SPEC 11 |
| R-106 | More block types (e.g. tabs, lists); accent colours and custom themes | Later | – | SPEC 5.10, 5.11 |

## Non-functional requirements

Numbers with "measured" come from the spikes. Numbers marked *proposed* are new targets for you to confirm.

**Responsiveness**

| ID | Requirement | Source |
|---|---|---|
| N-01 | Chat streams at 60 fps at 100 tokens/s, with key → paint under 50 ms while streaming (measured 14 ms, max 44 ms) | SPEC 5.8, SPIKE-022 |
| N-02 | Opening a chat with 200 messages doesn't block input for more than 100 ms (*proposed*; measured one task of 67–154 ms without virtualising) | SPEC 5.8 |
| N-03 | Tables of 10,000 rows scroll at 60 fps; a 500-row page arrives from Go in under 50 ms (measured 8–10 ms) | SPEC 5.3 |
| N-04 | A chart with 10,000 result rows draws in under 1 s (measured 126–333 ms with the size rules) | SPEC 5.4 |
| N-05 | A page with 6 blocks paints in under 1.5 s | SPEC 5.9 |
| N-06 | View queries time out after 2 s per block and never wait for the writer | SPEC 5.7, 7.1 |
| N-07 | Cold start to first paint under 1.5 s (*proposed*; measured 0.83 s) | SPIKE-022 |
| N-08 | A 5,000-row upsert with the change log takes under 1 s; undoing a 5,000-row run under 1 s (measured 0.3 s) | SPEC 2.6 |
| N-09 | History search p95 under 50 ms at 100,000 messages (measured 25 ms) | SPEC 2.3 |
| N-10 | After waking from sleep, due runs start within 2 minutes (*proposed*; the minute check fired within 1.5 s of waking) | SPEC 6.2 |

**Security**

| ID | Requirement | Source |
|---|---|---|
| N-20 | All agent SQL passes the three guard layers; the agent can't read internal tables or attach files | SPEC 2.2 |
| N-21 | Keys never appear in configs, prompts, logs, previews, URLs or the project folder; values are redacted everywhere | SPEC 6.7, 6.9 |
| N-22 | New hosts, destructive migrations, Starlark code and MCP tools need the user's approval; approvals are recorded | SPEC 6.7, 7.5, 8.7 |
| N-23 | Web pages, files, workspace contents and MCP results are treated as untrusted data | SPEC 3.7, 4.7, 8.5, 8.7 |
| N-24 | Scripts run in a child process with a 256 MB memory cap and a step limit; expressions have size and cost limits | SPEC 6.4, 6.5 |
| N-25 | Downloaded files are never run or rendered as HTML; SVG only through `<img>` | SPEC 4.7 |
| N-26 | Updates install only with a valid ed25519 signature over https; anything else is rejected | SPEC 2.8 |

**Storage and reliability**

| ID | Requirement | Source |
|---|---|---|
| N-30 | A crash never leaves a change pointing to a missing message, or an index row pointing to a missing file | SPEC 2.3, 4.3 |
| N-31 | Snapshots and exports are either complete or have no final name | SPEC 7.5 |
| N-32 | The WAL stays bounded under normal use and shrinks to 64 MB or less after checkpoints | SPEC 7.2 |
| N-33 | Size limits: uploads 100 MB, downloads 50 MB, HTTP responses 5 MB, cache 500 MB by default | SPEC 4.4, 4.7, 6.6 |
| N-34 | Scheduled runs that fail are never silent: log, notification and flag | SPEC 6.2 |

**Models and cost**

| ID | Requirement | Source |
|---|---|---|
| N-40 | Model-neutral: prompts, tools, schemas and the repair loop are the same for every model | SPEC 1 |
| N-41 | At least 80 % of configs are valid within the repair limit on the test models (*proposed*; measured 81 %) | SPIKE-021 |
| N-42 | Fewer tokens per turn and more accurate answers than a plain chat harness in the benchmark | PROPOSAL 11 |
| N-43 | Parallel LLM calls, pipeline runs and background tasks stay within the global limits | SPEC 7.6 |

**Platform**

| ID | Requirement | Source |
|---|---|---|
| N-50 | Windows 11 and macOS; per-user install without admin rights | SPEC 2.8 |
| N-51 | No cgo in the build | Gate-2 |
| N-52 | Idle memory under 300 MB (*proposed*; measured 245 MB) | SPIKE-022 |
| N-53 | Text meets WCAG AA contrast (4.5:1) in both themes, including every tone | SPEC 5.11 |

## Coverage of PROPOSAL §8

| §8 item | Requirements |
|---|---|
| Projects and chats in per-project folders | R-01, R-02, R-10 |
| Per-project SQLite, schema agent, migrations, change log, undo | R-30–R-35 |
| Context builder | R-20–R-23 |
| User memory, project memory, session notes, *Refresh memory* | R-24–R-26 |
| Bucket, lifecycle, links, saved links | R-70–R-72 |
| Views, image and file columns, row actions, pages | R-40–R-49, R-110–R-115 |
| YAML pipelines, validation, dry runs | R-50–R-55 |
| Scheduler, run log, retries, catch-up, notifications | R-56, R-57, R-120 |
| Background/tray mode | R-58 |
| Pipeline security | R-61, R-62, N-20–N-22 |
| API connections | R-60, R-61 |
| Read-only code workspace | R-81 |
| LLM providers | R-90, R-118, R-119 |
| Search providers and keyless sources | R-64–R-66 |
| Config file and keychain | R-91 |
| Subagent tool | R-80 |
| Mother chat, titles and roles | R-11–R-13 |
| MCP client | R-82 |

## Exit criteria
- [ ] Every item in [PROPOSAL §8](../PROPOSAL.md#8-scope-of-version-1) is covered by at least one requirement
- [ ] Every requirement has an ID, priority and source
- [ ] Non-functional requirements listed (responsiveness, security, storage limits)

## Open items
- Confirm the *proposed* targets: N-02, N-07, N-10, N-41, N-52.
- Confirm the Should items: R-04, R-52, R-66, R-81, R-82, R-94, R-112.
- R-90 depends on the Phase 1 provider choice (PROPOSAL §13).
