import * as React from "react"
import { WrenchIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { useNav } from "@/state/nav"
import { useSettings } from "@/state/settings"
import { backToChats } from "@/shell/open"
import { SettingsLayout } from "@/shell/settings-layout"
import { settingsPages } from "@/shell/settings-view"

// DevPage frames a developer tool (SPEC 8.4) as a wide page of Settings,
// under Developer. A page in the list opens Settings at its section. The
// tools are read-only, and every text has its secrets redacted.
export function DevPage({
  title,
  description,
  actions,
  children,
}: {
  title: string
  description: string
  actions?: React.ReactNode
  children: React.ReactNode
}) {
  const started = useSettings((s) => s.started)
  React.useEffect(() => {
    if (!useSettings.getState().started) useSettings.getState().loadStarted().catch(() => {})
  }, [])
  return (
    <SettingsLayout
      items={settingsPages}
      active="Developer"
      onSelect={(section) => useNav.getState().go({ view: "settings", section })}
      onBack={backToChats}
      wide
    >
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
          <p className="text-sm text-muted-foreground">{description}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {started?.dev_tools && actions}
          <Badge variant="outline">Read-only · secrets redacted</Badge>
        </div>
      </div>
      {started && !started.dev_tools ? <DevToolsOff /> : started && children}
    </SettingsLayout>
  )
}

function DevToolsOff() {
  return (
    <Empty className="border">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <WrenchIcon />
        </EmptyMedia>
        <EmptyTitle>The developer tools are off</EmptyTitle>
        <EmptyDescription>Turn them on in Settings → Developer. They apply at the next start of Jenab.</EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}
