// The theme (SPEC 5.11): a dark class on the root. config.yaml holds the
// choice; localStorage keeps a copy so index.html can set the class before
// the first paint.
export type ThemeChoice = "light" | "dark" | "system"

export const THEME_KEY = "jenab.theme"

const systemDark = () => window.matchMedia("(prefers-color-scheme: dark)")

let choice: ThemeChoice = "system"

export function isDark(c: ThemeChoice, system: boolean) {
  return c === "dark" || (c === "system" && system)
}

function paint() {
  document.documentElement.classList.toggle("dark", isDark(choice, systemDark().matches))
}

export function applyTheme(c: ThemeChoice) {
  choice = c
  localStorage.setItem(THEME_KEY, c)
  paint()
}

export function readTheme(v: unknown): ThemeChoice {
  return v === "light" || v === "dark" ? v : "system"
}

// watchSystem makes an OS change apply at once with System.
export function watchSystem() {
  systemDark().addEventListener("change", () => {
    if (choice === "system") paint()
  })
}
