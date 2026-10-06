import { beforeEach, expect, mock, test } from "bun:test"

import type { Settings, SettingsView } from "@/lib/api"

// A SettingsService that saves like Go: Save replaces all the settings.
// Each Save waits until release is called, so edits can overlap.
let disk: Settings
const saves: Settings[] = []
const waiting: Array<() => void> = []
let failNext = false
const themes: string[] = []

const view = (s: Settings): SettingsView => ({ settings: structuredClone(s), problems: [] })
const release = async () => {
  while (waiting.length === 0) await Bun.sleep(1)
  waiting.shift()!()
  await Bun.sleep(1)
}

const fake = {
  Get: async () => view(disk),
  Models: async () => [],
  Started: async () => ({}),
  Remove: async () => view(disk),
  Connect: async (req: { name: string }) => {
    disk = structuredClone(disk)
    ;(disk as unknown as { providers: Record<string, unknown> }).providers = { [req.name]: {} }
    return { view: view(disk), provider: req.name, models: 0 }
  },
  Save: (next: Settings) =>
    new Promise<SettingsView>((resolve, reject) => {
      saves.push(structuredClone(next))
      waiting.push(() => {
        if (failNext) {
          failNext = false
          reject(new Error("disk full"))
          return
        }
        disk = structuredClone(next)
        resolve(view(disk))
      })
    }),
}

const api = await import("@/lib/api")
mock.module("@/lib/api", () => ({ ...api, SettingsService: fake }))
const theme = await import("@/lib/theme")
mock.module("@/lib/theme", () => ({ ...theme, applyTheme: (c: string) => themes.push(c) }))
const { useSettings } = await import("@/state/settings")

beforeEach(async () => {
  disk = { dev_tools: false, approvals: { default_level: "standard" }, ui: { theme: "system" } } as unknown as Settings
  saves.length = 0
  themes.length = 0
  failNext = false
  await useSettings.getState().load()
})

test("two quick edits both survive", async () => {
  const s = useSettings.getState
  const a = s().edit((x) => void (x.dev_tools = true))
  const b = s().edit((x) => void (x.approvals.default_level = "auto"))
  // Both show at once, before any save is back.
  expect(s().view?.settings.dev_tools).toBe(true)
  expect(s().view?.settings.approvals.default_level).toBe("auto")
  await release()
  await release()
  await Promise.all([a, b])
  expect(saves.length).toBe(2)
  expect(saves[1].dev_tools).toBe(true) // the second save starts from the first
  expect(disk.dev_tools).toBe(true)
  expect(disk.approvals.default_level).toBe("auto")
  expect(s().view?.settings).toEqual(disk)
})

test("a failed save takes its edit out, and the next edit still saves", async () => {
  const s = useSettings.getState
  failNext = true
  const a = s().edit((x) => void (x.dev_tools = true))
  const b = s().edit((x) => void (x.approvals.default_level = "strict"))
  await release()
  await expect(a).rejects.toThrow("disk full")
  expect(s().view?.settings.dev_tools).toBe(false)
  expect(s().view?.settings.approvals.default_level).toBe("strict") // still pending
  await release()
  await b
  expect(disk.dev_tools).toBe(false)
  expect(disk.approvals.default_level).toBe("strict")
})

test("setTheme applies at once, joins the queue, and goes back if its save fails", async () => {
  const s = useSettings.getState
  const a = s().edit((x) => void (x.dev_tools = true))
  failNext = false
  const t = s().setTheme("dark")
  expect(themes.at(-1)).toBe("dark")
  await release()
  await a
  expect(saves[0].ui.theme).toBe("system") // the theme waits for the edit before it
  await release()
  await t
  expect(disk.ui.theme).toBe("dark")
  expect(disk.dev_tools).toBe(true)

  failNext = true
  const u = s().setTheme("light")
  expect(themes.at(-1)).toBe("light")
  await release()
  await expect(u).rejects.toThrow("disk full")
  expect(themes.at(-1)).toBe("dark")
  expect(s().view?.settings.ui.theme).toBe("dark")
})

test("a connect waits for the save before it, and the next edit keeps the provider", async () => {
  const s = useSettings.getState
  const providers = (x: Settings) => (x as unknown as { providers?: Record<string, unknown> }).providers
  const a = s().edit((x) => void (x.dev_tools = true))
  const c = s().connect({ name: "groq" } as Parameters<ReturnType<typeof s>["connect"]>[0])
  await release()
  await Promise.all([a, c])
  expect(disk.dev_tools).toBe(true) // the connect started from the saved edit
  expect(providers(disk)).toEqual({ groq: {} })
  const b = s().edit((x) => void (x.ui.theme = "dark"))
  await release()
  await b
  expect(providers(saves.at(-1)!)).toEqual({ groq: {} })
})
