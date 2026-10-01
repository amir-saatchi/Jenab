# Gate-0 — Vision
**Status:** Done (agreed 2026-09-29)

## Purpose
What we build and why, on one page. Details are in [PROPOSAL.md](../PROPOSAL.md).

## Content

**What.** An open-source desktop agent harness (Go + Wails) where every project has its own folder and database, and the agent uses the database as structured working memory.

**Problem.** Today's harnesses keep knowledge in the chat. That wastes tokens, gets less accurate as chats grow, leaves no lasting result, and handles recurring work poorly ([PROPOSAL §2](../PROPOSAL.md#2-problem)).

**Core idea.**
- Views read, pipelines write. Data never has to pass through the LLM's context to reach the database.
- Tables hold data; memory holds intent.
- A new chat needs no old chat history. The project is the state; chats are logs.

**Building blocks.**

| Block | Role |
|---|---|
| Tables | Designed and migrated by the schema agent |
| Views and pages | Table, chart, stat and form blocks from validated configs, arranged into pages; no LLM on open |
| Pipelines | YAML steps run by Go, scheduled; LLM only in `llm.*` steps |
| Memory | User, project and session layers, plus a short history window instead of compaction |
| Bucket | Per-project S3-style file store with stable links |
| Chats | A Mother chat per project that starts and continues chats with lasting roles |
| Connections | The user's APIs with their keys; MCP servers for apps such as Blender or Unity (Phase 5) |

**Users.** Individuals who are comfortable bringing their own LLM keys, and keys for the APIs they want to track.

**First domain.** Personal trackers and monitors: prices, news, weather, scholarships (e.g. Master's or PhD calls and deadlines), releases, competitors, habits. Reference example: the Bitcoin project ([PROPOSAL §4](../PROPOSAL.md#4-example)).

**Next domains (not the v1 focus).** Tracking software and creative projects: tickets, CI and releases from GitHub; game data such as items, levels and bugs for a Unity project through MCP. A read-only code workspace is in v1; editing code and running commands come in Phase 6.

**Not this.** An app builder, a team tool, a cloud service, or a full coding agent (v1).

**Success.** Fewer tokens and more accurate answers than a plain chat harness in the benchmark; users creating a second project; schedules still running after 30 days ([PROPOSAL §11](../PROPOSAL.md#11-success-measures)).

## Exit criteria
- [x] Problem and core idea agreed
- [x] v1 scope boundary agreed ([PROPOSAL §8](../PROPOSAL.md#8-scope-of-version-1))
- [x] First domain and reference example agreed
- [x] Success measures agreed

## Open items
- ~~Project name and license~~ Decided 2026-10-01: Jenab, Apache 2.0 ([PROPOSAL §10, §13](../PROPOSAL.md#13-open-questions)).
