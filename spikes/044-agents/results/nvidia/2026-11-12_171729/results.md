# SPIKE-044 results

34 runs, 11 with an error (left out below). Conditions: 0 every group, 1 key, 2 asked, groups, 3 asked, single tools, 4 unasked.

## Groups

Of the agent the task went to. Recall: needed groups present; precision: picked groups that are needed; forbidden: runs with a forbidden group. Condition 1 is the key, so it is right by design.

| Model | Condition | Runs | With an agent | Recall | Precision | Forbidden group | Every group |
|---|---|---|---|---|---|---|---|
| nvidia/mistralai/mistral-large-4-1t-a52b | 0 every group | 5 | 5/5 (100%) | 1.00 | 0.40 | 4/5 (80%) | 5/5 (100%) |
| nvidia/mistralai/mistral-large-4-1t-a52b | 1 key | 5 | 5/5 (100%) | 1.00 | 0.95 | 0/5 (0%) | 0/5 (0%) |
| nvidia/mistralai/mistral-large-4-1t-a52b | 2 asked, groups | 5 | 5/5 (100%) | 1.00 | 0.74 | 0/5 (0%) | 0/5 (0%) |
| nvidia/mistralai/mistral-large-4-1t-a52b | 3 asked, single tools | 3 | 3/3 (100%) | 1.00 | 0.78 | 0/3 (0%) | 0/3 (0%) |
| nvidia/mistralai/mistral-large-4-1t-a52b | 4 unasked | 5 | 5/5 (100%) | 0.93 | 0.72 | 0/5 (0%) | 0/5 (0%) |

## Prompt

The agent's custom prompt, by keyword checks (see `score.go`): states the job, says what to report, repeats a core rule, at most 500 tokens (the app's estimate), in the user's language. For the Persian request, also whether the prompt is in Persian or at least names Persian as the language to answer in.

| Model | Condition | Prompts | Job | Report | Repeats a core rule | ≤ 500 tokens | Mean tokens | Language | Persian request: Persian prompt | Persian request: Persian or names it |
|---|---|---|---|---|---|---|---|---|---|---|
| nvidia/mistralai/mistral-large-4-1t-a52b | 0 every group | 5 | 5/5 (100%) | 5/5 (100%) | 0/5 (0%) | 5/5 (100%) | 95 | 5/5 (100%) | – | – |
| nvidia/mistralai/mistral-large-4-1t-a52b | 1 key | 5 | 5/5 (100%) | 5/5 (100%) | 0/5 (0%) | 5/5 (100%) | 95 | 5/5 (100%) | – | – |
| nvidia/mistralai/mistral-large-4-1t-a52b | 2 asked, groups | 5 | 5/5 (100%) | 5/5 (100%) | 3/5 (60%) | 5/5 (100%) | 285 | 5/5 (100%) | – | – |
| nvidia/mistralai/mistral-large-4-1t-a52b | 3 asked, single tools | 3 | 3/3 (100%) | 3/3 (100%) | 2/3 (67%) | 3/3 (100%) | 303 | 3/3 (100%) | – | – |
| nvidia/mistralai/mistral-large-4-1t-a52b | 4 unasked | 5 | 4/5 (80%) | 5/5 (100%) | 1/5 (20%) | 5/5 (100%) | 197 | 5/5 (100%) | – | – |

## Task

The follow-up task's check, per scenario (passes / runs).

| Model | Scenario | 0 every group | 1 key | 2 asked, groups | 3 asked, single tools | 4 unasked |
|---|---|---|---|---|---|---|
| nvidia/mistralai/mistral-large-4-1t-a52b | reviewer | 1/1 | 1/1 | 1/1 | 1/1 | 1/1 |
| nvidia/mistralai/mistral-large-4-1t-a52b | eth | 1/1 | 1/1 | 1/1 | – | 1/1 |
| nvidia/mistralai/mistral-large-4-1t-a52b | entry | 1/1 | 1/1 | 1/1 | 1/1 | 1/1 |
| nvidia/mistralai/mistral-large-4-1t-a52b | docs | 1/1 | 1/1 | 1/1 | 1/1 | 1/1 |
| nvidia/mistralai/mistral-large-4-1t-a52b | research | 1/1 | 1/1 | 1/1 | – | 1/1 |
| nvidia/mistralai/mistral-large-4-1t-a52b | persian | – | – | – | – | – |
| nvidia/mistralai/mistral-large-4-1t-a52b | write | – | – | – | – | – |
| nvidia/mistralai/mistral-large-4-1t-a52b | **all** | **5/5 (100%)** | **5/5 (100%)** | **5/5 (100%)** | **3/3 (100%)** | **5/5 (100%)** |

