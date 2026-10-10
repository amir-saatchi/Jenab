package main

import (
	"encoding/json"
	"fmt"
)

// The page side of the bench. setupJS installs window.__bench once: the
// long-task and long-animation-frame observers, a frame recorder and the
// key → paint timer, as SPIKE-022 measured them. The UI is found by the
// attributes the app's components set; the app has no hooks for this.
const setupJS = `(() => {
  if (window.__bench) return true;
  const B = window.__bench = { lt: [], loaf: [], keys: [], frames: [], measureKeys: false };
  try { new PerformanceObserver(l => l.getEntries().forEach(e => B.lt.push([e.startTime, e.duration]))).observe({ type: "longtask", buffered: true }); } catch (e) {}
  try { new PerformanceObserver(l => l.getEntries().forEach(e => B.loaf.push([e.startTime, e.duration, e.blockingDuration || 0]))).observe({ type: "long-animation-frame", buffered: true }); } catch (e) {}
  // afterPaint resolves just after the next frame is produced.
  B.afterPaint = () => new Promise(r => requestAnimationFrame(() => { const ch = new MessageChannel(); ch.port1.onmessage = () => r(performance.now()); ch.port2.postMessage(0); }));
  B.until = async (f, ms = 30000, what = "a condition") => {
    const t0 = performance.now();
    for (;;) {
      const v = f();
      if (v) return v;
      if (performance.now() - t0 > ms) throw new Error("timed out waiting for " + what);
      await new Promise(r => setTimeout(r, 5));
    }
  };
  B.chatButton = t => [...document.querySelectorAll('[data-slot="sidebar-menu-button"]')].find(b => ((b.querySelector("span.truncate") || {}).textContent || "").trim() === t);
  B.composer = () => document.querySelector('textarea[aria-label="Message"]');
  B.stop = () => [...document.querySelectorAll("button")].find(b => b.textContent.trim() === "Stop");
  B.working = () => !!document.querySelector('[data-slot="spinner"][aria-label="Working"]');
  B.open = t => { const c = B.composer(); return c && (c.placeholder || "").includes(t); };
  B.rows = () => document.querySelectorAll('[data-slot="message-scroller-item"][data-message-id]').length;
  B.pct = (xs, p) => { if (!xs.length) return 0; const s = [...xs].sort((a, b) => a - b); return s[Math.min(s.length - 1, Math.max(0, Math.ceil(p / 100 * s.length) - 1))]; };
  B.r = x => Math.round(x * 10) / 10;
  B.blocking = (from, to) => {
    const lt = B.lt.filter(t => t[0] >= from && t[0] <= to).map(t => t[1]);
    const lf = B.loaf.filter(t => t[0] >= from && t[0] <= to);
    return { long_tasks: lt.length, long_task_max_ms: B.r(Math.max(0, ...lt)),
      loafs: lf.length, loaf_max_ms: B.r(Math.max(0, ...lf.map(x => x[1]))), loaf_max_blocking_ms: B.r(Math.max(0, ...lf.map(x => x[2]))) };
  };
  B.startFrames = () => {
    B.frames = []; B.recording = true; B.f0 = performance.now(); let last = 0;
    const tick = t => { if (!B.recording) return; if (last) B.frames.push(t - last); last = t; requestAnimationFrame(tick); };
    requestAnimationFrame(tick);
  };
  B.stopFrames = () => {
    B.recording = false; const d = B.frames, dur = performance.now() - B.f0;
    const over = ms => B.r(100 * d.filter(x => x > ms).length / Math.max(1, d.length));
    return { frames: d.length, seconds: B.r(dur / 1000), fps: B.r(d.length * 1000 / Math.max(1, dur)),
      frame_p95_ms: B.r(B.pct(d, 95)), frame_max_ms: B.r(Math.max(0, ...d)), over_18_ms_pct: over(18), over_33_ms_pct: over(33.4) };
  };
  document.addEventListener("keydown", e => {
    if (!B.measureKeys) return;
    const t = e.timeStamp;
    B.afterPaint().then(p => B.keys.push(p - t));
  }, true);
  return true;
})()`

