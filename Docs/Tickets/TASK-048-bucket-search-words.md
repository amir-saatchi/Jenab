# TASK-048 — Search the project's files by words
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** TASK-021
**Requirements:** R-128 (Gate 1)

## Goal
The agent can find a passage in any of the project's files without knowing which file holds it. `bucket_search` searches every indexed file by words and returns passages, so `read_ref(key, offset)` continues there. It needs no setup.

This is the FTS-only `bucket_search` that SPEC v0.7 puts first (4.9). Search by meaning comes later with semantic recall (R-124 to R-127), on top of the same chunks.

## Today
- **Search works one file at a time.** `read_ref` and `search_ref` (`internal/tool/bucket.go:151`, `:182`) need the file's ref. Nothing searches across files.
- **There is no `index.db`.** SPEC 2.9 describes it, but marks all of it Later. The code has no chunk table and no `bucket_search`.
- **The FTS rules exist for messages.** `store.Normalize` and the unexported `queryTerms` and `ftsQuery` (`internal/store/search.go`) build `messages_fts` and its queries (SPEC 2.3).
- **Word files have text.** TASK-021 keeps a Word file's Markdown in `cache/text/<sha256>-v<n>.md`. `read_ref` on the file's own key reads it, with offsets in bytes of that text.
- **Subagent transcripts are in the bucket,** as JSON under `subagents/<chat>/` (`internal/agent/subagent.go:362`).
- **The sweep** is `ProjectDB.SweepObjects` (`internal/store/objects.go:608`). It knows nothing about chunks.
- **The spike has a starting point.** `spikes/033-semantic-recall/` has a heading chunker (`chunk.go`, `headingChunker` and `tableParts`) and a chunk FTS table (`fts.go`, `chunkFTS`). It is throwaway code:
  - Its budget is 1,600 bytes (400 tokens at 4 bytes a token). The SPEC's is 1,200 bytes (3 bytes a token).
  - Its query builder is a copy of the store's, without the chat column.

## Scope
First write it into SPEC 2.9, 4.4, 4.9 and 8.1, then build it. Today those sections call all of this Later. Only the parts below move into v1. The vector tables, the embedding model and everything about meaning stay Later.

1. **The tool.** `bucket_search(query, prefix, limit)`, in the Files group (8.6).
   - `prefix` keeps the search under a key prefix.
   - `limit` is 10 by default and at most 50.
   - It returns passages. Each hit has the key, a byte offset for `read_ref(key, offset)`, the heading path, a snippet of about 300 characters and `matched`.
   - Without an embedding model, every hit matched by words, so `matched` is `words`.
   - The tool's description says what it does today. The same description and result text go to every model (SPEC 1, TASK-011).
2. **Search.** FTS only, over `chunks_fts`, with the rules of 2.3: the same tokenizer, normalization and query builder as `messages_fts`, bm25 ranking, and snippets built in Go from the stored text. Use the store's builder, not the spike's copy.
3. **What is indexed.** The newest version of:
   - text files: `text/*`, Markdown, CSV, JSON, XML, YAML and code;
   - documents turned into text (TASK-021): today, Word files, through their cached Markdown;
   - subagent transcripts (8.6).
   - Up to 10 MB of text a file. Offsets are in bytes of that text (3.7), so for a Word file they point into its Markdown, as `read_ref` already reads it.
   - Nothing under `cache/` is indexed. Web pages, tool outputs and workspace code come later (R-126).
4. **Chunks by heading,** from the spike's `headingChunker`:
   - Each section is one chunk, led by its file name and heading path, e.g. `contract.docx › 4 Termination › 4.2 Notice`.
   - A long section is split at paragraph ends. A table is split by rows, with its header row repeated.
   - A chunk has at most 1,200 bytes, including the file name and heading path that lead it.
   - Chunks are keyed by the text's sha256, so the same text is chunked once.
5. **Where it lives.** `index.db`, one per project, next to `project.db` and `chats.db` (2.9). For now it holds only two tables:
   - `chunks`: key, version, the text's sha256, byte offset, length and heading path;
   - `chunks_fts`: the FTS5 index over the chunks.
   - It can be rebuilt from the bucket. So it stays out of the change log, undo and exports, and a failed check when the project opens rebuilds it (2.1, 2.7, 2.9). It gets its own writer and reader pool, like the other two (2.7, 7).
   - While it is rebuilt, `bucket_search` doesn't wait. It says the file index is being rebuilt and suggests `search_ref` on a known file.
