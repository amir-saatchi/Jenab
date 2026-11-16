# TASK-052 — Pick the provider from a list in *Add provider*
**Type:** Task
**Status:** Done
**Gate:** 3
**Needs:** —

## Goal
*Add provider* (`frontend/src/settings/models.tsx`) shows the presets as a row of tag-like buttons. The user finds this a poor fit (2026-11-15): it reads as tags, not as one choice, and it doesn't say which are local or what each is. The list also lacks LM Studio, which SPEC 3.9 names.

## Scope
1. **A Select** (`components/ui/select.tsx`, already installed) labelled *Provider*, placeholder "Choose a provider", with three groups:
   - **Cloud:** Anthropic, OpenAI, Google Gemini, Groq, OpenRouter, Z.ai, Cloudflare Workers AI, Ollama Cloud.
   - **On this computer:** Ollama, LM Studio.
   - **Other:** *OpenAI-compatible…*, the form by hand.
   Each item has a second, muted line: the host for a cloud provider, "no key needed" for a local one.
2. **The fields follow the choice:** the key field only when the provider needs a key; a field for each base-URL placeholder, such as Cloudflare's `{account_id}` (SPEC 3.9); the base URL for *Other*. As today, a preset fills only the kind and base URL, and the user can edit both.
3. **LM Studio preset** in `internal/provider/presets.go`: *OpenAI-compatible* with `http://localhost:1234/v1`, no key, as SPEC 3.9 says.
4. The presets' groups and second lines come from the preset list (Go), not a second list in the frontend.

## Rules
- No new dependency. If the list passes about 15, a searchable combobox is a later ticket.
- No provider is special-cased beyond the preset list (SPEC 1): no URL detection, nothing set up when the app ships.
- shadcn Select as the project's shadcn skill describes; right-to-left text as elsewhere (5.8).

## Done when
- Frontend tests: the groups and second lines, the key field hidden for a local provider, a placeholder field for Cloudflare, *Other* shows the base URL.
- A Go test for the preset list, LM Studio included.
- Screenshots of the open list and of the form for Anthropic, Cloudflare and LM Studio, in light and dark.

## Result
Done on 2026-11-16. Report: [TASK-052-report](reports/TASK-052-report.md).

- **A *Provider* Select** ("Choose a provider") with three groups: **Cloud** (Anthropic, OpenAI, Google Gemini, Groq, OpenRouter, Z.ai, Cloudflare Workers AI, Ollama Cloud), each with its host on a muted second line; **On this computer** (Ollama, LM Studio), "no key needed"; **Other** (*OpenAI-compatible…*). A running Ollama still comes first and is picked.
- **The groups and hosts come from the Go preset list:** `Preset.Local` and `Host()`, sent as `group` and `host`. "Ollama on this computer" is now "Ollama", since the group says where it runs.
- **The fields follow the choice:** a key field only when one is needed, a field per base-URL placeholder (Cloudflare's *Account ID*), and Name and Base URL for *Other*.
- **LM Studio preset:** *OpenAI-compatible* with `http://localhost:1234/v1/`, no key. For it to work, the rule that let only a local Ollama connect without a key now covers any kind whose base URL is this computer, for *Connect* and for calls. Providers at their own remote URLs still need a key.
- **Checks:** 8 frontend tests, Go tests for the preset list, keyless local presets and `PresetItem`. The bindings match a fresh generation. Screenshots of the list and of the forms for Anthropic, Cloudflare and LM Studio, in light and dark, are kept outside the repo.
- **Open:** *Settings → Models* still says "Key in the keychain" for a connected LM Studio, which has no key.
