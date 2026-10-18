# SPIKE-033 — Semantic recall: find earlier messages and file passages by meaning
**Type:** Spike
**Status:** Open
**Gate:** 3
**Needs:** P1-07

## Question
After a cut, the model reaches older turns only through `search_history` (3.6). That search is lexical: FTS5 with bm25 and the Go normalization for Persian and German (2.3, SPIKE-011). It finds a message only if the query shares words with it. Asked "which currency did I want?", it misses "show prices in euros". An English query never finds a Persian message.

Files have the same problem, and worse. Once TASK-021 turns a Word file into text, the agent can read it with `read_ref` and search one file at a time with `search_ref`, which matches lines by words. Nothing searches across the project's files, and nothing finds a passage by meaning.

Does hybrid search find the right earlier message, or the right passage of a file, more often than word search alone? Hybrid means FTS5 bm25 plus embedding similarity, merged with reciprocal rank fusion. How should a file be cut into chunks? What does it cost in time and storage?

The design to test (proposed on 2026-10-18; widened to files the same day):
- **An embedding provider set up by the user, if they want one.** Nothing is set up when the app ships (3.9). Anthropic has no embeddings API, so with no provider set up, search stays FTS only.
- **Or a built-in embedder,** for users without Ollama (about 1.5 GB) or a cloud key. It is a small multilingual model that runs inside the app on the CPU, through hugot's pure-Go backend (no cgo, like the SQLite driver). The model isn't in the installer. The user turns it on in *Settings → Models*, sees its size, and the app downloads it from a fixed URL and checks a pinned sha256. It stays in the data folder, runs at background priority and is unloaded when idle. Files never leave the machine, so it needs no consent step. It is a component of the app rather than a provider, but it is still a built-in model; the user decides whether that fits the rule that providers are always set up by the user.
- **Vectors in their own file, `index.db`,** next to `chats.db` and `project.db`: one BLOB per message text part and one per file chunk. It holds only data that can be rebuilt, so it stays out of the change log and undo (2.5, 2.6), is left out of exports (P2-12), and is deleted and rebuilt when the user changes the embedding model. The driver is modernc.org/sqlite (pure Go, SPIKE-001), which can't load sqlite-vec, so there is no vector index.
- **File chunks keyed by the text's sha256,** so the same bytes are embedded once, and an old version of a file keeps its chunks until the sweep frees them (4.4).
- **A brute-force cosine scan in Go** over the vectors in scope.
- **Embedding in the background,** at background priority and within `max_parallel_calls` (7.6). A message or file is found by FTS at once, and by meaning once its vectors are written.
- **Files leave the machine only if the user agrees.** Chat messages were already sent to the chat model, but a file put in the bucket was not. With a local provider (Ollama or the built-in embedder), files are embedded without asking. With a cloud provider, files are embedded only after the user turns on *Embed files too* for that provider. Messages are embedded either way.
- **For history, no new tool.** `search_history` keeps its parameters and becomes hybrid when a provider is set up.
- **For files, one new tool:** `bucket_search(query, prefix, limit)` returns passages with their key, byte offset and a snippet, so `read_ref(key, offset)` continues from there. It is FTS only without a provider, and hybrid with one. The prompts stay the same for every model (SPEC 1).

## Method
1. **Corpus:**
   - **Messages:** about 300 synthetic messages in 6 chats, in English, German and Persian, with code, numbers and URLs mixed in. Each target message has distractors on a near topic (prices in dollars next to "show prices in euros").
   - **Files:** about 12 synthetic documents, 2 to 40 pages, four per language, written as the Markdown TASK-021 produces from a Word file: headings, lists, tables and footnotes. The spike doesn't wait for TASK-021. Two documents are on near topics (a rental contract and a work contract), so a passage has distractors in another file.
