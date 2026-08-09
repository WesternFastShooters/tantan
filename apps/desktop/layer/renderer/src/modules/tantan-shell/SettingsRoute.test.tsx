import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import * as React from "react"
import { act } from "react"
import type { Root } from "react-dom/client"
import { createRoot } from "react-dom/client"
import { MemoryRouter, Route, Routes } from "react-router"
import { afterEach, beforeAll, describe, expect, test, vi } from "vitest"

import { SettingsRoute } from "./SettingsRoute"

const sessionAPI = vi.hoisted(() => ({
  get: vi.fn(),
  logout: vi.fn(),
}))

vi.mock("~/lib/tantan-api/client", () => ({
  getLocalSession: sessionAPI.get,
  logoutTantan: sessionAPI.logout,
}))

describe("SettingsRoute account boundary", () => {
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

  test("FR-02 shows the Folo account and logs out through the same-origin Go session", async () => {
    sessionAPI.get.mockResolvedValue({
      user: {
        id: "user-1",
        name: "Mingrui",
        email: "mingrui@example.com",
        image: null,
      },
      timezone: "Asia/Shanghai",
      csrfToken: "csrf-memory-only",
    })
    sessionAPI.logout.mockResolvedValue(undefined)
    container = document.createElement("div")
    document.body.append(container)
    root = createRoot(container)
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    await act(async () => {
      root?.render(
        <QueryClientProvider client={queryClient!}>
          <MemoryRouter initialEntries={["/settings"]}>
            <Routes>
              <Route path="/settings" element={<SettingsRoute />} />
              <Route path="/login" element={<h1>Login destination</h1>} />
            </Routes>
          </MemoryRouter>
        </QueryClientProvider>,
      )
    })
    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)))

    expect(container.textContent).toContain("Mingrui")
    expect(container.textContent).toContain("mingrui@example.com")
    expect(container.querySelectorAll("[data-settings-group]").length).toBeGreaterThanOrEqual(2)
    expect(container.textContent).not.toMatch(/Plan|Power|Wallet|升级|额度|会员/u)
    expect(container.querySelector<HTMLAnchorElement>('a[href="/settings/general"]')).not.toBeNull()
    expect(
      container.querySelector<HTMLAnchorElement>('a[href="/settings/appearance"]'),
    ).not.toBeNull()
    const logout = Array.from(container.querySelectorAll("button")).find((button) =>
      button.textContent?.includes("退出登录"),
    )
    expect(logout).toBeDefined()
    await act(async () => logout?.click())

    expect(sessionAPI.logout).toHaveBeenCalledTimes(1)
    expect(container.textContent).toContain("Login destination")
  })
})
