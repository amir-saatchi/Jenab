import { CheckIcon, KeyRoundIcon, PlusIcon, ServerIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemMedia,
  ItemTitle,
} from "@/components/ui/item"
import { Label } from "@/components/ui/label"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { SettingsDivider, SettingsLayout, SettingsSection, SettingsTitle } from "@/jenab/settings"
import { tray } from "@/data/bitcoin"

const pages = ["General", "Appearance", "Models", "Usage", "Search", "Connections", "MCP servers", "Updates", "Developer"]

const providers: {
  name: string
  detail: string
  keyed: boolean
  local?: boolean
}[] = [
  { name: "Google", detail: "Key in the keychain · 2 models on", keyed: true },
  { name: "Z.ai", detail: "Key in the keychain · 1 model on", keyed: true },
  { name: "Ollama (local)", detail: "http://localhost:11434 · 1 model · no key", keyed: true, local: true },
  { name: "OpenAI", detail: "No key", keyed: false },
  { name: "Anthropic", detail: "No key", keyed: false },
]

const limits: [string, string, string][] = [
  ["Parallel background LLM calls", "8", "Chats count but never wait"],
  ["Parallel pipeline runs", "4", ""],
  ["Background tasks per chat", "3", ""],
  ["Subagents at once per chat", "5", ""],
  ["Requests per turn", "25", ""],
  ["Requests per app-started turn", "8", ""],
  ["Tokens per app-started turn", "20,000", ""],
]

function Choice({ id, label, value }: { id: string; label: string; value: string }) {
  return (
    <Field orientation="horizontal">
      <RadioGroupItem id={id} value={value} />
      <FieldLabel htmlFor={id} className="font-normal whitespace-nowrap">
        {label}
      </FieldLabel>
    </Field>
  )
}

// Other settings pages, shown as small excerpts beside the Models page.
function Excerpts() {
  return (
    <aside className="flex w-[21rem] shrink-0 flex-col gap-3 overflow-hidden border-s bg-muted/40 p-4">
      <span className="text-xs font-medium text-muted-foreground">Other pages (excerpts)</span>
      <Card size="sm">
        <CardHeader>
          <CardTitle>Appearance · Theme</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          <RadioGroup defaultValue="system" className="flex gap-3">
            <Choice id="t-light" value="light" label="Light" />
            <Choice id="t-dark" value="dark" label="Dark" />
            <Choice id="t-system" value="system" label="System" />
          </RadioGroup>
          <p className="text-xs text-muted-foreground">Applies at once, no reload.</p>
        </CardContent>
      </Card>
      <Card size="sm">
        <CardHeader>
          <CardTitle>Updates</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          <RadioGroup defaultValue="auto" className="flex gap-3">
            <Choice id="u-auto" value="auto" label="Automatic" />
            <Choice id="u-notify" value="notify" label="Notify only" />
            <Choice id="u-off" value="off" label="Off" />
          </RadioGroup>
          <p className="text-xs text-muted-foreground">Version 1.0.0 · up to date</p>
        </CardContent>
      </Card>
      <Card size="sm">
        <CardHeader>
          <CardTitle>Search</CardTitle>
        </CardHeader>
        <CardContent>
          <FieldGroup className="gap-3">
            <Field orientation="horizontal">
              <FieldLabel className="w-16">Provider</FieldLabel>
              <Select defaultValue="Tavily">
                <SelectTrigger size="sm" className="flex-1">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="Tavily">Tavily</SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            <Field orientation="horizontal">
              <FieldLabel className="w-16">Key</FieldLabel>
              <InputGroup className="h-8 flex-1">
                <InputGroupInput type="password" value="••••••••" readOnly />
                <InputGroupAddon align="inline-end">
                  <CheckIcon className="text-tone-green" />
                </InputGroupAddon>
              </InputGroup>
            </Field>
            <FieldDescription className="text-xs">
              Feeds, Wikipedia and Hacker News need no key.
            </FieldDescription>
          </FieldGroup>
        </CardContent>
      </Card>
      <Card size="sm">
        <CardHeader>
          <CardTitle>Tray menu</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col rounded-lg border bg-popover p-1 text-sm shadow-sm">
            <span className="rounded-md px-2 py-1.5">Open Jenab</span>
            <span className="px-2 py-1.5 text-xs text-muted-foreground">{tray}</span>
            <Separator className="my-1" />
            <span className="rounded-md px-2 py-1.5">Pause schedules</span>
            <span className="rounded-md px-2 py-1.5">Quit</span>
          </div>
        </CardContent>
      </Card>
    </aside>
  )
}

