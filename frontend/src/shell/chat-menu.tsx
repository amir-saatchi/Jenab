import * as React from "react"
import { DownloadIcon, EllipsisIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { ExportChat } from "@/shell/export-chat"

// ChatMenuItems are a chat's menu items, the same in the chat header and
// in the chat's row in the sidebar (SPEC 5.12).
export function ChatMenuItems({ onExport }: { onExport: () => void }) {
  return (
    <DropdownMenuGroup>
      <DropdownMenuItem onSelect={onExport}>
        <DownloadIcon />
        Export chat…
      </DropdownMenuItem>
    </DropdownMenuGroup>
  )
}

// ChatHeaderMenu is the chat header's menu button.
export function ChatHeaderMenu({ project, chat, title }: { project: string; chat: string; title: string }) {
  const [exporting, setExporting] = React.useState(false)
  return (
    <>
      <DropdownMenu>
        <Tooltip>
          <TooltipTrigger asChild>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon-sm" aria-label="Chat menu">
                <EllipsisIcon />
              </Button>
            </DropdownMenuTrigger>
          </TooltipTrigger>
          <TooltipContent>Chat menu</TooltipContent>
        </Tooltip>
        <DropdownMenuContent align="end" className="w-48">
          <ChatMenuItems onExport={() => setExporting(true)} />
        </DropdownMenuContent>
      </DropdownMenu>
      <ExportChat project={project} chat={chat} title={title} open={exporting} onOpenChange={setExporting} />
    </>
  )
}
