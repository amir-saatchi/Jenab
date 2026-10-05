import * as React from "react"

// Below about 900 px the rail and both sidebars open as overlays (SPEC 5.12).
// The name stays as shadcn's sidebar imports it.
export const NARROW_WIDTH = 900

const query = `(max-width: ${NARROW_WIDTH - 1}px)`

function subscribe(onChange: () => void) {
  const mql = window.matchMedia(query)
  mql.addEventListener("change", onChange)
  return () => mql.removeEventListener("change", onChange)
}

export function useIsMobile() {
  return React.useSyncExternalStore(subscribe, () => window.matchMedia(query).matches)
}
