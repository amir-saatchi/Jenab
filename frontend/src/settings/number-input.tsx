import * as React from "react"

import { Input } from "@/components/ui/input"
import { showError } from "@/lib/errors"
import { cn } from "@/lib/utils"
import { readNumber } from "@/settings/format"

// NumberInput edits a whole number and commits it on Enter or when it
// loses focus. A value below min goes back. With optional, an empty field
// commits null, e.g. to use a default again. When onCommit fails, the error
// shows, the field goes back to the value shown and is marked invalid;
// onInvalidChange tells the Field around it, for its data-invalid.
export function NumberInput({
  value,
  min,
  optional,
  placeholder,
  onCommit,
  onInvalidChange,
  className,
  ...props
}: {
  value: number | null
  min: number
  optional?: boolean
  placeholder?: string
  onCommit: (v: number | null) => Promise<void> | void
  onInvalidChange?: (invalid: boolean) => void
  className?: string
} & Omit<React.ComponentProps<"input">, "value" | "onChange" | "min">) {
  const shown = value === null ? "" : String(value)
  const [text, setText] = React.useState(shown)
  // bad is why the field is invalid: a value below min, or a failed save.
  const [bad, setBad] = React.useState<"min" | "save" | null>(null)
  const latest = React.useRef(shown)
  React.useEffect(() => {
    latest.current = shown
    setText(shown)
  }, [shown])

  const mark = (why: "min" | "save" | null) => {
    setBad(why)
    onInvalidChange?.(why !== null)
  }

  const commit = async () => {
    const r = readNumber(text, shown, min, optional)
    if (r === "same") return
    if (r === "bad") {
      mark("min")
      setText(shown)
      return
    }
    mark(null)
    try {
      await onCommit(r.value)
    } catch (err) {
      mark("save")
      setText(latest.current)
      showError(err)
    }
  }

  return (
    <Input
      {...props}
      type="number"
      inputMode="numeric"
      min={min}
      step={1}
      dir="ltr"
      value={text}
      placeholder={placeholder}
      aria-invalid={bad ? true : undefined}
      title={bad === "min" ? `At least ${min}` : bad === "save" ? "Not saved" : props.title}
      className={cn("w-24 text-end tabular-nums", className)}
      onChange={(e) => setText(e.target.value)}
      onBlur={() => void commit()}
      onKeyDown={(e) => {
        if (e.key === "Enter") void commit()
        if (e.key === "Escape") setText(shown)
      }}
    />
  )
}
