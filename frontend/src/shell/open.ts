import type { ProjectItem } from "@/lib/api"
import { showError } from "@/lib/errors"
import { useChats } from "@/state/chats"
import { current, samePlace, useNav, type Place } from "@/state/nav"
import { useProjects } from "@/state/projects"
import { useUI } from "@/state/ui"

// opening counts the openProject calls, so only the latest one moves.
let opening = 0

// openProject opens a project at chat, or else at the chat used last in
// it, or its Mother chat. It stays put when another project was asked for
// or the place changed while the project was opening.
export async function openProject(id: string, chat?: string) {
  const seq = ++opening
  const from = current(useNav.getState())
  try {
    const op = await useProjects.getState().open(id)
    if (seq !== opening || moved(from, current(useNav.getState()))) return
    const last = useNav.getState().mru.find((m) => m.project === id)
    openChat(id, chat ?? last?.chat ?? op.mother)
    if (!useChats.getState().byProject[id]) await useChats.getState().load(id)
  } catch (err) {
    showError(err)
  }
}

function moved(a: Place | undefined, b: Place | undefined) {
  return a !== b && !samePlace(a, b)
}

export function openChat(project: string, chat: string) {
  useNav.getState().go({ view: "chat", project, chat })
  useUI.getState().set({ leftOverlay: false })
}

export async function newChat(project: string) {
  try {
    openChat(project, await useChats.getState().create(project))
  } catch (err) {
    showError(err)
  }
}

// openMemory opens the settings with a memory section to edit, target
// memory:<scope>/<section>: user memory in Settings, project memory in the
// project's settings (SPEC 3.3).
export function openMemory(project: string, target: string) {
  useUI.getState().set({ memoryFocus: { project, target } })
  if (target.startsWith("memory:user/")) useNav.getState().go({ view: "settings", section: "Memory" })
  else useNav.getState().go({ view: "project-settings", project, section: "Memory" })
}

// useOpenProject is the project open behind Settings: the one of the chat
// used last, where *Back to chats* returns. None on the first-run screen.
export function useOpenProject(): ProjectItem | undefined {
  const id = useNav((s) => s.mru[0]?.project)
  const list = useProjects((s) => s.list)
  return openProjectOf(id, list)
}

// openProjectOf is the project with id in list; none for a project gone.
export function openProjectOf(id: string | undefined, list: ProjectItem[]): ProjectItem | undefined {
  return id ? list.find((p) => p.id === id) : undefined
}

// openProjectSettings opens the project's settings, at a section if given.
export function openProjectSettings(project: string, section?: string) {
  useNav.getState().go({ view: "project-settings", project, section })
  useUI.getState().set({ leftOverlay: false })
}

// openDesktop opens Settings › Desktop, the saved macros and recipes
// (SPEC 8.11).
export function openDesktop() {
  useNav.getState().go({ view: "settings", section: "Desktop" })
}

// backToChats leaves Settings for the chat used last, or the first-run
// screen when there is none.
export function backToChats() {
  const m = useNav.getState().mru[0]
  if (m) openChat(m.project, m.chat)
  else useNav.setState({ history: { places: [], index: -1 } })
}