// 07 · Settings as a full view, Models page (SPEC 3.9, 7.6).
export function Screen07() {
  return (
    <SettingsLayout items={pages} active="Models" aside={<Excerpts />}>
      <SettingsTitle
        title="Models"
        description="Jenab works the same with every model. Keys are stored in the OS keychain."
      />
      <SettingsSection title="Providers">
        <ItemGroup className="gap-2">
          {providers.map((p) => (
            <Item key={p.name} variant="outline" size="sm">
              <ItemMedia variant="icon">{p.local ? <ServerIcon /> : <KeyRoundIcon />}</ItemMedia>
              <ItemContent>
                <ItemTitle>
                  {p.name}
                  {p.keyed && !p.local && <CheckIcon className="size-3.5 text-tone-green" />}
                </ItemTitle>
                <ItemDescription className={p.local ? "font-mono text-xs" : undefined}>{p.detail}</ItemDescription>
              </ItemContent>
              <ItemActions>
                {p.keyed ? (
                  <>
                    {p.local ? (
                      <Button variant="ghost" size="sm">
                        Edit
                      </Button>
                    ) : (
                      <Button variant="ghost" size="sm">
                        Models
                      </Button>
                    )}
                    <Button variant="ghost" size="sm">
                      Test
                    </Button>
                    <Button variant="ghost" size="sm" className="text-muted-foreground">
                      Remove
                    </Button>
                  </>
                ) : (
                  <Button variant="outline" size="sm">
                    Add key
                  </Button>
                )}
              </ItemActions>
            </Item>
          ))}
        </ItemGroup>
        <div className="flex items-center gap-3">
          <Button variant="outline" size="sm">
            <PlusIcon data-icon="inline-start" />
            Add provider
          </Button>
          <span className="text-xs text-muted-foreground">Any OpenAI-compatible URL</span>
        </div>
      </SettingsSection>
      <SettingsDivider />
      <SettingsSection
        title="Default and fast model"
        description="Model list 2026.09, built in and updated with the app."
      >
        <FieldGroup className="grid grid-cols-2 gap-4">
          <Field>
            <FieldLabel>Default model</FieldLabel>
            <Select defaultValue="Gemma 4 31B · Google">
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="Gemma 4 31B · Google">Gemma 4 31B · Google</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
            <FieldDescription>New chats start with it.</FieldDescription>
          </Field>
          <Field>
            <FieldLabel>Fast model</FieldLabel>
            <Select defaultValue="GLM-4.7 Flash · Z.ai">
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value="GLM-4.7 Flash · Z.ai">GLM-4.7 Flash · Z.ai</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
            <FieldDescription>For titles, summaries and llm.select.</FieldDescription>
          </Field>
        </FieldGroup>
      </SettingsSection>
      <SettingsDivider />
      <SettingsSection title="Limits" description="Starting values, tuned with the benchmark. Changes apply at once.">
        <FieldGroup className="grid max-w-md gap-y-2">
          {limits.map(([label, value]) => (
            <Field key={label} orientation="horizontal" className="justify-between">
              <Label className="font-normal">{label}</Label>
              <Input className="w-24 text-end tabular-nums" defaultValue={value} />
            </Field>
          ))}
        </FieldGroup>
        <p className="text-xs text-muted-foreground">
          <Badge variant="outline" className="me-1.5">
            config.yaml
          </Badge>
          Saved in the user config file.
        </p>
      </SettingsSection>
    </SettingsLayout>
  )
}

