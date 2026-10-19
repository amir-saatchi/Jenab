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
One of each. The local model and the built-in candidates were chosen on 2026-10-18; the cloud model is confirmed with the user before any run.
- **Local, through Ollama's native API:** `embeddinggemma-2:270m`, the text-only tag of EmbeddingGemma 2 (304–378 MB). It is 768 wide, with Matryoshka widths of 512, 256 and 128, and covers over 100 languages; its page doesn't name Persian, so the Persian queries decide. It takes task prefixes for search, which the page doesn't spell out; the prefix run (Method 6) uses the ones from its model card.
- **Cloud, through an OpenAI-compatible `/embeddings` endpoint:** for example Gemini's `gemini-embedding-001` (3,072 wide, Matryoshka) with `GEMINI_API_KEY`, on the OpenAI-compatible base URL set by hand. The kind is chosen, not guessed from the URL.
- **Built-in, through hugot's Go backend** (`NewGoSession`). It must handle Persian, so English-only models such as `all-MiniLM-L6-v2` are out. The candidates, in order:
  1. **`embeddinggemma-2:270m`**, the same model as the Ollama run, if all three of these hold:
     - **ONNX:** an ONNX export of the 270m text model is published. In October 2026 there was one for the first EmbeddingGemma (`onnx-community/embeddinggemma-300m-ONNX`) but not for this one.
     - **It runs on hugot's Go backend,** tokenizer included. It is a Gemma 4 model, with sliding-window attention and a 262,144-token vocabulary, while hugot's docs aim the Go backend at small models such as MiniLM.
     - **No gate:** the app can download it for the user without an account or accepting terms on the site, and Google's Gemma terms allow the app to download and run it for every user. Ollama hosts its own copy, so this doesn't limit the Ollama run.

     With this model in both runs, the Ollama and hugot vectors for the same text are compared too: their cosine should be close to 1, which shows the hugot path is right.
  2. **Otherwise `multilingual-e5-small`** (384 wide) **or `granite-embedding-107m-multilingual`** (384), both BERT-style, which hugot's Go backend is made for. The spike checks each one's license, that it loads with its tokenizer, and its download size as fp32 and int8. The one with the better Persian recall on a first small run is used. Neither is trained for Matryoshka, so they run at full width and int8 only (Method 5).

  hugot's docs say the Go backend is for small models and batches of about 32 inputs. If it is too slow, one extra run uses hugot's ONNX Runtime backend (cgo, plus a native library per platform) on the same model, so the cost of staying pure Go is known. The result names which candidate ran and why the ones before it didn't.

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
### Round 1 (draft)
Runs of 2026-10-19. Tables are in `spikes/033-semantic-recall/results/summary.md`, and ranks per query are in `results/queries.jsonl`. Built-in embedder details: `spikes/033-semantic-recall/builtin/results.md`. Scan times: `spikes/033-semantic-recall/scan/results.md`.

**Models:**
- **Local: `embeddinggemma-2:270m` through Ollama** (0.40.6, Q8_0, 768 wide). Ollama 0.21.0 couldn't pull it, so the user installed 0.40.6. Port 11434 was free this time. The prefixes are the "query" and "document" prompts in the model's own `config_sentence_transformers.json`.
- **Built-in: granite-embedding-107m-multilingual,** fp32, on hugot's pure-Go backend, run as it would ship. EmbeddingGemma 2 was dropped for the Go backend (about 32 minutes per 100 pages, and out of memory on 16 GB). multilingual-e5-small lost to granite on Persian (`builtin/results.md`). granite has no Matryoshka and no prefixes, so it has no 256 run and no prefix run.
- **granite with the reference tokenizer** (`granite-reftok`): an extra condition. It is the same model, tokenized by Python `tokenizers` and run in ONNX Runtime (`reftok.py`). The Go tokenizer is wrong for this model (every Persian ZWNJ becomes `<unk>`), and this condition shows what that costs. Its times are ONNX Runtime's.
- **Cloud: gemini-embedding-001 stopped on the free tier.** The first two batches of 100 messages went through. The third got `429`: `embed_content_free_tier_requests, limit: 100`. That is 100 a minute, and it counts each text as a request. It stopped there, as the ticket asks. It has 200 of 300 message vectors and no queries, so it has no recall numbers. At that limit, one pass over the 1,469 texts would take at least 15 minutes.

