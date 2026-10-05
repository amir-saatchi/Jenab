import { MonitorIcon, MoonIcon, SunIcon } from "lucide-react"

import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { showError } from "@/lib/errors"
import { readTheme, type ThemeChoice } from "@/lib/theme"
import { useSettings } from "@/state/settings"
import { FolderWarning } from "@/shell/folder-warning"
import { backToChats } from "@/shell/open"
import { FactRow, SettingsDivider, SettingsLayout, SettingsSection, SettingsTitle } from "@/shell/settings-layout"

// SettingsView is *Settings* as a full view (SPEC 5.12). P1-16 adds
// Models, Usage and the other pages.
export function SettingsView() {
  const view = useSettings((s) => s.view)
  const setTheme = useSettings((s) => s.setTheme)
  const theme = readTheme(view?.settings.ui.theme)
  return (
    <SettingsLayout items={["General", "Appearance"]} onBack={backToChats}>
      <SettingsTitle title="Settings" />
      <SettingsSection title="General">
        <FactRow label="Data folder">
          <span className="block truncate font-mono text-xs" dir="ltr">
            {view?.settings.data_folder || "Default"}
          </span>
        </FactRow>
        <FolderWarning />
      </SettingsSection>
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
    </SettingsLayout>
  )
}
