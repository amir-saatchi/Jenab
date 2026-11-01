import { expect, test } from "bun:test"

import { ChatExportFormat } from "@/lib/api"
import { chatExportPath, chatFileName, formatText } from "@/shell/export-chat"

test("chatFileName", () => {
  expect(chatFileName("BTC prices", ChatExportFormat.FormatMarkdown)).toBe("BTC prices.md")
  expect(chatFileName("قیمت بیت‌کوین", ChatExportFormat.FormatJSON)).toBe("قیمت بیت‌کوین.json")
  expect(chatFileName('a/b:c*"d"?', ChatExportFormat.FormatMarkdown)).toBe("a-b-c-d.md")
  expect(chatFileName("", ChatExportFormat.FormatMarkdown)).toBe("chat.md")
})

test("chatExportPath", () => {
  expect(chatExportPath("C:\\Users\\me\\chat", ChatExportFormat.FormatMarkdown)).toBe("C:\\Users\\me\\chat.md")
  expect(chatExportPath("C:\\Users\\me\\chat.MD", ChatExportFormat.FormatMarkdown)).toBe("C:\\Users\\me\\chat.MD")
  expect(chatExportPath("/home/me/chat.md", ChatExportFormat.FormatJSON)).toBe("/home/me/chat.md.json")
})

test("formatText", () => {
  expect(formatText(ChatExportFormat.FormatJSON)).toContain("as stored")
  expect(formatText(ChatExportFormat.FormatMarkdown)).toContain("to read")
})
