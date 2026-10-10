# SPIKE-031 round 2: judges

Each pair was judged twice, once in each order. *X wins* and *Y wins* count pairs where both verdicts agree; *split* is a pair where the order changed the verdict. *Scores* are the means, 1–10. *A picked* is the share of all verdicts that picked the answer shown first.

| X vs Y | Builder | Judge | Pairs | X wins | Tie | Y wins | Split | Score X | Score Y | A picked |
|---|---|---|---|---|---|---|---|---|---|---|
| index vs path | all | ollama-cloud/gpt-oss:120b | 10 | 4 | 0 | 2 | 4 | 8.1 | 7.7 | 70% |
| index vs path | all | ollama-cloud/nemotron-3-ultra | 10 | 4 | 0 | 4 | 2 | 8.2 | 8.1 | 50% |
| index vs path | gemini/gemini-3.5-flash-lite | ollama-cloud/gpt-oss:120b | 5 | 1 | 0 | 1 | 3 | 7.9 | 8.0 | 80% |
| index vs path | gemini/gemini-3.5-flash-lite | ollama-cloud/nemotron-3-ultra | 5 | 1 | 0 | 3 | 1 | 7.9 | 8.4 | 60% |
| index vs path | ollama-cloud/gemma4:31b | ollama-cloud/gpt-oss:120b | 5 | 3 | 0 | 1 | 1 | 8.3 | 7.4 | 60% |
| index vs path | ollama-cloud/gemma4:31b | ollama-cloud/nemotron-3-ultra | 5 | 3 | 0 | 1 | 1 | 8.6 | 7.7 | 40% |
| index vs plain | all | ollama-cloud/gpt-oss:120b | 10 | 1 | 0 | 9 | 0 | 7.2 | 8.8 | 50% |
| index vs plain | all | ollama-cloud/nemotron-3-ultra | 10 | 1 | 0 | 8 | 1 | 7.5 | 8.7 | 55% |
| index vs plain | gemini/gemini-3.5-flash-lite | ollama-cloud/gpt-oss:120b | 5 | 0 | 0 | 5 | 0 | 6.9 | 9.0 | 50% |
| index vs plain | gemini/gemini-3.5-flash-lite | ollama-cloud/nemotron-3-ultra | 5 | 0 | 0 | 4 | 1 | 7.2 | 8.8 | 60% |
| index vs plain | ollama-cloud/gemma4:31b | ollama-cloud/gpt-oss:120b | 5 | 1 | 0 | 4 | 0 | 7.4 | 8.5 | 50% |
| index vs plain | ollama-cloud/gemma4:31b | ollama-cloud/nemotron-3-ultra | 5 | 1 | 0 | 4 | 0 | 7.7 | 8.6 | 50% |
| index vs thinking | all | ollama-cloud/gpt-oss:120b | 10 | 0 | 0 | 8 | 2 | 7.1 | 8.8 | 60% |
| index vs thinking | all | ollama-cloud/nemotron-3-ultra | 10 | 0 | 0 | 7 | 3 | 7.5 | 8.8 | 65% |
| index vs thinking | gemini/gemini-3.5-flash-lite | ollama-cloud/gpt-oss:120b | 5 | 0 | 0 | 3 | 2 | 7.2 | 8.7 | 70% |
| index vs thinking | gemini/gemini-3.5-flash-lite | ollama-cloud/nemotron-3-ultra | 5 | 0 | 0 | 3 | 2 | 7.4 | 8.7 | 70% |
| index vs thinking | ollama-cloud/gemma4:31b | ollama-cloud/gpt-oss:120b | 5 | 0 | 0 | 5 | 0 | 7.0 | 9.0 | 50% |
| index vs thinking | ollama-cloud/gemma4:31b | ollama-cloud/nemotron-3-ultra | 5 | 0 | 0 | 4 | 1 | 7.5 | 8.9 | 60% |
| path vs plain | all | ollama-cloud/gpt-oss:120b | 10 | 0 | 0 | 9 | 1 | 7.0 | 8.8 | 55% |
| path vs plain | all | ollama-cloud/nemotron-3-ultra | 10 | 0 | 0 | 10 | 0 | 7.0 | 9.0 | 50% |
| path vs plain | gemini/gemini-3.5-flash-lite | ollama-cloud/gpt-oss:120b | 5 | 0 | 0 | 4 | 1 | 7.0 | 8.9 | 60% |
| path vs plain | gemini/gemini-3.5-flash-lite | ollama-cloud/nemotron-3-ultra | 5 | 0 | 0 | 5 | 0 | 7.1 | 9.0 | 50% |
| path vs plain | ollama-cloud/gemma4:31b | ollama-cloud/gpt-oss:120b | 5 | 0 | 0 | 5 | 0 | 7.0 | 8.8 | 50% |
| path vs plain | ollama-cloud/gemma4:31b | ollama-cloud/nemotron-3-ultra | 5 | 0 | 0 | 5 | 0 | 7.0 | 9.0 | 50% |

