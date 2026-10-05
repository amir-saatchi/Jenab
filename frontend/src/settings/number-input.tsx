import * as React from "react"

import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

// NumberInput edits a whole number and commits it on Enter or when it
// loses focus. A value below min goes back. With optional, an empty field
// commits null, e.g. to use a default again.
export function NumberInput({
  value,
  min,
  optional,
  placeholder,
  onCommit,
  className,
  ...props
}: {
  value: number | null
  min: number
  optional?: boolean
  placeholder?: string
  onCommit: (v: number | null) => Promise<void> | void
  className?: string
} & Omit<React.ComponentProps<"input">, "value" | "onChange" | "min">) {
  const shown = value === null ? "" : String(value)
  const [text, setText] = React.useState(shown)
  const [bad, setBad] = React.useState(false)
  React.useEffect(() => setText(shown), [shown])

  const commit = () => {
    const t = text.trim()
    if (t === shown) return
    if (t === "" && optional) {
      setBad(false)
      void onCommit(null)
      return
    }
    const n = Number(t)
    if (!Number.isInteger(n) || n < min) {
      setBad(true)
      setText(shown)
      return
    }
    setBad(false)
    void onCommit(n)
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
      title={bad ? `At least ${min}` : props.title}
      className={cn("w-24 text-end tabular-nums", className)}
      onChange={(e) => setText(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === "Enter") commit()
        if (e.key === "Escape") setText(shown)
      }}
    />
  )
}
