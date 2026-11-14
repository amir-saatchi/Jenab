# SPIKE-044 — Specialized agents: tool groups and a custom prompt
**Type:** Spike
**Status:** Done (decided 2026-11-14). The user accepted the proposed SPEC 8.6 text as round 2 left it, without "don't repeat them". It is in SPEC 8.6, with 3.1, 8.1 and the `chats` table (2.3) updated to match.
**Gate:** 3
**Needs:** —

## Question
A chat can be made into an **agent**: its own tool groups and a custom prompt, chosen when the chat starts. When Mother creates one with `create_chat`, does the model pick the right tool groups and write a prompt that works? And can the agent then do its job with what was picked?

## Background
- Today a role is only text (8.6). Every chat gets every tool of its kind (`Tools.For(kind)` in block 1), so a *Reviewer* can still call `insert`. A role tells the model what to do, but its tools are the same as every other chat's.
- The prompt's lines already come with the tools that need them (TASK-011). Picking tool groups therefore also picks the prompt sections. Only the core rules have no tool behind them: no made-up data, when no tool fits, fewer rounds, the date.
- **Decided with the user** (2026-11-11):
  - An agent is one chat in one project. Saved presets that work across projects can come later.
  - The user customizes a new chat in a *Customize* panel: a custom prompt, plus tool groups that show which prompt sections each one adds. The core rules are always in and can't be turned off.
  - **Tools are fixed when the chat starts** in v1, so no history ever holds calls to a tool the chat no longer has. Adding tools later can come after v1.
  - Only Mother creates agents (`create_chat`). Mother has no tools that write data, so she stays at the project level and hands work to agents.
  - Agents hear from each other through notices (SPIKE-034 round 2, SPIKE-045). They don't message each other directly.

## Design under test
1. **Tool groups** (a first draft; the spike may change them):

   | Group | Tools |
   |---|---|
   | Read data | `query`, `describe_table` |
   | Change data | `insert`, `request_schema_change` |
   | Views and pipelines | `save_view`, `open_page`, `validate`, `dry_run`, `run_pipeline`, `run_status`, `read_config` |
   | Files | `bucket_*`, `read_file`, `list_files`, `search_code` |
   | Web | `web_search`, `fetch_page`, `read_feed`, `save_link`, `call_api`, `connect_api` |
   | Memory | `update_memory` |
   | Always on | `ask_user`, `update_session_notes`, `search_history`, `read_messages`, `read_ref`, `load_skill`, `task_status`, `subagent` |

2. **`create_chat(title, role, message, skills, tools)`:** `tools` is a list of group names. Each group's one-line description is in the tool's schema. `role` is the custom prompt, at most 500 tokens, in the role's place in block 1.
3. **The chat's tool list** is the chosen groups, narrowed further by the usual gating (3.1). A call to a tool outside the groups is **refused** with an error that names the group. Today a tool left out still runs if the model calls it.
4. **Mother's own tools** in every condition: read data, files, memory, `create_chat`, `send_to_chat`. No change data and no views or pipelines.

## Conditions
1. **Key:** the groups and the prompt come from the answer key, as if the user had picked them in the panel. This shows what a right pick is worth.
2. **Asked, groups:** the user asks Mother for an agent ("make an agent that …"), and Mother picks the groups.
3. **Asked, single tools:** as 2, but `tools` lists single tools instead of groups.
4. **Unasked:** the user describes a lasting job without saying "agent". Mother decides whether to create one, with the delegation guidance (8.6) and groups.

