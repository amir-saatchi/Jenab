import * as React from "react"
import { KeyRoundIcon, PlusIcon, ServerIcon } from "lucide-react"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Item, ItemActions, ItemContent, ItemDescription, ItemGroup, ItemMedia, ItemTitle } from "@/components/ui/item"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@/components/ui/select"
import type { ProviderSettings, Settings } from "@/lib/api"
import { showError } from "@/lib/errors"
import { ConnectForm, type Replace } from "@/chat/provider-card"
import { limitDefault } from "@/settings/format"
import { NumberInput } from "@/settings/number-input"
import { ProviderModelsDialog } from "@/settings/provider-models"
import { useSettings } from "@/state/settings"
import { SettingsSection } from "@/shell/settings-layout"

// ModelsSection is *Settings → Models* (SPEC 3.9): the connected
// providers with their keys, models and limits, and the default and fast
// models.
export function ModelsSection() {
  const llm = useSettings((s) => s.view?.settings.llm)
  const [adding, setAdding] = React.useState(false)
  const [models, setModels] = React.useState<string | null>(null)
  const [replace, setReplace] = React.useState<Replace | null>(null)
  const [removing, setRemoving] = React.useState<string | null>(null)
  const providers = Object.entries(llm?.providers ?? {}).sort(([a], [b]) => a.localeCompare(b))

  return (
    <>
      <SettingsSection title="Models" description="Jenab works the same with every model. Keys are stored in the OS keychain.">
        {providers.length > 0 ? (
          <ItemGroup className="gap-2">
            {providers.map(([name, p]) => (
              <ProviderRow
                key={name}
                name={name}
                p={p!}
                onModels={() => setModels(name)}
                onReplace={() => setReplace({ name, kind: p!.kind, base_url: p!.base_url })}
                onRemove={() => setRemoving(name)}
              />
            ))}
          </ItemGroup>
        ) : (
          <p className="text-sm text-muted-foreground">No provider yet. Add one with your own key.</p>
        )}
        <div className="flex items-center gap-3">
          <Button variant="outline" size="sm" onClick={() => setAdding(true)}>
            <PlusIcon data-icon="inline-start" />
            Add provider
          </Button>
          <span className="text-xs text-muted-foreground">Any OpenAI-compatible URL</span>
        </div>
      </SettingsSection>
      <Aliases />

      <Dialog open={adding} onOpenChange={setAdding}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Add provider</DialogTitle>
            <DialogDescription>The key is checked with the provider and stored in the OS keychain.</DialogDescription>
          </DialogHeader>
          {adding && <ConnectForm id="settings-connect" onConnected={() => setAdding(false)} />}
        </DialogContent>
      </Dialog>
      <Dialog open={!!replace} onOpenChange={(o) => !o && setReplace(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>New key for {replace?.name}</DialogTitle>
            <DialogDescription>The saved key is never shown. A new one replaces it; the models stay as they are.</DialogDescription>
          </DialogHeader>
          {replace && <ConnectForm id="settings-replace" replace={replace} onConnected={() => setReplace(null)} />}
        </DialogContent>
      </Dialog>
      {models && <ProviderModelsDialog name={models} onClose={() => setModels(null)} />}
      <RemoveProvider name={removing} onClose={() => setRemoving(null)} />
    </>
  )
}

function ProviderRow({
  name,
  p,
  onModels,
  onReplace,
  onRemove,
}: {
  name: string
  p: ProviderSettings
  onModels: () => void
  onReplace: () => void
  onRemove: () => void
}) {
  const llm = useSettings((s) => s.view!.settings.llm)
  const edit = useSettings((s) => s.edit)
  const local = p.kind === "ollama" && limitDefault(p.kind, p.base_url, 2) === 1
  const on = p.models?.length ?? 0
  const own = llm.provider_max_parallel_calls?.[name]
  const id = `limit-${name}`
  const setLimit = (v: number | null) =>
    edit((s) => {
      const m = (s.llm.provider_max_parallel_calls ??= {})
      if (v === null) delete m[name]
      else m[name] = v
    }).catch(showError)

  return (
    <Item variant="outline" size="sm">
      <ItemMedia variant="icon">{local ? <ServerIcon /> : <KeyRoundIcon />}</ItemMedia>
      <ItemContent className="min-w-0">
        <ItemTitle>
          {name}
          <Badge variant="outline" className="font-normal">
            {kindName(p.kind)}
          </Badge>
        </ItemTitle>
        <ItemDescription>
          {local ? "On this computer · no key" : "Key in the keychain"} · {on === 1 ? "1 model on" : `${on} models on`}
        </ItemDescription>
        {p.base_url && (
          <ItemDescription className="truncate font-mono text-xs" dir="ltr" title={p.base_url}>
            {p.base_url}
          </ItemDescription>
        )}
        <div className="mt-1 flex items-center gap-2">
          <Label htmlFor={id} className="text-xs font-normal text-muted-foreground">
            Background calls at once
          </Label>
          <NumberInput
            id={id}
            className="h-7 w-16"
            min={1}
            optional
            value={own ?? null}
            placeholder={String(limitDefault(p.kind, p.base_url, llm.max_parallel_calls))}
            onCommit={setLimit}
            title="Empty uses the default"
          />
        </div>
      </ItemContent>
      <ItemActions>
        <Button variant="ghost" size="sm" onClick={onModels}>
          Models
        </Button>
        {!local && (
          <Button variant="ghost" size="sm" onClick={onReplace}>
            Replace key
          </Button>
        )}
        <Button variant="ghost" size="sm" className="text-muted-foreground" onClick={onRemove}>
          Remove
        </Button>
      </ItemActions>
    </Item>
  )
}

const kinds: Record<string, string> = {
  anthropic: "Anthropic",
  openai: "OpenAI",
  gemini: "Gemini",
  openai_compatible: "OpenAI-compatible",
  ollama: "Ollama",
}

function kindName(kind: string) {
  return kinds[kind] ?? kind
}

function RemoveProvider({ name, onClose }: { name: string | null; onClose: () => void }) {
  const remove = useSettings((s) => s.removeProvider)
  const [last, setLast] = React.useState(name)
  if (name && name !== last) setLast(name)
  return (
    <AlertDialog open={!!name} onOpenChange={(o) => !o && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Remove {last}?</AlertDialogTitle>
          <AlertDialogDescription>
            Its key is deleted from the OS keychain and its models are turned off. Chats that use them need another
            model. Default and fast move to another provider&apos;s models.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={() => last && remove(last).catch(showError)}>
            Remove
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

// Aliases picks the models behind default and fast (SPEC 3.9).
function Aliases() {
  const groups = useSettings((s) => s.models)
  const aliases = useSettings((s) => s.view?.settings.llm.models)
  const edit = useSettings((s) => s.edit)
  const set = (alias: string, ref: string) =>
    edit((s: Settings) => {
      ;(s.llm.models ??= {})[alias] = ref
    }).catch(showError)
  const all = (groups ?? []).flatMap((g) => (g.models ?? []).map((m) => m.ref))

  const pick = (alias: string, label: string, hint: string) => {
    const value = aliases?.[alias] ?? ""
    return (
      <Field>
        <FieldLabel htmlFor={`alias-${alias}`}>{label}</FieldLabel>
        <Select value={value} onValueChange={(v) => set(alias, v)} disabled={!groups?.length}>
          <SelectTrigger id={`alias-${alias}`} className="w-full">
            <SelectValue placeholder="No model" />
          </SelectTrigger>
          <SelectContent>
            {(groups ?? []).map((g) => (
              <SelectGroup key={g.provider}>
                <SelectLabel>{g.provider}</SelectLabel>
                {(g.models ?? []).map((m) => (
                  <SelectItem key={m.ref} value={m.ref}>
                    {m.name}
                  </SelectItem>
                ))}
              </SelectGroup>
            ))}
            {value && !all.includes(value) && (
              <SelectGroup>
                <SelectLabel>Not on</SelectLabel>
                <SelectItem value={value}>{value}</SelectItem>
              </SelectGroup>
            )}
          </SelectContent>
        </Select>
        <FieldDescription>{hint}</FieldDescription>
      </Field>
    )
  }

  return (
    <SettingsSection title="Default and fast model" description="Built-in model list, updated with the app.">
      <FieldGroup className="grid grid-cols-2 gap-4 max-[899px]:grid-cols-1">
        {pick("default", "Default model", "New chats start with it.")}
        {pick("fast", "Fast model", "For titles and summaries.")}
      </FieldGroup>
    </SettingsSection>
  )
}

const limits: { label: string; hint?: string; min: number; get: (s: Settings) => number; set: (s: Settings, v: number) => void }[] = [
  {
    label: "Parallel background LLM calls",
    hint: "Chats count but never wait",
    min: 1,
    get: (s) => s.llm.max_parallel_calls,
    set: (s, v) => (s.llm.max_parallel_calls = v),
  },
  {
    label: "Parallel pipeline runs",
    min: 1,
    get: (s) => s.scheduler.max_parallel_runs,
    set: (s, v) => (s.scheduler.max_parallel_runs = v),
  },
  {
    label: "Background tasks per chat",
    min: 0,
    get: (s) => s.llm.max_background_tasks_per_chat,
    set: (s, v) => (s.llm.max_background_tasks_per_chat = v),
  },
  {
    label: "Subagents at once per chat",
    min: 1,
    get: (s) => s.llm.max_subagents_per_chat,
    set: (s, v) => (s.llm.max_subagents_per_chat = v),
  },
  {
    label: "Requests per turn",
    min: 2,
    get: (s) => s.llm.turn_max_requests,
    set: (s, v) => (s.llm.turn_max_requests = v),
  },
  {
    label: "Requests per app-started turn",
    min: 2,
    get: (s) => s.llm.system_turn_max_requests,
    set: (s, v) => (s.llm.system_turn_max_requests = v),
  },
  {
    label: "Tokens per app-started turn",
    min: 1000,
    get: (s) => s.llm.system_turn_max_tokens,
    set: (s, v) => (s.llm.system_turn_max_tokens = v),
  },
]

// LimitsSection holds the limits of SPEC 7.6. A change applies at once.
export function LimitsSection() {
  const settings = useSettings((s) => s.view?.settings)
  const edit = useSettings((s) => s.edit)
  if (!settings) return null
  return (
    <SettingsSection title="Limits" description="Starting values, tuned with the benchmark. Changes apply at once.">
      <FieldGroup className="grid max-w-md gap-y-2">
        {limits.map((l, i) => (
          <Field key={l.label} orientation="horizontal" className="justify-between">
            <div className="flex flex-col">
              <Label htmlFor={`limit-${i}`} className="font-normal">
                {l.label}
              </Label>
              {l.hint && <span className="text-xs text-muted-foreground">{l.hint}</span>}
            </div>
            <NumberInput
              id={`limit-${i}`}
              min={l.min}
              value={l.get(settings)}
              onCommit={(v) => edit((s) => void l.set(s, v!)).catch(showError)}
            />
          </Field>
        ))}
      </FieldGroup>
      <p className="text-xs text-muted-foreground">
        <Badge variant="outline" className="me-1.5">
          config.yaml
        </Badge>
        Saved in the user config file, with your comments kept. A limit for one provider is on its row above.
      </p>
    </SettingsSection>
  )
}
