import * as React from "react"

import { CopyButton } from "@/chat/parts"
import { SystemService, type VersionInfo } from "@/lib/api"
import { showError } from "@/lib/errors"
import { buildLine, versionLine } from "@/settings/format"
import { FactRow } from "@/shell/settings-layout"

// About is the app's version and build in Settings → General (TASK-037).
// Copy gives both lines, for a bug report.
export function About() {
  const [v, setV] = React.useState<VersionInfo | null>(null)
  React.useEffect(() => {
    let live = true
    SystemService.Version()
      .then((r) => live && setV(r))
      .catch(showError)
    return () => {
      live = false
    }
  }, [])
  if (!v) return null
  const line = versionLine(v)
  const build = buildLine(v)
  return (
    <FactRow label="About" action={<CopyButton text={`${line}\n${build}`} />}>
      <div className="flex flex-col gap-0.5" dir="ltr">
        <span className="truncate font-mono text-xs">{line}</span>
        <span className="truncate text-xs text-muted-foreground">{build}</span>
      </div>
    </FactRow>
  )
}
