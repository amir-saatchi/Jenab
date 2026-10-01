import { ArrowRightIcon, PencilIcon } from "lucide-react"

import { Bubble, BubbleContent } from "@/components/ui/bubble"
import { Button } from "@/components/ui/button"
import { Message, MessageContent, MessageHeader } from "@/components/ui/message"
import { AppShell } from "@/jenab/shell"
import {
  AgentMessage,
  AgentText,
  ChatHeader,
  Composer,
  Notice,
  StreamingText,
  Thread,
  ToolChip,
  TurnFooter,
  UserMessage,
} from "@/jenab/chat"
import type { Chat } from "@/data/bitcoin"

const list: Chat[] = [
  { id: "mother", title: "Mother", sub: "Project home", status: "idle", mother: true },
  { id: "reviewer", title: "Reviewer", sub: "Working for Mother", status: "working" },
  { id: "gold", title: "Gold tracker", sub: "New · working for Mother", status: "working" },
  { id: "eth", title: "ETH tracker", sub: "Tracks ETH daily", status: "idle" },
  { id: "fees", title: "Fees question", sub: "Yesterday", status: "idle" },
]

// The Reviewer chat, beside Mother, to show what the target chat sees.
function ReviewerChat() {
  return (
    <aside className="flex w-[22rem] shrink-0 flex-col border-s bg-sidebar">
      <div className="flex flex-col gap-1 border-b px-4 py-3">
        <span className="text-xs text-muted-foreground">What the Reviewer chat shows</span>
        <div className="flex items-center gap-2">
          <span className="font-medium">Reviewer</span>
          <Button variant="ghost" size="xs" className="ms-auto text-muted-foreground">
            <PencilIcon data-icon="inline-start" />
            Edit role
          </Button>
        </div>
        <span className="text-xs text-muted-foreground">
          Role: reviews every new pipeline for cost and error handling.
        </span>
      </div>
      <Thread dense fromTop>
        <Message align="start">
          <MessageContent>
            <MessageHeader className="px-0">from Mother</MessageHeader>
            <Bubble variant="muted">
              <BubbleContent dir="auto">Please review pipeline daily_btc and report the issues.</BubbleContent>
            </Bubble>
          </MessageContent>
        </Message>
        <AgentMessage>
          <ToolChip name="read_config" detail="daily_btc" />
          <StreamingText>The price step calls the API once and has no retry. The news step</StreamingText>
        </AgentMessage>
      </Thread>
      <Composer compact placeholder="You can write here at any time." className="px-3 pb-3" />
    </aside>
  )
}

// 05 · Mother delegates (SPEC 8.6): send_to_chat, create_chat, and the finish notice.
export function Screen05() {
  return (
    <AppShell activeChat="mother" chatList={list} right={<ReviewerChat />}>
      <div className="flex h-full min-h-0 flex-col">
        <ChatHeader mother title="Mother" sub="Project home · directs the other chats" />
        <Thread fromTop>
          <UserMessage>
            Check the new daily_btc pipeline, and start tracking gold every day.
          </UserMessage>
          <AgentMessage footer={<TurnFooter time="11:20" />}>
            <AgentText>
              I'll ask Reviewer to check daily_btc, and start a Gold tracker chat for the daily gold price.
            </AgentText>
            <ToolChip name="send_to_chat" detail="Reviewer · check daily_btc · task t_31" state="running" />
            <ToolChip
              name="create_chat"
              detail="Gold tracker · role: tracks the gold price daily · task t_32"
              state="running"
            />
          </AgentMessage>
          <Notice>Started by the app · Reviewer finished (t_31) · 11:22</Notice>
          <AgentMessage footer={<TurnFooter time="11:22" />}>
            <AgentText>
              Reviewer found **2 issues** in daily_btc: the price step has no retry, and the news step
              has no limit. Should I ask it to fix them?
            </AgentText>
            <Button variant="outline" size="sm" className="w-fit">
              Open Reviewer
              <ArrowRightIcon data-icon="inline-end" />
            </Button>
          </AgentMessage>
        </Thread>
        <Composer mother placeholder="Message Mother…" />
      </div>
    </AppShell>
  )
}