**Recall@5 and MRR, hybrid, full width** (no-answer queries left out; 45 message queries, 23 file queries):

| Set | FTS | granite | granite-reftok | Gemma | Gemma + prefixes |
|---|---|---|---|---|---|
| messages | 33%, 0.32 | 82%, 0.69 | 84%, 0.72 | 80%, 0.70 | 89%, 0.80 |
| files, fixed chunks | 26%, 0.26 | 65%, 0.56 | 70%, 0.61 | 87%, 0.73 | 91%, 0.87 |
| files, chunks by heading | 30%, 0.30 | 74%, 0.67 | 74%, 0.70 | 83%, 0.72 | 100%, 0.81 |

- **FTS finds only same-words queries:** all of them for messages, 75% (fixed) and 88% (heading) for files. It finds no paraphrase and no cross-language query.
- **Hybrid has the recall of vector search and a higher MRR.** Vector-only Recall@5 is the same in every cell above. Hybrid keeps every message FTS finds. One file query drops from rank 1 to rank 2 (qf003, chunks by heading, a table row) on both models. No same-words query is lost.
- **Persian is the weakest language.** On messages: Gemma with prefixes en 93%, de 100%, fa 73%; granite en 87%, de 87%, fa 73%; FTS 33% in each. On files with prefixes, Gemma found every Persian target.
- **Prefixes matter for Gemma:** +9 points on messages, +4 on fixed chunks and +17 on chunks by heading, and MRR +0.09 to +0.14. Without them, Gemma is close to granite on messages.
- **Widths:**
  - **int8 costs nothing:** the same Recall@5 in every condition, and MRR within 0.02.
  - **256 (Gemma only):** with prefixes, the same Recall@5 and an MRR 0.05 to 0.08 lower. Without prefixes, Recall@5 moves by a query either way.
  - granite's int8 messages run (87%, against 82% at full width) is 2 queries; that is noise, not a gain.
- **The Go tokenizer costs granite a few points:** 82% against 84% on messages, 65% against 70% on fixed chunks, 74% on both by heading. MRR is 0.02 to 0.05 lower. Ranks move both ways on single queries. Most texts are not Persian, and a ZWNJ is only part of a Persian text, so the loss is smaller than the Persian check in `builtin/results.md` suggested.

**Chunking and round 2:** round 2 is not triggered. No model has a 10-point gap between the chunkings in file Recall@5:
- granite: heading 74%, fixed 65%, 9 points (int8 the same). With the reference tokenizer: 4 points.
- Gemma with prefixes: heading 100%, fixed 91%, 9 points (256 and int8 the same).
- Gemma without prefixes: fixed is ahead, 87% against 83% (and 91% against 87% at 256).

A file query is 4.3 points, so 9 points is 2 queries. Heading chunks led in the two best conditions, but by less than the line. Round 2 is skipped.

**No-answer queries:** FTS returned nothing for all of them (15 of 15 messages, 7 of 7 files). Vector search always returns something, and its scores overlap the real hits:

| | messages: no-answer max / hits kept | fixed | heading |
|---|---|---|---|
| granite | 0.769 / 48% | 0.750 / 67% | 0.735 / 82% |
| Gemma + prefixes | 0.776 / 59% | 0.725 / 65% | 0.718 / 93% |

*Hits kept* is the share of real hits (target ranked first) above the highest no-answer score. A cutoff there would lose the rest. The scores also differ by model and by width.

**A chunk can be longer than the model's window.** The app counts 4 bytes a token, so a chunk has at most 1,600 bytes. With granite's XLM-R tokenizer, 13 chunks of each chunking are over 512 tokens (up to 701). Most are German and table chunks. hugot sets the Go tokenizer's limit to `max_position_embeddings` (514), but the limit isn't applied. The model then fails on the whole batch ("BroadcastToShape: incompatible dimensions"). The harness now cuts such a text to its longest start that fits 512 tokens (`max_tokens` in `models.yaml`), as the app would have to. The Go tokenizer cut 28 texts, the reference tokenizer 26. The cut-off end of a chunk is not embedded.

