import { describe, expect, test } from "vitest"

import { buildProviderSaveRequest, PROVIDER_PRESETS, validateProviderDraft } from "./provider-form"

describe("Tantan AI Provider form", () => {
  test("REQ:FE-05 only exposes locked provider endpoints", () => {
    expect(Object.keys(PROVIDER_PRESETS)).toEqual([
      "openai",
      "anthropic",
      "google",
      "deepseek",
      "openrouter",
    ])
    expect(
      Object.values(PROVIDER_PRESETS).every(({ baseUrl }) => baseUrl.startsWith("https://")),
    ).toBe(true)
  })

  test("REQ:FE-05 omits a blank existing key and validates a new key in memory", () => {
    const existing = { providerId: "openai" as const, model: "gpt-5", apiKey: "" }
    expect(validateProviderDraft(existing, true)).toBeNull()
    expect(buildProviderSaveRequest(existing)).toEqual({ providerId: "openai", model: "gpt-5" })

    expect(validateProviderDraft({ ...existing, apiKey: "short" }, false)).toBe(
      "API Key 至少需要 8 个字符",
    )
    expect(buildProviderSaveRequest({ ...existing, apiKey: "secret-key" })).toEqual({
      providerId: "openai",
      model: "gpt-5",
      apiKey: "secret-key",
    })
  })
})
