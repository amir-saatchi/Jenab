import * as React from "react"

import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { SidebarProvider } from "@/components/ui/sidebar"
import { InspectorView } from "@/dev/inspector"
import { RuntimeView } from "@/dev/runtime"
import { useIsMobile } from "@/hooks/use-mobile"
import { current, useNav } from "@/state/nav"
import { useUI } from "@/state/ui"
import { BottomBar } from "@/shell/bottom-bar"
import { ChatSidebar } from "@/shell/chat-sidebar"
import { ChatView } from "@/shell/chat-view"
import { NewProject } from "@/shell/new-project"
import { ProjectSettings } from "@/shell/project-settings"
import { Rail } from "@/shell/rail"
import { RightSidebar } from "@/shell/right-sidebar"
import { SettingsView } from "@/shell/settings-view"
import { Welcome } from "@/shell/welcome"

// AppShell is the window (SPEC 5.12): from the start edge the rail, the
// chat list, the main area and the right sidebar, with the bottom bar
// under them. Below 900 px the rail and both sidebars are overlays and the
// bottom bar is hidden.
export function AppShell() {
  const place = useNav(current)
  const sidebarHidden = useNav((s) => s.sidebarHidden)
  const narrow = useIsMobile()
  const project = place && "project" in place ? place.project : undefined
  const chat = place?.view === "chat" ? place : undefined
  useShortcuts()

  return (
    <SidebarProvider className="h-svh min-h-0 flex-col overflow-hidden">
      <div className="flex min-h-0 flex-1">
        {!narrow && <Rail chats={!!chat && sidebarHidden} />}
        {!narrow && chat && !sidebarHidden && <ChatSidebar project={chat.project} />}
        <main className="min-w-0 flex-1 overflow-hidden">
          {!place && <Welcome />}
          {chat && <ChatView key={chat.chat} project={chat.project} chat={chat.chat} />}
          {place?.view === "settings" && <SettingsView section={place.section} />}
          {place?.view === "project-settings" && <ProjectSettings project={place.project} />}
          {place?.view === "inspector" && (
            <InspectorView key={`${place.project}/${place.chat}/${place.turn}`} project={place.project} chat={place.chat} turn={place.turn} />
          )}
          {place?.view === "runtime" && <RuntimeView />}
        </main>
        {!narrow && chat && <RightSidebar project={chat.project} />}
      </div>
      {!narrow && <BottomBar project={project} />}
      {narrow && <Overlays project={chat?.project} />}
      <NewProject />
    </SidebarProvider>
  )
}

// Overlays are the rail with the chat list, and the right sidebar, below
// 900 px; ☰ and *Pages* in the header open them.
function Overlays({ project }: { project?: string }) {
  const left = useUI((s) => s.leftOverlay)
  const right = useUI((s) => s.rightOverlay)
  const set = useUI((s) => s.set)
  return (
    <>
      <Sheet open={left} onOpenChange={(o) => set({ leftOverlay: o })}>
        <SheetContent side="left" className="gap-0 p-0 data-[side=left]:w-auto data-[side=left]:sm:max-w-none [&>button]:hidden" onOpenAutoFocus={(e) => e.preventDefault()}>
          <SheetHeader className="sr-only">
            <SheetTitle>Projects and chats</SheetTitle>
            <SheetDescription>The rail and the chat list</SheetDescription>
          </SheetHeader>
          <div className="flex h-full">
            <Rail />
            {project && <ChatSidebar project={project} />}
          </div>
        </SheetContent>
      </Sheet>
      {project && (
        <Sheet open={right} onOpenChange={(o) => set({ rightOverlay: o })}>
          <SheetContent side="right" className="gap-0 p-0 data-[side=right]:w-auto data-[side=right]:sm:max-w-none [&>button]:hidden" onOpenAutoFocus={(e) => e.preventDefault()}>
            <SheetHeader className="sr-only">
              <SheetTitle>Pages, pipelines, files and links</SheetTitle>
              <SheetDescription>The right sidebar</SheetDescription>
            </SheetHeader>
            <RightSidebar project={project} className="h-full border-s-0" />
          </SheetContent>
        </Sheet>
      )}
    </>
  )
}

// isMac is read once; macOS uses ⌘[ and ⌘] for back and forward.
const isMac = /Mac/.test(navigator.platform)

// shortcut maps a key press to a navigation action, or null.
export function shortcut(
  e: Pick<KeyboardEvent, "key" | "ctrlKey" | "shiftKey" | "altKey" | "metaKey">,
  mac: boolean
): "next" | "previous" | "back" | "forward" | null {
  if (e.key === "Tab" && e.ctrlKey && !e.altKey && !e.metaKey) return e.shiftKey ? "previous" : "next"
  if (mac) {
    if (e.metaKey && !e.altKey && !e.ctrlKey && e.key === "[") return "back"
    if (e.metaKey && !e.altKey && !e.ctrlKey && e.key === "]") return "forward"
    return null
  }
  if (e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey && e.key === "ArrowLeft") return "back"
  if (e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey && e.key === "ArrowRight") return "forward"
  return null
}

// useShortcuts is Ctrl+Tab through the chats used last, and back and
// forward (SPEC 5.12).
function useShortcuts() {
  React.useEffect(() => {
    const nav = useNav.getState
    const down = (e: KeyboardEvent) => {
      const a = shortcut(e, isMac)
      if (!a) return
      e.preventDefault()
      if (a === "next") nav().nextChat(1)
      else if (a === "previous") nav().nextChat(-1)
      else if (a === "back") nav().back()
      else nav().forward()
    }
    const up = (e: KeyboardEvent) => {
      if (e.key === "Control") nav().endCycle()
    }
    const blur = () => nav().endCycle()
    window.addEventListener("keydown", down)
    window.addEventListener("keyup", up)
    window.addEventListener("blur", blur)
    return () => {
      window.removeEventListener("keydown", down)
      window.removeEventListener("keyup", up)
      window.removeEventListener("blur", blur)
    }
  }, [])
}
