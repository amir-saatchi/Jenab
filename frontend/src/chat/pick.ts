import * as React from "react"

import type { Approval, Message, Waiting } from "@/lib/api"

// The window card of desktop control (SPEC 8.10): the agent names an app
// and the user picks one of its windows.

type Option = NonNullable<Approval["options"]>[number]

// A pick option's grant carries what it picks: "pick:<value>".
export const isPick = (g: string) => g.startsWith("pick:")

export type PickView =
  // The one window of the app the agent named: Allow, Deny, Show all.
  | { kind: "one"; window: Option }
  // A list: the app's windows first; the rest folded when there are some.
  | { kind: "list"; first: Option[]; other: Option[]; folded: boolean }

// pickView lays out a pick card's options; all is "Show all windows".
// Options without a group (the agent named no app) are one plain list.
export function pickView(options: Option[] | null | undefined, all: boolean): PickView | null {
  const picks = (options ?? []).filter((o) => isPick(o.grant))
  if (picks.length === 0) return null
  if (!picks.some((o) => o.group)) return { kind: "list", first: picks, other: [], folded: false }
  const first = picks.filter((o) => o.group === "match")
  const other = picks.filter((o) => o.group !== "match")
  if (!all && first.length === 1) return { kind: "one", window: first[0] }
  // With no window of the app, the others are all there is: unfolded.
  return { kind: "list", first, other, folded: !all && first.length > 0 }
}

// pickWaiting: the turn waits for a window card. The chat then stops
// following new content, so the card holds still under the pointer.
export function pickWaiting(messages: Message[], waiting: Waiting | null): boolean {
  if (!waiting) return false
  const a = messages.find((m) => m.id === waiting.message)?.parts?.[waiting.index]?.approval
  return a?.kind === "desktop_window" && !a.answer && !a.stopped
}

// MoveGuard tells whether a card held still long enough for a click to be
// meant for what is under the pointer now: it moved when its position
// changed, its size changed, or it appeared.
export class MoveGuard {
  private at: { x: number; y: number } | null = null
  private movedAt = -Infinity

  constructor(readonly hold = 500) {}

  // seen records the card's position at now (ms).
  seen(x: number, y: number, now: number) {
    if (!this.at || this.at.x !== x || this.at.y !== y) {
      this.at = { x, y }
      this.movedAt = now
    }
  }

  // moved records a change that keeps the position, such as a resize.
  moved(now: number) {
    this.movedAt = now
  }

  steady(now: number) {
    return now - this.movedAt >= this.hold
  }
}

// useMoveGuard watches el's place on screen: a scroll, a window resize, or
// a size change of el or of anything around it. steady() reads the place
// once more, so a move no event reported still counts.
export function useMoveGuard(el: React.RefObject<HTMLElement | null>, on: boolean) {
  const guard = React.useRef(new MoveGuard())
  React.useEffect(() => {
    const node = el.current
    if (!on || !node) return
    const g = guard.current
    const sample = () => {
      const r = node.getBoundingClientRect()
      g.seen(r.left, r.top, performance.now())
    }
    sample()
    const ro = new ResizeObserver((entries) => {
      if (entries.some((e) => e.target === node)) g.moved(performance.now())
      sample()
    })
    for (let n: HTMLElement | null = node; n && n !== document.body; n = n.parentElement) ro.observe(n)
    window.addEventListener("scroll", sample, { capture: true, passive: true })
    window.addEventListener("resize", sample)
    return () => {
      ro.disconnect()
      window.removeEventListener("scroll", sample, { capture: true })
      window.removeEventListener("resize", sample)
    }
  }, [el, on])
  return React.useCallback(() => {
    const node = el.current
    if (node) {
      const r = node.getBoundingClientRect()
      guard.current.seen(r.left, r.top, performance.now())
    }
    return guard.current.steady(performance.now())
  }, [el])
}
