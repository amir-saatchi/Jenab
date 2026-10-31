# L3_memory_save

A memory save, asked for

Good behaviour: The user asks the chat to remember a format for all chats; it saves it with update_memory.

## Turn 1

**User:** Please remember this for all my chats: start every answer with a one-line summary that begins with "TL;DR:".

> call update_memory {"scope":"user","section":"preferences","content":"Always start every answer with a one-line summary that begins with \"TL;DR:\".","expected_revision":0}

> result update_memory:
> Saved user memory, section preferences, revision 1.

*Memory updated: preferences*

**Agent:** TL;DR: I've saved your preference — every answer will open with a one-line "TL;DR:" summary.

What would you like to work on next?

## User memory at the end

preferences: Always start every answer with a one-line summary that begins with "TL;DR:".

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3166 | 0 | 0 | 279 | tool_use | 20.1 s |  |
| 2 | 3470 | 0 | 0 | 59 | end | 6.5 s |  |
