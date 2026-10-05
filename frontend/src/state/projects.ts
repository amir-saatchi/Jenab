import { create } from "zustand"

import { ProjectService, type FolderWarning, type OpenedProject, type ProjectItem } from "@/lib/api"

interface ProjectsState {
  loaded: boolean
  list: ProjectItem[]
  // opened are the projects opened this session, with their Mother chat
  // and approval level.
  opened: Record<string, OpenedProject>
  // folderWarning is the 2.1 warning for the data folder, or null.
  folderWarning: FolderWarning | null
  load(): Promise<void>
  create(name: string): Promise<OpenedProject>
  open(id: string): Promise<OpenedProject>
  setLevel(id: string, level: string): void
}

export const useProjects = create<ProjectsState>((set, get) => ({
  loaded: false,
  list: [],
  opened: {},
  folderWarning: null,
  load: async () => {
    const [list, folderWarning] = await Promise.all([ProjectService.List(), ProjectService.FolderWarning()])
    set({ loaded: true, list: list ?? [], folderWarning })
  },
  create: async (name) => {
    const op = await ProjectService.Create(name)
    const list = (await ProjectService.List()) ?? []
    set((s) => ({ list, opened: { ...s.opened, [op.id]: op } }))
    return op
  },
  open: async (id) => {
    const known = get().opened[id]
    if (known) return known
    const op = await ProjectService.Open(id)
    set((s) => ({ opened: { ...s.opened, [op.id]: op } }))
    return op
  },
  setLevel: (id, level) =>
    set((s) => (s.opened[id] ? { opened: { ...s.opened, [id]: { ...s.opened[id], level } } } : s)),
}))

// initial is a project's mark in the rail: its first letter.
export function initial(name: string) {
  return (Array.from(name.trim())[0] ?? "?").toUpperCase()
}

// initials is a chat's mark in the rail: the first letters of its first two
// words, or its first two characters.
export function initials(title: string) {
  const words = title.trim().split(/\s+/).filter(Boolean)
  if (words.length === 0) return "?"
  const first = (w: string) => Array.from(w)[0] ?? ""
  return words.length > 1 ? (first(words[0]) + first(words[1])).toUpperCase() : Array.from(words[0]).slice(0, 2).join("")
}
