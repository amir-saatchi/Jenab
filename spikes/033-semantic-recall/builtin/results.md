# SPIKE-033: built-in embedder

This folder runs a small embedding model inside the process through hugot's
pure-Go backend (`hugot.NewGoSession`, no cgo). It covers the built-in part of
the ticket's Models section, Method 7 and the Measures. Only synthetic text was
used. Runs were on 2026-10-18 and 2026-10-19.

## Summary

- **EmbeddingGemma 2 is not usable on the Go backend today.** It has an ONNX
  export, no gate and an Apache-2.0 license. After a graph rewrite it runs and
  gives the same vectors as ONNX Runtime. But it takes about 32 minutes per
  100 pages, and it ran out of memory on a 16 GB machine at batches of 8 and 32.
- **granite-embedding-107m-multilingual (fp32) is the provisional pick.** It
  is Apache-2.0 and not gated. It had the best Persian recall of the two
  fallbacks (equal to Gemma). It is the fastest on the Go backend: 2m15s to
  2m47s per 100 pages.
- **Two problems block shipping the Go backend as it is:**
  - **The Go tokenizer is wrong for e5 and granite.** These models use an
    XLM-R Unigram tokenizer. hugot's Go tokenizer gave different ids on 28 of 38
    test texts. It turns every Persian zero-width non-joiner (U+200C, very
    common in Persian) into `<unk>`. It also splits some words differently, in
    English and German too. The model math itself is exact: on texts with the
    same ids, the vectors match ONNX Runtime at cosine 1.000000.
  - **Memory.** While embedding 100 pages, the process committed 8.7 to 12.2 GB
    (private bytes). Most of it stays held after GC, until the model is closed.
    Closing frees it (back to 70 MB).
- **int8 doesn't help on the Go backend.** e5 int8 was 12 times slower than
  fp32 (45 minutes per 100 pages). The Gemma int8 graph doesn't load.
- **ONNX Runtime is 6 to 10 times faster** on the same graphs: Gemma 3m19s,
  e5 37 s and granite 17 s per 100 pages. It costs cgo plus a native library
  per platform (the Windows DLL is 19 MB).
- **Still open:** the Ollama-versus-hugot check for Gemma, and whether chat
  stays smooth while the model embeds at background priority. Both are
  explained below.

## Machine

- 12th Gen Intel Core i5-1235U (10 cores, 12 logical CPUs), 16 GB RAM.
- Windows 11 Pro 10.0.26200, windows/amd64.
- Go 1.27.0, hugot v0.8.3, onnx-gomlx v0.5.14, gomlx v0.28.17,
  go-huggingface v0.4.15.
- ONNX Runtime 1.31.0 (Python wheel) for the comparison runs.
- Other sessions were running on the machine at the same time. Free memory
  moved between about 1 and 5 GB. Treat the times as rough.

## Candidates, in the ticket's order

### 1. EmbeddingGemma 2 (`embeddinggemma-2:270m`)

The ticket's four questions:

- **Is an ONNX export published?** Yes, now:
  `onnx-community/embeddinggemma-2-ONNX`. The text tower is `onnx/model.onnx`
  plus `model.onnx_data` (fp32), and `model_quantized.onnx` plus its data
  (q8). The repo also holds the audio and vision towers, which the app doesn't
  need.
- **Does it run on hugot's Go backend, tokenizer included?** Not as published.
  It does after a graph rewrite (`cmd/gemmafix`), but it is too slow and too
  big (see the numbers below). The tokenizer is fine: the Go tokenizer gave the
  same ids as the reference on every test text, Persian included.
- **Is the download ungated?** Yes. The Hugging Face API reports `gated: false`
  for both `google/embeddinggemma-2` and the ONNX repo.
- **Do the terms allow the app to download and run it for every user?** Yes.
  The model is under Apache-2.0. The Gemma 4 license page is the Apache
  License 2.0 itself, so there is no separate Gemma use policy to pass on.

Why it fails on the Go backend as published, and what the rewrite does:

- The graph uses two Microsoft ops that onnx-gomlx lacks or reads differently:
  - **SkipSimplifiedLayerNormalization (48 nodes):** onnx-gomlx doesn't have
    it. The rewrite turns each one into `Add` plus
    `SimplifiedLayerNormalization`, which onnx-gomlx has.
  - **RotaryEmbedding (48 nodes):** the graph passes `position_ids` as the
    constant `[0]`. ONNX Runtime reads that as a start offset, so token *i*
    takes row *i* of the cos/sin cache. onnx-gomlx instead takes row 0 for
    every token, so all positions are lost. The vectors came out wrong
    (cosine 0.81 to 0.92 against ONNX Runtime). The rewrite drops
    `position_ids`, and onnx-gomlx then uses the cache row by row.
