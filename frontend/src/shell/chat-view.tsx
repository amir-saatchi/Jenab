import { DiamondIcon, MenuIcon, MessageSquareIcon, PanelLeftCloseIcon, PanelLeftOpenIcon, PanelRightIcon, TriangleAlertIcon } from "lucide-react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useIsMobile } from "@/hooks/use-mobile"
import { ChatKind } from "@/lib/api"
import { useChats } from "@/state/chats"
import { useNav } from "@/state/nav"
import { useProjects } from "@/state/projects"
import { useUI } from "@/state/ui"
import { subline } from "@/shell/chat-sidebar"

// ChatView is the main area with one chat. The messages and the composer
// come with P1-15.
export function ChatView({ project, chat }: { project: string; chat: string }) {
  const item = useChats((s) => s.byProject[project]?.find((it) => it.chat.id === chat))
  const damage = useProjects((s) => s.opened[project]?.damage)
  const mother = item?.chat.kind === ChatKind.KindMother
  return (
    <div className="flex h-full min-h-0 flex-col">
      <ChatHeader title={item?.chat.title || "New chat"} sub={item ? subline(item) : ""} mother={mother} />
      {damage && (
        <Alert className="mx-4 mt-3 w-auto [&>svg]:text-tone-amber">
          <TriangleAlertIcon />
          <AlertTitle>This project is open read-only</AlertTitle>
          <AlertDescription>
            {damage.file} failed its check: {damage.problem}
          </AlertDescription>
        </Alert>
      )}
      <Empty className="flex-1">
        <EmptyHeader>
          <EmptyMedia variant="icon">{mother ? <DiamondIcon className="fill-current" /> : <MessageSquareIcon />}</EmptyMedia>
          <EmptyTitle>Messages show here</EmptyTitle>
          <EmptyDescription>The conversation and the composer are still being built.</EmptyDescription>
        </EmptyHeader>
      </Empty>
    </div>
  )
}

export function ChatHeader({ title, sub, mother }: { title: string; sub: string; mother?: boolean }) {
  const narrow = useIsMobile()
  const hidden = useNav((s) => s.sidebarHidden)
  const setHidden = useNav((s) => s.setSidebarHidden)
  const ui = useUI((s) => s.set)
  return (
    <header className="flex h-14 shrink-0 items-center gap-2 border-b px-3">
      {narrow ? (
        <Button variant="ghost" size="icon-sm" aria-label="Open the sidebar" onClick={() => ui({ leftOverlay: true })}>
          <MenuIcon />
        </Button>
      ) : (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={hidden ? "Show the chat list" : "Hide the chat list"}
              onClick={() => setHidden(!hidden)}
            >
              {hidden ? <PanelLeftOpenIcon /> : <PanelLeftCloseIcon />}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{hidden ? "Show the chat list" : "Hide the chat list"}</TooltipContent>
        </Tooltip>
      )}
      <div className="flex min-w-0 items-center gap-2">
        {mother && <DiamondIcon className="size-4 shrink-0 fill-current" />}
        <div className="grid min-w-0 leading-tight">
          <span className="truncate font-medium" dir="auto">
            {title}
          </span>
          {sub && (
            <span className="truncate text-xs text-muted-foreground" dir="auto">
              {sub}
            </span>
          )}
        </div>
      </div>
      {narrow && (
        <div className="ms-auto flex items-center gap-1">
          <Button variant="ghost" size="sm" onClick={() => ui({ rightOverlay: true })}>
            <PanelRightIcon data-icon="inline-start" />
            Pages
          </Button>
        </div>
      )}
    </header>
  )
}
