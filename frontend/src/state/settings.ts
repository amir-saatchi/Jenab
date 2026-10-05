import { create } from "zustand"

import { SettingsService, type ModelGroup, type Settings, type SettingsView, type Started } from "@/lib/api"
import { applyTheme, readTheme, type ThemeChoice } from "@/lib/theme"

interface SettingsState {
  view: SettingsView | null
  // models are the models that are on, for the model picker.
  models: ModelGroup[] | null
  // started is what this start of the app used: the data folder and the
  // developer tools apply at the next start.
  started: Started | null
  load(): Promise<void>
  loadModels(): Promise<void>
  loadStarted(): Promise<void>
  save(next: Settings): Promise<void>
  // edit saves a change made to a copy of the settings.
  edit(change: (s: Settings) => void): Promise<void>
  // removeProvider disconnects a provider and deletes its key.
  removeProvider(name: string): Promise<void>
  setTheme(t: ThemeChoice): Promise<void>
}

export const useSettings = create<SettingsState>((set, get) => ({
  view: null,
  models: null,
  started: null,
  loadStarted: async () => {
    set({ started: await SettingsService.Started() })
  },
  loadModels: async () => {
    set({ models: (await SettingsService.Models()) ?? [] })
  },
  load: async () => {
    const view = await SettingsService.Get()
    applyTheme(readTheme(view.settings.ui.theme))
    set({ view })
  },
  save: async (next) => {
    const view = await SettingsService.Save(next)
    applyTheme(readTheme(view.settings.ui.theme))
    set({ view })
    await get().loadModels()
  },
  edit: async (change) => {
    const view = get().view
    if (!view) return
    const next = structuredClone(view.settings)
    change(next)
    await get().save(next)
  },
  removeProvider: async (name) => {
    try {
      set({ view: await SettingsService.Remove(name) })
    } finally {
      await Promise.all([get().load(), get().loadModels()])
    }
  },
  // setTheme applies at once and then saves; a failed save puts the
  // saved theme back.
  setTheme: async (t) => {
    const view = get().view
    if (!view) return
    applyTheme(t)
    try {
      await get().save({ ...view.settings, ui: { ...view.settings.ui, theme: t } })
    } catch (err) {
      applyTheme(readTheme(view.settings.ui.theme))
      throw err
    }
  },
}))
