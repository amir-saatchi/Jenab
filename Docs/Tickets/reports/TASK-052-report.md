# TASK-052 report: pick the provider from a list in *Add provider*

## What changed

**Go: the preset list** (`internal/provider/presets.go`)
- `Preset` has a new field `Local`: the provider runs on this computer and needs no key. `Preset.Host()` returns the host it talks to (the base URL's, or the kind's own), e.g. `api.anthropic.com`.
- The list is in the form's order: Anthropic, OpenAI, Google Gemini, Groq, OpenRouter, Z.ai, Cloudflare Workers AI and Ollama Cloud, then Ollama and LM Studio.
- New **LM Studio** preset: *OpenAI-compatible* with `http://localhost:1234/v1/`, `Local`. (The trailing slash matches the other presets.)
- "Ollama on this computer" is now just "Ollama", since the group label says where it runs.

**Go: what the frontend gets** (`internal/app/connect.go`)
- `PresetItem` has two new fields: `group` (`"cloud"`, or `"local"` for `Local` presets) and `host`. `no_key` now comes from `Local`, not from "Ollama kind with an empty base URL".
- A running Ollama still comes first and is marked `running`. Only Ollama is checked, because `ollamaUp` asks Ollama's own port.

**Go: a provider on this computer may connect without a key** (`internal/provider/registry.go`)
- Before this, only a local Ollama could connect without a key. *Connect* refused LM Studio with "An API key is needed", and calls refused it too ("no key is stored").
- Now both checks use one rule for every kind: an empty key is allowed when the base URL is this computer (loopback). `localURL` takes the kind, so an empty base URL means the kind's own URL. Anthropic, OpenAI and Gemini with their own URLs still need a key.
- This uses the loopback check the code already had for Ollama and for http-only-on-this-machine. It doesn't guess a provider from the URL.
- With no key, the OpenAI SDK sends an empty bearer token to localhost. `WithAPIKey("")` overrides any `OPENAI_API_KEY` environment variable, so that key can't leak.

**Frontend** (`frontend/src/chat/provider-card.tsx`, new `frontend/src/chat/provider-choice.ts`)
- The row of toggle buttons is now a shadcn Select labelled *Provider*, with the placeholder "Choose a provider". `SelectItem`s sit inside `SelectGroup`s with a `SelectLabel` each.
- The groups come from the Go list (`group`), in the order the list first names them, so a running Ollama puts *On this computer* first (SPEC 3.9). *Other* comes last and holds *OpenAI-compatible…*.
- Each item has a muted second line:
  - the host for a cloud provider;
  - "no key needed" for a local one ("running · no key needed" for a running Ollama);
  - "any base URL, set by hand" for *Other*.
- The trigger shows only the name. Items set `textValue` to the name, so typeahead matches the name. The accessible name is "name host", with a space between them.
- The fields follow the choice:
  - nothing but the Select before a choice;
  - the key field only when the preset needs one;
  - a field for each base-URL placeholder, e.g. *Account ID* for Cloudflare, described as "Part of the base URL · stored in the OS keychain";
  - Name and Base URL for *Other*.
- For a local preset, a line says "No key is needed. Start LM Studio on this computer first.", or "Ollama runs on this computer; no key is needed." when Ollama runs.
- The form opens on a running local provider if there is one, else with nothing chosen.
- A preset fills only the kind and base URL, as before. The form doesn't show the preset's base URL for editing, which is also as before; any URL goes through *Other*.
- The logic sits in `provider-choice.ts` (`providerGroups`, `presetHint`, `firstPick`, `formFor`, `fieldLabel`), so `bun test` can check it without a DOM.
- `ConnectForm` is shared, so the first-run provider card in the chat gets the same Select. Nothing in `settings/models.tsx`, `memory.tsx` or `project-settings.tsx` changed.

**Bindings:** regenerated with `wails3 generate bindings -clean=true -ts -i -d frontend/bindings ./cmd/desktop`. Only `PresetItem` in `internal/app/models.ts` changed.

## Checks
- `bun test`: 144 pass, 0 fail (29 files), including 8 new tests in `frontend/src/chat/provider-choice.test.ts`:
  - the three groups, in order, with every item's second line;
  - a running Ollama puts its group first and is picked;
  - nothing is asked before a choice;
  - a cloud provider asks for a key;
  - Ollama and LM Studio hide the key field;
  - Cloudflare gets an *Account ID* field;
  - *Other* asks for the name, base URL and key;
  - a replaced key keeps its placeholders.
