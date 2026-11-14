# SPIKE-044 results

105 runs, 0 with an error (left out below). Conditions: 0 every group, 1 key, 2 asked, groups, 3 asked, single tools, 4 unasked.

## Groups

Of the agent the task went to. Recall: needed groups present; precision: picked groups that are needed; forbidden: runs with a forbidden group. Condition 1 is the key, so it is right by design.

| Model | Condition | Runs | With an agent | Recall | Precision | Forbidden group | Every group |
|---|---|---|---|---|---|---|---|
| anthropic/claude-haiku-5-5 | 0 every group | 21 | 21/21 (100%) | 1.00 | 0.36 | 18/21 (86%) | 21/21 (100%) |
| anthropic/claude-haiku-5-5 | 1 key | 21 | 21/21 (100%) | 1.00 | 0.89 | 0/21 (0%) | 0/21 (0%) |
| anthropic/claude-haiku-5-5 | 2 asked, groups | 21 | 21/21 (100%) | 0.93 | 0.81 | 0/21 (0%) | 0/21 (0%) |
| anthropic/claude-haiku-5-5 | 3 asked, single tools | 21 | 20/21 (95%) | 0.95 | 0.83 | 0/20 (0%) | 0/20 (0%) |
| anthropic/claude-haiku-5-5 | 4 unasked | 21 | 18/21 (86%) | 0.90 | 0.81 | 0/18 (0%) | 0/18 (0%) |

## Prompt

The agent's custom prompt, by keyword checks (see `score.go`): states the job, says what to report, repeats a core rule, at most 500 tokens (the app's estimate), in the user's language. For the Persian request, also whether the prompt is in Persian or at least names Persian as the language to answer in.

| Model | Condition | Prompts | Job | Report | Repeats a core rule | ≤ 500 tokens | Mean tokens | Language | Persian request: Persian prompt | Persian request: Persian or names it |
|---|---|---|---|---|---|---|---|---|---|---|
| anthropic/claude-haiku-5-5 | 0 every group | 21 | 21/21 (100%) | 21/21 (100%) | 0/21 (0%) | 21/21 (100%) | 93 | 21/21 (100%) | 3/3 (100%) | 3/3 (100%) |
| anthropic/claude-haiku-5-5 | 1 key | 21 | 21/21 (100%) | 21/21 (100%) | 0/21 (0%) | 21/21 (100%) | 93 | 21/21 (100%) | 3/3 (100%) | 3/3 (100%) |
| anthropic/claude-haiku-5-5 | 2 asked, groups | 21 | 18/21 (86%) | 20/21 (95%) | 5/21 (24%) | 21/21 (100%) | 197 | 18/21 (86%) | 0/3 (0%) | 3/3 (100%) |
| anthropic/claude-haiku-5-5 | 3 asked, single tools | 20 | 17/20 (85%) | 19/20 (95%) | 13/20 (65%) | 20/20 (100%) | 216 | 17/20 (85%) | 0/3 (0%) | 2/3 (67%) |
| anthropic/claude-haiku-5-5 | 4 unasked | 18 | 18/18 (100%) | 18/18 (100%) | 4/18 (22%) | 18/18 (100%) | 158 | 18/18 (100%) | – | – |

## Task

The follow-up task's check, per scenario (passes / runs).

| Model | Scenario | 0 every group | 1 key | 2 asked, groups | 3 asked, single tools | 4 unasked |
|---|---|---|---|---|---|---|
| anthropic/claude-haiku-5-5 | reviewer | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| anthropic/claude-haiku-5-5 | eth | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| anthropic/claude-haiku-5-5 | entry | 3/3 | 3/3 | 3/3 | 3/3 | 2/3 |
| anthropic/claude-haiku-5-5 | docs | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| anthropic/claude-haiku-5-5 | research | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| anthropic/claude-haiku-5-5 | persian | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |
| anthropic/claude-haiku-5-5 | write | 3/3 | 3/3 | 3/3 | 2/3 | 3/3 |
| anthropic/claude-haiku-5-5 | **all** | **21/21 (100%)** | **21/21 (100%)** | **21/21 (100%)** | **20/21 (95%)** | **20/21 (95%)** |

## Refused calls

Calls to a tool outside the chat's tools. After: what the agent did after its first refusal in a run: told the user it can't, asked, called the tool again, or went on another way without saying so.

| Model | Condition | Agent runs with a refusal | Agent refusals | Told | Asked | Retried | Worked around | Mother's refusals | Tools refused |
|---|---|---|---|---|---|---|---|---|---|
| anthropic/claude-haiku-5-5 | 0 every group | 0/21 (0%) | 0 | 0 | 0 | 0 | 0 | 0 | – |
| anthropic/claude-haiku-5-5 | 1 key | 0/21 (0%) | 0 | 0 | 0 | 0 | 0 | 0 | – |
| anthropic/claude-haiku-5-5 | 2 asked, groups | 0/21 (0%) | 0 | 0 | 0 | 0 | 0 | 0 | – |
| anthropic/claude-haiku-5-5 | 3 asked, single tools | 0/21 (0%) | 0 | 0 | 0 | 0 | 0 | 2 | mother:insert ×2 |
| anthropic/claude-haiku-5-5 | 4 unasked | 0/21 (0%) | 0 | 0 | 0 | 0 | 0 | 0 | – |

## Creating

Asked requests are scenarios 1–6 in conditions 2–3: one `create_chat` is right. In condition 4, a creation on the plain message is one nobody asked for; on the job message Mother decides. Asked first: no creation on the request, and Mother's reply asks a question (often "Should I create it?"). Created at the task: no agent came of the request, and Mother created one when the task came; its groups and prompt are scored above.

