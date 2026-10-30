# Project Portfolio Entry

> Filled in on 2026-10-30 from the repository (README, PROPOSAL, SPEC, tickets and `outreach/INTRODUCTION.md`).
> Rule from `outreach/INTRODUCTION.md`: say *does* only for what the app does today; the dashboards and scheduled pipelines are still the design.

---

## 1. Basics

- **Project name:** Jenab (جناب; working name Burrow)
- **One-line summary:** Desktop AI agent that keeps each project's work in its own local database, so long projects don't get lost in long chats. It works with any model, cloud or local.
- **Type:** AI agent / desktop app
- **Status:** In progress. Phase 1 (chat, tools, providers, approvals, skills, app shell) is built. Phase 2 (agent-designed tables, change log and undo, schema agent, memory) is built, but its benchmark and acceptance tickets are still open. Dashboards and scheduled pipelines are designed, not built yet.
- **Timeline:** Repository started 2026-10-01. 150 commits up to 2026-10-29 (about 4 weeks), and still going.
- **Context:** Personal, open source (Apache 2.0)
- **Can I show it publicly?** Yes. The repository is public.

## 2. The problem

- **Who was it for?** People who use AI agents for long-running work like research, tracking or data collection, and want their own keys, their own models and their data kept on their own machine. Developers and technical users first.
- **What problem did it solve?** Agent apps keep what they learn in the chat. Long chats cost more tokens on every turn and get less accurate as summaries replace details. They also leave nothing behind that lasts. Jenab gives each project a folder with its own SQLite database. The agent stores what it finds in tables and keeps intent in a short project memory. It finds older work by searching, so the whole chat doesn't have to go into every request.
- **What was there before?** General chat agents (Codex, Opencode, ChatGPT-style apps) where the conversation is the only memory and older parts are cut down by LLM compaction.

## 3. What I built

- **Main features:**
  - Native desktop app (Windows 11 first; macOS and a Linux beta planned) with light and dark themes and right-to-left text for Persian and Arabic
  - Five model connections: Anthropic, OpenAI (Responses API), Gemini (native API), any OpenAI-compatible API, and Ollama (local or cloud). The user brings their own keys, and the keys stay in the OS keychain.
  - Streaming chat with tool use: web pages turned into readable text, a versioned file store per project (the "bucket"), history search, questions back to the user, and subagents
  - Approvals before risky actions (Once / Always / Deny, with Strict, Standard and Auto levels). A mark on content from the web, so that deletes or outside calls ask again after the agent has read untrusted text.
  - Per-project SQLite database that the agent designs through a separate schema agent. Migrations are checked in one transaction, writes are typed and logged in a change log, and any chat turn can be undone.
  - User and project memory, session notes, and *Refresh memory* in place of LLM compaction
  - Skills the agent loads when a task needs them, and a Mother chat per project with chat roles
  - Settings for models, keys and token usage (with a chart), plus a turn inspector and runtime panel for developers
  - Project export as one folder
- **My role:** Solo: product idea, specification, architecture, Go backend, React UI, test tooling and all the model experiments.
- **Most challenging part and how I solved it:** Making one agent work the same way on very different models (Claude, GPT, Gemini, Gemma, GLM, local Qwen) with **no per-model prompts**. Each API breaks in its own way: tool-call IDs, thinking blocks, refusals, context overflow, JSON arguments that don't parse, streams that stall during long tool calls. All of that is handled in the provider layer, and the prompts, tools and loop stay the same for every model. To prove it works and to stop it from breaking again, I built a **scenario runner**. It replays scripted conversations on real models, checks tool calls, approvals, database state and token and cache use, and compares the results with an earlier report. It can also run offline from scripted replies. The risky decisions were settled first in **37 short spikes**: SQLite crash safety, a SQL guard without an authorizer, sandboxes, keychains in background mode, sleep and wake catch-up, FTS5 for German and Persian, structured output, MCP, and semantic recall.

## 4. Tech stack

| Layer | Technologies |
|---|---|
| Frontend | React 19, TypeScript, Vite, Tailwind CSS 4, shadcn/ui (Radix), Zustand, Recharts, react-markdown + remark-gfm, Geist and Vazirmatn fonts, Bun |
| Backend | Go 1.26: agent loop, provider layer, tools, scheduler design; anthropic-sdk-go, openai-go, go-readability, html-to-markdown, jsonschema, YAML |
| Desktop | Wails v3 |
| Database | SQLite through modernc.org/sqlite (pure Go, no cgo), WAL mode, one writer and parallel readers per database, FTS5 search tuned for German and Persian |
| Hosting / DevOps | Local desktop app. Taskfile builds, GitHub Actions CI (currently blocked by a billing lock), staticcheck. Self-update through the Wails updater was tested in a spike. |
| Auth / payments / other APIs | OS keychain (go-keyring) for API keys; Tavily or SearXNG for web search; MCP client (official go-sdk) designed for Phase 5 |

## 5. AI / LLM details (only for AI projects)

