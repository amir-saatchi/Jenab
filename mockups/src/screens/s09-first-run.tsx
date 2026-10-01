import * as React from "react"
import { CheckIcon, DiamondIcon, FolderIcon, KeyRoundIcon, PlusIcon, SettingsIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { Separator } from "@/components/ui/separator"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import {
  AgentMessage,
  AgentText,
  Composer,
  Notice,
  Thread,
  UserMessage,
} from "@/jenab/chat"

// One app window, drawn small so three steps fit side by side.
function Frame({ step, title, children }: { step: string; title: string; children: React.ReactNode }) {
  return (
    <div className="flex min-h-0 flex-col gap-2">
      <span className="text-xs font-medium text-muted-foreground">{step}</span>
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border bg-background shadow-sm">
        <div className="flex h-8 shrink-0 items-center border-b bg-muted/50 px-3 text-xs text-muted-foreground">
          {title}
        </div>
        {children}
      </div>
    </div>
  )
}

function MiniHeader() {
  return (
    <header className="flex h-12 shrink-0 items-center gap-2 border-b px-4">
      <DiamondIcon className="size-4 fill-current" />
      <div className="grid leading-tight">
        <span className="text-sm font-medium">Mother</span>
        <span className="text-xs text-muted-foreground">Project home</span>
      </div>
    </header>
  )
}

function Welcome() {
  return (
    <div className="flex flex-1 flex-col">
      <Empty className="flex-1">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <FolderIcon />
          </EmptyMedia>
          <EmptyTitle>Welcome to Jenab</EmptyTitle>
          <EmptyDescription>Each project is one folder with its data, chats and files.</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <div className="flex gap-2">
            <Button>
              <PlusIcon data-icon="inline-start" />
              Create project
            </Button>
            <Button variant="outline">Open the example</Button>
          </div>
          <p className="text-xs text-muted-foreground">
            Saved in <span className="font-mono">C:\Users\you\Jenab</span> · change in Settings
          </p>
        </EmptyContent>
      </Empty>
      <div className="px-6 pb-6">
        <Card size="sm" className="shadow-lg">
          <CardHeader>
            <CardTitle>New project</CardTitle>
          </CardHeader>
          <CardContent>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="np-name">Name</FieldLabel>
                <Input id="np-name" defaultValue="Bitcoin" dir="auto" />
              </Field>
            </FieldGroup>
          </CardContent>
          <CardFooter className="justify-end gap-2">
            <Button variant="outline" size="sm">
              Cancel
            </Button>
            <Button size="sm">Create</Button>
          </CardFooter>
        </Card>
      </div>
      <Separator />
      <div className="flex items-center gap-2 px-4 py-3 text-sm text-muted-foreground">
        <SettingsIcon className="size-4" />
        Settings
      </div>
    </div>
  )
}

function NeedsModel() {
  return (
    <Card size="sm" className="not-typeset">
      <CardHeader>
        <CardTitle>Mother needs an AI model to answer</CardTitle>
        <CardDescription>
          Add a provider with your own key. Your message is kept and sent once it is connected.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <ToggleGroup type="single" value="google" variant="outline" size="sm" className="flex-wrap">
          <ToggleGroupItem value="google">Google</ToggleGroupItem>
          <ToggleGroupItem value="openai">OpenAI</ToggleGroupItem>
          <ToggleGroupItem value="anthropic">Anthropic</ToggleGroupItem>
          <ToggleGroupItem value="ollama">Ollama</ToggleGroupItem>
          <ToggleGroupItem value="other">Other…</ToggleGroupItem>
        </ToggleGroup>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="fr-key">API key</FieldLabel>
            <InputGroup>
              <InputGroupAddon>
                <KeyRoundIcon />
              </InputGroupAddon>
              <InputGroupInput id="fr-key" type="password" defaultValue="AIza-demo-0000000000" />
            </InputGroup>
            <FieldDescription>Stored in the OS keychain · also in Settings → Models</FieldDescription>
          </Field>
        </FieldGroup>
      </CardContent>
      <CardFooter>
        <Button size="sm">Connect</Button>
      </CardFooter>
    </Card>
  )
}

function Starters() {
  return (
    <div className="flex flex-wrap gap-2">
      <Button variant="outline" size="sm">
        Track a price every day
      </Button>
      <Button variant="outline" size="sm">
        Summarise a news feed
      </Button>
    </div>
  )
}

const hello =
  "Hi, I'm Mother, the home of this project. Tell me what to track, and I'll set up the tables, a pipeline and a page."

// The model picker, drawn open (static, so it sits inside the frame).
function PickerOpen() {
  const row = "flex items-center gap-2 rounded-md px-2 py-1.5"
  return (
    <div className="mx-4 mb-2 flex flex-col rounded-lg border bg-popover p-1 text-sm shadow-md">
      <span className="px-2 py-1 text-xs font-medium text-muted-foreground">Google</span>
      <span className={`${row} bg-accent`}>
        Gemma 4 31B <Badge variant="secondary">default</Badge>
        <CheckIcon className="ms-auto size-4" />
      </span>
      <span className={row}>
        Gemini 2.5 Flash <Badge variant="secondary">fast</Badge>
      </span>
      <Separator className="my-1" />
      {[
        ["OpenAI", "Add key"],
        ["Anthropic", "Add key"],
        ["Z.ai", "Add key"],
        ["Ollama", "not running"],
      ].map(([p, s]) => (
        <span key={p} className={row}>
          {p}
          <span className="ms-auto text-xs text-muted-foreground">{s}</span>
        </span>
      ))}
    </div>
  )
}

// 09 · First run (SPEC 3.9): no wizard. Create a project, then add a provider in the chat.
export function Screen09() {
  return (
    <div className="grid h-svh grid-cols-3 gap-6 overflow-hidden bg-muted/40 px-8 py-6">
      <Frame step="1 · The app opens with no projects" title="Jenab">
        <Welcome />
      </Frame>
      <Frame step="2 · A message without a provider" title="Jenab — Bitcoin">
        <MiniHeader />
        <Thread dense fromTop>
          <AgentMessage>
            <AgentText>{hello}</AgentText>
            <Starters />
          </AgentMessage>
          <UserMessage>Track Bitcoin every day: the price and the news.</UserMessage>
          <NeedsModel />
        </Thread>
        <Composer mother compact model="No model" placeholder="Message Mother…" className="px-4 pb-4" />
      </Frame>
      <Frame step="3 · Connected: the kept message is sent" title="Jenab — Bitcoin">
        <MiniHeader />
        <Thread dense>
          <AgentMessage>
            <AgentText>{hello}</AgentText>
          </AgentMessage>
          <UserMessage>Track Bitcoin every day: the price and the news.</UserMessage>
          <Notice>Google connected · 2 models on</Notice>
          <AgentMessage>
            <AgentText>
              I'll set it up: a table for prices and news, a pipeline every day at 08:00, and a page.
            </AgentText>
          </AgentMessage>
        </Thread>
        <PickerOpen />
        <Composer mother compact running placeholder="Message Mother…" className="px-4 pb-4" />
      </Frame>
    </div>
  )
}
