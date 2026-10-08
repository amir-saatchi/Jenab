# SPIKE-030 results

Generated from 20261006-223020.jsonl, 20261006-232600.jsonl: 686 records.

*Valid* is schema-valid plus the index checks; *right* is valid and equal to the fixed answer. Percentages are of the runs that weren't skipped. Tokens and time are per run, the retry included.

## Thinking off

### By model and method

| Model | Method | Runs | Valid first | Valid after retry | Right | Made-up values | Copied values | Text around JSON | Prompt | Output | Requests | Time | Errors | Skipped |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite | tool | 39 | 100% | 100% | 74% | 4 | 3 | 0% | 2224 | 235 | 1.0 | 5.9 s | 0 | 0 |
| gemini/gemini-3.5-flash-lite | tool-forced | 39 | 100% | 100% | 64% | 6 | 4 | 0% | 2224 | 237 | 1.0 | 6.7 s | 0 | 0 |
| gemini/gemini-3.5-flash-lite | text | 39 | 92% | 100% | 79% | 0 | 3 | 0% | 2357 | 292 | 1.1 | 9.4 s | 0 | 0 |
| gemini/gemini-3.5-flash-lite | native | 39 | 100% | 100% | 85% | 0 | 2 | 0% | 2277 | 324 | 1.0 | 5.5 s | 0 | 0 |
| ollama-cloud/gemma4:31b | tool | 39 | 92% | 100% | 79% | 0 | 5 | 0% | 2287 | 247 | 1.1 | 40.6 s | 0 | 0 |
| ollama-cloud/gemma4:31b | tool-forced | 0 | | | | | | | | | | | | 39 |
| ollama-cloud/gemma4:31b | text | 39 | 100% | 100% | 77% | 0 | 6 | 100% | 2292 | 377 | 1.0 | 31.8 s | 0 | 0 |
| ollama-cloud/gemma4:31b | native | 39 | 100% | 100% | 74% | 0 | 6 | 100% | 2292 | 378 | 1.0 | 35.9 s | 0 | 0 |
| ollama-cloud/nemotron-3-ultra | tool | 39 | 97% | 97% | 59% | 3 | 4 | 0% | 3093 | 327 | 1.0 | 35.8 s | 0 | 0 |
| ollama-cloud/nemotron-3-ultra | tool-forced | 0 | | | | | | | | | | | | 39 |
| ollama-cloud/nemotron-3-ultra | text | 39 | 95% | 100% | 67% | 0 | 4 | 0% | 2387 | 343 | 1.1 | 35.7 s | 0 | 0 |
| ollama-cloud/nemotron-3-ultra | native | 39 | 100% | 100% | 67% | 2 | 3 | 0% | 2321 | 284 | 1.0 | 37.3 s | 0 | 0 |
| zai/glm-4.5-flash | tool | 26 | 50% | 50% | 38% | 0 | 0 | 0% | 511 | 643 | 1.0 | 104.7 s | 13 | 0 |
| zai/glm-4.5-flash | tool-forced | 25 | 52% | 56% | 44% | 0 | 0 | 0% | 1782 | 842 | 1.0 | 58.0 s | 11 | 0 |
| zai/glm-4.5-flash | text | 25 | 84% | 84% | 56% | 0 | 2 | 100% | 1287 | 1589 | 1.0 | 82.3 s | 4 | 0 |
| zai/glm-4.5-flash | native | 25 | 84% | 84% | 52% | 1 | 1 | 100% | 702 | 1463 | 1.0 | 125.5 s | 4 | 0 |

### By method, all models

| Method | Runs | Valid first | Valid after retry | Right | Made-up values | Copied values | Output | Time |
|---|---|---|---|---|---|---|---|---|
| tool | 143 | 88% | 90% | 65% | 7 | 12 | 338 | 41.5 s |
| tool-forced | 64 | 81% | 83% | 56% | 6 | 4 | 473 | 26.7 s |
| text | 142 | 94% | 97% | 71% | 0 | 15 | 558 | 35.6 s |
| native | 142 | 97% | 97% | 71% | 3 | 12 | 528 | 43.7 s |

### Right answers by task and method, all models

Right of runs.

