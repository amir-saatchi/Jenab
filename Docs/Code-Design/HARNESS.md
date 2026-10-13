# Harness map
**Status:** Draft, 2026-10-11; updated 2026-10-13 for TASK-006 to TASK-014

The harness is everything around the model that decides what it sees, what it can do, what needs the user's permission, when it stops, and what it is told when something goes wrong. This file walks one turn through the code and names every control it passes, then lists the controls by job. Section numbers are the SPEC's; [CODE-OUTLINE](CODE-OUTLINE.md) has the package signatures.

Two rules hold throughout:

- **What must hold goes in code; the prompt is for judgment.** The prompt says tool results are data, and the untrusted mark and approval cards (8.8) enforce it. The model can't skip a check that runs in Go.
- **One harness for every model (1).** Prompts, tools, limits and the loop are the same for all models. Protocol differences stay in the provider layer (3.8).

## 1. A turn, step by step

A turn is one user message and everything the agent does until it answers (8.3). Each step is one model request plus the tool calls in its answer.

| # | What happens | Control | Where |
|---|---|---|---|
| 1 | The user sends a message. It is stored, then joins the running turn at its next step, or starts a new one. A card the turn waits for closes as *Deny*, with the message as the note. | One runner per chat; join or queue | `agent.Orchestrator.Send` |
| 2 | The turn reads the chat and resolves its model. | Unknown model: the turn stops with a notice | `runner.begin` |
| 3 | The window is cut if due: past 8 turns or 24,000 tokens, after 5 minutes idle, or in a new process. Block 1 is rebuilt only at a cut. | History window (3.6), cache stability (3.1) | `runner.cutIfDue` |
| 4 | The request is built from the system blocks, the window and this turn. Old tool results become stubs, and calls left open by a crash get a result. | What the model sees (3.1, 3.7) | `runner.request`, `shape` |
| 5 | If this turn passes half the model's context window, its older previews become stubs, all but the two newest. | In-turn trimming (3.7) | `runner.trim` |
| 6 | The chat's tools are added. The last request keeps them, so the cached prefix holds, but asks for no calls (8.3). | Tool list | `tool.Registry.For` |
| 7 | The request streams. Silence over the stall limit cancels it; backends send a heartbeat while a tool call's arguments stream, except Ollama, which sends nothing then. Rate limits, overloads and network errors are retried for up to 10 minutes, with the wait shown. | Stall watchdog, retries (3.8) | `provider.Registry.run`, `runner.ask` |
| 8 | The answer is written whole, once complete. Tool calls cut off by the output limit are dropped. | Nothing half-written is stored | `runner.step`, `dropCutCalls` |
| 9 | An answer without tool calls ends the step. A cut-off, refused or empty answer adds a notice the user and the model can read. | `answer_cut` notice | `cut` in `turn.go` |
| 10 | Each tool call, in order: bad JSON, an unknown tool, or a Mother-only tool in another chat becomes an error result, and the tool doesn't run. | Argument and tool checks | `runner.runTool` |
| 11 | The tool may say what this call needs, such as a new host. | `tool.Preflighter` | each tool |
| 12 | The approvals are split: those already given are dropped, the level approves some, and the user is asked for the rest. The turn waits with no transaction held. | Approval level, untrusted mark (8.8) | `runner.approve`, `runner.await` |
| 13 | The tool runs with its timeout. Arguments are checked against its JSON Schema first. A panic becomes an error result. | Schema, timeout, recovery | `tool.Run`, `tool.Func`, `runner.safeRun` |
| 14 | After a tool that reads outside data, the chat is marked untrusted. | Untrusted mark | `runner.runTool` |
| 15 | A large result is stored in the bucket and enters as a preview with a ref. | `tool_preview_tokens` (3.7) | `tool.Output` |
| 16 | The tool's first call in the chat loads the skills whose `load_with` names it. Their text is stored as a `skill_text` notice after the chip and joined to the result in each request; after a cut, a skill in block 1 joins as a stub. | Skill safety net (8.9) | `runner.loadWith`, `shape` |
| 17 | If any call ran, the turn takes another step. Three requests in a row whose calls all failed to run stop the turn. | Bad-call limit | `runner.runTools` |
| 18 | At the request cap, the last request keeps the tools with tool choice `none` (Ollama, which has none, gets no tools) and a line asking for an answer. If that answer has no text, an `answer_cut` notice says the turn reached its limit. A message sent during it gets one more such request. | `turn_max_requests` (8.3) | `runner.turn`, `lastRequest` |
| 19 | A turn that stops on an error writes a `turn_failed` notice saying what happened and what to do. *Retry* continues the turn. | Failure notice | `runner.finish`, `failure` |
| 20 | After the first turn, the chat gets a title from the fast model. | Title prompt | `runner.title` |

