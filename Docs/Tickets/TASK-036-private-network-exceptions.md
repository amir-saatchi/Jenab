# TASK-036 — Ask before reaching this computer or the local network
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** none

## Goal
SPEC 6.7 blocks requests to loopback, private, link-local and unique-local addresses, with exceptions per host in project settings. The code can store the exceptions (`project.SetPrivateHosts`, `internal/project/settings.go`) and turns use them (`internal/agent/turn.go`, `internal/web/fetch.go`), but nothing outside tests can add one: no card, no field. TASK-034's fake price site (`127.0.0.1:8099`) and P2-14's manual run need it.

The user's rule: **when the agent wants to reach an address on this computer or the local network, ask the user.**

## Scope
1. **Approval card** (8.8): when a chat's request would reach a private address and the host isn't an exception, the turn waits on a card instead of failing.
   - Options: *Allow once*, *Allow in this project*, *Deny*. *Allow in this project* adds the host to the exceptions.
   - It asks at every level, Auto too: Auto never allows this by itself.
   - Details: the host, the address it resolved to, and what that is in plain words ("this computer" for loopback, "your local network" for the others), the tool and the agent's reason. When a public name resolves to a private address, the card says so.
   - A redirect to a private address asks the same way, naming both hosts.
   - *Deny* returns the usual blocked error, with the user's note if any.
2. **Where it can't ask** (pipelines, scheduled runs, anything without a chat to show a card): blocked as today unless the host is an exception, with an error that names the project setting.
3. **Settings field:** a "Private-network exceptions" section in `frontend/src/shell/project-settings.tsx`: the hosts, with add and remove, and one line on what it allows. A host is a name or address, without scheme, path or port. A binding to read and set them, like `ChatService.SetLevel`, with `KindInvalid` for an invalid host.
4. **Effect:** the next request uses the new list; no restart.
5. **Export:** unchanged (the exceptions are left out, as the export dialog says).

## Done when
- Go tests: the card at each level (Auto included), once vs in this project, deny, a name resolving to a private address, a redirect, the pipeline error, the binding. A frontend test for the field.
- In the built app, a chat asked to read a day from `jenab-fakesite` shows the card; *Allow in this project* lets it read Bitcoin's close (TASK-034's open check), and the host appears in settings. Removed there, the next request asks again.

## Result
Done on 2026-11-02. SPEC 5.12, 6.7 and 8.8 describe it.

- **Built:** a `private` approval kind that asks at every level. The fetch's dialer holds back private addresses the exceptions don't allow, asks once per host, and connects only to the addresses the answer named; redirects record where a host came from. A waiting card pauses the tool's time limit. Without a card the request is blocked, naming the setting. `ProjectService.PrivateHosts` and `SetPrivateHosts` (a bad host is `KindInvalid`, checked by `web.ParseHost`), and a *Private-network exceptions* section in the project settings.
- **Tested:** web (the held addresses, a public name resolving inside, redirects, an answer's addresses only), tool, agent at Strict, Standard and Auto, project and app; a frontend test for the field.
- **Live:** a server build with `jenab-fakesite` and gemini-3.5-flash-lite (2 turns, 4 requests): the card appeared; *Allow in this project* read Bitcoin's close, $62,418, as on the page, and `127.0.0.1` appeared in the settings. Removed there, the next request asked again; *Deny* passed the user's note through.
- **Open:** at Strict and Standard a new private host gets two cards (host, then private); the card's reason is empty when the model writes nothing before the call; an exception covers every port.
