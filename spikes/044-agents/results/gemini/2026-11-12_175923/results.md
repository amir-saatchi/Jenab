# SPIKE-044 results

91 runs, 2 with an error (left out below). Conditions: 0 every group, 1 key, 2 asked, groups, 3 asked, single tools, 4 unasked.

## Groups

Of the agent the task went to. Recall: needed groups present; precision: picked groups that are needed; forbidden: runs with a forbidden group. Condition 1 is the key, so it is right by design.

| Model | Condition | Runs | With an agent | Recall | Precision | Forbidden group | Every group |
|---|---|---|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite | 0 every group | 7 | 7/7 (100%) | 1.00 | 0.36 | 6/7 (86%) | 7/7 (100%) |
| gemini/gemini-3.5-flash-lite | 1 key | 21 | 21/21 (100%) | 1.00 | 0.89 | 0/21 (0%) | 0/21 (0%) |
| gemini/gemini-3.5-flash-lite | 2 asked, groups | 21 | 21/21 (100%) | 0.93 | 0.78 | 0/21 (0%) | 3/21 (14%) |
| gemini/gemini-3.5-flash-lite | 3 asked, single tools | 20 | 20/20 (100%) | 0.91 | 0.78 | 1/20 (5%) | 1/20 (5%) |
| gemini/gemini-3.5-flash-lite | 4 unasked | 20 | 20/20 (100%) | 0.90 | 0.75 | 2/20 (10%) | 0/20 (0%) |

## Prompt

The agent's custom prompt, by keyword checks (see `score.go`): states the job, says what to report, repeats a core rule, at most 500 tokens (the app's estimate), in the user's language. For the Persian request, also whether the prompt is in Persian or at least names Persian as the language to answer in.

| Model | Condition | Prompts | Job | Report | Repeats a core rule | ≤ 500 tokens | Mean tokens | Language | Persian request: Persian prompt | Persian request: Persian or names it |
|---|---|---|---|---|---|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite | 0 every group | 7 | 7/7 (100%) | 7/7 (100%) | 0/7 (0%) | 7/7 (100%) | 93 | 7/7 (100%) | 1/1 (100%) | 1/1 (100%) |
| gemini/gemini-3.5-flash-lite | 1 key | 21 | 21/21 (100%) | 21/21 (100%) | 0/21 (0%) | 21/21 (100%) | 93 | 21/21 (100%) | 3/3 (100%) | 3/3 (100%) |
| gemini/gemini-3.5-flash-lite | 2 asked, groups | 21 | 19/21 (90%) | 9/21 (43%) | 0/21 (0%) | 21/21 (100%) | 63 | 20/21 (95%) | 2/3 (67%) | 2/3 (67%) |
| gemini/gemini-3.5-flash-lite | 3 asked, single tools | 20 | 18/20 (90%) | 6/20 (30%) | 0/20 (0%) | 20/20 (100%) | 64 | 18/20 (90%) | 1/3 (33%) | 1/3 (33%) |
| gemini/gemini-3.5-flash-lite | 4 unasked | 20 | 16/20 (80%) | 2/20 (10%) | 0/20 (0%) | 20/20 (100%) | 40 | 18/20 (90%) | 1/3 (33%) | 1/3 (33%) |

## Task

The follow-up task's check, per scenario (passes / runs).

| Model | Scenario | 0 every group | 1 key | 2 asked, groups | 3 asked, single tools | 4 unasked |
|---|---|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite | reviewer | 1/1 | 3/3 | 3/3 | 3/3 | 3/3 |
| gemini/gemini-3.5-flash-lite | eth | 1/1 | 3/3 | 3/3 | 3/3 | 3/3 |
| gemini/gemini-3.5-flash-lite | entry | 1/1 | 3/3 | 3/3 | 3/3 | 3/3 |
| gemini/gemini-3.5-flash-lite | docs | 1/1 | 3/3 | 3/3 | 3/3 | 3/3 |
| gemini/gemini-3.5-flash-lite | research | 1/1 | 3/3 | 3/3 | 3/3 | 3/3 |
| gemini/gemini-3.5-flash-lite | persian | 1/1 | 3/3 | 2/3 | 3/3 | 0/3 |
| gemini/gemini-3.5-flash-lite | write | 1/1 | 3/3 | 3/3 | 2/2 | 2/2 |
| gemini/gemini-3.5-flash-lite | **all** | **7/7 (100%)** | **21/21 (100%)** | **20/21 (95%)** | **20/20 (100%)** | **17/20 (85%)** |