*Stop* can come at any point. The answer so far is kept and marked stopped, open tool calls get "cancelled by user", and a waiting card closes as stopped.

## 2. The controls by job

### 2.1 What the model sees

The context is built in blocks, from most stable to most changing, so provider caches reuse the start (3.1). Between cuts a request only grows at the end.

| Block | Content | Built | Where |
|---|---|---|---|
| 1 | System prompt, which names the parts of a project the tools cover and says the rest aren't in this version, and the line for when no tool fits; the multi-part rule, with a background tool; Mother's line and, with `create_chat`, her delegation lines; today's date; the chat's role; loaded skills; the skill list, without skills whose `load_with` tools are all missing | At a cut | `runner.system`, `base`, `prompts/*.md`, `skill.Set.Block` |
| 2–3 | User memory, project memory | Phase 2 | – |
| 4 | Project card; in Mother, the chat list | At a cut | `Deps.Card`, `runner.chatList` |
| 5 | Session notes | Phase 2 | – |
| – | "This chat has N earlier turns. Use search_history or read_messages." | At a cut | `earlierTurns` |
| 6 | History window: the last 3 to 8 turns, older tool results as stubs | Each request | `shape` |
| 7 | This turn, with previews; older ones trimmed past half the window | Each request | `runner.request`, `runner.trim` |
| Tools | Name, description and JSON Schema of each tool, sorted by name | Each request | `tool.Registry.For` |

The cache point is after the last system block. Tokens are estimated at 4 bytes a token everywhere (`provider.EstimateTokens`).

### 2.2 What the model can do

- **Tools.** Phase 1 has 12:
  - `search_history`, `read_messages`, `list_chats`
  - `read_ref`, `search_ref`, `fetch_page`
  - `bucket_list`, `bucket_read`, `bucket_put`, `bucket_delete`
  - `ask_user`, `load_skill`

  They are registered in `cmd/desktop/main.go`. A tool is a `Spec` and a `Run`. `Spec.Mother` limits a tool to the Mother chat. `Spec.Background` (the tool can start background work) and `Spec.Areas` (the tables, pipelines or views it works on) decide which prompt lines come with it (TASK-011, TASK-014).
- **Effects.** Each tool declares what it does (`tool.Effects`), so the agent decides approvals and undo in one place and never by tool name:
  - `Untrusted`: `fetch_page`, `read_ref`, `search_ref`, `bucket_read` and `read_messages`.
  - `AsksUser`: `ask_user`, which has no timeout.
- **Arguments.** `tool.Func` checks them against the schema before the tool runs. The schema is the tool's real contract; the description is advice.

### 2.3 What needs permission

| Kind | Strict | Standard | Auto |
|---|---|---|---|
| `host` (a new host for `fetch_page`) | Ask | Ask | Auto, unless the chat is untrusted |
| `starlark` | Ask | Ask | Auto |
| Anything else | Ask | Ask | Ask |

