import * as React from "react"
import { ReplyIcon, XIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { InputGroupButton } from "@/components/ui/input-group"
import { useMessageScroller } from "@/components/ui/message-scroller"
import { Role, type Message, type Quote } from "@/lib/api"
import { cn } from "@/lib/utils"
import { rows } from "@/chat/rows"
import { quoteOf, useQuotes } from "@/state/quote"
import { useThreads } from "@/state/thread"

// Reply to part of a message (TASK-019). Text the user can quote carries
// data-quote-source with its message's ID; a selection inside one such
// element gets a *Reply* button, and the quote goes to the composer.

// sourceOf is the message ID of the quotable text n is in.
function sourceOf(n: Node | null): HTMLElement | null {
  const el = n instanceof Element ? n : n?.parentElement
  return el?.closest<HTMLElement>("[data-quote-source]") ?? null
}

// reply puts a quote of text from message m into the chat's composer,
// which takes the focus.
export function reply(chat: string, m: Message, text: string) {
  const q = quoteOf(m, text)
  if (!q) return
  useQuotes.getState().set(chat, q)
  document.querySelector<HTMLTextAreaElement>(`[data-composer="${CSS.escape(chat)}"]`)?.focus()
}

// SelectionReply shows *Reply* above a selection inside one message's text
// in root.
export function SelectionReply({ chat, root }: { chat: string; root: React.RefObject<HTMLElement | null> }) {
  const [at, setAt] = React.useState<{ top: number; left: number; source: string; text: string } | null>(null)
  React.useEffect(() => {
    let frame = 0
    const update = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        const sel = window.getSelection()
        const range = sel && sel.rangeCount > 0 && !sel.isCollapsed ? sel.getRangeAt(0) : null
        const src = range && sourceOf(range.startContainer)
        const text = sel?.toString().trim() ?? ""
        // A message's text parts are apart; the selection may span them.
        const same = src && src.dataset.quoteSource === sourceOf(range.endContainer)?.dataset.quoteSource
        if (!range || !src || !same || !root.current?.contains(src) || !text) {
          setAt(null)
          return
        }
        const r = range.getBoundingClientRect()
        const box = root.current.getBoundingClientRect()
        if (r.bottom < box.top || r.top > box.bottom) {
          setAt(null) // scrolled out of the chat
          return
        }
        // Above the selection, or below it at the chat's top edge.
        const top = r.top - box.top > 40 ? r.top - 36 : r.bottom + 6
        const left = Math.min(Math.max(r.left + r.width / 2, box.left + 40), box.right - 40)
        setAt({ top, left, source: src.dataset.quoteSource ?? "", text })
      })
    }
    document.addEventListener("selectionchange", update)
    document.addEventListener("scroll", update, true)
    window.addEventListener("resize", update)
    return () => {
      cancelAnimationFrame(frame)
      document.removeEventListener("selectionchange", update)
      document.removeEventListener("scroll", update, true)
      window.removeEventListener("resize", update)
    }
  }, [root])
  if (!at) return null
  const go = () => {
    const m = useThreads.getState().threads[chat]?.messages.find((m) => m.id === at.source)
    if (m) reply(chat, m, at.text)
    window.getSelection()?.removeAllRanges()
  }
  return (
    <Button
      variant="secondary"
      size="xs"
      className="fixed z-50 -translate-x-1/2 shadow-md rtl:translate-x-1/2"
      style={{ top: at.top, left: at.left }}
      // Keep the selection until the click.
      onPointerDown={(e) => e.preventDefault()}
      onClick={go}
    >
      <ReplyIcon data-icon="inline-start" />
      Reply
    </Button>
  )
}

// QuoteText is a quote's text: a bar at its start, in its own direction,
// so a right-to-left quote keeps its direction (SPEC 5.8).
function QuoteText({ quote, lines }: { quote: Quote; lines: string }) {
  return (
    <span dir="auto" className={cn("block border-s-2 border-primary/40 ps-2.5 text-start whitespace-pre-wrap", lines)}>
      {quote.text}
      {quote.cut && "…"}
    </span>
  )
}

function whose(q: Quote) {
  return q.role === Role.RoleAssistant ? "the answer" : "your message"
}

// QuoteChip is the quote above the composer's input, until it is sent or
// removed.
export function QuoteChip({ quote, onRemove }: { quote: Quote; onRemove: () => void }) {
  return (
    <div className="flex w-full min-w-0 items-start gap-2 rounded-md bg-muted px-2 py-1.5 font-normal">
      <div className="grid min-w-0 flex-1 gap-0.5">
        <span className="text-xs text-muted-foreground">Replying to {whose(quote)}</span>
        <QuoteText quote={quote} lines="line-clamp-2 text-foreground" />
      </div>
      <InputGroupButton size="icon-xs" variant="ghost" aria-label="Remove the quote" onClick={onRemove}>
        <XIcon />
      </InputGroupButton>
    </div>
  )
}

// QuoteView is a sent message's quote, above its text; a click goes to the
// quoted message.
export function QuoteView({ chat, quote }: { chat: string; quote: Quote }) {
  const jump = useJump(chat)
  return (
    <button
      type="button"
      data-slot="quote"
      className="max-w-[80%] min-w-0 cursor-pointer rounded-md py-0.5 text-sm text-muted-foreground transition-colors outline-none hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50"
      aria-label={`Go to the quoted text: ${quote.text}`}
      onClick={() => void jump(quote.message)}
    >
      <QuoteText quote={quote} lines="line-clamp-3" />
    </button>
  )
}

// useJump scrolls to a message of the chat, loading older turns until it
// is there, and marks it for a moment.
function useJump(chat: string) {
  const { scrollToMessage } = useMessageScroller()
  return React.useCallback(
    async (message: string) => {
      const threads = useThreads.getState()
      let t = threads.threads[chat]
      while (t && t.from > 1 && !t.messages.some((m) => m.id === message)) {
        const from = t.from
        await threads.older(chat).catch(() => {})
        t = useThreads.getState().threads[chat]
        if (!t || t.from === from) break
      }
      const row = t && rows(t.messages).find((r) => (r.kind === "agent" ? r.messages.some((m) => m.id === message) : r.message.id === message))
      if (!row) {
        toast.info("The quoted message is no longer in this chat.")
        return
      }
      // A frame for older turns to render.
      requestAnimationFrame(() => {
        scrollToMessage(row.id, { align: "center", behavior: "smooth" })
        const el = document.querySelector<HTMLElement>(`[data-quote-source="${CSS.escape(message)}"]`)
        if (!el) return
        el.dataset.flash = ""
        setTimeout(() => delete el.dataset.flash, 1600)
      })
    },
    [chat, scrollToMessage],
  )
}

// flash is the class of quotable text, so a jump to it shows where it is.
export const flash =
  "rounded-xl ring-offset-background transition-shadow duration-500 data-flash:ring-3 data-flash:ring-ring/50 data-flash:ring-offset-4"
