import * as React from "react"
import {
  BrainIcon,
  CheckIcon,
  ChevronDownIcon,
  ChevronRightIcon,
  CopyIcon,
  DiamondIcon,
  EllipsisIcon,
  NotebookTextIcon,
  PanelLeftIcon,
  PanelRightCloseIcon,
  PlusIcon,
  RefreshCwIcon,
  SendIcon,
  ShieldCheckIcon,
  SparklesIcon,
  SquareIcon,
  Undo2Icon,
  XIcon,
} from "lucide-react"

import { Bubble, BubbleContent } from "@/components/ui/bubble"
import { Button } from "@/components/ui/button"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Badge } from "@/components/ui/badge"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupTextarea,
} from "@/components/ui/input-group"
import { ButtonGroup } from "@/components/ui/button-group"
import { Message, MessageContent, MessageFooter } from "@/components/ui/message"
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
} from "@/components/ui/message-scroller"
import { Spinner } from "@/components/ui/spinner"
import { Marker, MarkerContent } from "@/components/ui/marker"
import { SidebarTrigger } from "@/components/ui/sidebar"
import { cn } from "@/lib/utils"
import { Markdown } from "@/jenab/markdown"

export function ChatHeader({
  title,
  sub,
  mother,
  compact,
}: {
  title: string
  sub: string
  mother?: boolean
  compact?: boolean
}) {
  return (
    <header className="flex h-14 shrink-0 items-center gap-2 border-b px-3">
      {compact && <SidebarTrigger />}
      <div className="flex min-w-0 items-center gap-2">
        {mother && <DiamondIcon className="size-4 shrink-0 fill-current" />}
        <div className="grid min-w-0 leading-tight">
          <span className="truncate font-medium">{title}</span>
          <span className="truncate text-xs text-muted-foreground">{sub}</span>
        </div>
      </div>
      <div className="ms-auto flex items-center gap-1">
        {!compact && (
          <>
            <Button variant="ghost" size="sm">
              <RefreshCwIcon data-icon="inline-start" />
              Refresh memory
            </Button>
            <Button variant="ghost" size="sm">
              <NotebookTextIcon data-icon="inline-start" />
              Session notes
            </Button>
          </>
        )}
        <Button variant="ghost" size="icon-sm" aria-label="More">
          <EllipsisIcon />
        </Button>
      </div>
    </header>
  )
}

export function DockHeader({ title, mother }: { title: string; mother?: boolean }) {
  return (
    <header className="flex h-12 shrink-0 items-center gap-2 border-b px-3">
      {mother && <DiamondIcon className="size-4 shrink-0 fill-current" />}
      <span className="truncate font-medium">{title}</span>
      <div className="ms-auto flex items-center gap-1">
        <Button variant="ghost" size="sm">
          <PanelLeftIcon data-icon="inline-start" />
          Dock left
        </Button>
        <Button variant="ghost" size="sm">
          <PanelRightCloseIcon data-icon="inline-start" />
          Hide
        </Button>
      </div>
    </header>
  )
}

export function Notice({ children }: { children: React.ReactNode }) {
  return (
    <Marker variant="separator">
      <MarkerContent>{children}</MarkerContent>
    </Marker>
  )
}

export function Thread({
  children,
  dense,
  fromTop,
}: {
  children: React.ReactNode
  dense?: boolean
  fromTop?: boolean
}) {
  return (
    <MessageScrollerProvider autoScroll={!fromTop} defaultScrollPosition={fromTop ? "start" : "end"}>
      <MessageScroller className="flex-1">
        <MessageScrollerViewport>
          <MessageScrollerContent
            className={cn("mx-auto w-full max-w-3xl gap-5 px-6 py-6", dense && "gap-4 px-4 py-4")}
          >
            {React.Children.map(children, (child, i) =>
              child ? (
                // content-visibility clips painting at the item's edge, so pad it
                // enough for focus rings (3 px).
                <MessageScrollerItem key={i} className="-m-1 p-1">
                  {child}
                </MessageScrollerItem>
              ) : null
            )}
          </MessageScrollerContent>
        </MessageScrollerViewport>
        <MessageScrollerButton />
      </MessageScroller>
    </MessageScrollerProvider>
  )
}

