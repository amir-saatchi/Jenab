import * as React from "react"
import {
  ChevronsUpDownIcon,
  CircleAlertIcon,
  DiamondIcon,
  FileTextIcon,
  LayoutGridIcon,
  LinkIcon,
  MessageSquareIcon,
  PinIcon,
  PlusIcon,
  SearchIcon,
  SettingsIcon,
  WorkflowIcon,
  FolderCogIcon,
  CheckIcon,
  FlagIcon,
  CalendarClockIcon,
} from "lucide-react"

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
} from "@/components/ui/sidebar"
import { Spinner } from "@/components/ui/spinner"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Button } from "@/components/ui/button"
import { Separator } from "@/components/ui/separator"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import {
  chats,
  otherProjects,
  pages,
  pipelines,
  project,
  tray,
  type Chat,
  type ChatStatus,
} from "@/data/bitcoin"

export type RightTab = "pages" | "pipelines" | "files" | "links"

export function AppShell({
  activeChat = "mother",
  activeItem,
  rightTab = "pages",
  leftCollapsed = false,
  statuses = {},
  chatList,
  right,
  children,
}: {
  chatList?: Chat[]
  right?: React.ReactNode
  statuses?: Record<string, ChatStatus>
  activeChat?: string
  activeItem?: string
  rightTab?: RightTab
  leftCollapsed?: boolean
  children: React.ReactNode
}) {
  return (
    <SidebarProvider
      defaultOpen={!leftCollapsed}
      className="h-svh overflow-hidden"
      style={{ "--sidebar-width-icon": "3.5rem" } as React.CSSProperties}
    >
      <LeftSidebar activeChat={activeChat} statuses={statuses} chatList={chatList} />
      <SidebarInset className="min-w-0 overflow-hidden">{children}</SidebarInset>
      {right ?? <RightSidebar tab={rightTab} activeItem={activeItem} />}
    </SidebarProvider>
  )
}

function ProjectMark({ initial, className }: { initial: string; className?: string }) {
  return (
    <div
      className={cn(
        "flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary text-sm font-semibold text-primary-foreground",
        className
      )}
    >
      {initial}
    </div>
  )
}

function ChatIcon({ chat }: { chat: Chat }) {
  return chat.mother ? <DiamondIcon className="fill-current" /> : <MessageSquareIcon />
}

function StatusIcon({ chat }: { chat: Chat }) {
  if (chat.status === "working") {
    return (
      <SidebarMenuBadge>
        <Spinner className="size-3.5 text-muted-foreground" />
      </SidebarMenuBadge>
    )
  }
  if (chat.status === "waiting") {
    return (
      <SidebarMenuBadge>
        <CircleAlertIcon className="size-4 text-tone-amber" />
      </SidebarMenuBadge>
    )
  }
  if (chat.mother && chat.status === "idle") {
    return (
      <SidebarMenuBadge>
        <PinIcon className="size-3.5 text-muted-foreground" />
      </SidebarMenuBadge>
    )
  }
  return null
}

