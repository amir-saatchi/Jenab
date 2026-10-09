# SPIKE-029 results

48 runs, prompt rule v1. Conditions: 1 baseline, 2 tool, 3 tool + index, 4 tool + index + the app's check.

## Keeping the subjects

| model | condition | runs | work turns with a subject, on their own | with the app's check | nudges | subjects at the end, per run | made for questions | changes that updated the existing subject | duplicates | update_subject alone in its response |
|---|---|---|---|---|---|---|---|---|---|---|
| ollama-cloud/gemma4:31b | 2 tool | 6 | 50/54 (93%) | 50/54 (93%) | 0 | 7.5 | 0 | 5/18 (28%) | 12 | 7/52 (13%) |
| ollama-cloud/gemma4:31b | 3 tool+index | 6 | 51/54 (94%) | 51/54 (94%) | 0 | 6.5 | 0 | 11/18 (61%) | 5 | 10/52 (19%) |
| ollama-cloud/gemma4:31b | 4 tool+index+check | 6 | 47/54 (87%) | 53/54 (98%) | 8 | 7.8 | 2 | 8/18 (44%) | 9 | 19/57 (33%) |
| ollama-cloud/gpt-oss:120b | 2 tool | 6 | 49/54 (91%) | 49/54 (91%) | 0 | 7.2 | 1 | 4/17 (24%) | 11 | 50/50 (100%) |
| ollama-cloud/gpt-oss:120b | 3 tool+index | 6 | 45/54 (83%) | 45/54 (83%) | 0 | 6.7 | 0 | 7/17 (41%) | 7 | 47/47 (100%) |
| ollama-cloud/gpt-oss:120b | 4 tool+index+check | 6 | 50/54 (93%) | 52/54 (96%) | 2 | 8.7 | 0 | 6/16 (38%) | 10 | 58/58 (100%) |

## Correctness and payoff

| model | condition | subject status right | follow-up answers right | calls that needed earlier turns right | history reads per run | get_subject per run |
|---|---|---|---|---|---|---|
| ollama-cloud/gemma4:31b | 1 baseline | – | 9/12 (75%) | 18/18 (100%) | 3.2 | 0.0 |
| ollama-cloud/gemma4:31b | 2 tool | 21/27 (78%) | 12/12 (100%) | 18/18 (100%) | 4.7 | 0.8 |
| ollama-cloud/gemma4:31b | 3 tool+index | 26/27 (96%) | 12/12 (100%) | 18/18 (100%) | 3.8 | 1.3 |
| ollama-cloud/gemma4:31b | 4 tool+index+check | 26/27 (96%) | 12/12 (100%) | 17/18 (94%) | 3.7 | 0.7 |
| ollama-cloud/gpt-oss:120b | 1 baseline | – | 10/12 (83%) | 15/18 (83%) | 0.5 | 0.0 |
| ollama-cloud/gpt-oss:120b | 2 tool | 21/27 (78%) | 10/12 (83%) | 15/18 (83%) | 1.0 | 0.7 |
| ollama-cloud/gpt-oss:120b | 3 tool+index | 20/27 (74%) | 11/12 (92%) | 15/18 (83%) | 1.7 | 0.5 |
| ollama-cloud/gpt-oss:120b | 4 tool+index+check | 18/27 (67%) | 11/12 (92%) | 16/18 (89%) | 0.8 | 0.5 |

## Cost

| model | condition | requests per run | prompt tokens per run | output tokens per run | request errors | runs that ended early |
|---|---|---|---|---|---|---|
| ollama-cloud/gemma4:31b | 1 baseline | 30.2 | 75070.8 | 7102.2 | 1 | 0 |
| ollama-cloud/gemma4:31b | 2 tool | 32.5 | 105118.5 | 8218.7 | 0 | 0 |
| ollama-cloud/gemma4:31b | 3 tool+index | 32.3 | 96999.0 | 7624.8 | 0 | 0 |
| ollama-cloud/gemma4:31b | 4 tool+index+check | 35.0 | 103936.3 | 7819.0 | 1 | 0 |
| ollama-cloud/gpt-oss:120b | 1 baseline | 31.5 | 59535.3 | 5080.3 | 0 | 0 |
| ollama-cloud/gpt-oss:120b | 2 tool | 39.3 | 90913.2 | 5650.0 | 0 | 0 |
| ollama-cloud/gpt-oss:120b | 3 tool+index | 39.7 | 95101.2 | 5933.5 | 2 | 0 |
| ollama-cloud/gpt-oss:120b | 4 tool+index+check | 40.7 | 93795.3 | 5730.8 | 1 | 0 |

## Checks one by one

Passes out of runs, per condition.

### ollama-cloud/gemma4:31b

