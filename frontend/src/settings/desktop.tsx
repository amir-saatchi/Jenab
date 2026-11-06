import * as React from "react"
import { PlusIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import {
  SettingsService,
  type DesktopAppItem,
  type DesktopList,
  type DesktopMacroItem,
  type DesktopRecipe,
  type DesktopRun,
} from "@/lib/api"
import { showError } from "@/lib/errors"
import { SettingsSection } from "@/shell/settings-layout"

const maxRecipe = 1200

// when is a stored time as the user reads it; Go's zero time is never.
export function when(t: string | undefined) {
  if (!t || t.startsWith("0001-")) return "never"
  const d = new Date(t)
  return Number.isNaN(d.getTime()) ? t : d.toLocaleString()
}

// sourceLabel says who wrote a recipe.
export function sourceLabel(source: string) {
  return source === "app" ? "comes with the app" : source === "user" ? "yours" : "saved by an agent"
}

// DesktopSection is *Settings → Desktop* (SPEC 8.11): per app, the macros
// agents saved, with their steps, params, last runs and failures, and the
// recipes; each can be edited or deleted. What was removed, by the user or
// the rules, stays listed for 30 days.
export function DesktopSection() {
  const [list, setList] = React.useState<DesktopList | null>(null)
  const load = React.useCallback(() => {
    SettingsService.Desktop().then(setList).catch(showError)
  }, [])
  React.useEffect(load, [load])
  const apps = list?.apps ?? []
  const removed = [...(list?.removed_macros ?? []).map((m) => ({ kind: "Macro", name: m.name, app: m.app, at: m.removed_at, why: m.removed_why })),
    ...(list?.removed_recipes ?? []).map((r) => ({ kind: "Recipe", name: r.name, app: r.app, at: r.removed_at, why: r.removed_why }))]
  return (
    <SettingsSection
      title="Desktop"
      description="What agents saved while they used other apps' windows: macros they replay without you, and recipes, short notes on how to do a task in an app. A macro goes after 3 failed replays in a row or 90 days unused, an agent's recipe after a year unused; yours and those that come with the app stay."
    >
      {list && apps.length === 0 && <p className="text-sm text-muted-foreground">Nothing saved yet.</p>}
      {apps.map((a) => (
        <AppBlock key={a.app} app={a} reload={load} />
      ))}
      {list && <NewRecipe apps={apps.map((a) => a.app)} reload={load} />}
      {removed.length > 0 && (
        <div className="flex flex-col gap-1.5">
          <h3 className="text-sm font-medium">Removed in the last 30 days</h3>
          <ul className="flex flex-col gap-1 text-sm text-muted-foreground">
            {removed.map((r, i) => (
              <li key={i} dir="auto">
                {r.kind} {r.name} ({r.app}), {when(r.at)}: {r.why}
              </li>
            ))}
          </ul>
        </div>
      )}
    </SettingsSection>
  )
}

function AppBlock({ app, reload }: { app: DesktopAppItem; reload: () => void }) {
  return (
    <div className="flex flex-col gap-2 rounded-lg border p-3">
      <h3 className="text-sm font-medium" dir="ltr">
        {app.app}
      </h3>
      {(app.macros ?? []).map((m) => (
        <MacroRow key={m.macro.id} item={m} reload={reload} />
      ))}
      {(app.recipes ?? []).map((r) => (
        <RecipeRow key={r.id} recipe={r} reload={reload} />
      ))}
    </div>
  )
}

function runLine(r: DesktopRun) {
  const at = when(r.at)
  if (r.ok) return `${at}: worked in ${Math.round(r.seconds)} s${r.skipped ? `, ${r.skipped} optional skipped` : ""}`
  return `${at}: failed at step ${r.failed_at}: ${r.reason}`
}

function MacroRow({ item, reload }: { item: DesktopMacroItem; reload: () => void }) {
  const m = item.macro
  const [edit, setEdit] = React.useState<{ description: string; params: string; steps: string } | null>(null)
  const [busy, setBusy] = React.useState(false)
  const runs = item.runs ?? []
  const save = async () => {
    if (!edit) return
    setBusy(true)
    try {
      await SettingsService.EditDesktopMacro({
        id: m.id,
        description: edit.description,
        params: edit.params.split(",").map((p) => p.trim()).filter(Boolean),
        steps: edit.steps,
      })
      setEdit(null)
      reload()
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }
  const remove = async () => {
    setBusy(true)
    try {
      await SettingsService.DeleteDesktopMacro(m.id)
      reload()
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="flex flex-col gap-1.5 border-t pt-2 first-of-type:border-t-0 first-of-type:pt-0">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <Badge variant="outline">Macro</Badge>
        <span className="font-medium" dir="ltr">
          {m.name}({(m.params ?? []).join(", ")})
        </span>
        {m.site && <span className="text-xs text-muted-foreground">on {m.site}</span>}
        <span className="text-xs text-muted-foreground">
          version {m.version} · {m.runs} runs, {m.failures} failed · last worked {when(m.last_worked_at)}
        </span>
        {!edit && (
          <span className="ms-auto flex gap-1">
            <Button
              size="xs"
              variant="ghost"
              disabled={busy}
              onClick={() => setEdit({ description: m.description, params: (m.params ?? []).join(", "), steps: item.steps })}
            >
              Edit
            </Button>
            <Button size="xs" variant="ghost" className="text-destructive" disabled={busy} onClick={() => void remove()}>
              Delete
            </Button>
          </span>
        )}
      </div>
      {edit ? (
        <FieldGroup className="gap-3">
          <Field>
            <FieldLabel htmlFor={`macro-${m.id}-desc`}>Description</FieldLabel>
            <Input
              id={`macro-${m.id}-desc`}
              dir="auto"
              maxLength={300}
              value={edit.description}
              onChange={(e) => setEdit({ ...edit, description: e.target.value })}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor={`macro-${m.id}-params`}>Params</FieldLabel>
            <Input
              id={`macro-${m.id}-params`}
              dir="ltr"
              value={edit.params}
              onChange={(e) => setEdit({ ...edit, params: e.target.value })}
            />
            <FieldDescription>Comma-separated; each is written as {"{name}"} in the steps.</FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor={`macro-${m.id}-steps`}>Steps</FieldLabel>
            <Textarea
              id={`macro-${m.id}-steps`}
              dir="ltr"
              className="min-h-40 font-mono text-xs"
              value={edit.steps}
              onChange={(e) => setEdit({ ...edit, steps: e.target.value })}
            />
          </Field>
          <div className="flex gap-2">
            <Button size="sm" disabled={busy} onClick={() => void save()}>
              Save
            </Button>
            <Button size="sm" variant="ghost" disabled={busy} onClick={() => setEdit(null)}>
              Cancel
            </Button>
          </div>
        </FieldGroup>
      ) : (
        <>
          <p dir="auto" className="text-start text-sm">
            {m.description}
          </p>
          <pre dir="auto" className="overflow-x-auto rounded-lg bg-muted/60 p-2 text-start font-mono text-xs whitespace-pre-wrap">
            {item.listing}
          </pre>
          {runs.length > 0 && (
            <ul className="flex flex-col gap-0.5 text-xs text-muted-foreground">
              {runs.slice(0, 5).map((r, i) => (
                <li key={i} dir="auto">
                  {runLine(r)}
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </div>
  )
}

function RecipeRow({ recipe, reload }: { recipe: DesktopRecipe; reload: () => void }) {
  const [text, setText] = React.useState<string | null>(null)
  const [busy, setBusy] = React.useState(false)
  const run = async (f: () => Promise<unknown>) => {
    setBusy(true)
    try {
      await f()
      setText(null)
      reload()
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="flex flex-col gap-1.5 border-t pt-2 first-of-type:border-t-0 first-of-type:pt-0">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <Badge variant="outline">Recipe</Badge>
        <span className="font-medium" dir="ltr">
          {recipe.name}
        </span>
        {recipe.site && <span className="text-xs text-muted-foreground">on {recipe.site}</span>}
        <span className="text-xs text-muted-foreground">
          {sourceLabel(recipe.source)} · used {recipe.uses} times
        </span>
        {text === null && (
          <span className="ms-auto flex gap-1">
            <Button size="xs" variant="ghost" disabled={busy} onClick={() => setText(recipe.text)}>
              Edit
            </Button>
            <Button
              size="xs"
              variant="ghost"
              className="text-destructive"
              disabled={busy}
              onClick={() => void run(() => SettingsService.DeleteDesktopRecipe(recipe.id))}
            >
              Delete
            </Button>
          </span>
        )}
      </div>
      {text === null ? (
        <p dir="auto" className="text-start text-sm whitespace-pre-wrap">
          {recipe.text}
        </p>
      ) : (
        <FieldGroup className="gap-3">
          <Field>
            <Textarea dir="auto" className="min-h-24" maxLength={maxRecipe} value={text} onChange={(e) => setText(e.target.value)} />
            <FieldDescription>
              {text.length} of {maxRecipe.toLocaleString()} characters.
              {recipe.source === "app" && " Your edit is kept as your own recipe; agents then see yours."}
            </FieldDescription>
          </Field>
          <div className="flex gap-2">
            <Button
              size="sm"
              disabled={busy || !text.trim()}
              onClick={() => void run(() => SettingsService.WriteDesktopRecipe({ id: recipe.id, app: "", site: "", name: "", text }))}
            >
              Save
            </Button>
            <Button size="sm" variant="ghost" disabled={busy} onClick={() => setText(null)}>
              Cancel
            </Button>
          </div>
        </FieldGroup>
      )}
    </div>
  )
}

function NewRecipe({ apps, reload }: { apps: string[]; reload: () => void }) {
  const [draft, setDraft] = React.useState<{ app: string; site: string; name: string; text: string } | null>(null)
  const [busy, setBusy] = React.useState(false)
  if (!draft)
    return (
      <Button size="sm" variant="outline" className="w-fit" onClick={() => setDraft({ app: apps[0] ?? "", site: "", name: "", text: "" })}>
        <PlusIcon data-icon="inline-start" />
        Add recipe
      </Button>
    )
  const save = async () => {
    setBusy(true)
    try {
      await SettingsService.WriteDesktopRecipe({ id: 0, ...draft })
      setDraft(null)
      reload()
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }
  return (
    <FieldGroup className="gap-3 rounded-lg border p-3">
      <Field>
        <FieldLabel htmlFor="recipe-app">App</FieldLabel>
        <Input id="recipe-app" dir="ltr" placeholder="such as calculatorapp, chrome or browser" value={draft.app}
          onChange={(e) => setDraft({ ...draft, app: e.target.value })} />
        <FieldDescription>The program's name without .exe; browser for every browser.</FieldDescription>
      </Field>
      <Field>
        <FieldLabel htmlFor="recipe-site">Site</FieldLabel>
        <Input id="recipe-site" dir="ltr" placeholder="only for a browser, such as https://example.com" value={draft.site}
          onChange={(e) => setDraft({ ...draft, site: e.target.value })} />
      </Field>
      <Field>
        <FieldLabel htmlFor="recipe-name">Name</FieldLabel>
        <Input id="recipe-name" dir="ltr" placeholder="such as site_search" maxLength={41} value={draft.name}
          onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
      </Field>
      <Field>
        <FieldLabel htmlFor="recipe-text">Text</FieldLabel>
        <Textarea id="recipe-text" dir="auto" className="min-h-24" maxLength={maxRecipe} value={draft.text}
          onChange={(e) => setDraft({ ...draft, text: e.target.value })} />
        <FieldDescription>
          {draft.text.length} of {maxRecipe.toLocaleString()} characters.
        </FieldDescription>
      </Field>
      <div className="flex gap-2">
        <Button size="sm" disabled={busy || !draft.app.trim() || !draft.name.trim() || !draft.text.trim()} onClick={() => void save()}>
          Save
        </Button>
        <Button size="sm" variant="ghost" disabled={busy} onClick={() => setDraft(null)}>
          Cancel
        </Button>
      </div>
    </FieldGroup>
  )
}
