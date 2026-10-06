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
import {
  ChatService,
  ChatState,
  NoticeKind,
  PartKind,
  Role,
  type Message,
  type Part,
  type ToolResult,
  type Waiting,
} from "@/lib/api"
import { showError } from "@/lib/errors"
import { ApprovalCard, FailedCard, QuestionForm } from "@/chat/cards"
import { modelName, starters } from "@/chat/composer"
import { AgentText, CopyButton, ImagePart, NoticeLine, SkillChip, Thinking, ToolChip } from "@/chat/parts"
import { ProviderCard } from "@/chat/provider-card"
import { copyText, firstRows, lastNotice, results, rows, splitJoining, type Row } from "@/chat/rows"
import { useNav } from "@/state/nav"
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
  const failed = notice?.kind === NoticeKind.NoticeTurnFailed && !running ? notice.message.parts?.at(-1) : undefined
  const streaming = thread.streaming !== "" && !thread.messages.some((m) => m.id === thread.streaming)
  const [shown, joined] = splitJoining(full ? all : all.slice(-first), thread.joining)
  const lastRow = shown[shown.length - 1]
  const row = (r: Row) => {
    const last = r === lastRow
    // Rows get only the turn state they show, so a new try re-renders the
    // last row and those with cards, not every row.
    const cards = r.kind === "agent" && hasCards(r.messages)
    return (
      // content-visibility clips painting at the item's edge, so pad it
      // enough for focus rings.
      <MessageScrollerItem key={r.id} messageId={r.id} className="-m-1 p-1">
        <RowView
          row={r}
          thread={thread}
          stream={streaming && last && r.kind === "agent" ? thread.streaming : ""}
          running={running && last}
          busy={cards && running}
          waiting={cards ? thread.waiting : null}
          joining={r.kind === "user" && thread.joining.includes(r.id)}
          failed={r.kind === "notice" && failed !== undefined && r.part === failed}
        />
      </MessageScrollerItem>
    )
  }

  return (
    <MessageScrollerProvider autoScroll defaultScrollPosition="end">
      <Finished running={running} />
      <MessageScroller className="flex-1">
        <MessageScrollerViewport preserveScrollOnPrepend>
          <MessageScrollerContent className="mx-auto w-full max-w-3xl gap-5 px-6 py-6">
            {full && thread.from > 1 && (
              <MessageScrollerItem>
                <Older chat={thread.chat.id} />
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

// Finished tells a screen reader when an answer is done, not every token.
function Finished({ running }: { running: boolean }) {
  const [said, setSaid] = React.useState("")
  const was = React.useRef(running)
  React.useEffect(() => {
    if (was.current && !running) setSaid("Answer finished")
    else if (running) setSaid("")
    was.current = running
  }, [running])
  return (
    <div className="sr-only" aria-live="polite">
      {said}
    </div>
  )
}

// hasCards tells whether an agent item has an approval card or a question.
function hasCards(messages: Message[]) {
  return messages.some((m) => (m.parts ?? []).some((p) => p.kind === PartKind.PartApproval || p.kind === PartKind.PartQuestion))
}

// Older loads earlier turns when it scrolls into view. Its callback stays
// the same, so the observer is made once.
function Older({ chat }: { chat: string }) {
  const ref = React.useRef<HTMLButtonElement>(null)
  const [busy, setBusy] = React.useState(false)
  const load = React.useCallback(() => {
    setBusy(true)
    useThreads
      .getState()
      .older(chat)
      .catch(showError)
      .finally(() => setBusy(false))
  }, [chat])
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

// RowProps hold what a row shows of the turn's state, so a row re-renders
// only when that changes; thread gives the chat's IDs and title.
interface RowProps {
  row: Row
  thread: Thread
  stream: string // the answer streaming at the end of this agent item
  running: boolean // the turn runs, and this is its last row
  busy: boolean // the turn runs, for an item with cards
  waiting: Waiting | null // the card the turn waits for, for an item with cards
  joining: boolean // a user message that joins at the next step
  failed: boolean // the notice of the turn that failed last
}

const RowView = React.memo(
  function RowView({ row, thread, stream, running, busy, waiting, joining, failed }: RowProps) {
    if (row.kind === "user") return <UserRow message={row.message} project={thread.project} joining={joining} />
    if (row.kind === "notice") {
      const n = row.part.notice
      if (!n) return null
      if (failed && n.kind === NoticeKind.NoticeTurnFailed) return <Failed thread={thread} text={n.text} />
      return <NoticeLine notice={n} time={clock(row.message.created_at)} />
    }
    return (
      <AgentRow
        messages={row.messages}
        last={row.last}
        thread={thread}
        stream={stream}
        running={running}
        busy={busy}
        waiting={waiting}
      />
    )
  },
  (a, b) =>
    sameRow(a.row, b.row) &&
    a.stream === b.stream &&
    a.running === b.running &&
    a.busy === b.busy &&
    a.waiting === b.waiting &&
    a.joining === b.joining &&
    a.failed === b.failed &&
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

function UserRow({ message, project, joining }: { message: Message; project: string; joining: boolean }) {
  return (
    <MessageRow align="end">
      <MessageContent>
        {(message.parts ?? []).map((p, i) =>
          p.image ? (
            <ImagePart key={i} project={project} part={p} />
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
  stream,
  running,
  busy,
  waiting,
}: {
  messages: Message[]
  last: boolean
  thread: Thread
  stream: string
  running: boolean
  busy: boolean
  waiting: Waiting | null
}) {
  const res = React.useMemo(() => results(messages), [messages])
  const groups = useSettings((s) => s.models)
  const devTools = useSettings((s) => s.started?.dev_tools ?? false)
  const answer = messages.findLast((m) => m.role === Role.RoleAssistant)
  const footer = last && !running && answer
  return (
    <MessageRow align="start">
      <MessageContent className="gap-2">
        {messages.flatMap((m) =>
          (m.parts ?? []).map((p, i) => (
            <PartView
              key={`${m.id}:${i}`}
              part={p}
              message={m}
              index={i}
              thread={thread}
              results={res}
              running={running}
              busy={busy}
              waiting={waiting}
            />
          )),
        )}
        {stream && <StreamParts chat={thread.chat.id} message={stream} />}
        {footer && (
          <MessageFooter className="gap-1 px-0">
            <CopyButton text={copyText(messages)} />
            <span className="ps-1">{clock(answer.created_at)}</span>
            {answer.model && <span>· {modelName(groups ?? [], answer.model)}</span>}
            {devTools && (
              <Button
                variant="ghost"
                size="xs"
                className="ms-auto"
                onClick={() => useNav.getState().go({ view: "inspector", project: thread.project, chat: thread.chat.id, turn: answer.turn })}
              >
                Inspect
              </Button>
            )}
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
  busy,
  waiting: w,
}: {
  part: Part
  message: Message
  index: number
  thread: Thread
  results: Map<string, ToolResult>
  running: boolean
  busy: boolean
  waiting: Waiting | null
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
      const live = (w?.message === message.id && w.index === index) || busy
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
