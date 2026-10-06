import { create } from "zustand"

// A place is what the main area shows. There are no tabs (SPEC 5.12): one
// chat, Settings, a project's settings or a developer tool (8.4).
// Settings may open at a section; the turn inspector at a turn.
export type Place =
  | { view: "chat"; project: string; chat: string }
  | { view: "settings"; section?: string }
  | { view: "project-settings"; project: string }
  | { view: "inspector"; project?: string; chat?: string; turn?: number }
  | { view: "runtime" }

export interface ChatRef {
  project: string
  chat: string
}

export function samePlace(a: Place | undefined, b: Place | undefined) {
  if (!a || !b || a.view !== b.view) return false
  if (a.view === "chat" && b.view === "chat") return a.project === b.project && a.chat === b.chat
  if (a.view === "project-settings" && b.view === "project-settings") return a.project === b.project
  if (a.view === "settings" && b.view === "settings") return a.section === b.section
  if (a.view === "inspector" && b.view === "inspector") return a.project === b.project && a.chat === b.chat && a.turn === b.turn
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

const noCycle = { cycle: null, cycleFrom: null }

function isChat(p: Place | undefined, c: ChatRef | undefined) {
  return p?.view === "chat" && !!c && p.project === c.project && p.chat === c.chat
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
  // when not cycling. cycleFrom is the history when the cycle began: the
  // chats passed on the way replace each other, so a cycle adds one place.
  cycle: number | null
  cycleFrom: History | null
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
  cycleFrom: null,
  sidebarHidden: false,
  // Opening a place, back and forward end a Ctrl+Tab cycle.
  go: (p) =>
    set((s) => ({
      history: push(s.history, p),
      mru: p.view === "chat" ? used(s.mru, p) : s.mru,
      ...noCycle,
    })),
  back: () => set((s) => ({ ...moved(s, step(s.history, -1)), ...noCycle })),
  forward: () => set((s) => ({ ...moved(s, step(s.history, 1)), ...noCycle })),
  // nextChat starts from the chat shown when it is the one used last, and
  // else from before the list, so the first Ctrl+Tab opens that chat.
  nextChat: (by) => {
    const { mru, cycle, history } = get()
    const start = cycle ?? (isChat(current({ history }), mru[0]) ? 0 : -1)
    if (mru.length === 0 || (start >= 0 && mru.length < 2)) return
    const at = start < 0 ? (by === 1 ? 0 : mru.length - 1) : (start + by + mru.length) % mru.length
    const from = get().cycleFrom ?? history
    set({ cycle: at, cycleFrom: from, history: push(from, { view: "chat", ...mru[at] }) })
  },
  endCycle: () => {
    const { cycle, mru } = get()
    if (cycle === null) return
    set({ mru: used(mru, mru[cycle]), ...noCycle })
  },
  setSidebarHidden: (sidebarHidden) => set({ sidebarHidden }),
}))

export function current(s: { history: History }): Place | undefined {
  return s.history.places[s.history.index]
}
