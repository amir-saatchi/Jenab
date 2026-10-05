// The direction of the first strong character, as dir="auto" finds it.
const rtl = /[\u0590-\u08ff\ufb1d-\ufdff\ufe70-\ufeff]/
const strong = /[A-Za-z\u00c0-\u024f\u0370-\u03ff\u0400-\u04ff\u0590-\u08ff\ufb1d-\ufdff\ufe70-\ufeff]/

// Arabic and Persian digits are weak, like Latin ones.
const digits = /[\u0660-\u0669\u06f0-\u06f9]/g

export function textDir(s: string): "rtl" | "ltr" {
  const c = s.replace(digits, "").match(strong)?.[0]
  return c && rtl.test(c) ? "rtl" : "ltr"
}
