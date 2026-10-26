# SPIKE-037 runs

## Part B: earlier turns

Prompt tokens as the provider reports them (input + cache read + cache write). Trace tokens are estimated (4 bytes a token) in the last request of the run.

| chat | cond | model | runs | err | requests | prompt / request | prompt / run | largest | cache read | cuts / run | follow-ups right | reads / run | repeated / run | wrong names / run | calls / later turn | fake calls | wrong claims | trace tokens |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| b_trip | window | gpt-oss:120b | 1 | 0 | 42.0 | 2236 | 93914 | 4342 | 0% | 5.0 | 67% | 7.0 | 2.0 | 0.0 | 1.5 | 0 | 0 | 0 |

### Follow-ups right, per kind

Each cell is right/asked, and in brackets how many of those turns read the history, a ref, a table or a page.

| chat | cond | model | table | detail | source | task | ref | more | fact | claims |
|---|---|---|---|---|---|---|---|---|---|---|
| b_trip | window | gpt-oss:120b | 2/2 (1) | 1/1 (0) | 1/1 (1) | 1/1 (0) | 1/1 (1) | 0/1 (0) | 0/1 (0) | 0/1 (1) |

### Repeated work, wrong names, fake calls and wrong claims

- b_trip_window_ollama-cloud-gpt-oss-120b_r1 repeated: t9 fetch_page travel.example.com/hotels/baixa-house again
- b_trip_window_ollama-cloud-gpt-oss-120b_r1 repeated: t13 fetch_page travel.example.com/hotels/alfama-patio again

