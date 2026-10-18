import { expect, test } from "bun:test"

import { ExportFormat, ExportStep, type ExportProgress } from "@/lib/api"
import { exportPath, fileName, percent, stepText } from "@/shell/export-project"

test("fileName", () => {
  expect(fileName("BTC prices", ExportFormat.ExportZip)).toBe("BTC prices.zip")
  expect(fileName("قیمت‌ها", ExportFormat.ExportFolder)).toBe("قیمت‌ها")
  expect(fileName('a/b:c*"d"?', ExportFormat.ExportFolder)).toBe("a-b-c-d")
  expect(fileName(" ..", ExportFormat.ExportZip)).toBe("project.zip")
})

test("exportPath", () => {
  expect(exportPath("C:\\Users\\me\\BTC", ExportFormat.ExportZip)).toBe("C:\\Users\\me\\BTC.zip")
  expect(exportPath("C:\\Users\\me\\BTC.ZIP", ExportFormat.ExportZip)).toBe("C:\\Users\\me\\BTC.ZIP")
  expect(exportPath("/home/me/BTC", ExportFormat.ExportFolder)).toBe("/home/me/BTC")
})

test("percent and stepText", () => {
  const p = (done: number, total: number) => ({ project: "p", target: "t", step: ExportStep.StepObjects, done, total }) as ExportProgress
  expect(percent(null)).toBe(0)
  expect(percent(p(5, 0))).toBe(0)
  expect(percent(p(25, 100))).toBe(25)
  expect(percent(p(200, 100))).toBe(100)
  expect(stepText(ExportStep.StepChats)).toBe("Copying the chats…")
  expect(stepText(undefined)).toBe("Starting…")
})
