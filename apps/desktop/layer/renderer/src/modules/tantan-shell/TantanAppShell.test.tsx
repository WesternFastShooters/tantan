import * as React from "react"
import { act } from "react"
import type { Root } from "react-dom/client"
import { createRoot } from "react-dom/client"
import { MemoryRouter, Outlet, Route, Routes } from "react-router"
import { afterEach, beforeAll, describe, expect, test, vi } from "vitest"

import { TantanAppShell } from "./TantanAppShell"

const { useMobileMock } = vi.hoisted(() => ({ useMobileMock: vi.fn() }))

vi.mock("./useTantanMobile", () => ({ useTantanMobile: useMobileMock }))

vi.mock("~/modules/tantan-service-status/LocalServiceGuard", () => ({
  LocalServiceGuard: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

const renderShell = async (path: string) => {
  const container = document.createElement("div")
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route element={<TantanAppShell />}>
            <Route element={<div data-testid="route-content">Route content</div>} path="*" />
          </Route>
        </Routes>
        <Outlet />
      </MemoryRouter>,
    )
  })

  return { container, root }
}

describe("TantanAppShell", () => {
  let container: HTMLElement | null = null
  let root: Root | null = null

  beforeAll(() => {
    ;(globalThis as typeof globalThis & { React: typeof React }).React = React
    ;(
      globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true
  })

  afterEach(async () => {
    if (root) await act(async () => root?.unmount())
    container?.remove()
    container = null
    root = null
    vi.clearAllMocks()
  })

  test("REQ:FE-02 renders the shared three routes in desktop navigation", async () => {
    useMobileMock.mockReturnValue(false)
    ;({ container, root } = await renderShell("/"))

    const navigation = container.querySelector('nav[aria-label="Primary navigation"]')
    expect(navigation).not.toBeNull()
    expect(navigation?.querySelectorAll("a")).toHaveLength(3)
    expect(navigation?.textContent).toContain("首页")
    expect(navigation?.textContent).toContain("订阅")
    expect(navigation?.textContent).toContain("设置")
    expect(container.querySelector('nav[aria-label="Mobile navigation"]')).toBeNull()
    expect(container.querySelector('[data-testid="route-content"]')).not.toBeNull()
  })

  test("REQ:FE-02 renders the same three routes in mobile bottom navigation", async () => {
    useMobileMock.mockReturnValue(true)
    ;({ container, root } = await renderShell("/subscriptions"))

    const navigation = container.querySelector('nav[aria-label="Mobile navigation"]')
    expect(navigation).not.toBeNull()
    expect(navigation?.querySelectorAll("a")).toHaveLength(3)
    expect(navigation?.querySelector('a[aria-current="page"]')?.textContent).toContain("订阅")
    expect(container.querySelector('nav[aria-label="Primary navigation"]')).toBeNull()
  })

  test("REQ:FE-03 hides the mobile bottom navigation on Entry detail routes", async () => {
    useMobileMock.mockReturnValue(true)
    ;({ container, root } = await renderShell("/entries/41147805272531997"))

    expect(container.querySelector('nav[aria-label="Mobile navigation"]')).toBeNull()
    expect(container.querySelector('[data-testid="route-content"]')).not.toBeNull()
  })
})
