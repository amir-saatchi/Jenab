import * as React from "react"
import { BellIcon, CheckIcon, ChevronRightIcon, CircleSlashIcon, RefreshCwIcon, XIcon } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { Input } from "@/components/ui/input"
import { Item, ItemActions, ItemContent, ItemMedia, ItemTitle } from "@/components/ui/item"
import {
  Questionnaire,
  QuestionnaireActions,
  QuestionnaireChoice,
  QuestionnaireChoiceDescription,
  QuestionnaireChoices,
  QuestionnaireInput,
  QuestionnaireItem,
  QuestionnaireNext,
  QuestionnairePrevious,
  QuestionnaireProgress,
  QuestionnaireSubmit,
  QuestionnaireTitle,
} from "@/components/ui/questionnaire"
import { ChatService, Grant, type Approval, type Question, type Retry } from "@/lib/api"
import { showError } from "@/lib/errors"
import { textDir } from "@/lib/dir"
import { cn } from "@/lib/utils"
import { noticeText } from "@/chat/parts"

// Cards waiting for the user, by "message:index", so the waiting bar can
// tell whether one is on screen and scroll to it.
const cards = new Map<string, HTMLElement>()
const cardListeners = new Set<() => void>()

export function cardKey(message: string, index: number) {
  return `${message}:${index}`
}

function useCardRef(key: string) {
  return React.useCallback(
    (el: HTMLElement | null) => {
      if (el) cards.set(key, el)
      else cards.delete(key)
      cardListeners.forEach((f) => f())
    },
    [key],
  )
}

function Kicker({ children }: { children: React.ReactNode }) {
  return <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{children}</span>
}

const kinds: Record<string, string> = {
  host: "New host",
  migration: "Schema change",
  starlark: "Script",
  connection: "Connection",
  mcp: "MCP server",
  command: "Command",
}

