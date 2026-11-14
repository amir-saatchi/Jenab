# SPIKE-044 results

42 runs, 0 with an error (left out below). Conditions: 2 asked, groups, 4 unasked.

## Groups

Of the agent the task went to. Recall: needed groups present; precision: picked groups that are needed; forbidden: runs with a forbidden group. Condition 1 is the key, so it is right by design.

| Model | Condition | Runs | With an agent | Recall | Precision | Forbidden group | Every group |
|---|---|---|---|---|---|---|---|
| anthropic/claude-haiku-5-5 | 2 asked, groups | 21 | 21/21 (100%) | 0.95 | 0.89 | 0/21 (0%) | 0/21 (0%) |
| anthropic/claude-haiku-5-5 | 4 unasked | 21 | 18/21 (86%) | 0.94 | 0.88 | 0/18 (0%) | 0/18 (0%) |

## Prompt

The agent's custom prompt, by keyword checks (see `score.go`): states the job, says what to report, repeats a core rule, at most 500 tokens (the app's estimate), in the user's language. For the Persian request, also whether the prompt is in Persian or at least names Persian as the language to answer in.

| Model | Condition | Prompts | Job | Report | Repeats a core rule | ≤ 500 tokens | Mean tokens | Language | Persian request: Persian prompt | Persian request: Persian or names it |
|---|---|---|---|---|---|---|---|---|---|---|
| anthropic/claude-haiku-5-5 | 2 asked, groups | 21 | 21/21 (100%) | 21/21 (100%) | 6/21 (29%) | 21/21 (100%) | 233 | 21/21 (100%) | 3/3 (100%) | 3/3 (100%) |
| anthropic/claude-haiku-5-5 | 4 unasked | 18 | 18/18 (100%) | 18/18 (100%) | 5/18 (28%) | 18/18 (100%) | 179 | 18/18 (100%) | – | – |

## Task

The follow-up task's check, per scenario (passes / runs).

| Model | Scenario | 2 asked, groups | 4 unasked |
|---|---|---|---|
| anthropic/claude-haiku-5-5 | reviewer | 3/3 | 3/3 |
| anthropic/claude-haiku-5-5 | eth | 3/3 | 3/3 |
| anthropic/claude-haiku-5-5 | entry | 3/3 | 3/3 |
| anthropic/claude-haiku-5-5 | docs | 3/3 | 3/3 |
| anthropic/claude-haiku-5-5 | research | 3/3 | 3/3 |
| anthropic/claude-haiku-5-5 | persian | 3/3 | 3/3 |
| anthropic/claude-haiku-5-5 | write | 3/3 | 3/3 |
| anthropic/claude-haiku-5-5 | **all** | **21/21 (100%)** | **21/21 (100%)** |

## Refused calls

Calls to a tool outside the chat's tools. After: what the agent did after its first refusal in a run: told the user it can't, asked, called the tool again, or went on another way without saying so.

| Model | Condition | Agent runs with a refusal | Agent refusals | Told | Asked | Retried | Worked around | Mother's refusals | Tools refused |
|---|---|---|---|---|---|---|---|---|---|
| anthropic/claude-haiku-5-5 | 2 asked, groups | 0/21 (0%) | 0 | 0 | 0 | 0 | 0 | 0 | – |
| anthropic/claude-haiku-5-5 | 4 unasked | 0/21 (0%) | 0 | 0 | 0 | 0 | 0 | 0 | – |

## Creating

Asked requests are scenarios 1–6 in conditions 2–3: one `create_chat` is right. In condition 4, a creation on the plain message is one nobody asked for; on the job message Mother decides. Asked first: no creation on the request, and Mother's reply asks a question (often "Should I create it?"). Created at the task: no agent came of the request, and Mother created one when the task came; its groups and prompt are scored above.

| Model | Condition | Requests | Exactly one create_chat | No create_chat | More than one | send_to_chat | Asked first | Created at the task | Plain messages with a creation |
|---|---|---|---|---|---|---|---|---|---|
| anthropic/claude-haiku-5-5 | 2 asked, groups | 18 | 18/18 (100%) | 0/18 (0%) | 0/18 (0%) | 0 | 0/18 (0%) | 0/18 (0%) | – |
| anthropic/claude-haiku-5-5 | 4 unasked | 18 | 13/18 (72%) | 5/18 (28%) | 0/18 (0%) | 0 | 3/18 (17%) | 2/18 (11%) | 0/21 (0%) |

The write request with no agent yet (scenario 7): Mother delegated it (create_chat or send_to_chat), the two keywords landed, or she neither delegated nor said she can't (pretended).

| Model | Condition | Runs | Delegated | Landed | Pretended | Mother tried a write |
|---|---|---|---|---|---|---|
| anthropic/claude-haiku-5-5 | 2 asked, groups | 3 | 3/3 (100%) | 3/3 (100%) | 0/3 (0%) | 0/3 (0%) |
| anthropic/claude-haiku-5-5 | 4 unasked | 3 | 3/3 (100%) | 3/3 (100%) | 0/3 (0%) | 0/3 (0%) |

## Tokens

Prompt tokens per request in the agent's task turns (as the provider reports them), and the estimated tokens of the agent's tool definitions against every chat tool's: what each request saves.

| Model | Condition | Prompt tokens per request | Agent's tools | Every tool | Saved per request | Requests per run |
|---|---|---|---|---|---|---|
| anthropic/claude-haiku-5-5 | 2 asked, groups | 7714 | 1788 | 3648 | 1860 | 9.2 |
| anthropic/claude-haiku-5-5 | 4 unasked | 7822 | 1797 | 3648 | 1851 | 10.3 |

## Groups picked

Per scenario, the groups each run's agent got (asked conditions), as `read_data+files`; `none` when no agent got the task.

**anthropic/claude-haiku-5-5, 2 asked, groups**

- reviewer (needs read; never change_data, web): `read`, `read`, `read`
- eth (needs web+build_run+change_data; never none): `read+change_data+build_run+web`, `read+change_data+build_run+web`, `read+change_data+build_run+web`
- entry (needs read+change_data; never web): `read+change_data`, `read+change_data`, `read+change_data`
- docs (needs read+files; never change_data): `read+files`, `read+files`, `read+files`
- research (needs web+files+memory; never change_data): `files+web`, `files+web`, `files+web`
- persian (needs read+files; never change_data, web): `read+files`, `read+files`, `read+files`
- write (needs change_data; never web): `read+change_data`, `read+change_data`, `read+change_data`

**anthropic/claude-haiku-5-5, 4 unasked**

- reviewer (needs read; never change_data, web): `read`, `read`, `read`
- eth (needs web+build_run+change_data; never none): `read+change_data+build_run+web`, `read+change_data+build_run+web`, `read+change_data+build_run+web`
- entry (needs read+change_data; never web): `read+change_data`, `read+change_data`, `read+change_data`
- docs (needs read+files; never change_data): `read+files`, `read+files`, `read+files`
- research (needs web+files+memory; never change_data): `files+web`, `files+web`, `files+web`
- persian (needs read+files; never change_data, web): `none`, `none`, `none`
- write (needs change_data; never web): `read+change_data`, `read+change_data`, `read+change_data`