2. **Labelled queries:** about 60 for messages and 30 for files, split evenly by language, each with its target messages or target passages. A passage is a byte range in a file; a chunk that overlaps it counts as a hit. They are written the way an agent writes a `search_history` or `bucket_search` query, in four kinds:
   - **Same words:** the query shares words with the target. FTS should already find these, so hybrid must not lose them.
   - **Paraphrase:** no word in common ("which currency did I want?" → "show prices in euros"; "when can I leave the flat?" → "notice period of three months").
   - **Cross-language:** the query in one language, the target in another (English → Persian, German → English).
   - **No answer:** nothing in the corpus matches. FTS returns nothing; a vector search always returns something.
3. **Chunking** (files only), two ways, both at most 400 tokens:
   - **Fixed:** 400-token windows with 50 tokens of overlap, cut at the nearest paragraph or sentence end.
   - **By heading:** one chunk per section, a long section split at paragraph ends, each chunk led by its file name and heading path (`contract.docx › 4 Termination › 4.2 Notice`). A table stays in one chunk if it fits, otherwise it is split by rows with its header row repeated.
4. **Conditions**, each run over every query:
   - **FTS:** for messages, today's `search_history`, the app's own `ChatsDB.Search` on a `chats.db` built from the corpus. For files, an FTS5 table over the chunks with the same normalization and query builder (2.3), as a lexical `bucket_search` would use.
   - **Vector:** cosine similarity only.
   - **Hybrid:** the top 50 of each list merged with reciprocal rank fusion, score = Σ 1 / (60 + rank). An item in only one list keeps its one term.
5. **Widths:** the vector and hybrid runs at the model's full width, at 256 (Matryoshka truncation, then renormalized, only for models trained for it) and int8-quantized, so the quality cost of each smaller form is known next to its speed.
6. **Text sent to the model:** the stored text as it is, the same for every model. One extra run per model adds the query and document prefixes the model's card asks for (for example `search_query:` and `search_document:` for nomic-embed-text), to know what leaving them out costs. If they matter, the decision says whether the provider layer adds them.
7. **Speed:**
   - Embedding time per 1,000 messages and per 100 pages, one model at a time, in batches as the endpoint allows.
   - The time to embed one query, since every hybrid search waits for it.
   - For the built-in embedder: the app's memory while it embeds, the time to load the model, and whether a chat still streams smoothly (N-01) while it embeds 100 pages at background priority. The machine is named in the result, since CPU speed decides these.
   - The Go scan over 100,000 and 250,000 vectors at full width, at 256 and as int8. 250,000 is about 1,500 long documents next to a long chat history. Scan time doesn't depend on the text, so these vectors may be random unit vectors at the right width.

## Models
To confirm with the user before any run, one of each:
- **Local, through Ollama's native API:** for example `embeddinggemma` (768 wide, Matryoshka down to 128) or `nomic-embed-text` (768, Matryoshka).
- **Cloud, through an OpenAI-compatible `/embeddings` endpoint:** for example Gemini's `gemini-embedding-001` (3,072 wide, Matryoshka) with `GEMINI_API_KEY`, on the OpenAI-compatible base URL set by hand. The kind is chosen, not guessed from the URL.
- **Built-in, through hugot's Go backend** (`NewGoSession`), with an ONNX model of about 100–300 MB. It must handle Persian, so English-only models such as `all-MiniLM-L6-v2` are out. Candidates: `multilingual-e5-small` (384 wide), `granite-embedding-107m-multilingual` (384) or `embeddinggemma-300m` (768, Matryoshka). Before the run, the spike checks for each candidate:
  - **License:** whether the app may download and run it for every user. Gemma models come with Google's own terms.
  - **It loads:** whether hugot's Go backend runs it, tokenizer included.
  - **Size:** the download in MB, as fp32 and int8 if both are published.

  hugot's docs say the Go backend is for small models and batches of about 32 inputs. If it is too slow, one extra run uses hugot's ONNX Runtime backend (cgo, plus a native library per platform) on the same model, so the cost of staying pure Go is known.

Failures of a free tier (rate limits, quota) are recorded and that model stops; there are no workarounds for them.

