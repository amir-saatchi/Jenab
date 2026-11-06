# TASK-021 report: read Word files

- **Branch:** `claude/adoring-varahamihira-1dfb46`
- **Head:** `05f026d506e1ad9c0f89ad97268d5f897475620c`
- **Commits:** 6, all signed off. Nothing is pushed or merged.

| Commit | What |
|---|---|
| 69e931b | `internal/docx`: Word to Markdown |
| b4b4d74 | Word detected when stored; OLE type; system-app rules |
| d9562ee | read tools read Word through its text; links as keys; default-style fix |
| 530aed8 | upload route `/upload/<project_id>` |
| 5d26e1c | composer attach button |
| 05f026d | real Word files in the tests; blank line between lists |

## What was built

1. **Attach button.**
   - The paperclip in the composer picks any number of files. Each one is stored right away through `POST /upload/<project_id>`.
   - Each file shows as a shadcn `Attachment`, uploading → done/error, with a remove button. Removing a file from the message keeps it in the bucket.
   - Send waits while uploads run. It appends each stored file's `jenab://` link on its own line, so a message can also be links alone.
   - Over 100 MB is refused in the browser and by the route.
   - Keys:
     - The key is `uploads/<name>`, made to follow the key rules: each run of other characters becomes `-`, ends are trimmed, a name with nothing left is `file`, and the name part is at most 120 bytes.
     - A taken key gets `-2`, `-3` and so on before the extension. A per-route reserved set covers uploads of the same name at the same time.
     - The original name goes in `metadata.name`, which the Files panel already shows.
   - Fields: source `upload`, `created_by` `user`.
   - The route needs the `X-Jenab-Upload-Name` header (percent-encoded name). It refuses `Sec-Fetch-Site` other than same-origin, and OPTIONS gets 405, so another origin or a sandboxed object can't post.
   - The answer is `{key, link, mime, size, name, note}`. `note` says why a document can't be read ("it is password-protected"). The file is stored anyway.
2. **Detection.**
   - A zip is looked at after it is stored. `[Content_Types].xml` must give the main document part the type `…wordprocessingml.document.main+xml`, compared case-insensitively, and that part must exist. Such a zip is `application/vnd.openxmlformats-officedocument.wordprocessingml.document`.
   - A macro-enabled main part gives `application/vnd.ms-word.document.macroEnabled.12`.
   - Any other zip stays `application/zip`.
   - OLE compound files (magic `D0CF11E0`) are now `application/x-ole-storage`. These are old .doc/.xls/.ppt files, and Office files saved with a password.