- The text tower also has `image_features`, `video_features` and
  `audio_features` inputs. hugot's text pipeline can't fill them. The rewrite
  makes them [1, 512] zero constants. Text has no image or audio tokens, so
  they are never read.
- The q8 graph also uses `MatMulNBits` (146 nodes) and `GatherBlockQuantized`
  (1 node). onnx-gomlx has neither, so the int8 Gemma doesn't load at all.
- hugot looks for `.onnx_data` next to the graph in the model folder, so the
  files go in one flat folder.
- After the rewrite the vectors are right. Over 38 en/de/fa texts, the minimum
  cosine against ONNX Runtime on the original graph is 1.000000. The model
  card's example scores come out within 0.009.

All of these gaps are worth reporting to onnx-gomlx (the two ops and the
`position_ids` reading) and to hugot (filling extra model inputs).

Why it is dropped for the Go backend: about 32 minutes per 100 pages, against
the 5 minute line in the brief. The Go heap also grew to 11.8 GB on just 6
pages, and both full runs (batch 32 and batch 8) died with "out of memory".
`GOMEMLIMIT=3GiB` didn't stop the growth. With ONNX Runtime, the same model
takes 3m19s per 100 pages and peaks at 2.6 GB.

### 2. multilingual-e5-small and granite-embedding-107m-multilingual

| | multilingual-e5-small | granite-embedding-107m-multilingual |
|---|---|---|
| License | MIT | Apache-2.0 |
| Gated | no | no |
| Width | 384 | 384 |
| Loads on the Go backend | yes | yes |
| Go tokenizer matches the reference | no, 28 of 38 texts differ | no, 28 of 38 texts differ |
| Pooling | mean over tokens | CLS (first token), per its `pooling_config.json` |
| Prefixes | "query: " / "passage: " | none |
| fp32 download (graph + tokenizer) | 470 MB + 17 MB | 428 MB + 9 MB |
| int8 download (graph + tokenizer) | 118 MB + 17 MB | 107 MB + 9 MB, third-party only |

- e5's int8 is from `Xenova/multilingual-e5-small` (`onnx/model_quantized.onnx`).
  intfloat also publishes `onnx/model_qint8_avx512_vnni.onnx` (118 MB), which
  is tuned for one CPU feature.
- IBM publishes no int8 for granite. Third-party int8 exports exist (for
  example `sirasagi62/...-ONNX`, Apache-2.0, 107 MB). They weren't run, since
  int8 is slow on the Go backend anyway (see below).
- granite's ONNX graph has two outputs. `logits` is the token-level hidden
  state [batch, tokens, 384]; pool its first token. The other output (named
  `894`) is BERT's pooler with tanh, which is not the embedding.

## Numbers

100 pages = 300 chunks of about 1,200 characters, 261,389 characters in all,
mixed English, German and Persian (`cmd/measure/text.go`). Batches of 32 unless
noted. "Single query" is one short query, after warm-up.

### hugot Go backend

| Model | Load | Single query p50 / p95 | First batch (compiles) | Per 100 pages | Cores busy | Peak working set | Peak private | Private after GC |
|---|---|---|---|---|---|---|---|---|
| Gemma fp32, rewritten, batch 8 | 2.3–2.7 s | 0.87–1.6 s / 6–9 s | 91 s | **32m30s** (6 pages measured) | 6.5 | 3.0 GB | 11.9 GB or more | — |
| e5 fp32 | 0.94 s | 80 ms / 478 ms | 22.6 s | **3m40s** | 7.9 | 4.2 GB | — | 11.3 GB |
| e5 int8 | 0.51 s | 111 ms / 190 ms | 69 s | **44m57s** | 2.5 | 3.8 GB | — | 3.2 GB |
| granite fp32, batch 32 | 1.15 s | 37 ms / 591 ms | 18.1 s | **2m47s** | 7.1 | 2.3 GB | 8.7 GB | 7.8 GB |
| granite fp32, batch 8 | 1.43 s | 35 ms / 350 ms | 3.5 s | **2m15s** | 7.3 | 4.8 GB | 12.2 GB | 11.7 GB |
| granite fp32, batch 8, 50 pages | 1.63 s | 34 ms / 529 ms | 4.9 s | **2m58s** | 6.4 | 3.5 GB | 9.1 GB | 8.8 GB |

- Memory right after load: Gemma 1.5 GB, e5 1.6 GB (int8 0.46 GB), granite
  1.4 GB.
