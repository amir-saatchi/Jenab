# P2-09 — User and project memory
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 2)
**Needs:** P2-02, P2-03
**Requirements:** R-20, R-24

## Goal
User memory and project memory, in sections with caps and revisions, as blocks 2 and 3 (SPEC 3.1, 3.3).

## Scope
- **Storage:** `_jenab_memory` in `project.db`, and `user_memory` in `registry.db` (created in P1-03). Default sections `goals`, `preferences`, `decisions` and `conventions`; the agent may add more.
- **`update_memory(scope, section, content, expected_revision)`:** a stale revision is refused with the current content, so the agent can merge. Empty content deletes the section. Caps: 1,000 tokens for user memory, 2,000 for project memory.
- **Change log and undo:** every edit is a change with the target `memory:<scope>/<section>`. User memory lives in `registry.db`, so its before-revision is kept there and the entry in the project's log points to it. Decide this in the ticket.
- **Content rule:** memory holds intent, never values that live in tables (3.3). It is in the tool's description, the same for every model.
- **Context:** blocks 2 and 3, rebuilt at a cut. An edit from another chat between cuts reaches this chat as a notice, `[memory updated by another chat: preferences → prices in EUR]`, and joins its block at the next cut. For Anthropic, the cache point moves after block 5 (3.1).
- **Chat:** the chip `Memory updated: preferences [undo] [edit]`.
- **UI:** project memory in project settings and user memory in *Settings*, as editable sections. A user's edit is recorded with the source `user`.
- Subagents, pipelines and the schema agent can't write memory.

## Done when
- Tests: a revision conflict and the merge, the caps, delete by empty content, undo of an edit, the notice to a second chat and the block after its cut.
- A scenario: the user states a preference in one chat, and a new chat follows it without being told.

## Result
