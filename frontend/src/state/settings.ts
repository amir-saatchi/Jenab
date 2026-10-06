import { create } from "zustand"

import {
  SettingsService,
  type ConnectRequest,
  type ConnectResult,
  type ModelGroup,
  type Settings,
  type SettingsView,
  type Started,
} from "@/lib/api"
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
  // edit makes a change to the settings. The view shows it at once; it is
  // saved after the edits before it, on top of the settings as last saved,
  // since Save replaces all of them. A failed save takes it out again.
  edit(change: (s: Settings) => void): Promise<void>
  // connect connects a provider, or replaces its key, in turn with the
  // edits, so no save in flight drops it.
  connect(req: ConnectRequest): Promise<ConnectResult>
  // removeProvider disconnects a provider and deletes its key.
  removeProvider(name: string): Promise<void>
  // setTheme is an edit: the theme applies at once, and a failed save puts
  // the saved theme back.
  setTheme(t: ThemeChoice): Promise<void>
}

interface Pending {
  change: (s: Settings) => void
}

// saved is the view as Go last sent it, and pending the edits not saved
// yet, in order. queue runs the calls that read or replace the settings one
// at a time, so each save starts from the one before it.
let saved: SettingsView | null = null
const pending: Pending[] = []
let queue: Promise<unknown> = Promise.resolve()

// enqueue runs job once the jobs before it are done, failed or not.
function enqueue<T>(job: () => Promise<T>): Promise<T> {
  const run = queue.then(job)
  queue = run.catch(() => {})
  return run
}

// withPending is the saved view with the pending edits made to a copy.
function withPending(): SettingsView | null {
  if (!saved || pending.length === 0) return saved
  const settings = structuredClone(saved.settings)
  for (const p of pending) p.change(settings)
  return { ...saved, settings }
}

export const useSettings = create<SettingsState>((set, get) => {
  // show puts the saved view with the pending edits on screen, and applies
  // its theme.
  const show = () => {
    const view = withPending()
    if (view) applyTheme(readTheme(view.settings.ui.theme))
    set({ view })
  }
  const keep = (view: SettingsView) => {
    saved = view
    show()
  }

  return {
    view: null,
    models: null,
    started: null,
    loadStarted: async () => {
      set({ started: await SettingsService.Started() })
    },
    loadModels: async () => {
      set({ models: (await SettingsService.Models()) ?? [] })
    },
    load: () => enqueue(async () => keep(await SettingsService.Get())),
    edit: (change) => {
      if (!saved) return Promise.resolve()
      const p: Pending = { change }
      pending.push(p)
      show()
      return enqueue(async () => {
        try {
          const next = structuredClone(saved!.settings)
          change(next)
          saved = await SettingsService.Save(next)
        } finally {
          pending.splice(pending.indexOf(p), 1)
          show()
        }
        await get().loadModels()
      })
    },
    connect: (req) =>
      enqueue(async () => {
        const r = await SettingsService.Connect(req)
        keep(r.view)
        await get().loadModels()
        return r
      }),
    removeProvider: (name) =>
      enqueue(async () => {
        try {
          keep(await SettingsService.Remove(name))
        } finally {
          const [view] = await Promise.all([SettingsService.Get(), get().loadModels()])
          keep(view)
        }
      }),
    setTheme: (t) =>
      get().edit((s) => {
        s.ui.theme = t
      }),
  }
})
