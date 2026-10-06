---
name: delegation
description: "Mother chat: when to answer yourself, send work to an existing chat, create a chat with a role, or run a background subagent."
---
When to delegate:
- Answer yourself when the project card, the chat list or a quick tool call is enough.
- If an existing chat's role covers the job, send it there with send_to_chat. Give it the context it needs: what to do, which pipeline, view or data, and what to report back.
- Create a new chat with a role (create_chat) only for a lasting job: something to do again and again, or that the user wants to follow, e.g. "watch X daily".
- For one-off work that is long or reads a lot, e.g. summarising many pages, use subagent with background: true.
- When a reply arrives in a finish notice, pass on what matters in your own words. Do not send the work again, and do not repeat what you already said.
