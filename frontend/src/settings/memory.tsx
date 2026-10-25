import * as React from "react"
import { PlusIcon, TriangleAlertIcon } from "lucide-react"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Progress } from "@/components/ui/progress"
import { Textarea } from "@/components/ui/textarea"
import { ProjectService, SettingsService, type MemoryEdit, type MemorySaved, type MemorySection, type MemoryView } from "@/lib/api"
import { showError } from "@/lib/errors"
import { useUI } from "@/state/ui"
import { SettingsSection } from "@/shell/settings-layout"

// A section being edited: its name ("" until a new one is named), the
// text and the revision the edit started from.
interface Draft {
  section: string
  content: string
  revision: number
  isNew: boolean
}

// Focus is a section a chip's [edit] asked to open; n counts the asks, so
// the same section opens again.
interface Focus {
  section: string
  n: number
}

// MemoryEditor is a memory in its settings (SPEC 3.3): user memory in
// Settings, project memory in the project's settings. Each section is
// edited on its own; empty text deletes it. When a chat changed the section
// since it was opened, the save is refused with the section as it is now,
// to merge by hand. focus opens a section for editing, from a chat's chip.
export function MemoryEditor({
  load,
  save,
  focus,
}: {
  load: () => Promise<MemoryView>
  save: (e: MemoryEdit) => Promise<MemorySaved>
  focus?: Focus
}) {
  const id = React.useId()
  const [view, setView] = React.useState<MemoryView | null>(null)
  const [draft, setDraft] = React.useState<Draft | null>(null)
  const [conflict, setConflict] = React.useState<MemorySection | null>(null)
  const [busy, setBusy] = React.useState(false)

  React.useEffect(() => {
    let live = true
    load()
      .then((v) => {
        if (!live) return
        setView(v)
        const s = focus ? v.sections?.find((x) => x.section === focus.section) : undefined
        // A section already being edited keeps its unsaved text.
        if (s) setDraft((d) => d ?? { section: s.section, content: s.content, revision: s.revision, isNew: false })
      })
      .catch(showError)
    return () => {
      live = false
    }
  }, [load, focus])

  const sections = view?.sections ?? []
  const editing = (s: string) => draft && !draft.isNew && draft.section === s
  // A section deleted elsewhere while it was being edited: its editor
  // stays, below the others, so the user can save it again or let it go.
  const orphan = !!draft && !draft.isNew && !sections.some((s) => s.section === draft.section)
  const close = () => {
    setDraft(null)
    setConflict(null)
  }

  const submit = async (d: Draft, content: string) => {
    if (busy) return
    setBusy(true)
    try {
      const r = await save({ section: d.section, content, revision: d.revision })
      setView(r.memory)
      if (r.conflict) {
        setConflict(r.conflict)
        setDraft({ ...d, content, revision: r.conflict.revision })
      } else {
        close()
      }
    } catch (err) {
      showError(err)
    } finally {
      setBusy(false)
    }
  }

  const editor = (d: Draft) => (
    <FieldGroup className="gap-3">
      {d.isNew && (
        <Field>
          <FieldLabel htmlFor={`${id}-name`}>Section name</FieldLabel>
          <Input
            id={`${id}-name`}
            placeholder="such as preferences"
            dir="auto"
            value={d.section}
            maxLength={40}
            autoFocus
            onChange={(e) => setDraft({ ...d, section: e.target.value })}
          />
        </Field>
      )}
      {conflict && (
        <Alert>
          <TriangleAlertIcon className="text-tone-amber" />
          <AlertTitle>{conflict.content ? "This section changed since you opened it" : "This section was deleted since you opened it"}</AlertTitle>
          <AlertDescription>
            {conflict.content && (
              <p dir="auto" className="text-start whitespace-pre-wrap">
                {conflict.content}
              </p>
            )}
            <p>
              {conflict.content
                ? "Merge it into your text and save again, or use it as it is."
                : "Save to keep your text, or let it go."}
            </p>
          </AlertDescription>
        </Alert>
      )}
      <Field>
        <FieldLabel htmlFor={`${id}-text`}>Text</FieldLabel>
        <Textarea
          id={`${id}-text`}
          dir="auto"
          className="min-h-24"
          value={d.content}
          autoFocus={!d.isNew}
          onChange={(e) => setDraft({ ...d, content: e.target.value })}
        />
      </Field>
      <div className="flex flex-wrap gap-2">
        <Button size="sm" disabled={busy || !d.section.trim()} onClick={() => void submit(d, d.content)}>
          Save
        </Button>
        {conflict && (
          <Button
            size="sm"
            variant="outline"
            disabled={busy}
            onClick={() => {
              setConflict(null)
              if (!conflict.content) close()
              else setDraft({ ...d, content: conflict.content })
            }}
          >
            {conflict.content ? "Use theirs" : "Let it go"}
          </Button>
        )}
        <Button size="sm" variant="ghost" disabled={busy} onClick={close}>
          Cancel
        </Button>
        {!d.isNew && !orphan && (
          <Button size="sm" variant="ghost" className="ms-auto text-destructive" disabled={busy} onClick={() => void submit(d, "")}>
            Delete section
          </Button>
        )}
      </div>
    </FieldGroup>
  )

  return (
    <div className="flex flex-col gap-3">
      {view && (
        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          <Progress value={Math.min(100, (view.tokens / view.cap) * 100)} className="max-w-40" aria-label="Memory used" />
          <span>
            About {view.tokens.toLocaleString()} of {view.cap.toLocaleString()} tokens
          </span>
        </div>
      )}
      {view && sections.length === 0 && !(draft && (draft.isNew || orphan)) && (
        <p className="text-sm text-muted-foreground">Nothing yet. Chats save here what you ask them to remember.</p>
      )}
      {sections.map((s) => (
        <div key={s.section} className="flex flex-col gap-2 rounded-lg border p-3">
          <div className="flex items-center gap-2 text-sm">
            <span className="font-medium" dir="auto">
              {s.section}
            </span>
            <span className="text-xs text-muted-foreground">revision {s.revision}</span>
            {!editing(s.section) && (
              <Button
                size="xs"
                variant="ghost"
                className="ms-auto"
                disabled={!!draft}
                onClick={() => setDraft({ section: s.section, content: s.content, revision: s.revision, isNew: false })}
              >
                Edit
              </Button>
            )}
          </div>
          {editing(s.section) && draft ? (
            editor(draft)
          ) : (
            <p dir="auto" className="text-start text-sm whitespace-pre-wrap">
              {s.content}
            </p>
          )}
        </div>
      ))}
      {draft && (draft.isNew || orphan) ? (
        <div className="flex flex-col gap-2 rounded-lg border p-3">
          {orphan && (
            <span className="text-sm font-medium" dir="auto">
              {draft.section}
            </span>
          )}
          {editor(draft)}
        </div>
      ) : (
        <Button
          size="sm"
          variant="outline"
          className="w-fit"
          disabled={!view || !!draft}
          onClick={() => setDraft({ section: "", content: "", revision: 0, isNew: true })}
        >
          <PlusIcon data-icon="inline-start" />
          Add section
        </Button>
      )}
    </div>
  )
}

