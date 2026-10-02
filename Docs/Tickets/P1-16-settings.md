# P1-16 — Settings: models, keys and usage
**Type:** Feature
**Status:** Open
**Gate:** 3 (Phase 1)
**Needs:** P1-14, P1-08
**Requirements:** R-91, R-119, N-21, N-43

## Goal
The settings a Phase 1 user needs (SPEC 3.9).

## Scope
- ***Settings → Models*:**
  - *Connect* for each provider, with the key going to the keychain
  - the catalog models and *Other models*
  - the `default` and `fast` aliases
  - Ollama listed first when it runs on this machine
  - *Limits*: the limits of SPEC 7.6, with a limit per provider on each provider's row
- ***Settings → General*:** the data folder, and the default approval level.
- ***Settings → Usage*:** tokens per day, per chat and per model, with the cost from the catalog.
- ***Settings → Developer*:** turns the developer tools on.

## Done when
- A key added here turns on its catalog models in the model picker at once.
- Removing a provider deletes its key from the keychain.
- A saved key is never shown again.
- A changed limit applies without a restart, and a comment the user wrote in `config.yaml` is still there after saving.
