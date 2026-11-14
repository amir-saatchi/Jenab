# SPIKE-046 — A LoRA for the database agent on lfm3:1.7b
**Type:** Spike
**Status:** Open (Part A now; Part B after SPIKE-044)
**Gate:** 3
**Needs:** SPIKE-038; SPIKE-044 for Part B

## Question
lfm3:1.7b is the best local model we tested (SPIKE-038), and Liquid says it is made to be fine-tuned on one GPU. Can a LoRA trained on Jenab's own runs make it a reliable **database agent** on a laptop CPU, and is the upkeep worth it?

1. **Baseline:** how well does the base model do database work that is harder than SPIKE-038's single question? If it already passes about 90% on the held-out set, there is nothing to train.
2. **Data:** can we make 1,000-3,000 good examples from a teacher model, kept only when a check passes?
3. **Gain:** does the LoRA beat the base model on database tasks it never saw, without getting worse at other basic work?
4. **Upkeep:** how long does one full retrain take, from new data to a model in Ollama? How much does the tuned model lose when Jenab's prompt or tools change and it isn't retrained?

## Background
- **SPIKE-038:** lfm3:1.7b passed every basic run (tool + answer, a database question, a memory save) at 1.8 GB and 2-3 times the speed of the 3-4B models. It failed *Refresh memory* without JSON mode (0/3) and broke the schema on retries.
- **SPIKE-044:** a chat can be an agent with tool groups and a custom prompt. A database agent is *Read data* (`query`, `describe_table`) and *Change data* (`insert`, `request_schema_change`), plus the always-on tools. That fixed set of prompt and tools is what the LoRA learns.
- **What a LoRA learns:** behaviour, not facts. Which tool to call, SQL that SQLite and the SQL guard (SPEC 2.2) accept, answers only from tool results, and the JSON schemas. A small tuned model follows what it was trained on more than the prompt, so a change to a tool's name, arguments, result format or a prompt rule needs a retrain. Rewording mostly doesn't.
- **The data is already recorded:** each `agent.RequestTrace` keeps `Req`, the System, Tools and Messages exactly as sent (`internal/agent/trace.go`), and the scenario asserts say which runs were right.

## Rules
- **Model-neutral (SPEC 1, 3.9):** the tuned model gets the same prompts, tools, loop and configs as every model. Nothing in the app knows about it. The user picks it in the model picker like any other model; it is never preset or suggested by the app.
- **Training data is synthetic:** made from fixtures and scripted requests only, never real chats. No names, account IDs or user-name paths in the data, the notebook or a published model.
- **Teachers:** open-weight models whose licenses, and whose provider's terms, allow training on their outputs. First choice: mistral-large-4 on NVIDIA (a reference model), once NVIDIA's free-endpoint terms and the model's license are checked. gemma4:31b is out since Ollama Cloud's free plan ended (2026-11-09); `gemma-4-26b-a4b-it` on the Gemini API works with tools, but the Gemini API's terms need the same check. Not Claude, OpenAI or Gemini models, whose terms restrict training competing models on their outputs. Claude Haiku 5.5 may still run the held-out set as a ceiling, since nothing is trained on those runs.
- **License:** read the LFM Open License v1.0 before publishing a tuned model anywhere. Training and using it locally comes first.
- **Keys:** the keys cleared for scenario runs only. Ask before using the reserve keys or the Anthropic key.
- Keep the PC awake during long local runs. One local model loaded at a time; don't pull or update models without asking.

## Scope

### Part A: baseline and data (can start now)
1. **SQL task set.** A generator of database requests over several fixtures: the Bitcoin and crypto-watch fixtures, the `data-tools` ledger, and 3-4 new ones with other schemas (a shop, a clinic's bookings, a reading list). Kinds:
   - totals, averages and counts; joins; date ranges; filters and sorting; top-N;
   - a table the chat hasn't seen, so it needs `describe_table` first;
   - adding rows with `insert`;
   - a request the guard refuses (an internal `_jenab_` table), which the chat must handle;
   - a question the data can't answer, where the right answer says so;
   - the same requests in Persian.

   Each task has a reference answer: the expected rows, checked by running SQL on the run's own database, or a `db_query` check for inserts. No judge model.
2. **Split by fixture.** Some fixtures are for training and the rest are held out, so the model can't learn the tables by heart. The existing sets (`data-tools`, `local-basic`, `missing-features`, `schema-design`) are never used for training.
3. **Baseline.** Base lfm3:1.7b on the held-out set, 3 reps, local Ollama. The teachers on the same set, for the ceiling. **Stop here** if the base model passes about 90%.
4. **Export.** `jenab-scenarios export` writes every request of a passing run as one chat-format JSONL example: system, tools, messages, and the assistant's reply with its tool calls, from `RequestTrace.Req`. Subagent requests (the schema agent's) go to their own file. A scrub check fails the export on personal IDs.
5. **Teacher runs** on the training fixtures until there are 1,000-3,000 passing examples. Record how many runs failed and why.

### Part B: training (after SPIKE-044 settles the database agent's prompt and tools)
6. **Notebook** in `spikes/046-sql-lora/`, for Google Colab:
   - Unsloth and TRL's `SFTTrainer`, LoRA on `LiquidAI/LFM3-1.7B` from Hugging Face (not Ollama's GGUF).
   - On an L4 or A100 from a Colab account with compute units: bf16 LoRA, as the model was trained. Start on the L4, which costs fewer units an hour, and move to the A100 if a run doesn't fit in one session.
   - Either way: gradient checkpointing for 4-8k-token examples, checkpoints saved to Drive every few hundred steps, and the runtime disconnected as soon as training ends, since units are spent while it is connected.
   - Fallback on a T4 (free tier, or an account without units): fp16 or 4-bit QLoRA, batch size 1 with gradient accumulation. Kaggle is another fallback.
   - A Hugging Face token, if needed, in Colab's Secrets, never in the notebook.
7. **Back to Ollama:** merge the LoRA, convert to GGUF and quantize to Q4_K_M, write a Modelfile with lfm3's chat template, and `ollama create jenab-lfm3-sql`.
8. **Evaluation,** 3 reps each, base against tuned:
   - the held-out SQL set;
   - SPIKE-038's `local-basic`, `local-refresh` (with and without JSON mode) and `memory`, to see what else changed;
   - speed and RAM, which should stay the same.
9. **Drift test.** On a branch, make one contract change of each kind: rename a tool argument, change `query`'s result format, add a tool. Rerun the held-out set on base and tuned without retraining. This answers question 4.
10. **DPO, only if needed.** If SFT helps but not enough, a second pass with a failing and a passing run of the same task as a pair.

## Done when
- A table for the held-out set and SPIKE-038's sets: base, tuned and teacher, with pass rates, speed and RAM.
- The data: how many examples, how many teacher runs it took, and the share that failed.
- The upkeep: time and cost of one full retrain, step by step, and the drift test's result.
- A go or no-go. If go: the tasks it needs (the export command in the runner, how the model is published and versioned, a docs note for users of local models, and how the model is checked again each release).
