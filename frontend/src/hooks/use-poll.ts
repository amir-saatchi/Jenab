import * as React from "react"

// usePoll calls fn every ms while the window is visible, and once at once.
// A failed call clears the value, so nothing stale stays on screen.
export function usePoll<T>(fn: () => Promise<T>, ms: number): T | undefined {
  const [value, setValue] = React.useState<T>()
  React.useEffect(() => poll(fn, ms, setValue), [fn, ms])
  return value
}

// Page is the part of document that poll uses.
export type Page = Pick<Document, "visibilityState" | "addEventListener" | "removeEventListener">

// poll calls fn every ms while page is visible and hands each answer, or
// undefined after a failure, to onValue. A tick is skipped while the call
// before is still out, so answers come in order. It returns the stop.
export function poll<T>(fn: () => Promise<T>, ms: number, onValue: (v: T | undefined) => void, page: Page = document) {
  let timer: ReturnType<typeof setInterval> | undefined
  let busy = false
  let done = false
  const tick = () => {
    if (busy) return
    busy = true
    fn()
      .then(
        (v) => !done && onValue(v),
        () => !done && onValue(undefined),
      )
      .finally(() => (busy = false))
  }
  const start = () => {
    if (timer || page.visibilityState !== "visible") return
    tick()
    timer = setInterval(tick, ms)
  }
  const stop = () => {
    clearInterval(timer)
    timer = undefined
  }
  const onVisibility = () => (page.visibilityState === "visible" ? start() : stop())
  start()
  page.addEventListener("visibilitychange", onVisibility)
  return () => {
    done = true
    stop()
    page.removeEventListener("visibilitychange", onVisibility)
  }
}
