import type { UsageLine } from "@/lib/api"

const compact = new Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 })

// tokens is a token count in short form: 950, 12.5K, 1.2M.
export function tokens(n: number) {
  return compact.format(n)
}

export function total(l: UsageLine) {
  return l.input + l.output + l.cache_read + l.cache_write
}

// cost is a line's cost in US dollars. Tokens without a price add a "+";
// a line with only such tokens has no cost.
export function cost(l: UsageLine) {
  if (l.cost === 0 && l.unpriced > 0) return "—"
  const c = l.cost > 0 && l.cost < 0.01 ? "<$0.01" : `$${l.cost.toFixed(2)}`
  return l.unpriced > 0 ? c + "+" : c
}

// contextSize is a context window in short form, e.g. "1M" or "128K".
export function contextSize(n: number) {
  return n > 0 ? compact.format(n) : ""
}

// limitDefault is the background limit a provider gets with no setting of
// its own: 1 for a local Ollama, else the global limit (SPEC 7.6).
export function limitDefault(kind: string, baseURL: string | undefined, global: number) {
  return kind === "ollama" && isLocal(baseURL) ? 1 : global
}

function isLocal(baseURL: string | undefined) {
  if (!baseURL) return true
  try {
    const h = new URL(baseURL).hostname
    return h === "localhost" || h === "127.0.0.1" || h === "[::1]" || h === "::1"
  } catch {
    return false
  }
}

// placeholders are the {field} names in a base URL, e.g. account_id.
export function placeholders(baseURL: string | undefined) {
  return [...(baseURL ?? "").matchAll(/\{([A-Za-z0-9_]+)\}/g)].map((m) => m[1])
}

// readNumber reads what was typed in a whole-number field that shows shown:
// the same, bad (not a whole number, or below min), or the value to save.
// With optional, an empty field is null.
export function readNumber(text: string, shown: string, min: number, optional?: boolean): "same" | "bad" | { value: number | null } {
  const t = text.trim()
  if (t === shown) return "same"
  if (t === "") return optional ? { value: null } : "bad"
  const n = Number(t)
  return Number.isInteger(n) && n >= min ? { value: n } : "bad"
}
