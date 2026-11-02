# TASK-019 — Reply to part of a message
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** P1-15

## Goal
The user can select part of an earlier message and reply to that part, so the agent knows exactly what the reply is about. Found while testing the app on 2026-10-13.

## Today
- The user can only write a new message. To point at an earlier passage, they copy it in by hand.

## Scope
First write it into the SPEC (2.3, 5.8), then build it:
1. **Where:** selecting text in a message shows *Reply*. *Reply* on a whole message quotes all of it, up to a size limit.
2. **Composer:** the quote shows above the input as a chip that can be removed. One quote per message.
3. **Stored:** the user message gets a `quote` part with the quoted text and the message it came from (2.3), so the chat can jump to the source.
4. **Sent to the model:** the quote goes into the user message as plain text, marked as a quote, the same for every model (SPEC 1). A long quote is cut, with its source.
5. **Shown:** the quote appears above the user's text in the chat, and clicking it scrolls to the source.
6. **Right-to-left** quotes keep their direction (5.8).

## Not in scope
- Several quotes in one message.
- Quoting tool results or pages (they have refs).

## Done when
- The SPEC describes it.
- A quote round-trips: stored, shown, sent and found by history search.
- Go and frontend tests cover the part and the text sent to the model.

## Result
Done on 2026-11-02. SPEC 2.3, 5.8 and 5.12 describe it.

- **Built:** a `quote` part, stored first; Go checks the source and fills in its turn and role. The model gets it as a text part when the request is shaped, so providers never see a new kind and requests stay append-only. A `chats.db` format step widens the index's delete trigger to quotes.
- **UI:** *Reply* on a selection, on a user message and in the answer's footer; the chip above the input; a sent quote above the text that jumps to its source and rings it for 1.6 s, or says the message is gone.
- **Decided here:** one 4,000-byte limit for the chip, storage and the model. A whole-message *Reply* quotes the stored text, so an answer's Markdown shows as written; a selection quotes the rendered text.
- **Checked:** Go tests for storage, Persian search (index and substring), the format step, the exact text the model gets, the cut line, refused sources, history cards, `read_messages` and the export; frontend tests for the cut, the store and the chip. By hand in a server-mode build with a fake model at 1280 and 760 px: a Persian selection, the chip, the text sent, the jump, both whole-message replies and a restart. Search has no UI yet, so search was checked in Go only.
- **Seen, not fixed:** an answer bubble is only as wide as its longest line, so a Persian paragraph in a mixed answer aligns right within that width.
