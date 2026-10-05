import { create } from "zustand"

import { SettingsService, type ModelGroup, type Settings, type SettingsView } from "@/lib/api"
import { applyTheme, readTheme, type ThemeChoice } from "@/lib/theme"

interface SettingsState {
  view: SettingsView | null
  // models are the models that are on, for the model picker.
  models: ModelGroup[] | null
  load(): Promise<void>
  loadModels(): Promise<void>
  save(next: Settings): Promise<void>
  setTheme(t: ThemeChoice): Promise<void>
}

export const useSettings = create<SettingsState>((set, get) => ({
  view: null,
  models: null,
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
