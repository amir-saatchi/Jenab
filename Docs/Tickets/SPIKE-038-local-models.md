# SPIKE-038 — Local models on the user's PC
**Type:** Spike
**Status:** Open
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