**Embedding time:**

| | per 1,000 messages | per 100 pages, fixed | per 100 pages, heading | query p50 / p95 |
|---|---|---|---|---|
| Gemma, Ollama (CPU) | 2m12s | 2m49s | 3m03s | 46 ms / 65 ms |
| Gemma + prefixes | 2m20s | 2m54s | 3m03s | 58–63 ms / 75 ms |
| granite, Go backend | 2m31s | 2m30s | 4m09s | 78–99 ms / 0.77–1.31 s |
| granite, ONNX Runtime | 22 s | 22 s | 26 s | 11–12 ms / 23–27 ms |

- The built-in on the Go backend is about as fast as Gemma in Ollama, for a model under half its size. On ONNX Runtime it is 7 to 10 times faster.
- There are 687 chunks by heading against 392 fixed ones, since sections are short and each chunk is led by its heading path. On the Go backend they took 66% longer per page; in Ollama, 8% longer.
- The Go backend's p95 for a query includes compiling a new shape after each load.
- **Built-in memory and load:** the model loaded in 1.3 to 4.4 s. The harness loads it for each job and closes it after. The process held 6 GB after the messages, 7 GB after the fixed chunks, and peaked at 9.9 GB private bytes on the heading chunks. Free commit fell to 2.1 GB on this 16 GB machine.
- Machine: Intel Core i5-1235U (10 cores, 12 logical CPUs), 16 GB RAM, no GPU, Windows 11 Pro. Other sessions ran on it at the same time. During the granite runs, free physical memory was 0.1 to 1.8 GB. Treat the times as rough.
- Chat smoothness while embedding (N-01) wasn't measured.

**Storage** (bytes; FTS is 0.4 × the text):

| | per vector | per message | per page, fixed | per page, heading |
|---|---|---|---|---|
| Gemma full (768) | 3,072 | 3,072 | 4,973 | 8,715 |
| Gemma 256 | 1,024 | 1,024 | 1,658 | 2,905 |
| Gemma int8 | 772 | 772 | 1,250 | 2,190 |
| granite full (384) | 1,536 | 1,536 | 2,486 | 4,357 |
| granite int8 | 388 | 388 | 628 | 1,101 |
| FTS | — | 51 | 967 | 1,040 |

**Scan time** (from `scan/`): over 250,000 vectors on all cores, about 25 ms at 384 and 256 wide, 40 ms at 768, and 100 ms at 3,072. int8 isn't faster to scan in plain Go, but it takes 4× less memory and storage.

**Decisions (round 1).** The drafts are for the user to settle:
- *Draft:* hybrid search is worth building, for messages and for files. On this corpus it has 2.4 to 3.5 times FTS's recall, and it keeps the FTS hits.
- *Draft:* store vectors as int8 at full width. int8 lost nothing. 256 lost MRR with prefixes and saves less than int8 does.
- *Draft:* chunk files by heading. It led in the best conditions, by 2 queries. Round 2 is skipped.
- *Draft:* query and document prefixes belong in the provider layer. They are set per embedding model in the user's configuration (a preset may fill them), and the text is the same for every model otherwise. Gemma gained 4 to 17 points from them.
- *Draft:* no fixed score cutoff. No-answer and real scores overlap, and the scores differ by model and width.
- **Decided (user, 2026-10-19):** no built-in embedder for now. Embeddings come from Ollama or a cloud provider the user sets up, and N-51 (no cgo) stays. The Go backend can't ship as it is: its speed is near Ollama's and the tokenizer bug costs only a few points, but it held up to 10 GB while it embedded, and it needs a cut to the model's window that hugot doesn't apply.
  - *Later:* a built-in embedder as a separate helper program on ONNX Runtime (7 to 10 times faster than the Go backend). The helper needs cgo, the app doesn't. A crash stops only the helper, and its memory is freed when it exits. The cost is a second program to build, sign and update. Other options weighed: loading ONNX Runtime's library without cgo (unproven), and cgo in the app (breaks N-51).
- Still open: the cloud model. Gemini's free tier stopped it, so round 1 has no cloud numbers.
