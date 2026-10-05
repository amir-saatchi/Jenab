import * as React from "react"
import { TriangleAlertIcon } from "lucide-react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { SettingsService, type ModelOption, type ProviderModels } from "@/lib/api"
import { showError } from "@/lib/errors"
import { contextSize } from "@/settings/format"
import { NumberInput } from "@/settings/number-input"
import { useSettings } from "@/state/settings"

type Models = ProviderModels & { models: ModelOption[]; other: ModelOption[] }

// ProviderModelsDialog turns a provider's models on and off (SPEC 3.9):
// the catalog models it lists, *Other models* the catalog doesn't know,
// which need a context window, and models typed in by hand.
export function ProviderModelsDialog({ name, onClose }: { name: string; onClose: () => void }) {
  const [pm, setPm] = React.useState<Models | null>(null)
  const aliases = useSettings((s) => s.view?.settings.llm.models ?? {})
  const on = useSettings((s) => s.view?.settings.llm.providers?.[name]?.models ?? [])
  const edit = useSettings((s) => s.edit)

  const close = React.useRef(onClose)
  close.current = onClose
  React.useEffect(() => {
    SettingsService.ProviderModels(name)
      .then((pm) => setPm({ ...pm, models: pm.models ?? [], other: pm.other ?? [] }))
      .catch((err) => {
        showError(err)
        close.current()
      })
  }, [name])

  // aliased models can't be turned off until default or fast moves.
  const aliased = (id: string) =>
    Object.entries(aliases)
      .filter(([, ref]) => ref === `${name}/${id}`)
      .map(([a]) => a)

  const toggle = (m: ModelOption, value: boolean, context = m.context) =>
    edit((s) => {
      const p = s.llm.providers?.[name]
      if (!p) return
      const ms = (p.models ?? []).filter((x) => x.id !== m.id)
      if (value) ms.push(m.known ? { id: m.id } : { id: m.id, context })
      p.models = ms
    }).catch(showError)

  const isOn = (id: string) => on.some((m) => m.id === id)
  const listed = new Set([...(pm?.models ?? []), ...(pm?.other ?? [])].map((m) => m.id))
  const typed = on.filter((m) => !listed.has(m.id)).map((m): ModelOption => ({ id: m.id, name: m.id, on: true, context: m.context ?? 0, known: false }))

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Models of {name}</DialogTitle>
          <DialogDescription>Models that are on appear in the model picker at once.</DialogDescription>
        </DialogHeader>
        {!pm ? (
          <Spinner />
        ) : (
          <ScrollArea className="max-h-[60vh] pe-3">
            <div className="flex flex-col gap-4">
              {pm.problem && (
                <Alert>
                  <TriangleAlertIcon className="text-tone-amber" />
                  <AlertDescription>The provider&apos;s model list couldn&apos;t be read: {pm.problem}</AlertDescription>
                </Alert>
              )}
              <ModelList
                models={[...pm.models, ...typed.filter((t) => !pm.models.some((m) => m.id === t.id))]}
                isOn={isOn}
                aliased={aliased}
                onToggle={toggle}
              />
              {pm.other.length > 0 && (
                <div className="flex flex-col gap-2">
                  <h3 className="text-sm font-medium">Other models</h3>
                  <p className="text-xs text-muted-foreground">
                    Listed by the provider but not in Jenab&apos;s model list. Turning one on needs its context window.
                  </p>
                  <ModelList models={pm.other} isOn={isOn} aliased={aliased} onToggle={toggle} askContext />
                </div>
              )}
              {(pm.no_model_list || pm.kind === "openai_compatible" || pm.kind === "ollama") && (
                <AddModel onAdd={(id, context) => toggle({ id, name: id, on: false, context, known: false }, true, context)} />
              )}
            </div>
          </ScrollArea>
        )}
      </DialogContent>
    </Dialog>
  )
}

function ModelList({
  models,
  isOn,
  aliased,
  onToggle,
  askContext,
}: {
  models: ModelOption[]
  isOn: (id: string) => boolean
  aliased: (id: string) => string[]
  onToggle: (m: ModelOption, on: boolean, context?: number) => void
  askContext?: boolean
}) {
  if (models.length === 0) return <p className="text-sm text-muted-foreground">No models listed.</p>
  return (
    <ul className="flex flex-col divide-y rounded-lg border">
      {models.map((m) => (
        <ModelRow key={m.id} m={m} on={isOn(m.id)} aliases={aliased(m.id)} onToggle={onToggle} askContext={askContext} />
      ))}
    </ul>
  )
}

function ModelRow({
  m,
  on,
  aliases,
  onToggle,
  askContext,
}: {
  m: ModelOption
  on: boolean
  aliases: string[]
  onToggle: (m: ModelOption, on: boolean, context?: number) => void
  askContext?: boolean
}) {
  const [context, setContext] = React.useState<number | null>(m.context || null)
  const id = `model-${m.id}`
  const locked = on && aliases.length > 0
  const needs = askContext && !on && !context
  return (
    <li className="flex items-center gap-3 px-3 py-2">
      <div className="flex min-w-0 flex-1 flex-col">
        <label htmlFor={id} className="flex items-center gap-1.5 text-sm">
          <span className="truncate">{m.name}</span>
          {aliases.map((a) => (
            <Badge key={a} variant="secondary">
              {a}
            </Badge>
          ))}
        </label>
        <span className="truncate font-mono text-xs text-muted-foreground" dir="ltr">
          {m.id}
          {!askContext && m.context > 0 && ` · ${contextSize(m.context)} context`}
        </span>
      </div>
      {askContext && (
        <NumberInput
          aria-label={`Context window of ${m.name}`}
          className="h-7 w-24"
          min={1000}
          optional
          value={context}
          placeholder="Context"
          disabled={on}
          onCommit={setContext}
        />
      )}
      <Switch
        id={id}
        checked={on}
        disabled={locked || needs}
        title={locked ? `Used as ${aliases.join(" and ")}; pick another model for it first` : needs ? "Enter the context window first" : undefined}
        onCheckedChange={(v) => onToggle(m, v, context ?? 0)}
      />
    </li>
  )
}

// AddModel types in a model the provider doesn't list, with its context
// window (SPEC 3.9).
function AddModel({ onAdd }: { onAdd: (id: string, context: number) => void }) {
  const [id, setId] = React.useState("")
  const [context, setContext] = React.useState("")
  const n = Number(context)
  const ok = id.trim() !== "" && Number.isInteger(n) && n >= 1000
  return (
    <form
      className="flex flex-col gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        if (!ok) return
        onAdd(id.trim(), n)
        setId("")
        setContext("")
      }}
    >
      <h3 className="text-sm font-medium">Add a model</h3>
      <FieldGroup className="grid grid-cols-[1fr_8rem_auto] items-end gap-2">
        <Field>
          <FieldLabel htmlFor="add-model-id">Model ID</FieldLabel>
          <Input id="add-model-id" dir="ltr" className="font-mono text-xs" value={id} onChange={(e) => setId(e.target.value)} />
        </Field>
        <Field>
          <FieldLabel htmlFor="add-model-context">Context</FieldLabel>
          <Input
            id="add-model-context"
            type="number"
            min={1000}
            dir="ltr"
            className="text-end tabular-nums"
            value={context}
            placeholder="32000"
            onChange={(e) => setContext(e.target.value)}
          />
        </Field>
        <Button type="submit" variant="outline" disabled={!ok}>
          Add
        </Button>
      </FieldGroup>
      <FieldDescription>For a model the provider doesn&apos;t list. The context window is in tokens.</FieldDescription>
    </form>
  )
}
