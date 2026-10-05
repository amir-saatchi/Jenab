import * as React from "react"

import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { showError } from "@/lib/errors"
import { useChats } from "@/state/chats"
import { listen } from "@/state/events"
import { useProjects } from "@/state/projects"
import { useSettings } from "@/state/settings"
import { AppShell } from "@/shell/app-shell"
import { openProject } from "@/shell/open"

// start loads what the shell shows first and opens the project used last.
async function start() {
  await Promise.all([useSettings.getState().load(), useProjects.getState().load(), useChats.getState().loadWaiting()])
  const last = [...useProjects.getState().list].sort((a, b) => (b.last_opened ?? "").localeCompare(a.last_opened ?? ""))[0]
  if (last) await openProject(last.id)
}

export function App() {
  const [ready, setReady] = React.useState(false)
  React.useEffect(() => {
    const off = listen()
    start()
      .catch(showError)
      .finally(() => setReady(true))
    return off
  }, [])
  return (
    <TooltipProvider>
      {ready && <AppShell />}
      <Toaster />
    </TooltipProvider>
  )
}