- Gemma ran out of memory on 100 pages at batch 32 and at batch 8. The row is
  from a 6-page run, scaled to 100 pages.
- "Peak private" wasn't recorded for the e5 runs. The e5 "after GC" value was
  read right after embedding, before an explicit GC.
- After `Close()` and GC, granite went back to 27 MB working set and 70 MB
  private. So the held memory belongs to the loaded model and is freed with
  it. The cause inside GoMLX wasn't found.
- p95 single-query times vary a lot between runs. All the queries fall in
  the same padded shape, so this isn't compiling. The cause wasn't checked;
  other load on the machine may add to it.

### ONNX Runtime 1.31.0 (stand-in for hugot's ORT backend)

hugot's ORT backend needs cgo and a C toolchain. This machine has none (no gcc
or zig), so it wasn't built. Instead, `ortbench.py` runs the same ONNX Runtime
version that hugot's build pins (1.31.0) from Python. It uses the same graphs
and the same 300 chunks. It shows the engine's speed. hugot's ORT backend adds
its own tokenizer and Go glue, which this doesn't measure.

| Model | Load | Single query p50 / p95 | Per 100 pages | Cores busy | Peak RSS |
|---|---|---|---|---|---|
| Gemma fp32 (original graph) | 3.1 s | 70 / 119 ms | **3m19s** | 7.6 | 2.6 GB |
| Gemma q8 | 2.0 s | 425 / 667 ms | **3m30s** | 8.1 | 2.3 GB |
| e5 fp32 | 2.3 s | 15 / 26 ms | **37 s** | 7.2 | 1.5 GB |
| granite fp32 | 2.0 s | 4 / 7 ms | **17 s** | 8.1 | 1.2 GB |

- Gemma q8 was no faster than fp32 on this CPU.
- The cost of the ORT backend:
  - cgo, so a C toolchain for every target in the build.
  - A native library per platform. The ONNX Runtime 1.31.0 release archives
    are 10.9 MB (linux-x64), 9.9 MB (linux-aarch64), 34.9 MB (osx-arm64),
    79 MB (win-x64 zip) and 81.8 MB (win-arm64). The unpacked
    `onnxruntime.dll` for win-x64 is 19.3 MB.

### Vector agreement with ONNX Runtime

Over 38 en/de/fa texts, embedded one at a time (`ortcheck.py`):

| Model | All texts: min / median cosine | Texts with the same token ids | Texts with different ids |
|---|---|---|---|
| Gemma fp32, rewritten | 1.000000 / 1.000000 | 38: 1.000000 | 0 |
| e5 fp32 | 0.967 / 0.992 | 10: 1.000000 | 28: min 0.967 |
| granite fp32 | 0.936 / 0.985 | 10: 1.000000 | 28: min 0.936 |

So the Go backend computes the models exactly. All of the gap comes from the
tokenizer.

### The Go tokenizer on XLM-R models

e5 and granite share the XLM-R SentencePiece Unigram tokenizer. hugot's Go
backend tokenizes with `gomlx/go-huggingface` v0.4.15. Against the reference
(Rust `tokenizers` 0.23.3), 28 of 38 texts got different ids: 14 of 16
Persian and 14 of 22 English or German.

- **Zero-width non-joiner:** all 11 texts that contain U+200C got `<unk>` for
  it. The reference maps it to a word break. For example, the Go tokenizer
  turns `می‌شود` into `▁می`, `<unk>`, `شود`. The reference gives `▁می`,
  `▁شود`. Persian writes this character inside most verbs and many nouns.
- **Different splits:** 17 more texts were split differently. For example,
  `Urlaubstage` became `▁Urlaub`, `stag`, `e` instead of `▁Urlaub`, `s`,
  `tage`.

This should be reported to go-huggingface. Until it is fixed, Persian recall
with e5 or granite on the Go backend is lower than the model can give (next
section). Gemma's tokenizer (BPE) wasn't affected.

## Persian check (provisional pick)

22 Persian passages: 10 targets and 12 near-topic distractors. 10 Persian
queries reworded so they share few words with their target. The same 10
questions in English, for the cross-language case. See `cmd/measure/persian.go`.

