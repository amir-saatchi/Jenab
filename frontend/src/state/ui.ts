import { create } from "zustand"

// What is open in the shell besides the main area.
interface UIState {
  newProject: boolean
  // overlays below 900 px (SPEC 5.12): the rail with the chats, and the
  // right sidebar
  leftOverlay: boolean
  rightOverlay: boolean
  // a memory section to open for editing in the settings, the [edit] of a
  // chat's chip (SPEC 3.3): its project and target, memory:<scope>/<section>.
  // The settings page that shows it clears it.
  memoryFocus: { project: string; target: string } | null
  set(p: Partial<Omit<UIState, "set">>): void
}

export const useUI = create<UIState>((set) => ({
  newProject: false,
  leftOverlay: false,
  rightOverlay: false,
  memoryFocus: null,
  set: (p) => set(p),
}))
