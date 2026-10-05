import { FolderIcon, PlusIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useNav } from "@/state/nav"
import { useSettings } from "@/state/settings"
import { useUI } from "@/state/ui"
import { FolderWarning } from "@/shell/folder-warning"

// Welcome is the first run (SPEC 3.9): no wizard, just *Create project*.
// *Open the example* comes with the Bitcoin example (Phases 3 and 4).
export function Welcome() {
  const folder = useSettings((s) => s.view?.settings.data_folder) || "~/Jenab"
  return (
    <div className="flex h-full flex-col">
      <Empty className="flex-1">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <FolderIcon />
          </EmptyMedia>
          <EmptyTitle>Welcome to Jenab</EmptyTitle>
          <EmptyDescription>Each project is one folder with its data, chats and files.</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <div className="flex gap-2">
            <Button onClick={() => useUI.getState().set({ newProject: true })}>
              <PlusIcon data-icon="inline-start" />
              Create project
            </Button>
            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <Button variant="outline" disabled>
                    Open the example
                  </Button>
                </span>
              </TooltipTrigger>
              <TooltipContent>The example comes with a later version</TooltipContent>
            </Tooltip>
          </div>
          <p className="text-xs text-muted-foreground">
            Saved in{" "}
            <span className="font-mono" dir="ltr">
              {folder}
            </span>{" "}
            ·{" "}
            <button type="button" className="underline underline-offset-2 hover:text-foreground" onClick={() => useNav.getState().go({ view: "settings" })}>
              change in Settings
            </button>
          </p>
          <FolderWarning className="text-start" />
        </EmptyContent>
      </Empty>
    </div>
  )
}