// useFocus is the section a chip's [edit] asked to open in this memory,
// for project memory only the project's own. It follows the asks while the
// page is open, and clears each one it takes, so a later visit opens no
// section.
function useFocus(scope: "user" | "project", project?: string): Focus | undefined {
  const want = useUI((s) => s.memoryFocus)
  const [focus, setFocus] = React.useState<Focus>()
  React.useEffect(() => {
    const prefix = `memory:${scope}/`
    if (!want || !want.target.startsWith(prefix) || (scope === "project" && want.project !== project)) return
    setFocus((f) => ({ section: want.target.slice(prefix.length), n: (f?.n ?? 0) + 1 }))
    useUI.getState().set({ memoryFocus: null })
  }, [want, scope, project])
  return focus
}

// UserMemorySection is *Settings → Memory*: user memory, for all projects.
export function UserMemorySection() {
  const focus = useFocus("user")
  const load = React.useCallback(() => SettingsService.UserMemory(), [])
  const save = React.useCallback((e: MemoryEdit) => SettingsService.SaveUserMemory(e), [])
  return (
    <SettingsSection
      title="Memory"
      description="What every chat in every project knows about you: your goals, preferences and decisions. Keep values that change, such as prices, in tables."
    >
      <MemoryEditor load={load} save={save} focus={focus} />
    </SettingsSection>
  )
}

// ProjectMemorySection is the project's memory in its settings.
export function ProjectMemorySection({ project }: { project: string }) {
  const focus = useFocus("project", project)
  const load = React.useCallback(() => ProjectService.Memory(project), [project])
  const save = React.useCallback((e: MemoryEdit) => ProjectService.SaveMemory(project, e), [project])
  return (
    <SettingsSection
      title="Memory"
      description="What every chat in this project knows: its goals, decisions and conventions. Keep values that change, such as prices, in tables."
    >
      <MemoryEditor load={load} save={save} focus={focus} />
    </SettingsSection>
  )
}
