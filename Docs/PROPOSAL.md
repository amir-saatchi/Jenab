# Proposal — An Agent Harness with Structured Project Memory

**Name:** Jenab (جناب, a Persian honorific, roughly "Your Excellency"). Chosen 2026-10-01; the working name was Burrow.
**Author:** Amir
**Status:** Draft v0.2
**Related documents:** `SPEC.md` (v0.6), `app_structure.svg`

> **v0.2:** one folder per project (`project.db`, `chats.db`, bucket); project memory and a short history window instead of compaction; per-project bucket and saved links; change-log undo instead of per-turn snapshots; migrations in one transaction with their dependents; pipeline security rules; catch-up after missed runs; corrected Bitcoin example; accuracy benchmark.

---

## 1. Summary

This project is an open-source desktop agent harness, in the family of Codex and Opencode, built around one idea: **every project gets its own database, and the agent uses it as structured working memory.**

When a user starts a project, the agent designs tables for it, fills them through small declarative pipelines, and shows the results in generic views (tables, charts, forms) in a side panel. Scheduled jobs keep the data up to date without the user being present. Instead of scrolling back through chat history to find what it fetched ten turns ago, the agent queries its own database. A short project memory holds what tables cannot: goals, preferences and decisions.

The expected result is a harness that uses fewer tokens, gives more accurate answers over long projects, and produces something that keeps working after the conversation ends. Both claims will be measured against a plain chat harness (section 11).

---

## 2. Problem

Current agent harnesses treat the conversation as the main store of knowledge. This causes four problems.

**Token waste.** Data the agent fetched earlier (prices, search results, file contents) either stays in the context window, costing tokens on every turn, or gets compacted away and must be fetched again.

**Declining accuracy.** As chats grow, summaries replace details. The agent answers from a blurry memory of what it saw instead of from the data itself. Numbers get misremembered and sources get lost.

**No lasting result.** A chat produces answers, not state. If a user wants to track something over weeks, they have to ask again each time, and the history of previous answers is scattered across chats.

**Recurring work is awkward.** Tasks like "every morning, get the Bitcoin price and the five news items most likely to move it" are natural to ask for but poorly supported. Where scheduling exists, results usually land in more chat text rather than in data the user can browse, filter and chart.

---

## 3. Solution

The harness adds five things to the familiar chat-plus-tools design.

**A per-project database the agent designs.** Each project has its own SQLite file. When the user describes a goal, a dedicated schema agent proposes the tables. Every migration is checked together with the views and pipelines that depend on it, inside one transaction that is committed only if everything still validates.

**A short context instead of a long chat.** Each turn, the agent sees a compact project card (schema summary, pipelines, views), a short project memory, and only the last few turns of the chat. Older history stays in the chat database and is searched when needed. There is no LLM compaction.

**Pages built from generic blocks instead of generated code.**
- **Views:** the agent writes short view configs. Each view is a table, chart (line, bar, scatter, pie), single number or form, with a read-only SQL query and display settings.
- **Pages:** it arranges views, headings, text, cards, filters, buttons and images into pages, as rows on a simple grid. Pages follow the light or dark theme; configs name tones, never colours.
- **Opening a page:** pages are listed in the right sidebar. A page opens in the main area, with the chat docked beside it.
- **No UI code:** the agent never writes UI code. Opening a page costs no tokens and involves no LLM call.
- **Extending:** new block types plug into a registry.

**Declarative pipelines and a scheduler.** Data moves into the database through short YAML pipelines that chain generic Go steps such as `http.get`, `feed.read`, `llm.select` and `db.insert_many`. The agent writes the pipeline; the Go runtime moves the data. A pipeline that ran once in chat can be scheduled to run daily, with the LLM involved only in the steps that truly need judgment.

**One folder per project.** Everything a project owns lives in one folder: the project database, the chat database, and an S3-style bucket for web pages, images and other files, each with a stable link. A project can be backed up, exported or deleted as one unit.

The guiding rules are simple: **views read, pipelines write, and data never has to pass through the LLM's context to reach the database. Tables hold data; memory holds intent.**

---

## 4. Example

A user creates a project and writes: *"Every day, get the Bitcoin price and the top five news items likely to affect it."*

