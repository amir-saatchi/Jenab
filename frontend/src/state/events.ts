import { Events } from "@wailsio/runtime"
import { toast } from "sonner"

import { ChatState, type ChatStatus, type Notice } from "@/lib/api"
import { showError } from "@/lib/errors"
import { useChats } from "@/state/chats"

// reloads holds the chat lists due to be read again, one timer per project.
const reloads = new Map<string, ReturnType<typeof setTimeout>>()

// reload reads a project's chat list again soon. A chat:status event
// carries no title, so a new chat or a generated title shows after it.
function reload(project: string) {
  if (reloads.has(project)) return
  reloads.set(
    project,
    setTimeout(() => {
      reloads.delete(project)
      useChats.getState().load(project).catch(showError)
    }, 300),
  )
}

function onStatus(s: ChatStatus) {
  const chats = useChats.getState()
  const items = chats.byProject[s.project]
  chats.status(s)
  if (items && (s.state === ChatState.StateIdle || !items.some((it) => it.chat.id === s.chat))) reload(s.project)
}

function onNotice(n: Notice) {
  if (n.kind === "damaged") toast.warning(n.text, { duration: Infinity })
  else toast.info(n.text)
}

// listen subscribes the stores to the Go events; it returns the unsubscribe.
export function listen() {
  const off = [
    Events.On("chat:status", (e) => onStatus(e.data)),
    Events.On("project:notice", (e) => onNotice(e.data)),
  ]
  return () => off.forEach((f) => f())
}
