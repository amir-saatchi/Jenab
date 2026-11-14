# TASK-049 — Search the project's files by words
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
- **What reads as text** is decided by `isText` (`internal/tool/bucket.go:44`): `text/*`, JSON, XML, and `+json` and `+xml` types.
- **Word files have text.** TASK-021 keeps a Word file's Markdown in `cache/text/<sha256>-v<n>.md`. `read_ref` on the file's own key reads it, with offsets in bytes of that text.
- **Subagent transcripts are in the bucket,** as indented JSON under `subagents/<chat>/<id>.json` (`internal/agent/subagent.go:362`).
- **The untrusted mark comes from the tool's effects.** A call to a tool with the `Untrusted` effect sets it, whatever the result (`internal/agent/turn.go:734`, TASK-012).
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
2. **Search.** FTS only, over `chunks_fts`, with the rules of 2.3: the same tokenizer, normalization and query builder as `messages_fts`, and snippets built in Go from the stored text. Use the store's builder, not the spike's copy.
   - **One hit per chunk,** with no cap per file, as the spike measured it. To stay in one file, the agent narrows with `prefix` or uses `search_ref`.
   - **Ranking:** bm25 orders the newest 2,000 matching chunks, as 2.3 does for messages. "Newest" is by the chunk's rowid, so the most recently indexed chunks come first.
3. **What is indexed.** The newest version of:
   - text files: whatever `isText` reads as text. That covers Markdown, CSV, JSON, XML, YAML and code, so there is no separate list of code types;
   - documents turned into text (TASK-021): today, Word files, through their cached Markdown;
   - subagent transcripts (8.6).
   - Offsets are in bytes of that text (3.7), so for a Word file they point into its Markdown, as `read_ref` already reads it.
   - **Up to 10 MB of text a file.** A longer file has its first 10 MB indexed, cut at a line end. Hits say nothing about it. `search_ref` on the file searches further (20 MB).
   - Nothing under `cache/` is indexed. Web pages, tool outputs and workspace code come later (R-126).
4. **Chunks,** from the spike's `headingChunker`:
   - **Each chunk is led** by the file name and heading path, e.g. `contract.docx › 4 Termination › 4.2 Notice`. The file name is the key's last segment: every file has one, and it is what the agent sees.
   - **Markdown and Word files:** each section under a heading is one chunk. A long section is split at paragraph ends. A table is split by rows, with its header row repeated.
   - **Files without headings** (plain text, code, JSON, XML, YAML, and the JSON transcripts): the whole file is one section, led by the file name only. It is split at blank lines, then at line ends, then at spaces for a line too long.
   - **CSV** is a table: split by records, with the first record repeated as the header. Records are found with `encoding/csv`, so a quoted field with a line break stays in one record.
   - **A chunk has at most 1,200 bytes,** including the file name and heading path that lead it. It is never cut inside a character.
   - Chunks are keyed by the text's sha256, as in 2.9.
5. **Where it lives.** `index.db`, one per project, next to `project.db` and `chats.db` (2.9). For now it holds only two tables:
   - `chunks`: key, version, the text's sha256, byte offset, length and heading path;
   - `chunks_fts`: the FTS5 index over the chunks.
   - It can be rebuilt from the bucket. So it stays out of the change log, undo and exports, and a failed check when the project opens rebuilds it (2.1, 2.7, 2.9). It gets its own writer and reader pool, like the other two (2.7, 7).
   - No project exists yet, so nothing needs migrating. A new project starts with an empty `index.db`, and a missing one is rebuilt like a damaged one.
   - While it is rebuilt, `bucket_search` doesn't wait. It says the file index is being rebuilt and suggests `search_ref` on a known file.