1. The schema agent creates `btc_prices`, `btc_news` and `watch_keywords` tables, each pipeline table carrying provenance columns (`_run_id`, `_fetched_at`).
2. The main agent writes a `daily_btc` pipeline: fetch the price, insert it, read news feeds filtered by the watch keywords, let an LLM step pick the five most relevant items, insert them.
3. It dry-runs the pipeline, runs it once to fill the tables, then schedules it for every morning.
4. Project memory records the goal: *daily BTC price and the news most likely to move it, every morning at 08:00 UTC.*
5. The right sidebar gains a *Bitcoin* page: a price chart and today's price at the top, the price and news tables below, then the keyword list and a form for adding keywords.
6. A week later, in a new chat, the user asks, *"Did any news line up with the drop on Tuesday?"* The new chat has none of the earlier history in its context, only the project card and memory. The agent answers with a single join query across the two tables.
7. The user says *"also track Ethereum."* The schema agent plans a migration: move `btc_prices` into a general `prices(coin, date, price)` table, copy the existing rows with `coin = 'BTC'`, add a `coin` column to the news table, and update the pipeline and views that depend on them. Because the old table is dropped, it asks for approval first. After approval, everything is applied in one transaction and nothing already collected is lost.

---

## 5. Architecture

The application is a Go desktop app built with Wails. The formats and storage are defined in `SPEC.md`; the component diagram is in `app_structure.svg`.

| Layer | Components |
|---|---|
| Frontend (webview) | Left sidebar (projects, chats); main area with the chat, or an open page with the chat docked beside it; right sidebar (pages; Files and Links); block registry (table, chart, stat, form; heading, text, card, filter, button, image); light and dark theme |
| Orchestration | Orchestrator (owns each turn, change log, undo), context builder (project card, memory, history window), main agent in each chat, Mother chat that directs chats with roles, schema agent, subagents |
| Execution | Tool registry, pipeline runner, scheduler, sandboxed Starlark escape hatch |
| Data | Async data layer: one writer goroutine and a read-only pool per database file, WAL mode; object store for the bucket |
| Providers | LLM interface (OpenAI-compatible, Anthropic, local via Ollama/LM Studio), search and extract interfaces |
| Storage | `config.yaml`, `registry.db` (project list, user memory), one folder per project; API keys in the OS keychain |

```
<user data dir>/Jenab/
  config.yaml
  registry.db
  projects/<project_id>/
    project.db     data tables, views, pipelines, memory, change log, runs, bucket index
    chats.db       chats, messages, tool outputs, session notes
    objects/       bucket contents, stored by content hash
    snapshots/     copies taken before destructive migrations
```

**Chats are logs; the project is the state.** A new chat in an existing project does not need earlier chat history, because everything that matters lives in the project's database, memory, views and pipelines. Every state change is recorded in a change log against the message, form submit or pipeline run that caused it. This gives an audit trail and a per-turn undo that reverts only that turn's changes, so pipelines writing at the same time are not affected.

**Planned technology:**

| Need | Choice |
|---|---|
| Desktop shell | Wails v3 (Go), pinned to an exact tag, with the normal OS window frame (SPIKE-003) |
| Frontend | React + TypeScript, shadcn/ui (Radix base), Zustand, TanStack Table, Recharts; Bun for installs and scripts. Tokens drawn once per frame, large charts downsampled, charts and Markdown loaded lazily (SPIKE-022) |
| SQLite driver | `modernc.org/sqlite` (pure Go, no cgo) |
| History search | SQLite FTS5 (`unicode61`, with Go text normalization for German and Persian; SPIKE-011) |
| Object store | Local folder behind a small interface (`gocloud.dev/blob` style), so S3-compatible backends can be added later |
| LLM providers | Official Anthropic and OpenAI Go SDKs behind our own `Provider` interface; local models through the OpenAI-compatible API (SPIKE-012) |
| Config files | `go.yaml.in/yaml/v3` and `santhosh-tekuri/jsonschema/v6` (SPIKE-013) |
| Scheduling | `adhocore/gronx` for cron, with our own syntax check and DST rule, plus a wall-clock check for missed runs and OS resume events (SPIKE-014, SPIKE-004) |
| Web pages to text | `readeck/go-readability/v2` + `html-to-markdown/v2` (SPIKE-015) |
| Expressions in pipelines | `expr-lang/expr` (SPIKE-005) |
| Script escape hatch | `go.starlark.net`, run in a child process with a memory cap (SPIKE-016) |
| App updates | Wails `pkg/updater` (endpoint provider) with our own fail-closed ed25519 check, per-user install (SPIKE-019) |
| Secrets | `zalando/go-keyring` (SPIKE-006) |
| MCP client | To be chosen in SPIKE-024 (the official Go SDK is the first candidate) |