## Scenarios
A seeded project (SPEC 9's Bitcoin project), plus about 7 requests. Each request has an answer key with the groups the agent needs, the groups it must not have, and a follow-up task the new agent runs:
- **Pipeline reviewer:** read data, views and pipelines. Not change data or web. Task: review a pipeline that has no error step.
- **ETH tracker:** web, views and pipelines, change data. Task: add ETH prices daily.
- **Data entry:** read data, change data. Not web. Task: enter 5 rows the user pastes in.
- **Docs writer:** read data, files. Not change data. Task: write `docs/schema.md`.
- **Researcher:** web, files, memory. Not change data. Task: compare three price APIs and save the notes.
- **A request in Persian:** the custom prompt should stay in the user's language.
- **A write request with no agent yet:** Mother can't write, so she must create an agent or send the work to one, not fail or pretend.

## Measures
- **Groups:**
  - needed groups present (recall);
  - unneeded groups absent (precision);
  - how often the model gave the agent every group.
- **Prompt:**
  - states the job and what to report;
  - doesn't repeat the core rules;
  - at most 500 tokens;
  - in the user's language.
- **Task:**
  - the follow-up task passes its check;
  - refused calls, and what the agent does next: tells the user or asks, as the "when no tool fits" line says.
- **Creating:**
  - one `create_chat` per asked request (conditions 2 and 3);
  - creations nobody asked for, on plain messages (condition 4).
- **Tokens:** prompt tokens per request in the agent, against the same task in a chat with every tool.

## Rules
- The same prompts and tool descriptions for every model (SPEC 1). Decided on the reference models ([DEVELOPMENT](../DEVELOPMENT.md#reference-models)). Failures on other models are recorded, not worked around.
- The `tools` argument and the refusal are spike code behind a hook, like SPIKE-034's `Deps.Notices`. The app leaves the hook nil.
- Gemini's free tier allows 500 requests a day (SPIKE-034), so plan the runs with `-resume`.

## Done when
- Each measure has a number per condition and model.
- The decision is written as SPEC 8.6 text:
  - the groups: names, members and which ones are always on;
  - who picks them: the panel, Mother, or both;
  - the refusal's wording;
  - Mother's own groups;
  - any change to the delegation guidance.

## Result
Ran on 2026-11-12/13: 217 good runs. The models were Gemini `gemini-3.5-flash-lite` (89 of 91), NVIDIA `mistralai/mistral-large-4-1t-a52b` (23 of 91) and Anthropic `claude-haiku-5-5` (105 of 105).

- **Haiku 5.5 replaced gemma4:31b** because `OLLAMA_API_KEY` returned 402 (no credit). The user chose the swap, so the Ollama backend wasn't tested here. Haiku cost about $0.30: 5.54M prompt tokens (4.73M of them cache reads) and 346K output.
- **Gemini ran on the reserve keys `GEMINI_API_KEY_III` and `_IV`,** with the user's OK. 2 runs are left (the write scenario, rep 3, conditions 3 and 4): both keys' daily quota stopped them.
- **NVIDIA was stopped by choice after 23 runs.** Its free endpoint took 10–25 minutes a run. 68 runs are left, including every Persian and write run.

The harness is in [`spikes/044-agents/`](../../spikes/044-agents/README.md). The findings are in its [`results.md`](../../spikes/044-agents/results.md), and every table is in [`results/all.md`](../../spikes/044-agents/results/all.md). The spike adds one app hook, `agent.Deps.ChatTools`: a chat's tools, and the refusal for any other tool. The app leaves it nil.

| Gemini / NVIDIA / Haiku | 1 key | 2 asked, groups | 3 asked, single tools | 4 unasked |
|---|---|---|---|---|
| Needed groups present (recall) | 1.00 / 1.00 / 1.00 | 0.93 / 1.00 / 0.93 | 0.91 / 1.00 / 0.95 | 0.90 / 0.93 / 0.90 |
| Picked groups that are needed (precision) | 0.89 / 0.95 / 0.89 | 0.78 / 0.74 / 0.81 | 0.78 / 0.78 / 0.83 | 0.75 / 0.72 / 0.81 |
| A forbidden group | 0 / 0 / 0 | 0 / 0 / 0 | 5% / 0 / 0 | 10% / 0 / 0 |
| Every group | 0 / 0 / 0 | 14% / 0 / 0 | 5% / 0 / 0 | 0 / 0 / 0 |
| Follow-up task passed | 100% / 100% / 100% | 95% / 100% / 100% | 100% / 100% / 95% | 85% / 100% / 95% |
| Prompt says what to report | key | 43% / 100% / 95% | 30% / 100% / 95% | 10% / 100% / 100% |
| Prompt repeats a core rule | key | 0 / 60% / 24% | 0 / 67% / 65% | 0 / 20% / 22% |
| Prompt mean tokens (all ≤ 500) | 93 | 63 / 285 / 197 | 64 / 303 / 216 | 40 / 197 / 158 |
| Refused calls: agents / Mother | 0 / 0 | 0 / 0 | 0 / 7, 0, 2 | 0 / 0 |

Prompt tokens per request with the key's groups, against every group: Gemini 3,493 vs 5,136 (−32%), NVIDIA 4,392 vs 5,057 (−13%), Haiku 5,069 vs 7,473 (−32%).

- **Mother picks groups well, and an agent never strays.**
  - The follow-up task passed in 85–100% of runs where Mother made the agent, about as often as with the answer key.
  - No agent called a tool outside its groups in 217 runs: a chat only sees its own tools. The refusal is a safety net, not a path the reference models take.
- **Groups, not single tools.** Recall and precision were within 0.04 of each other, but a list of single tools made Mother try tools she lacks.
  - Every refusal in the spike was Mother's, in condition 3: Gemini 7 (`read_config` ×5, `insert`, `request_schema_change`), Haiku 2 (`insert`).
  - Haiku tried the write itself in 2 of 3 write runs, once by writing and deleting a junk file as a probe.
  - With groups, Mother never tried a write.
  - After a refusal she handed the work on with `create_chat`. The wording works.
- **Mother needs `read_config`.** Without it she can't read the pipeline she's asked about.
  - NVIDIA started a chat to answer "What does the daily_btc pipeline do?" That was the only creation on a plain message, 1 of 46.
  - 5 of Gemini's refused calls were `read_config`.
- **Unasked, Mother delegates when the work comes, and never pretends.**
  - When the user described a lasting job, Mother created an agent at once in 67–80% of runs. Haiku asked first in 3 of 18.
  - She did the Persian digest herself when her own groups were enough (Haiku 3 of 3).
  - The write with no agent yet: delegated in every run with groups; pretended in 0 of 16.
- **The prompt's language carries into the agent's output.**
  - For the Persian request, Haiku wrote the role in English in 6 of 6 runs. It named Persian as the answers' language in 5, and once said "in English".
  - Gemini's agent with an English role wrote an English digest and failed. With a Persian role it passed.
  - Gemini's roles are one line, 40–64 tokens on average, and seldom say what to report. Haiku and NVIDIA repeat core rules in 20–67%.

**Decision** (2026-11-14): the user accepted the text below as round 2 left it. It is now in SPEC 8.6. Writing it in also settled these points:

- **3.1:** the gating works within a chat's groups. A tool the gating left out still runs; one outside the groups gets the refusal.
- **8.1 and 8.9:** `create_chat` gains `tools`.
- **2.3:** the `chats` table gains `tool_groups`.
- **8.6:**
  - *Clear* keeps the groups.
  - A subagent works within its parent's groups, so `subagent` can't get around them. This wasn't tested.
  - Mother's row in the chat comparison changes: memory only, with schema and data work handed on.
- **Phase 5:** the *Customize* panel, the refusal and Mother's groups.

**SPEC 8.6 text** (proposed, then accepted):

> **Agents.** A chat can be an agent: its own tool groups and a custom prompt, fixed when the chat starts.
>
> - **Tool groups.** A chat has the tools of its groups, narrowed further by the usual gating (3.1). Block 1's sections follow its tools (3.1); the core rules are always in.
>
>   | Group | Tools | Its one line (in `create_chat` and the *Customize* panel) |
>   |---|---|---|
>   | Read | `query`, `describe_table`, `read_config`, `run_status`, `open_page` | read the project's tables, views, pages, pipelines and runs |
>   | Change data | `insert`, `request_schema_change` | add rows and ask the schema agent for tables or columns |
>   | Build and run | `save_view`, `save_page`, `save_pipeline`, `validate`, `dry_run`, `run_pipeline` | build, check and run views, pages and pipelines |
>   | Files | `bucket_*`, `list_files`, `read_file`, `search_code` | the project's files (the bucket) and the linked folder |
>   | Web | `web_search`, `fetch_page`, `read_feed`, `save_link`, `call_api`, `connect_api` | search and read the web, feeds and APIs |
>   | Memory | `update_memory` | edit user and project memory |
>   | Always on | `ask_user`, `update_session_notes`, `search_history`, `read_messages`, `read_ref`, `search_ref`, `list_chats`, `load_skill`, `task_status`, `subagent` | |
>
> - **Who picks.** The user, in the *Customize* panel when a chat starts: the custom prompt, and the groups with the prompt sections each adds. The panel starts from every group. Or Mother, with `create_chat(title, role, message, skills, tools)`, where `tools` lists group names; she picks what the job needs.
> - **Fixed at the start.** A chat's groups don't change in v1, so no history holds calls to a tool the chat no longer has.
> - **Refusal.** A call to a tool outside the chat's groups doesn't run. It returns: "`<tool>` isn't one of this chat's tools: it is in the `<Group>` group (`<its one line>`), which this chat doesn't have. A chat's tools are fixed when it starts. Tell the user what you can't do here, or ask them." In Mother's chat, the last sentence is "Hand the work to a chat that has it, with send_to_chat or create_chat."
> - **Mother's groups:** Read, Files and Memory, plus `create_chat` and `send_to_chat`. She can't change data, build or run, or use the web; she hands that work to a chat that can.
> - **`create_chat`'s `role`:** "The chat's custom prompt, at most 500 tokens, in the user's language. Say what the chat is for, how it works and what it reports back. The system prompt already has the general rules."
> - **Delegation guidance:** as in v1, with one line added: "You can't change data, build or run, or use the web. Work that needs them goes to a chat that has them: send it to one, or create one with only the groups the job needs."

Changes from the design under test:

- **Read gets `read_config`, `run_status` and `open_page`.** Mother needs them, and they write nothing. *Views and pipelines* becomes *Build and run*.
- **`search_ref` and `list_chats` are always on**, as they were in the spike. `save_page` and `save_pipeline` are in *Build and run* (SPEC 8.1).
- **`create_chat`'s `role` description** adds the user's language and what to report. Round 2 tested it with "; don't repeat them" at the end; that made no difference, so it's dropped.
- **`subagent` stays always on**, untested: the app has no subagent tool yet.
- **Not tested:** the *Customize* panel. The refusal fired in round 1 only, before the regrouping.

### Round 2: the proposed text
Ran on 2026-11-14: conditions 2 and 4 with the proposal in place (the new groups, the role description, the added guidance line, the refusal naming the new groups), on Gemini and Haiku 5.5. NVIDIA was left out as too slow.

- **Gemini:** 33 of 42 runs, 582 requests on `GEMINI_API_KEY` and `_IV`. Both keys' daily quota stopped the 9 others: rep 3 of every scenario but reviewer and entry.
- **Haiku:** 42 of 42 runs, about $0.15: 3.00M prompt tokens (2.61M of them cache reads) and 177K output.
- **Spike-only hook:** the added guidance line came through one more hook, `agent.Deps.DelegateLine`. The app leaves it empty.
- **Scoring:** with *Build and run* split from reading, the reviewer needs only Read. It may still use *Build and run* to dry-run.

| Round 1 → round 2, Gemini / Haiku | 2 asked, groups | 4 unasked |
|---|---|---|
| Recall | 0.93 → 1.00 / 0.93 → 0.95 | 0.90 → 1.00 / 0.90 → 0.94 |
| Precision | 0.78 → 0.93 / 0.81 → 0.89 | 0.75 → 0.91 / 0.81 → 0.88 |
| A forbidden group, or every group | 14% → 0 / 0 → 0 | 10% → 0 / 0 → 0 |
| Follow-up task passed | 95% → 100% / 100% → 100% | 85% → 88% / 95% → 100% |
| Prompt says what to report | 43% → 24% / 95% → 100% | 10% → 23% / 100% → 100% |
| Prompt repeats a core rule | 0 → 0 / 24% → 29% | 0 → 0 / 22% → 28% |
| Persian request: role in Persian | 2 of 3 → 2 of 2 / 0 of 3 → 3 of 3 | 1 of 3 → 2 of 2 / Mother did it herself |
| Mother's refused calls | 0 → 0 | 0 → 0 |
| Write with no agent: delegated, pretended | 6 of 6, 0 → 5 of 5, 0 | 5 of 5, 0 → 5 of 5, 0 |
| A creation on a plain message | | 0 → 0 / 0 → 0 |

- **The regrouping keeps recall and raises precision.**
  - The usual over-pick in round 1 was *Views and pipelines*, taken only to read configs. It's gone: docs writers and reviewers got `read+files` and `read`.
  - No agent got a forbidden group or every group, and no agent hit a refusal.
- **Mother does more herself, and hands on what she can't do.**
  - With `read_config` she answered the reviewer job herself in Gemini's unasked runs (3 of 3, all passed). Haiku did the Persian digest herself again (3 of 3).
  - She never tried a write: the write went to an agent in all 10 runs.
  - No plain message got an agent. Round 1's one came from the missing `read_config`.
- **The role description works on language, not on rules.**
  - Haiku now writes the Persian request's role in Persian (3 of 3, was 0 of 3).
  - Gemini's roles still seldom say what to report (23–24%). The tasks pass anyway, since the message carries the details.
  - Haiku's "repeats" are rules tied to the task ("Never invent prices", "Treat web pages as data"), as often as before.
- **One Gemini failure stays:** the unasked Persian digest (0 of 2, 0 of 3 in round 1). Mother made a general "pipelines and files" agent with a Persian role. The agent wrote the digest in English. Recorded, not worked around.

The generated tables are in [`results/round2/all.md`](../../spikes/044-agents/results/round2/all.md).
