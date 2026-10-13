# SPIKE-029 results

36 runs, prompt rules v2, v3, v4. Conditions: 1 baseline, 2 tool, 3 tool + index, 4 tool + index + the app's check. v2 is round 2's rule; v3 makes a subject one goal, with notes and merge_subjects; v4 is v3 with open items instead of a status.

## Keeping the subjects

| model | condition | runs | work turns with a subject, on their own | with the app's check | nudges | subjects at the end, per run | made for questions | changes that updated the existing subject | duplicates | work in one subject after the merge message | merges made | subject calls alone in their response |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| ollama-cloud/gemma4:31b | 3 tool+index · v2 | 6 | 50/54 (93%) | 50/54 (93%) | 0 | 5.8 | 0 | 17/18 (94%) | 0 | 0/6 (0%) | 0 | 14/61 (23%) |
| ollama-cloud/gemma4:31b | 3 tool+index · v3 | 6 | 51/54 (94%) | 51/54 (94%) | 0 | 1.8 | 0 | 17/18 (94%) | 0 | 5/6 (83%) | 4 | 16/62 (26%) |
| ollama-cloud/gemma4:31b | 3 tool+index · v4 | 6 | 53/54 (98%) | 53/54 (98%) | 0 | 1.7 | 0 | 17/18 (94%) | 0 | 5/6 (83%) | 0 | 19/60 (32%) |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v2 | 6 | 49/54 (91%) | 49/54 (91%) | 0 | 7.3 | 0 | 13/17 (76%) | 1 | 0/6 (0%) | 0 | 63/63 (100%) |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v3 | 6 | 52/54 (96%) | 52/54 (96%) | 0 | 1.7 | 0 | 12/17 (71%) | 3 | 6/6 (100%) | 7 | 62/62 (100%) |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v4 | 6 | 51/54 (94%) | 51/54 (94%) | 0 | 1.5 | 0 | 18/18 (100%) | 0 | 6/6 (100%) | 6 | 61/61 (100%) |

## Correctness and payoff

| model | condition | subject status right | waiting work shown | follow-up answers right | calls that needed earlier turns right | history reads per run | get_subject per run |
|---|---|---|---|---|---|---|---|
| ollama-cloud/gemma4:31b | 3 tool+index · v2 | 25/27 (93%) | 6/6 (100%) | 12/12 (100%) | 18/18 (100%) | 4.7 | 0.7 |
| ollama-cloud/gemma4:31b | 3 tool+index · v3 | 16/27 (59%) | 6/6 (100%) | 12/12 (100%) | 18/18 (100%) | 3.8 | 1.7 |
| ollama-cloud/gemma4:31b | 3 tool+index · v4 | – | 6/6 (100%) | 12/12 (100%) | 17/18 (94%) | 3.7 | 0.7 |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v2 | 18/27 (67%) | 5/6 (83%) | 11/12 (92%) | 15/18 (83%) | 1.2 | 0.8 |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v3 | 11/27 (41%) | 6/6 (100%) | 8/12 (67%) | 15/18 (83%) | 1.5 | 0.8 |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v4 | – | 6/6 (100%) | 7/12 (58%) | 17/18 (94%) | 1.7 | 1.5 |

## Cost

| model | condition | requests per run | prompt tokens per run | output tokens per run | request errors | runs that ended early |
|---|---|---|---|---|---|---|
| ollama-cloud/gemma4:31b | 3 tool+index · v2 | 36.7 | 124561.7 | 7928.5 | 0 | 0 |
| ollama-cloud/gemma4:31b | 3 tool+index · v3 | 36.3 | 132402.8 | 8745.0 | 0 | 0 |
| ollama-cloud/gemma4:31b | 3 tool+index · v4 | 36.8 | 138505.8 | 8852.0 | 0 | 0 |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v2 | 44.7 | 112070.8 | 6759.0 | 2 | 0 |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v3 | 44.0 | 124775.0 | 6766.7 | 0 | 0 |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v4 | 43.2 | 130962.2 | 7023.2 | 0 | 0 |

## Merges

