# L3_memory_save

A memory save, asked for

Good behaviour: The user asks the chat to remember a format for all chats; it saves it with update_memory.

## Turn 1

**User:** Please remember this for all my chats: start every answer with a one-line summary that begins with "TL;DR:".

> call update_memory {"scope":"user","section":"preferences","content":"Every answer must start with a one-line summary beginning with \"TL;DR:\".","expected_revision":0}

> result update_memory:
> Saved user memory, section preferences, revision 1.

*Memory updated: preferences*

**Agent:** Done - your preference is saved for all chats from now on.

**TL;DR:** Answer format preference updated.

I'll start my next answers with "TL;DR:" followed by a one-line summary.

## User memory at the end

preferences: Every answer must start with a one-line summary beginning with "TL;DR:".

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3548 | 0 | 0 | 166 | tool_use | 51.8 s |  |
| 2 | 3740 | 0 | 0 | 77 | end | 11.7 s |  |
