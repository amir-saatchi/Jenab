import "./index.css";
import { createRoot } from "react-dom/client";
import { Shell } from "./shell";
import { afterPaint, blockingIn, eventTimings, loafs, longTasks, observeAll, phases, r1 } from "./lib/metrics";
import { call, loadRuntime, log } from "./lib/wails";

const params = new URLSearchParams(location.search);
const scenario = params.get("s") ?? "idle";
observeAll();

const errors: string[] = [];
window.addEventListener("error", (e) => { errors.push(String(e.message)); log("error: " + e.message); });
window.addEventListener("unhandledrejection", (e) => { errors.push(String(e.reason)); log("rejection: " + String(e.reason?.stack ?? e.reason)); });
const visibility: { at: number; state: string }[] = [{ at: performance.now(), state: document.visibilityState }];
document.addEventListener("visibilitychange", () => visibility.push({ at: performance.now(), state: document.visibilityState }));

const scriptStart = performance.now();
const root = createRoot(document.getElementById("root")!);
root.render(<Shell title={scenario} />);

export type Ctx = { root: ReturnType<typeof createRoot>; params: URLSearchParams; info: any };

async function main() {
  const shellPaint = await afterPaint();
  await loadRuntime();
  const runtimeReady = performance.now();
  const info = await call("Hello");
  const helloDone = performance.now();
  // FCP is not reported when the page starts hidden (WebView2 starts the page before the window is visible)
  const fcp = performance.getEntriesByName("first-contentful-paint")[0]?.startTime ?? NaN;
  const firstVisible = visibility.find((v) => v.state === "visible")?.at ?? NaN;
  const nav = performance.getEntriesByType("navigation")[0] as PerformanceNavigationTiming | undefined;
  const startup = {
    procStartEpoch: info.procStartEpoch,
    timeOrigin: performance.timeOrigin,
    procToNavStartMs: r1(performance.timeOrigin - info.procStartEpoch),
    procToFcpMs: r1(performance.timeOrigin + fcp - info.procStartEpoch),
    procToShellPaintMs: r1(performance.timeOrigin + shellPaint - info.procStartEpoch),
    procToRuntimeReadyMs: r1(performance.timeOrigin + runtimeReady - info.procStartEpoch),
    procToMainMs: r1(info.mainStartEpoch - info.procStartEpoch),
    procToAppStartedMs: r1(info.winShownEpoch - info.procStartEpoch),
    navToFcpMs: r1(fcp),
    navToFirstVisibleMs: r1(firstVisible),
    procToFirstVisibleMs: r1(performance.timeOrigin + firstVisible - info.procStartEpoch),
    navToScriptMs: r1(scriptStart),
    domContentLoadedMs: r1(nav?.domContentLoadedEventEnd ?? NaN),
    firstBindingCallMs: r1(helloDone - runtimeReady),
    transferBytes: performance.getEntriesByType("resource").reduce((a, e: any) => a + (e.decodedBodySize || 0), 0) + (nav?.decodedBodySize ?? 0),
  };
  const memStart = await call("Mem");

  let mod: { run: (ctx: Ctx) => Promise<any> };
  switch (scenario) {
    case "chat": mod = await import("./scenarios/chat"); break;
    case "table": mod = await import("./scenarios/table"); break;
    case "chart": mod = await import("./scenarios/chart"); break;
    case "wails": mod = await import("./scenarios/wails"); break;
    default: mod = await import("./scenarios/idle");
  }
  let result: any;
  try {
    result = await mod.run({ root, params, info });
  } catch (e: any) {
    errors.push(String(e?.stack ?? e));
    result = { error: String(e?.stack ?? e) };
  }
  const page = {
    scenario,
    params: Object.fromEntries(params),
    startup,
    memStart: { webviewWorkingSetMB: memStart.webviewWorkingSetMB, webviewPrivateMB: memStart.webviewPrivateMB, totalWorkingSetMB: memStart.totalWorkingSetMB },
    visibility,
    hiddenAfterVisible: visibility.findIndex((v) => v.state === "visible") >= 0 && visibility.slice(visibility.findIndex((v) => v.state === "visible")).some((v) => v.state !== "visible"),
    errors,
    wholeRunBlocking: blockingIn(0, performance.now()),
    jsHeapMB: (performance as any).memory ? Math.round((performance as any).memory.usedJSHeapSize / 1048576) : null,
    domNodes: document.getElementsByTagName("*").length,
    supportedEntryTypes: (PerformanceObserver as any).supportedEntryTypes,
    totals: { longTasks: longTasks.length, loafs: loafs.length, eventTimings: eventTimings.length },
    devicePixelRatio: devicePixelRatio,
    viewport: [innerWidth, innerHeight],
    userAgent: navigator.userAgent,
    result,
  };
  await call("Report", JSON.stringify(page), phases);
}

main().catch((e) => {
  log("fatal: " + String(e?.stack ?? e));
  call("Report", JSON.stringify({ scenario, fatal: String(e?.stack ?? e), errors }), phases).catch(() => {});
});
