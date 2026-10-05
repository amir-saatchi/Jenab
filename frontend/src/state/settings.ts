import { create } from "zustand"

import { SettingsService, type Settings, type SettingsView } from "@/lib/api"
import { applyTheme, readTheme, type ThemeChoice } from "@/lib/theme"

interface SettingsState {
  view: SettingsView | null
  load(): Promise<void>
  save(next: Settings): Promise<void>
  setTheme(t: ThemeChoice): Promise<void>
}

export const useSettings = create<SettingsState>((set, get) => ({
  view: null,
  load: async () => {
    const view = await SettingsService.Get()
    applyTheme(readTheme(view.settings.ui.theme))
    set({ view })
  },
  save: async (next) => {
    const view = await SettingsService.Save(next)
    applyTheme(readTheme(view.settings.ui.theme))
    set({ view })
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