## Do the judges agree?

40 pairs judged by both; same result in 33.

- b1-exam · gemini/gemini-3.5-flash-lite · index vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- b1-exam · gemini/gemini-3.5-flash-lite · index vs thinking: ollama-cloud/gpt-oss:120b: split, ollama-cloud/nemotron-3-ultra: split
- b1-exam · gemini/gemini-3.5-flash-lite · index vs path: ollama-cloud/gpt-oss:120b: x, ollama-cloud/nemotron-3-ultra: x
- b1-exam · gemini/gemini-3.5-flash-lite · path vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- b1-exam · ollama-cloud/gemma4:31b · index vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- b1-exam · ollama-cloud/gemma4:31b · index vs thinking: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- b1-exam · ollama-cloud/gemma4:31b · index vs path: ollama-cloud/gpt-oss:120b: x, ollama-cloud/nemotron-3-ultra: x
- b1-exam · ollama-cloud/gemma4:31b · path vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- bakery · gemini/gemini-3.5-flash-lite · index vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: split
- bakery · gemini/gemini-3.5-flash-lite · index vs thinking: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- bakery · gemini/gemini-3.5-flash-lite · index vs path: ollama-cloud/gpt-oss:120b: split, ollama-cloud/nemotron-3-ultra: split
- bakery · gemini/gemini-3.5-flash-lite · path vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- bakery · ollama-cloud/gemma4:31b · index vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- bakery · ollama-cloud/gemma4:31b · index vs thinking: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- bakery · ollama-cloud/gemma4:31b · index vs path: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- bakery · ollama-cloud/gemma4:31b · path vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- db-move · gemini/gemini-3.5-flash-lite · index vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- db-move · gemini/gemini-3.5-flash-lite · index vs thinking: ollama-cloud/gpt-oss:120b: split, ollama-cloud/nemotron-3-ultra: split
- db-move · gemini/gemini-3.5-flash-lite · index vs path: ollama-cloud/gpt-oss:120b: split, ollama-cloud/nemotron-3-ultra: y
- db-move · gemini/gemini-3.5-flash-lite · path vs plain: ollama-cloud/gpt-oss:120b: split, ollama-cloud/nemotron-3-ultra: y
- db-move · ollama-cloud/gemma4:31b · index vs plain: ollama-cloud/gpt-oss:120b: x, ollama-cloud/nemotron-3-ultra: x
- db-move · ollama-cloud/gemma4:31b · index vs thinking: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: split
- db-move · ollama-cloud/gemma4:31b · index vs path: ollama-cloud/gpt-oss:120b: x, ollama-cloud/nemotron-3-ultra: x
- db-move · ollama-cloud/gemma4:31b · path vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- report-page · gemini/gemini-3.5-flash-lite · index vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- report-page · gemini/gemini-3.5-flash-lite · index vs thinking: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- report-page · gemini/gemini-3.5-flash-lite · index vs path: ollama-cloud/gpt-oss:120b: split, ollama-cloud/nemotron-3-ultra: y
- report-page · gemini/gemini-3.5-flash-lite · path vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- report-page · ollama-cloud/gemma4:31b · index vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- report-page · ollama-cloud/gemma4:31b · index vs thinking: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- report-page · ollama-cloud/gemma4:31b · index vs path: ollama-cloud/gpt-oss:120b: x, ollama-cloud/nemotron-3-ultra: split
- report-page · ollama-cloud/gemma4:31b · path vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- umzug · gemini/gemini-3.5-flash-lite · index vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- umzug · gemini/gemini-3.5-flash-lite · index vs thinking: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- umzug · gemini/gemini-3.5-flash-lite · index vs path: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- umzug · gemini/gemini-3.5-flash-lite · path vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- umzug · ollama-cloud/gemma4:31b · index vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- umzug · ollama-cloud/gemma4:31b · index vs thinking: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
- umzug · ollama-cloud/gemma4:31b · index vs path: ollama-cloud/gpt-oss:120b: split, ollama-cloud/nemotron-3-ultra: x
- umzug · ollama-cloud/gemma4:31b · path vs plain: ollama-cloud/gpt-oss:120b: y, ollama-cloud/nemotron-3-ultra: y