| model | condition | scenario | rep | message | result |
|---|---|---|---|---|---|
| ollama-cloud/gemma4:31b | 3 tool+index · v3 | crypto | 1 | 10 · merge | merged s2 (Bitcoin prices data collection) + s1 (Bitcoin prices table setup) + s3 (BTC price drop alerts) + s4 (Bitcoin price visualization) + s5 (Ethereum prices data collection) into s2: Crypto price tracking · in_progress |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v3 | crypto | 1 | 10 · merge | merged s1 (Bitcoin price data) + s2 (Ethereum price data) into s1: Crypto price tracking · in_progress |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v4 | crypto | 1 | 10 · merge | merged s1 (Bitcoin price table) + s2 (Ethereum price data) into s1: Crypto price tracking · 1 open |
| ollama-cloud/gemma4:31b | 3 tool+index · v3 | jobs | 1 | 11 · merge | merged s1 (Job applications tracking) + s2 (Application follow-up automation) into s1: Friday review · in_progress |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v3 | jobs | 1 | 11 · merge | merged s1 (Job applications tracker) + s3 (Follow-up reminders) into s1: Friday review · in_progress |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v4 | jobs | 1 | 11 · merge | merged s1 (Job applications tracking) + s2 (Application follow‑up monitoring) into s1: Friday review · nothing open |
| ollama-cloud/gemma4:31b | 3 tool+index · v3 | crypto | 2 | 10 · merge | merged s1 (Bitcoin prices table setup) + s3 (Ethereum prices table setup) into s1: Crypto price tracking · done |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v3 | crypto | 2 | 7 · change | merged s2 (Daily BTC price pipeline) + s3 (Daily BTC price pipeline) into s2: Daily BTC price pipeline · in_progress |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v3 | crypto | 2 | 10 · merge | merged s1 (Bitcoin price table) + s2 (Daily BTC price pipeline) + s4 (BTC drop alert) + s5 (BTC chart view) + s6 (Ethereum price table) + s7 (Ethereum price pipeline) + s8 (Ethereum chart view) into s1: Crypto price tracking · in_progress |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v4 | crypto | 2 | 10 · merge | merged s1 (Bitcoin price table) + s2 (Daily BTC price pipeline) + s3 (Daily BTC price pipeline) + s4 (BTC drop alert) + s5 (BTC closing price chart) + s6 (Ethereum price table) + s7 (Daily ETH price pipeline) + s8 (ETH closing price chart) into s1: Crypto price tracking · 1 open |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v3 | jobs | 2 | 11 · merge | merged s1 (Job applications table) + s3 (Application follow‑up pipeline) into s1: Friday review · open |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v4 | jobs | 2 | 11 · merge | merged s1 (Job applications tracking) + s2 (Application follow‑up automation) into s1: Friday review · nothing open |
| ollama-cloud/gemma4:31b | 3 tool+index · v3 | crypto | 3 | 10 · merge | merged s1 (Bitcoin price tracking) + s2 (Ethereum price tracking) into s1: Crypto price tracking · in_progress |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v3 | crypto | 3 | 10 · merge | merged s1 (Bitcoin price table) + s2 (Bitcoin price collection) + s3 (Bitcoin price chart) + s4 (Ethereum price collection) into s1: Crypto price tracking · in_progress |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v4 | crypto | 3 | 10 · merge | merged s1 (Bitcoin price table) + s2 (Bitcoin price pipeline) + s3 (Bitcoin price drop alert) + s4 (Bitcoin price chart) + s5 (Ethereum price table) + s6 (Ethereum price pipeline) + s7 (Ethereum price chart) into s1: Crypto price tracking · 1 open |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v3 | jobs | 3 | 11 · merge | merged s2 (Zalando data job monitoring) + s1 (Job applications) into s2: Friday review · open |
| ollama-cloud/gpt-oss:120b | 3 tool+index · v4 | jobs | 3 | 11 · merge | merged s1 (Job applications tracking) + s2 (Follow-up pipeline) into s1: Friday review · nothing open |

## Checks one by one

Passes out of runs, per condition.

### ollama-cloud/gemma4:31b

