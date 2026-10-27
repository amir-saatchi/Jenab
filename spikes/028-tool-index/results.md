# SPIKE-028 results: loading tools and history when needed

Run on 2026-10-27, in two rounds:

- **Round 1** (`results/live/`) had a harness bug: the index hid three
  skills (see below). Its conditions 2 and 3 are superseded. Its
  condition 1 for the three sets still stands, because the fixes don't
  touch condition 1.
- **Round 2** (`results/round2-sets/`, `results/round2-long/`) re-ran
  condition 2 on the sets, and conditions 1–4 on the long chats.

The merged report is `results/round2-long/results.md`. Where a run appears
in both rounds, round 2's counts. The harness is described in `README.md`.

**Models.** Gemini was not run. The user chose to save Gemini's shared
quota, so this decision rests on two models only:

- `ollama-cloud/gemma4:31b`;
- `mistral/ministral-14b-latest`, which reports cache reads.

Ollama reports no cache reads. No quota or rate limit stopped a run in
either round.

**Conditions.**

1. Today: every tool native.
2. The tool index: the core tools and the history tools stay native; the
   rest are behind `describe_tool` and `call_tool`.
3. The index plus history cards.
4. Today's tools plus history cards.

Conditions 3 and 4 run on long chats only.

**Runs per model.**

| set | scenarios | conditions | reps | runs |
|---|---|---|--:|--:|
| multi-part | 9 (11 skipped: they need background runs) | 1, 2 | 3 | 54 |
| skill-loading | all 12 | 1, 2 | 3 | 72 |
| rare-tools | 4 | 1, 2 | 3 | 24 |
| long chats | 5 | 1, 2, 3, 4 | 5 | 100 |

## What round 2 fixed

- **Skills under the index.** The app hides a skill when every tool in its
  `load_with` is missing from the registry (`Skills.Usable`). With the
  index, those tools weren't in the registry, so config-guide, pipelines
  and web-research vanished in conditions 2 and 3.
  - `call_tool`'s `load_with` then failed silently.
  - In round 1, config-guide loaded 19 times in condition 1 of
    skill-loading and 0 times in condition 2.
  - Round 1's line "the skill still arrives after the first write" was
    wrong: it never arrived.
  - Now `keepSkills` keeps condition 1's skills (`TestSkillsWithIndex`).
    config-guide loads 18 times in condition 2 of skill-loading. In long
    chats it loads in 36, 34, 35 and 35 runs (conditions 1–4).
- **Native calls of index tools.**
  - Before, they got the app's plain "there is no tool named X".
  - Now the model gets "X is in the More tools list … call_tool(name: "X",
    …)". The app has no hook for this result, so the spike rewrites it in
    the requests it sends (`redirect.go`).
  - Round 1's ministral R3 0/3 was this: it called read_feed natively,
    was told the tool doesn't exist, and told the user so. With the
    redirect it scores 3/3.
- **History tools.** `search_history` and `read_messages` are now native
  with the index. Block 1's "Earlier turns" line names them.
- **Argument names.** call_tool refuses extra argument names only where
  the tool's own schema does, as a native call would. Round 1 refused
  three of ministral's web_search calls that a native call would have
  allowed.
- **Counting.** Nested calls (`call_tool` naming call_tool or
  describe_tool) are now counted on their own.
  - Round 1's gemma "11 index tools called natively" were nested calls.
  - The column "index tool called natively" counts every unknown name
    called natively. That includes made-up names: ministral calls
    `read_config` 13 times in condition 1 of long chats. config-guide
    mentions it, but it is not a tool of these sets.
- **Long chats.** Condition 4 is new. Each long chat now runs 5 reps, and
  two paraphrase chats were added:
  - H4: "per-week spread" for "weekly range";
  - H5: "weekly roundup" for a Monday digest.

## The tool index (8.1): condition 2 against 1

### Size

