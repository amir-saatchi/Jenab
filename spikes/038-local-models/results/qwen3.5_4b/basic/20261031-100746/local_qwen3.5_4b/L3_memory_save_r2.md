# L3_memory_save

A memory save, asked for

Good behaviour: The user asks the chat to remember a format for all chats; it saves it with update_memory.

## Turn 1

**User:** Please remember this for all my chats: start every answer with a one-line summary that begins with "TL;DR:".

> call update_memory {"scope":"user","section":"preferences","content":"Every answer should begin with a one-line summary starting with \"TL;DR:\".","expected_revision":0}

> result update_memory:
> Saved user memory, section preferences, revision 1.

*Memory updated: preferences*

**Agent:** TL;DR: Your preference to start every answer with a one-line summary beginning with "TL;DR:" has been saved to your user memory and will apply across all your chats.

## User memory at the end

preferences: Every answer should begin with a one-line summary starting with "TL;DR:".

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3548 | 0 | 0 | 226 | tool_use | 29.7 s |  |
| 2 | 3800 | 0 | 0 | 95 | end | 14.1 s |  |
