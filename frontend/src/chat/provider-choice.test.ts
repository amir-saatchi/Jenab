import { expect, test } from "bun:test"

import type { PresetItem } from "@/lib/api"
import { OTHER, fieldLabel, firstPick, formFor, providerGroups } from "@/chat/provider-choice"

// The list as SettingsService.Presets sends it (internal/provider/presets.go).
const preset = (id: string, name: string, kind: string, base_url: string, group: string, host: string, extra: Partial<PresetItem> = {}): PresetItem => ({
  id,
  name,
  kind,
  base_url,
  group,
  host,
  fields: [...base_url.matchAll(/\{(\w+)\}/g)].map((m) => m[1]),
  no_key: group === "local",
  running: false,
  ...extra,
})

const presets: PresetItem[] = [
  preset("anthropic", "Anthropic", "anthropic", "", "cloud", "api.anthropic.com"),
  preset("openai", "OpenAI", "openai", "", "cloud", "api.openai.com"),
  preset("gemini", "Google Gemini", "gemini", "", "cloud", "generativelanguage.googleapis.com"),
  preset("groq", "Groq", "openai_compatible", "https://api.groq.com/openai/v1/", "cloud", "api.groq.com"),
  preset("openrouter", "OpenRouter", "openai_compatible", "https://openrouter.ai/api/v1/", "cloud", "openrouter.ai"),
  preset("zai", "Z.ai", "openai_compatible", "https://api.z.ai/api/paas/v4/", "cloud", "api.z.ai"),
  preset("cloudflare", "Cloudflare Workers AI", "openai_compatible", "https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1/", "cloud", "api.cloudflare.com"),
  preset("ollama-cloud", "Ollama Cloud", "ollama", "https://ollama.com/", "cloud", "ollama.com"),
  preset("ollama", "Ollama", "ollama", "", "local", "localhost:11434"),
  preset("lmstudio", "LM Studio", "openai_compatible", "http://localhost:1234/v1/", "local", "localhost:1234"),
]

test("the providers come in three groups, each with a second line", () => {
  const groups = providerGroups(presets)
  expect(groups.map((g) => g.label)).toEqual(["Cloud", "On this computer", "Other"])
  expect(groups[0].items.map((c) => c.name)).toEqual([
    "Anthropic", "OpenAI", "Google Gemini", "Groq", "OpenRouter", "Z.ai", "Cloudflare Workers AI", "Ollama Cloud",
  ])
  expect(groups[0].items[0].hint).toBe("api.anthropic.com")
  expect(groups[0].items[6].hint).toBe("api.cloudflare.com")
  expect(groups[1].items).toEqual([
    { id: "ollama", name: "Ollama", hint: "no key needed" },
    { id: "lmstudio", name: "LM Studio", hint: "no key needed" },
  ])
  expect(groups[2].items.map((c) => [c.id, c.name])).toEqual([[OTHER, "OpenAI-compatible…"]])
  expect(groups.flatMap((g) => g.items).every((c) => c.hint !== "")).toBe(true)
})

test("a running Ollama puts its group first and is picked", () => {
  const running = presets.map((p) => (p.id === "ollama" ? { ...p, running: true } : p))
  const ps = [running.find((p) => p.running)!, ...running.filter((p) => !p.running)] // as the Go list orders it
  const groups = providerGroups(ps)
  expect(groups.map((g) => g.label)).toEqual(["On this computer", "Cloud", "Other"])
  expect(groups[0].items[0].hint).toBe("running · no key needed")
  expect(firstPick(ps)).toBe("ollama")
  expect(firstPick(presets)).toBe("")
})

test("nothing is asked before a provider is chosen", () => {
  expect(formFor("", presets)).toEqual({ chosen: false, other: false, key: false, fields: [] })
})

test("a cloud provider asks for its key", () => {
  const f = formFor("anthropic", presets)
  expect([f.chosen, f.other, f.key, f.fields]).toEqual([true, false, true, []])
  expect(f.preset?.base_url).toBe("")
})

test("a provider on this computer hides the key field", () => {
  for (const id of ["ollama", "lmstudio"]) {
    const f = formFor(id, presets)
    expect([f.chosen, f.key, f.other, f.fields]).toEqual([true, false, false, []])
  }
  expect(formFor("lmstudio", presets).preset?.base_url).toBe("http://localhost:1234/v1/")
})

test("Cloudflare gets a field for its account ID", () => {
  const f = formFor("cloudflare", presets)
  expect([f.key, f.fields]).toEqual([true, ["account_id"]])
  expect(fieldLabel("account_id")).toBe("Account ID")
  expect(fieldLabel("region")).toBe("Region")
})

test("Other asks for the name, base URL and key", () => {
  expect(formFor(OTHER, presets)).toEqual({ chosen: true, other: true, key: true, fields: [] })
})

test("a new key keeps the provider's placeholders", () => {
  expect(formFor("", [], { base_url: "https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1/" })).toEqual({
    chosen: true, other: false, key: true, fields: ["account_id"],
  })
})
