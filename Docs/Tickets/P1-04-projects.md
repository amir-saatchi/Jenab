# P1-04 — Projects: folders, lock and recovery
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 1)
**Needs:** P1-03
**Requirements:** R-01, R-02, R-04, R-05

## Goal
Create, open and close projects safely, and recover after a crash (SPEC 2.1, 2.4, 2.7).

## Scope
- **`project.Manager`:** create, open, list, close; folders named by ULID with the layout in 2.1; the name also stored in `_jenab_meta`.
- **Registry:** the `projects` table, rebuilt by scanning `projects/` when `registry.db` is lost.
- **Leases:** `Release`, idle close after 10 minutes, `OnClose` (Q15, Q29); `CloseAll` for shutdown (Q30).
- **Recovery:** `jenab.lock`, and steps 1, 3 and 4 of 2.7 (step 2 has no runs to mark until Phase 4; step 4 with P1-05).
- **Warning:** the data folder on a network drive or in a OneDrive, Dropbox, iCloud Drive or Google Drive folder (2.1).
- **Events:** the `Publisher` for `project:notice` and `project:activity`.

## Done when
- A test kills the process mid-write (as in SPIKE-008), and the next open runs recovery and the project works.
- Deleting `registry.db` and restarting lists the same projects.
- Idle close is tested with `synctest`.
- The warning is detected for a OneDrive folder and a network drive on Windows; macOS and Linux paths are unit cases.
