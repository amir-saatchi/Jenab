import { Bubble, BubbleContent } from "@/components/ui/bubble"
import { Button } from "@/components/ui/button"
import { Message, MessageContent, MessageFooter } from "@/components/ui/message"
import { AppShell } from "@/jenab/shell"
import {
  AgentMessage,
  AgentText,
  ChatHeader,
  Composer,
  MemoryChip,
  Notice,
  StreamingText,
  Thread,
  ToolChip,
  ToolDetail,
  TurnFooter,
  UserMessage,
} from "@/jenab/chat"

// 03 · Anatomy of a turn (SPEC 8.3): a quick part first, background work,
// a finish turn, and a message written during a turn.
export function Screen03() {
  return (
    <AppShell activeChat="mother" rightTab="pipelines" activeItem="daily_btc" statuses={{ mother: "working" }}>
      <div className="flex h-full min-h-0 flex-col">
        <ChatHeader mother title="Mother" sub="Project home · directs the other chats" />
        <Thread fromTop>
          <UserMessage>What is BTC now? And refresh this week's news.</UserMessage>
          <AgentMessage footer={<TurnFooter time="10:02" />}>
            <AgentText>BTC is **$64,210**, up 2.1 % since yesterday.</AgentText>
            <ToolChip name="query" detail="btc_prices · 1 row · 38 ms" />
            <div className="flex items-center gap-2">
              <ToolChip name="run_pipeline" detail="daily_btc · run r_88 · running 0:24" state="running" />
              <Button variant="ghost" size="xs" className="text-muted-foreground">
                Stop run
              </Button>
            </div>
            <AgentText>
              The news refresh runs in the background. I'll post the new items when it's done.
            </AgentText>
          </AgentMessage>
          <Notice>Started by the app · run r_88 finished · 10:03</Notice>
          <AgentMessage footer={<TurnFooter time="10:03" />}>
            <AgentText>
              5 new items this week. The biggest is the **ETF options ruling** on 12 Sep.
            </AgentText>
            <MemoryChip>
              Memory updated: <span className="text-foreground">preferences (English sources)</span>
            </MemoryChip>
          </AgentMessage>
          <UserMessage>Add ETH as well.</UserMessage>
          <AgentMessage>
            <StreamingText>One table for all coins keeps the views simple. Planning the change</StreamingText>
            <ToolChip agent name="Schema agent" detail="designing prices(coin, day, close)" state="running" defaultOpen>
              <ToolDetail label="plan">create prices; copy btc_prices with coin = 'BTC'</ToolDetail>
              <ToolDetail label="updates">3 views · daily_btc · 1 page filter</ToolDetail>
            </ToolChip>
          </AgentMessage>
          <Message align="end">
            <MessageContent>
              <Bubble variant="outline" align="end">
                <BubbleContent dir="auto" className="border-dashed text-start">
                  Use EUR for both.
                </BubbleContent>
              </Bubble>
              <MessageFooter>Joins at the next step</MessageFooter>
            </MessageContent>
          </Message>
        </Thread>
        <Composer running mother />
      </div>
    </AppShell>
  )
}