| Run | fa→fa R@1 | fa→fa R@3 | fa→fa MRR | en→fa R@1 | en→fa R@3 | en→fa MRR |
|---|---|---|---|---|---|---|
| **Reference tokenizer, ONNX Runtime** (`ortpersian.py`) | | | | | | |
| granite | **10/10** | 10/10 | **1.000** | **9/10** | 10/10 | **0.950** |
| e5 with "query: " / "passage: " | 8/10 | 10/10 | 0.900 | 8/10 | 10/10 | 0.900 |
| e5 without prefixes | 8/10 | 10/10 | 0.900 | 8/10 | 10/10 | 0.883 |
| Gemma with its prefixes | 10/10 | 10/10 | 1.000 | 9/10 | 10/10 | 0.950 |
| **hugot Go backend** (`measure persian`) | | | | | | |
| granite | 9/10 | 10/10 | 0.950 | 8/10 | 9/10 | 0.875 |
| e5 with "query: " / "passage: " | 6/10 | 9/10 | 0.775 | 7/10 | 10/10 | 0.833 |
| e5 without prefixes | 9/10 | 10/10 | 0.950 | 8/10 | 10/10 | 0.900 |
| Gemma with its prefixes | 10/10 | 10/10 | 1.000 | 9/10 | 10/10 | 0.950 |
| Gemma without prefixes | 9/10 | 10/10 | 0.950 | 8/10 | 10/10 | 0.900 |

Gemma's two runs agree, which shows `ortpersian.py` scores the same way as
`measure persian`.

**Provisional pick: granite-embedding-107m-multilingual, fp32.**

- With the correct tokenizer it ties Gemma and beats e5 on both sets.
- On the Go backend it is still at least as good as e5 on fa→fa.
- It is the fastest of the three on both backends.
- It needs no prefixes, so there is nothing to get wrong.

This pick is provisional. It rests on 10 query pairs. The spike's Persian
corpus should confirm or change it.

## Ollama check (pending)

The ticket asks to compare Ollama's and hugot's Gemma vectors. It wasn't done:

- Ollama 0.21.0 is installed but wasn't running.
- Its default port 11434 is inside a Windows excluded port range
  (11400–11499), so it can't bind there on this machine.
- On another port (11600) it started, but pulling `embeddinggemma-2:270m`
  failed with a message to download the latest Ollama version.

Updating Ollama is the user's call. `measure ollama` is ready for it, but it
has the port 11434 hard-coded. The rewritten Gemma graph already matches ONNX
Runtime on the original graph at cosine 1.000000. That shows the hugot path is
right, but it doesn't test Ollama's own copy of the model.

## Not measured here

- **Chat smoothness (N-01) while embedding at background priority.** This needs
  the app or the harness. Note that the Go backend kept 6 to 8 of 12 logical
  CPUs busy, and the process held 8 to 12 GB until the model was closed.
- **Recall on the spike's corpora.** That is the harness's job.

## What this means for the app

- The pure-Go route works for granite (and e5), but not yet well enough to
  ship:
  - The tokenizer has to be fixed for Persian, upstream or in the app.
  - The memory has to stay bounded. One option is to close the model after
    each burst of embedding. Whether a smaller batch or a GoMLX setting caps
    it wasn't found.
- Gemma would need ONNX Runtime (cgo) or much more work in onnx-gomlx.
- hugot v0.8.3 needs Go 1.27. The app is on Go 1.26.
- int8 is no use on the Go backend. Ship fp32: about 437 MB for granite.

## Files

Code (module `burrow/spikes/semanticrecall/builtin`, Go 1.27):

- `embedder.go`: the `Embedder` type. `New(ctx, dir, Config)` loads a model
  folder; `Name()`, `Dims()`, `Embed(ctx, texts)` and `Close()`. Texts go in as
  given. Vectors come back un-normalized.
- `cmd/measure`: the measurements (modes `probe`, `bench`, `persian`,
  `reference`, `vectors`, `tokens`, `texts`, `ollama`).
- `cmd/gemmafix`: the Gemma graph rewrite.
- `cmd/onnxops`: lists a graph's inputs, outputs and op counts.
- `ortbench.py`, `ortcheck.py`, `ortpersian.py`: the ONNX Runtime comparison
  runs (Python with onnxruntime 1.31.0, tokenizers, numpy, psutil).

For the harness (`case "builtin":` in `newEmbedder`):

```go
// granite (provisional pick)
builtin.New(ctx, "models/granite-embedding-107m-multilingual", builtin.Config{
	Name: "granite-embedding-107m-multilingual", OnnxFile: "model.onnx",
	Output: "logits", Pooling: "cls",
})
// e5: OnnxFile "model.onnx", Output "last_hidden_state", Pooling "mean";
//     prefix queries with "query: " and passages with "passage: ".
// Gemma 2: OnnxFile "model_go.onnx" (from cmd/gemmafix); output defaults to
//     "sentence_embedding"; queries "task: search result | query: ",
//     documents "title: none | text: ".
```

The module needs a `require` plus a `replace` pointing at this folder.

