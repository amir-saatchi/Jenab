# TASK-050 — Find Project settings and the project's memory
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** —

## Goal
In P2-14's manual run (2026-11-15) the user looked for the project's memory in *Settings → Memory*, found it empty and couldn't edit it, and didn't see *Project settings* under the project name's menu. The project's memory was there all along. Three small fixes.

## Scope
1. **Project settings from Settings.** *Settings*' page list gets an entry for the open project, under its name, that opens its *Project settings*. With no project open, it isn't shown. The project name's menu keeps *Project settings*. The project's name is the title of *Project settings*.
2. **Settings → Memory says whose memory it is.** One line under the title: this memory is for every project, and the open project's own memory is in its settings, with a link to that page's *Memory* section. With no project open, the line has no link.
3. **The schema tool's row is readable.** The collapsed row of `request_schema_change` shows its state, such as "applied" or "asked", not the raw `ref: subagents/…` path. The full result, ref included, stays in the expanded row; what the model gets doesn't change.

## Rules
- UI only; nothing the model is sent changes.
- Right-to-left text as elsewhere (5.8).

## Done when
- A frontend test for each of the three.
- Screenshots of the Settings page list with a project open and without one, and of the schema row, in light and dark.

## Result
Done on 2026-11-16. Frontend only; what the model is sent is unchanged. Report: [TASK-050-report](reports/TASK-050-report.md).

- **Project settings from Settings:** under the pages, a group with the open project's name has *Project settings*. The open project is the one of the chat used last, where *Back to chats* returns; with none, the group isn't there. The project name's menu keeps *Project settings*. Project settings now has the project's name as its title.
- **Settings → Memory:** a line says this memory is for every project, and that the open project has its own, with a link to its *Project settings › Memory*. With no project open, there is no link.
- **The schema tool's row:** a result headed `[<tool>: <state> — ref: …]` shows only its state in the collapsed row, so `request_schema_change` reads "applied". The expanded row keeps the whole result.
- **Checks:** new frontend tests for all three; `bun test` 152 pass after the merge with TASK-051 and TASK-052; `bun run build` and the typecheck pass. Screenshots in light and dark, with and without a project, are kept outside the repo.
