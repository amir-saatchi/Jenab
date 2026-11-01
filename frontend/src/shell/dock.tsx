import * as React from "react"
import { ArrowLeftRightIcon, MessageSquareIcon, PanelLeftCloseIcon, PanelRightCloseIcon, XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useIsMobile } from "@/hooks/use-mobile"
import { cn } from "@/lib/utils"
import { chatKey, clampWidth, MIN_DOCK, useDock } from "@/state/dock"
import { current, useNav } from "@/state/nav"

// useListHidden says whether the left sidebar with the chat list is
// hidden, its chats then in the rail, and sets it. While something is
// docked it hides, as for an open page, until the user shows it again
// (SPEC 5.12); otherwise it is the user's own choice.
export function useListHidden(): [boolean, (hidden: boolean) => void] {
  const place = useNav(current)
  const docked = useDock((s) => place?.view === "chat" && !!s.open[chatKey(place.project, place.chat)])
  const listShown = useDock((s) => s.listShown)
  const navHidden = useNav((s) => s.sidebarHidden)
  const hidden = docked ? !listShown : navHidden
  const set = (h: boolean) => (docked ? useDock.getState().setListShown(!h) : useNav.getState().setSidebarHidden(h))
  return [hidden, set]
}

// Dock is the main area of a chat (SPEC 5.9, 5.12). With nothing open it
// is the chat alone. With main, such as a file or later a page, main
// fills the area and the chat sits in a resizable dock on the project's
// side, at least 320 px wide and a third to start; the user can swap the
// side, hide the chat, or close main to get the full chat back. Below
// 900 px the chat is a drawer over main.
//
// The chat keeps its place among the children, so opening and closing
// main doesn't mount it again; the side is set with CSS order.
export function Dock({ project, chat, main, children }: { project: string; chat: string; main: React.ReactNode; children: React.ReactNode }) {
  const narrow = useIsMobile()
  const k = chatKey(project, chat)
  const side = useDock((s) => s.side[project] ?? "right")
  const width = useDock((s) => s.width[project])
  const hidden = useDock((s) => !!s.chatHidden[k])
  const drawer = useDock((s) => !!s.drawer[k])
  const setDrawer = useDock((s) => s.setDrawer)
  const box = React.useRef<HTMLDivElement>(null)
  const docked = !!main
  const left = side === "left"

  if (docked && narrow) {
    return (
      <div className="h-full min-h-0">
        {main}
        <Sheet open={drawer} onOpenChange={(o) => setDrawer(project, chat, o)}>
          <SheetContent side="right" className="w-[min(100vw,28rem)] gap-0 p-0 sm:max-w-none [&>button]:hidden">
            <SheetHeader className="sr-only">
              <SheetTitle>Chat</SheetTitle>
              <SheetDescription>The chat next to the open file</SheetDescription>
            </SheetHeader>
            {children}
          </SheetContent>
        </Sheet>
      </div>
    )
  }
  return (
    <div ref={box} className="flex h-full min-h-0">
      {docked ? (
        <section className="h-full min-h-0 min-w-0 flex-1 overflow-hidden" style={{ order: left ? 3 : 1, minWidth: MIN_DOCK }}>
          {main}
        </section>
      ) : null}
      {docked && !hidden ? <Resizer project={project} box={box} left={left} /> : null}
      <div
        className={cn("h-full min-h-0", docked ? "shrink-0 overflow-hidden" : "min-w-0 flex-1", docked && hidden && "hidden")}
        style={docked ? { order: left ? 1 : 3, width: width ?? "33.333%", minWidth: MIN_DOCK, maxWidth: `calc(100% - ${MIN_DOCK}px)` } : undefined}
      >
        {children}
      </div>
    </div>
  )
}

// Resizer is the dock's edge: drag it, or use the arrow keys when it has
// focus.
function Resizer({ project, box, left }: { project: string; box: React.RefObject<HTMLDivElement | null>; left: boolean }) {
  const setWidth = useDock((s) => s.setWidth)
  const widthAt = (x: number) => {
    const r = box.current?.getBoundingClientRect()
    if (!r) return null
    return clampWidth(left ? x - r.left : r.right - x, r.width)
  }
  const move = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!e.currentTarget.hasPointerCapture(e.pointerId)) return
    const w = widthAt(e.clientX)
    if (w !== null) setWidth(project, w)
  }
  const key = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return
    e.preventDefault()
    const r = box.current?.getBoundingClientRect()
    const dock = box.current?.querySelector<HTMLElement>(":scope > div:last-child")
    if (!r || !dock) return
    const grow = (e.key === "ArrowLeft") !== left
    setWidth(project, clampWidth(dock.getBoundingClientRect().width + (grow ? 32 : -32), r.width))
  }
  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label="Resize the chat"
      tabIndex={0}
      className="relative z-10 w-px shrink-0 cursor-col-resize bg-border outline-none after:absolute after:inset-y-0 after:-inset-x-1.5 hover:bg-ring focus-visible:bg-ring"
      style={{ order: 2 }}
      onPointerDown={(e) => {
        e.preventDefault()
        e.currentTarget.setPointerCapture(e.pointerId)
      }}
      onPointerMove={move}
      onKeyDown={key}
    />
  )
}

// DockControls are the buttons in the header of what is docked: swap the
// chat's side, hide or show the chat (below 900 px, open its drawer), and
// close.
export function DockControls({ project, chat, closeLabel }: { project: string; chat: string; closeLabel: string }) {
  const narrow = useIsMobile()
  const k = chatKey(project, chat)
  const left = useDock((s) => s.side[project] === "left")
  const hidden = useDock((s) => !!s.chatHidden[k])
  const dock = useDock.getState
  if (narrow) {
    return (
      <>
        <Button variant="outline" size="sm" onClick={() => dock().setDrawer(project, chat, true)}>
          <MessageSquareIcon data-icon="inline-start" />
          Chat
        </Button>
        <IconButton label={closeLabel} onClick={() => dock().close(project, chat)}>
          <XIcon />
        </IconButton>
      </>
    )
  }
  return (
    <>
      {!hidden && (
        <IconButton label={left ? "Move the chat to the right" : "Move the chat to the left"} onClick={() => dock().swap(project)}>
          <ArrowLeftRightIcon />
        </IconButton>
      )}
      {hidden ? (
        <Button variant="outline" size="sm" onClick={() => dock().setChatHidden(project, chat, false)}>
          <MessageSquareIcon data-icon="inline-start" />
          Show chat
        </Button>
      ) : (
        <IconButton label="Hide the chat" onClick={() => dock().setChatHidden(project, chat, true)}>
          {left ? <PanelLeftCloseIcon /> : <PanelRightCloseIcon />}
        </IconButton>
      )}
      <IconButton label={closeLabel} onClick={() => dock().close(project, chat)}>
        <XIcon />
      </IconButton>
    </>
  )
}

export function IconButton({ label, onClick, disabled, children }: { label: string; onClick: () => void; disabled?: boolean; children: React.ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label={label} onClick={onClick} disabled={disabled}>
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}
