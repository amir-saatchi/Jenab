import { showError } from "@/lib/errors"
import { useChats } from "@/state/chats"
import { useNav } from "@/state/nav"
import { useProjects } from "@/state/projects"
import { useUI } from "@/state/ui"

// openProject opens a project at chat, or else at the chat used last in
// it, or its Mother chat.
export async function openProject(id: string, chat?: string) {
  try {
    const op = await useProjects.getState().open(id)
    const last = useNav.getState().mru.find((m) => m.project === id)
    openChat(id, chat ?? last?.chat ?? op.mother)
    if (!useChats.getState().byProject[id]) await useChats.getState().load(id)
  } catch (err) {
    showError(err)
  }
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

// backToChats leaves Settings for the chat used last, or the first-run
// screen when there is none.
export function backToChats() {
  const m = useNav.getState().mru[0]
  if (m) openChat(m.project, m.chat)
  else useNav.setState({ history: { places: [], index: -1 } })
}
