import * as React from "react"
import { MonitorIcon, MoonIcon, SunIcon, TriangleAlertIcon } from "lucide-react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { showError } from "@/lib/errors"
import { readTheme, type ThemeChoice } from "@/lib/theme"
import { DeveloperSection } from "@/settings/developer"
import { GeneralSection } from "@/settings/general"
import { LimitsSection, ModelsSection } from "@/settings/models"
import { UsageSection } from "@/settings/usage"
import { useSettings } from "@/state/settings"
import { backToChats } from "@/shell/open"
import { FactRow, SettingsDivider, SettingsLayout, SettingsSection, SettingsTitle } from "@/shell/settings-layout"

const pages = ["General", "Appearance", "Models", "Usage", "Developer"]

// SettingsView is *Settings* as a full view (SPEC 5.12). Search,
// connections, MCP servers and updates come with their tickets.
export function SettingsView() {
  const view = useSettings((s) => s.view)
  const setTheme = useSettings((s) => s.setTheme)
  const theme = readTheme(view?.settings.ui.theme)

  React.useEffect(() => {
    const s = useSettings.getState()
    Promise.all([s.load(), s.loadModels(), s.started ? null : s.loadStarted()]).catch(showError)
  }, [])

  return (
    <SettingsLayout items={pages} onBack={backToChats}>
      <SettingsTitle title="Settings" />
      <Problems />
      <GeneralSection />
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
      <DeveloperSection />
    </SettingsLayout>
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
