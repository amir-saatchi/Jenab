import * as React from "react"

import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { ChatService } from "@/lib/api"
import { showError } from "@/lib/errors"
import { useProjects } from "@/state/projects"
import { FolderWarning } from "@/shell/folder-warning"
import { backToChats } from "@/shell/open"
import { ProjectMemorySection } from "@/settings/memory"
import { FactRow, SettingsDivider, SettingsLayout, SettingsSection, SettingsTitle, sectionId } from "@/shell/settings-layout"

export const levels = [
  { value: "strict", label: "Strict" },
  { value: "standard", label: "Standard" },
  { value: "auto", label: "Auto" },
]

// ProjectSettings is one scrolling page, opened from the project's menu
// (SPEC 5.12). The workspace, storage, hosts and Export come with their
// tickets.
export function ProjectSettings({ project, section }: { project: string; section?: string }) {
  const item = useProjects((s) => s.list.find((p) => p.id === project))
  const opened = useProjects((s) => s.opened[project])
  const created = item ? new Date(item.created_at).toLocaleDateString(undefined, { dateStyle: "medium" }) : ""
  // Opened at a section, from a chat's memory chip.
  React.useEffect(() => {
    if (section) document.getElementById(sectionId(section))?.scrollIntoView({ block: "start" })
  }, [section])

  // One change at a time, so the calls can't finish out of order.
  const [busy, setBusy] = React.useState(false)
  const out = React.useRef(false)
  const setLevel = async (level: string) => {
    if (!opened || out.current || level === opened.level) return
    out.current = true
    setBusy(true)
    try {
      await ChatService.SetLevel(project, opened.mother, level)
      useProjects.getState().setLevel(project, level)
    } catch (err) {
      showError(err)
    } finally {
      out.current = false
      setBusy(false)
    }
  }

  return (
    <SettingsLayout
      key={section}
      label={item ? `Project: ${item.name}` : "Project"}
      items={["General", "Memory", "Approvals"]}
      active={section}
      onBack={backToChats}
    >
      <SettingsTitle title="Project settings" description={item ? `${item.name} · created ${created}` : ""} />
      <SettingsSection title="General">
        <FactRow label="Name">
          <span dir="auto">{item?.name}</span>
        </FactRow>
        <FactRow label="Folder">
          <span className="block truncate font-mono text-xs" dir="ltr">
            {item?.folder}
          </span>
        </FactRow>
        <FolderWarning />
      </SettingsSection>
      <SettingsDivider />
      <ProjectMemorySection project={project} />
      <SettingsDivider />
      <SettingsSection
        title="Approvals"
        description="How much the agent asks before it acts. A change applies from the next tool call."
      >
        <FactRow label="Approval level">
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            value={opened?.level ?? ""}
            disabled={!opened || busy}
            onValueChange={(v) => v && void setLevel(v)}
          >
            {levels.map((l) => (
              <ToggleGroupItem key={l.value} value={l.value}>
                {l.label}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        </FactRow>
      </SettingsSection>
    </SettingsLayout>
  )
}
