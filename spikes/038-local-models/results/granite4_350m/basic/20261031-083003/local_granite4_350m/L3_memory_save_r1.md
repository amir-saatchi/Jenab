# L3_memory_save

A memory save, asked for

Good behaviour: The user asks the chat to remember a format for all chats; it saves it with update_memory.

## Turn 1

**User:** Please remember this for all my chats: start every answer with a one-line summary that begins with "TL;DR:".

> call load_skill {"name":"memory"}

> result load_skill (error):
> there is no skill named "memory"; the skills are: database-design, migrations, sql-queries

**Agent:** No skill named "memory" found.

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3005 | 0 | 0 | 20 | tool_use | 1.4 s |  |
| 2 | 3058 | 0 | 0 | 9 | end | 1.4 s |  |