- `bun run build` (tsc + vite): passes. The only warning is the existing chunk-size one.
- With `GOPROXY=off GOMEMLIMIT=1500MiB`:
  - `go build ./...`: OK.
  - `go vet ./...`: OK.
  - `go test -p 2 -short -count=1 ./...`: all 31 packages ok.
- New and changed Go tests:
  - `TestPresetList`: the whole list in order, with kind, host and `Local`; the LM Studio base URL; Cloudflare's placeholder.
  - `TestPresetsConnect`: local presets now connect with no key, and every cloud preset fails with `ErrNeedKey` when it has none.
  - `TestLocalKey` replaces `TestOllamaKey`: no key works for a local Ollama and for OpenAI-compatible on localhost. It fails for Ollama Cloud, Groq, and Anthropic or OpenAI at their own URLs, on both *Connect* and a call.
  - `internal/app` `TestPresets`: groups, host, and LM Studio's `no_key`.
- `gofmt -l` (CRs stripped) on every changed Go file: clean.
- **By hand:** a server-mode build (`go build -tags server ./cmd/desktop`) run with `LOCALAPPDATA`, `APPDATA` and `USERPROFILE` pointed at a scratch folder, on 127.0.0.1:9352, in the Browser pane at 1100×1000.
  - I opened *Settings → Models → Add provider*, opened the list, and picked Anthropic, Cloudflare, LM Studio and *Other*. I entered no keys and pressed no Connect.
  - Connect is enabled for LM Studio with no key.
  - Keyboard use works: End and Enter pick *Other*.
  - The only console errors were from my two server restarts.

## Screenshots
Outside the repo, not committed: `C:\Users\<user>\AppData\Local\Temp\claude\25b35c126a232c5d\2472dd82-e41f-47bc-95af-5f88283f0308\scratchpad\screenshots\`. The user name is left out here because the repo is public; Main-Session has the full path in the hand-off message.
- `light-1-list-top.jpg`, `light-2-list-full.jpg`: the open list.
- `light-3-anthropic.jpg`, `light-4-cloudflare.jpg`, `light-5-lmstudio.jpg`: the form for each.
- `dark-1-list-top.jpg`, `dark-2-list-full.jpg`, `dark-3-anthropic.jpg`, `dark-4-cloudflare.jpg`, `dark-5-lmstudio.jpg`: the same in dark.
- `dark-6-other.jpg`: *Other*.

The Browser pane was hidden for the dark shots, so the open animation froze halfway. For those shots only, I turned off CSS animations in the page from the console. No source change.

## Notes for review
- **The list is taller than a small window.** With 11 two-line items it is about 590 px. In a 760 px window it scrolls, as any long Select does. It shows whole at 1000 px once scrolled.
- **The no-key rule now covers any kind on this computer**, not only Ollama. That was needed for LM Studio to work at all. *Other* still always asks for a key: the frontend doesn't read the URL. A local server added by hand needs any text as its key.
- **`ProviderRow` in *Settings → Models* still treats only a local Ollama as "On this computer · no key".** A connected LM Studio row says "Key in the keychain" and offers *Replace key*. I left it out of scope; it could become a small follow-up.
- **The Select's check mark is pinned to the right** (`right-2`, `pr-8` in `components/ui/select.tsx`). This is the existing shadcn component, as in the default and fast model pickers. In a right-to-left layout it would sit on the far side. The item text itself is app text and flows with the page.

## Proposed Result paragraph
**Result:** *Add provider* has a *Provider* Select ("Choose a provider") with three groups. **Cloud** has Anthropic, OpenAI, Google Gemini, Groq, OpenRouter, Z.ai, Cloudflare Workers AI and Ollama Cloud, each with its host on a muted second line. **On this computer** has Ollama and LM Studio, with "no key needed". **Other** has *OpenAI-compatible…*. The groups and hosts come from the Go preset list: `Preset.Local` and `Host()`, sent as `group` and `host` in `PresetItem`. A running Ollama still comes first and is picked. The fields follow the choice: a key field only when one is needed, a field per base-URL placeholder (Cloudflare's *Account ID*), and Name and Base URL for *Other*. The LM Studio preset is *OpenAI-compatible* with `http://localhost:1234/v1/` and no key. To make it work, the rule that let only a local Ollama connect without a key now covers any kind whose base URL is this computer, for *Connect* and for calls. Tests: 8 frontend tests (`provider-choice.test.ts`), plus Go tests for the preset list (`TestPresetList`), keyless local presets (`TestPresetsConnect`, `TestLocalKey`) and `PresetItem` (`TestPresets`). The ticket's screenshots were taken in light and dark: the open list and the forms for Anthropic, Cloudflare and LM Studio. Left open: the *Settings → Models* row still shows "Key in the keychain" for a connected LM Studio.
