import { create } from "zustand"

// The Files panel's place in each project's bucket (SPEC 5.12).
interface FilesState {
  // folder is the folder shown, by project: "" for the top, else ending in "/"
  folder: Record<string, string>
  // rev counts the changes made from the app, by project, so the panel
  // lists the folder again
  rev: Record<string, number>
  setFolder(project: string, prefix: string): void
  changed(project: string): void
}

export const useFiles = create<FilesState>((set) => ({
  folder: {},
  rev: {},
  setFolder: (project, prefix) => set((s) => ({ folder: { ...s.folder, [project]: prefix } })),
  changed: (project) => set((s) => ({ rev: { ...s.rev, [project]: (s.rev[project] ?? 0) + 1 } })),
}))