function time(at?: string | null) {
  return at ? new Date(at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : ""
}

interface CardProps {
  project: string
  chat: string
  message: string
  index: number
  // live: the turn still waits for it; else it shows how it ended.
  live: boolean
}

// ApprovalCard is an approval (SPEC 8.8): what, why, the risk, the details
// and the choice. Deny takes an optional note for the agent.
export function ApprovalCard({ approval: a, ...p }: CardProps & { approval: Approval }) {
  const ref = useCardRef(cardKey(p.message, p.index))
  const [note, setNote] = React.useState("")
  const [busy, setBusy] = React.useState(false)
  if (a.answer || a.stopped || !p.live) return <ApprovalRecord approval={a} />
  const answer = (grant: Grant) => {
    setBusy(true)
    ChatService.Answer(p.project, p.chat, { message: p.message, index: p.index, grant, note: note.trim() || undefined })
      .catch(showError)
      .finally(() => setBusy(false))
  }
  const deny = a.options?.some((o) => o.grant === Grant.GrantDeny)
  return (
    <Card ref={ref} size="sm" className="not-typeset">
      <CardHeader>
        <Kicker>Approval · {kinds[a.kind] ?? a.kind}</Kicker>
        <CardTitle className="text-base" dir="auto">
          {a.ask}
        </CardTitle>
        <CardDescription className="flex flex-col gap-1">
          {a.why && (
            <span className="text-foreground" dir="auto">
              {a.why}
            </span>
          )}
          {a.risk && <span dir="auto">{a.risk}</span>}
        </CardDescription>
      </CardHeader>
      {a.details && (
        <CardContent>
          <Collapsible className="flex flex-col gap-2">
            <CollapsibleTrigger asChild>
              <Button variant="ghost" size="xs" className="w-fit text-muted-foreground">
                <ChevronRightIcon data-icon="inline-start" className="transition-transform group-data-[state=open]/button:rotate-90" />
                Details
              </Button>
            </CollapsibleTrigger>
            <CollapsibleContent>
              <p dir="auto" className="rounded-lg bg-muted/60 p-3 text-start text-sm whitespace-pre-wrap">
                {a.details}
              </p>
            </CollapsibleContent>
          </Collapsible>
        </CardContent>
      )}
      <CardFooter className="flex-wrap gap-2">
        {(a.options ?? []).map((o, i) => (
          <Button
            key={o.grant}
            size="sm"
            variant={o.grant === Grant.GrantDeny || i > 0 ? "outline" : "default"}
            disabled={busy}
            onClick={() => answer(o.grant)}
          >
            {o.label}
          </Button>
        ))}
        {deny && (
          <Input
            className="h-7 min-w-40 flex-1 text-sm"
            dir="auto"
            aria-label="Note for the agent"
            placeholder="Note for the agent (optional)"
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        )}
      </CardFooter>
    </Card>
  )
}

// ApprovalRecord is an answered or stopped card, in one line: what was
// asked and how it ended, e.g. "Approved 10:14".
function ApprovalRecord({ approval: a }: { approval: Approval }) {
  const at = time(a.answered_at)
  let result: string
  let icon = <CheckIcon className="size-4 text-tone-green" />
  if (a.answer === Grant.GrantDeny) {
    result = a.note ? `Denied ${at} · “${a.note}”` : `Denied ${at}`
    icon = <XIcon className="size-4 text-tone-red" />
  } else if (a.answer) {
    result = a.by === "auto" ? `Approved by the Auto level ${at}` : `Approved ${at}`
  } else {
    result = "Stopped without an answer"
    icon = <CircleSlashIcon className="size-4 text-muted-foreground" />
  }
  return (
    <Item variant="outline" size="xs" className="w-fit not-typeset">
      <ItemMedia>{icon}</ItemMedia>
      <ItemContent>
        <ItemTitle className="font-normal text-muted-foreground">
          <span dir="auto">{a.ask}</span> · <span className="text-foreground">{result.trim()}</span>
        </ItemTitle>
      </ItemContent>
    </Item>
  )
}

// QuestionForm is ask_user's form (SPEC 8.8): 1–4 questions, one at a
// time, with Other always there. Writing in the chat answers it too.
export function QuestionForm({ question: q, ...p }: CardProps & { question: Question }) {
  const ref = useCardRef(cardKey(p.message, p.index))
  const [busy, setBusy] = React.useState(false)
  const items = React.useMemo(
    () =>
      (q.questions ?? []).map((it) => ({
        name: it.header,
        required: true,
        choices: (it.options ?? []).map((o) => ({ value: o.label })),
      })),
    [q.questions],
  )
  if (q.answered_at || q.stopped || !p.live) return <QuestionRecord question={q} />
  const submit = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const fd = new FormData(e.currentTarget)
    const answers: Record<string, string[]> = {}
    for (const it of q.questions ?? []) answers[it.header] = fd.getAll(it.header).map(String).filter((v) => v.trim())
    setBusy(true)
    ChatService.Answer(p.project, p.chat, { message: p.message, index: p.index, answers })
      .catch(showError)
      .finally(() => setBusy(false))
  }
  const many = (q.questions?.length ?? 0) > 1
  return (
    <div ref={ref} className="not-typeset">
      <Questionnaire items={items} shortcuts="numbers" onSubmit={submit}>
        {(q.questions ?? []).map((it) => (
          <QuestionnaireItem key={it.header} name={it.header} required multiple={it.multi}>
            <Card size="sm">
              <CardHeader>
                <Kicker>Question · {it.header}</Kicker>
                <QuestionnaireTitle render={<CardTitle className="text-base" dir="auto" />}>{it.question}</QuestionnaireTitle>
              </CardHeader>
              <CardContent>
                <QuestionnaireChoices>
                  {(it.options ?? []).map((o) => (
                    <QuestionnaireChoice
                      key={o.label}
                      value={o.label}
                      defaultChecked={!it.multi && o.recommended}
                      dir={textDir(o.label + " " + (o.description ?? ""))}
                    >
                      <span className="flex items-center gap-2 font-medium" dir="auto">
                        {o.label}
                        {o.recommended && <Badge variant="secondary">Recommended</Badge>}
                      </span>
                      {o.description && (
                        <QuestionnaireChoiceDescription dir="auto">{o.description}</QuestionnaireChoiceDescription>
                      )}
                    </QuestionnaireChoice>
                  ))}
                  <QuestionnaireInput aria-label="Other answer" placeholder="Other…" dir="auto" />
                </QuestionnaireChoices>
              </CardContent>
              <CardFooter className="flex-col items-stretch gap-2">
                <QuestionnaireActions>
                  <QuestionnairePrevious size="sm" variant="outline" />
                  <QuestionnaireProgress className={cn(!many && "invisible")} />
                  <QuestionnaireNext size="sm" />
                  <QuestionnaireSubmit size="sm" disabled={busy}>
                    Submit
                  </QuestionnaireSubmit>
                </QuestionnaireActions>
                <span className="text-sm text-muted-foreground">Or type your answer in the chat.</span>
              </CardFooter>
            </Card>
          </QuestionnaireItem>
        ))}
      </Questionnaire>
    </div>
  )
}

