import { readFileSync } from "node:fs"
import { fileURLToPath } from "node:url"

import { dirname, join } from "pathe"
import { describe, expect, test } from "vitest"

const moduleRoot = dirname(fileURLToPath(import.meta.url))

describe("Tantan Provider secret boundary", () => {
  test("REQ:FE-05 production Provider modules never write browser persistence", () => {
    const sources = ["AIProviderForm.tsx", "api.ts"]
      .map((file) => readFileSync(join(moduleRoot, file), "utf8"))
      .join("\n")

    expect(sources).not.toMatch(/localStorage|sessionStorage|indexedDB|caches\.open/)
    expect(sources).not.toMatch(/apiKey.*(?:URLSearchParams|searchParams|location)/s)
    expect(sources).not.toContain('type="password"')
    expect(sources).not.toContain("saveAIProvider")
    expect(sources).not.toContain("deleteAIProvider")
    expect(sources).toContain("浏览器不能查看、提交、修改或删除 API Key")
  })
})