export function UserMessage({ children }: { children: string }) {
  return (
    <Message align="end">
      <MessageContent>
        <Bubble variant="secondary" align="end">
          <BubbleContent dir="auto" className="text-start">
            {children}
          </BubbleContent>
        </Bubble>
      </MessageContent>
    </Message>
  )
}

export function AgentMessage({
  children,
  footer,
}: {
  children: React.ReactNode
  footer?: React.ReactNode
}) {
  return (
    <Message align="start">
      <MessageContent className="gap-2">
        {children}
        {footer && <MessageFooter className="px-0">{footer}</MessageFooter>}
      </MessageContent>
    </Message>
  )
}

export function AgentText({ children }: { children: string }) {
  return (
    <Bubble variant="ghost">
      <BubbleContent>
        <Markdown>{children}</Markdown>
      </BubbleContent>
    </Bubble>
  )
}

export function ToolChip({
  name,
  detail,
  state = "done",
  agent,
  defaultOpen,
  children,
}: {
  name: string
  detail: string
  state?: "done" | "running" | "failed"
  agent?: boolean
  defaultOpen?: boolean
  children?: React.ReactNode
}) {
  return (
    <Collapsible defaultOpen={defaultOpen} className="flex flex-col gap-1.5 not-typeset">
      <CollapsibleTrigger asChild>
        <Button
          variant="outline"
          size="xs"
          className="w-fit max-w-full justify-start font-normal text-muted-foreground"
        >
          {state === "done" && <CheckIcon data-icon="inline-start" className="text-tone-green" />}
          {state === "running" && <Spinner data-icon="inline-start" />}
          {state === "failed" && <XIcon data-icon="inline-start" className="text-tone-red" />}
          <span className={cn("text-foreground", agent ? "font-medium" : "font-mono")}>{name}</span>
          <span className={cn("truncate", state === "running" && "shimmer")}>{detail}</span>
          <ChevronRightIcon
            data-icon="inline-end"
            className="transition-transform group-data-[state=open]/button:rotate-90"
          />
        </Button>
      </CollapsibleTrigger>
      {children && (
        <CollapsibleContent>
          <div className="ms-3 flex flex-col gap-1.5 rounded-lg border bg-muted/40 p-2.5 font-mono text-xs">
            {children}
          </div>
        </CollapsibleContent>
      )}
    </Collapsible>
  )
}

export function ToolDetail({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex gap-3">
      <span className="w-14 shrink-0 text-muted-foreground">{label}</span>
      <span className="min-w-0 break-words">{children}</span>
    </div>
  )
}

export function MemoryChip({ children }: { children: React.ReactNode }) {
  return (
    <ButtonGroup className="not-typeset">
      <Button variant="outline" size="xs" className="font-normal text-muted-foreground">
        <BrainIcon data-icon="inline-start" />
        {children}
      </Button>
      <Button variant="outline" size="xs">
        Undo
      </Button>
      <Button variant="outline" size="xs">
        View changes
      </Button>
    </ButtonGroup>
  )
}

export function TurnFooter({ time }: { time: string }) {
  return (
    <div className="flex items-center gap-1 text-xs text-muted-foreground">
      <Button variant="ghost" size="xs" className="text-muted-foreground">
        <Undo2Icon data-icon="inline-start" />
        Undo turn
      </Button>
      <Button variant="ghost" size="xs" className="text-muted-foreground">
        <CopyIcon data-icon="inline-start" />
        Copy
      </Button>
      <span className="ps-1">{time}</span>
    </div>
  )
}

