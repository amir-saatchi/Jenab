# Gate-2 — Technical Design
**Status:** In progress (technical choices ready for review; design started)

## Purpose
The technical decisions and libraries, each with a short reason, and the UI design. Details are in [SPEC.md](../SPEC.md) and [app_structure.svg](../app_structure.svg).

## Content

| Area | Choice | Reason / key result | Evidence |
|---|---|---|---|
| SQLite driver | `modernc.org/sqlite` v1.59.0 | Most widely used cgo-free driver, with FTS5; the guards cover its looser defaults | [SPIKE-001](../Tickets/SPIKE-001-sqlite-driver.md) |
| Library survey | The "Recommended" column, as changed by the spikes | One tested choice for every need | [SPIKE-002](../Tickets/SPIKE-002-library-survey.md) |
| Desktop shell | Wails v3, pinned to `v3.0.0-beta.26`, normal OS frame | Built-in tray, autostart and notifications; the only freeze seen was frameless | [SPIKE-003](../Tickets/SPIKE-003-wails-version.md) |
| Sleep and catch-up | One-minute wall-clock check, plus `PowerRegisterSuspendResumeNotification` | Minute check fired within 1.5 s of every wake; Wails' sleep/wake events never arrived on Modern Standby | [SPIKE-004](../Tickets/SPIKE-004-sleep-and-catch-up.md) (Windows) |
| Expressions | `expr-lang/expr` behind Jenab's sandbox | Only listed syntax, limits on size and nesting, 1 s watchdog | [SPIKE-005](../Tickets/SPIKE-005-expr-sandbox.md) |
| Secrets | `zalando/go-keyring`, no caching | 0.1 ms per read, at most 2,560 bytes, works with the screen locked | [SPIKE-006](../Tickets/SPIKE-006-keychain-background.md) (Windows) |
| Agent SQL | Three layers: text check, `EXPLAIN` check, guarded read-only connection | 63/63 query cases and 19/19 migration cases on both drivers | [SPIKE-007](../Tickets/SPIKE-007-sql-guard.md) |
| Crash safety | `synchronous = NORMAL`, recovery using `jenab.lock` | `.partial` then rename for copies; message written before any change | [SPIKE-008](../Tickets/SPIKE-008-crash-safety.md) |
| Change log and undo | Per-row before-images, 90-day undo window; file bytes kept for the same window, with the size shown | Undoing a 5,000-row run takes 0.3 s | [SPIKE-009](../Tickets/SPIKE-009-change-log-cost.md) |
| WAL and large tables | 64 MB journal and cache limits, checkpoint at idle, rules for long migrations | WAL stays bounded; reads keep working during migrations | [SPIKE-010](../Tickets/SPIKE-010-wal-and-large-tables.md) |
| Search | FTS5 `unicode61`, contentless, Go normalization for Persian and German | 45/50 relevant hits, 0 false hits; "inside words" scan 114–166 ms per 100k messages | [SPIKE-011](../Tickets/SPIKE-011-fts5-languages.md) |
| LLM SDKs | Official Anthropic and OpenAI SDKs behind Jenab's `Provider` interface, no framework | Every framework tested failed at least one thing Jenab needs | [SPIKE-012](../Tickets/SPIKE-012-llm-sdk-agent-loop.md) |
| Config files | `go.yaml.in/yaml/v3` + `santhosh-tekuri/jsonschema/v6` | Errors at the exact line and column; strict parsing rules | [SPIKE-013](../Tickets/SPIKE-013-yaml-and-json-schema.md) |
| Cron | `adhocore/gronx` + Jenab's syntax check, DST rule and `time/tzdata` | Strict 5 fields; DST handled like cronie | [SPIKE-014](../Tickets/SPIKE-014-cron-parsing.md) |
| Web pages | readeck `go-readability/v2` → `html-to-markdown/v2` | Charset decoding, 5 MB cap, `needs_javascript` for empty pages | [SPIKE-015](../Tickets/SPIKE-015-readable-text.md) |
| Scripts | `go.starlark.net` in a child process: `db.query` through the parent; memory capped by a Job Object on Windows, a soft limit plus the parent's check elsewhere | Step, time and memory limits all enforced on Windows | [SPIKE-016](../Tickets/SPIKE-016-starlark-sandbox.md) |
| Local model | `qwen3.5:4b` on Ollama only for short tool-loop tests | 14/15 tool tasks, but about 4 tokens/s; Ollama cuts long prompts silently | [SPIKE-017](../Tickets/SPIKE-017-local-model-ollama.md) |
| Development models | Ollama Cloud `nemotron-3-ultra` + `gemma4:31b`; backup Z.ai `glm-4.5-flash` | 11/11 and 10/11 tool tasks; a 32k prompt answered in under 4 s | [SPIKE-018](../Tickets/SPIKE-018-cloud-models.md) |
| App updates | Wails `pkg/updater` + Jenab's fail-closed signature check, per-user install, watchdog | About 0.25 s downtime; every bad release rejected | [SPIKE-019](../Tickets/SPIKE-019-self-update.md) |
| Frontend | React + TypeScript, TanStack Table, Recharts, shadcn/ui, Zustand, Bun | Confirmed by the user | [SPIKE-002](../Tickets/SPIKE-002-library-survey.md) |
| Web search | No keyless web search: Tavily or Brave with the user's key, or the user's SearXNG; keyless feeds, Wikipedia, Hacker News, GDELT as separate sources | No allowed keyless search worked (public SearXNG 0/16, DuckDuckGo has no web API); crypto feeds gave 5/5 relevant, dated items | [SPIKE-020](../Tickets/SPIKE-020-keyless-search.md) |
| Config writing | Save through tools, full error lists with paths, 4 repair rounds; model-neutral guide and schemas | 50 % valid on the first try, 81 % after repairs; peak request 15k tokens | [SPIKE-021](../Tickets/SPIKE-021-models-write-configs.md) |
| Frontend load | Tokens drawn once per frame with memoised Markdown blocks; chart size rules; column + array-row pages; lazy loading | 60 fps at 1,000 tokens/s and on 10k-row tables; tuned charts draw in 126–333 ms | [SPIKE-022](../Tickets/SPIKE-022-frontend-under-load.md) |
| Agent instructions | Skills: a skill list in the prompt, `load_skill`, and `load_with` as a safety net, instead of the whole guide | Right skill before 60/60 first writes, no unneeded loads; first request 4.5k → 2k tokens; configs as valid on nemotron and glm | [SPIKE-025](../Tickets/SPIKE-025-skills.md) |

