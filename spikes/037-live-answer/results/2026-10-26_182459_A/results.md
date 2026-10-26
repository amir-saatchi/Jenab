# SPIKE-037 runs

## Part A: the live answer

Per condition and model, all tests together. A finish turn is `whole` when it keeps ≥60 % of the facts that arrived before it (or, for the first, repeats ≥40 % of the first answer's longer words), `appended` when it starts with the previous answer word for word and adds a block, `rest` otherwise, `none` without text. The view is what the user reads at the end: in `live`, a finish turn replaces its request's answer while no user message came since; in `stacked`, every answer stays.

| cond | model | runs | err | bg runs | finish turns | whole | rest | appended | none | new facts | earlier facts kept | dropped / turn | view right | last answer right | union right | slots | output / update | requests / update | cache read |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| stacked | ministral-14b-latest | 24 | 0 | 23 | 69 | 67% | 33% | 0% | 0% | 80% | 75% | 0.7 | 95% | 83% | 95% | 4.1 | 199 | 1.5 | 35% |
| stacked | gemma4:31b | 24 | 0 | 24 | 72 | 79% | 21% | 0% | 0% | 89% | 95% | 0.0 | 97% | 97% | 97% | 4.1 | 357 | 1.0 | 0% |
| stacked | gpt-oss:120b | 24 | 0 | 24 | 72 | 79% | 21% | 0% | 0% | 87% | 93% | 0.2 | 97% | 95% | 97% | 4.1 | 284 | 1.0 | 0% |
| live | ministral-14b-latest | 24 | 0 | 23 | 69 | 86% | 14% | 0% | 0% | 84% | 94% | 0.1 | 97% | 97% | 98% | 1.4 | 252 | 1.7 | 31% |
| live | gemma4:31b | 24 | 0 | 24 | 72 | 86% | 14% | 0% | 0% | 90% | 99% | 0.0 | 98% | 98% | 98% | 1.4 | 388 | 1.0 | 0% |
| live | gpt-oss:120b | 24 | 0 | 23 | 69 | 91% | 9% | 0% | 0% | 90% | 98% | 0.1 | 98% | 96% | 98% | 1.4 | 318 | 1.0 | 0% |

### Per test

Finish turns as whole/rest/appended/none, and the view's facts at the end (mean over reps).

| test | cond | model | runs | bg | classes | view right | last answer right | slots |
|---|---|---|---|---|---|---|---|---|
| a_empty | stacked | ministral-14b-latest | 3 | 3 | 9/0/0/0 | 100% | 100% | 4.0 |
| a_empty | stacked | gemma4:31b | 3 | 3 | 8/1/0/0 | 100% | 100% | 4.0 |
| a_empty | stacked | gpt-oss:120b | 3 | 3 | 8/1/0/0 | 100% | 100% | 4.0 |
| a_empty | live | ministral-14b-latest | 3 | 3 | 9/0/0/0 | 100% | 100% | 1.0 |
| a_empty | live | gemma4:31b | 3 | 3 | 7/2/0/0 | 100% | 100% | 1.0 |
| a_empty | live | gpt-oss:120b | 3 | 3 | 9/0/0/0 | 100% | 100% | 1.0 |
| a_failed | stacked | ministral-14b-latest | 3 | 3 | 5/4/0/0 | 100% | 100% | 4.0 |
| a_failed | stacked | gemma4:31b | 3 | 3 | 6/3/0/0 | 100% | 100% | 4.0 |
| a_failed | stacked | gpt-oss:120b | 3 | 3 | 6/3/0/0 | 100% | 100% | 4.0 |
| a_failed | live | ministral-14b-latest | 3 | 3 | 6/3/0/0 | 100% | 100% | 1.0 |
| a_failed | live | gemma4:31b | 3 | 3 | 8/1/0/0 | 100% | 100% | 1.0 |
| a_failed | live | gpt-oss:120b | 3 | 2 | 4/2/0/0 | 100% | 100% | 1.0 |
| a_late | stacked | ministral-14b-latest | 3 | 3 | 5/4/0/0 | 88% | 71% | 5.0 |
| a_late | stacked | gemma4:31b | 3 | 3 | 6/3/0/0 | 100% | 100% | 5.0 |
| a_late | stacked | gpt-oss:120b | 3 | 3 | 6/3/0/0 | 92% | 83% | 5.0 |
| a_late | live | ministral-14b-latest | 3 | 3 | 7/2/0/0 | 100% | 100% | 4.0 |
| a_late | live | gemma4:31b | 3 | 3 | 8/1/0/0 | 100% | 100% | 4.0 |
| a_late | live | gpt-oss:120b | 3 | 3 | 8/1/0/0 | 88% | 75% | 4.0 |
| a_mix | stacked | ministral-14b-latest | 3 | 3 | 9/0/0/0 | 100% | 100% | 4.0 |
| a_mix | stacked | gemma4:31b | 3 | 3 | 9/0/0/0 | 100% | 100% | 4.0 |
| a_mix | stacked | gpt-oss:120b | 3 | 3 | 9/0/0/0 | 100% | 100% | 4.0 |
| a_mix | live | ministral-14b-latest | 3 | 3 | 9/0/0/0 | 100% | 100% | 1.0 |
| a_mix | live | gemma4:31b | 3 | 3 | 9/0/0/0 | 100% | 100% | 1.0 |
| a_mix | live | gpt-oss:120b | 3 | 3 | 9/0/0/0 | 100% | 100% | 1.0 |
| a_persian | stacked | ministral-14b-latest | 3 | 2 | 5/1/0/0 | 100% | 100% | 4.0 |
| a_persian | stacked | gemma4:31b | 3 | 3 | 8/1/0/0 | 100% | 100% | 4.0 |
| a_persian | stacked | gpt-oss:120b | 3 | 3 | 7/2/0/0 | 100% | 100% | 4.0 |
| a_persian | live | ministral-14b-latest | 3 | 2 | 5/1/0/0 | 100% | 100% | 1.0 |
| a_persian | live | gemma4:31b | 3 | 3 | 8/1/0/0 | 100% | 100% | 1.0 |
| a_persian | live | gpt-oss:120b | 3 | 3 | 9/0/0/0 | 100% | 100% | 1.0 |
| a_table | stacked | ministral-14b-latest | 3 | 3 | 8/4/0/0 | 100% | 67% | 5.0 |
| a_table | stacked | gemma4:31b | 3 | 3 | 11/1/0/0 | 100% | 100% | 5.0 |
| a_table | stacked | gpt-oss:120b | 3 | 3 | 12/0/0/0 | 100% | 100% | 5.0 |
| a_table | live | ministral-14b-latest | 3 | 3 | 10/2/0/0 | 100% | 100% | 1.0 |
| a_table | live | gemma4:31b | 3 | 3 | 12/0/0/0 | 100% | 100% | 1.0 |
| a_table | live | gpt-oss:120b | 3 | 3 | 12/0/0/0 | 100% | 100% | 1.0 |
| a_text | stacked | ministral-14b-latest | 3 | 3 | 1/8/0/0 | 89% | 74% | 4.0 |
| a_text | stacked | gemma4:31b | 3 | 3 | 5/4/0/0 | 89% | 89% | 4.0 |
| a_text | stacked | gpt-oss:120b | 3 | 3 | 6/3/0/0 | 96% | 85% | 4.0 |
| a_text | live | ministral-14b-latest | 3 | 3 | 7/2/0/0 | 93% | 93% | 1.0 |
| a_text | live | gemma4:31b | 3 | 3 | 7/2/0/0 | 96% | 96% | 1.0 |
| a_text | live | gpt-oss:120b | 3 | 3 | 7/2/0/0 | 100% | 100% | 1.0 |
| a_together | stacked | ministral-14b-latest | 3 | 3 | 4/2/0/0 | 86% | 57% | 3.0 |
| a_together | stacked | gemma4:31b | 3 | 3 | 4/2/0/0 | 86% | 86% | 3.0 |
| a_together | stacked | gpt-oss:120b | 3 | 3 | 3/3/0/0 | 90% | 90% | 3.0 |
| a_together | live | ministral-14b-latest | 3 | 3 | 6/0/0/0 | 86% | 86% | 1.0 |
| a_together | live | gemma4:31b | 3 | 3 | 3/3/0/0 | 86% | 86% | 1.0 |
| a_together | live | gpt-oss:120b | 3 | 3 | 5/1/0/0 | 95% | 95% | 1.0 |

### Edge cases

| cond | model | failure said | failure in view | empty result: classes | together: both in one answer | late message: Saturday | later finish: Saturday | Persian: lowest share | Persian held |
|---|---|---|---|---|---|---|---|---|---|
| stacked | ministral-14b-latest | 3/3 | 3/3 | whole 3 | 2/3 | 3/3 | 3/3 | 58% | 2/3 |
| stacked | gemma4:31b | 3/3 | 3/3 | whole 3 | 3/3 | 3/3 | 3/3 | 75% | 3/3 |
| stacked | gpt-oss:120b | 3/3 | 3/3 | whole 3 | 3/3 | 3/3 | 3/3 | 76% | 3/3 |
| live | ministral-14b-latest | 3/3 | 3/3 | whole 3 | 3/3 | 2/3 | 3/3 | 79% | 3/3 |
| live | gemma4:31b | 3/3 | 3/3 | whole 3 | 3/3 | 3/3 | 3/3 | 77% | 3/3 |
| live | gpt-oss:120b | 2/2 | 2/2 | whole 3 | 3/3 | 3/3 | 3/3 | 81% | 3/3 |

