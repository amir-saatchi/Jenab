# P1-05 — Bucket
**Type:** Feature
**Status:** Open
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
