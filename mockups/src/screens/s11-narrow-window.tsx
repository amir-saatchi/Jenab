import { DiamondIcon, MenuIcon, MessageSquareIcon, PanelRightIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import {
  AgentMessage,
  AgentText,
  Composer,
  Notice,
  Thread,
  ToolChip,
  TurnFooter,
  UserMessage,
} from "@/jenab/chat"
import { PageBitcoin } from "@/screens/s02-page-docked"

// The window itself, below 900 px. Rendered in an iframe by Screen11 so the
// real breakpoints apply.
export function NarrowWindow() {
  const drawer = new URLSearchParams(location.search).get("drawer") === "1"
  return (
    <div className="flex h-svh flex-col overflow-hidden">
      <header className="flex h-12 shrink-0 items-center gap-2 border-b px-2">
        <Button variant="ghost" size="icon-sm" aria-label="Open sidebar">
          <MenuIcon />
        </Button>
        <span className="font-medium">Bitcoin</span>
        <div className="ms-auto flex items-center gap-1">
          <Button variant="ghost" size="sm" className="relative">
            <MessageSquareIcon data-icon="inline-start" />
            Chat
            <span className="absolute top-1 end-1 size-1.5 rounded-full bg-tone-blue" aria-label="New activity" />
          </Button>
          <Button variant="ghost" size="sm">
            <PanelRightIcon data-icon="inline-start" />
            Pages
          </Button>
        </div>
      </header>
      <div className="min-h-0 flex-1">
        <PageBitcoin />
      </div>
      <Sheet open={drawer}>
        <SheetContent side="right" className="w-[22rem] gap-0 p-0 sm:max-w-none">
          <SheetHeader className="flex-row items-center gap-2 border-b px-4 py-3">
            <DiamondIcon className="size-4 fill-current" />
            <SheetTitle>Mother</SheetTitle>
            <SheetDescription className="sr-only">Chat drawer over the page</SheetDescription>
          </SheetHeader>
          <Thread dense>
            <Notice>You opened Bitcoin</Notice>
            <UserMessage>Why did the price drop on 12 Sep?</UserMessage>
            <AgentMessage footer={<TurnFooter time="10:31" />}>
              <AgentText>Three news items that morning mention the SEC ruling on ETF options.</AgentText>
              <ToolChip name="query" detail="news · 3 rows" />
            </AgentMessage>
          </Thread>
          <Composer mother compact placeholder="Ask about this page…" className="px-3 pb-3" />
        </SheetContent>
      </Sheet>
    </div>
  )
}

function Frame({ label, src }: { label: string; src: string }) {
  return (
    <div className="flex flex-col gap-2">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <iframe
        title={label}
        src={src}
        className="h-[800px] w-[640px] rounded-xl border bg-background shadow-sm"
      />
    </div>
  )
}

// 11 · Narrow window (SPEC 5.9, 5.12): two windows at 640 px, the smallest size.
export function Screen11() {
  const theme = new URLSearchParams(location.search).get("theme") ?? "light"
  return (
    <div className="flex h-svh items-start justify-center gap-8 overflow-hidden bg-muted/40 px-8 py-6">
      <Frame label="Page: rows stack into one column" src={`?screen=11w&theme=${theme}`} />
      <Frame label="Chat opens as a drawer over the page" src={`?screen=11w&theme=${theme}&drawer=1`} />
    </div>
  )
}