## Refused calls

Calls to a tool outside the chat's tools. After: what the agent did after its first refusal in a run: told the user it can't, asked, called the tool again, or went on another way without saying so.

| Model | Condition | Agent runs with a refusal | Agent refusals | Told | Asked | Retried | Worked around | Mother's refusals | Tools refused |
|---|---|---|---|---|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite | 0 every group | 0/7 (0%) | 0 | 0 | 0 | 0 | 0 | 0 | – |
| gemini/gemini-3.5-flash-lite | 1 key | 0/21 (0%) | 0 | 0 | 0 | 0 | 0 | 0 | – |
| gemini/gemini-3.5-flash-lite | 2 asked, groups | 0/21 (0%) | 0 | 0 | 0 | 0 | 0 | 0 | – |
| gemini/gemini-3.5-flash-lite | 3 asked, single tools | 0/20 (0%) | 0 | 0 | 0 | 0 | 0 | 7 | mother:read_config ×5, mother:insert, mother:request_schema_change |
| gemini/gemini-3.5-flash-lite | 4 unasked | 0/20 (0%) | 0 | 0 | 0 | 0 | 0 | 0 | – |

## Creating

Asked requests are scenarios 1–6 in conditions 2–3: one `create_chat` is right. In condition 4, a creation on the plain message is one nobody asked for; on the job message Mother decides. Asked first: no creation on the request, and Mother's reply asks a question (often "Should I create it?"). Created at the task: no agent came of the request, and Mother created one when the task came; its groups and prompt are scored above.

| Model | Condition | Requests | Exactly one create_chat | No create_chat | More than one | send_to_chat | Asked first | Created at the task | Plain messages with a creation |
|---|---|---|---|---|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite | 2 asked, groups | 18 | 18/18 (100%) | 0/18 (0%) | 0/18 (0%) | 0 | 0/18 (0%) | 0/18 (0%) | – |
| gemini/gemini-3.5-flash-lite | 3 asked, single tools | 18 | 18/18 (100%) | 0/18 (0%) | 0/18 (0%) | 0 | 0/18 (0%) | 0/18 (0%) | – |
| gemini/gemini-3.5-flash-lite | 4 unasked | 18 | 14/18 (78%) | 4/18 (22%) | 0/18 (0%) | 0 | 0/18 (0%) | 4/18 (22%) | 0/20 (0%) |

The write request with no agent yet (scenario 7): Mother delegated it (create_chat or send_to_chat), the two keywords landed, or she neither delegated nor said she can't (pretended).

| Model | Condition | Runs | Delegated | Landed | Pretended | Mother tried a write |
|---|---|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite | 2 asked, groups | 3 | 3/3 (100%) | 3/3 (100%) | 0/3 (0%) | 0/3 (0%) |
| gemini/gemini-3.5-flash-lite | 3 asked, single tools | 2 | 2/2 (100%) | 2/2 (100%) | 0/2 (0%) | 2/2 (100%) |
| gemini/gemini-3.5-flash-lite | 4 unasked | 2 | 2/2 (100%) | 2/2 (100%) | 0/2 (0%) | 0/2 (0%) |

## Tokens

Prompt tokens per request in the agent's task turns (as the provider reports them), and the estimated tokens of the agent's tool definitions against every chat tool's: what each request saves.

| Model | Condition | Prompt tokens per request | Agent's tools | Every tool | Saved per request | Requests per run |
|---|---|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite | 0 every group | 5136 | 3144 | 3648 | 504 | 7.9 |
| gemini/gemini-3.5-flash-lite | 1 key | 3493 | 1830 | 3648 | 1818 | 6.1 |
| gemini/gemini-3.5-flash-lite | 2 asked, groups | 6060 | 1983 | 3648 | 1665 | 18.6 |
| gemini/gemini-3.5-flash-lite | 3 asked, single tools | 5293 | 1649 | 3648 | 1999 | 16.7 |
| gemini/gemini-3.5-flash-lite | 4 unasked | 5305 | 1992 | 3648 | 1656 | 18.6 |

