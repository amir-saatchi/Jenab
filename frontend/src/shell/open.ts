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

// backToChats leaves Settings for the chat used last, or the first-run
// screen when there is none.
export function backToChats() {
  const m = useNav.getState().mru[0]
  if (m) openChat(m.project, m.chat)
  else useNav.setState({ history: { places: [], index: -1 } })
}
