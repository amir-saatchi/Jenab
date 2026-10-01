// Q3: 10,000-point line and bar charts. Recharts (default, tuned, downsampled)
// and uPlot (canvas) as a comparison. Measures first render, resize and hover.
import { useEffect, useLayoutEffect, useRef } from "react";
import { Bar, BarChart, CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import type { Ctx } from "../main";
import { Shell } from "../shell";
import { afterPaint, blockingIn, FrameRecorder, nextFrame, phase, r1, sleep, summary } from "../lib/metrics";
import { call } from "../lib/wails";

type P = { x: number; y: number };

/** Largest-Triangle-Three-Buckets downsampling. */
export function lttb(data: P[], threshold: number): P[] {
  const n = data.length;
  if (threshold >= n || threshold < 3) return data;
  const out: P[] = [data[0]];
  const every = (n - 2) / (threshold - 2);
  let a = 0;
  for (let i = 0; i < threshold - 2; i++) {
    const s = Math.floor((i + 1) * every) + 1, e = Math.min(Math.floor((i + 2) * every) + 1, n);
    let ax = 0, ay = 0;
    for (let j = s; j < e; j++) { ax += data[j].x; ay += data[j].y; }
    ax /= e - s; ay /= e - s;
    const rs = Math.floor(i * every) + 1, re = Math.floor((i + 1) * every) + 1;
    let maxA = -1, pick = rs;
    for (let j = rs; j < re; j++) {
      const area = Math.abs((data[a].x - ax) * (data[j].y - data[a].y) - (data[a].x - data[j].x) * (ay - data[a].y));
      if (area > maxA) { maxA = area; pick = j; }
    }
    out.push(data[pick]);
    a = pick;
  }
  out.push(data[n - 1]);
  return out;
}

/** Mean per bucket, for bar charts. */
function bucketMean(data: P[], buckets: number): P[] {
  const size = Math.ceil(data.length / buckets);
  const out: P[] = [];
  for (let i = 0; i < data.length; i += size) {
    const sl = data.slice(i, i + size);
    out.push({ x: sl[0].x, y: sl.reduce((a, p) => a + p.y, 0) / sl.length });
  }
  return out;
}

const fmtDay = (x: number) => new Date(x).toISOString().slice(0, 10);
let commitAt = 0;

function RechartsView({ data, kind, opt }: { data: P[]; kind: "line" | "bar"; opt: boolean }) {
  useLayoutEffect(() => { if (!commitAt) commitAt = performance.now(); }, []);
  const anim = opt ? { isAnimationActive: false } : {};
  return (
    <ResponsiveContainer width="100%" height="100%">
      {kind === "line" ? (
        <LineChart data={data} margin={{ top: 10, right: 20, bottom: 10, left: 10 }}>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis dataKey="x" type="number" domain={["dataMin", "dataMax"]} tickFormatter={fmtDay} />
          <YAxis domain={["auto", "auto"]} />
          <Tooltip labelFormatter={(x) => fmtDay(Number(x))} />
          <Line dataKey="y" stroke="#2563eb" {...anim} {...(opt ? { dot: false } : {})} />
        </LineChart>
      ) : (
        <BarChart data={data} margin={{ top: 10, right: 20, bottom: 10, left: 10 }}>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis dataKey="x" tickFormatter={fmtDay} />
          <YAxis domain={["auto", "auto"]} />
          <Tooltip labelFormatter={(x) => fmtDay(Number(x))} />
          <Bar dataKey="y" fill="#2563eb" {...anim} />
        </BarChart>
      )}
    </ResponsiveContainer>
  );
}

function UPlotView({ data, kind }: { data: P[]; kind: "line" | "bar" }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    let u: any, ro: ResizeObserver | undefined, dead = false;
    (async () => {
      const { default: uPlot } = await import("uplot");
      await import("uplot/dist/uPlot.min.css");
      if (dead || !ref.current) return;
      const el = ref.current;
      const xs = data.map((p) => p.x / 1000), ys = data.map((p) => p.y);
      const series: any = { label: "y", stroke: "#2563eb", ...(kind === "bar" ? { fill: "#2563eb", paths: uPlot.paths.bars!({ size: [0.9, 100] }) } : {}) };
      u = new uPlot({ width: el.clientWidth, height: el.clientHeight - 40, series: [{}, series], scales: { x: { time: true } } }, [xs, ys], el);
      commitAt = performance.now();
      ro = new ResizeObserver(() => u.setSize({ width: el.clientWidth, height: el.clientHeight - 40 }));
      ro.observe(el);
    })();
    return () => { dead = true; ro?.disconnect(); u?.destroy(); };
  }, [data, kind]);
  return <div ref={ref} className="h-full w-full" />;
}

function ready(lib: string, kind: string): boolean {
  if (lib === "uplot") return !!document.querySelector(".uplot canvas");
  if (kind === "line") return !!document.querySelector(".recharts-line-curve")?.getAttribute("d");
  return document.querySelectorAll(".recharts-bar-rectangle").length > 0;
}

const surfaceWidth = (lib: string) => lib === "uplot"
  ? (document.querySelector(".uplot canvas") as HTMLCanvasElement | null)?.getBoundingClientRect().width ?? 0
  : Number(document.querySelector(".recharts-surface")?.getAttribute("width") ?? 0);

