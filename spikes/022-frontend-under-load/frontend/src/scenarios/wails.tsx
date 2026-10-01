// Q5: Wails transport. Binding round trips and payloads, rows via event,
// event flood at rising rates (ordering, loss, latency, backpressure), and
// the same flood over a GoStream for comparison.
import type { Ctx } from "../main";
import { Shell } from "../shell";
import { afterPaint, blockingIn, epoch, FrameRecorder, phase, r1, sleep, summary } from "../lib/metrics";
import { call, jsonStream, on } from "../lib/wails";

async function timeCalls(n: number, fn: () => Promise<any>) {
  const ms: number[] = [];
  let bytes = 0;
  for (let i = 0; i < n; i++) {
    const t = performance.now();
    const r = await fn();
    ms.push(performance.now() - t);
    if (i === 0) bytes = JSON.stringify(r).length;
  }
  return { ms: summary(ms), bytes };
}

function analyse(seqs: number[], lat: number[], recvAt: number[], n: number, end: any) {
  let outOfOrder = 0, dupes = 0;
  const seen = new Set<number>();
  for (let i = 0; i < seqs.length; i++) {
    if (seen.has(seqs[i])) dupes++;
    seen.add(seqs[i]);
    if (i > 0 && seqs[i] < seqs[i - 1]) outOfOrder++;
  }
  const first = recvAt[0] ?? NaN, last = recvAt[recvAt.length - 1] ?? NaN;
  return {
    sent: n, received: seqs.length, missing: n - seen.size, dupes, outOfOrder,
    goAchievedHz: end ? r1(end.achievedHz) : null,
    recvHz: r1((seqs.length * 1000) / Math.max(1, last - first)),
    latencyMs: summary(lat),
    drainLagMs: end ? r1(last - end.endEpoch) : null, // last receive after last emit
    goEmitTotalMs: end ? r1(end.emitTotalMs) : null,
    goEmitMaxMs: end ? r2(end.emitMaxMs) : null,
    goPacingLateMaxMs: end ? r1(end.lateMaxMs) : null,
  };
}
const r2 = (x: number) => Math.round(x * 100) / 100;

export async function run({ root, params }: Ctx) {
  root.render(<Shell title="wails transport" />);
  await afterPaint();
  const quick = params.get("quick") === "1";

  // bindings
  const bindings: Record<string, any> = {};
  await phase("bindings", async () => {
    bindings.noop = await timeCalls(300, () => call("Noop"));
    bindings.echo1k = await timeCalls(100, () => call("Echo", 1000));
    bindings.rows500arr = await timeCalls(30, () => call("GetRows", 0, 500));
    bindings.rows500obj = await timeCalls(30, () => call("GetRowsObj", 0, 500));
    bindings.rows5000arr = await timeCalls(5, () => call("GetRows", 0, 5000));
    bindings.history200 = await timeCalls(5, () => call("History", 200));
    bindings.chart10k = await timeCalls(5, () => call("Chart", 10000));
    const costs = [];
    for (let i = 0; i < 10; i++) costs.push(await call("RowsCost", 0, 500));
    bindings.rows500goSide = { genMs: summary(costs.map((c: any) => c.genMs)), marshalMs: summary(costs.map((c: any) => c.marshalMs)), bytes: costs[0].bytes };
    // 10 concurrent 500-row calls (a view loading several pages at once)
    const t = performance.now();
    await Promise.all(Array.from({ length: 10 }, (_, i) => call("GetRows", i * 500, 500)));
    bindings.rows500x10concurrentMs = r1(performance.now() - t);
  });

  // 500 rows delivered as an event (payload > 8 KB goes by reference + fetch in Wails)
  const evMs: number[] = [];
  let evBytes = 0;
  await phase("rowsEvent", async () => {
    for (let i = 0; i < 20; i++) {
      const t = performance.now();
      const got = new Promise<any>((res) => { const off = on("rows", (d) => { off(); res(d); }); });
      await call("RowsViaEvent", 0, 500);
      const d = await got;
      evMs.push(performance.now() - t);
      if (i === 0) evBytes = JSON.stringify(d).length;
    }
  });

  // event flood
  const rates = quick ? [1000, 5000, 0] : [100, 1000, 2000, 5000, 10000, 20000, 0];
  const floods: Record<string, any> = {};
  for (const rate of rates) {
    const n = rate === 0 ? 20000 : Math.max(200, rate * 2);
    const seqs: number[] = [], lat: number[] = [], recvAt: number[] = [];
    let end: any = null;
    const offF = on("fl", (d) => { const now = epoch(); seqs.push(d.i); lat.push(now - d.ts); recvAt.push(now); });
    const offE = on("fl-end", (d) => { end = d; });
    const fr = new FrameRecorder();
    const t0 = performance.now();
    await phase("flood" + rate, async () => {
      fr.start();
      await call("Flood", "flood" + rate, rate, n);
      const dl = performance.now() + 60000;
      while ((!end || seqs.length < n) && performance.now() < dl) await sleep(20);
      await sleep(300);
    });
    offF(); offE();
    floods[rate === 0 ? "max" : String(rate)] = { ...analyse(seqs, lat, recvAt, n, end), frames: fr.stop(), blocking: blockingIn(t0, performance.now()) };
    await sleep(300);
  }

  // same over a GoStream
  const streams: Record<string, any> = {};
  try {
    const s = jsonStream("flood");
    await new Promise<void>((res, rej) => { s.onopen = () => res(); s.onerror = (e: any) => rej(e); setTimeout(() => rej(new Error("stream open timeout")), 5000); });
    for (const rate of quick ? [1000, 0] : [1000, 5000, 20000, 0]) {
      const n = rate === 0 ? 20000 : Math.max(200, rate * 2);
      const seqs: number[] = [], lat: number[] = [], recvAt: number[] = [];
      let end: any = null;
      s.onmessage = (ev: MessageEvent) => {
        const d = ev.data;
        if (d.end) { end = d.end; return; }
        const now = epoch(); seqs.push(d.i); lat.push(now - d.ts); recvAt.push(now);
      };
      const fr = new FrameRecorder();
      await phase("stream" + rate, async () => {
        fr.start();
        s.send({ id: "stream" + rate, rate, n });
        const dl = performance.now() + 60000;
        while ((!end || seqs.length < n) && performance.now() < dl) await sleep(20);
        await sleep(300);
      });
      streams[rate === 0 ? "max" : String(rate)] = { ...analyse(seqs, lat, recvAt, n, end), frames: fr.stop() };
    }
    s.close();
  } catch (e: any) {
    streams.error = String(e?.message ?? e);
  }

  return { bindings, rowsEvent: { ms: summary(evMs), bytes: evBytes }, floods, streams };
}