## Setup
- **Harness:** `spikes/033-semantic-recall/`. It builds a temporary `chats.db` with the app's store, so the FTS condition for messages is the real one. The file chunker and the chunk FTS table are spike code that reuses the store's normalization. The embedding client is spike code that calls the two endpoints and runs the built-in model through hugot; the app's provider layer has no embeddings call yet. A fake embedder tests the harness (`go test`).
- **Data:** the messages, the documents and the labels in `corpus/`, as YAML and Markdown, so they can be read and fixed by hand. Vectors are cached per model, width and chunking in `results/vectors.db`, so a rerun doesn't embed again.
- **Output:** per query and condition, the ranks of the targets and the top 5 with their scores, in `results/`. A summary table in `results/summary.md`.
- **Safety:** keys come from `.env` and stay in memory. Only synthetic text is sent. A quota error stops that model. The built-in model is downloaded once into `results/models/` (not committed), with its sha256 recorded in the result.

## Measures
- **Recall@5:** queries with a target in the top 5, per condition, width, model, language and query kind; for files also per chunking.
- **MRR:** the mean of 1 / rank of the first target (0 when none is in the top 50).
- **Same-words queries:** any query FTS finds that hybrid ranks lower or loses.
- **No-answer queries:** the top similarity score, against the scores of real hits, to see whether a cutoff could keep vector noise out.
- **Embedding time** per 1,000 messages, per 100 pages, and per query.
- **Storage:** bytes per vector at each width and as int8, per message and per page, next to FTS's 0.4 × the text size (SPIKE-011).
- **Scan time** over 100,000 and 250,000 vectors, p50 and p95, at full width, at 256 and as int8.
- **Built-in embedder:** its recall next to the other two models, per language, and its CPU time, memory and load time.

## Round 2: chunks with help from a model
Only if round 1 shows that chunking matters: the two chunkings differ by 10 points or more in file Recall@5 on some model. Otherwise round 2 is skipped, and the result says so.

The width and the chunk size stay as round 1 chose them, so only the chunks change. Two more conditions, on the same documents and file queries:
- **LLM chunks:** the chat model reads a whole document and returns where to cut it, as JSON with byte offsets, through the structured output from SPIKE-030. The cuts are checked: each must fall on a paragraph end, and no chunk may pass 400 tokens. A bad answer is recorded, and that document falls back to chunks by heading.
- **Context line:** the chunks by heading, each led by one sentence the chat model writes for it, saying where the chunk sits in the document ("section 4.2 of the rental contract, on the notice period"). The line is embedded and indexed with the chunk. The whole document is sent once per document, with prompt caching where the provider has it.

The same prompt for every model (SPEC 1), run on the reference chat model (one cloud model; a second only if the first fails). Besides recall, round 2 measures:
- **Cost:** tokens and time per 100 pages, and how much of the input caching saved.
- **Stability:** the same document chunked twice by the same model, and how many cuts or lines change.
- **Privacy:** both send the whole file to the chat provider, so the decision covers how the user agrees to that, as for cloud embedding.

## Done when
Every round 1 condition has run on the three models at each width, for messages and for files with both chunkings, with the extra prefix run. Round 2 has run, or the result shows why it wasn't needed. There is a decision:
- whether hybrid search is worth building, for messages, for files or both, and at which width and form (float or int8)
- whether the built-in embedder ships as an option, which model it uses, and whether the pure-Go backend is fast enough
- whether a model may help with chunking (round 2), and if so, as an option the user turns on
- whether `search_history` turns hybrid by itself when a provider is set up, or needs an option
- how files are chunked, and the shape of `bucket_search`
- whether a score cutoff is needed, and its value
- whether query and document prefixes belong in the provider layer
- whether the vectors go in `index.db` or stay in `chats.db` and `project.db`, and the consent rule for files with a cloud provider
- what goes in SPEC 2.3, 3.8 and 4: the vector tables, the chunk table, the background embedding, and what happens when the user changes the embedding model (all vectors embedded again)

## Result