function LeftSidebar({
  activeChat,
  statuses,
  chatList,
}: {
  activeChat: string
  statuses: Record<string, ChatStatus>
  chatList?: Chat[]
}) {
  const list = (chatList ?? chats).map((c) => ({ ...c, status: statuses[c.id] ?? c.status }))
  return (
    <Sidebar collapsible="icon">
      <Rail activeChat={activeChat} list={list} />
      <SidebarHeader className="group-data-[collapsible=icon]:hidden">
        <SidebarMenu className="gap-2">
          <SidebarMenuItem>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <SidebarMenuButton size="lg" tooltip={project.name}>
                  <ProjectMark initial={project.initial} />
                  <div className="grid flex-1 text-start leading-tight group-data-[collapsible=icon]:hidden">
                    <span className="truncate font-medium">{project.name}</span>
                    <span className="truncate text-xs text-muted-foreground">
                      {project.folder}
                    </span>
                  </div>
                  <ChevronsUpDownIcon className="ms-auto group-data-[collapsible=icon]:hidden" />
                </SidebarMenuButton>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="w-60">
                <DropdownMenuGroup>
                  <DropdownMenuLabel>Projects</DropdownMenuLabel>
                  <DropdownMenuItem>
                    <ProjectMark initial={project.initial} className="size-6 text-xs" />
                    {project.name}
                    <CheckIcon className="ms-auto" />
                  </DropdownMenuItem>
                  {otherProjects.map((p) => (
                    <DropdownMenuItem key={p.name}>
                      <ProjectMark initial={p.initial} className="size-6 text-xs" />
                      {p.name}
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuGroup>
                <DropdownMenuSeparator />
                <DropdownMenuGroup>
                  <DropdownMenuItem>
                    <PlusIcon />
                    New project
                  </DropdownMenuItem>
                  <DropdownMenuItem>
                    <FolderCogIcon />
                    Project settings
                  </DropdownMenuItem>
                </DropdownMenuGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          </SidebarMenuItem>
          <SidebarMenuItem className="flex gap-2 group-data-[collapsible=icon]:flex-col">
            <SidebarMenuButton tooltip="New chat" variant="outline" className="flex-1">
              <PlusIcon />
              <span>New chat</span>
            </SidebarMenuButton>
            <SidebarMenuButton tooltip="Search" variant="outline" className="w-auto">
              <SearchIcon />
              <span className="sr-only">Search</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent className="group-data-[collapsible=icon]:hidden">
        <SidebarGroup>
          <SidebarGroupLabel>Chats</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {list.map((chat) => (
                <SidebarMenuItem key={chat.id}>
                  <SidebarMenuButton
                    size="lg"
                    isActive={chat.id === activeChat}
                    tooltip={chat.title}
                    className={cn("relative", chat.mother && "mother-ring")}
                    data-working={chat.mother && chat.status === "working"}
                  >
                    <ChatIcon chat={chat} />
                    <div className="grid flex-1 text-start leading-tight group-data-[collapsible=icon]:hidden">
                      <span className="truncate">{chat.title}</span>
                      <span
                        className={cn(
                          "truncate text-xs text-muted-foreground",
                          chat.status === "waiting" && "text-tone-amber"
                        )}
                      >
                        {chat.sub}
                      </span>
                    </div>
                  </SidebarMenuButton>
                  <StatusIcon chat={chat} />
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter className="group-data-[collapsible=icon]:hidden">
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton tooltip="Settings">
              <SettingsIcon />
              <span>Settings</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
        <p className="px-2 pb-1 text-xs text-muted-foreground group-data-[collapsible=icon]:hidden">
          Tray: {tray}
        </p>
      </SidebarFooter>
    </Sidebar>
  )
}

// The folded left sidebar has its own design: one 36 px tile per chat.
// Mother is a gradient tile (it turns while Mother works); other chats show
// their initials. A bar on the start edge marks the open chat.
function initials(title: string) {
  const words = title.split(/\s+/)
  return words.length > 1 ? (words[0][0] + words[1][0]).toUpperCase() : title.slice(0, 2)
}

function RailTip({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  )
}

function RailChat({ chat, active }: { chat: Chat; active: boolean }) {
  const label =
    chat.status === "waiting"
      ? `${chat.title} · waiting for you`
      : chat.status === "working"
        ? `${chat.title} · working`
        : chat.title
  return (
    <RailTip label={label}>
      <button
        type="button"
        aria-label={label}
        aria-current={active ? "page" : undefined}
        className="group/tile relative flex w-full justify-center outline-none"
      >
        <span
          className={cn(
            "absolute start-0 w-1 rounded-e-full bg-sidebar-foreground transition-all",
            active ? "inset-y-1.5" : "inset-y-3 scale-y-0 group-hover/tile:scale-y-100"
          )}
        />
        {chat.mother ? (
          <span
            className="mother-tile flex size-9 items-center justify-center rounded-xl text-background group-focus-visible/tile:ring-2 group-focus-visible/tile:ring-sidebar-ring"
            data-working={chat.status === "working"}
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
            {initials(chat.title)}
          </span>
        )}
        {chat.status === "working" && !chat.mother && (
          <span className="absolute -bottom-0.5 end-2 flex size-4 items-center justify-center rounded-full bg-sidebar">
            <Spinner className="size-3 text-muted-foreground" />
          </span>
        )}
        {chat.status === "waiting" && (
          <span className="absolute -top-0.5 end-2 size-2.5 rounded-full bg-tone-amber ring-2 ring-sidebar" />
        )}
      </button>
    </RailTip>
  )
}

function Rail({ activeChat, list }: { activeChat: string; list: Chat[] }) {
  const mother = list.find((c) => c.mother)
  const rest = list.filter((c) => !c.mother)
  return (
    <div className="hidden h-full flex-col items-center gap-1 py-2 group-data-[collapsible=icon]:flex">
      <RailTip label={`${project.name} · switch project`}>
        <button
          type="button"
          className="mb-1 rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring"
        >
          <ProjectMark initial={project.initial} />
        </button>
      </RailTip>
      <RailTip label="New chat">
        <Button variant="ghost" size="icon" aria-label="New chat">
          <PlusIcon />
        </Button>
      </RailTip>
      <RailTip label="Search">
        <Button variant="ghost" size="icon" aria-label="Search">
          <SearchIcon />
        </Button>
      </RailTip>
      <Separator className="my-1.5 w-6!" />
      <div className="flex w-full flex-col gap-2">
        {mother && <RailChat chat={mother} active={mother.id === activeChat} />}
        {rest.map((c) => (
          <RailChat key={c.id} chat={c} active={c.id === activeChat} />
        ))}
      </div>
      <div className="mt-auto flex flex-col items-center gap-1">
        <RailTip label={`Tray: ${tray}`}>
          <Button variant="ghost" size="icon" aria-label={`Schedules: ${tray}`} className="text-muted-foreground">
            <CalendarClockIcon />
          </Button>
        </RailTip>
        <RailTip label="Settings">
          <Button variant="ghost" size="icon" aria-label="Settings">
            <SettingsIcon />
          </Button>
        </RailTip>
      </div>
    </div>
  )
}

function PipelineState({ last }: { last: (typeof pipelines)[number]["last"] }) {
  if (last === "running") return <Spinner className="size-3.5 text-muted-foreground" />
  if (last === "flagged") return <FlagIcon className="size-3.5 text-tone-red" />
  return <CheckIcon className="size-3.5 text-tone-green" />
}

function RightSidebar({ tab, activeItem }: { tab: RightTab; activeItem?: string }) {
  return (
    <Sidebar
      side="right"
      collapsible="none"
      className="border-s"
      style={{ "--sidebar-width": "18rem" } as React.CSSProperties}
    >
      <SidebarHeader className="p-3">
        <Tabs value={tab}>
          <TabsList className="w-full">
            <TabsTrigger value="pages">Pages</TabsTrigger>
            <TabsTrigger value="pipelines">Pipelines</TabsTrigger>
            <TabsTrigger value="files">Files</TabsTrigger>
            <TabsTrigger value="links">Links</TabsTrigger>
          </TabsList>
        </Tabs>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
              {tab === "pages" &&
                pages.map((p) => (
                  <SidebarMenuItem key={p.id}>
                    <SidebarMenuButton isActive={p.id === activeItem}>
                      <LayoutGridIcon />
                      <span>{p.title}</span>
                    </SidebarMenuButton>
                    <SidebarMenuBadge className="font-normal text-muted-foreground">
                      {p.blocks} blocks
                    </SidebarMenuBadge>
                  </SidebarMenuItem>
                ))}
              {tab === "pipelines" &&
                pipelines.map((p) => (
                  <SidebarMenuItem key={p.id}>
                    <SidebarMenuButton size="lg" isActive={p.id === activeItem}>
                      <WorkflowIcon />
                      <div className="grid flex-1 text-start leading-tight group-data-[collapsible=icon]:hidden">
                        <span className="truncate font-mono text-[0.8rem]">{p.id}</span>
                        <span
                          className={cn(
                            "truncate text-xs text-muted-foreground",
                            p.last === "flagged" && "text-tone-red"
                          )}
                        >
                          {p.lastText}
                        </span>
                      </div>
                    </SidebarMenuButton>
                    <SidebarMenuBadge>
                      <PipelineState last={p.last} />
                    </SidebarMenuBadge>
                  </SidebarMenuItem>
                ))}
              {tab === "files" && (
                <SidebarMenuItem>
                  <SidebarMenuButton>
                    <FileTextIcon />
                    <span>btc-history-2024.csv</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              )}
              {tab === "links" && (
                <SidebarMenuItem>
                  <SidebarMenuButton>
                    <LinkIcon />
                    <span>coindesk.com/markets</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              )}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
        {tab === "pages" && (
          <p className="px-4 text-xs text-muted-foreground">
            Pages are built by the agent. Ask in the chat to add or change one.
          </p>
        )}
      </SidebarContent>
    </Sidebar>
  )
}