## Groups picked

Per scenario, the groups each run's agent got (asked conditions), as `read_data+files`; `none` when no agent got the task.

**gemini/gemini-3.5-flash-lite, 2 asked, groups**

- reviewer (needs read_data+views_pipelines; never change_data, web): `views_pipelines+files`, `views_pipelines+files`, `views_pipelines+files`
- eth (needs web+views_pipelines+change_data; never none): `read_data+change_data+views_pipelines+files+web+memory`, `read_data+change_data+views_pipelines+files+web+memory`, `read_data+change_data+views_pipelines+files+web+memory`
- entry (needs read_data+change_data; never web): `read_data+change_data`, `read_data+change_data`, `read_data+change_data`
- docs (needs read_data+files; never change_data): `read_data+views_pipelines+files`, `read_data+views_pipelines+files`, `read_data+views_pipelines+files`
- research (needs web+files+memory; never change_data): `files+web+memory`, `files+web+memory`, `files+web+memory`
- persian (needs read_data+files; never change_data, web): `read_data+files`, `read_data+files`, `read_data+files`
- write (needs change_data; never web): `change_data`, `change_data`, `read_data+change_data+views_pipelines`

**gemini/gemini-3.5-flash-lite, 3 asked, single tools**

- reviewer (needs read_data+views_pipelines; never change_data, web): `views_pipelines`, `views_pipelines+files`, `views_pipelines+files+web`
- eth (needs web+views_pipelines+change_data; never none): `read_data+change_data+views_pipelines+files+web+memory`, `read_data+change_data+views_pipelines+web`, `read_data+change_data+views_pipelines+web`
- entry (needs read_data+change_data; never web): `read_data+change_data`, `read_data+change_data`, `read_data+change_data`
- docs (needs read_data+files; never change_data): `read_data+views_pipelines+files`, `read_data+views_pipelines+files+memory`, `read_data+views_pipelines+files`
- research (needs web+files+memory; never change_data): `files+web+memory`, `files+web+memory`, `files+web`
- persian (needs read_data+files; never change_data, web): `read_data+files`, `read_data+files`, `read_data+files`
- write (needs change_data; never web): `read_data+change_data`, `read_data+change_data`

**gemini/gemini-3.5-flash-lite, 4 unasked**

- reviewer (needs read_data+views_pipelines; never change_data, web): `read_data+views_pipelines+files+web`, `views_pipelines`, `views_pipelines+web`
- eth (needs web+views_pipelines+change_data; never none): `read_data+change_data+views_pipelines+web`, `read_data+change_data+views_pipelines+web`, `change_data+views_pipelines+web`
- entry (needs read_data+change_data; never web): `read_data+change_data`, `read_data+change_data`, `read_data+change_data`
- docs (needs read_data+files; never change_data): `read_data+views_pipelines+files+memory`, `read_data+views_pipelines+files`, `read_data+views_pipelines+files`
- research (needs web+files+memory; never change_data): `files+web+memory`, `files+web+memory`, `files+web+memory`
- persian (needs read_data+files; never change_data, web): `views_pipelines+files`, `read_data+views_pipelines+files`, `views_pipelines+files`
- write (needs change_data; never web): `read_data+change_data`, `read_data+change_data`

## Errors

- gemini-gemini-3.5-flash-lite_write_c4_r3: the turn failed: [the turn stopped: gemini says the quota or credit is used up: {"error":{"message":"Rate limit exceeded for model gemini-3.5-flash-lite (limit: 500 requests per day on Free Tier). Please retry in 11h43m59s or upgrade your tier at https://ai.dev/rate-limit.","code":"too_many_requests…
- gemini-gemini-3.5-flash-lite_write_c3_r3: the turn failed: [the turn stopped: gemini says the quota or credit is used up: {"error":{"message":"Rate limit exceeded for model gemini-3.5-flash-lite (limit: 500 requests per day on Free Tier). Please retry in 11h43m59s or upgrade your tier at https://ai.dev/rate-limit.","code":"too_many_requests…