6. **The index holds only the current version of each key.**
   - **Every write follows it.** One hook in the store runs after each commit that changes `_jenab_objects`, so no path is missed:
     - `put`: the key's old chunks go and the new version is chunked.
     - `delete`: the key's chunks go.
     - `move`: the chunks go under the old key and the file is chunked again under the new one, since the file name in the lead changes.
     - Undo, and anything else that makes a version current again: that version is chunked again.
   - **"At once":** the hook runs in the same call, right after the write's transaction commits and before the call returns, in its own transaction in `index.db`. So the next `bucket_search` finds the new text.
   - **After a crash** between the two commits, a check when the project opens compares each key's current version in `project.db` with `chunks`. It chunks what is missing and drops what is stale.
   - **The sweep and *Free now* have nothing to free** in `index.db`, since it holds no old versions.
   - This differs from SPEC 4.4 and 4.9, where an old version keeps its chunks until the sweep frees it. That rule saves embedding again, and comes with vectors (R-124). Without vectors, chunking again is cheap.
7. **Untrusted.** File text is untrusted, like a web page (3.7). `bucket_search` has the effects of `read_ref`: it reads, and it is untrusted. So a call sets the mark for the turn, with hits or without, like every other untrusted tool (8.8, TASK-012).
8. **Tests:**
   - **Chunker:** port the spike's `headingChunker` and `tableParts` with the 1,200-byte budget, and test them. No chunk is over 1,200 bytes with its lead. A long section splits at a paragraph end. A long table splits by rows, each part with the header row. A file without headings, a CSV with a quoted line break, and a transcript are chunked as above. Offsets point at the chunk's text, so `read_ref(key, offset)` starts there. Persian and German text is never cut inside a character.
   - **Corpus:** the spike's file corpus, `spikes/033-semantic-recall/corpus/docs/` (12 made-up documents, four each in English, German and Persian) and its 30 queries in `queries-files.yaml`. In the spike, FTS over chunks by heading found 7 of 8 same-words targets in the top 5, and nothing for any of the 7 no-answer queries. The app's search must do at least as well on the same queries. Record the numbers at 1,200 bytes in the result, since the spike's were at 1,600.
   - **Writes:** after a `put`, only the new version's text is found. After an undo, the old text is found again. A deleted key isn't found. A moved file is found under its new key only, with its new name in the lead. A crash between the write and the index is repaired when the project opens.
   - **What is indexed:** a Word file is found through its own key, with offsets into its Markdown. A subagent transcript is found, and `read_ref` reads it at the hit's offset. A file under `cache/` isn't found. Text past 10 MB isn't found. `prefix` and `limit` work, and a limit over 50 is cut to 50.
   - **`index.db`:** a damaged or missing one is rebuilt, and `bucket_search` answers with the rebuild line meanwhile. An export leaves it out. Undo and the change log never touch it.
   - **Untrusted:** after a `bucket_search` call, the turn carries the mark, as in TASK-012's tests.

## Not in scope
- Everything about embeddings: vectors, `chunk_vectors`, `embedding_model`, hybrid search, *Embed files too*, `meaning` and `both` in `matched`, and the "closest by meaning" line (R-124 to R-127). Keeping old versions' chunks until the sweep comes with them.
- Web pages, tool outputs, workspace code and other sources (R-126).
- PDF and other document types (TASK-021 left them out).
- Testing chunking again with context lines or chunks made by a model (SPIKE-033, *Later*).
- A file search in the UI. This ticket is the agent's tool only.

## Decisions
Made on 2026-11-15, for the points the SPEC and SPIKE-033 left open.

- **By the user:**
  - No project exists yet, so there is no first index to build for old projects.
  - The index holds no old versions.
- **Recommended in the ticket session at the user's request, and accepted by the user:**
  - The index follows every write through one hook, right after the commit; a check at open repairs a crash.
  - `move` chunks again; the sweep and *Free now* free nothing in `index.db`.
  - The first 10 MB of a longer file is indexed.
  - Files without headings split at blank lines; CSV is a table; transcripts are chunked as JSON text.
  - Text types are what `isText` reads, with no list of code types.
  - The lead's file name is the key's last segment.
  - One hit per chunk with no cap per file; "newest" is by rowid.
  - A call sets the untrusted mark, with hits or without.

## Done when
- The SPEC describes the FTS-only `bucket_search` as v1, with the rules above.
- The agent finds a passage in a Markdown, text or Word file by words, and continues with `read_ref(key, offset)`, in English, German and Persian.
- The tests above pass, with the corpus numbers in the result.

## Result