| scenario | # | check | message | 3 tool+index · v2 | 3 tool+index · v3 | 3 tool+index · v4 |
|---|---|---|---|---|---|---|
| crypto | 3 | status | Show all prices in EUR, not USD. I'm in Germany and compare … | 3/3 | 1/3 | – |
| crypto | 4 | status | I want an alert when BTC drops more than 5% in a day. Don't … | 3/3 | 2/3 | – |
| crypto | 4 | waiting | I want an alert when BTC drops more than 5% in a day. Don't … | 3/3 | 3/3 | 3/3 |
| crypto | 7 | call | Change daily_btc to run at 07:00 instead, and skip weekends:… | 3/3 | 3/3 | 3/3 |
| crypto | 7 | status | Change daily_btc to run at 07:00 instead, and skip weekends:… | 3/3 | 1/3 | – |
| crypto | 8 | call | Now do the same for Ethereum: a table, a pipeline and a char… | 3/3 | 3/3 | 3/3 |
| crypto | 10 | merge | Bitcoin and Ethereum are one project for me: my crypto price… | 0/3 | 3/3 | 3/3 |
| crypto | 11 | call | About the alert I mentioned earlier: send it to Telegram. Se… | 3/3 | 3/3 | 2/3 |
| crypto | 11 | status | About the alert I mentioned earlier: send it to Telegram. Se… | 3/3 | 3/3 | – |
| crypto | 13 | answer | Why did we stop collecting weekend prices? | 3/3 | 3/3 | 3/3 |
| crypto | 14 | status | Actually, drop the Telegram alert. I'll use my exchange's ow… | 3/3 | 1/3 | – |
| crypto | 15 | answer | Give me a short list of what we set up in this chat, and wha… | 3/3 | 3/3 | 3/3 |
| jobs | 3 | status | From now on, when I say a company rejected me, set the statu… | 1/3 | 1/3 | – |
| jobs | 6 | status | I also want a pipeline that checks Zalando's careers page ev… | 3/3 | 3/3 | – |
| jobs | 6 | waiting | I also want a pipeline that checks Zalando's careers page ev… | 3/3 | 3/3 | 3/3 |
| jobs | 9 | call | N26 rejected me. | 3/3 | 3/3 | 3/3 |
| jobs | 10 | call | Make follow_up run on Fridays instead, and use 10 days inste… | 3/3 | 3/3 | 3/3 |
| jobs | 10 | status | Make follow_up run on Fridays instead, and use 10 days inste… | 3/3 | 1/3 | – |
| jobs | 11 | merge | follow_up and the board are really one thing for me: my Frid… | 0/3 | 2/3 | 2/3 |
| jobs | 12 | call | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | 3/3 | 3/3 | 3/3 |
| jobs | 12 | status | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | 3/3 | 3/3 | – |
| jobs | 14 | answer | What happened with my N26 application, and why does it say c… | 3/3 | 3/3 | 3/3 |
| jobs | 15 | answer | What have we set up so far, and is anything still waiting on… | 3/3 | 3/3 | 3/3 |

### ollama-cloud/gpt-oss:120b

| scenario | # | check | message | 3 tool+index · v2 | 3 tool+index · v3 | 3 tool+index · v4 |
|---|---|---|---|---|---|---|
| crypto | 3 | status | Show all prices in EUR, not USD. I'm in Germany and compare … | 3/3 | 0/3 | – |
| crypto | 4 | status | I want an alert when BTC drops more than 5% in a day. Don't … | 3/3 | 1/3 | – |
| crypto | 4 | waiting | I want an alert when BTC drops more than 5% in a day. Don't … | 3/3 | 3/3 | 3/3 |
| crypto | 7 | call | Change daily_btc to run at 07:00 instead, and skip weekends:… | 3/3 | 3/3 | 3/3 |
| crypto | 7 | status | Change daily_btc to run at 07:00 instead, and skip weekends:… | 3/3 | 0/3 | – |
| crypto | 8 | call | Now do the same for Ethereum: a table, a pipeline and a char… | 3/3 | 3/3 | 3/3 |
| crypto | 10 | merge | Bitcoin and Ethereum are one project for me: my crypto price… | 0/3 | 3/3 | 3/3 |
| crypto | 11 | call | About the alert I mentioned earlier: send it to Telegram. Se… | 1/3 | 3/3 | 3/3 |
| crypto | 11 | status | About the alert I mentioned earlier: send it to Telegram. Se… | 1/3 | 3/3 | – |
| crypto | 13 | answer | Why did we stop collecting weekend prices? | 2/3 | 2/3 | 1/3 |
| crypto | 14 | status | Actually, drop the Telegram alert. I'll use my exchange's ow… | 2/3 | 0/3 | – |
| crypto | 15 | answer | Give me a short list of what we set up in this chat, and wha… | 3/3 | 3/3 | 3/3 |
| jobs | 3 | status | From now on, when I say a company rejected me, set the statu… | 1/3 | 2/3 | – |
| jobs | 6 | status | I also want a pipeline that checks Zalando's careers page ev… | 2/3 | 3/3 | – |
| jobs | 6 | waiting | I also want a pipeline that checks Zalando's careers page ev… | 2/3 | 3/3 | 3/3 |
| jobs | 9 | call | N26 rejected me. | 3/3 | 3/3 | 3/3 |
| jobs | 10 | call | Make follow_up run on Fridays instead, and use 10 days inste… | 3/3 | 3/3 | 3/3 |
| jobs | 10 | status | Make follow_up run on Fridays instead, and use 10 days inste… | 1/3 | 2/3 | – |
| jobs | 11 | merge | follow_up and the board are really one thing for me: my Frid… | 0/3 | 3/3 | 3/3 |
| jobs | 12 | call | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | 2/3 | 0/3 | 2/3 |
| jobs | 12 | status | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | 2/3 | 0/3 | – |
| jobs | 14 | answer | What happened with my N26 application, and why does it say c… | 3/3 | 3/3 | 3/3 |
| jobs | 15 | answer | What have we set up so far, and is anything still waiting on… | 3/3 | 0/3 | 0/3 |