The index replaces the full definitions of the non-core tools. With the
history tools now native, these figures are lower than round 1's:

| set | non-core tools | full definitions | index + describe_tool + call_tool | saved per request |
|---|--:|--:|--:|--:|
| multi-part | 16 | 1,388 tok | 550 tok | 838 (60%) |
| skill-loading | 13 | 1,227 tok | 498 tok | 729 (59%) |
| rare-tools | 16 | 1,704 tok | 549 tok | 1,155 (68%) |
| long chats | 13 | 1,262 tok | 503 tok | 759 (60%) |

In every set, the index holds every non-core tool, not only the rare
ones. This includes rare-tools, so that set doesn't test an index of rare
tools alone.

### Tasks done

| set | gemma c1 | gemma c2 | ministral c1 | ministral c2 |
|---|--:|--:|--:|--:|
| multi-part | 26/27 | 27/27 | 21/27 | 21/27 |
| skill-loading | 33/36 | 34/36 | 27/36 | 28/36 |
| rare-tools | 12/12 | 12/12 | 12/12 | 12/12 |
| long chats | 19/25 | 18/25 | 10/25 | 10/25 |
| **all** | **90/100** | **91/100** | **70/100** | **71/100** |

With the fixes, the index costs no tasks on these sets.

### Prompt tokens per run

| set | gemma c1 → c2 | ministral c1 → c2 |
|---|--:|--:|
| multi-part | 8,791 → 6,457 (−27%) | 10,988 → 6,969 (−37%) |
| skill-loading | 9,249 → 13,722 (+48%) | 10,607 → 12,326 (+16%) |
| rare-tools | 7,600 → 6,702 (−12%) | 7,826 → 7,893 (+1%) |
| long chats | 25,460 → 24,706 (−3%) | 31,152 → 22,792 (−27%) |

Multi-part is the cleanest number: few skills and no history. In the other
sets, the extra requests eat the saving:

- skill-loading: 2.2 → 3.3 requests per run on gemma, 2.4 → 3.4 on
  ministral;
- rare-tools: 2.0 → 3.5 requests per run on ministral.

Each extra request carries the skill text and the earlier results again.

### Mistakes with the index (condition 2, all sets)

| | gemma | ministral |
|---|--:|--:|
| wrong arguments | 16 of 92 calls | 8 of 78 calls |
| blind calls (call_tool before describe_tool) | 48 | 28 |
| unknown name called natively (index tools, redirected; and made-up names) | 6 | 39 |
| nested call_tool | 8 | 0 |
| failed → later worked | 44 → 33 | 50 → 41 |

The most common native call is `save_view`: 36 times across skill-loading
condition 2 and long chats conditions 2 and 3, mostly by ministral. The
redirect turns those calls into recoveries: in skill-loading, every tool
ministral first called wrongly later worked (16 of 16 tools).

### Proposed decision on 8.1: no index for Jenab's own tools; keep the design for MCP

Once the harness stops hiding skills, the index neither loses nor wins
tasks (90 → 91 and 70 → 71 of 100). The token saving is real only where
there are few extra requests: multi-part saved 27–37%. With skills in
play, it costs more than it saves (+16% to +48% in skill-loading). It
also adds mistakes that a native tool doesn't have: blind calls, wrong
arguments, and native calls that need a redirect.

Jenab's own non-core tools are 1,200–1,700 tokens, cached on Mistral.
That isn't worth those costs or the app changes the index would need:

- a redirect result for native calls of index tools;
- `Skills.Usable` counting indexed tools;
- history tools and tools that skills name kept native.

For MCP tools (SPEC 8.7, `mcp_describe` / `mcp_call`), the design holds up
once the skills fix and the redirect are in. Take those two into 8.7.

## History cards (3.6)

Cards are found the same way in every run of a chat. These are the cards
in conditions 3 and 4 for both models, in all 5 reps:

| chat | needed turn | cards | |
|---|--:|---|---|
| H1 "Make that 14-day average chart for ETH too" | 2 | [2] | hit |
| H2 "Could ETH get the early warning as well?" | 3 | none | miss: a pure paraphrase of "drop alert" |
| H3 in window (control) | 9 | none | correct |
| H4 "per-week spread for Ether" | 5 | [5] | hit |
| H5 "that weekly roundup too? Store it in eth_news" | 4 | [2, 4] | hit, plus an unrelated card for turn 2 |

So the card had the needed turn in 15 of 20 runs per condition and model:
every run of H1, H4 and H5, and no run of H2.

### Condition 4 against 1 (today's tools, with and without cards)

This is the cleanest comparison: the same tools and the same skills.

| | gemma c1 | gemma c4 | ministral c1 | ministral c4 |
|---|--:|--:|--:|--:|
| done | 19/25 | **25/25** | 10/25 | **17/25** |
| H1 | 4/5 | 5/5 | 5/5 | 5/5 |
| H2 (no card) | 5/5 | 5/5 | 0/5 | 0/5 |
| H3 (control) | 5/5 | 5/5 | 5/5 | 5/5 |
| H4 | 1/5 | 5/5 | 0/5 | 5/5 |
| H5 | 4/5 | 5/5 | 0/5 | 2/5 |
| requests/run | 4.8 | 3.1 | 5.4 | 2.9 |
| prompt tok/run | 25,460 | 14,801 (−42%) | 31,152 | 15,950 (−49%) |
| history tool calls | 52 | 17 | 39 | 5 |

Condition 3 against 2 (the same, with the index) points the same way:

- done goes 18 → 22/25 on gemma and 10 → 15/25 on ministral;
- prompt tokens per run go 24,706 → 14,170 on gemma and 22,792 → 18,185 on
  ministral.

Ministral fails H2 in every condition: it says what it will do and stops,
without a call.

### Proposed decision on 3.6: add history cards, with today's tools

When the card has the turn, the follow-up gets done:

- H4 went from 1/5 to 5/5 on gemma and from 0/5 to 5/5 on ministral;
- requests fell by a third or more, prompt tokens by 42–49%, and history
  searches by two thirds or more.

The cards added no requests. They gave no card in the window (H3). Their
one wrong card, H5's turn 2, came alongside the right one and didn't
stop gemma, which scored 5/5.

Cards miss a follow-up that shares no word with the old turn (H2). So
`search_history` and `read_messages` must stay native, as the fallback.

## Noise, and what the numbers can't show

- **Correlated reps.** Cells flip between 0/3 and 3/3 where the condition
  can't matter. Examples:
  - ministral L05: 0/3 in condition 1, 3/3 in round 1's condition 2;
  - ministral t4_d: 0/3 in round 1's condition 1, 2/3 in round 1's
    condition 2;
  - gemma H2: 0/3 against 2/3 in round 1, with the same setup.

  Reps of one cell run back to back, so they look correlated. A gap of one
  or two tasks in a cell is noise. The differences above that this
  writeup leans on are 6–7 tasks out of 25, or a whole scenario going from
  0–1/5 to 5/5.
- **Cache figures are noise.** Ministral's cache-read share swings widely
  between identical prompts. In round 1, the same 4,817-token H3 prompt
  got between 512 and 4,736 cache reads. Mistral's cache numbers here
  (39–67%) are not a measure of either the index or the cards. Ollama
  reports none.
- **Different rounds.** Condition 1 for the three sets comes from round 1,
  and condition 2 from round 2. The fixes don't touch condition 1, but
  the two ran about an hour apart.
- **Fake write tools.** save_view and save_pipeline answer "saved" without
  checking the config. So a write made without the skill costs nothing
  here; in the app, it would cost a failed write and a retry.
- **Card thresholds.** They were set on the offline scripts before any
  live run, and were not changed after either round.
