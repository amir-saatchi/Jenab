# TASK-070 report: what the Wails app left behind

**Branch:** `claude/task-070-wails-leftovers`, from local `main` at `2e24221`. Work on 2026-11-17, Windows 11.
**Model requests:** none. **Downloads:** none. No new tools: callers were found with the compiler, `go vet` and grep (a small script that lists exported and unexported names with no caller outside tests, in the three packages).

## Commits

| Commit | What |
|---|---|
| `03ea3d0` | The removal, with the moved tests and the comments that named the removed code. One commit, so it is easy to read and revert. |
| the next one | The skills' line, CODE-OUTLINE, SPEC, this report and the ticket. |

## Removed

| What | Where | Why it could go |
|---|---|---|
| `bucket.Handler`, `bucket.Route`, `bucket.Served`, `bucket.Opener`, `sandboxCSP`, `servedType`, and its `shownImages` and `shownText` | `internal/bucket/serve.go` (the whole file) | Only `app.ObjectHandler` called them. |
| `TestServeTypes`, `TestServeRefuses`, `TestServeCachingAndRanges` and their helpers | `internal/bucket/serve_test.go` (the whole file) | They tested the HTTP headers (nosniff, CSP, `Sec-Fetch-Dest`, ETag, ranges), which only the route had. `bucket_test.go` used none of their helpers. |
| `app.ObjectHandler` and `leased` | `internal/app/bucket.go` | Only `TestBucket` called it; no server mounts it since TASK-061. |
| `TestBucket`'s route part and `objectPath` | `internal/app/bucket_test.go` | They went through `ObjectHandler`. |
| `UIError.MarshalJSON` and its test | `internal/app/errors.go`, `errors_test.go` | Nothing marshals a `UIError`. The logs use slog's text handler (`internal/logfile`), which writes `Error()`. The exports (`internal/chatexport`, `internal/project/export.go`) marshal chats, messages and progress, never an error. `internal/app/dev.go` marshals chat parts. The native code reads a `UIError` with `errors.AsType`, which `MarshalJSON` doesn't touch. |

## Moved tests

- **The route's refusals, now in `TestNativeBucketRead`**, through `NativeBucket.Read`, the path the file view takes: a missing key and another project are `not_found`; a bad project ID (`not-an-id`) is `not_found`, as the route's 404 was; a key out of the bucket (`../a.md`) and an empty key are `invalid` (the route answered 404 for both, but `Read` has always said `invalid`, so nothing changed); and after the reads no lease is kept (`!pm.Busy()`), as the route's test checked.
- **The route's types, now in `TestKindsSizesAndWho`** (`internal/native/state`), with `KindOf`'s current answers: XHTML, `text/xml`, RSS, CSV and plain text are text; `application/javascript`, ZIP and "not a type" are other. HTML, SVG, PNG, audio, PDF, JSON and octet-stream were in it already. `KindOf` has its own picture list, so nothing in it used `serve.go`.
- Not moved: the HTTP-only checks (headers, the 405 for POST, the 500 for a failing opener, ETag, `If-None-Match`, ranges, HEAD). They tested the transport, which is gone.

## Kept, and why

