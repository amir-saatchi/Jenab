import * as React from "react"
import { PlusIcon, XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group"
import { Item, ItemActions, ItemContent, ItemGroup, ItemTitle } from "@/components/ui/item"
import { ProjectService } from "@/lib/api"
import { showError, toUIError } from "@/lib/errors"
import { SettingsSection } from "@/shell/settings-layout"

// hostProblem catches the common slips before the call: a URL, a path or
// a port typed with the host. The Go side checks the rest (SPEC 6.7).
export function hostProblem(text: string): string | null {
  const t = text.trim()
  if (!t) return "Type a host."
  if (/\s/.test(t)) return "Add one host at a time."
  if (t.includes("://") || t.includes("/")) return "Give the host alone, without http:// or a path."
  if (/^[^:[\]]+:\d*$/.test(t) || /^\[.*\]:\d*$/.test(t)) return "Give the host without a port."
  return null
}

// withHost and withoutHost are the list after an add or a remove. The
// hosts are compared as the user typed them; the Go side stores them in
// one spelling and returns the list.
export function withHost(hosts: string[], host: string): string[] {
  const h = host.trim()
  return hosts.includes(h) ? hosts : [...hosts, h]
}

export function withoutHost(hosts: string[], host: string): string[] {
  return hosts.filter((h) => h !== host)
}

// PrivateHostsSection is the project's private-network exceptions (SPEC
// 5.12, 6.7): hosts whose requests may reach this computer or the local
// network without a card. A card's "Allow in this project" adds one; the
// next request uses the list.
export function PrivateHostsSection({ project }: { project: string }) {
  const [hosts, setHosts] = React.useState<string[] | null>(null)
  const [text, setText] = React.useState("")
  const [problem, setProblem] = React.useState<string | null>(null)
  const [busy, setBusy] = React.useState(false)

  React.useEffect(() => {
    let live = true
    ProjectService.PrivateHosts(project)
      .then((hs) => live && setHosts(hs ?? []))
      .catch(showError)
    return () => {
      live = false
    }
  }, [project])

  // Each change starts from the stored list, which a card may have
  // changed since the page opened.
  const change = async (next: (hs: string[]) => string[]) => {
    setBusy(true)
    try {
      const stored = await ProjectService.PrivateHosts(project)
      const saved = await ProjectService.SetPrivateHosts(project, next(stored ?? []))
      setHosts(saved ?? [])
      return true
    } catch (err) {
      const e = toUIError(err)
      if (e.kind === "invalid") setProblem(e.message)
      else showError(err)
      return false
    } finally {
      setBusy(false)
    }
  }

  const add = async (e: React.FormEvent) => {
    e.preventDefault()
    const p = hostProblem(text)
    setProblem(p)
    if (p) return
    if (await change((hs) => withHost(hs, text))) setText("")
  }

  return (
    <SettingsSection
      title="Private-network exceptions"
      description="Requests from this project to these hosts may reach this computer or your local network without asking. Other hosts ask in a card each time."
    >
      {hosts && hosts.length > 0 && (
        <ItemGroup>
          {hosts.map((h) => (
            <Item key={h} variant="outline" size="xs">
              <ItemContent>
                <ItemTitle className="font-mono text-xs" dir="ltr">
                  {h}
                </ItemTitle>
              </ItemContent>
              <ItemActions>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  aria-label={`Remove ${h}`}
                  disabled={busy}
                  onClick={() => void change((hs) => withoutHost(hs, h))}
                >
                  <XIcon />
                </Button>
              </ItemActions>
            </Item>
          ))}
        </ItemGroup>
      )}
      <form onSubmit={add}>
        <FieldGroup>
          <Field data-invalid={problem ? true : undefined}>
            <FieldLabel htmlFor="private-host">Add a host</FieldLabel>
            <InputGroup>
              <InputGroupInput
                id="private-host"
                dir="ltr"
                className="font-mono text-xs"
                placeholder="127.0.0.1 or nas.local"
                value={text}
                disabled={!hosts}
                aria-invalid={problem ? true : undefined}
                onChange={(e) => {
                  setText(e.target.value)
                  setProblem(null)
                }}
              />
              <InputGroupAddon align="inline-end">
                <InputGroupButton type="submit" disabled={busy || !hosts || !text.trim()}>
                  <PlusIcon data-icon="inline-start" />
                  Add
                </InputGroupButton>
              </InputGroupAddon>
            </InputGroup>
            {problem ? (
              <FieldError>{problem}</FieldError>
            ) : (
              <FieldDescription>A name or an address, without http://, a path or a port.</FieldDescription>
            )}
          </Field>
        </FieldGroup>
      </form>
    </SettingsSection>
  )
}
