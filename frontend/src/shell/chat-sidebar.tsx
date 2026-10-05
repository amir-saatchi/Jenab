import * as React from "react"
import { ChevronDownIcon, CircleAlertIcon, DiamondIcon, FolderCogIcon, MessageSquareIcon, PinIcon, PlusIcon } from "lucide-react"

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSkeleton,
} from "@/components/ui/sidebar"
import { Spinner } from "@/components/ui/spinner"
import { ChatKind, ChatState, type ChatItem } from "@/lib/api"
import { cn } from "@/lib/utils"
import { useChats, visible } from "@/state/chats"
import { current, useNav } from "@/state/nav"
import { useProjects } from "@/state/projects"
import { useUI } from "@/state/ui"
import { newChat, openChat } from "@/shell/open"

// ChatSidebar is the left sidebar (SPEC 5.12): the project's name with its
// menu, *New chat*, and the chat list with Mother pinned at the top (8.6).
export function ChatSidebar({ project, className }: { project: string; className?: string }) {
  const item = useProjects((s) => s.list.find((p) => p.id === project))
  const items = useChats((s) => s.byProject[project])
  const place = useNav(current)
  const list = React.useMemo(() => visible(items ?? []), [items])
  const active = place?.view === "chat" ? place.chat : undefined

  return (
    <Sidebar collapsible="none" className={cn("w-64 border-e", className)}>
      <SidebarHeader className="gap-2">
        <SidebarMenu>
          <SidebarMenuItem>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <SidebarMenuButton size="lg">
                  <div className="grid flex-1 text-start leading-tight">
                    <span className="truncate font-medium" dir="auto">
                      {item?.name}
                    </span>
                    <span className="truncate text-xs text-muted-foreground" dir="ltr">
                      {item?.folder}
                    </span>
                  </div>
                  <ChevronDownIcon className="ms-auto" />
                </SidebarMenuButton>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="w-56">
                <DropdownMenuGroup>
                  <DropdownMenuItem
                    onSelect={() => {
                      useNav.getState().go({ view: "project-settings", project })
                      useUI.getState().set({ leftOverlay: false })
                    }}
                  >
                    <FolderCogIcon />
                    Project settings
                  </DropdownMenuItem>
                </DropdownMenuGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          </SidebarMenuItem>
          <SidebarMenuItem>
            <SidebarMenuButton variant="outline" onClick={() => void newChat(project)}>
              <PlusIcon />
              <span>New chat</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Chats</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {!items &&
                Array.from({ length: 3 }, (_, i) => (
                  <SidebarMenuItem key={i}>
                    <SidebarMenuSkeleton showIcon />
                  </SidebarMenuItem>
                ))}
              {list.map((it) => (
                <ChatRow key={it.chat.id} item={it} active={it.chat.id === active} onOpen={() => openChat(project, it.chat.id)} />
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
    </Sidebar>
  )
}

function ChatRow({ item, active, onOpen }: { item: ChatItem; active: boolean; onOpen: () => void }) {
  const { chat, state } = item
  const mother = chat.kind === ChatKind.KindMother
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        size="lg"
        isActive={active}
        onClick={onOpen}
        className={cn("relative", mother && "mother-ring")}
        data-working={mother && state === ChatState.StateWorking}
      >
        {mother ? <DiamondIcon className="fill-current" /> : <MessageSquareIcon />}
        <div className="grid flex-1 text-start leading-tight">
          <span className="truncate" dir="auto">
            {chat.title || "New chat"}
          </span>
          <span className={cn("truncate text-xs text-muted-foreground", state === ChatState.StateWaiting && "text-tone-amber")} dir="auto">
            {subline(item)}
          </span>
        </div>
      </SidebarMenuButton>
      <StatusBadge item={item} />
    </SidebarMenuItem>
  )
}

// subline is the row's second line: what the chat is doing, else its role,
// else when it was last active.
export function subline({ chat, state, last_activity }: ChatItem, now = new Date()) {
  if (state === ChatState.StateWaiting) return "Waiting for you"
  if (state === ChatState.StateWorking) return "Working…"
  if (chat.kind === ChatKind.KindMother) return "Project home"
  const role = chat.role.split("\n")[0]
  if (role) return role
  if (!last_activity) return "No messages yet"
  const t = new Date(last_activity)
  const today = t.toDateString() === now.toDateString()
  return "Last active " + (today ? t.toLocaleTimeString([], { timeStyle: "short" }) : t.toLocaleDateString([], { dateStyle: "medium" }))
}

function StatusBadge({ item }: { item: ChatItem }) {
  if (item.state === ChatState.StateWorking) {
    return (
      <SidebarMenuBadge>
        <Spinner className="size-3.5 text-muted-foreground" />
      </SidebarMenuBadge>
    )
  }
  if (item.state === ChatState.StateWaiting) {
    return (
      <SidebarMenuBadge>
        <CircleAlertIcon className="size-4 text-tone-amber" />
      </SidebarMenuBadge>
    )
  }
  if (item.chat.kind === ChatKind.KindMother) {
    return (
      <SidebarMenuBadge>
        <PinIcon className="size-3.5 text-muted-foreground" />
      </SidebarMenuBadge>
    )
  }
  return null
}