- **`internal/bucket`'s `folder.go`, `key.go`, `mime.go`:** the store, the agent's tools and the exports use them. `DetectMIME` is the type detection the file view relies on. `bucket.ErrNotFound` lives in `key.go` and is used by `store`, `tool`, `project/export.go` and `app/errors.go`.
- **`app.UploadRoute`, `UploadNameHeader`, `UploadHandler`, `Uploaded` and its JSON tags (`upload.go`):** the native app calls the route in its own process (`cmd/desktop`'s `uploader`, through `httptest`), and decodes `Uploaded` from its JSON answer. Only its comment changed: it said a binding would carry the body as JSON and a web page might call the route.
- **The event names and `app.Open` (`events.go`):** `internal/native/state.Stores.Apply` switches on them, and `cmd/desktop` builds the `Publisher`. They are Wails-style strings (`chat:delta`), but they are only names now. `EventActivity` has no case in `Apply`, but `project` emits it through its `Publisher` interface, so it stays with that interface.
- **`project.Publisher`, `Notice`, `Damage`, `Reporter`, `Status`, `Activity` (`internal/project/events.go`):** the agent and the app use them.
- **`UIError`'s JSON tags:** nothing marshals it, but the tags cost nothing and keep the field names if a log or export ever does. Removing them would be a change with no gain.
- **The other JSON tags in `internal/app`:** the DTOs (`ChatItem`, `Object`, `ObjectPage` and the rest) are Go values the native UI reads in its own process. Their tags were for Wails, but the sweep looks for unused code, and a tag is not code: removing them would touch many lines for no change. None was removed. `Uploaded`'s tags are in use, as above.
- **The rest of the sweep:** every other exported name in `internal/app`, `internal/bucket` and `internal/project` has a caller outside tests, and no unexported name is unused outside tests.

## Comments and names changed in the removal commit

- `BucketService`'s doc said files load from `/objects/<project_id>/<key>`, "never through a binding". It now points to `NativeBucket`.
- `NativeBucket`'s doc said "the app has no webview, so it reads files here, not through the objects route". The route is gone, so that part is cut.
- `upload.go`'s comment, as above.
- `TestPrivateHostsBinding` is `TestPrivateHostsSettings`: it tests the project settings, and "binding" was the Wails word.

## Skills

The twelve `.claude/skills/golang-*/SKILL.md` files say "a Go desktop app with a native UI on MyGo" instead of "a Go + Wails v3 desktop app". Nothing else changed in them, as the ticket says. One more Wails line is left: `golang-project-layout/SKILL.md` line 89, "Jenab's build tool is chosen in Gate 3 (Wails v3 uses a Taskfile)". It is out of this ticket's scope; it could say the app builds with `go tool mygo build`.

## Docs

- **CODE-OUTLINE:** `bucket`'s outline loses `Handler`; the note under it says the app serves no objects route and reads through `app.NativeBucket`. `app`'s outline loses `ObjectHandler`.
- **SPEC 4.7** was "Serving untrusted files", with the route's headers (`nosniff`, `Sec-Fetch-Dest`, CSP, `Content-Disposition`). It is now "Showing untrusted files": the file view shows a file by the MIME type detected when it was stored (`state.KindOf`); Markdown as Markdown, HTML, XML, JSON and scripts as text; pictures the kit can draw, SVG through MyGo's renderer; everything else, with audio, video and PDF, through *Open with the system app*. The upload bullet says the app calls the upload route in its own process. The rest of 4.7 (opening with the system app, limits, `.docx`) was already true.
- **History stays:** QUESTIONS Q31 ("files and images go through the asset handler `/objects/...`") and SPEC's version notes are decision records.

## Checks

- `go build ./...`, `go vet ./...`, `go vet -tags bench ./cmd/desktop`: pass.
- `go test ./...`: pass (43 packages ok, exit 0).
- The changed Go files are gofmt-clean.
- **Live check, bench build:** `go build -tags bench -o bin/jenab-bench-app.exe ./cmd/desktop`. Only the user's own Jenab was running; it was left alone.
  - `go run ./cmd/jenab-bench -app bin/jenab-bench-app.exe -hold 2s -keep -temp <scratch>` seeded the *Bench* project, opened *History 200*, held and ended with exit 0.
  - The app then started again on that scratch home (`LOCALAPPDATA`, `APPDATA`, `USERPROFILE`, `TEMP`, `TMP`) with `JENAB_BENCH_PORT`. `/state` said loaded, `build` `0.1.0-dev (2e24221, modified)`. `POST /open?title=History 200` opened the chat.
  - `POST /file?path=<scratch>/notes.md` stored `uploads/notes.md` as `text/markdown` and opened it next to the chat. `/shot` showed it rendered as Markdown (the heading, bold, the list) with *Version 1, Current*.
  - `POST /file` with a `page.html` holding a `<script>`: stored as `text/html`, and the file view showed the source as plain text.
  - `POST /quit` closed it. No system dialog opened.

## Notes

- The Files panel on the right still listed only `notes.md` after the second upload. This change doesn't touch the panel's listing, and whether `main` does the same wasn't checked; the bench's `/file` opens the file without going through the panel.
- Nothing was pushed or merged.
