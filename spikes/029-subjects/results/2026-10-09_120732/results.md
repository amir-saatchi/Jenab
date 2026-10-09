# SPIKE-029 results

24 runs, prompt rule v2. Conditions: 1 baseline, 2 tool, 3 tool + index, 4 tool + index + the app's check.

## Keeping the subjects

| model | condition | runs | work turns with a subject, on their own | with the app's check | nudges | subjects at the end, per run | made for questions | changes that updated the existing subject | duplicates | update_subject alone in its response |
|---|---|---|---|---|---|---|---|---|---|---|
| ollama-cloud/gemma4:31b | 2 tool | 6 | 52/54 (96%) | 52/54 (96%) | 0 | 6.8 | 0 | 13/18 (72%) | 4 | 9/61 (15%) |
| ollama-cloud/gemma4:31b | 3 tool+index | 6 | 46/54 (85%) | 46/54 (85%) | 0 | 5.8 | 0 | 13/17 (76%) | 0 | 9/55 (16%) |
| ollama-cloud/gpt-oss:120b | 2 tool | 6 | 49/54 (91%) | 49/54 (91%) | 0 | 8.0 | 0 | 4/17 (24%) | 12 | 56/56 (100%) |
| ollama-cloud/gpt-oss:120b | 3 tool+index | 6 | 49/54 (91%) | 49/54 (91%) | 0 | 6.7 | 0 | 15/17 (88%) | 3 | 58/58 (100%) |

## Correctness and payoff

| model | condition | subject status right | follow-up answers right | calls that needed earlier turns right | history reads per run | get_subject per run |
|---|---|---|---|---|---|---|
| ollama-cloud/gemma4:31b | 2 tool | 25/27 (93%) | 12/12 (100%) | 18/18 (100%) | 4.5 | 2.0 |
| ollama-cloud/gemma4:31b | 3 tool+index | 22/27 (81%) | 12/12 (100%) | 17/18 (94%) | 3.2 | 0.7 |
| ollama-cloud/gpt-oss:120b | 2 tool | 18/27 (67%) | 10/12 (83%) | 14/18 (78%) | 2.8 | 1.3 |
| ollama-cloud/gpt-oss:120b | 3 tool+index | 22/27 (81%) | 10/12 (83%) | 17/18 (94%) | 1.8 | 1.5 |

## Cost

| model | condition | requests per run | prompt tokens per run | output tokens per run | request errors | runs that ended early |
|---|---|---|---|---|---|---|
| ollama-cloud/gemma4:31b | 2 tool | 32.3 | 104728.8 | 7650.0 | 0 | 0 |
| ollama-cloud/gemma4:31b | 3 tool+index | 31.8 | 102657.0 | 7619.2 | 0 | 0 |
| ollama-cloud/gpt-oss:120b | 2 tool | 44.8 | 122589.7 | 5854.0 | 2 | 0 |
| ollama-cloud/gpt-oss:120b | 3 tool+index | 44.3 | 112831.5 | 6070.2 | 2 | 0 |

## Checks one by one

Passes out of runs, per condition.

### ollama-cloud/gemma4:31b