---

## 6. Key design decisions

**Views are configs, not code.** Generic components plus a validated config are safer, cheaper and more reliable than LLM-written React, and the agent can edit them without breaking the app.

**One owner for the schema.** Only the schema agent changes the schema. Each migration runs in one transaction together with the updates to the views, pipelines and forms that depend on it, and is committed only if all of them still validate. SQLite's DDL is transactional, so a failed check leaves no trace. The schema agent returns `applied`, `rejected` with a reason, or `needs_approval` for destructive changes such as dropping a column. A snapshot is taken before every approved destructive change.

**The LLM writes pipelines, not rows.** Fetching, parsing and inserting happen in Go. The LLM sees results like "inserted 30 rows," not the rows themselves. A small `insert` tool exists for manual entries in chat; anything recurring or bulk goes through a pipeline.

**Async database access from day one.** All writes go through one writer goroutine per database file with interactive and background priorities; reads run in parallel on read-only connections. No transaction is held open across a network or LLM call, so several chats, pipelines and forms can work in the same project at once.

**Short context instead of compaction.** Each turn contains the project card, memory and only the last few turns (3 to 8, with a token cap). The window moves in blocks, so the providers' prompt caches stay valid. Tool outputs such as web pages are stored in the bucket and enter the context as a preview with a reference. Older history is reached through a search tool. Nothing is summarized automatically; *Refresh memory* summarizes into the session notes when the user asks.

**Memory holds intent, tables hold data.** There are three layers: user memory across projects, project memory shared by all chats, and session notes per chat. Memory never stores values that belong in tables. Edits are made per section with a revision check, recorded in the change log, and shown in the chat with an undo button. A *Refresh memory* button reads the chat in chunks, updates memory and a summary in the session notes, and then shortens the history. Its edits show a chip with the changes and undo.

**One folder per project.** Data, chats and files of a project live in one folder. Views, pipelines, memory and the change log live in the same database file as the data, so a migration and everything it changes commit together.

**Everything is validated before it is saved.** Views and pipelines are checked against JSON schemas, the step catalog, the project schema and SQLite's own query preparation. All errors are returned together, with paths, so the agent can fix them in one attempt.

**Pipelines cannot leak secrets.** The agent reads untrusted web content and writes pipelines that run unattended. Secrets are therefore used only through saved API connections, each bound to one host; new hosts need the user's approval, private network addresses are blocked by default, and secret values are redacted from everything the LLM or the logs see.

**Bring your own keys.** Users connect their own LLM and search providers. Web search uses search APIs built for programmatic access, never scraping of search result pages. Users bring a key for a search API (several have free monthly tiers) or point Jenab at a SearXNG instance they run. Without either, Jenab still has keyless sources for trackers: RSS/Atom feeds, Wikipedia, Hacker News and GDELT news (best effort) (SPIKE-020). For any other API, the user adds a **connection**: the agent reads the API's docs and fills in where the key goes, and the user pastes the key into a form. The key goes to the OS keychain, and chats and pipelines name only the connection (SPEC 6.9).

**A Mother chat that directs the others.** Every project has a Mother chat, pinned at the top. It sees a list of the project's chats and can start or continue chats with a lasting **role**, such as a reviewer, whose role text is added to that chat's prompt. One-off work still goes to subagents. Only Mother sends, so chats can't loop, and the user can read and step into every chat (SPEC 8.6).

**Model-neutral.** Prompts, guides, tools, schemas and the repair loop are the same for every model. Only the provider layer knows each API's protocol details. Free models are used for testing because they are weak: a config format they can write, a stronger model can write too (SPIKE-021).

---

## 7. Positioning

The harness space already has strong open-source projects. The goal is not to compete on breadth but on one clear capability.

| | Typical harness | This project |
|---|---|---|
| Long-term memory | Chat history, summaries, markdown memory files, full-text search over past chats | Relational tables for data, a short project memory for intent, searchable chat history without compaction |
| Output of a task | Text in the chat | Data in tables, visible in views, updated by schedules |
| Recurring tasks | Scheduled prompts whose results arrive as messages | Scheduled pipelines where most steps run without an LLM |
| UI for results | Chat transcript | Generic tables, charts and forms from configs |
| Files | Attachments in a chat | A per-project bucket with stable links used by tables and views |

General assistants now offer projects, memory and scheduled tasks; automation tools such as n8n cover pipelines, and tools such as Datasette cover browsing data. The difference here is the combination: an agent that designs the schema, writes the pipelines and maintains the views itself, inside the chat the user already works in.

