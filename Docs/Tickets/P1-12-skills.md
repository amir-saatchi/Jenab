# P1-12 — Skills
**Type:** Feature
**Status:** Open
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
- SPIKE-025's skill-loading set passes in the scenario runner (TASK-001) on two models.
