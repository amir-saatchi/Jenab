import * as React from "react"
import { GlobeIcon, KeyRoundIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { SettingsService, type ConnectResult, type PresetItem } from "@/lib/api"
import { showError } from "@/lib/errors"
import { fieldLabel, firstPick, formFor, providerGroups } from "@/chat/provider-choice"
import { useSettings } from "@/state/settings"

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
        setPick(firstPick(ps ?? []))
      })
      .catch(showError)
  }, [replace])

  const shape = formFor(pick, presets ?? [], replace)
  const { preset, other: isOther, key: needsKey, fields: wanted } = shape
  const ready =
    !busy &&
    shape.chosen &&
    (!isOther || (other.name.trim() !== "" && other.base_url.trim() !== "")) &&
    (!needsKey || key.trim() !== "") &&
    wanted.every((f) => (fields[f] ?? "").trim() !== "")

  const connect = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!ready) return
    setBusy(true)
    try {
      const r = await useSettings.getState().connect(
        replace
          ? { name: replace.name, kind: replace.kind, base_url: replace.base_url ?? "", key, fields }
          : isOther
            ? { name: other.name.trim(), kind: "openai_compatible", base_url: other.base_url.trim(), key, fields: {} }
            : { name: freeName(preset!.id, providers), kind: preset!.kind, base_url: preset!.base_url, key, fields },
      )
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
      <FieldGroup>
        {!replace && (
          <Field>
            <FieldLabel htmlFor={`${id}-provider`}>Provider</FieldLabel>
            <Select
              value={pick}
              onValueChange={(v) => {
                if (v === pick) return
                // A key typed for one provider never goes to another one's URL.
                setPick(v)
                setKey("")
              }}
            >
              <SelectTrigger id={`${id}-provider`} className="w-full">
                <SelectValue placeholder="Choose a provider">{isOther ? "OpenAI-compatible…" : preset?.name}</SelectValue>
              </SelectTrigger>
              <SelectContent>
                {providerGroups(presets).map((g) => (
                  <SelectGroup key={g.key}>
                    <SelectLabel>{g.label}</SelectLabel>
                    {g.items.map((c) => (
                      <SelectItem key={c.id} value={c.id} textValue={c.name}>
                        <span className="flex flex-col gap-0.5">
                          <span>{c.name}</span>{" "}
                          <span className="text-xs text-muted-foreground">{c.hint}</span>
                        </span>
                      </SelectItem>
                    ))}
                  </SelectGroup>
                ))}
              </SelectContent>
            </Select>
          </Field>
        )}
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
            <FieldLabel htmlFor={`${id}-${f}`}>{fieldLabel(f)}</FieldLabel>
            <Input id={`${id}-${f}`} value={fields[f] ?? ""} onChange={(e) => setFields({ ...fields, [f]: e.target.value })} />
            <FieldDescription>Part of the base URL · stored in the OS keychain</FieldDescription>
          </Field>
        ))}
        {needsKey && (
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
        )}
        {preset?.no_key && (
          <FieldDescription>
            {preset.running
              ? `${preset.name} runs on this computer; no key is needed.`
              : `No key is needed. Start ${preset.name} on this computer first.`}
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