App builders generate whole applications from a prompt; this project stays a harness. The user keeps working in chat, and the database and views are a byproduct that makes the agent more capable, not a separate app to deploy.

**First domain:** personal data trackers and monitors, meaning projects that collect something regularly and ask questions about it over time (prices, news, weather, scholarship calls and deadlines, releases, competitors, habits). The three primitives (tables, pipelines, views) fit this domain completely.

---

## 8. Scope of version 1

**Included**

- Projects and chats in per-project folders, with the storage layout and chat data model from the spec
- Per-project SQLite with schema agent, transactional migrations, change log and per-turn undo
- Context builder: project card, history window, history search, tool outputs as previews with references
- User memory, project memory, session notes, and the *Refresh memory* button
- Per-project bucket with versioning, lifecycle rules and `jenab://` links; saved links
- Table, chart (including pie), stat and form views, including image and file columns and row actions; pages that arrange them with headers, cards, filters, buttons and images
- Light, dark and system theme
- YAML pipelines with the v1 step catalog, validation and dry runs
- Scheduler with run log, retries, catch-up after missed runs, and failure notifications
- Background/tray mode so schedules run with the window closed
- Pipeline security: host approvals, secrets only through connections, private network block, redaction
- API connections with the user's own keys (a key in a header or query), used from chat and pipelines
- A read-only code workspace: one linked folder the agent can list, read and search
- LLM providers: OpenAI-compatible, Anthropic, local models
- Search providers: Tavily or Brave with the user's key, or the user's own SearXNG; keyless RSS/Atom feeds, Wikipedia, Hacker News and GDELT news
- User config file and OS keychain for secrets
- One simple subagent tool, used for reading long pages
- A Mother chat per project, chat titles and roles; Mother can start and continue chats
- MCP client: servers the user adds (e.g. Blender, Unity, GitHub); their tools in chat, with approval

**Not included**

- Team or multi-machine access
- Cloud-run schedules and sync
- S3-compatible remote bucket backends and shareable (presigned) links
- Editing files in the workspace and running commands (Phase 6)
- MCP tools in pipelines (v1.1), and Jenab as an MCP server
- Visual pipeline editor and parallel pipeline branches
- `http.post`, and API auth other than a key in a header or query (e.g. OAuth)
- Automatic memory review when a chat goes idle
- Schema graph view (planned for v1.1)

---

## 9. Roadmap

Phases are defined by what must work at the end, not by dates.

**Phase 1 — Core loop.** Wails shell, registry and per-project folders, `chats.db` with the chat data model, the Mother chat, chat titles and roles, agent loop with the Phase 1 providers (Anthropic, OpenAI, Gemini, OpenAI-compatible and Ollama; SPEC 3.9), history window and history search, tool outputs stored in the bucket as previews with references, config file and keychain. *Done when:* a user can hold a normal tool-using chat that survives restarts, and the agent finds something said 20 turns earlier through history search.

**Phase 2 — Structured memory.** `project.db`, async data layer, schema agent with transactional migrations and approvals, change log and per-turn undo, `query` and `insert` tools, project card, user and project memory, session notes and *Refresh memory*. Also the token and accuracy benchmark (section 11). *Done when:* the agent can create and evolve a schema from a conversation, and a new chat answers questions about earlier work using only queries and memory.

**Phase 3 — Views and pages.** Block registry with table, chart, stat and form blocks; pages with the grid layout and the chat dock; image and file columns; row actions; view and page validation; right sidebar with pages, Files and Links. *Done when:* the Bitcoin page opens without any LLM involvement, and a page with six blocks paints in under 1.5 s.

**Phase 4 — Pipelines and scheduling.** Pipeline runner, v1 step catalog including bucket steps, API connections, validation and dry runs, pipeline security rules, scheduler with catch-up, run log, notifications, background mode; the read-only workspace tools. *Done when:* the Bitcoin pipeline runs daily for a week unattended, including across laptop sleep, with every run visible in the log.

**Phase 5 — Public release.** More providers, subagent tool, `create_chat` and `send_to_chat` for the Mother chat, MCP client, documentation, example projects, installer builds. *Done when:* a new user can install the app and reproduce the Bitcoin example in under ten minutes with keyless data providers (feeds and news APIs) and their own LLM key.

**Phase 6 — Code (after the release).** File editing in the workspace with diffs and per-turn undo; then running commands, after a spike on approvals, process control and prompt injection. *Done when:* the agent can fix a failing test in a linked repository, with every file change undoable and every command approved.

