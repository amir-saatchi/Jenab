# TASK-018 — Rewind a chat to an earlier message
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** P1-15

## Goal
The user can go back to one of their earlier messages, change it or keep it, and go on from there, as if the later turns hadn't happened. Found while testing the app on 2026-10-13.

## Today
- A chat only grows. A wrong turn stays in the context, and the user can only argue with it in the next message.
- *Undo turn* (SPEC 2.6) reverts a turn's changes to the project's data, but it doesn't take the conversation back, and it comes with Phase 2's change log.

## Scope
First write it into the SPEC (2.3, 3.6, 8.3), then build it:
1. **Where:** *Rewind to here* on a user message, and *Edit and resend*, which rewinds and puts the message back in the composer.
2. **What goes:** the later turns leave the chat and the context. Whether they are deleted or kept as a hidden branch is decided here. Without branches, they go into the undo window like other deletions (4.4).
3. **Data changes:** if the dropped turns changed project data, the app lists the changes and offers *Undo turn* for them (2.6). Rewinding never undoes changes silently.
4. **Everything that follows the chat** stays consistent:
   - the history search index (2.3)
   - session notes written in the dropped turns (3.4)
   - skills loaded in them (8.9)
   - background tasks and subagents they started (8.3, 8.6), which are stopped or kept, said in a notice
   - chats Mother created or messaged from them (8.6)
5. **Cache:** a rewind is a cut (3.6), so the next request starts a new cached prefix.
6. **Not during a turn:** like *Clear*, only between turns.

## Not in scope
- Branches you can switch between, unless point 2 decides for them.

## Done when
- The SPEC describes it.
- A rewound chat answers as if the later turns never happened, also after a restart.
- Go tests cover the history index, loaded skills and running background work.

## Result
Done on 2026-11-06. SPEC 2.3, 2.6, 3.4, 3.6, 8.3, 8.6 and 8.9 describe it.

- **No branches:** *Rewind to here* and *Edit and resend* on a user message move the later turns to `rewound_messages` (chats.db step 6). They leave the chat, the context and the history index. *Undo rewind* brings them back for the newest rewind while the chat hasn't gone on; they are dropped with the change log after 90 days.
- **State follows:** session notes and loaded skills go back through a new `turn_state` table (the state before each turn first changed it); *Refresh memory*'s progress is reset. Usage still counts the dropped turns.
- **Data changes stay:** the dialog lists them, the model gets a hidden notice, and the rewind's mark has *Undo changes*.
- **Between turns only,** like *Clear*.
- **Checked:** Go tests in store and agent (index, notes, skills, Usage, data kept, the next request free of the dropped turns, undo, restart); by hand in a server build with a fake model: rewind, undo, edit and resend, restart.
- **Open:**
  - Desktop sessions are keyed by turn number, so after a rewind or *Clear* a new turn with the same number within 10 minutes could reuse a dropped turn's window; end the chat's desktop session on *Clear*, rewind and undo (TASK-038's area).
  - Chats from before this change have no earlier notes or skills recorded, so a rewind keeps theirs.
  - Subagent transcripts of dropped turns stay in the bucket until *Clear* or delete, like a stopped turn's.
  - *Edit and resend* brings back attached files as links, not chips.
