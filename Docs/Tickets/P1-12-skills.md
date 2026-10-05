# P1-12 — Skills
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 1)
**Needs:** P1-10
**Requirements:** R-83

## Goal
The skill mechanism, as decided in SPIKE-025 (SPEC 8.9).

## Scope
- **Format:** the skill format and its checks: name, description of at most 200 characters, `load_with`, a body of at most 3,000 tokens, and extra files.
- **Built-in:** built-in skills are embedded in the app.
- **Loading:**
  - the `## Skills` list in block 1, showing only the skills this chat can use
  - `load_skill(name, file)`
  - `load_with`
  - the limit of 6 skills or 10,000 tokens per chat
- **Staying loaded:** loaded skills are stored in `chats.skills` and move into block 1 at the next cut.
- **Phase 1 skill:** `config-guide`, as 8.9 lists. The others come with their tools.

## Done when
- Each bad field gives its own error.
- `load_with` loads the skill with the tool's first call.
- SPIKE-025's skill-loading set passes in the scenario runner (TASK-001) on two models. Moved to TASK-001's Done-when.

## Result
- **Format (`internal/skill`):** a skill is a folder: `SKILL.md` with front matter (`name`, `description`, `load_with`) and the body, and extra `.md` files. Each bad field is its own `*skill.Error` with the file and the field: unknown fields, a name that isn't the folder's, text fields that aren't text, a description over 200 characters or on two lines, `load_with` entries that aren't tool names or are listed twice, a body over 3,000 tokens, extra files that are folders, badly named, not UTF-8 or empty. CRLF and a byte order mark are accepted.
- **Built-in:** embedded with `go:embed`; `skill.Builtin()` reads them. Phase 1 has `config-guide` (views and pages from SPEC 5, three examples, `load_with: [save_view, save_page]`).
- **Block 1:** after the role come the loaded skills' text, then the `## Skills` list with SPIKE-025's header. `Skill.Mother` keeps a skill out of other chats' lists; `delegation` itself comes with Mother's guidance in Phase 5.
- **`load_skill(name, file)`:** in `agent.Tools()`, through `tool.Env.Skill`. The text comes back with `Result.Whole`, so it is never cut to a preview. An extra file comes back like any result and doesn't count as loaded. An unknown name or file lists what there is; a loaded skill says so.
- **`load_with`:** after a tool's output, the skills it names are loaded if they fit, and their text is added to the result, also when the call failed. Over the limit they are skipped without an error. The turn marks that result whole, so in-turn trimming never stubs it.
- **Limit:** 6 skills or 10,000 tokens, counting only skills that still exist. Over it, `load_skill` returns an error naming the loaded skills.
- **Staying loaded:** `ChatsDB.SetSkills` writes `chats.skills`. At the next cut, block 1 has the text; the old results become stubs as usual. *Clear* keeps the loaded skills (SPEC 8.6).
- **Chip:** a `skill_loaded` notice in the tool message; providers don't send notices.
- **Done-when check:** the user's decision was to move SPIKE-025's live check to TASK-001, whose Done-when now includes it.
- **Tests:** every bad field, folders and extra files, the set and its Mother-only skill, the built-in set, `load_skill` results, the chip and `chats.skills`, the move into block 1 at the cut, `load_with`, the limit by count and by tokens, `load_with` text kept whole, and no `Env.Skill`.
- **Mutation checks:** 51 on the new and changed code in `skill`, `agent`, `store` and `tool`; 46 made the tests fail at first. Of the other 5:
  - 2 got tests (the token rounding and folders among the extra files).
  - 1 was a broken mutant; redone, it makes the tests fail.
  - 1 was unused code for Mother-only skills, now removed.
  - 1 stays: `bumpSeq` in `setSkills`, which the notice written right after also bumps.
- **SPEC 5 questions from writing `config-guide`** (the skill follows the most likely reading):
  - parameters a view's query doesn't bind, and `VALUES` queries
  - the allowed `align` values and the chart series order
  - page actions with `open_form` and `open_page`, and `confirm` on `set`
  - composite keys in row actions
  - where validation checks `open_page` references
  - the "recipes from SPIKE-021" aren't a separate text; their content is in the examples
- **Not done:**
  - Wiring `skill.Builtin()` into `agent.Deps` and `agent.Tools()` into the app's registry (P1-13).
  - The extra loads from `request_schema_change` (Phase 2).
  - Skills in `create_chat` and `subagent`, user and project skills, and `delegation` (Phase 5).