| scenario | # | check | message | 1 | 2 | 3 | 4 |
|---|---|---|---|---|---|---|---|
| crypto | 3 | status | Show all prices in EUR, not USD. I'm in Germany and compare … | – | 3/3 | 3/3 | – |
| crypto | 4 | status | I want an alert when BTC drops more than 5% in a day. Don't … | – | 3/3 | 3/3 | – |
| crypto | 7 | call | Change daily_btc to run at 07:00 instead, and skip weekends:… | – | 3/3 | 3/3 | – |
| crypto | 7 | status | Change daily_btc to run at 07:00 instead, and skip weekends:… | – | 3/3 | 3/3 | – |
| crypto | 8 | call | Now do the same for Ethereum: a table, a pipeline and a char… | – | 3/3 | 3/3 | – |
| crypto | 10 | call | About the alert I mentioned earlier: send it to Telegram. Se… | – | 3/3 | 2/3 | – |
| crypto | 10 | status | About the alert I mentioned earlier: send it to Telegram. Se… | – | 3/3 | 2/3 | – |
| crypto | 12 | answer | Why did we stop collecting weekend prices? | – | 3/3 | 3/3 | – |
| crypto | 13 | status | Actually, drop the Telegram alert. I'll use my exchange's ow… | – | 3/3 | 3/3 | – |
| crypto | 14 | answer | Give me a short list of what we set up in this chat, and wha… | – | 3/3 | 3/3 | – |
| jobs | 3 | status | From now on, when I say a company rejected me, set the statu… | – | 2/3 | 0/3 | – |
| jobs | 6 | status | I also want a pipeline that checks Zalando's careers page ev… | – | 3/3 | 2/3 | – |
| jobs | 9 | call | N26 rejected me. | – | 3/3 | 3/3 | – |
| jobs | 10 | call | Make follow_up run on Fridays instead, and use 10 days inste… | – | 3/3 | 3/3 | – |
| jobs | 10 | status | Make follow_up run on Fridays instead, and use 10 days inste… | – | 3/3 | 3/3 | – |
| jobs | 11 | call | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | – | 3/3 | 3/3 | – |
| jobs | 11 | status | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | – | 2/3 | 3/3 | – |
| jobs | 13 | answer | What happened with my N26 application, and why does it say c… | – | 3/3 | 3/3 | – |
| jobs | 14 | answer | What have we set up so far, and is anything still waiting on… | – | 3/3 | 3/3 | – |

### ollama-cloud/gpt-oss:120b

| scenario | # | check | message | 1 | 2 | 3 | 4 |
|---|---|---|---|---|---|---|---|
| crypto | 3 | status | Show all prices in EUR, not USD. I'm in Germany and compare … | – | 3/3 | 3/3 | – |
| crypto | 4 | status | I want an alert when BTC drops more than 5% in a day. Don't … | – | 3/3 | 3/3 | – |
| crypto | 7 | call | Change daily_btc to run at 07:00 instead, and skip weekends:… | – | 3/3 | 3/3 | – |
| crypto | 7 | status | Change daily_btc to run at 07:00 instead, and skip weekends:… | – | 3/3 | 3/3 | – |
| crypto | 8 | call | Now do the same for Ethereum: a table, a pipeline and a char… | – | 3/3 | 3/3 | – |
| crypto | 10 | call | About the alert I mentioned earlier: send it to Telegram. Se… | – | 2/3 | 3/3 | – |
| crypto | 10 | status | About the alert I mentioned earlier: send it to Telegram. Se… | – | 1/3 | 3/3 | – |
| crypto | 12 | answer | Why did we stop collecting weekend prices? | – | 3/3 | 3/3 | – |
| crypto | 13 | status | Actually, drop the Telegram alert. I'll use my exchange's ow… | – | 3/3 | 3/3 | – |
| crypto | 14 | answer | Give me a short list of what we set up in this chat, and wha… | – | 2/3 | 2/3 | – |
| jobs | 3 | status | From now on, when I say a company rejected me, set the statu… | – | 0/3 | 0/3 | – |
| jobs | 6 | status | I also want a pipeline that checks Zalando's careers page ev… | – | 2/3 | 2/3 | – |
| jobs | 9 | call | N26 rejected me. | – | 3/3 | 3/3 | – |
| jobs | 10 | call | Make follow_up run on Fridays instead, and use 10 days inste… | – | 3/3 | 3/3 | – |
| jobs | 10 | status | Make follow_up run on Fridays instead, and use 10 days inste… | – | 2/3 | 3/3 | – |
| jobs | 11 | call | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | – | 0/3 | 2/3 | – |
| jobs | 11 | status | Here's the Zalando link: https://jobs.zalando.com/en/jobs?se… | – | 1/3 | 2/3 | – |
| jobs | 13 | answer | What happened with my N26 application, and why does it say c… | – | 3/3 | 3/3 | – |
| jobs | 14 | answer | What have we set up so far, and is anything still waiting on… | – | 2/3 | 2/3 | – |

