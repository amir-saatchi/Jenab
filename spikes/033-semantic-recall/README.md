# SPIKE-033: Semantic recall

Throwaway code for [SPIKE-033](../../Docs/Tickets/SPIKE-033-semantic-recall.md). The harness asks whether hybrid search finds earlier chat messages and file passages more often than word search alone. It runs each labelled query three ways:
- **fts:** word search. For messages, it is the app's own `ChatsDB.Search` on a temporary `chats.db` built with the app's store. For files, it is an FTS5 table over the chunks, with `store.Normalize` and a copy of the store's query builder.
- **vector:** cosine similarity only, a brute-force scan in Go.
- **hybrid:** the top 50 of each list, merged with reciprocal rank fusion: score = Σ 1 / (60 + rank).

Vector and hybrid run at each width (Method 5): the model's full width, 256 (cut and renormalized, only for Matryoshka models) and int8. Each model runs once with the text as stored, and once more with the prefixes from its card, if it has any (Method 6).

This folder has five owners (see the ticket):
- The harness: the root, `models.yaml`, `results/` and `testdata/`.
- The corpora: `corpus/`.
- The scan benchmark: `scan/`, its own module.
- The built-in embedder: `builtin/`, its own module.

## Run

Check the corpus first. This finds every quote and prints the counts and the largest chunk:

```bash
go run . -check
```

Print one document's chunks, to see where the chunkers cut it:

```bash
go run . -chunks rental-contract-de.md -chunking heading
```

Run FTS only, with no model:

```bash
go run . -v
```

Run the models, one at a time:

```bash
go run . -v -model gemma-ollama,gemini
```

Run the built-in model from a model folder kept elsewhere. Run it alone: it holds 8 to 12 GB while it embeds.

```bash
go run . -v -model granite -builtin-models ../path/to/builtin/models
```

Write the reference-tokenizer vectors for `granite-reftok` (Python with onnxruntime, tokenizers and numpy, as for `builtin/ortcheck.py`), then read them like any model:

```bash
go run . -texts results/tmp/texts.jsonl
python -I reftok.py builtin/models/granite-embedding-107m-multilingual results/tmp/texts.jsonl results/vectors.db
go run . -model granite-reftok
```

Try the whole run on the test corpus with the fake embedder. Nothing leaves the machine:

```bash
go run . -corpus testdata/corpus -model fake -results ../../.task/spike033
```

Run the tests:

```bash
go test ./...
```

Flags:
- `-corpus`: the corpus folder (`corpus`). It may hold the messages, the files or both.
- `-models`: the embedders file (`models.yaml`). `-model` picks them by name.
- `-env`: the `.env` file with the keys (`../../.env`).
- `-chunking`: `fixed`, `heading` or both.
- `-results`: where `vectors.db`, `queries.jsonl` and `summary.md` go (`results`).
- `-builtin-models`: the folder that holds the built-in models' folders (`builtin/models`).
- `-texts`: write every text a model would embed to a file, then stop.

## models.yaml

Each embedder has:
- `kind`: `ollama` (native `/api/embed`), `openai_compatible` (`/embeddings`), `builtin`, `cached` or `fake`. The kind is chosen here. It is never guessed from the URL.
- `base_url`, `model`, and `key_env`, the name of the key in `.env`.
- `batch_size`: the most texts in one request.
- `matryoshka`: yes for models trained to be cut, which get the 256 run.
- `query_prefix`, `document_prefix`: from the model's card. They are used only in the prefix run.

The built-in embedder is the `builtin` kind. It runs the nested module `builtin/` (a `replace` in `go.mod`, so the harness needs Go 1.27) on hugot's pure-Go backend. Its settings:
- `dir`: the model folder, inside `-builtin-models` when relative.
- `onnx_file`, `output`, `pooling`: as in `builtin.Config`.
- `max_tokens`: a longer text is cut to its longest start that fits. hugot doesn't apply the model's limit on the Go backend, and the model fails on a longer text.

The harness loads the model before each job (a set's items, or its queries) and closes it after, since the Go backend keeps its memory until the model is closed. The load time is logged, not counted in the embedding time. A run that finds every vector in the cache never loads the model.

The `cached` kind can't embed. It reads vectors another tool wrote into `vectors.db`, such as `reftok.py`'s. A text not in the cache stops it.

## How it measures

- **Tokens** are estimated as the app does, at 4 bytes a token. A chunk has at most 400.
- **Fixed chunks** are 400-token windows with up to 50 tokens of overlap. Each cut is at the nearest paragraph or sentence end before the limit. A line end counts as one, so list items and table rows aren't cut.
- **Chunks by heading:** one chunk per section, led by `file.docx › heading › subheading`. A long section is split at paragraph ends. A table stays whole if it fits. Otherwise it is split by rows, with its header repeated. The first chunk of a section covers its heading line.
- **A hit** is a target message, or a chunk that overlaps a target's byte range. A quote must be in its file exactly once, or loading fails. Ranges count in the file with CRLF made LF, so a Windows checkout gives the same ranges.
- **Recall@5** is the share of queries with a target in the top 5. **MRR** is the mean of 1 / rank of the first target, 0 past rank 50. No-answer queries are left out of both.
- **Same-words regressions:** same-words queries that FTS finds and hybrid ranks lower or loses.
- **No-answer queries:** their top cosine, next to the cosine of real hits (targets ranked first by vector search). The share of real hits above the highest no-answer score shows whether a cutoff could work.
- **Embedding time:** each vector keeps its share of its batch's time in the cache. So the times are the first run's, also on a rerun. A page is 1,800 characters. Queries are embedded one at a time, since a hybrid search waits for one.
- **Storage:** 4 bytes a dimension, or 1 byte plus a 4-byte scale for int8. Per message and per page, next to FTS at 0.4 × the text size (SPIKE-011).

## Results

- `results/summary.md`: the tables.
- `results/queries.jsonl`: one line per query and condition, with the targets' ranks and the top 5 with their scores. ChatsDB.Search gives no score, so message FTS scores are 0.
- `results/vectors.db`: the cache (git-ignored). It holds the model's raw vectors by model, use (messages, a chunking or queries), prefix setting and the sha256 of the text as sent. The 256 and int8 forms are made from these when loaded. Delete it if a model changes behind the same name.
- `results/models/`: the built-in model's download (git-ignored).
- `results/tmp/`: temporary files, builds and run logs (git-ignored).

## Keys and safety

- Keys come from the repo-root `.env` or the environment, by `key_env`. They stay in memory. An error text is cleaned of the key before it is shown.
- Only synthetic text is sent.
- A quota or rate-limit error stops that model at once. It is recorded in `summary.md`, and the other models go on. Overloads and lost connections are tried up to 3 times.

## Files

- `main.go`: flags, `-check` and `-chunks`.
- `config.go`: `models.yaml` and the keys.
- `corpus.go`: loads the corpus and checks it; quotes become byte ranges.
- `chunk.go`: the `chunker` interface and the two chunkers. Round 2's model-made chunks would be a third.
- `fts.go`: message FTS through the store, chunk FTS, and the copied query builder.
- `embed.go`: the Ollama, OpenAI-compatible and fake embedders.
- `builtin.go`: the built-in embedder's wrapper (load, close, cut) and the `cached` kind.
- `reftok.py`: granite with the reference tokenizer, through ONNX Runtime, into `vectors.db`.
- `cache.go`: `vectors.db` and batching.
- `vector.go`: the forms, the scan and RRF.
- `eval.go`, `report.go`: the conditions, the measures and the output.
- `testdata/corpus/`: a small corpus in the shared format, for the tests.