| Model | Condition | Requests | Exactly one create_chat | No create_chat | More than one | send_to_chat | Asked first | Created at the task | Plain messages with a creation |
|---|---|---|---|---|---|---|---|---|---|
| anthropic/claude-haiku-5-5 | 2 asked, groups | 18 | 18/18 (100%) | 0/18 (0%) | 0/18 (0%) | 0 | 0/18 (0%) | 0/18 (0%) | – |
| anthropic/claude-haiku-5-5 | 3 asked, single tools | 18 | 18/18 (100%) | 0/18 (0%) | 0/18 (0%) | 0 | 0/18 (0%) | 0/18 (0%) | – |
| anthropic/claude-haiku-5-5 | 4 unasked | 18 | 12/18 (67%) | 6/18 (33%) | 0/18 (0%) | 0 | 3/18 (17%) | 3/18 (17%) | 0/21 (0%) |

The write request with no agent yet (scenario 7): Mother delegated it (create_chat or send_to_chat), the two keywords landed, or she neither delegated nor said she can't (pretended).

| Model | Condition | Runs | Delegated | Landed | Pretended | Mother tried a write |
|---|---|---|---|---|---|---|
| anthropic/claude-haiku-5-5 | 2 asked, groups | 3 | 3/3 (100%) | 3/3 (100%) | 0/3 (0%) | 0/3 (0%) |
| anthropic/claude-haiku-5-5 | 3 asked, single tools | 3 | 2/3 (67%) | 2/3 (67%) | 0/3 (0%) | 2/3 (67%) |
| anthropic/claude-haiku-5-5 | 4 unasked | 3 | 3/3 (100%) | 3/3 (100%) | 0/3 (0%) | 0/3 (0%) |

## Tokens

Prompt tokens per request in the agent's task turns (as the provider reports them), and the estimated tokens of the agent's tool definitions against every chat tool's: what each request saves.

| Model | Condition | Prompt tokens per request | Agent's tools | Every tool | Saved per request | Requests per run |
|---|---|---|---|---|---|---|
| anthropic/claude-haiku-5-5 | 0 every group | 7473 | 3144 | 3648 | 504 | 3.8 |
| anthropic/claude-haiku-5-5 | 1 key | 5069 | 1830 | 3648 | 1818 | 3.3 |
| anthropic/claude-haiku-5-5 | 2 asked, groups | 7945 | 1840 | 3648 | 1808 | 9.7 |
| anthropic/claude-haiku-5-5 | 3 asked, single tools | 7615 | 1588 | 3648 | 2060 | 9.5 |
| anthropic/claude-haiku-5-5 | 4 unasked | 7694 | 1825 | 3648 | 1823 | 10.1 |

## Groups picked

Per scenario, the groups each run's agent got (asked conditions), as `read_data+files`; `none` when no agent got the task.

**anthropic/claude-haiku-5-5, 2 asked, groups**

- reviewer (needs read_data+views_pipelines; never change_data, web): `read_data+views_pipelines`, `views_pipelines`, `read_data+views_pipelines`
- eth (needs web+views_pipelines+change_data; never none): `read_data+change_data+views_pipelines+web`, `read_data+change_data+views_pipelines+web`, `read_data+change_data+views_pipelines+web`
- entry (needs read_data+change_data; never web): `read_data+change_data`, `read_data+change_data`, `read_data+change_data`
- docs (needs read_data+files; never change_data): `read_data+views_pipelines+files`, `read_data+views_pipelines+files`, `read_data+views_pipelines+files`
- research (needs web+files+memory; never change_data): `files+web`, `read_data+files+web`, `read_data+files+web`
- persian (needs read_data+files; never change_data, web): `read_data+files`, `read_data+files`, `read_data+files`
- write (needs change_data; never web): `read_data+change_data`, `read_data+change_data`, `read_data+change_data`

**anthropic/claude-haiku-5-5, 3 asked, single tools**

- reviewer (needs read_data+views_pipelines; never change_data, web): `read_data+views_pipelines`, `read_data+views_pipelines`, `read_data+views_pipelines`
- eth (needs web+views_pipelines+change_data; never none): `read_data+change_data+views_pipelines+web`, `read_data+change_data+views_pipelines+web`, `read_data+change_data+views_pipelines+web`
- entry (needs read_data+change_data; never web): `read_data+change_data`, `read_data+change_data`, `read_data+change_data`
- docs (needs read_data+files; never change_data): `read_data+views_pipelines+files`, `read_data+views_pipelines+files`, `read_data+views_pipelines+files`
- research (needs web+files+memory; never change_data): `read_data+files+web`, `read_data+files+web`, `files+web`
- persian (needs read_data+files; never change_data, web): `read_data+files`, `read_data+files`, `read_data+files`
- write (needs change_data; never web): `read_data+change_data`, `none`, `read_data+change_data`

**anthropic/claude-haiku-5-5, 4 unasked**

- reviewer (needs read_data+views_pipelines; never change_data, web): `read_data+views_pipelines`, `views_pipelines`, `read_data+views_pipelines`
- eth (needs web+views_pipelines+change_data; never none): `read_data+change_data+views_pipelines+web`, `read_data+change_data+views_pipelines`, `read_data+change_data+views_pipelines+web`
- entry (needs read_data+change_data; never web): `read_data+change_data`, `read_data+change_data`, `read_data+change_data`
- docs (needs read_data+files; never change_data): `read_data+views_pipelines+files`, `read_data+views_pipelines+files`, `read_data+views_pipelines+files`
- research (needs web+files+memory; never change_data): `files+web`, `files+web`, `files+web`
- persian (needs read_data+files; never change_data, web): `none`, `none`, `none`
- write (needs change_data; never web): `read_data+change_data`, `read_data+change_data`, `read_data+change_data`

