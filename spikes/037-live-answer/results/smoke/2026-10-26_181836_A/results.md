# SPIKE-037 runs

## Part A: the live answer

Per condition and model, all tests together. A finish turn is `whole` when it keeps ≥60 % of the facts that arrived before it (or, for the first, repeats ≥40 % of the first answer's longer words), `appended` when it starts with the previous answer word for word and adds a block, `rest` otherwise, `none` without text. The view is what the user reads at the end: in `live`, a finish turn replaces its request's answer while no user message came since; in `stacked`, every answer stays.

| cond | model | runs | err | bg runs | finish turns | whole | rest | appended | none | new facts | earlier facts kept | dropped / turn | view right | last answer right | union right | slots | output / update | requests / update | cache read |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| stacked | ministral-14b-latest | 1 | 0 | 1 | 4 | 100% | 0% | 0% | 0% | 100% | 100% | 0.0 | 100% | 100% | 100% | 5.0 | 255 | 1.5 | 1% |
| stacked | gemma4:31b | 1 | 0 | 1 | 4 | 100% | 0% | 0% | 0% | 100% | 100% | 0.0 | 100% | 100% | 100% | 5.0 | 344 | 1.0 | 0% |
| stacked | gpt-oss:120b | 1 | 0 | 1 | 4 | 100% | 0% | 0% | 0% | 100% | 100% | 0.0 | 100% | 100% | 100% | 5.0 | 194 | 1.0 | 0% |
| live | ministral-14b-latest | 1 | 0 | 1 | 4 | 75% | 25% | 0% | 0% | 75% | 67% | 1.0 | 100% | 100% | 100% | 1.0 | 193 | 1.8 | 3% |
| live | gemma4:31b | 1 | 0 | 1 | 4 | 100% | 0% | 0% | 0% | 100% | 100% | 0.0 | 100% | 100% | 100% | 1.0 | 430 | 1.0 | 0% |
| live | gpt-oss:120b | 1 | 0 | 1 | 4 | 100% | 0% | 0% | 0% | 85% | 81% | 0.0 | 85% | 85% | 85% | 1.0 | 210 | 1.0 | 0% |

### Per test

Finish turns as whole/rest/appended/none, and the view's facts at the end (mean over reps).

| test | cond | model | runs | bg | classes | view right | last answer right | slots |
|---|---|---|---|---|---|---|---|---|
| a_table | stacked | ministral-14b-latest | 1 | 1 | 4/0/0/0 | 100% | 100% | 5.0 |
| a_table | stacked | gemma4:31b | 1 | 1 | 4/0/0/0 | 100% | 100% | 5.0 |
| a_table | stacked | gpt-oss:120b | 1 | 1 | 4/0/0/0 | 100% | 100% | 5.0 |
| a_table | live | ministral-14b-latest | 1 | 1 | 3/1/0/0 | 100% | 100% | 1.0 |
| a_table | live | gemma4:31b | 1 | 1 | 4/0/0/0 | 100% | 100% | 1.0 |
| a_table | live | gpt-oss:120b | 1 | 1 | 4/0/0/0 | 85% | 85% | 1.0 |

### Edge cases

| cond | model | failure said | failure in view | empty result: classes | together: both in one answer | late message: Saturday | later finish: Saturday | Persian: lowest share | Persian held |
|---|---|---|---|---|---|---|---|---|---|
| stacked | ministral-14b-latest | – | – |  | – | – | – | – | – |
| stacked | gemma4:31b | – | – |  | – | – | – | – | – |
| stacked | gpt-oss:120b | – | – |  | – | – | – | – | – |
| live | ministral-14b-latest | – | – |  | – | – | – | – | – |
| live | gemma4:31b | – | – |  | – | – | – | – | – |
| live | gpt-oss:120b | – | – |  | – | – | – | – | – |

