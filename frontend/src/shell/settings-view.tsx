import * as React from "react"
import { System } from "@wailsio/runtime"
import { FolderCogIcon, MonitorIcon, MoonIcon, SunIcon, TriangleAlertIcon } from "lucide-react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import type { ProjectItem } from "@/lib/api"
import { showError } from "@/lib/errors"
import { readTheme, type ThemeChoice } from "@/lib/theme"
import { DesktopSection } from "@/settings/desktop"
import { DeveloperSection } from "@/settings/developer"
import { GeneralSection } from "@/settings/general"
import { UserMemorySection } from "@/settings/memory"
import { LimitsSection, ModelsSection } from "@/settings/models"
import { UsageSection } from "@/settings/usage"
import { useSettings } from "@/state/settings"
import { backToChats, openProjectSettings, useOpenProject } from "@/shell/open"
import { FactRow, SettingsDivider, SettingsLayout, SettingsSection, SettingsTitle, sectionId } from "@/shell/settings-layout"

// Desktop control is Windows-only in v1 (SPEC 8.10), and so is its page
// (tests run without a window).
export const settingsPages = ["General", "Memory", "Appearance", "Models", "Usage", ...(typeof window !== "undefined" && System.IsWindows() ? ["Desktop"] : []), "Developer"]

// SettingsView is *Settings* as a full view (SPEC 5.12). Search,
// connections, MCP servers and updates come with their tickets.
export function SettingsView({ section }: { section?: string }) {
  const view = useSettings((s) => s.view)
  const setTheme = useSettings((s) => s.setTheme)
  const theme = readTheme(view?.settings.ui.theme)
  const openProject = useOpenProject()

  React.useEffect(() => {
    const s = useSettings.getState()
    Promise.all([s.load(), s.loadModels(), s.started ? null : s.loadStarted()]).catch(showError)
  }, [])
  // Opened at a section, from a developer tool's page list.
  const loaded = !!view
  React.useEffect(() => {
    if (section && loaded) document.getElementById(sectionId(section))?.scrollIntoView({ block: "start" })
  }, [section, loaded])

  return (
    <SettingsLayout key={section} items={settingsPages} active={section} onBack={backToChats} more={<OpenProjectPages project={openProject} />}>
      <SettingsTitle title="Settings" />
      <Problems />
      <GeneralSection />
      <SettingsDivider />
      <UserMemorySection />
      <SettingsDivider />
      <SettingsSection title="Appearance" description="Applies at once.">
        <FactRow label="Theme">
          <ToggleGroup
            type="single"
            variant="outline"
            size="sm"
            value={theme}
            onValueChange={(v) => v && setTheme(v as ThemeChoice).catch(showError)}
          >
            <ToggleGroupItem value="light">
              <SunIcon data-icon="inline-start" />
              Light
            </ToggleGroupItem>
            <ToggleGroupItem value="dark">
              <MoonIcon data-icon="inline-start" />
              Dark
            </ToggleGroupItem>
            <ToggleGroupItem value="system">
              <MonitorIcon data-icon="inline-start" />
              System
            </ToggleGroupItem>
          </ToggleGroup>
        </FactRow>
      </SettingsSection>
      <SettingsDivider />
      <ModelsSection />
      <LimitsSection />
      <SettingsDivider />
      <UsageSection />
      <SettingsDivider />
      {System.IsWindows() && (
        <>
          <DesktopSection />
          <SettingsDivider />
        </>
      )}
      <DeveloperSection />
    </SettingsLayout>
  )
}

// OpenProjectPages is the open project in Settings' page list, under its
// name: its *Project settings*. Nothing with no project open.
export function OpenProjectPages({ project }: { project?: ProjectItem }) {
  if (!project) return null
  return (
    <SidebarGroup>
      <SidebarGroupLabel>
        <span className="truncate" dir="auto">
          {project.name}
        </span>
      </SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton onClick={() => openProjectSettings(project.id)}>
              <FolderCogIcon />
              <span>Project settings</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  )
}

// Problems lists the settings in config.yaml that were ignored, with the
// defaults used instead.
function Problems() {
  const problems = useSettings((s) => s.view?.problems ?? [])
  if (problems.length === 0) return null
  return (
    <Alert>
      <TriangleAlertIcon className="text-tone-amber" />
      <AlertTitle>Some settings in config.yaml were ignored</AlertTitle>
      <AlertDescription>
        <ul className="list-disc ps-4">
          {problems.map((p, i) => (
            <li key={i}>
              {p.line > 0 && `Line ${p.line}: `}
              {p.path && <span className="font-mono text-xs">{p.path}</span>}
              {p.path && ": "}
              {p.msg}
            </li>
          ))}
        </ul>
      </AlertDescription>
    </Alert>
  )
}
