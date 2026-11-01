import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty"
import { Sidebar, SidebarContent, SidebarHeader } from "@/components/ui/sidebar"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { cn } from "@/lib/utils"
import { useUI, type RightTab } from "@/state/ui"
import { FilesPanel } from "@/shell/files-panel"

// RightSidebar holds the project's Pages, Pipelines, Files and Links
// (SPEC 5.12). Pages and pipelines come in later phases; Files is the
// bucket. A jenab:// link in the chat switches to Files.
export function RightSidebar({ project, className }: { project: string; className?: string }) {
  const tab = useUI((s) => s.rightTab)
  const setTab = (t: RightTab) => useUI.getState().set({ rightTab: t })
  return (
    <Sidebar collapsible="none" className={cn("w-72 border-s", className)}>
      <SidebarHeader className="p-3">
        <Tabs value={tab} onValueChange={(v) => setTab(v as RightTab)}>
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
        {tab === "files" && <FilesPanel key={project} project={project} />}
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
