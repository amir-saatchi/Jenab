import { Events } from "@wailsio/runtime"
import { toast } from "sonner"

import { ChatState, PartKind, SystemService, type ChatStatus, type Notice } from "@/lib/api"
import { showError } from "@/lib/errors"
import { useChats } from "@/state/chats"
import { current, useNav } from "@/state/nav"
import { useProjects } from "@/state/projects"
import { useThreads } from "@/state/thread"
import { openProject } from "@/shell/open"

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

// states is each chat's last state, to see when it starts to wait.
const states = new Map<string, ChatState>()

function onStatus(s: ChatStatus) {
  const chats = useChats.getState()
  const items = chats.byProject[s.project]
  const before = states.get(s.chat)
  states.set(s.chat, s.state)
  chats.status(s)
  useThreads.getState().status(s)
  if (items && (s.state === ChatState.StateIdle || !items.some((it) => it.chat.id === s.chat))) reload(s.project)
  if (s.state === ChatState.StateWaiting && before !== ChatState.StateWaiting && s.waiting && !onScreen(s)) notify(s)
}

// onScreen tells whether the user sees the chat now.
function onScreen(s: ChatStatus) {
  const p = current(useNav.getState())
  const shown = p?.view === "chat" && p.project === s.project && p.chat === s.chat
  return shown && document.visibilityState === "visible" && document.hasFocus()
}

// notify shows a desktop notification for a chat that waits (SPEC 8.8).
function notify(s: ChatStatus) {
  const chat =
    useChats.getState().byProject[s.project]?.find((it) => it.chat.id === s.chat)?.chat.title ||
    useThreads.getState().threads[s.chat]?.chat.title ||
    "A chat"
  const project = useProjects.getState().opened[s.project]?.name ?? useProjects.getState().list.find((p) => p.id === s.project)?.name
  const what = s.waiting?.kind === PartKind.PartQuestion ? "answer" : "approval"
  SystemService.Notify({
    project: s.project,
    chat: s.chat,
    title: `${chat} is waiting for your ${what}`,
    body: project ? `${project} · ${s.waiting?.text ?? ""}` : (s.waiting?.text ?? ""),
  }).catch(showError)
}

function onNotice(n: Notice) {
  if (n.kind === "damaged") toast.warning(n.text, { duration: Infinity })
  else toast.info(n.text)
}

// listen subscribes the stores to the Go events; it returns the unsubscribe.
export function listen() {
  const off = [
    Events.On("chat:status", (e) => onStatus(e.data)),
    Events.On("chat:delta", (e) => useThreads.getState().delta(e.data)),
    Events.On("chat:part", (e) => useThreads.getState().part(e.data)),
    Events.On("app:open", (e) => void openProject(e.data.project, e.data.chat)),
    Events.On("project:notice", (e) => onNotice(e.data)),
  ]
  return () => off.forEach((f) => f())
}
