# P2-12 — Export a project
**Type:** Feature
**Status:** Open
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