| scenario | # | check | message | 1 | 2 | 3 | 4 |
|---|---|---|---|---|---|---|---|
| crypto | 3 | status | Show all prices in EUR, not USD. I'm in Germany and compare … | – | 3/3 | 3/3 | 3/3 |
| crypto | 4 | status | I want an alert when BTC drops more than 5% in a day. Don't … | – | 3/3 | 3/3 | 3/3 |
| crypto | 7 | call | Change daily_btc to run at 07:00 instead, and skip weekends:… | 3/3 | 3/3 | 3/3 | 3/3 |
| crypto | 7 | status | Change daily_btc to run at 07:00 instead, and skip weekends:… | – | 3/3 | 3/3 | 3/3 |
| crypto | 8 | call | Now do the same for Ethereum: a table, a pipeline and a char… | 3/3 | 3/3 | 3/3 | 3/3 |
| crypto | 10 | call | About the alert I mentioned earlier: send it to Telegram. Se… | 3/3 | 3/3 | 3/3 | 2/3 |
| crypto | 10 | status | About the alert I mentioned earlier: send it to Telegram. Se… | – | 2/3 | 3/3 | 2/3 |
| crypto | 12 | answer | Why did we stop collecting weekend prices? | 2/3 | 3/3 | 3/3 | 3/3 |
| crypto | 13 | status | Actually, drop the Telegram alert. I'll use my exchange's ow… | – | 3/3 | 3/3 | 3/3 |
| crypto | 14 | answer | Give me a short list of what we set up in this chat, and wha… | 1/3 | 3/3 | 3/3 | 3/3 |
| jobs | 3 | status | From now on, when I say a company rejected me, set the statu… | – | 1/3 | 2/3 | 3/3 |
| jobs | 6 | status | I also want a pipeline that checks Zalando's careers page ev… | – | 3/3 | 3/3 | 3/3 |
| jobs | 9 | call | N26 rejected me. | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 10 | call | Make follow_up run on Fridays instead, and use 10 days inste… | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 10 | status | Make follow_up run on Fridays instead, and use 10 days inste… | – | 3/3 | 3/3 | 3/3 |
| jobs | 11 | call | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 11 | status | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | – | 0/3 | 3/3 | 3/3 |
| jobs | 13 | answer | What happened with my N26 application, and why does it say c… | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 14 | answer | What have we set up so far, and is anything still waiting on… | 3/3 | 3/3 | 3/3 | 3/3 |

### ollama-cloud/gpt-oss:120b

| scenario | # | check | message | 1 | 2 | 3 | 4 |
|---|---|---|---|---|---|---|---|
| crypto | 3 | status | Show all prices in EUR, not USD. I'm in Germany and compare … | – | 3/3 | 3/3 | 3/3 |
| crypto | 4 | status | I want an alert when BTC drops more than 5% in a day. Don't … | – | 3/3 | 3/3 | 3/3 |
| crypto | 7 | call | Change daily_btc to run at 07:00 instead, and skip weekends:… | 3/3 | 3/3 | 3/3 | 3/3 |
| crypto | 7 | status | Change daily_btc to run at 07:00 instead, and skip weekends:… | – | 3/3 | 3/3 | 2/3 |
| crypto | 8 | call | Now do the same for Ethereum: a table, a pipeline and a char… | 3/3 | 3/3 | 3/3 | 3/3 |
| crypto | 10 | call | About the alert I mentioned earlier: send it to Telegram. Se… | 3/3 | 3/3 | 3/3 | 3/3 |
| crypto | 10 | status | About the alert I mentioned earlier: send it to Telegram. Se… | – | 1/3 | 3/3 | 1/3 |
| crypto | 12 | answer | Why did we stop collecting weekend prices? | 3/3 | 2/3 | 3/3 | 3/3 |
| crypto | 13 | status | Actually, drop the Telegram alert. I'll use my exchange's ow… | – | 3/3 | 1/3 | 0/3 |
| crypto | 14 | answer | Give me a short list of what we set up in this chat, and wha… | 2/3 | 3/3 | 3/3 | 3/3 |
| jobs | 3 | status | From now on, when I say a company rejected me, set the statu… | – | 2/3 | 1/3 | 3/3 |
| jobs | 6 | status | I also want a pipeline that checks Zalando's careers page ev… | – | 2/3 | 1/3 | 1/3 |
| jobs | 9 | call | N26 rejected me. | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 10 | call | Make follow_up run on Fridays instead, and use 10 days inste… | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 10 | status | Make follow_up run on Fridays instead, and use 10 days inste… | – | 3/3 | 3/3 | 3/3 |
| jobs | 11 | call | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | 0/3 | 0/3 | 0/3 | 1/3 |
| jobs | 11 | status | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | – | 1/3 | 2/3 | 2/3 |
| jobs | 13 | answer | What happened with my N26 application, and why does it say c… | 3/3 | 3/3 | 3/3 | 3/3 |
| jobs | 14 | answer | What have we set up so far, and is anything still waiting on… | 2/3 | 2/3 | 2/3 | 2/3 |

