import { TriangleAlertIcon } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { useProjects } from "@/state/projects"

// FolderWarning is the 2.1 banner: the data folder is on a network drive
// or in a synced folder, where SQLite files can be damaged.
export function FolderWarning({ className }: { className?: string }) {
  const w = useProjects((s) => s.folderWarning)
  if (!w) return null
  return (
    <Alert className={className}>
      <TriangleAlertIcon className="text-tone-amber" />
      <AlertDescription>{w.text}</AlertDescription>
    </Alert>
  )
}