3. **Extraction** (`internal/docx`, pure Go; only `golang.org/x/text`, which was already in go.mod).
   - Output is Markdown.
   - **Headings:**
     - Taken from the paragraph's own outline level, then the style chain through `basedOn` (32 hops, loops stopped).
     - Within the chain, the style's *name* counts: "heading N", or "Title" as level 1. So does its outline level. Never the style ID.
     - Levels deeper than 6 become 6. An outline level of 9 means body text.
   - **Lists:**
     - From numbering.xml (abstractNum, num, lvlOverride/startOverride, numStyleLink, a style's numPr), with counters per list and level.
     - Bullets are `-` and numbered items `N.`; other formats are written as numbers.
     - Items are nested by indentation.
   - **Tables:**
     - Header = first row; gridSpan and vMerge are mirrored; `|` is escaped and line breaks become `<br>`.
     - Over the SPIKE-015 bounds a table becomes paragraphs. Those bounds are 250,000 cells, 1,000 columns, 50,000 span cells and 2 MB of span text per table, plus an 8 MB span budget per document.
   - **Links:** hyperlinks by relationship or anchor, plus HYPERLINK fields and fldSimple. Edge spaces stay outside the brackets.
   - **Notes:** footnotes and endnotes as `[^n]` with their text at the end.
   - **Tracked changes:** accepted. Insertions and moveTo are kept; deletions, moveFrom and deleted rows are dropped; a deleted paragraph mark merges two paragraphs.
   - **Other content:**
     - Text boxes are written after their paragraph.
     - `[image: alt]` uses the drawing descr or title or VML alt; charts count as images.
     - OMML equations and Equation OLE objects become `[equation]`.
     - `mc:AlternateContent` takes its first Choice.
     - Hidden (vanish) text is dropped.
   - **Text:** NFC with the ZWNJ kept.
4. **Limits for untrusted input:**
   - Nothing is unpacked to disk; the zip directory is read through a 4 MB budget.
   - At most 2,000 entries and 50 MB unpacked. The declared sizes are checked first, so a bomb fails at once.
   - Per-part ratio stop: 100× the packed size, with a 1 MB floor. `[Content_Types].xml` and .rels files are capped at 4 MB.
   - XML depth is capped at 256. The zip "encrypted" flag is refused.
   - Errors, all of the form "could not read <key>: …":
     - "it is password-protected"
     - "it is an old Word, Excel or PowerPoint file (.doc, .xls or .ppt); only .docx can be read, so save it as .docx"
     - "it is damaged: …"
     - "it is not a Word document"
     - too large
5. **Text cache:** `cache/text/<sha256>-v<n>.md` (n = `docx.Version` = 1), source `tool`, with title "Text of <key>". It expires with `cache/`.
   - The text is made at upload, and again on read when the cache is missing or expired.
   - `bucket_read`, `read_ref` and `search_ref` read Word through it. The ref stays the document's own key, and the cache key is never shown. Header: `[object uploads/x.docx — Word document — N tokens, showing first M — ref: uploads/x.docx]`.
   - All three tools, and `bucket_read`'s key, also take the file's `jenab://` link. A link to another project is refused.
   - If the project can't be written to, the text is returned without caching.
6. **Tests:**
   - Zip bomb, over 50 MB, over 2,000 parts, a 120k-entry directory, nesting too deep, cut short, not XML, no document part, a zip that only claims to be Word, encrypted OLE, an old .doc, the zip "encrypted" flag, random bytes, too short.
   - Each of those runs in under 1 s with under 100 MB allocated.
   - Also: cancelation, table bounds, Detect cases, and golden files for the built documents (headings with German style IDs, lists, tables, links and notes, changes, objects, Persian).
   - Mutations: TestMutations runs 20k deterministic inputs, 3k with `-short`; the slowest was about 38 ms. `FuzzExtract` has seeds. On this Windows machine `go test -fuzz` stalls at 0 execs/s even for a trivial target, so the fuzzer itself only ran for seconds.
   - Tool tests: header, link as key, read_ref/search_ref offsets, re-made after delete and after expiry, and the encrypted, old and damaged messages.
   - Upload tests: key making, numbering (also at the same time), notes, refusals, a body over 100 MB, no lease kept.
   - Frontend: `lib/upload.test.ts`.

## Proposed SPEC text (for Main-Session to write)

**3.7, a new bullet after "Web pages are fetched…":**

> - Word documents (TASK-021) are read through their text: Markdown made in Go and stored under `cache/text/<sha256>-v<n>.md`, where `n` is the extractor's version. The text is made when the file is uploaded and again when it has expired or the version changes. `bucket_read`, `read_ref` and `search_ref` read and search it; the ref stays the document's own key and the header names the type: `[object uploads/report.docx — Word document — 2,400 tokens, showing first 1,500 — ref: uploads/report.docx]`. The cache key is never shown to the agent.
> - **Word to Markdown:**
>   - Headings come from the paragraph's outline level, then its style and the styles it is `basedOn`. A style counts by its name ("heading 2", "Title") or its outline level, never by its ID, which Word translates.
>   - Lists come from the numbering definitions: `-` for bullets, the item's number for numbered lists, nested by level.
>   - Tables follow the bounds above.
>   - Links are kept. Footnotes and endnotes become `[^n]` with their text at the end. Tracked changes are accepted. Text boxes follow their paragraph. Pictures and charts become `[image: alt text]` and equations `[equation]`. Hidden text is left out.
>   - The text is normalized to NFC with the ZWNJ kept.
> - A Word file is untrusted input:
>   - It is never unpacked to disk.
>   - Reading stops at 2,000 parts, 50 MB unpacked, or a part that unpacks to more than 100 times its packed size (at least 1 MB).
>   - A password-protected, old (.doc), damaged or fake file gets an error that says which.

**3.7, the `read_ref` line could add:**

> - Keys and refs may also be given as the object's `jenab://` link to this project.

**4.2, after the MIME bullet:**

> - A zip is looked at once it is stored. If `[Content_Types].xml` gives its main part the Word document type, it is stored as a Word document (`application/vnd.openxmlformats-officedocument.wordprocessingml.document`, or `application/vnd.ms-word.document.macroEnabled.12` with macros). An OLE compound file is `application/x-ole-storage`: an old Office file, or one saved with a password.
> - The attach button stores a file under `uploads/<name>`, with `source` `upload` and the original name in `metadata.name`. The name keeps the key's characters, each run of others becomes `-`, and a name with none left is `file`. A taken key gets `-2`, `-3`, … before the extension; an upload never replaces a file. The user's message carries the file's `jenab://` link.

**4.7, the size-limits bullet becomes:**

> - Size limits: 100 MB per upload, 50 MB per `http.download` by default. Uploads go to `POST /upload/<project_id>` with the file as the body. The route needs the `X-Jenab-Upload-Name` header, which a form or another origin can't send, and refuses other sites.
> - A `.docx` opens with the system app only when it was detected as a Word document; a zip that only has the name doesn't.

**8.1, the rows become:**

> | `read_ref(ref, offset)`, `search_ref(ref, query)` | Read or search a stored tool output, page or Word document; `search_ref` returns matching lines with their offsets. A ref may be a key or a `jenab://` link |
>
> | `bucket_read(key)` | Text and Word documents as preview and ref; images to vision-capable models. The key may be a `jenab://` link |

## Real test files

These were saved with the local **Word 16 (Microsoft 365, x86, English UI)** through COM: VBScript under 32-bit cscript, because 64-bit PowerShell can't load Word's type library. Word was not running before; my instance quit itself every time.

- `internal/docx/testdata/word-en.docx`: title, headings, a link with a query string, a footnote and an endnote, a bullet list and a numbered list, a table with horizontal and vertical merges and a `|` in a cell, a tracked insertion and a tracked deletion, an equation, a picture with alt text, a text box.
- `word-de.docx`: German text, numbered list, table, footnote, tracked insertion.
- `word-fa.docx`: Persian RTL with ZWNJ and Persian digits, a heading, a bullet list by style, a table, a footnote.
- `word-password.docx`: saved with a password (`test-only`); the test expects ErrEncrypted.

Golden Markdown is next to each file; I reviewed all of it.

**Still missing** (not faked):
- Any file saved by **LibreOffice Writer** (not installed).
- Any file from **Google Docs** (no download without approval).
- A Word file saved with a **German or Persian UI**, which uses translated style IDs. This is covered only by built tests.
- A real Word file with a **nested list**: Word refused to set the list level from script (error 4608). Nesting is covered by built tests.

## Extraction time

- Word file, 497 KB, made by repeating word-en 400 times (6.0 MB document.xml): 147 ms, 267 KB of Markdown, 58 MB allocated in total.
- Generated document near the limit (45 MB XML, 10 MB zip): about 1.0 s, 33 MB of Markdown, 514 MB allocated in total (not peak).

## Checks

All with GOPROXY=off GOMEMLIMIT=1500MiB:
- `go build ./...`: ok
- `go vet ./...`: ok
- `go test -p 2 -short -count=1 ./...`: all ok
- gofmt on changed Go files, CR stripped: clean
- `go run ./cmd/jenab-scenarios check scenarios`: all ok

Frontend:
- `bun install --frozen-lockfile`
- `bun run typecheck`: ok
- `bun test`: 106 pass
- `bun run build`: ok (the usual chunk-size warning)

Hand check with the server-mode build (`-tags server`, 127.0.0.1:9310, LOCALAPPDATA, APPDATA and USERPROFILE in the scratchpad):
- Covered at 1280 and 760 px:
  - The attach button with a Persian-named Word file, a password-protected file and a CSV.
  - Chip states, the "can't be read" note and remove.
  - Send: the message carries the links.
- The route also refuses requests with no header (403), cross-site requests (403) and OPTIONS (405).
- No console errors.

## Decided differently or left open

- **Macros:** a .docm keeps its own macro type. It is read like Word, but *Open with the system app* refuses it.
- **Opening .docx:** a `.docx` that isn't Word (a plain zip) no longer opens with the system app; it used to go by extension.
- **Limits:** the ratio stop has a 1 MB floor; content types and .rels are capped at 4 MB; there is an 8 MB span budget per document.
- **Not detected:** .dotx templates.
- **Numbering:** numbered items show their real number; non-decimal formats are written as numbers.
- **Lists:** a list right after another list now starts after a blank line. The `lists` golden changed.
- **Bug fixed:** a style without `w:default` was taken as the default paragraph style, which made every unstyled paragraph a heading in documents with no default style.
- **Not built:** drag-and-drop and paste of files.
- **Cache title:** the text cache is per content. Its title names the key that first asked ("Text of uploads/x.docx"), so it can name another key, or one since deleted.
- **Follow-up (not done):** `jenab://` links in the user's own message bubble show as plain text. They are not clickable file links there.
- **Tool descriptions changed:** `read_ref`, `search_ref` and `bucket_read` now mention documents and links. The scenario check passes.
