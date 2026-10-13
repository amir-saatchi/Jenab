# TASK-018 — Rewind a chat to an earlier message
**Type:** Task
**Status:** Open
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
