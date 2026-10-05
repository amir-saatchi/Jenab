import * as React from "react"

// usePoll calls fn every ms while the window is visible, and once at once.
export function usePoll<T>(fn: () => Promise<T>, ms: number): T | undefined {
  const [value, setValue] = React.useState<T>()
  React.useEffect(() => {
    let timer: ReturnType<typeof setInterval> | undefined
    const tick = () => void fn().then(setValue, () => {})
    const start = () => {
      if (timer || document.visibilityState !== "visible") return
      tick()
      timer = setInterval(tick, ms)
    }
    const stop = () => {
      clearInterval(timer)
      timer = undefined
    }
    const onVisibility = () => (document.visibilityState === "visible" ? start() : stop())
    start()
    document.addEventListener("visibilitychange", onVisibility)
    return () => {
      stop()
      document.removeEventListener("visibilitychange", onVisibility)
    }
  }, [fn, ms])
  return value
}