function QuestionRecord({ question: q }: { question: Question }) {
  const answered = q.answers && Object.keys(q.answers).length > 0
  return (
    <div className="flex flex-col gap-1.5 not-typeset">
      {(q.questions ?? []).map((it) => (
        <Item key={it.header} variant="outline" size="xs" className="w-fit">
          <ItemMedia>
            {answered ? <CheckIcon className="size-4 text-tone-green" /> : <CircleSlashIcon className="size-4 text-muted-foreground" />}
          </ItemMedia>
          <ItemContent>
            <ItemTitle className="font-normal text-muted-foreground" dir="auto">
              {it.question} ·{" "}
              <span className="text-foreground">
                {answered ? (q.answers?.[it.header] ?? []).join(", ") || "no answer" : q.note ? "you wrote instead" : "not answered"}
              </span>
            </ItemTitle>
          </ItemContent>
        </Item>
      ))}
    </div>
  )
}

// WaitingBar shows above the composer while the card the turn waits for
// is out of view (SPEC 8.8). Show scrolls to it.
export function WaitingBar({ message, index, text, kind }: { message: string; index: number; text: string; kind: string }) {
  const key = cardKey(message, index)
  const [visible, setVisible] = React.useState(true)
  const [el, setEl] = React.useState<HTMLElement | null>(() => cards.get(key) ?? null)
  React.useEffect(() => {
    const f = () => setEl(cards.get(key) ?? null)
    f()
    cardListeners.add(f)
    return () => void cardListeners.delete(f)
  }, [key])
  React.useEffect(() => {
    if (!el) return setVisible(false)
    const io = new IntersectionObserver(([e]) => setVisible(e.isIntersecting), { threshold: 0.2 })
    io.observe(el)
    return () => io.disconnect()
  }, [el])
  if (visible) return null
  return (
    <Item variant="muted" size="xs" className="not-typeset">
      <ItemMedia>
        <BellIcon className="size-4 text-tone-amber" />
      </ItemMedia>
      <ItemContent>
        <ItemTitle className="font-normal">
          Waiting for your {kind === "question" ? "answer" : "approval"}:{" "}
          <span className="font-medium" dir="auto">
            {text}
          </span>
        </ItemTitle>
      </ItemContent>
      <ItemActions>
        <Button size="xs" variant="outline" disabled={!el} onClick={() => el?.scrollIntoView({ block: "center", behavior: "smooth" })}>
          Show
        </Button>
      </ItemActions>
    </Item>
  )
}

const retryKinds: Record<string, string> = {
  rate_limited: "is rate limited",
  overloaded: "is overloaded",
  transport: "can't be reached",
}

// RetryBar is a turn waiting to try a failed request again (8.3).
export function RetryBar({ project, chat, retry }: { project: string; chat: string; retry: Retry }) {
  const [now, setNow] = React.useState(() => Date.now())
  React.useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [])
  const left = Math.max(0, Math.ceil((Date.parse(retry.at) - now) / 1000))
  return (
    <Item variant="muted" size="xs" className="not-typeset">
      <ItemMedia>
        <RefreshCwIcon className="size-4 text-tone-amber" />
      </ItemMedia>
      <ItemContent>
        <ItemTitle className="font-normal">
          {retry.provider} {retryKinds[retry.kind] ?? "failed"}, retrying {left > 0 ? `in ${left} s` : "now"}
        </ItemTitle>
      </ItemContent>
      <ItemActions>
        <Button size="xs" variant="outline" onClick={() => ChatService.Retry(project, chat).catch(showError)}>
          Retry now
        </Button>
        <Button size="xs" variant="ghost" onClick={() => ChatService.Stop(project, chat).catch(showError)}>
          Cancel
        </Button>
      </ItemActions>
    </Item>
  )
}

// FailedCard is a turn a provider error stopped; Retry continues it (8.3).
export function FailedCard({ project, chat, text }: { project: string; chat: string; text: string }) {
  const [busy, setBusy] = React.useState(false)
  return (
    <Card size="sm" className="not-typeset">
      <CardHeader>
        <CardTitle>The turn stopped</CardTitle>
        <CardDescription dir="auto">{noticeText(text)}</CardDescription>
      </CardHeader>
      <CardFooter>
        <Button
          size="sm"
          disabled={busy}
          onClick={() => {
            setBusy(true)
            ChatService.Retry(project, chat)
              .catch(showError)
              .finally(() => setBusy(false))
          }}
        >
          <RefreshCwIcon data-icon="inline-start" />
          Retry
        </Button>
      </CardFooter>
    </Card>
  )
}
