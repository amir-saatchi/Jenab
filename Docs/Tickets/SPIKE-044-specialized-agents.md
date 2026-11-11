# SPIKE-044 — Specialized agents: tool groups and a custom prompt
**Type:** Spike
**Status:** Open (Phase 5)
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
