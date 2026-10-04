# P1-05 — Bucket
**Type:** Feature
**Status:** Done
**Gate:** 3 (Phase 1)
**Needs:** P1-04
**Requirements:** R-70, N-25, N-30

## Goal
Each project's object store: keys, deduplicated content, versions and links (SPEC 4).

## Scope
- **Backend:** `bucket.ObjectStore` on the local folder, files named by SHA-256 under `objects/` (4.8).
- **Index:** the tables in 4.2, in `project.db`; the operations in 4.3, including the sweep for files no row points to (recovery step 4).
- **Links:** `jenab://` links and refs (4.5).
- **Serving:** `/objects/<project_id>/<key>` with the rules for untrusted files (4.7); the handler is mounted in P1-13.
- **Cache:** `cache/` objects get their expiry (4.4). The cleanup, size cap and *Free now* come in Phase 3 (R-71).

## Done when
- The same content stored under two keys is one file.
- A crash between writing the file and the index row never leaves a row pointing to a missing file (N-30).
- HTML is never rendered and SVG is served only for `<img>` (N-25), checked by handler tests.

## Result
- **`internal/bucket`:**
  - `Folder` stores bytes as `objects/ab/12/<sha256>`. `Put` streams into `tmp/` while hashing, flushes, renames, and on macOS and Linux also flushes the folders. Bytes already there are stored once. A size limit gives `ErrTooLarge`.
  - `CheckKey`, `CheckPrefix`, `Link` and `ParseLink` (4.1, 4.5).
  - `DetectMIME` reads the first 512 bytes, and finds SVG after an XML declaration, comments and a doctype. The key's extension only turns plain text into CSV, TSV, Markdown or JSON.
  - `Handler` serves `/objects/<project_id>/<key>`, for P1-13 to mount. HTML, XML and scripts go out as `text/plain`. SVG is an image only when the browser asks for an image. Unknown types are downloads. Every response has `nosniff`, and all but PDF get a sandboxing CSP. It also sends an ETag and supports ranges.
- **`store`:**
  - Format step 2 adds `_jenab_objects` and `_jenab_object_versions`, and the default lifecycle rules in `_jenab_meta`.
  - `PutObject`, `HeadObject`, `OpenObject`, `ListObjects` (`/` delimiter, pages), `ObjectVersions`, `DeleteObject`, `MoveObject` and `SweepObjects`.
  - Every put adds a version row; the same bytes as the current version add none. A delete keeps the versions and the bytes, for undo.
  - `Expires` comes from the lifecycle rules: 30 days for `cache/`.
- **Sweep:** removes bytes that no current or kept version points to, and `tmp/` files older than a day. A lock keeps it from removing bytes that a put is about to point to. Recovery runs it after a crash (2.7 step 4).
- **Left for later:**
  - The change log entries and undo (Phase 2). Until then, nothing in the undo window keeps bytes.
  - Object-reference columns: the check that a key exists, and updating them on `move` (Phase 2, with `_jenab_columns`).
  - Removing expired `cache/` objects and old versions, the cache size cap, *Kept for undo* and *Free now* (Phase 3, R-71).
  - The daily schedule of the sweep comes with the scheduler.
- **Tests:**
  - Keys, links, the folder store and MIME detection.
  - Handler tests for every type and for HTML and SVG opened any way (N-25).
  - Versions, moves, lists across folders and pages, read-only projects, rejected puts, and sweeps that race with puts.
  - Crash tests (N-30): a worker that puts, leaves stray bytes and sweeps is killed at random. After each kill, every row's bytes exist and match their hash, and the sweep leaves no file without a row. The project crash test checks that recovery removes stray bytes.
  - Mutation checks: 27 on keys, the folder store, MIME detection, the handler, the index, the sweep and recovery. 26 made the tests fail; the other, putting bytes that are already stored, only renames the same bytes over themselves.
