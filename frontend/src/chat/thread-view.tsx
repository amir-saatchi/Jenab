import * as React from "react"

import { Bubble, BubbleContent } from "@/components/ui/bubble"
import { Button } from "@/components/ui/button"
import { Message as MessageRow, MessageContent, MessageFooter } from "@/components/ui/message"
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
} from "@/components/ui/message-scroller"
import { Spinner } from "@/components/ui/spinner"
import { ChatService, ChatState, NoticeKind, PartKind, Role, type Message, type Part, type ToolResult } from "@/lib/api"
import { showError } from "@/lib/errors"
import { ApprovalCard, FailedCard, QuestionForm } from "@/chat/cards"
import { modelName, starters } from "@/chat/composer"
import { AgentText, CopyButton, ImagePart, NoticeLine, SkillChip, Thinking, ToolChip } from "@/chat/parts"
import { ProviderCard } from "@/chat/provider-card"
import { copyText, firstRows, lastNotice, results, rows, type Row } from "@/chat/rows"
import { useSettings } from "@/state/settings"
import { useStream } from "@/state/stream"
import { useThreads, type Thread } from "@/state/thread"

function clock(at: string) {
  return new Date(at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
}

export function ThreadView({ thread, mother }: { thread: Thread; mother: boolean }) {
  const all = React.useMemo(() => rows(thread.messages), [thread.messages])
  // The last rows render at once when a chat opens; the rest follow in a
  // transition, so a long chat doesn't block input (N-02).
  const [first] = React.useState(() => firstRows(all))
  const [full, setFull] = React.useState(first === all.length)
  React.useEffect(() => {
    if (!full) {
      const t = setTimeout(() => React.startTransition(() => setFull(true)), 0)
      return () => clearTimeout(t)
    }
  }, [full])
  const running = thread.state !== ChatState.StateIdle
  const notice = lastNotice(thread.messages)
  const streaming = thread.streaming !== "" && !thread.messages.some((m) => m.id === thread.streaming)
  // Messages written during the answer that is streaming are stored after
  // it, so they show below it.
  let shown = full ? all : all.slice(-first)
  let joined: Row[] = []
  if (streaming) {
    let i = shown.length
    while (i > 0 && shown[i - 1].kind === "user" && thread.joining.includes(shown[i - 1].id)) i--
    joined = shown.slice(i)
    shown = shown.slice(0, i)
  }
  const lastRow = shown[shown.length - 1]
  const row = (r: Row) => (
    // content-visibility clips painting at the item's edge, so pad it
    // enough for focus rings.
    <MessageScrollerItem key={r.id} messageId={r.id} className="-m-1 p-1">
      <RowView
        row={r}
        thread={thread}
        streaming={streaming && r === lastRow && r.kind === "agent"}
        isLast={r === lastRow && joined.length === 0}
        failed={notice?.kind === NoticeKind.NoticeTurnFailed && !running && r === lastRow}
      />
    </MessageScrollerItem>
  )

  return (
    <MessageScrollerProvider autoScroll defaultScrollPosition="end">
      <MessageScroller className="flex-1">
        <MessageScrollerViewport preserveScrollOnPrepend>
          <MessageScrollerContent className="mx-auto w-full max-w-3xl gap-5 px-6 py-6">
            {full && thread.from > 1 && (
              <MessageScrollerItem>
                <Older chat={thread.chat.id} more={() => useThreads.getState().older(thread.chat.id)} />
              </MessageScrollerItem>
            )}
            {all.length === 0 && !streaming && <Empty thread={thread} mother={mother} />}
            {shown.map(row)}
            {streaming && lastRow?.kind !== "agent" && (
              <MessageScrollerItem key={thread.streaming} className="-m-1 p-1">
                <MessageRow align="start">
                  <MessageContent className="gap-2">
                    <StreamParts chat={thread.chat.id} message={thread.streaming} />
                  </MessageContent>
                </MessageRow>
              </MessageScrollerItem>
            )}
            {joined.map(row)}
            {running && !streaming && lastRow?.kind !== "agent" && !thread.waiting && (
              <MessageScrollerItem className="-m-1 p-1">
                <Spinner className="text-muted-foreground" aria-label="Working" />
              </MessageScrollerItem>
            )}
          </MessageScrollerContent>
        </MessageScrollerViewport>
        <MessageScrollerButton />
      </MessageScroller>
    </MessageScrollerProvider>
  )
}

// Older loads earlier turns when it scrolls into view.
function Older({ chat, more }: { chat: string; more: () => unknown }) {
  const ref = React.useRef<HTMLButtonElement>(null)
  const [busy, setBusy] = React.useState(false)
  const load = React.useCallback(() => {
    setBusy(true)
    Promise.resolve(more())
      .catch(showError)
      .finally(() => setBusy(false))
  }, [more])
  React.useEffect(() => {
    const el = ref.current
    if (!el) return
    const io = new IntersectionObserver(([e]) => e.isIntersecting && load())
    io.observe(el)
    return () => io.disconnect()
  }, [chat, load])
  return (
    <div className="flex justify-center">
      <Button ref={ref} variant="ghost" size="xs" className="text-muted-foreground" disabled={busy} onClick={load}>
        {busy && <Spinner data-icon="inline-start" />}
        Earlier messages
      </Button>
    </div>
  )
}

const hello = (title: string) =>
  `Hi, I'm ${title}, the home of this project. Tell me what to track, and I'll set up the tables, a pipeline and a page.`

function Empty({ thread, mother }: { thread: Thread; mother: boolean }) {
  if (!mother) return <p className="m-auto text-sm text-muted-foreground">Write the first message below.</p>
  const send = (text: string) => ChatService.Send(thread.project, thread.chat.id, text).catch(showError)
  return (
    <MessageRow align="start">
      <MessageContent className="gap-2">
        <AgentText text={hello(thread.chat.title || "Mother")} />
        <div className="flex flex-wrap gap-2">
          {starters.map((s) => (
            <Button key={s} variant="outline" size="sm" onClick={() => void send(s)}>
              {s}
            </Button>
          ))}
        </div>
      </MessageContent>
    </MessageRow>
  )
}

interface RowProps {
  row: Row
  thread: Thread
  streaming: boolean
  isLast: boolean
  failed: boolean
}

const RowView = React.memo(
  function RowView({ row, thread, streaming, isLast, failed }: RowProps) {
    if (row.kind === "user") return <UserRow message={row.message} thread={thread} />
    if (row.kind === "notice") {
      const n = row.part.notice
      if (!n) return null
      if (failed && n.kind === NoticeKind.NoticeTurnFailed) return <Failed thread={thread} text={n.text} />
      return <NoticeLine notice={n} time={clock(row.message.created_at)} />
    }
    return <AgentRow messages={row.messages} last={row.last} thread={thread} streaming={streaming} isLast={isLast} />
  },
  (a, b) =>
    sameRow(a.row, b.row) &&
    a.streaming === b.streaming &&
    a.isLast === b.isLast &&
    a.failed === b.failed &&
    a.thread.state === b.thread.state &&
    a.thread.waiting === b.thread.waiting &&
    a.thread.joining === b.thread.joining &&
    a.thread.chat.title === b.thread.chat.title,
)

function sameRow(a: Row, b: Row) {
  if (a.kind !== b.kind || a.id !== b.id) return false
  if (a.kind === "agent" && b.kind === "agent")
    return a.last === b.last && a.messages.length === b.messages.length && a.messages.every((m, i) => m === b.messages[i])
  if (a.kind === "user" && b.kind === "user") return a.message === b.message
  return a.kind === "notice" && b.kind === "notice" && a.part === b.part
}

// Failed is a turn that stopped on a provider error. With no provider set
// up, it is the *Connect* card: the kept message is sent once connected.
function Failed({ thread, text }: { thread: Thread; text: string }) {
  const none = useSettings((s) => Object.keys(s.view?.settings.llm.providers ?? {}).length === 0)
  const retry = () => ChatService.Retry(thread.project, thread.chat.id).catch(showError)
  if (none) return <ProviderCard title={thread.chat.title || "This chat"} onConnected={() => void retry()} />
  return <FailedCard project={thread.project} chat={thread.chat.id} text={text} />
}

function UserRow({ message, thread }: { message: Message; thread: Thread }) {
  const joining = thread.joining.includes(message.id)
  return (
    <MessageRow align="end">
      <MessageContent>
        {(message.parts ?? []).map((p, i) =>
          p.image ? (
            <ImagePart key={i} project={thread.project} part={p} />
          ) : p.text ? (
            <Bubble key={i} variant={joining ? "outline" : "secondary"} align="end">
              <BubbleContent dir="auto" className={joining ? "border-dashed text-start whitespace-pre-wrap" : "text-start whitespace-pre-wrap"}>
                {p.text.text}
              </BubbleContent>
            </Bubble>
          ) : null,
        )}
        {joining && <MessageFooter>Joins at the next step</MessageFooter>}
      </MessageContent>
    </MessageRow>
  )
}

function AgentRow({
  messages,
  last,
  thread,
  streaming,
  isLast,
}: {
  messages: Message[]
  last: boolean
  thread: Thread
  streaming: boolean
  isLast: boolean
}) {
  const res = React.useMemo(() => results(messages), [messages])
  const running = thread.state !== ChatState.StateIdle && isLast
  const groups = useSettings((s) => s.models)
  const answer = messages.findLast((m) => m.role === Role.RoleAssistant)
  const footer = last && !running && answer
  return (
    <MessageRow align="start">
      <MessageContent className="gap-2">
        {messages.flatMap((m) =>
          (m.parts ?? []).map((p, i) => (
            <PartView key={`${m.id}:${i}`} part={p} message={m} index={i} thread={thread} results={res} running={running} />
          )),
        )}
        {streaming && <StreamParts chat={thread.chat.id} message={thread.streaming} />}
        {footer && (
          <MessageFooter className="gap-1 px-0">
            <CopyButton text={copyText(messages)} />
            <span className="ps-1">{clock(answer.created_at)}</span>
            {answer.model && <span>· {modelName(groups ?? [], answer.model)}</span>}
          </MessageFooter>
        )}
      </MessageContent>
    </MessageRow>
  )
}

function PartView({
  part: p,
  message,
  index,
  thread,
  results,
  running,
}: {
  part: Part
  message: Message
  index: number
  thread: Thread
  results: Map<string, ToolResult>
  running: boolean
}) {
  switch (p.kind) {
    case PartKind.PartText:
      return p.text ? <AgentText text={p.text.text} stopped={p.text.stopped} /> : null
    case PartKind.PartThinking:
      return p.thinking ? <Thinking text={p.thinking.text} /> : null
    case PartKind.PartToolCall:
      return p.tool_call ? <ToolChip call={p.tool_call} result={results.get(p.tool_call.id)} running={running} /> : null
    case PartKind.PartImage:
      return <ImagePart project={thread.project} part={p} />
    case PartKind.PartNotice:
      if (!p.notice) return null
      return p.notice.kind === NoticeKind.NoticeSkillLoaded ? <SkillChip text={p.notice.text} /> : <NoticeLine notice={p.notice} />
    case PartKind.PartApproval:
    case PartKind.PartQuestion: {
      const w = thread.waiting
      const live = (w?.message === message.id && w.index === index) || thread.state !== ChatState.StateIdle
      const props = { project: thread.project, chat: thread.chat.id, message: message.id, index, live }
      if (p.approval) return <ApprovalCard approval={p.approval} {...props} />
      if (p.question) return <QuestionForm question={p.question} {...props} />
      return null
    }
  }
  return null
}

// StreamParts is the answer still streaming. Only it re-renders as tokens
// come, once per frame.
function StreamParts({ chat, message }: { chat: string; message: string }) {
  const s = useStream(chat)
  if (!s || s.message !== message) return <Spinner className="text-muted-foreground" aria-label="Working" />
  return (
    <>
      {s.parts.map((p, i) =>
        p.text ? (
          <AgentText key={i} text={p.text.text} />
        ) : p.thinking ? (
          <Thinking key={i} text={p.thinking.text} />
        ) : p.tool_call ? (
          <ToolChip key={i} call={p.tool_call} running />
        ) : null,
      )}
      {s.kind === PartKind.PartThinking ? (
        <Thinking text={s.text} streaming />
      ) : s.text ? (
        <AgentText text={s.text} />
      ) : (
        <Spinner className="text-muted-foreground" aria-label="Working" />
      )}
    </>
  )
}
