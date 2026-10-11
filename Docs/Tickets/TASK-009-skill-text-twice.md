# TASK-009 — Skill text is sent twice after a cut
**Type:** Bug
**Status:** Done
**Gate:** 3

## Goal
After a cut, a loaded skill's text is only in block 1, not also in the tool results kept in the window. Found in the harness review of 2026-10-10.

## Today
- `load_skill` returns the skill's text with `Whole` and no ref (`agent/skills.go:72`).
- `tool.Stub` leaves a result without a ref as it is (`tool/output.go:78-81`), and `shape` uses it for results from before the cut (`agent/context.go:146-149`).
- At the next cut the skill moves into block 1 (`context.go:233`). The turns kept in the window (`history_min_turns`, 3) still have the full text in their results.
- A skill loaded with `load_with` is added to the tool's own result (`agent/turn.go:629`), and stays there too.
- So a skill can be in the request twice for several turns, up to 10,000 tokens each (`skill.MaxLoadedTokens`).
- CODE-OUTLINE 7 says the text in results becomes a stub at the next cut. `TestSkillMovesIntoBlock1AtTheCut` checks block 1 but not the results.

## Scope
- **`load_skill` results.** After the cut, the result of a skill that is in block 1 becomes a short stub, such as `[skill sql: moved into the system prompt]`. The result is found from the history: it follows a `skill_loaded` chip, as in `carriesSkill` (`context.go:211`).
- **`load_with` results.** The result keeps the tool's own output and drops the added skill text. This needs the boundary between the two. Pick one:
  - (a) Store the tool's output length in `ToolResult`, as a new field.
  - (b) Store the skill text as a separate part after the chip, and join them only when the request is built.
- **Unchanged:**
  - Extra files read with `load_skill(name, file)` aren't loaded skills, so they stay as they are.
  - Until the cut the text stays whole, and trimming keeps it.
  - Retry and restart decide from the history, not from memory.

## Done when
- A test shows that after the cut, block 1 has the skill and no result in the window has its text.
- A `load_with` result keeps the tool's output after the cut.
- A file read with `load_skill` stays whole.
- `go test ./...` passes.

## Result
After a cut, a loaded skill's text is only in block 1. Done with option (b): the text is a part of its own.

- **Storage.** `addSkill` writes the chip and then a `skill_text` notice with the skill's text. Both carry the skill's name in a new `Notice.Skill` field. `load_skill`'s result itself is empty. `load_with` no longer adds to the tool's result.
- **The request.** `shape` joins each `skill_text` part to the next result in its tool message, so the model reads what it read before. In turns before the last cut, a skill that is in block 1 joins as `[skill sql: moved into the system prompt]`. A skill that isn't in block 1, for example one removed from disk, keeps its text. The window records which skills block 1 has at each cut. `skill_text` parts are not sent on their own.
- **Trimming** no longer needs `carriesSkill` (removed). A `load_with` result is trimmed like any other, and the skill text, which is not in the result, stays.
- **Unchanged:** files read with `load_skill(name, file)` stay whole. Retry and restart read from the history.
- **Elsewhere:**
  - The frontend hides `skill_text` notices (bindings regenerated).
  - The scenario recorder skips them.
  - `read_messages` shows the text as a notice, where it used to show it in the result.
  - SPEC 8.9 and CODE-OUTLINE 7 are updated.
- **Tests:**
  - `TestSkillTextAfterTheCut`: after the cut, block 1 has both skills; `load_skill`'s result is the stub; `load_with`'s keeps "saved" with the stub; the file read stays whole; no `skill_text` part is sent.
  - The skill tests read results as the model sees them.
  - The trimming tests now check that a trimmed `load_with` result keeps its skill, also after Retry.
