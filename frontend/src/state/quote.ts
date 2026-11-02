import { create } from "zustand"

import { Role, type Message, type Quote } from "@/lib/api"

// QUOTE_MAX is the most a quote holds, in UTF-8 bytes, as chat.QuoteMax:
// a longer passage is cut at a character before it (TASK-019).
export const QUOTE_MAX = 4000

const encoder = new TextEncoder()

// cutQuote trims s and cuts it to QUOTE_MAX bytes, at a character, as Go
// does.
export function cutQuote(s: string): { text: string; cut: boolean } {
  const t = s.trim()
  if (encoder.encode(t).length <= QUOTE_MAX) return { text: t, cut: false }
  let n = 0
  let end = 0
  for (const ch of t) {
    n += encoder.encode(ch).length
    if (n > QUOTE_MAX) break
    end += ch.length
  }
  return { text: t.slice(0, end).trim(), cut: true }
}

// messageText is a message's text, for *Reply* on the whole message.
export function messageText(m: Message): string {
  return (m.parts ?? [])
    .flatMap((p) => (p.text?.text.trim() ? [p.text.text.trim()] : []))
    .join("\n\n")
}

// quoteOf is a quote of text from message m, or null when m can't be
// quoted: only the user's and the assistant's text can.
export function quoteOf(m: Message, text: string): Quote | null {
  if (m.role !== Role.RoleUser && m.role !== Role.RoleAssistant) return null
  const { text: t, cut } = cutQuote(text)
  if (!t) return null
  return { text: t, message: m.id, turn: m.turn, role: m.role, ...(cut && { cut }) }
}

// The composer's quote per chat: one per message, kept while another chat
// is open, like the draft.
interface QuoteState {
  quotes: Record<string, Quote>
  set(chat: string, q: Quote | null): void
}

export const useQuotes = create<QuoteState>((set) => ({
  quotes: {},
  set: (chat, q) =>
    set((s) => {
      const quotes = { ...s.quotes }
      if (q) quotes[chat] = q
      else delete quotes[chat]
      return { quotes }
    }),
}))