6. **New versions and the sweep.**
   - A new version is found by FTS at once.
   - An old version keeps its chunks until the sweep frees that version (4.4). A version that becomes current again, for example after an undo, needs no new chunking while its chunks are kept.
7. **Untrusted.** File text is untrusted, like a web page (3.7). A `bucket_search` result keeps that mark for the turn, as reading the file does (8.8, TASK-012). The tool's effects are those of `read_ref`: it reads, and it is untrusted.
8. **Tests:**
   - **Chunker:** port the spike's `headingChunker` and `tableParts` with the 1,200-byte budget, and test them. No chunk is over 1,200 bytes with its lead. A long section splits at a paragraph end. A long table splits by rows, each part with the header row. Offsets point at the chunk's text, so `read_ref(key, offset)` starts there. Persian and German text is never cut inside a character.
   - **Corpus:** the spike's file corpus, `spikes/033-semantic-recall/corpus/docs/` (12 made-up documents, four each in English, German and Persian) and its 30 queries in `queries-files.yaml`. In the spike, FTS over chunks by heading found 7 of 8 same-words targets in the top 5, and nothing for any of the 7 no-answer queries. The app's search must do at least as well on the same queries. Record the numbers at 1,200 bytes in the result, since the spike's were at 1,600.
   - **Versions:** a new version replaces the old one's hits at once. An undo brings the old hits back without chunking again. A deleted key isn't found. The sweep frees the chunks of a version it frees.
   - **What is indexed:** a Word file is found through its own key, with offsets into its Markdown. A file under `cache/` isn't found. `prefix` and `limit` work, and a limit over 50 is cut to 50.
   - **`index.db`:** a damaged one is rebuilt, and `bucket_search` answers with the rebuild line meanwhile. An export leaves it out. Undo and the change log never touch it.
   - **Untrusted:** after a `bucket_search` result, the turn carries the mark, as in TASK-012's tests.

## Not in scope
- Everything about embeddings: vectors, `chunk_vectors`, `embedding_model`, hybrid search, *Embed files too*, `meaning` and `both` in `matched`, and the "closest by meaning" line (R-124 to R-127).
- Web pages, tool outputs, workspace code and other sources (R-126).
- PDF and other document types (TASK-021 left them out).
- Testing chunking again with context lines or chunks made by a model (SPIKE-033, *Later*).
- A file search in the UI. This ticket is the agent's tool only.

## Open questions
The SPEC and SPIKE-033 don't say these. Each needs a decision, written into the SPEC, before or while it is built.

1. **Index for existing projects.** A project made before this has no `index.db`. Is the first build the same as a rebuild (2.9), with the same line from `bucket_search` while it runs?
2. **"At once."** Is a new version chunked inside the `put`, or right after it? The two live in different files (`project.db`, `index.db`), so they can't share a transaction. What happens after a crash between them: a check when the project opens, like the one for message vectors (2.9), or a rebuild?
3. **Old versions in `chunks_fts`.** Only the newest version is searched. Do an old version's chunks stay in `chunks_fts` and get filtered out at query time, or leave `chunks_fts` and stay only in `chunks` until the sweep?
4. **Delete and move.** After `delete`, the key has no current version, but its bytes are kept for undo. Are its chunks kept like an old version's? `move` renames a key with its versions (4.3). Are the chunk rows renamed with it, or chunked again?
5. **Free now.** The sweep frees chunks with their version. Does *Free now* (4.4) free them too, at once?
6. **Over 10 MB.** Is the first 10 MB of the text indexed, or is the file skipped? Does the result say so?
7. **Files without headings.** Plain text, JSON, YAML, XML and code have no Markdown headings. Is each file one section, split at paragraph ends? Is a CSV a table, split by rows with its header row repeated?
8. **Subagent transcripts.** They are JSON. Are they chunked as JSON text, or turned into text first (one section per message, say)? What is their file name in the chunk's lead?
9. **"Code."** Which MIME types count as code? Today the key's extension only turns plain text into another text type (4.2).
10. **The file name in the lead.** Is it the key's last segment, or the original name in `metadata.name` (4.2)?
11. **Hits per file.** 2.3 gives one hit per message. Is it one hit per chunk here, so one file can fill the result, or at most some number per file? 2.3's "newest 2,000 matches" is by message; what is "newest" for chunks?
12. **The untrusted mark with no hits.** Does a result with no hits still set the mark?

## Done when
- The SPEC describes the FTS-only `bucket_search` as v1, and the open questions above are answered in it.
- The agent finds a passage in a Markdown, text or Word file by words, and continues with `read_ref(key, offset)`, in English, German and Persian.
- The tests above pass, with the corpus numbers in the result.

## Result
