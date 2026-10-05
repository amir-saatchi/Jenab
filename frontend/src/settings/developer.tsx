import { Field, FieldContent, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Switch } from "@/components/ui/switch"
import { showError } from "@/lib/errors"
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
    </SettingsSection>
  )
}
