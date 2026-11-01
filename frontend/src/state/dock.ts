import { create } from "zustand"

// Docked is what the main area shows next to the chat. Files come first;
// pages and pipelines add their kinds (SPEC 5.9).
export type Docked = { kind: "file"; key: string }

export type DockSide = "left" | "right"

// MIN_DOCK is the narrowest the dock gets, and the narrowest the main area
// next to it gets (SPEC 5.12).
export const MIN_DOCK = 320

export const chatKey = (project: string, chat: string) => `${project}/${chat}`

// The dock's layout. What is open, and whether the chat is hidden, belong
// to each chat; the side and the width to the project, its last choice
// (SPEC 5.9, 5.12). It lives for the session: saving it in
// chats.ui_state comes with pages.
interface DockState {
  open: Record<string, Docked | undefined> // by chatKey
  chatHidden: Record<string, boolean> // by chatKey
  drawer: Record<string, boolean> // by chatKey: the chat's drawer below 900 px
  side: Record<string, DockSide> // by project
  width: Record<string, number> // by project, in px; none is a third
  // listShown is the user's choice to show the chat list while something
  // is docked; it hides by default then (SPEC 5.12)
  listShown: boolean
  show(project: string, chat: string, d: Docked): void
  close(project: string, chat: string): void
  // closeFile closes key wherever it is open in project, as after a delete.
  closeFile(project: string, key: string): void
  setChatHidden(project: string, chat: string, hidden: boolean): void
  setDrawer(project: string, chat: string, open: boolean): void
  swap(project: string): void
  setWidth(project: string, px: number): void
  setListShown(shown: boolean): void
}

export const useDock = create<DockState>((set) => ({
  open: {},
  chatHidden: {},
  drawer: {},
  side: {},
  width: {},
  listShown: false,
  // Docking into a chat with nothing docked hides the chat list again.
  show: (project, chat, d) =>
    set((s) => {
      const k = chatKey(project, chat)
      return { open: { ...s.open, [k]: d }, drawer: { ...s.drawer, [k]: false }, listShown: s.open[k] ? s.listShown : false }
    }),
  close: (project, chat) =>
    set((s) => {
      const k = chatKey(project, chat)
      return { open: { ...s.open, [k]: undefined }, chatHidden: { ...s.chatHidden, [k]: false }, drawer: { ...s.drawer, [k]: false } }
    }),
  closeFile: (project, key) =>
    set((s) => {
      const open = { ...s.open }
      for (const [k, d] of Object.entries(open)) {
        if (d?.kind === "file" && d.key === key && k.startsWith(project + "/")) open[k] = undefined
      }
      return { open }
    }),
  setChatHidden: (project, chat, hidden) => set((s) => ({ chatHidden: { ...s.chatHidden, [chatKey(project, chat)]: hidden } })),
  setDrawer: (project, chat, open) => set((s) => ({ drawer: { ...s.drawer, [chatKey(project, chat)]: open } })),
  swap: (project) => set((s) => ({ side: { ...s.side, [project]: s.side[project] === "left" ? "right" : "left" } })),
  setWidth: (project, px) => set((s) => ({ width: { ...s.width, [project]: Math.round(px) } })),
  setListShown: (listShown) => set({ listShown }),
}))

// clampWidth keeps the dock at least MIN_DOCK wide and leaves the main
// area as much; a box too narrow for both gives the dock MIN_DOCK.
export function clampWidth(px: number, box: number) {
  return Math.max(MIN_DOCK, Math.min(px, box - MIN_DOCK))
}