// startupJS waits until the shell is up and painted in a visible page. It
// gives the paint as epoch milliseconds, and whether the shell was already
// there when the probe began, so the bench knows it came too late.
const startupJS = `(async () => {
  const there = !!document.querySelector('nav[aria-label="Projects"]') && document.visibilityState === "visible";
  for (;;) {
    if (document.querySelector('nav[aria-label="Projects"]') && document.visibilityState === "visible") break;
    await new Promise(r => setTimeout(r, 2));
  }
  await new Promise(r => requestAnimationFrame(() => { const ch = new MessageChannel(); ch.port1.onmessage = r; ch.port2.postMessage(0); }));
  const paint = performance.getEntriesByType("paint").map(e => [e.name, Math.round(performance.timeOrigin + e.startTime)]);
  return { painted: Math.round(performance.timeOrigin + performance.now()), already: there, navigation: Math.round(performance.timeOrigin), paint };
})()`

// openJS clicks a chat in the chat list and measures until its rows are
// painted: the first rows, then all rows once their count holds for 300 ms.
func openJS(title string) string {
	return fmt.Sprintf(`(async () => {
  const B = window.__bench, title = %s;
  const btn = await B.until(() => B.chatButton(title), 10000, "the chat " + title);
  const t0 = performance.now();
  btn.click();
  await B.until(() => B.open(title) && B.rows() > 0 && !(document.querySelector('[data-slot="message-scroller-viewport"]') || {hasAttribute: () => true}).hasAttribute("data-pending-scroll"), 10000, "the rows of " + title);
  const first = await B.afterPaint();
  let n = B.rows(), since = performance.now();
  while (performance.now() - since < 300) {
    await new Promise(r => setTimeout(r, 20));
    const k = B.rows();
    if (k !== n) { n = k; since = performance.now(); }
  }
  return Object.assign({ first_rows_ms: B.r(first - t0), all_rows_ms: B.r(since - t0), rows: n }, B.blocking(t0, performance.now()));
})()`, jsString(title))
}

// switchJS opens a chat without measuring.
func switchJS(title string) string {
	return fmt.Sprintf(`(async () => {
  const B = window.__bench, title = %s;
  (await B.until(() => B.chatButton(title), 10000, "the chat " + title)).click();
  await B.until(() => B.open(title), 10000, "the chat " + title + " to open");
  B.composer().focus();
  return true;
})()`, jsString(title))
}

// streamingJS waits until the answer streams: the spinner is gone and the
// Stop button shows.
const streamingJS = `window.__bench.until(() => !window.__bench.working() && window.__bench.stop(), 30000, "the answer to stream").then(() => true)`

const startMeasureJS = `(() => { const B = window.__bench; B.keys = []; B.t0 = performance.now(); B.startFrames(); B.measureKeys = true; return true; })()`

const stopMeasureJS = `(async () => {
  const B = window.__bench;
  B.measureKeys = false;
  await new Promise(r => setTimeout(r, 100));
  const f = B.stopFrames(), k = B.keys;
  return Object.assign(f, { keys: k.length, key_paint_p50_ms: B.r(B.pct(k, 50)), key_paint_p95_ms: B.r(B.pct(k, 95)), key_paint_max_ms: B.r(Math.max(0, ...k)) }, B.blocking(B.t0, performance.now()));
})()`

// clearJS empties the composer as a user would, through React's setter.
const clearJS = `(() => {
  const c = window.__bench.composer();
  Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value").set.call(c, "");
  c.dispatchEvent(new Event("input", { bubbles: true }));
  return true;
})()`

// finishedJS waits until no turn runs in the open chat.
const finishedJS = `window.__bench.until(() => !window.__bench.stop() && !window.__bench.working(), 120000, "the answer to finish").then(() => true)`

// sentJS waits until the composer is empty: the message went.
const sentJS = `window.__bench.until(() => window.__bench.composer() && window.__bench.composer().value === "", 10000, "the message to go").then(() => true)`

const heapJS = `(() => performance.memory ? Math.round(performance.memory.usedJSHeapSize / 1048576) : 0)()`

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
