import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { usePoll } from "@/hooks/use-poll"
import {
  ChatState,
  MemoryLevel,
  SettingsService,
  SystemService,
  type ChatItem,
  type MemoryReading,
  type ProviderStatus,
} from "@/lib/api"
import { cn } from "@/lib/utils"
import { useChats } from "@/state/chats"
import { openChat } from "@/shell/open"

const readMemory = () => SystemService.Memory()
const readProviders = () => SettingsService.Providers()

// BottomBar shows only what changes and what the user can act on (SPEC
// 5.12). Commands, pipelines and the context fill come in later tickets;
// the chat's model is in the composer's chip.
export function BottomBar({ project }: { project?: string }) {
  const items = useChats((s) => (project ? s.byProject[project] : undefined))
  return (
    <footer className="flex h-6 shrink-0 items-center gap-4 border-t bg-sidebar px-3 text-xs text-muted-foreground">
      {project && <Work project={project} items={items ?? []} />}
      <div className="ms-auto flex items-center gap-4">
        <Problems />
        <Memory />
      </div>
    </footer>
  )
}

export function workText(items: ChatItem[]) {
  const working = items.filter((it) => it.state === ChatState.StateWorking).length
  const waiting = items.filter((it) => it.state === ChatState.StateWaiting).length
  return [working && `${working} working`, waiting && `${waiting} waiting`].filter(Boolean).join(" · ")
}

function Work({ project, items }: { project: string; items: ChatItem[] }) {
  const text = workText(items)
  if (!text) return null
  // Clicking opens the chat that waits, else one that works.
  const first =
    items.find((it) => it.state === ChatState.StateWaiting) ?? items.find((it) => it.state === ChatState.StateWorking)
  return (
    <button type="button" className="hover:text-foreground" onClick={() => first && openChat(project, first.chat.id)}>
      {text}
    </button>
  )
}

// gb is bytes as gigabytes with one decimal.
export function gb(bytes: number) {
  return (bytes / 2 ** 30).toFixed(1)
}

const wholeGB = (bytes: number) => Math.round(bytes / 2 ** 30)

// memoryText is what the OS can still hand out. No total: on Windows it is
// RAM plus the page file, which reads as the wrong RAM size.
export function memoryText(r: MemoryReading) {
  if (r.free > 0) return `Memory ${gb(r.free)} GB free`
  return r.level === MemoryLevel.OK ? `Memory ${wholeGB(r.ram)} GB` : "Memory low"
}

// memoryTip says what the free memory is out of: the RAM, and on Windows
// the page file too.
export function memoryTip(r: MemoryReading) {
  if (r.level === MemoryLevel.Critical) return "Memory is almost full: new commands wait until some is free."
  const paged = r.total - r.ram
  if (r.free > 0 && paged >= 2 ** 29) {
    return `What the system can still hand out: ${wholeGB(r.ram)} GB RAM + ${wholeGB(paged)} GB page file`
  }
  return r.free > 0 ? `Free memory, of ${wholeGB(r.ram)} GB RAM` : `${wholeGB(r.ram)} GB RAM`
}

function Memory() {
  const r = usePoll(readMemory, 2000)
  if (!r) return null
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "tabular-nums",
            r.level === MemoryLevel.Low && "text-tone-amber",
            r.level === MemoryLevel.Critical && "font-medium text-tone-red"
          )}
        >
          {memoryText(r)}
        </span>
      </TooltipTrigger>
      <TooltipContent side="top">{memoryTip(r)}</TooltipContent>
    </Tooltip>
  )
}

export function problemText(p: ProviderStatus) {
  return `${p.name} paused, retry in ${Math.ceil(p.paused_ms / 1000)} s`
}

// Problems are provider pauses while they last (3.8).
function Problems() {
  const ps = usePoll(readProviders, 2000)
  const paused = (ps ?? []).filter((p) => p.paused_ms > 0)
  return (
    <>
      {paused.map((p) => (
        <Tooltip key={p.name}>
          <TooltipTrigger asChild>
            <span className="text-tone-amber">{problemText(p)}</span>
          </TooltipTrigger>
          {p.last_problem && <TooltipContent side="top">{p.last_problem}</TooltipContent>}
        </Tooltip>
      ))}
    </>
  )
}
