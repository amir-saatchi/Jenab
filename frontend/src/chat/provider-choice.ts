import type { PresetItem } from "@/lib/api"
import { placeholders } from "@/settings/format"

// OTHER is the choice for an OpenAI-compatible provider set up by hand.
export const OTHER = "other"

export interface Choice {
  id: string
  name: string
  hint: string // the muted second line
}

export interface ChoiceGroup {
  key: string
  label: string
  items: Choice[]
}

const groupLabels: Record<string, string> = { cloud: "Cloud", local: "On this computer" }

// presetHint is a preset's second line: the host for a cloud provider,
// "no key needed" for one on this computer.
export function presetHint(p: PresetItem) {
  if (p.no_key) return p.running ? "running · no key needed" : "no key needed"
  return p.host
}

// providerGroups puts the presets in their groups, in the order the list
// names them first (a running Ollama comes first, SPEC 3.9), then Other.
export function providerGroups(presets: PresetItem[]): ChoiceGroup[] {
  const groups: ChoiceGroup[] = []
  for (const p of presets) {
    let g = groups.find((g) => g.key === p.group)
    if (!g) groups.push((g = { key: p.group, label: groupLabels[p.group] ?? p.group, items: [] }))
    g.items.push({ id: p.id, name: p.name, hint: presetHint(p) })
  }
  groups.push({ key: OTHER, label: "Other", items: [{ id: OTHER, name: "OpenAI-compatible…", hint: "any base URL, set by hand" }] })
  return groups
}

// firstPick is the choice the form opens with: a provider that runs on
// this computer, else none.
export function firstPick(presets: PresetItem[]) {
  return presets.find((p) => p.running)?.id ?? ""
}

export interface FormShape {
  preset?: PresetItem
  chosen: boolean // a provider is picked
  other: boolean // the name and base URL are typed in
  key: boolean // the key field is shown and needed
  fields: string[] // a field for each base-URL placeholder
}

// formFor is what the form asks for once pick is chosen. A replaced key
// keeps its provider's placeholders.
export function formFor(pick: string, presets: PresetItem[], replace?: { base_url?: string }): FormShape {
  if (replace) return { chosen: true, other: false, key: true, fields: placeholders(replace.base_url) }
  if (pick === OTHER) return { chosen: true, other: true, key: true, fields: [] }
  const preset = presets.find((p) => p.id === pick)
  if (!preset) return { chosen: false, other: false, key: false, fields: [] }
  return { preset, chosen: true, other: false, key: !preset.no_key, fields: preset.fields ?? [] }
}

// fieldLabel names a base-URL placeholder: account_id reads "Account ID".
export function fieldLabel(field: string) {
  const words = field.split("_").filter(Boolean).map((w) => (w.toLowerCase() === "id" ? "ID" : w))
  const s = words.join(" ")
  return s.charAt(0).toUpperCase() + s.slice(1)
}
