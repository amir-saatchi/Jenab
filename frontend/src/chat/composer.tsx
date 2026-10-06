import * as React from "react"
import { CheckIcon, ChevronDownIcon, PlusIcon, SendIcon, ShieldCheckIcon, SparklesIcon, SquareIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupTextarea } from "@/components/ui/input-group"
import { ChatService, ChatState, type ModelGroup } from "@/lib/api"
import { showError } from "@/lib/errors"
import { cn } from "@/lib/utils"
import { ConnectForm } from "@/chat/provider-card"
import { useProjects } from "@/state/projects"
import { useSettings } from "@/state/settings"
import { drafts, useThreads, type Thread } from "@/state/thread"

// modelName is the name the picker shows for a chat's model: an alias
// shows the model it points at.
export function modelName(groups: ModelGroup[], model: string): string {
  if (groups.length === 0) return "No model"
  for (const g of groups) {
    for (const m of g.models ?? []) if (m.ref === model || m.aliases?.includes(model)) return m.name
  }
  return model
}

const levels = [
  { id: "strict", name: "Strict", hint: "Asks before every change" },
  { id: "standard", name: "Standard", hint: "Asks for new hosts, code and destructive changes" },
  { id: "auto", name: "Auto", hint: "Asks less, but always for destructive changes" },
]

// Composer is where the user writes (SPEC 8.3): Enter sends, also during a
// turn, where the message joins at the next step.
export function Composer({ thread, mother, readOnly }: { thread: Thread; mother: boolean; readOnly?: boolean }) {
  const chat = thread.chat.id
  const project = thread.project
  const [text, setText] = React.useState(() => drafts.get(chat) ?? "")
  const ref = React.useRef<HTMLTextAreaElement>(null)
  const running = thread.state !== ChatState.StateIdle
  const title = thread.chat.title || "the chat"

  React.useEffect(() => {
    setText(drafts.get(chat) ?? "")
    ref.current?.focus()
  }, [chat])

  const change = (v: string) => {
    setText(v)
    if (v) drafts.set(chat, v)
    else drafts.delete(chat)
  }

  const send = async () => {
    const t = text.trim()
    if (!t || readOnly) return
    change("")
    try {
      const id = await ChatService.Send(project, chat, t)
      // joined checks the turn as it is now: it may have ended, or a new
      // try taken the message, while Send ran.
      if (running) useThreads.getState().joined(chat, id)
    } catch (err) {
      change(text)
      showError(err)
    }
  }

  const keyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      void send()
    }
  }

  return (
    <div className="mx-auto w-full max-w-3xl px-6 pb-4">
      <InputGroup className={cn(mother && "mother-ring")} data-working={mother && running}>
        <InputGroupTextarea
          ref={ref}
          dir="auto"
          rows={2}
          value={text}
          disabled={readOnly}
          onChange={(e) => change(e.target.value)}
          onKeyDown={keyDown}
          aria-label="Message"
          placeholder={
            readOnly
              ? "This project is open read-only."
              : running
                ? "Write anytime. Your message joins the running turn."
                : `Message ${title}…`
          }
          className="max-h-60 min-h-16 text-start"
        />
        <InputGroupAddon align="block-end">
          <ModelPicker project={project} chat={chat} model={thread.chat.model} />
          <LevelPicker project={project} chat={chat} />
          {running && !text.trim() ? (
            <InputGroupButton
              variant="default"
              size="sm"
              className="ms-auto"
              onClick={() => ChatService.Stop(project, chat).catch(showError)}
            >
              <SquareIcon className="fill-current" />
              Stop
            </InputGroupButton>
          ) : (
            <InputGroupButton
              variant="default"
              size="icon-sm"
              className="ms-auto"
              aria-label="Send"
              disabled={!text.trim() || readOnly}
              onClick={() => void send()}
            >
              <SendIcon />
            </InputGroupButton>
          )}
        </InputGroupAddon>
      </InputGroup>
    </div>
  )
}

// ModelPicker lists the models that are on, grouped by provider, with
// the aliases marked (SPEC 3.9).
function ModelPicker({ project, chat, model }: { project: string; chat: string; model: string }) {
  const groups = useSettings((s) => s.models)
  const [connect, setConnect] = React.useState(false)
  React.useEffect(() => {
    if (!groups) useSettings.getState().loadModels().catch(showError)
  }, [groups])
  const set = (ref: string) =>
    ChatService.SetModel(project, chat, ref)
      .then(() => {
        const t = useThreads.getState().threads[chat]
        if (t) useThreads.getState().setChat({ ...t.chat, model: ref })
      })
      .catch(showError)
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <InputGroupButton variant="ghost">
            <SparklesIcon />
            <span className="max-w-40 truncate">{modelName(groups ?? [], model)}</span>
            <ChevronDownIcon />
          </InputGroupButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-64">
          {(groups ?? []).map((g) => (
            <DropdownMenuGroup key={g.provider}>
              <DropdownMenuLabel>{g.provider}</DropdownMenuLabel>
              {(g.models ?? []).map((m) => (
                <DropdownMenuItem key={m.ref} onSelect={() => void set(m.ref)}>
                  <span className="truncate">{m.name}</span>
                  {(m.aliases ?? []).map((a) => (
                    <Badge key={a} variant="secondary">
                      {a}
                    </Badge>
                  ))}
                  {(m.ref === model || m.aliases?.includes(model)) && <CheckIcon className="ms-auto" />}
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
          ))}
          {(groups ?? []).length > 0 && <DropdownMenuSeparator />}
          <DropdownMenuGroup>
            <DropdownMenuItem onSelect={() => setConnect(true)}>
              <PlusIcon />
              Connect a provider…
            </DropdownMenuItem>
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
      <Dialog open={connect} onOpenChange={setConnect}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Connect a provider</DialogTitle>
            <DialogDescription>Use your own key. Its models are turned on.</DialogDescription>
          </DialogHeader>
          <ConnectForm id="connect-dialog" onConnected={() => setConnect(false)} />
        </DialogContent>
      </Dialog>
    </>
  )
}

// LevelPicker is the project's approval level (SPEC 8.8).
function LevelPicker({ project, chat }: { project: string; chat: string }) {
  const level = useProjects((s) => s.opened[project]?.level ?? "standard")
  const name = levels.find((l) => l.id === level)?.name ?? level
  const set = (id: string) =>
    ChatService.SetLevel(project, chat, id)
      .then(() => useProjects.getState().setLevel(project, id))
      .catch(showError)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <InputGroupButton variant="ghost" aria-label={`Approvals: ${name}`}>
          <ShieldCheckIcon />
          Approvals: {name}
          <ChevronDownIcon />
        </InputGroupButton>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-72">
        <DropdownMenuGroup>
          <DropdownMenuLabel>Approvals for this project</DropdownMenuLabel>
          {levels.map((l) => (
            <DropdownMenuItem key={l.id} onSelect={() => void set(l.id)}>
              <div className="grid leading-tight">
                <span>{l.name}</span>
                <span className="text-xs text-muted-foreground">{l.hint}</span>
              </div>
              {l.id === level && <CheckIcon className="ms-auto" />}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// starters are first messages for an empty Mother chat (SPEC 3.9); a
// click sends one.
export const starters = [
  "Track a price every day",
  "Summarise a news feed",
  "Collect a table from a web page",
]
