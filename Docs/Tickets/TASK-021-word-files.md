# TASK-021 — Read Word files: text from .docx
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** P1-05, P1-09

## Goal
The user puts a Word file in the project, and the agent can read it and search it like a fetched page. This is the first step towards finding passages by meaning (SPIKE-033). PDF comes later, as its own step.

## Today
- **A file can't get in.** `ChatService.Send` takes text only (`internal/app/chat.go:152`), the composer has no attach button, and the Files panel has no upload (TASK-015 leaves it out). SPEC 4.2 has the source `upload` and 4.7 a 100 MB limit, but nothing uses them.
- **A Word file is a zip.** `DetectMIME` (`internal/bucket/mime.go`) uses `http.DetectContentType`, which calls a .docx `application/zip`.
- **It can't be read.** `bucket_read` answers "only text and images can be read" (`internal/tool/bucket.go:299`), and `read_ref` and `search_ref` refuse anything that isn't text.

## Scope
First write it into SPEC 3.7, 4.2, 4.7 and 8.1, then build it.

1. **Getting a file in.** The composer gets an attach button that takes any file up to 100 MB (4.7). The file is stored with `source: upload` under `uploads/<name>`, with the name cut to the key rules (4.1) and a number added if the key is taken. The user's message carries its `jenab://` link, so the agent sees which file it is.
2. **Detection.** A zip whose `[Content_Types].xml` names a WordprocessingML main part (`…wordprocessingml.document.main+xml`, or the macro-enabled form) is stored as `application/vnd.openxmlformats-officedocument.wordprocessingml.document`. Macros are never run; only the text is read.
3. **Extraction** in pure Go, with `archive/zip` and `encoding/xml`, into Markdown:
   - **Headings** from the paragraph's outline level, or its style's `w:name` (`heading 1` to `heading 6`, `Title`), following `basedOn`. Never from the style ID: a German Word names the style `berschrift1`.
   - **Lists** from `numbering.xml`: bullets as `-`, numbers as `1.`, nested by level.
   - **Tables** as Markdown tables, with SPIKE-015's bounds for spans; a table over the bounds becomes paragraphs. Nested tables are flattened.
   - **Links** from the document's relationships, as `[text](url)`.
   - **Footnotes and endnotes** as `[^n]`, listed at the end.
   - **Tracked changes** as if accepted: insertions kept, deletions left out. Comments, headers and footers are left out.
   - **Text boxes** in place. **Images** as `[image: alt text]`, or `[image]` without one. **Equations** as `[equation]`.
   - **Text** normalized to NFC, with the ZWNJ kept. Word stores right-to-left text in logical order, so Persian needs nothing more.
4. **Untrusted input.** The file is never unpacked to disk. At most 2,000 zip entries and 50 MB unpacked for the parts read, and a part that unpacks to more than 100 × its packed size stops the extraction. A broken or encrypted file gets a clear error: "could not read report.docx: it is password-protected".
5. **Storing the text.** The Markdown is kept in `cache/text/<sha256>-v<n>.md`, made at upload and again on demand if the cache expired. `<n>` is the extractor's version, so a better extractor makes the text again.
   - The agent never sees this key. The ref is the file's own key: `bucket_read`, `read_ref` and `search_ref` on `uploads/contract.docx` read its text, with offsets in bytes of the text.
   - The header names the type: `[object uploads/contract.docx — Word document — 8,000 tokens, showing first 1,500 — ref: uploads/contract.docx]`.
   - The text is untrusted, like a web page (3.7).
6. **Tests:** real files saved by Word, LibreOffice and Google Docs, in English, German and Persian, with headings, lists, tables with merged cells, footnotes and tracked changes, each checked against its Markdown in `testdata/`. Also a zip bomb, an encrypted file, a broken zip, a zip that only claims to be Word, and a fuzz test of the extractor.

## Not in scope
- **PDF.** Its text often comes out of Persian files in visual order or as Arabic presentation forms, and scanned PDFs need OCR. That needs measuring first, in its own ticket.
- Older `.doc` files, `.odt`, `.rtf` and `.pptx`.
- Search across files and embeddings (SPIKE-033 decides them, then a ticket builds them).
- Showing the Markdown in the Files panel. TASK-015 can use it once this is in.
- A pipeline step for extraction.

## Done when
A Word file attached in the composer is in the bucket as a Word document, and the agent reads it with `bucket_read`, continues with `read_ref` and finds a line with `search_ref`. This holds for Persian and German files from Word, LibreOffice and Google Docs. The bad files above fail with a clear error, and none of them takes more than a second or 100 MB of memory.

## Result