**Deferred:** the macOS parts of SPIKE-004 (sleep and wake) and SPIKE-006 (keychain in signed and unsigned builds), and the Linux and macOS memory-cap test of SPIKE-016. For the Linux beta: the keyring through Secret Service (SPIKE-006), sleep and wake through systemd-logind (SPIKE-004), the updater with an AppImage (SPIKE-019), and the frontend load on WebKitGTK (SPIKE-022); these can run in WSL. They are needed before Phase 5 (public release). No Mac is available now.

**Later checks** (they don't change the decisions):
- Local SearXNG and its engine block rates, once Docker works (SPIKE-020).
- Guide v3, with T2 and T4 rerun on every test model, plus a new T5 "build a page", before Phase 2 (SPIKE-021, SPEC 5.9).
- The same config tests on at least one frontier model, before Phase 5 (SPIKE-021).
- gemma's "add ETH" migration task with skills (valid 0/2 in SPIKE-025), with at least 5 reps, and the reworded skill descriptions, in the scenario runner before Phase 2 (SPIKE-025, SPEC 8.9).
- Mother's routing to existing role chats, on a frontier model and with each chat's full role in the chat list, before Phase 5 (SPIKE-023, SPEC 8.6).
- Chat history rendering, plus the 1,000-point chart animation limit, in the Phase 1 perf tests (SPIKE-022).
- A full page (6 blocks, including the 10,000-row table and a chart) paints in under 1.5 s, in the Phase 3 perf tests (SPEC 5.9).
- A spike on running commands (approvals, process-tree kill on Windows, macOS and Linux, prompt injection) before Phase 6 (SPEC 8.5).
- The MCP client: SDK, tool style, approvals and real servers, before Phase 5 ([SPIKE-024](../Tickets/SPIKE-024-mcp-client.md), SPEC 8.7).

## Design
In [Gate-2-Design](Gate-2-Design/README.md):
- **Low fidelity:** 12 grayscale wireframes, with notes, open questions and the requirements each covers. Reviewed on 2026-09-29; the agreed proposals are in the SPEC.
- **High fidelity:** after the low-fidelity review. First an inventory of every shadcn/ui component, then mockups built with them, in light and dark ([TASK-002](../Tickets/TASK-002-high-fidelity-design.md)).

## Exit criteria
- [x] Libraries chosen for every need (SPIKE-002)
- [x] Spikes SPIKE-001, SPIKE-003 to SPIKE-023 and SPIKE-025 done, each with a decision (macOS parts of SPIKE-004 and SPIKE-006 deferred: no Mac available)
- [x] SPEC updated with the spike results (v0.5); connections and workspace added in v0.6
- [x] Architecture diagram redrawn for SPEC v0.4, labels updated for v0.5
- [x] Low-fidelity wireframes reviewed, and what they propose added to the SPEC (v0.6: 3.5, 3.9, 5.12, 6.10, 8.8); the remaining open questions go to TASK-002
- [ ] High-fidelity design reviewed, in both themes (TASK-002)
