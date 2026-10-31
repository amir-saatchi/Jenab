# L3_memory_save

A memory save, asked for

Good behaviour: The user asks the chat to remember a format for all chats; it saves it with update_memory.

## Turn 1

**User:** Please remember this for all my chats: start every answer with a one-line summary that begins with "TL;DR:".

> call update_memory {"scope":"user","section":"user","content":"TL;DR: I can assist with tasks within this project, providing relevant information as needed."}

> result update_memory (error):
> wrong arguments:
> - at /: missing property 'expected_revision'

> call update_memory {"content":"TL;DR: I can assist with tasks within this project, providing relevant information as needed.","scope":"project","section":"project"}

> result update_memory (error):
> wrong arguments:
> - at /: missing property 'expected_revision'

> call update_memory {"section":"user","content":"TL;DR: I can assist with tasks within this project, providing relevant information as needed.","scope":"user"}

> result update_memory (error):
> wrong arguments:
> - at /: missing property 'expected_revision'

*[the turn stopped: the model sent tool calls that couldn't run, 3 times in a row. Use Retry to try again, or pick another model.]*

## Requests

| # | input | cache read | cache write | output | stop | took | error |
|---|---|---|---|---|---|---|---|
| 1 | 3005 | 0 | 0 | 61 | tool_use | 2.8 s |  |
| 2 | 3075 | 0 | 0 | 44 | tool_use | 3.5 s |  |
| 3 | 3145 | 0 | 0 | 44 | tool_use | 3.2 s |  |