- **LLM(s) used:** Any model the user configures, through Anthropic, OpenAI, Gemini, OpenAI-compatible APIs or Ollama. The reference models for testing are gemma4:31b (Ollama Cloud) and gemini-3.5-flash-lite. Groq, Z.ai GLM, Mistral, local Qwen 3.5 4B and others were also tried in spikes.
- **Embedding model:** Decided, not built yet. It is user-chosen, with Ollama and OpenAI-compatible first. In SPIKE-033, embeddinggemma-2 (with prefixes) did best. A built-in granite-embedding-107m-multilingual was tested in pure Go and then dropped for now.
- **Vector store:** No vector database. The design is a rebuildable `index.db` per project with int8 vectors and a brute-force cosine scan in Go (about 40 ms over 250,000 vectors at 768 dimensions).
- **Data sources ingested:** Chat history, web pages (turned into readable Markdown), and files in the project bucket. The agent's own tables hold structured data. Word files are planned.
- **Chunking / retrieval approach:** Today history search uses FTS5, under 12 ms (p95) over 100,000 messages. The decided design is hybrid search: FTS5 and embedding cosine, the top 50 of each merged with reciprocal rank fusion (k = 60), and file chunks split by heading at 1,200 bytes. Each hit says whether it matched by `words`, `meaning` or `both`, and there is no fixed score cutoff. In the spike, hybrid search had 2.4 to 3.5 times the recall of FTS alone.
- **Agent / harness / tool use:** A custom agent loop with native function calling. Tools include `fetch_page`, `read_ref`, `bucket_*`, `search_history`, `read_messages`, `query`, `describe_table`, `insert`, `request_schema_change`, `update_memory`, `update_session_notes`, `load_skill`, `ask_user`, `create_chat` and delegation to subagents. A schema agent designs and migrates tables, with a `widen_table` step that keeps the data. The context is a short window: a project card, memory, session notes and the last few turns, with cuts in place of compaction. For MCP, two tools (`mcp_describe` and `mcp_call`) replace the full tool lists. That cut prompt tokens from 12–15k to about 1.3k per request for one app's tools.
- **Guardrails & quality:**
  - A SQL guard so that views can only read
  - Change-log undo
  - Approval cards
  - An untrusted-content mark against prompt injection, tested with planted instructions in fake notes
  - Starlark and expr sandboxes, and per-host approval for web requests
  - One shared handling of broken responses for every provider
  - A stream watchdog
  - A scenario runner with checks and baselines, run on reference models with several repetitions
  - A made-up-number check was built and measured, then kept out of v1 because it flagged too many honest answers. That decision is recorded in SPIKE-032.
- **Channels / integrations:** The desktop app itself. MCP servers are designed (tested against a real 32-tool app). Saved API connections and scheduled pipelines are in the SPEC.
- **Cost / latency work:**
  - A short context window in place of the full history
  - Prompt-cache tracking per run (cache-read share in reports)
  - Skills and tool details loaded only when needed
  - Per-provider limits and pauses
  - Background calls limited to 8 at a time
  - Tokens painted once per frame in the UI
  - A token and accuracy benchmark against a plain chat harness (P2-13) is planned but not run yet

## 6. Results

- **Measurable outcomes** (Windows 11, the built app):
  - Window painted in under 1 s from start
  - Streaming at 60 fps, with about 22 ms from a key press to the screen
  - About 60 MB for the app's own process
  - History search over 100,000 messages in under 12 ms (p95)
  - Code size: about 46,000 lines of Go, about 35,000 lines of Go tests (778 test functions), and about 14,000 lines of TypeScript/React
  - 37 spikes, 30 tasks and 33 phase tickets, each with a written result and decision
  - Scenario runs: multi-part requests 9/9 on both reference models; skill loading 11/12; schema design 4–5 of 5 runs on gemini-3.5-flash-lite
- **Client / user feedback:** None yet (personal project, not released).

## 7. Assets for the gig

- **Live URL:** None. A landing page draft is in `outreach/website/index.html` and not published yet.
- **GitHub / code sample:** https://github.com/amir-saatchi/Jenab (public)
- **Screenshots:** `outreach/website/img/`
  - **Best 1:** `01-chat-tool-calls.png` (streaming chat with tool calls)
  - **Best 2:** `02-persian-rtl.png` (right-to-left Persian chat)
  - **Best 3:** `04-turn-inspector.png` (developer depth)
  - Also: `03-approval.png`, `05-usage.png`, `06-settings.png`, `07-add-provider.png`, `08-chat-light.png`, the full-page preview `outreach/website/preview-full-page.png`, and the banner `Docs/images/banner.png`
  - High-fidelity mockups of screens that aren't built yet are in `mockups/` and `Docs/Gates/Gate-2-Design/`. Label them *design* if you show them.
- **Demo video / GIF:** Not made yet. Planned: a 30–60 s recording of one chat with tool use.
- **Architecture diagram:** Yes. See `Docs/app_structure.svg`.

## 8. Gig fit (my opinion)

- **Which gig does this prove?**
  - AI agent / chatbot apps with tool use
  - Multi-provider LLM integration (Claude, GPT, Gemini, Ollama)
  - Go + Wails desktop apps
  - React + shadcn/ui frontends
  - SQLite-backed local-first apps
  - RAG and search design (FTS5 plus hybrid retrieval)
  - LLM evaluation and test harnesses
- **What would a buyer pay for something like this?** Rough guess, adjust to your market. A smaller client version (desktop chat agent with 2–3 providers, tools, approvals, local storage) might fetch about $5,000–15,000. A full harness like this, with the schema agent, undo, memory and an eval runner, is more like $30,000+.
- **How long would I need to rebuild a similar thing for a client?** Rough guess: 3–5 weeks for a focused client version (chat, providers, tools, approvals, storage), reusing what I learned here. This project's Phase 1 and Phase 2 took about 4 weeks, but with the spikes and specs already written.
