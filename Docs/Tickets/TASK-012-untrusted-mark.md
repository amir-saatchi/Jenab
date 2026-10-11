# TASK-012 — The untrusted mark and messages that join a turn
**Type:** Task
**Status:** Open
**Gate:** 3

## Goal
Decide when a user's message clears the untrusted mark, so a web page can't steer Auto into approving a host. Found in the harness review of 2026-10-10.

## Today
- SPEC 8.8 says Auto approves a host "only if the turn hasn't read untrusted content (web pages, files, MCP results) since the user's last message".
- `Send` clears `cs.untrusted` for every message (`agent/agent.go:243`), including one that joins a running turn.
  - The page read earlier in that turn is still in the context, and the model goes on with the same work.
  - So after a short message such as "also add ETH", Auto approves the next new host without asking.
- After a restart, `untrustedIn` (`agent.go:587`) clears the mark at any user text message, so it matches.
- Only Auto is affected; Strict and Standard always ask for a host. In Phase 1 that means `fetch_page`.
- No test covers a joining message (`approve_test.go:353` covers a new turn only).

## Scope
Decide one:
- **(a) Keep as is.** A message from the user means they're back in charge. Write the reason into SPEC 8.8.
- **(b) Only a new turn clears the mark.** A message that joins a running turn leaves it set; `untrustedIn` clears it only at the first user message of a turn.
- **(c) The mark stays while untrusted results are in the window**, and clears at the cut that drops them. This is the strictest; Auto would ask much more in research chats.

Proposal: (b). It closes the case within a turn and keeps the rule's meaning.

## Done when
- The decision is in SPEC 8.8.
- A test with Auto shows the decided behaviour: `fetch_page`, then a message that joins the turn, then a new host. It also holds after a restart.
- `go test ./...` passes.