export async function run({ root, params }: Ctx) {
  const v = params.get("v") ?? "line-opt";
  const kind = v.includes("bar") ? "bar" : "line";
  const lib = v.startsWith("uplot") ? "uplot" : "recharts";
  const opt = v !== "line" && v !== "bar";
  const nPoints = Number(params.get("n") ?? 10000);

  let t = performance.now();
  const raw = await call("Chart", nPoints);
  const fetchMs = performance.now() - t;
  const bytes = JSON.stringify(raw).length;
  let data: P[] = raw.x.map((x: number, i: number) => ({ x, y: raw.y[i] }));
  t = performance.now();
  const m = /(lttb|agg)(\d+)/.exec(v);
  if (m) data = m[1] === "lttb" ? lttb(data, Number(m[2])) : bucketMean(data, Number(m[2]));
  const downsampleMs = performance.now() - t;

  let setW: (w: number) => void = () => {};
  function Frame() {
    const ref = useRef<HTMLDivElement>(null);
    useLayoutEffect(() => { setW = (w) => { if (ref.current) ref.current.style.width = w + "px"; }; });
    return (
      <div className="p-4">
        <div ref={ref} style={{ width: 1200, height: 520 }} className="rounded-lg border p-2">
          {lib === "uplot" ? <UPlotView data={data} kind={kind} /> : <RechartsView data={data} kind={kind} opt={opt} />}
        </div>
      </div>
    );
  }

  await afterPaint();
  // first render
  const fr0 = new FrameRecorder();
  const t0 = performance.now();
  commitAt = 0;
  let firstPaint = NaN;
  await phase("mount", async () => {
    fr0.start();
    root.render(<Shell title={`chart ${v} (${data.length} points)`}><Frame /></Shell>);
    const deadline = t0 + 30000;
    while (!ready(lib, kind) && performance.now() < deadline) await nextFrame();
    firstPaint = (await afterPaint()) - t0;
    await sleep(Math.max(0, 3000 - (performance.now() - t0))); // cover the default 1.5 s animation
  });
  const mountFrames = fr0.stop();
  const mountBlocking = blockingIn(t0, performance.now());
  const svgNodes = document.querySelectorAll(".recharts-wrapper *").length;

  // resize
  const resize: number[] = [];
  let resizeMisses = 0;
  const tR = performance.now();
  await phase("resize", async () => {
    for (const w of [800, 1200, 800, 1200, 900, 1200]) {
      const before = surfaceWidth(lib);
      const s = performance.now();
      setW(w);
      const dl = s + 5000;
      while (surfaceWidth(lib) === before && performance.now() < dl) await nextFrame();
      if (surfaceWidth(lib) === before) resizeMisses++;
      resize.push((await afterPaint()) - s);
      await sleep(300);
    }
  });
  const resizeBlocking = blockingIn(tR, performance.now());

  // hover: one move, wait for the tooltip/legend to change, then the next paint
  const target = (lib === "uplot" ? document.querySelector(".u-over") : document.querySelector(".recharts-wrapper")) as HTMLElement;
  const tipText = () => (lib === "uplot" ? document.querySelector(".u-legend")?.textContent : document.querySelector(".recharts-tooltip-wrapper")?.textContent) ?? "";
  const rect = target.getBoundingClientRect();
  const move = (fx: number) => {
    const x = rect.left + 70 + fx * (rect.width - 110), y = rect.top + rect.height / 2;
    target.dispatchEvent(new MouseEvent("mousemove", { clientX: x, clientY: y, bubbles: true }));
    target.dispatchEvent(new PointerEvent("pointermove", { clientX: x, clientY: y, bubbles: true }));
  };
  const hover: number[] = [];
  let hoverMisses = 0;
  const tH = performance.now();
  await phase("hover", async () => {
    target.dispatchEvent(new MouseEvent("mouseenter", { bubbles: false }));
    for (let k = 0; k < 40; k++) {
      const before = tipText();
      const s = performance.now();
      move((k * 7919 % 40 + 0.5) / 40);
      let f = 0;
      while (tipText() === before && f++ < 60) await nextFrame();
      if (tipText() === before) hoverMisses++;
      hover.push((await afterPaint()) - s);
      await sleep(50);
    }
  });
  const hoverBlocking = blockingIn(tH, performance.now());
  // sweep: one move per frame, like dragging the mouse across
  const frS = new FrameRecorder();
  const tS = performance.now();
  frS.start();
  for (let k = 0; k < 120; k++) { move(k / 120); await nextFrame(); }
  const sweepFrames = frS.stop();
  const sweepBlocking = blockingIn(tS, performance.now());

  return {
    variant: v, lib, kind, points: data.length, sourcePoints: nPoints,
    fetch: { ms: r1(fetchMs), bytes }, downsampleMs: r1(downsampleMs),
    mount: { commitMs: r1(commitAt - t0), firstPaintMs: r1(firstPaint), frames3s: mountFrames, blocking3s: mountBlocking, domNodes: svgNodes },
    resize: { ms: summary(resize), misses: resizeMisses, blocking: resizeBlocking },
    hover: { ms: summary(hover), misses: hoverMisses, blocking: hoverBlocking, sweepFrames, sweepBlocking },
  };
}