This is the `rules` table in `agent/approve.go`. The rest of SPEC 8.8's table is added with the tools that need it.
- **The untrusted mark** is set after an `Untrusted` tool runs. Only a new turn clears it: a message that joins a running turn doesn't, since the page read earlier is still in the context (TASK-012).
- **Approvals are recorded** in `_jenab_approvals`, with `user` or `auto` as the source. A call that is denied grants nothing.

### 2.4 Limits

| Limit | Value | Setting or constant | Where | Why |
|---|---|---|---|---|
| Requests per turn | 25, the last without tools | `llm.turn_max_requests` | `runner.turn` | A turn can't loop forever |
| Failed tool calls in a row | 3 requests | `maxBadRequests` | `turn.go` | A model that can't form a call stops instead of looping |
| Retrying a failing request | 10 min | `maxRetrying` | `turn.go` | Rate limits pass; outages get an error card |
| Provider's first event | 2 min (local Ollama 10 min) | `FirstEvent`, `FirstEventOllama` | `provider/registry.go` | A dead connection is noticed |
| Silence between events | 60 s | `Between` | `provider/registry.go` | The same, mid-stream (TASK-006) |
| Tool call | 1 min; `fetch_page` 45 s; none for `ask_user` | `Spec.Timeout`, `tool.DefaultTimeout` | `tool.Run` | A stuck tool doesn't block the turn |
| History window | 3 to 8 turns, 24,000 tokens | `context.history_*` | `cutIfDue` | Old turns cost tokens and cache |
| Cache lifetime | 5 min | `cacheIdle` | `context.go` | After that, a cut costs nothing extra |
| Tool result in the context | 1,500 tokens, then a preview and ref | `context.tool_preview_tokens` | `tool.Output` | Large results go to the bucket, not the context |
| In-turn trimming | Half the model's window; the 2 newest stay | `context.turn_trim_ratio` | `runner.trim` | A long turn fits its model |
| Skills in a chat | 6 skills or 10,000 tokens | `skill.MaxLoaded`, `MaxLoadedTokens` | `runner.addSkill` | Block 1 stays small |
| One skill | 3,000 tokens; description 200 characters | `skill.MaxBodyTokens`, `MaxDescription` | `skill` | The same |
| `read_messages` | 20 turns a call | `maxReadTurns` | `tool/history.go` | One call can't pull the whole history |
| `bucket_list`, `search_ref` | 100 items, 20 hits | `maxList`, `maxRefHits` | `tool/bucket.go` | Results stay previewable |
| `ask_user` | 1–4 questions, 2–4 options | its schema | `agent/ask.go` | Forms stay answerable |

Settings are what a user may reasonably tune. Constants are safety limits, or values a test or spike fixed.

### 2.5 What the model is told

Every outcome is something the model can read and act on, never a silent drop.

| Situation | What the model gets | Where |
|---|---|---|
| Arguments that aren't valid JSON | "the arguments are not valid JSON: …, at byte N of M. Send the call again with valid JSON." | `invalidArgs` |
| Arguments that fail the schema | "wrong arguments:" and one line per error with its path | `tool.decodeArgs` |
| An unknown tool | "there is no tool named X" | `runTool` |
| A tool's own mistake (missing object, blocked URL) | The tool's `*tool.Error` message | each tool |
| A timeout | "X timed out after …" | `tool.Run` |
| A crash or a system failure | "X failed: …" or "the tool crashed: …", also logged | `runTool`, `safeRun` |
| Denied | "not run: the user denied it (…); their note: …" | `denied` |
| Stopped or closed | "cancelled by user", "cancelled: the project closed" | `cancelled` |
| A crash left a call open | "no result: the app closed before this call finished" | `shape` |
| The request cap | The last request's line, then "not run: the turn reached its request limit" for any calls | `lastRequest`, `runTools` |
| A large result | A preview with its size and ref, then how to read on with `read_ref` / `search_ref` | `tool.Output` |
| An answer cut off, refused or empty | An `answer_cut` notice | `cut` |
| The turn stopped | A `turn_failed` notice saying what happened and what to do | `failure` |

