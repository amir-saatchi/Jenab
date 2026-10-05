import { create } from "zustand"

// A place is what the main area shows. There are no tabs (SPEC 5.12): one
// chat, Settings or a project's settings.
export type Place =
  | { view: "chat"; project: string; chat: string }
  | { view: "settings" }
  | { view: "project-settings"; project: string }

export interface ChatRef {
  project: string
  chat: string
}

export function samePlace(a: Place | undefined, b: Place | undefined) {
  if (!a || !b || a.view !== b.view) return false
  if (a.view === "chat" && b.view === "chat") return a.project === b.project && a.chat === b.chat
  if (a.view === "project-settings" && b.view === "project-settings") return a.project === b.project
  return true
}

const MAX_HISTORY = 100

// History is back and forward through the places opened (Alt+← and Alt+→).
export interface History {
  places: Place[]
  index: number
}

export function push(h: History, p: Place): History {
  if (samePlace(h.places[h.index], p)) return h
  const places = [...h.places.slice(0, h.index + 1), p].slice(-MAX_HISTORY)
  return { places, index: places.length - 1 }
}

export function step(h: History, by: -1 | 1): History {
  const index = h.index + by
  return index < 0 || index >= h.places.length ? h : { ...h, index }
}

// used moves the chat to the front of the most-recently-used list that
// Ctrl+Tab walks.
export function used(mru: ChatRef[], c: ChatRef): ChatRef[] {
  const ref = { project: c.project, chat: c.chat }
  return [ref, ...mru.filter((m) => m.chat !== c.chat || m.project !== c.project)].slice(0, 50)
}

// moved is the state after back or forward: the chat landed on counts as used.
function moved(s: { mru: ChatRef[] }, history: History) {
  const p = history.places[history.index]
  return { history, mru: p?.view === "chat" ? used(s.mru, p) : s.mru }
}

interface NavState {
  history: History
  mru: ChatRef[]
  // cycle is how far back Ctrl+Tab has gone while Ctrl is held; null
  // when not cycling.
  cycle: number | null
  // sidebarHidden hides the left sidebar and puts its chats into the rail,
  // as while a page is open (5.12).
  sidebarHidden: boolean
  go(p: Place): void
  back(): void
  forward(): void
  nextChat(by: 1 | -1): void
  endCycle(): void
  setSidebarHidden(hidden: boolean): void
}

export const useNav = create<NavState>((set, get) => ({
  history: { places: [], index: -1 },
  mru: [],
  cycle: null,
  sidebarHidden: false,
  go: (p) =>
    set((s) => ({
      history: push(s.history, p),
      mru: p.view === "chat" ? used(s.mru, p) : s.mru,
    })),
  back: () => set((s) => moved(s, step(s.history, -1))),
  forward: () => set((s) => moved(s, step(s.history, 1))),
  nextChat: (by) => {
    const { mru, cycle } = get()
    if (mru.length < 2) return
    const at = ((cycle ?? 0) + by + mru.length) % mru.length
    const c = mru[at]
    set((s) => ({ cycle: at, history: push(s.history, { view: "chat", ...c }) }))
  },
  endCycle: () => {
    const { cycle, mru } = get()
    if (cycle === null) return
    set({ cycle: null, mru: used(mru, mru[cycle]) })
  },
  setSidebarHidden: (sidebarHidden) => set({ sidebarHidden }),
}))

export function current(s: { history: History }): Place | undefined {
  return s.history.places[s.history.index]
}
