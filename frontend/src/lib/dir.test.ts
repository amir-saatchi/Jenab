import { expect, test } from "bun:test"

import { textDir } from "@/lib/dir"

test("the first strong character sets the direction", () => {
  expect(textDir("سلام, hello")).toBe("rtl")
  expect(textDir("Hello سلام")).toBe("ltr")
  expect(textDir("۸ صبح")).toBe("rtl")
  expect(textDir("۶۴ BTC")).toBe("ltr")
  expect(textDir("12 %")).toBe("ltr")
})