The base prompt tells the model that lines in square brackets are notices from the app, not the user.

### 2.6 Watching the harness

- **Turn inspector (8.4).** Shows each request's blocks with their estimated tokens and cache points, timings, usage, stop reason, and each tool call's size and time (`agent/trace.go`). It is on with `dev_tools`.
- **Runtime panel.** Shows each running turn, its request number, and whether it has stopped moving (`chatState.Status`).
- **Scenario runner** (`internal/scenario`, `cmd/jenab-scenarios`). Runs YAML scenarios through the real orchestrator on real models, with checks on tool use, skills and answers. It is the only way to measure a prompt or harness change on models. It records cache-read and cache-write tokens, compares with a baseline report, and plays scripted replies without keys (TASK-013).
- **Go tests in `agent`.** Cover the loop's behaviours on a fake provider.

## 3. Everything the model reads

Text the model reads is spread across these places. Review them together when the wording changes, for model neutrality and for languages other than English.

| Text | Where |
|---|---|
| System prompt, multi-part rule, Mother's line and delegation lines | `agent/prompts/base.md`, `rule.md`, `mother.md`, `delegate.md` |
| Skill list header, skill text | `skill/set.go` (`listHeader`), `skill/builtin/*/SKILL.md` |
| The areas line, date line, earlier-turns line, chat list | `agent/context.go` (`areaText`, `base`, `today`, `earlierTurns`, `chatList`) |
| Tool descriptions | each tool's `Spec`, in `tool/*.go` and `agent/ask.go`, `agent/skills.go` |
| Error and result texts | `agent/turn.go`, `agent/approve.go`, `tool/tool.go`, each tool |
| Last-request line, notices | `agent/turn.go` |
| Preview header and footer, stubs | `tool/output.go` |
| Question-form answers | `agent/ask.go` (`answerText`) |
| Title prompt | `agent/title.go` |

## 4. Changing the harness

- **Choose the right layer.** A rule that must hold goes in code: an approval kind in `rules`, a limit in the loop or the tool, a check in the schema. Guidance on how to work goes in the prompt, a skill, or a tool description. A rule needed for one kind of work belongs in a skill, so it costs nothing when unused.
- **Keep the start of the request stable.** Anything that changes between cuts goes at the end as a notice, or waits for the next cut (3.1). Changing block 1 or the tool list mid-window costs a full cache write.
- **Give feedback, not silence.** A new failure becomes a tool result or a notice that says what happened and what to do.
- **Keep it model-neutral.** No model or provider names above the provider layer. If one model fails, record it in the spike or scenario results; don't add a rule for it.
- **Measure before and after.** Run the scenario sets on the reference models before and after a prompt or loop change, and compare the reports.
- **Settings or constants.** A limit becomes a setting only if users have a reason to tune it. Otherwise it stays a constant with a comment that says why.

## 5. Open work and what comes later

- **Done since the review:** TASK-006 to TASK-014 (stream heartbeat, tool-call IDs, reasoning and refusals, skill text after a cut, the request cap's answer, prompts that match the tools, the untrusted mark, measuring the harness, and telling the model what this version lacks).
- **Open:** SPIKE-032, a harness check for numbers no source gave.
- **A loop core.** Today the loop is tied to chats: `runner` writes through `chatState`, the chat store and the event publisher. Subagents, turns started by the app (`system_turn_max_requests`) and model steps in pipelines will need the same loop with their own limits. When the first of them is built, the step → tools → limits → feedback core moves out of the chat code, with chats as one of its users.
- **More context blocks.** User and project memory, the project card and session notes (Phase 2) fill blocks 2–5, with change notices between cuts (3.1).
- **Loading tools when needed.** SPIKE-028 may change how `Registry.For` picks the tools a request carries.