## Refused calls

Calls to a tool outside the chat's tools. After: what the agent did after its first refusal in a run: told the user it can't, asked, called the tool again, or went on another way without saying so.

| Model | Condition | Agent runs with a refusal | Agent refusals | Told | Asked | Retried | Worked around | Mother's refusals | Tools refused |
|---|---|---|---|---|---|---|---|---|---|
| nvidia/mistralai/mistral-large-4-1t-a52b | 0 every group | 0/5 (0%) | 0 | 0 | 0 | 0 | 0 | 0 | – |
| nvidia/mistralai/mistral-large-4-1t-a52b | 1 key | 0/5 (0%) | 0 | 0 | 0 | 0 | 0 | 0 | – |
| nvidia/mistralai/mistral-large-4-1t-a52b | 2 asked, groups | 0/5 (0%) | 0 | 0 | 0 | 0 | 0 | 0 | – |
| nvidia/mistralai/mistral-large-4-1t-a52b | 3 asked, single tools | 0/3 (0%) | 0 | 0 | 0 | 0 | 0 | 0 | – |
| nvidia/mistralai/mistral-large-4-1t-a52b | 4 unasked | 0/5 (0%) | 0 | 0 | 0 | 0 | 0 | 0 | – |

## Creating

Asked requests are scenarios 1–6 in conditions 2–3: one `create_chat` is right. In condition 4, a creation on the plain message is one nobody asked for; on the job message Mother decides. Asked first: no creation on the request, and Mother's reply asks a question (often "Should I create it?"). Created at the task: no agent came of the request, and Mother created one when the task came; its groups and prompt are scored above.

| Model | Condition | Requests | Exactly one create_chat | No create_chat | More than one | send_to_chat | Asked first | Created at the task | Plain messages with a creation |
|---|---|---|---|---|---|---|---|---|---|
| nvidia/mistralai/mistral-large-4-1t-a52b | 2 asked, groups | 5 | 5/5 (100%) | 0/5 (0%) | 0/5 (0%) | 0 | 0/5 (0%) | 0/5 (0%) | – |
| nvidia/mistralai/mistral-large-4-1t-a52b | 3 asked, single tools | 3 | 3/3 (100%) | 0/3 (0%) | 0/3 (0%) | 0 | 0/3 (0%) | 0/3 (0%) | – |
| nvidia/mistralai/mistral-large-4-1t-a52b | 4 unasked | 5 | 4/5 (80%) | 1/5 (20%) | 0/5 (0%) | 0 | 0/5 (0%) | 1/5 (20%) | 1/5 (20%) |

The write request with no agent yet (scenario 7): Mother delegated it (create_chat or send_to_chat), the two keywords landed, or she neither delegated nor said she can't (pretended).

| Model | Condition | Runs | Delegated | Landed | Pretended | Mother tried a write |
|---|---|---|---|---|---|---|

## Tokens

Prompt tokens per request in the agent's task turns (as the provider reports them), and the estimated tokens of the agent's tool definitions against every chat tool's: what each request saves.

| Model | Condition | Prompt tokens per request | Agent's tools | Every tool | Saved per request | Requests per run |
|---|---|---|---|---|---|---|
| nvidia/mistralai/mistral-large-4-1t-a52b | 0 every group | 5057 | 3144 | 3648 | 504 | 5.2 |
| nvidia/mistralai/mistral-large-4-1t-a52b | 1 key | 4392 | 1912 | 3648 | 1736 | 6.6 |
| nvidia/mistralai/mistral-large-4-1t-a52b | 2 asked, groups | 6921 | 2166 | 3648 | 1482 | 17.6 |
| nvidia/mistralai/mistral-large-4-1t-a52b | 3 asked, single tools | 4373 | 1549 | 3648 | 2099 | 9.0 |
| nvidia/mistralai/mistral-large-4-1t-a52b | 4 unasked | 6642 | 2137 | 3648 | 1511 | 18.8 |

## Groups picked

