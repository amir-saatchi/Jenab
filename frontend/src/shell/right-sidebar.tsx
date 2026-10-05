import * as React from "react"
import { FileTextIcon, FolderIcon } from "lucide-react"

import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty"
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSkeleton,
} from "@/components/ui/sidebar"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { BucketService } from "@/lib/api"
import { toUIError } from "@/lib/errors"
import { cn } from "@/lib/utils"

type Tab = "pages" | "pipelines" | "files" | "links"

// RightSidebar holds the project's Pages, Pipelines, Files and Links
// (SPEC 5.12). Pages and pipelines come in later phases; Files lists the
// bucket.
export function RightSidebar({ project, className }: { project: string; className?: string }) {
  const [tab, setTab] = React.useState<Tab>("pages")
  return (
    <Sidebar collapsible="none" className={cn("w-72 border-s", className)}>
      <SidebarHeader className="p-3">
        <Tabs value={tab} onValueChange={(v) => setTab(v as Tab)}>
          <TabsList className="w-full">
            <TabsTrigger value="pages">Pages</TabsTrigger>
            <TabsTrigger value="pipelines">Pipelines</TabsTrigger>
            <TabsTrigger value="files">Files</TabsTrigger>
            <TabsTrigger value="links">Links</TabsTrigger>
          </TabsList>
        </Tabs>
      </SidebarHeader>
      <SidebarContent>
        {tab === "pages" && <Nothing title="No pages yet" text="Pages are built by the agent. Ask in the chat to add one." />}
        {tab === "pipelines" && <Nothing title="No pipelines yet" text="Pipelines are built by the agent and run on a schedule." />}
        {tab === "files" && <Files key={project} project={project} />}
        {tab === "links" && <Nothing title="No links yet" text="Links the agent saves for this project show here." />}
      </SidebarContent>
    </Sidebar>
  )
}

function Nothing({ title, text }: { title: string; text: string }) {
  return (
    <Empty className="p-4">
      <EmptyHeader>
        <EmptyTitle className="text-sm">{title}</EmptyTitle>
        <EmptyDescription>{text}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}

// Files is the top folder of the project's bucket.
function Files({ project }: { project: string }) {
  const [page, setPage] = React.useState<{ folders: string[]; files: string[] } | null>(null)
  const [error, setError] = React.useState("")
  React.useEffect(() => {
    let live = true
    BucketService.List(project, "", "")
      .then((p) => live && setPage({ folders: p.folders ?? [], files: (p.objects ?? []).map((o) => o.key) }))
      .catch((err) => live && setError(toUIError(err).message))
    return () => {
      live = false
    }
  }, [project])
  if (error) return <Nothing title="Files can't be shown" text={error} />
  if (!page) {
    return (
      <SidebarGroup>
        <SidebarMenu>
          {Array.from({ length: 3 }, (_, i) => (
            <SidebarMenuItem key={i}>
              <SidebarMenuSkeleton showIcon />
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
      </SidebarGroup>
    )
  }
  if (page.folders.length + page.files.length === 0) {
    return <Nothing title="No files yet" text="Files you upload and files the agent saves show here." />
  }
  return (
    <SidebarGroup>
      <SidebarGroupContent>
        <SidebarMenu>
          {page.folders.map((f) => (
            <SidebarMenuItem key={f}>
              <SidebarMenuButton>
                <FolderIcon />
                <span className="truncate" dir="auto">
                  {f}
                </span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          ))}
          {page.files.map((f) => (
            <SidebarMenuItem key={f}>
              <SidebarMenuButton>
                <FileTextIcon />
                <span className="truncate" dir="auto">
                  {f}
                </span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  )
}
