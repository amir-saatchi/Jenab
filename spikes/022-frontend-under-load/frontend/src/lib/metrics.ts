// Frame, long-task, LoAF and event-timing collection plus small stats helpers.
export const epoch = () => performance.timeOrigin + performance.now();
export const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
export const nextFrame = () => new Promise<number>((r) => requestAnimationFrame(r));

/** Resolves just after the next frame has been produced (rAF + message-channel task). */
export function afterPaint(): Promise<number> {
  return new Promise((resolve) => {
    requestAnimationFrame(() => {
      const ch = new MessageChannel();
      ch.port1.onmessage = () => resolve(performance.now());
      ch.port2.postMessage(0);
    });
  });
}

export function pct(xs: number[], p: number): number {
  if (!xs.length) return NaN;
  const s = [...xs].sort((a, b) => a - b);
  const i = Math.min(s.length - 1, Math.max(0, Math.ceil((p / 100) * s.length) - 1));
  return s[i];
}

export const r1 = (x: number) => Math.round(x * 10) / 10;
export const r2 = (x: number) => Math.round(x * 100) / 100;

export function summary(xs: number[]) {
  if (!xs.length) return { n: 0 };
  const sum = xs.reduce((a, b) => a + b, 0);
  return { n: xs.length, mean: r2(sum / xs.length), p50: r2(pct(xs, 50)), p95: r2(pct(xs, 95)), p99: r2(pct(xs, 99)), max: r2(Math.max(...xs)), min: r2(Math.min(...xs)) };
}

export class FrameRecorder {
  deltas: number[] = [];
  private last = 0;
  private running = false;
  private t0 = 0;
  private t1 = 0;
  start() {
    this.deltas = [];
    this.running = true;
    this.t0 = performance.now();
    this.last = 0;
    const tick = (t: number) => {
      if (!this.running) return;
      if (this.last) this.deltas.push(t - this.last);
      this.last = t;
      requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  }
  stop() {
    this.running = false;
    this.t1 = performance.now();
    return this.stats();
  }
  stats() {
    const d = this.deltas;
    const dur = this.t1 - this.t0;
    const over = (ms: number) => r2((100 * d.filter((x) => x > ms).length) / Math.max(1, d.length));
    return {
      frames: d.length,
      durationMs: r1(dur),
      fps: r1((d.length * 1000) / Math.max(1, dur)),
      p50: r2(pct(d, 50)),
      p95: r2(pct(d, 95)),
      p99: r2(pct(d, 99)),
      max: r2(d.length ? Math.max(...d) : 0),
      pctOver16_7: over(16.7),
      pctOver18: over(18), // over 16.7 plus vsync jitter tolerance: a missed 60 Hz frame
      pctOver33_4: over(33.4),
      pctOver50: over(50),
    };
  }
}

type Span = { start: number; duration: number; blocking?: number };
export const longTasks: Span[] = [];
export const loafs: Span[] = [];
export const eventTimings: { name: string; start: number; duration: number; inputDelay: number; processing: number }[] = [];
let observing = false;

export function observeAll() {
  if (observing) return;
  observing = true;
  try {
    new PerformanceObserver((l) => l.getEntries().forEach((e) => longTasks.push({ start: e.startTime, duration: e.duration }))).observe({ type: "longtask", buffered: true });
  } catch { /* unsupported */ }
  try {
    new PerformanceObserver((l) =>
      l.getEntries().forEach((e: any) => loafs.push({ start: e.startTime, duration: e.duration, blocking: e.blockingDuration })),
    ).observe({ type: "long-animation-frame", buffered: true });
  } catch { /* unsupported */ }
  try {
    new PerformanceObserver((l) =>
      l.getEntries().forEach((e: any) =>
        eventTimings.push({ name: e.name, start: e.startTime, duration: e.duration, inputDelay: e.processingStart - e.startTime, processing: e.processingEnd - e.processingStart }),
      ),
    ).observe({ type: "event", buffered: true, durationThreshold: 16 } as PerformanceObserverInit);
  } catch { /* unsupported */ }
}

/** Long tasks and long animation frames that started inside [from, to] (performance.now ms). */
export function blockingIn(from: number, to: number) {
  const lt = longTasks.filter((t) => t.start >= from && t.start <= to).map((t) => t.duration);
  const lf = loafs.filter((t) => t.start >= from && t.start <= to);
  return {
    longTasks: lt.length,
    longTaskMaxMs: r1(lt.length ? Math.max(...lt) : 0),
    longTaskTotalMs: r1(lt.reduce((a, b) => a + b, 0)),
    longTasksOver100: lt.filter((x) => x > 100).length,
    loafs: lf.length,
    loafMaxMs: r1(lf.length ? Math.max(...lf.map((x) => x.duration)) : 0),
    loafMaxBlockingMs: r1(lf.length ? Math.max(...lf.map((x) => x.blocking ?? 0)) : 0),
  };
}

export function eventsIn(from: number, to: number) {
  return eventTimings.filter((e) => e.start >= from && e.start <= to);
}

export type Phase = { name: string; start: number; end: number };
export const phases: Phase[] = [];
/** Records an epoch-ms window for Go-side CPU statistics. */
export async function phase<T>(name: string, fn: () => Promise<T>): Promise<T> {
  const start = epoch();
  try {
    return await fn();
  } finally {
    phases.push({ name, start, end: epoch() });
  }
}
