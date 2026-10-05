import * as React from "react"
import { GlobeIcon, KeyRoundIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { Spinner } from "@/components/ui/spinner"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { SettingsService, type ConnectResult, type PresetItem } from "@/lib/api"
import { showError } from "@/lib/errors"
import { placeholders } from "@/settings/format"
import { useSettings } from "@/state/settings"

const OTHER = "other"

// freeName is name, or name-2, name-3… when a provider has it.
function freeName(name: string, taken: Record<string, unknown>) {
  if (!(name in taken)) return name
  for (let i = 2; ; i++) if (!(`${name}-${i}` in taken)) return `${name}-${i}`
}

// Replace names a connected provider whose key is replaced; its models stay.
export interface Replace {
  name: string
  kind: string
  base_url?: string
}

// ConnectForm adds a provider with the user's own key (SPEC 3.9). The key
// goes to the OS keychain; the models it lists are turned on. With replace,
// it takes a new key for a connected provider.
export function ConnectForm({
  onConnected,
  id = "connect",
  replace,
}: {
  onConnected?: (r: ConnectResult) => void
  id?: string
  replace?: Replace
}) {
  const [presets, setPresets] = React.useState<PresetItem[] | null>(replace ? [] : null)
  const [pick, setPick] = React.useState("")
  const [key, setKey] = React.useState("")
  const [fields, setFields] = React.useState<Record<string, string>>({})
  const [other, setOther] = React.useState({ name: "", base_url: "" })
  const [busy, setBusy] = React.useState(false)
  const providers = useSettings((s) => s.view?.settings.llm.providers ?? {})

  React.useEffect(() => {
    if (replace) return
    SettingsService.Presets()
      .then((ps) => {
        setPresets(ps ?? [])
        setPick((ps ?? [])[0]?.id ?? OTHER)
      })
      .catch(showError)
  }, [replace])

  const preset = presets?.find((p) => p.id === pick)
  const isOther = !replace && pick === OTHER
  const needsKey = !!replace || isOther || !preset?.no_key
  const wanted = replace ? placeholders(replace.base_url) : (preset?.fields ?? [])
  const ready =
    !busy &&
    (replace || (isOther ? other.name.trim() !== "" && other.base_url.trim() !== "" : !!preset)) &&
    (!needsKey || key.trim() !== "") &&
    wanted.every((f) => (fields[f] ?? "").trim() !== "")

  const connect = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!ready) return
    setBusy(true)
    try {
      const r = await SettingsService.Connect(
        replace
          ? { name: replace.name, kind: replace.kind, base_url: replace.base_url ?? "", key, fields }
          : isOther
            ? { name: other.name.trim(), kind: "openai_compatible", base_url: other.base_url.trim(), key, fields: {} }
            : { name: freeName(preset!.id, providers), kind: preset!.kind, base_url: preset!.base_url, key, fields },
      )
      useSettings.setState({ view: r.view })
      await useSettings.getState().loadModels()
      setKey("")
      const name = replace || isOther ? r.provider : preset!.name
      toast.success(
        replace
          ? `${name}: the new key works and is saved`
          : r.no_model_list
          ? `${name} connected. It lists no models; add them in Settings → Models.`
          : `${name} connected · ${r.models} ${r.models === 1 ? "model" : "models"} on`,
      )
      onConnected?.(r)
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }

  if (!presets) return <Spinner />
  return (
    <form className="flex flex-col gap-4" onSubmit={connect}>
      {!replace && (
        <ToggleGroup
          type="single"
          value={pick}
          onValueChange={(v) => v && setPick(v)}
          variant="outline"
          size="sm"
          className="flex-wrap"
          aria-label="Provider"
        >
          {presets.map((p) => (
            <ToggleGroupItem key={p.id} value={p.id}>
              {p.name}
              {p.running && <span className="text-xs text-tone-green">running</span>}
            </ToggleGroupItem>
          ))}
          <ToggleGroupItem value={OTHER}>Other…</ToggleGroupItem>
        </ToggleGroup>
      )}
      <FieldGroup>
        {isOther && (
          <>
            <Field>
              <FieldLabel htmlFor={`${id}-name`}>Name</FieldLabel>
              <Input
                id={`${id}-name`}
                value={other.name}
                placeholder="my-provider"
                onChange={(e) => setOther({ ...other, name: e.target.value })}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={`${id}-url`}>Base URL</FieldLabel>
              <InputGroup>
                <InputGroupAddon>
                  <GlobeIcon />
                </InputGroupAddon>
                <InputGroupInput
                  id={`${id}-url`}
                  value={other.base_url}
                  placeholder="https://example.com/v1/"
                  onChange={(e) => setOther({ ...other, base_url: e.target.value })}
                />
              </InputGroup>
              <FieldDescription>Any OpenAI-compatible API.</FieldDescription>
            </Field>
          </>
        )}
        {wanted.map((f) => (
          <Field key={f}>
            <FieldLabel htmlFor={`${id}-${f}`}>{f.replaceAll("_", " ")}</FieldLabel>
            <Input id={`${id}-${f}`} value={fields[f] ?? ""} onChange={(e) => setFields({ ...fields, [f]: e.target.value })} />
          </Field>
        ))}
        {needsKey ? (
          <Field>
            <FieldLabel htmlFor={`${id}-key`}>API key</FieldLabel>
            <InputGroup>
              <InputGroupAddon>
                <KeyRoundIcon />
              </InputGroupAddon>
              <InputGroupInput
                id={`${id}-key`}
                type="password"
                autoComplete="off"
                value={key}
                onChange={(e) => setKey(e.target.value)}
              />
            </InputGroup>
            <FieldDescription>
              {replace ? "Checked with the provider, then stored in the OS keychain" : "Stored in the OS keychain · also in Settings → Models"}
            </FieldDescription>
          </Field>
        ) : (
          <FieldDescription>
            {preset?.running ? "Ollama runs on this computer; no key is needed." : "Start Ollama on this computer first."}
          </FieldDescription>
        )}
      </FieldGroup>
      <Button type="submit" size="sm" className="w-fit" disabled={!ready}>
        {busy && <Spinner data-icon="inline-start" />}
        {replace ? "Save key" : "Connect"}
      </Button>
    </form>
  )
}

// ProviderCard is shown when a message was sent with no provider set up.
// The message is kept; once a provider is connected, Retry sends it.
export function ProviderCard({ title, onConnected }: { title: string; onConnected: () => void }) {
  return (
    <Card size="sm" className="not-typeset">
      <CardHeader>
        <CardTitle dir="auto">{title} needs an AI model to answer</CardTitle>
        <CardDescription>Add a provider with your own key. Your message is kept and sent once it is connected.</CardDescription>
      </CardHeader>
      <CardContent>
        <ConnectForm onConnected={onConnected} />
      </CardContent>
    </Card>
  )
}
