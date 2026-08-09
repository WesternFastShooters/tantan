import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import * as React from "react"
import { act } from "react"
import type { Root } from "react-dom/client"
import { createRoot } from "react-dom/client"
import { afterEach, beforeAll, describe, expect, test, vi } from "vitest"

import { AIProviderForm } from "./AIProviderForm"

const providerAPI = vi.hoisted(() => ({
  get: vi.fn(),
  test: vi.fn(),
}))

vi.mock("./api", () => ({
  getAIProvider: providerAPI.get,
  testAIProvider: providerAPI.test,
}))

const renderForm = async () => {
  const container = document.createElement("div")
  document.body.append(container)
  const root = createRoot(container)
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  await act(async () => {
    root.render(
      <QueryClientProvider client={queryClient}>
        <AIProviderForm />
      </QueryClientProvider>,
    )
  })
  await act(async () => new Promise((resolve) => setTimeout(resolve, 0)))
  return { container, root, queryClient }
}

describe("AIProviderForm", () => {
  let container: HTMLElement | null = null
  let root: Root | null = null
  let queryClient: QueryClient | null = null

  beforeAll(() => {
    ;(globalThis as typeof globalThis & { React: typeof React }).React = React
    ;(
      globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true
  })

  afterEach(async () => {
    if (root) await act(async () => root?.unmount())
    queryClient?.clear()
    container?.remove()
    container = null
    root = null
    queryClient = null
    vi.clearAllMocks()
  })

  test("SEC-05 exposes only the fixed server-owned Gemini preset and no browser Key controls", async () => {
    providerAPI.get.mockResolvedValue({
      configured: true,
      providerId: "google-gemini-openai",
      model: "gemini-3.5-flash-lite",
      baseUrl: "https://generativelanguage.googleapis.com/v1beta/openai",
      hasApiKey: true,
      keyFingerprint: null,
    })
    ;({ container, root, queryClient } = await renderForm())

    expect(container.textContent).toContain("Google Gemini")
    expect(container.textContent).toContain("gemini-3.5-flash-lite")
    expect(container.textContent).toContain("由 Go 服务端私密配置")
    expect(container.querySelector('input[type="password"]')).toBeNull()
    expect(container.querySelector("select")).toBeNull()
    expect(container.textContent).not.toContain("保存配置")
    expect(container.textContent).not.toContain("删除配置")
  })

  test("SEC-05 tests the server preset without sending provider, endpoint, model or Key", async () => {
    providerAPI.get.mockResolvedValue({
      configured: true,
      providerId: "google-gemini-openai",
      model: "gemini-3.5-flash-lite",
      baseUrl: "https://generativelanguage.googleapis.com/v1beta/openai",
      hasApiKey: true,
      keyFingerprint: null,
    })
    providerAPI.test.mockResolvedValue({ ok: true, latencyMs: 73, model: "gemini-3.5-flash-lite" })
    ;({ container, root, queryClient } = await renderForm())

    const testButton = Array.from(container.querySelectorAll("button")).find((button) =>
      button.textContent?.includes("测试连接"),
    )
    expect(testButton).toBeDefined()
    await act(async () => testButton?.click())

    expect(providerAPI.test).toHaveBeenCalledTimes(1)
    expect(providerAPI.test.mock.calls[0]?.[0]).toBeUndefined()
    expect(container.textContent).toContain("连接成功")
  })
})