| Task | tool | tool-forced | text | native |
|---|---|---|---|---|
| decide-de | 9/11 | 4/5 | 10/11 | 9/11 |
| decide-en | 11/11 | 3/5 | 10/11 | 8/11 |
| decide-fa | 11/11 | 5/5 | 6/11 | 9/11 |
| extract-de | 0/11 | 0/5 | 0/11 | 2/11 |
| extract-en | 6/11 | 3/5 | 6/11 | 7/11 |
| extract-fa | 6/11 | 3/5 | 7/11 | 8/11 |
| inject-en | 9/11 | 3/5 | 10/11 | 9/11 |
| long-en | 3/11 | 0/4 | 3/10 | 6/10 |
| nested-en | 11/11 | 5/5 | 11/11 | 11/11 |
| select-de | 7/11 | 3/5 | 7/11 | 7/11 |
| select-en | 9/11 | 4/5 | 10/11 | 7/11 |
| select-fa | 7/11 | 3/5 | 11/11 | 11/11 |
| wide-en | 4/11 | 0/5 | 10/11 | 7/11 |

### First-try failures by method

| Method | Failures |
|---|---|
| tool | format 3, item_count 1 |
| tool-forced | item_count 1 |
| text | item_count 2, wrong_type 3 |
| native |  |

## Thinking on

### By model and method

| Model | Method | Runs | Valid first | Valid after retry | Right | Made-up values | Copied values | Text around JSON | Prompt | Output | Requests | Time | Errors | Skipped |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite | native | 39 | 100% | 100% | 92% | 0 | 1 | 0% | 2277 | 360 | 1.0 | 5.3 s | 0 | 0 |
| ollama-cloud/gemma4:31b | native | 39 | 95% | 95% | 79% | 0 | 6 | 100% | 1788 | 1259 | 1.0 | 111.5 s | 2 | 0 |
| ollama-cloud/nemotron-3-ultra | native | 39 | 92% | 92% | 69% | 2 | 5 | 0% | 1300 | 1154 | 1.0 | 111.2 s | 3 | 0 |

### By method, all models

| Method | Runs | Valid first | Valid after retry | Right | Made-up values | Copied values | Output | Time |
|---|---|---|---|---|---|---|---|---|
| native | 117 | 96% | 96% | 80% | 2 | 12 | 924 | 76.0 s |

### Right answers by task and method, all models

Right of runs.

| Task | native |
|---|---|
| decide-de | 7/9 |
| decide-en | 7/9 |
| decide-fa | 8/9 |
| extract-de | 2/9 |
| extract-en | 4/9 |
| extract-fa | 9/9 |
| inject-en | 9/9 |
| long-en | 6/9 |
| nested-en | 9/9 |
| select-de | 8/9 |
| select-en | 9/9 |
| select-fa | 9/9 |
| wide-en | 7/9 |

### First-try failures by method

| Method | Failures |
|---|---|
| native |  |

## What each API accepts (probe)

| Model | tool_choice named | tool_choice required | JSON schema mode | JSON object mode | Notes |
|---|---|---|---|---|---|
| gemini/gemini-3.5-flash-lite | yes | yes | yes | yes |  |
| ollama-cloud/gemma4:31b | no | no | yes | no | no tool_choice in Ollama's /api/chat |
| ollama-cloud/nemotron-3-ultra | no | no | yes | no | no tool_choice in Ollama's /api/chat |
| zai/glm-4.5-flash | yes | yes | yes | yes |  |

## Provider errors

