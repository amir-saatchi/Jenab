import { expect, test } from "bun:test"
import { readFileSync } from "node:fs"

// N-53: text meets WCAG AA (4.5:1) in both themes, read from the tokens in
// index.css.

type Tokens = Record<string, string>

function tokens(css: string, selector: ":root" | ".dark"): Tokens {
  const out: Tokens = {}
  const block = new RegExp(`(^|\\n)${selector.replace(".", "\\.")} \\{([^}]*)\\}`, "g")
  for (const m of css.matchAll(block)) {
    for (const d of m[2].matchAll(/--([\w-]+):\s*([^;]+);/g)) out[d[1]] = d[2].trim()
  }
  return out
}

// luminance is the WCAG relative luminance of an oklch() colour.
function luminance(value: string): number {
  const m = value.match(/^oklch\(([\d.]+) ([\d.]+) ([\d.]+)\)$/)
  if (!m) throw new Error(`not an opaque oklch colour: ${value}`)
  const [L, C, h] = [Number(m[1]), Number(m[2]), (Number(m[3]) * Math.PI) / 180]
  const a = C * Math.cos(h), b = C * Math.sin(h)
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3
  const mm = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3
  const clamp = (x: number) => Math.min(1, Math.max(0, x))
  const r = clamp(4.0767416621 * l - 3.3077115913 * mm + 0.2309699292 * s)
  const g = clamp(-1.2684380046 * l + 2.6097574011 * mm - 0.3413193965 * s)
  const bl = clamp(-0.0041960863 * l - 0.7034186147 * mm + 1.707614701 * s)
  return 0.2126 * r + 0.7152 * g + 0.0722 * bl
}

function ratio(fg: string, bg: string) {
  const [x, y] = [luminance(fg), luminance(bg)].sort((p, q) => q - p)
  return (x + 0.05) / (y + 0.05)
}

// pairs are the text colours and what they are shown on.
const pairs: [string, string][] = [
  ["foreground", "background"],
  ["card-foreground", "card"],
  ["popover-foreground", "popover"],
  ["primary-foreground", "primary"],
  ["secondary-foreground", "secondary"],
  ["muted-foreground", "muted"],
  ["muted-foreground", "background"],
  ["muted-foreground", "sidebar"],
  ["accent-foreground", "accent"],
  ["sidebar-foreground", "sidebar"],
  ["sidebar-accent-foreground", "sidebar-accent"],
  ["sidebar-primary-foreground", "sidebar-primary"],
  ["destructive", "background"],
  // the waiting count on its amber badge
  ["background", "tone-amber"],
  ...["neutral", "blue", "green", "amber", "red", "purple"].flatMap((t): [string, string][] => [
    [`tone-${t}`, "background"],
    [`tone-${t}`, `tone-${t}-soft`],
    [`tone-${t}`, "sidebar"],
  ]),
]

const css = readFileSync(new URL("./index.css", import.meta.url), "utf8")

for (const theme of [":root", ".dark"] as const) {
  test(`contrast ${theme === ":root" ? "light" : "dark"}`, () => {
    const t = { ...tokens(css, ":root"), ...(theme === ".dark" ? tokens(css, ".dark") : {}) }
    const low = pairs
      .map(([fg, bg]) => ({ fg, bg, r: ratio(t[fg], t[bg]) }))
      .filter((p) => p.r < 4.5)
      .map((p) => `${p.fg} on ${p.bg}: ${p.r.toFixed(2)}`)
    expect(low).toEqual([])
  })
}

test("luminance", () => {
  expect(ratio("oklch(0 0 0)", "oklch(1 0 0)")).toBeCloseTo(21, 1)
  expect(ratio("oklch(1 0 0)", "oklch(1 0 0)")).toBeCloseTo(1, 5)
  expect(ratio("oklch(0.708 0 0)", "oklch(1 0 0)")).toBeLessThan(4.5) // --ring is too light for text
})
