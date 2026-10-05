import * as React from "react"
import { CalendarClockIcon, DiamondIcon, InboxIcon, PlusIcon, SearchIcon, SettingsIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Separator } from "@/components/ui/separator"
import { Spinner } from "@/components/ui/spinner"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { ChatKind, ChatState, type ChatItem } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useChats, visible } from "@/state/chats"
import { current, useNav } from "@/state/nav"
import { initial, initials, useProjects } from "@/state/projects"
import { useUI } from "@/state/ui"
import { newChat, openChat, openProject } from "@/shell/open"

// The rail (SPEC 5.12): always shown, 56 px wide, with what belongs to the
// whole app. With chats, the open project's chats are in it too, as while
// the sidebar is hidden.
export function Rail({ chats = false, className }: { chats?: boolean; className?: string }) {
  const projects = useProjects((s) => s.list)
  const place = useNav(current)
  const go = useNav((s) => s.go)
  const openNew = () => useUI.getState().set({ newProject: true, leftOverlay: false })
  const project = place && "project" in place ? place.project : undefined

  return (
    <nav
      aria-label="Projects"
      className={cn("flex h-full w-14 shrink-0 flex-col items-center gap-1 border-e bg-sidebar py-2 text-sidebar-foreground", className)}
    >
      <div className="flex w-full flex-col gap-2">
        {projects.map((p) => (
          <RailTip key={p.id} label={p.name}>
            <Tile active={p.id === project} label={p.name} onClick={() => void openProject(p.id)}>
              <span className="flex size-9 items-center justify-center rounded-xl bg-primary text-sm font-semibold text-primary-foreground group-focus-visible/tile:ring-2 group-focus-visible/tile:ring-sidebar-ring" dir="auto">
                {initial(p.name)}
              </span>
            </Tile>
          </RailTip>
        ))}
      </div>
      <RailTip label="New project">
        <Button variant="ghost" size="icon" aria-label="New project" onClick={openNew}>
          <PlusIcon />
        </Button>
      </RailTip>
      <Separator className="my-1.5 w-6!" />
      <RailTip label="Search comes with a later version">
        <span>
          <Button variant="ghost" size="icon" aria-label="Search" disabled>
            <SearchIcon />
          </Button>
        </span>
      </RailTip>
      <WaitingButton />
      {chats && project && <RailChats project={project} />}
      <div className="mt-auto flex flex-col items-center gap-1">
        <RailTip label="Schedules come with pipelines">
          <span>
            <Button variant="ghost" size="icon" aria-label="Schedules" disabled className="text-muted-foreground">
              <CalendarClockIcon />
            </Button>
          </span>
        </RailTip>
        <RailTip label="Settings">
          <Button
            variant="ghost"
            size="icon"
            aria-label="Settings"
            aria-current={place?.view === "settings" ? "page" : undefined}
            onClick={() => {
              go({ view: "settings" })
              useUI.getState().set({ leftOverlay: false })
            }}
          >
            <SettingsIcon />
          </Button>
        </RailTip>
      </div>
    </nav>
  )
}

export function RailTip({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  )
}

// Tile is a rail button with the start-edge bar that marks what is open.
function Tile({
  active,
  label,
  onClick,
  children,
  ...props
}: { active: boolean; label: string } & React.ComponentProps<"button">) {
  return (
    <button
      type="button"
      aria-label={label}
      aria-current={active ? "page" : undefined}
      onClick={onClick}
      className="group/tile relative flex w-full justify-center outline-none"
      {...props}
    >
      <span
        className={cn(
          "absolute start-0 w-1 rounded-e-full bg-sidebar-foreground transition-all",
          active ? "inset-y-1.5" : "inset-y-3 scale-y-0 group-hover/tile:scale-y-100"
        )}
      />
      {children}
    </button>
  )
}

export function statusLabel(title: string, state: string) {
  if (state === ChatState.StateWaiting) return `${title} · waiting for you`
  if (state === ChatState.StateWorking) return `${title} · working`
  return title
}

