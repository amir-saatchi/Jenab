import { ActivityIcon, SearchIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Field, FieldContent, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Switch } from "@/components/ui/switch"
import { showError } from "@/lib/errors"
import { useNav } from "@/state/nav"
import { useSettings } from "@/state/settings"
import { SettingsSection } from "@/shell/settings-layout"

// DeveloperSection turns the developer tools on (SPEC 8.4). They are set
// up when the app starts.
export function DeveloperSection() {
  const on = useSettings((s) => s.view?.settings.dev_tools ?? false)
  const started = useSettings((s) => s.started)
  const edit = useSettings((s) => s.edit)
  const pending = !!started && started.dev_tools !== on
  return (
    <SettingsSection title="Developer">
      <Field orientation="horizontal">
        <FieldContent>
          <FieldLabel htmlFor="dev-tools">Developer tools</FieldLabel>
          <FieldDescription>
            The turn inspector, the runtime panel and debug logging.
            {pending && " Applies at the next start."}
          </FieldDescription>
        </FieldContent>
        <Switch
          id="dev-tools"
          checked={on}
          onCheckedChange={(v) =>
            edit((s) => {
              s.dev_tools = v
            }).catch(showError)
          }
        />
      </Field>
      {started?.dev_tools && (
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => useNav.getState().go({ view: "inspector" })}>
            <SearchIcon data-icon="inline-start" />
            Turn inspector
          </Button>
          <Button variant="outline" size="sm" onClick={() => useNav.getState().go({ view: "runtime" })}>
            <ActivityIcon data-icon="inline-start" />
            Runtime
          </Button>
        </div>
      )}
    </SettingsSection>
  )
}
