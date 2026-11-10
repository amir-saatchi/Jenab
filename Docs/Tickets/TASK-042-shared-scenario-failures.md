# TASK-042 — Scenario failures every model shares
**Type:** Task
**Status:** Open
**Gate:** 3
**Needs:** —

## Goal
On 2026-11-10 the three reference-level models ran the same 7 scenario sets with the same code: `gemini-3.5-flash-lite`, `mistral-large-4-1t-a52b` (NVIDIA) and `muse-glimmer-30b`. Some scenarios fail on all three ([NVIDIA-MODELS-report](reports/NVIDIA-MODELS-report.md), [GEMINI-BASELINE-report](reports/GEMINI-BASELINE-report.md)). A failure every model shares may be a bad check, a gap in Jenab's prompt, skills or tools, or a real limit of the models. Find out which, for each one, and fix what is Jenab's.

## The failures
1. **`memory` M2_tldr:** in a new chat, the model calls `list_chats`, `search_history` or `read_messages` before answering, though the TL;DR rule is in its memory block. The answer does start with "TL;DR:".
2. **`project-card` C1_keywords_query:** the model calls `describe_table` on `btc_news` before making the keyword table, though the project card lists its columns.
3. **`grounding-honest` R1_moving_avg_table:** the 7-day average uses only rows inside the asked range (`WHERE` before the window), so the first six days average fewer than 7 days.
4. **`grounding-honest` R4_worked_example, R5_supply_cap, R7_saved_config:** the number check (SPIKE-032) fires on numbers the model computed itself or knows (a worked example, the 21 million supply cap, a saved config's values).

## Scope
For each failure:
1. Read the transcripts of all three models (the run reports are in the reports above; rerun only where needed).
2. Decide: **the check** (it asks for something the SPEC doesn't require, or is too strict), **Jenab** (the prompt, a skill, a tool description, the project card or the number check gives the model wrong or missing guidance), or **the model** (the guidance is clear and models still fail).
3. Fix what is Jenab's or the check's, the same way for every model (no model-specific setup). A fix to the number check must keep SPIKE-032's made-up-number scenarios (`grounding`) catching what they catch today.
4. Rerun the affected scenarios on `gemini-3.5-flash-lite` and `mistral-large-4-1t-a52b`, before and after the fix, with `-reps 3`.

## Rules
- Reference models only for decisions: `gemini-3.5-flash-lite` (`GEMINI_API_KEY`) and `mistralai/mistral-large-4-1t-a52b` (NVIDIA, `NVIDIA_API_KEY`). Failures on other models are recorded, not worked around.
- Gemini: under 300 requests for the ticket, keep the key under 400 a day (the day resets at 08:00 UTC). NVIDIA: about 40 requests a minute, shared. Stop on a quota error.
- Don't loosen a check only to make it pass: say why the SPEC allows what the model did.

## Done when
- Each of the four has a verdict with the evidence (the transcript lines that show it).
- The fixes are in, with Go tests where the change is in code, and `jenab-scenarios check scenarios` passes.
- Before and after pass shares for the affected scenarios on both models.
- The report proposes any SPEC text.