function RailChats({ project }: { project: string }) {
  const items = useChats((s) => s.byProject[project])
  const place = useNav(current)
  const list = React.useMemo(() => visible(items ?? []), [items])
  return (
    <>
      <Separator className="my-1.5 w-6!" />
      <div className="flex min-h-0 w-full flex-col gap-2 overflow-y-auto py-0.5">
        {list.map((it) => (
          <RailChat key={it.chat.id} project={project} item={it} active={place?.view === "chat" && place.chat === it.chat.id} />
        ))}
      </div>
      <RailTip label="New chat">
        <Button variant="ghost" size="icon" aria-label="New chat" onClick={() => void newChat(project)}>
          <PlusIcon />
        </Button>
      </RailTip>
    </>
  )
}

function RailChat({ project, item, active }: { project: string; item: ChatItem; active: boolean }) {
  const { chat, state } = item
  const mother = chat.kind === ChatKind.KindMother
  const title = chat.title || "New chat"
  const label = statusLabel(title, state)
  return (
    <RailTip label={label}>
      <Tile active={active} label={label} onClick={() => openChat(project, chat.id)}>
        {mother ? (
          <span
            className="mother-tile flex size-9 items-center justify-center rounded-xl text-background group-focus-visible/tile:ring-2 group-focus-visible/tile:ring-sidebar-ring"
            data-working={state === ChatState.StateWorking}
          >
            <DiamondIcon className="size-4 fill-current" />
          </span>
        ) : (
          <span
            className={cn(
              "flex size-9 items-center justify-center rounded-xl bg-sidebar-accent text-xs font-semibold text-sidebar-accent-foreground transition-colors group-hover/tile:bg-sidebar-border group-focus-visible/tile:ring-2 group-focus-visible/tile:ring-sidebar-ring",
              active && "bg-sidebar-primary text-sidebar-primary-foreground group-hover/tile:bg-sidebar-primary"
            )}
            dir="auto"
          >
            {initials(title)}
          </span>
        )}
        {state === ChatState.StateWorking && !mother && (
          <span className="absolute -bottom-0.5 end-2 flex size-4 items-center justify-center rounded-full bg-sidebar">
            <Spinner className="size-3 text-muted-foreground" />
          </span>
        )}
        {state === ChatState.StateWaiting && (
          <span className="absolute -top-0.5 end-2 size-2.5 rounded-full bg-tone-amber ring-2 ring-sidebar" />
        )}
      </Tile>
    </RailTip>
  )
}

// WaitingButton is *Waiting*: the approvals and questions from every
// project (8.8). Clicking one opens its chat.
function WaitingButton() {
  const waiting = useChats((s) => s.waiting)
  const projects = useProjects((s) => s.list)
  const name = (id: string) => projects.find((p) => p.id === id)?.name ?? ""
  const label = waiting.length ? `Waiting for you: ${waiting.length}` : "Nothing is waiting for you"
  return (
    <DropdownMenu>
      <RailTip label={label}>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon" aria-label={label} className="relative">
            <InboxIcon />
            {waiting.length > 0 && (
              <span className="absolute -top-0.5 -end-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-tone-amber px-1 text-[0.625rem] font-semibold text-background tabular-nums">
                {waiting.length}
              </span>
            )}
          </Button>
        </DropdownMenuTrigger>
      </RailTip>
      <DropdownMenuContent side="right" align="start" className="w-72">
        <DropdownMenuGroup>
          <DropdownMenuLabel>Waiting for you</DropdownMenuLabel>
          {waiting.length === 0 && <DropdownMenuItem disabled>Nothing is waiting.</DropdownMenuItem>}
          {waiting.map((w) => (
            <DropdownMenuItem
              key={w.chat}
              onSelect={() => void openProject(w.project, w.chat)}
            >
              <div className="grid min-w-0 leading-tight">
                <span className="truncate" dir="auto">
                  {w.title || "New chat"} <span className="text-muted-foreground">· {name(w.project)}</span>
                </span>
                <span className="truncate text-xs text-muted-foreground" dir="auto">
                  {w.waiting.text}
                </span>
              </div>
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