---

## 10. Open source and sustainability

The project will be open source. The leading harnesses in this space are free and permissively licensed, and users who hand an app their API keys, local data and permission to run scheduled jobs reasonably expect to be able to inspect it.

**License:** Apache 2.0, decided 2026-10-01. It is the license most companies and developers accept, includes a patent grant from every contributor, and works with every library chosen (SPIKE-002). AGPL was the alternative; its hosting rule protects little for a desktop app and would cost users and contributors. The license gives no rights to the name, so forks can't call themselves Jenab.

**Sustainability later, if adoption justifies it:** optional paid services for things that are genuinely hard to do locally, such as cloud-run schedules when the laptop is off, sync across devices, team workspaces and managed model access. The desktop harness itself stays fully functional without them.

**Personal value regardless of commercial outcome:** a tool used daily, deep experience in agent engineering, and a public portfolio project in a field with high demand.

---

## 11. Success measures

**Technical**

A benchmark is built in Phase 2: the same scripted long-running task (for example, two weeks of the Bitcoin project with follow-up questions) is run in this harness and in a plain chat harness.

- Tokens per turn in the benchmark, compared with the plain chat harness
- Answer accuracy on questions about earlier work in the same benchmark
- Prompt cache hit rate per chat
- Share of scheduled pipeline runs that succeed without intervention
- Share of generated views and pipelines that pass validation on the first or second attempt

**Adoption**

- Number of users who create a second project (the clearest sign the idea is useful)
- Projects with a schedule still running after 30 days
- Community contributions of new steps, providers and view types

---

## 12. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Established projects add a similar feature | Move fast on the specific combination (agent-designed schemas, views, pipelines); stay focused on one domain first |
| LLMs write invalid pipelines or views | Strict schemas, complete error lists with paths, up to 4 repair rounds, dry runs, a small step catalog. With free test models, 50 % of configs were valid on the first try and 81 % after repairs (SPIKE-021) |
| Schema changes corrupt data | Single schema agent, one transaction per migration together with its dependents, snapshot and approval before destructive changes |
| Prompt injection in web content leads to pipelines that leak secrets or reach the local network | Secrets only through connections bound to one host, approval for new hosts, private network block, redaction of secret values |
| Pipelines break when websites or APIs change | Run log, retries, failure notifications, provenance columns to spot stale data |
| LLM steps make schedules expensive | LLM only in explicit `llm.*` steps, cheap model alias by default, token usage recorded per step, global limits on parallel calls |
| Schedules miss runs when the app is closed or the laptop sleeps | Background/tray mode and catch-up after wake in v1; cloud schedules as a later paid option |
| Memory drifts from the data or fills with noise | Memory holds intent only, size caps, per-section edits with visible chips and undo, *Refresh memory* |
| Project folders grow large | Content-hash deduplication, lifecycle rules for cached files, a cache size cap |
| Scope grows beyond one developer | Fixed v1 scope, four view types plus pages with a fixed grid, about eighteen steps, one domain |
| Code tools or MCP servers let web content act on the user's machine | Read-only workspace in v1; commands only after a spike, with approval per command pattern and for every command after web content; MCP servers added only by the user, each tool approved, write tools asked again after web content; pipelines never run commands or MCP tools |
| Search providers restrict automated access | No keyless web search is promised; provider interfaces, user-supplied keys, keyless feeds and news sources for trackers, caching of fetched pages in the bucket |

---

## 13. Open questions

1. ~~Final project name~~ Decided (2026-10-01): Jenab. A web and GitHub search found no conflicting product; a formal trademark search (USPTO, EUIPO, classes 9 and 42) is due before the public release
2. ~~License: Apache 2.0 or AGPL~~ Decided (2026-10-01): Apache 2.0 (section 10)
3. ~~Which LLM and search providers to support at the Phase 1 launch~~ Decided (2026-09-29): Anthropic, OpenAI, Gemini, OpenAI-compatible and Ollama; web search with Tavily or SearXNG, plus the keyless sources; Brave later (SPEC 3.9, 6.5)
4. ~~Wails v2 or v3~~ Decided: Wails v3, pinned, with the OS frame (SPIKE-003, 2026-09-28)
5. Default sizes for the history window, token caps and memory caps (to be tuned with the benchmark)
6. Whether the schema graph view should move into v1

*Resolved in v0.2:* compaction for long chats. There is no LLM compaction; see "Short context instead of compaction" in section 6.
