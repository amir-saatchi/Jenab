# TASK-016 — Export a chat
**Type:** Task
**Status:** Done
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

## Result
Done on 2026-11-01. SPEC 2.3 and 5.12 describe it.

- **Built:** `internal/chatexport` writes Markdown or JSON from batches of 50 turns, to a temporary file renamed when complete. `ChatService.Export` names every provider's key and URL fields for the redactor and writes the model as "default (provider/model)" when the chat follows the default. In the UI, a ⋯ menu in the chat header and on each sidebar row opens a dialog with a Markdown/JSON toggle, then the save dialog.
- **Redaction:** `secret.Store.Redactor` reads the keys once per export and replaces each one, raw and in both JSON-escaped forms, longest first. `Store.Redact` now goes through it, so provider error bodies are also caught when JSON-escaped. It has no minimum length, as before: the keychain holds only secrets. Only keys Jenab stores are known.
- **Tests:** a fixture with a key in every field (title, text, thinking, tool arguments across the 80-character cut, results, errors, approvals, questions, image alt, notices, notes), checked raw, JSON-escaped and by its first 14 characters in both files; exact Markdown lines and right-to-left blocks; a JSON round trip; 1,200 turns read 50 at a time; a failed write keeps the old file.
- **On Windows:** a 300-turn Persian chat with tool calls exported in about 26 ms per format (Markdown 119 KB, JSON 894 KB, 1,200 messages). The native save dialog is the one project export uses and wasn't driven in the test.