Per scenario, the groups each run's agent got (asked conditions), as `read_data+files`; `none` when no agent got the task.

**nvidia/mistralai/mistral-large-4-1t-a52b, 2 asked, groups**

- reviewer (needs read_data+views_pipelines; never change_data, web): `read_data+views_pipelines+files`
- eth (needs web+views_pipelines+change_data; never none): `read_data+change_data+views_pipelines+files+web`
- entry (needs read_data+change_data; never web): `read_data+change_data`
- docs (needs read_data+files; never change_data): `read_data+views_pipelines+files`
- research (needs web+files+memory; never change_data): `read_data+files+web+memory`

**nvidia/mistralai/mistral-large-4-1t-a52b, 3 asked, single tools**

- reviewer (needs read_data+views_pipelines; never change_data, web): `read_data+views_pipelines+files`
- entry (needs read_data+change_data; never web): `read_data+change_data`
- docs (needs read_data+files; never change_data): `read_data+views_pipelines+files`

**nvidia/mistralai/mistral-large-4-1t-a52b, 4 unasked**

- reviewer (needs read_data+views_pipelines; never change_data, web): `read_data+views_pipelines+files`
- eth (needs web+views_pipelines+change_data; never none): `read_data+change_data+views_pipelines+files+web`
- entry (needs read_data+change_data; never web): `read_data+change_data`
- docs (needs read_data+files; never change_data): `read_data+views_pipelines+files`
- research (needs web+files+memory; never change_data): `read_data+files+web`

## Errors

- nvidia-mistralai-mistral-large-4-1t-a52b_persian_c1_r1: the turn failed: [the turn stopped: nvidia refused the request: <html> <head><title>403 Forbidden</title></head> <body> <center><h1>403 Forbidden</h1></center> </body> </html>. Check the provider's settings, or pick another model.]
- nvidia-mistralai-mistral-large-4-1t-a52b_persian_c2_r1: the turn failed: [the turn stopped: nvidia refused the request: <html> <head><title>403 Forbidden</title></head> <body> <center><h1>403 Forbidden</h1></center> </body> </html>. Check the provider's settings, or pick another model.]
- nvidia-mistralai-mistral-large-4-1t-a52b_persian_c3_r1: the turn failed: [the turn stopped: nvidia refused the request: <html> <head><title>403 Forbidden</title></head> <body> <center><h1>403 Forbidden</h1></center> </body> </html>. Check the provider's settings, or pick another model.]
- nvidia-mistralai-mistral-large-4-1t-a52b_persian_c4_r1: the turn failed: [the turn stopped: nvidia refused the request: <html> <head><title>403 Forbidden</title></head> <body> <center><h1>403 Forbidden</h1></center> </body> </html>. Check the provider's settings, or pick another model.]
- nvidia-mistralai-mistral-large-4-1t-a52b_write_c1_r1: the turn failed: [the turn stopped: nvidia refused the request: <html> <head><title>403 Forbidden</title></head> <body> <center><h1>403 Forbidden</h1></center> </body> </html>. Check the provider's settings, or pick another model.]
- nvidia-mistralai-mistral-large-4-1t-a52b_write_c2_r1: the turn failed: [the turn stopped: nvidia refused the request: <html> <head><title>403 Forbidden</title></head> <body> <center><h1>403 Forbidden</h1></center> </body> </html>. Check the provider's settings, or pick another model.]
- nvidia-mistralai-mistral-large-4-1t-a52b_write_c3_r1: the turn failed: [the turn stopped: nvidia refused the request: <html> <head><title>403 Forbidden</title></head> <body> <center><h1>403 Forbidden</h1></center> </body> </html>. Check the provider's settings, or pick another model.]
- nvidia-mistralai-mistral-large-4-1t-a52b_write_c4_r1: the turn failed: [the turn stopped: nvidia refused the request: <html> <head><title>403 Forbidden</title></head> <body> <center><h1>403 Forbidden</h1></center> </body> </html>. Check the provider's settings, or pick another model.]
- nvidia-mistralai-mistral-large-4-1t-a52b_eth_c3_r1: the run took longer than 25m0s
- nvidia-mistralai-mistral-large-4-1t-a52b_research_c3_r1: the run took longer than 25m0s
- nvidia-mistralai-mistral-large-4-1t-a52b_persian_c0_r1: the run took longer than 25m0s
