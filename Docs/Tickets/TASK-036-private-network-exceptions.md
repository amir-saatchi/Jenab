# TASK-036 — Private-network exceptions in project settings
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** none

## Goal
SPEC 6.7 and 5.12 say a project can let hosts reach private network addresses, set in project settings. The code can store them (`project.SetPrivateHosts`, `internal/project/settings.go`), and turns use them (`internal/agent/turn.go`), but nothing calls it outside tests, and the settings page has no field. TASK-034's fake price site (`127.0.0.1:8099`) and P2-14's manual run need it.

## Scope
1. **Binding:** read and set the exceptions for a project, like `ChatService.SetLevel`, with `KindInvalid` for a host that isn't valid.
2. **Settings page:** a "Private-network exceptions" section in `frontend/src/shell/project-settings.tsx`: a list of hosts with add and remove, and one line on what it allows (requests to loopback and local addresses, for these hosts only). A host is a name or address without scheme, path or port.
3. **Effect:** the next request uses the new list; no restart.
4. **Export:** stays as it is (the exceptions are left out, as the export dialog says).

## Done when
- Go tests for the binding; a frontend test for the field.
- In the built app, with `127.0.0.1` added, a chat fetches a day from `jenab-fakesite` and reads Bitcoin's close (TASK-034's open check); removed, the fetch is blocked again.