export function Composer({
  running,
  model = "Gemma 4 31B",
  approvals = "Standard",
  value,
  placeholder,
  mother,
  compact,
  className,
}: {
  running?: boolean
  mother?: boolean
  compact?: boolean
  model?: string
  approvals?: string
  value?: string
  placeholder?: string
  className?: string
}) {
  return (
    <div className={cn("mx-auto w-full max-w-3xl px-6 pb-4", className)}>
      <InputGroup
        className={cn(mother && "mother-ring")}
        data-working={mother && running}
      >
        <InputGroupTextarea
          dir="auto"
          rows={compact ? 1 : 2}
          defaultValue={value}
          placeholder={
            placeholder ??
            (running ? "Write anytime. Your message joins the running turn." : "Message Mother…")
          }
          className={compact ? "min-h-9" : "min-h-16"}
        />
        <InputGroupAddon align="block-end">
          <InputGroupButton size="icon-xs" variant="ghost" aria-label="Attach">
            <PlusIcon />
          </InputGroupButton>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <InputGroupButton variant="ghost">
                <SparklesIcon />
                {model}
                <ChevronDownIcon />
              </InputGroupButton>
            </DropdownMenuTrigger>
            <ModelMenu current={model} />
          </DropdownMenu>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <InputGroupButton variant="ghost">
                <ShieldCheckIcon />
                {approvals}
                <ChevronDownIcon />
              </InputGroupButton>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <DropdownMenuGroup>
                <DropdownMenuItem>Strict</DropdownMenuItem>
                <DropdownMenuItem>Standard</DropdownMenuItem>
                <DropdownMenuItem>Auto</DropdownMenuItem>
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
          {running ? (
            <InputGroupButton variant="default" size="sm" className="ms-auto">
              <SquareIcon className="fill-current" />
              Stop
            </InputGroupButton>
          ) : (
            <InputGroupButton variant="default" size="icon-sm" className="ms-auto" aria-label="Send">
              <SendIcon />
            </InputGroupButton>
          )}
        </InputGroupAddon>
      </InputGroup>
    </div>
  )
}

export function StreamingText({ children }: { children: string }) {
  return (
    <div dir="auto" className="typeset typeset-chat">
      <p>
        {children}
        <span className="ms-0.5 inline-block h-4 w-1.5 translate-y-0.5 animate-pulse rounded-sm bg-foreground/70" />
      </p>
    </div>
  )
}

// Models that are on, grouped by provider (SPEC 3.9). Aliases are marked.
const modelGroups = [
  {
    provider: "Google",
    models: [
      { name: "Gemma 4 31B", alias: "default" },
      { name: "Gemini 2.5 Flash" },
    ],
  },
  { provider: "Z.ai", models: [{ name: "GLM-4.7 Flash", alias: "fast" }] },
  { provider: "Ollama (this computer)", models: [{ name: "gemma4:31b" }] },
]

export function ModelMenu({ current }: { current: string }) {
  return (
    <DropdownMenuContent align="start" className="w-64">
      {modelGroups.map((g) => (
        <DropdownMenuGroup key={g.provider}>
          <DropdownMenuLabel>{g.provider}</DropdownMenuLabel>
          {g.models.map((m) => (
            <DropdownMenuItem key={m.name}>
              {m.name}
              {m.alias && <Badge variant="secondary">{m.alias}</Badge>}
              {m.name === current && <CheckIcon className="ms-auto" />}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      ))}
      <DropdownMenuSeparator />
      <DropdownMenuGroup>
        <DropdownMenuItem>
          OpenAI
          <span className="ms-auto text-xs text-muted-foreground">Add key</span>
        </DropdownMenuItem>
        <DropdownMenuItem>
          Anthropic
          <span className="ms-auto text-xs text-muted-foreground">Add key</span>
        </DropdownMenuItem>
      </DropdownMenuGroup>
    </DropdownMenuContent>
  )
}
