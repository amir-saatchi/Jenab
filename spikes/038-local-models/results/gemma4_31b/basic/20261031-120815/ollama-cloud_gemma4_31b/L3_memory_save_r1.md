# L3_memory_save

A memory save, asked for

Good behaviour: The user asks the chat to remember a format for all chats; it saves it with update_memory.

## Turn 1

**User:** Please remember this for all my chats: start every answer with a one-line summary that begins with "TL;DR:".

> call update_memory {"content":"- Start every answer with a one-line summary that begins with \"TL;DR:\".","expected_revision":0,"scope":"user","section":"preferences"}

> result update_memory:
> Saved user memory, section preferences, revision 1.

*Memory updated: preferences*

**Agent:** TL;DR: I've saved your preference to start every answer with a "TL;DR:" summary.

Done. I will now start every response with a one-line summary beginning with "TL;DR:".

## User memory at the end

preferences: - Start every answer with a one-line summary that begins with "TL;DR:".

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3117 | 0 | 0 | 296 | tool_use | 5.4 s |  |
| 2 | 3440 | 0 | 0 | 69 | end | 2.0 s |  |
