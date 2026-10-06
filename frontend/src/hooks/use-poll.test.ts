import { expect, test } from "bun:test"

import { poll, type Page } from "@/hooks/use-poll"

// A page whose visibility the test sets.
function page(visible = true) {
  const listeners = new Set<() => void>()
  const p = {
    visibilityState: visible ? "visible" : "hidden",
    addEventListener: (_: string, f: () => void) => listeners.add(f),
    removeEventListener: (_: string, f: () => void) => listeners.delete(f),
    show(v: boolean) {
      p.visibilityState = v ? "visible" : "hidden"
      for (const f of listeners) f()
    },
  }
  return p as typeof p & Page
}

// calls hands out promises the test settles by hand.
function calls<T>() {
  const out: Array<{ resolve: (v: T) => void; reject: (e: unknown) => void }> = []
  const fn = () => new Promise<T>((resolve, reject) => out.push({ resolve, reject }))
  return { out, fn }
}

test("a slow call skips the ticks while it is out", async () => {
  const { out, fn } = calls<number>()
  const seen: Array<number | undefined> = []
  const stop = poll(fn, 5, (v) => seen.push(v), page())
  await Bun.sleep(30) // several ticks
  expect(out.length).toBe(1)
  out[0].resolve(1)
  await Bun.sleep(15)
  expect(out.length).toBe(2)
  expect(seen).toEqual([1])
  stop()
})

test("a failed call clears the value", async () => {
  const { out, fn } = calls<number>()
  const seen: Array<number | undefined> = []
  const stop = poll(fn, 5, (v) => seen.push(v), page())
  out[0].resolve(1)
  await Bun.sleep(15)
  out[1].reject(new Error("gone"))
  await Bun.sleep(1)
  expect(seen).toEqual([1, undefined])
  stop()
})

test("no calls while hidden, and none after stop", async () => {
  const { out, fn } = calls<number>()
  const seen: Array<number | undefined> = []
  const pg = page(false)
  const stop = poll(fn, 5, (v) => seen.push(v), pg)
  await Bun.sleep(20)
  expect(out.length).toBe(0)
  pg.show(true)
  expect(out.length).toBe(1)
  pg.show(false)
  out[0].resolve(1)
  await Bun.sleep(20)
  expect(out.length).toBe(1)
  stop()
  pg.show(true)
  expect(out.length).toBe(1)
  expect(seen).toEqual([1])
})
