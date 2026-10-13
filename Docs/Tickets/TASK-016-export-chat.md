# TASK-016 — Export a chat
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** P1-15

## Goal
The user can save a chat as a file, to read, share or keep it outside the app. Found while testing the app on 2026-10-13.

## Today
- A chat can't be exported. The SPEC exports only whole projects (2.1, project settings in 5.12).

## Scope
First write it into the SPEC (2.3 and 5.12), then build it:
1. **Where:** *Export chat…* in the chat header's menu, and in the chat row's menu in the sidebar. A save dialog picks the file.
2. **Markdown**, for reading:
   - a header with the chat's title, its project, its model and the date
   - each turn's user message and answer
   - tool calls as one line each (tool, short arguments, result size), not full results
   - notices and approvals as short lines
   - thinking left out
   - images and files as their `jenab://` links
3. **JSON**, for keeping everything: every message part as stored (2.3), with the format version.
4. **Safe:** secrets are redacted as in 6.7, and keys never appear.
5. **Text direction:** right-to-left text stays readable in the Markdown (5.8).

## Not in scope
- Importing a chat.
- Exporting several chats at once (the project export covers that).

## Done when
- The SPEC describes it.
- Both formats work from the app on Windows, and a long chat with tool calls and Persian text exports correctly.
- Go tests cover both formats and the redaction.
