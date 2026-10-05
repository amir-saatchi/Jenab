import { create } from "zustand"

// What is open in the shell besides the main area.
interface UIState {
  newProject: boolean
  // overlays below 900 px (SPEC 5.12): the rail with the chats, and the
  // right sidebar
  leftOverlay: boolean
  rightOverlay: boolean
  set(p: Partial<Omit<UIState, "set">>): void
}

export const useUI = create<UIState>((set) => ({
  newProject: false,
  leftOverlay: false,
  rightOverlay: false,
  set: (p) => set(p),
}))
