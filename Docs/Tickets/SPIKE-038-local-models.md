# SPIKE-038 — Local models on the user's PC
**Type:** Spike
**Status:** Done
**Gate:** 3
**Needs:** TASK-029

## Question
Many users will run Jenab with a local Ollama model and no key. Can small local models do Jenab's basic work, on a laptop with no usable GPU, and how slow is it?

1. **Speed:** how long does each model take to read a 2k, 8k and 16k-token prompt and to write an answer, on this PC? This tells which tests are realistic.
2. **Basic work:** do they call a tool and answer, query the database, save to memory, and run *Refresh memory* on a short chat?
3. **JSON mode:** TASK-029 sends the answer schema as Ollama's `format`, and local Ollama holds the model to it while it writes. Does that make *Refresh memory* work on small models that fail without it?

## The PC
Intel i5-1235U, 16 GB RAM, Intel Iris Xe (Ollama runs on the CPU). Ollama 0.40.6, served at `http://127.0.0.1:11434`.

## Models
Local, with tool calling: `qwen3.5:4b`, `llama3.2:3b`, `granite4:350m`. `lfm3:1.7b` (tools, structured output, 256K context) needs Ollama 0.40.13 or newer; it is added once the user updates Ollama. Not used: models without tools (deepseek-r1:1.5b, medgemma), embedding models, and the `:cloud` models, which run on Ollama's servers.

## Scope
1. **Speed probe:** for each model, the time to read prompts of 2k, 8k and 16k tokens and the writing speed, from Ollama's own timings. One model at a time, unloaded after its run.
2. **A short local scenario set,** with short prompts, written before any run:
   - one tool call and an answer;
   - a database question;
   - a memory save;
   - *Refresh memory* on a short chat (about 6 turns), with the schema sent as `format` and without it.
3. **Runs:** each model, 3 reps where time allows. Local Ollama is set up as a normal user connection (kind `ollama`, base URL `http://127.0.0.1:11434`), the same prompts, loop and configs as every model. Results recorded as they are; no tuning for these models.

## Rules
- One model loaded at a time, and unloaded after its runs, so the laptop stays usable. No runs at the same time as other heavy work.
- Don't pull or update models without asking. Don't stop or restart the user's Ollama app.
- No cloud quota is used.

## Done when
- A table per model: speed, and which tasks pass, with and without JSON mode.
- A short answer to the three questions, and what it means for users with local models (for example, a note in the UI or docs about speed or minimum model size).

## Result
Done on 2026-10-31. Commit 2b4fb8c; results and scripts are in `spikes/038-local-models/`.

- **Setup:** Ollama 0.40.6 on the CPU. granite4:350m ran at a 32k context; the other two at 8k, because 32k needed 5-6 GB that wasn't free. A guard stopped our runs when free RAM stayed under 500 MiB; it did so 3 times, so some runs have fewer reps.
- **Per model** (read and write speeds from Ollama's timings; RAM is the Ollama processes' peak working set):

  | Model | Read 2k / 8k prompt | Write | RAM | Tool + answer | Database question | Memory save | Refresh, JSON mode on | Refresh, off |
  |---|---|---|---|---|---|---|---|---|
  | granite4:350m | 290 / 125 tok/s | 25-30 tok/s, about 5 at 16k | 2.6 GB | 1/3 | 0/3 | 0/3 | 1/3 | 0/3 |
  | llama3.2:3b | 55 / 38 tok/s | 11 tok/s, 3 at 8k | 4.6 GB | broke 3/3 | broke 3/3 | broke 2/2 | 0/3 | 0/3 |
  | qwen3.5:4b | 35 / 35 tok/s | 6.5 tok/s, 5.5 at 8k | 4.6 GB | 2/2 | 2/2 | 2/2 | 1/3 | not run (RAM) |

- **1. Speed:** reading costs the most. Jenab's prompt and tools are about 3,000-3,700 tokens, so a 3-4B model spends about 1-2 minutes on a turn's first request; a task took 30 s to 4 minutes. A 16k prompt took granite 3.5 minutes. Writing is 3-11 tokens a second on the 3-4B models.
- **2. Basic work:** qwen3.5:4b did all 6 basic runs. granite4:350m can't: it made up numbers instead of querying and didn't save to memory. llama3.2:3b couldn't be judged: its Ollama template sends the tools only when the last message is the user's, so after a tool result Ollama reads about 600 tokens of Jenab's estimated 3,400, and Jenab's truncation check stops the turn (TASK-032).
- **3. JSON mode** fixes the format, not the content. With it, 30 of 32 schema answers were plain JSON (both misses were one granite merge, 527 s and a 1,169 s retry); without it, 9 of 21. Most misses were llama's JSON in a code fence, which Jenab reads anyway. Refresh still mostly failed: the small models drop facts. llama kept the newer report day in 4 of 6 runs but lost the "no chemical pesticides" rule in all 6, with JSON mode or without.
- **For users with local models:**
  - About 4B is the smallest model for Jenab's tools; under 1B doesn't work.
  - On a laptop CPU, expect 1-4 minutes a turn, and 4-5 GB of free RAM for a 4B model at an 8k context.
  - *Refresh memory* loses facts on 3-4B models; a bigger or cloud model does it better.
  - A short note in the connection settings and the docs should say this.
- **Compared with gemma4:31b** on Ollama Cloud (the same sets, 3 reps): every run passed, in 6-17 s, Refresh memory included, with JSON mode and without. Its schema answers came in a code fence both ways, so Ollama Cloud doesn't seem to hold the model to `format` as local Ollama does.
- **After TASK-032,** llama3.2:3b's basic runs no longer break: tool + answer 3/3, database question 0/3, memory save 0/3.
- **Not done:** lfm3:1.7b needs Ollama 0.40.13 or newer. qwen3.5:4b's refresh without JSON mode didn't run for lack of free RAM.