- zai/glm-4.5-flash, tool-forced, extract-de r1: rate_limited zai: rate_limited (429): {"error":{"code":"1302","message":"Rate limit reached for requests"}}
- zai/glm-4.5-flash, tool, extract-de r1: transport zai: transport: tool call submit: the arguments are not valid JSON
- zai/glm-4.5-flash, tool-forced, extract-en r1: rate_limited zai: rate_limited (429): {"error":{"code":"1302","message":"Rate limit reached for requests"}}
- zai/glm-4.5-flash, tool, extract-en r1: transport zai: transport: tool call submit: the arguments are not valid JSON
- zai/glm-4.5-flash, tool-forced, extract-fa r1: rate_limited zai: rate_limited (429): {"error":{"code":"1302","message":"Rate limit reached for requests"}}
- zai/glm-4.5-flash, tool, extract-fa r1: transport zai: transport: tool call submit: the arguments are not valid JSON
- zai/glm-4.5-flash, tool, inject-en r1: transport zai: transport: tool call submit: the arguments are not valid JSON
- zai/glm-4.5-flash, tool-forced, inject-en r1: transport zai: transport: tool call submit: the arguments are not valid JSON
- zai/glm-4.5-flash, native, inject-en r1:  context deadline exceeded
- zai/glm-4.5-flash, tool, wide-en r1: transport zai: transport: tool call submit: the arguments are not valid JSON
- zai/glm-4.5-flash, tool-forced, wide-en r1: transport zai: transport: tool call submit: the arguments are not valid JSON
- zai/glm-4.5-flash, tool, long-en r1:  context deadline exceeded
- zai/glm-4.5-flash, native, long-en r1:  context deadline exceeded
- zai/glm-4.5-flash, tool, decide-de r2:  context deadline exceeded
- zai/glm-4.5-flash, tool-forced, decide-de r2: rate_limited zai: rate_limited (429): {"error":{"code":"1302","message":"Rate limit reached for requests"}}
- zai/glm-4.5-flash, tool, extract-de r2: transport zai: transport: tool call submit: the arguments are not valid JSON
- zai/glm-4.5-flash, tool-forced, extract-de r2: transport zai: transport: the stream stalled: no event for 1m0s
- zai/glm-4.5-flash, tool, extract-en r2: rate_limited zai: rate_limited (429): {"error":{"code":"1302","message":"Rate limit reached for requests"}}
- zai/glm-4.5-flash, tool-forced, extract-en r2: rate_limited zai: rate_limited (429): {"error":{"code":"1302","message":"Rate limit reached for requests"}}
- zai/glm-4.5-flash, text, extract-en r2: rate_limited zai: rate_limited (429): {"error":{"code":"1302","message":"Rate limit reached for requests"}}
- zai/glm-4.5-flash, tool, extract-fa r2: transport zai: transport: tool call submit: the arguments are not valid JSON
- zai/glm-4.5-flash, text, extract-fa r2: rate_limited zai: rate_limited (429): {"error":{"code":"1302","message":"Rate limit reached for requests"}}
- zai/glm-4.5-flash, tool-forced, extract-fa r2:  context deadline exceeded
- zai/glm-4.5-flash, tool, inject-en r2: rate_limited zai: rate_limited (429): {"error":{"code":"1302","message":"Rate limit reached for requests"}}
- zai/glm-4.5-flash, tool-forced, inject-en r2: rate_limited zai: rate_limited (429): {"error":{"code":"1302","message":"Rate limit reached for requests"}}
- zai/glm-4.5-flash, text, inject-en r2: rate_limited zai: rate_limited (429): {"error":{"code":"1302","message":"Rate limit reached for requests"}}
- zai/glm-4.5-flash, native, inject-en r2:  context deadline exceeded
- zai/glm-4.5-flash, tool, wide-en r2: transport zai: transport: tool call submit: the arguments are not valid JSON
- zai/glm-4.5-flash, tool-forced, wide-en r2: transport zai: transport: the stream stalled: no event for 1m0s
- zai/glm-4.5-flash, text, wide-en r2: rate_limited zai: rate_limited (429): {"error":{"code":"1302","message":"Rate limit reached for requests"}}
- zai/glm-4.5-flash, native, wide-en r2:  context deadline exceeded
- zai/glm-4.5-flash, tool, long-en r2: rate_limited zai: rate_limited (429): {"error":{"code":"1302","message":"Rate limit reached for requests"}}
- ollama-cloud/nemotron-3-ultra, native, long-en r1:  context deadline exceeded
- ollama-cloud/gemma4:31b, native, long-en r1:  context deadline exceeded
- ollama-cloud/nemotron-3-ultra, native, long-en r2:  context deadline exceeded
- ollama-cloud/nemotron-3-ultra, native, decide-de r3:  context deadline exceeded
- ollama-cloud/gemma4:31b, native, decide-de r3:  context deadline exceeded
