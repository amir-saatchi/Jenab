# TASK-041 — One card for a new private host
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** TASK-036

## Goal
At Strict and Standard, a fetch to a new host on this computer or the local network asks twice: first the new-host card (SPEC 6.7, 8.8), then the private-network card (TASK-036). The user answers the same question twice. Ask once.

## Scope
1. **One card:** when the host is new and its address is private, only the private-network card is shown. Its answer covers both: *Allow once* allows this request; *Allow in this project* adds the host to the private-network exceptions and approves it as a host for the project; *Deny* denies both.
2. **A public host** asks as today (the host card only). An exception host that isn't approved as a host yet still asks only the host card, as today.
3. **Auto:** unchanged: the private card still asks at every level.
4. **The card's reason:** when the model wrote nothing before the call, the reason is empty today. Show "The agent gave no reason" instead of an empty line, on this card and the host card.
5. Redirects keep naming both hosts.

## Rules
- Go first (`internal/agent/approve.go`, `internal/web/fetch.go`, `internal/project`), the card's frontend only if its text changes. No new dependencies.
- Tests with local fake servers only; no live third-party pages.

## Done when
- Go tests: a new private host at Strict and Standard gets one card; each answer's effect (once, in this project, deny) on the next request; a public host unchanged; Auto unchanged; the empty reason.
- In a server-mode build with `jenab-fakesite` (TASK-034), a new chat reading from `127.0.0.1` shows one card, at Standard. A fake model is fine; no API key needed.
- The report proposes the SPEC 6.7 and 8.8 text.
