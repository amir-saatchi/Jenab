# P1-16 — Settings: models, keys and usage
**Type:** Feature
**Status:** Done
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

## Result
- **Package,** pinned: recharts 3.8.0 for the usage chart, which loads lazily. Switch, select, table, alert-dialog and chart were copied from the mockups.
- **Go additions:**
  - `SettingsService.Remove` deletes a provider, its limit and its key. Aliases that pointed at it move to another provider's models.
  - `ProviderModels` lists a provider's catalog models and its *Other models*, the ones the catalog doesn't know. A provider without a model list can have models typed in.
  - *Connect* with a saved name replaces the key and keeps the models.
  - `Usage` adds up the tokens of every project by local day, chat and model. Cost comes from the catalog prices; models without a price count as tokens only.
  - `project.MoveProjects`: a new data folder is saved, and at the next start, before any project opens, the project folders move there (a rename, or a copy across drives). A folder that fails stays and is tried again.
  - Saving `config.yaml` keeps a map's comment on its own line when the map gets its first entry or loses its last.
- **Settings page:**
  - *General:* the data folder with *Browse…*, the network and synced-folder warning, and a note after a move. The default approval level.
  - *Models:* a row per provider with its limit, *Models*, *Replace key* and *Remove*. The `default` and `fast` pickers, then *Limits*.
  - *Usage:* 7, 30 or 90 days. A chart, then tables by model and by chat.
  - *Developer:* the switch, with "Applies at the next start."
- **Done-when checks,** in server mode against a local fake provider with scratch data:
  - A provider connected with a key turned on its catalog models, and they showed in the model picker at once. So did an *Other model* turned on with its context window.
  - *Remove* deleted `provider:g` from Windows Credential Manager, and `default` moved to the remaining provider.
  - *Replace key* shows an empty field; the saved key is never sent to the frontend.
  - A provider limit saved from the page reached the running registry through `OnChange` (tested in `provider`). Every comment in `config.yaml` stayed on its line.
  - The data folder moved one project at the next start, and the page showed the note.
- **Tests:** Go tests for `ProviderModels`, re-keying, `Remove`, `Usage` with prices, `CheckFolder`, `MoveProjects` (including a copy that fails part way) and the map comments. `bun test` for the number and cost formats.
- **Not done:**
  - The move has no progress window; a large data folder delays the start.
  - The Ollama-first order and a local Ollama's limit of 1 were not tried with a real Ollama.
