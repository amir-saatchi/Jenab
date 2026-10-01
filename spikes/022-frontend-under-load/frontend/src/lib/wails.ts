// Loads the Wails runtime that the Go asset server serves at /wails/runtime.js
// (kept out of the bundle so it always matches the Go side), and wraps
// name-based binding calls to the Spike service.
/* eslint-disable @typescript-eslint/no-explicit-any */
let rt: any;

export async function loadRuntime(): Promise<any> {
  if (!rt) {
    const url = "/wails/runtime.js";
    rt = await import(/* @vite-ignore */ url);
  }
  return rt;
}

export function call<T = any>(method: string, ...args: any[]): Promise<T> {
  return rt.Call.ByName("main.Spike." + method, ...args);
}

export function on(name: string, cb: (data: any) => void): () => void {
  return rt.Events.On(name, (ev: any) => cb(ev.data));
}

export function jsonStream(name: string): any {
  return rt.JSONStream(name);
}

export function log(msg: string) {
  try { call("Log", msg); } catch { /* runtime not ready */ }
}