How the numbers were made, from this folder:

```
go run ./cmd/measure -dir models/granite-embedding-107m-multilingual -onnx model.onnx -output logits -pooling cls bench
go run ./cmd/measure -dir models/granite-embedding-107m-multilingual -onnx model.onnx -output logits -pooling cls persian
go run ./cmd/measure -dir models/multilingual-e5-small -onnx model.onnx -output last_hidden_state -qprefix "query: " -pprefix "passage: " persian
go run ./cmd/gemmafix models/embeddinggemma-2/model.onnx models/embeddinggemma-2/model_go.onnx
go run ./cmd/measure -dir models/embeddinggemma-2 -onnx model_go.onnx -batch 8 -pages 6 bench
go run ./cmd/measure -pages 100 texts texts100.json
python -I ortbench.py models/granite-embedding-107m-multilingual model.onnx texts100.json 32
```

Run one model benchmark at a time. Gemma on the Go backend needs well over
12 GB free.

## Model files and sha256

Downloaded into `models/` (git-ignored). Every sha256 matches the LFS hash on
Hugging Face. URLs are
`https://huggingface.co/<repo>/resolve/<revision>/<path>`.

**`onnx-community/embeddinggemma-2-ONNX`**, revision
`daa72c51243991dfcaf9f9137d2c573d8f7790c0`, into `models/embeddinggemma-2/`
(flat):

| Path in repo | Bytes | sha256 |
|---|---|---|
| onnx/model.onnx | 421,421 | bc47de15f81208a5c99e5ab10f746d5e33b51ea228b7dc0bef9c133a94f1c1c3 |
| onnx/model.onnx_data | 1,084,170,240 | 9fd452bfc6916e5a92f080f8dada35598ca4512b21c65897d4c5974d5f9630a2 |
| onnx/model_quantized.onnx | 495,165 | d06edd601f851c633a2519304cbeb8dc6170d7ceb61b436625c17fb9b6e74953 |
| onnx/model_quantized.onnx_data | 313,724,928 | 278a7ff1248c3618e4bd11a607fc54f7bdc7778854230f3956d3f86bd9db4f3b |
| tokenizer.json | 32,170,510 | 4d777ef5bdc1aa36227abdfb77c3e49e7b9c892d16e1b6bda41c393504828be4 |
| config.json | | 8d011bfe08b5e345bbe0b81e5c6fd02c381920b345b986047bc2a33ce7b90d1d |
| tokenizer_config.json | | 17bd5d6e9364ca49a534e1502076593317c298d4a663623091ed45388f004874 |

`model_go.onnx` is made by `cmd/gemmafix` from `model.onnx`. The output is the
same every run: sha256
`3d379cafc1ac1c9ecbd0bac5924517906aedfdd6868d1f7c5c18d0640e75a406`.

**`intfloat/multilingual-e5-small`**, revision
`614241f622f53c4eeff9890bdc4f31cfecc418b3`, into `models/multilingual-e5-small/`:

| Path in repo | Bytes | sha256 |
|---|---|---|
| onnx/model.onnx | 470,268,510 | ca456c06b3a9505ddfd9131408916dd79290368331e7d76bb621f1cba6bc8665 |
| tokenizer.json | 17,082,730 | 0b44a9d7b51c3c62626640cda0e2c2f70fdacdc25bbbd68038369d14ebdf4c39 |
| config.json | | 69137736cab8b8903a07fe8afaafdda25aac55415a12a55d1bffa9f581abf959 |

Plus the int8 graph from **`Xenova/multilingual-e5-small`**, revision
`761b726dd34fb83930e26aab4e9ac3899aa1fa78`:

| Path in repo | Bytes | sha256 |
|---|---|---|
| onnx/model_quantized.onnx | 118,308,185 | f80102d3f2a1229f387d3c81909990d8945513e347b0eab049f7de3c6f98c193 |

**`ibm-granite/granite-embedding-107m-multilingual`**, revision
`d6cffd338414d6a1c1f5decfad5fec62eebc90d5`, into
`models/granite-embedding-107m-multilingual/`:

| Path in repo | Bytes | sha256 |
|---|---|---|
| model.onnx | 428,102,968 | 387e6f91dce3c651cb3dc06661d74fb1d3947f20ed9c3ecf49d2b3b77086b954 |
| tokenizer.json | 9,081,382 | 7fc9e475f2ac473f070052d92b0c46ec2b91d9cd672935feb6fd837a65d33f78 |

granite's `tokenizer.json` is stored in git, not LFS, so Hugging Face lists no
hash for it. The sha256 above is of the downloaded file.
