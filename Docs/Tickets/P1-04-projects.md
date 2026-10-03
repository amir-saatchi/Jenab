# P1-04 — Projects: folders, lock and recovery
**Type:** Feature
**Status:** Done
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

## Result
- `internal/project`: `Manager` (create, list, open, `Busy`, `CloseAll`), `Project` (leases, `Release`, `Context`, `Report`, `Activity`, `OnClose`), recovery and the lock file, the folder warning, and the `Publisher` with `Notice` and `Activity`. 15 tests; `go vet` passes for Windows, macOS and Linux, and `go test ./...` passes on Windows.
- **Create** makes the 2.1 folders and `project.db`, writes the ID, name and creation time to `_jenab_meta`, then adds the registry row. A failed create removes its new folder.
- **Registry rebuild:** `List` adds every project folder the registry doesn't know, reading its `_jenab_meta` without starting a writer. This covers a lost `registry.db` and a create cut off before its registry row. Folders that aren't readable projects are logged and skipped.
- **Leases:** one `Project` per open project; parallel opens share it (tested with 20). An open during an idle close waits for the close, then opens again. A timer that has already fired is ignored through a generation number. `CloseAll` refuses new opens and closes every project in parallel. `OnClose` runs newest first, and a panic in one is logged.
- **Recovery** (2.7) runs when `jenab.lock` is there:
  - `quick_check` on `project.db` and `chats.db`;
  - then `tmp/` is emptied and partial snapshots are deleted;
  - step 2 waits for runs (Phase 4), step 4 for the bucket (P1-05).

  A failed check opens the project read-only, with the newest snapshot of that file in `Damage`, and writes return `store.ErrReadOnly`. If the databases don't close cleanly, the lock stays, so the next open checks again.
- **Store additions:** `Options.ReadOnly`, `QuickCheck`, `OpenProjectReadOnly`, `ReadProjectMeta` and `ErrReadOnly`.
- **Folder warning:**
  - Network drives: a UNC path or `GetDriveType` on Windows; the file system type on macOS and Linux.
  - Synced folders: OneDrive, Dropbox, iCloud Drive and Google Drive, found from their usual folders, the OneDrive environment variables, Dropbox's `info.json` and Google Drive's volume label.
  - Links are followed first.
  - iCloud's *Desktop & Documents* sync is not detected.
- **Tests:**
  - The kill test ran 10 kills on Windows. Each time the lock was found, `tmp/` and the partial snapshot were cleared, no batch was half-written, and the project took writes.
  - Idle close runs under `synctest`.
  - A damaged file opens read-only.
  - The OneDrive folder and `\\localhost\C$` are detected on this machine.
  - Three mutation checks made the tests fail: skipping recovery, removing both idle-timer guards, and ignoring the drive type.
  - The race detector needs cgo, which this machine doesn't have.
