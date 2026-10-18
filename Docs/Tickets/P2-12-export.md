# P2-12 — Export a project
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 2)
**Needs:** P1-04
**Requirements:** R-03, N-31

## Goal
Export makes a consistent copy of an open project (SPEC 2.1, 7.5).

## Scope
- **Copy:** `VACUUM INTO` for `project.db` and `chats.db` with the safe-copy rule, then `objects/` and `skills/`. The folder is never copied as it is, since the WAL may hold committed data.
- **Format:** a folder, or one zip file, chosen in a save dialog. Copied into the data folder, it opens as a project.
- **While it runs:** writes go on; the copy is of one moment per database.
- **UI:** *Export* in the project's menu, with progress and the size.
- **Not here:** importing someone else's project with its skills, which asks first (8.9), and exporting a chat (TASK-016).

## Done when
- An export taken during a run of writes opens as a valid project with every committed row.
- A crash test: a killed export leaves no file with a final name.

## Result
Done on 2026-10-18 (commit 33ea187), after three review rounds.

- **Built:** `internal/project/export.go` (folder or zip, progress, stop), `internal/store/export.go` (`VACUUM INTO` and the edit of the copy), and *Export…* in the project menu with progress and the size.
- **Tests:** an export taken during a run of writes opens as a project with every committed row. A crash test kills exports mid-way: a finished export has its final name, and a cut one leaves only a `.partial`, which the next open removes.
- **Changes from the ticket:**
  - The copy gets a new project ID and records `exported_from`.
  - Only the objects and skills the copy references are copied.
  - Approvals are left out unless the user ticks *Include approvals* (off by default).
  - The export is written to `<target>.<id>.partial` and renamed without replacing anything. A record in `tmp/` lets the next open remove a partial left behind, through recovery too. Long Windows paths use the `\\?\` prefix.
  - The export works with a query worker reading: `VACUUM INTO` doesn't wait for it.
- **Known limits:**
  - `project.db` and `chats.db` are copied one after the other, so `chats.db` can refer to an object added between the two copies.
  - Old `jenab://` links in the copy don't resolve, because the project ID is new.
  - Where the no-replace rename isn't available, a file that appears at the target in the last moment can be replaced.
  - No coordination yet with a future live sweep of `objects/`.
