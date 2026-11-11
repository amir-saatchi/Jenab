# SPIKE-043 — Windows write sandbox without admin rights
**Type:** Spike
**Status:** Open
**Gate:** 3 (Phase 6)
**Needs:** SPIKE-042

## Question
Can Jenab run commands so that they can write only inside the linked folder, with no admin rights, from Go (`golang.org/x/sys/windows`)?

## Scope
1. **Token:** a write-restricted token with a random capability SID for each workspace, and an allow-write ACE for that SID on the folder. Deny-write on protected folders (`.git/`, Jenab's own).
2. **Tools still work:** `go build` (cache under `%LOCALAPPDATA%`), `bun install`, `git` (`index.lock`), temp folders. Which extra write roots are needed?
3. **Cleanup:** remove the ACEs when the folder is unlinked, and after a crash.
4. **Fit:** with the Job Object, below-normal priority and the memory checks (8.5).
5. **Consent:** the ACEs change permissions on the user's folder. How is that shown and agreed?
6. **Limits to document:** reads are not isolated; the network is not controlled (SPEC note A).

## Rules
- No admin rights, no new users, services or firewall rules.
- Write it fresh from the idea; copy no Codex code.
- Windows only; macOS and Linux are out of scope.

## Done when
- A spike program shows the four checks above on Windows 11, with results.
- A go or no-go decision, and if go, SPEC 8.5 text. Whether the sandbox lets Auto skip any command card is **not** decided here; that would need its own spike (8.8).
