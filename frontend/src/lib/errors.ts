import { toast } from "sonner"

// UIError is what a failed Go call carries as its cause (app.UIError, Q25).
export interface UIError {
  kind: string
  message: string
  details?: string
}

// toUIError reads the UIError out of a rejected call. Anything else, such
// as a network or binding error, becomes an internal error with its text
// as the details.
export function toUIError(err: unknown): UIError {
  const cause = (err as { cause?: unknown } | null)?.cause
  if (isUIError(cause)) return cause
  if (isUIError(err)) return err
  return { kind: "internal", message: "Something went wrong.", details: String(err) }
}

function isUIError(v: unknown): v is UIError {
  return typeof v === "object" && v !== null && typeof (v as UIError).kind === "string" && typeof (v as UIError).message === "string"
}

// showError shows a failed call as a toast, with Copy details when there
// are details.
export function showError(err: unknown) {
  const e = toUIError(err)
  toast.error(e.message, {
    action: e.details
      ? { label: "Copy details", onClick: () => void navigator.clipboard.writeText(e.details ?? "") }
      : undefined,
  })
}
