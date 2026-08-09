import * as React from "react"
import { act } from "react"
import type { Root } from "react-dom/client"
import { createRoot } from "react-dom/client"
import { MemoryRouter, Outlet, Route, Routes } from "react-router"
import { afterEach, beforeAll, describe, expect, test, vi } from "vitest"

import { TantanAppShell } from "./TantanAppShell"

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

  test("FR-01 renders exactly four Folo Mobile tabs and never a desktop sidebar", async () => {
    ;({ container, root } = await renderShell("/"))

    const navigation = container.querySelector('nav[role="tablist"][aria-label="主导航"]')
    expect(navigation).not.toBeNull()
    expect(navigation?.querySelectorAll('[role="tab"]')).toHaveLength(4)
    expect(navigation?.textContent).toContain("首页")
    expect(navigation?.textContent).toContain("订阅")
    expect(navigation?.textContent).toContain("发现")
    expect(navigation?.textContent).toContain("设置")
    expect(container.querySelector('nav[aria-label="Primary navigation"]')).toBeNull()
    expect(container.querySelector("aside")).toBeNull()
    expect(container.querySelector('[data-testid="route-content"]')).not.toBeNull()
  })

  test("FR-01 marks the selected bottom tab and keeps all targets at least 44px", async () => {
    ;({ container, root } = await renderShell("/subscriptions"))

    const navigation = container.querySelector('nav[role="tablist"][aria-label="主导航"]')
    expect(navigation).not.toBeNull()
    expect(navigation?.querySelectorAll('[role="tab"]')).toHaveLength(4)
    const selected = navigation?.querySelector('[role="tab"][aria-selected="true"]')
    expect(selected?.textContent).toContain("订阅")
    expect(selected?.className).toContain("min-h-11")
    expect(navigation?.className).toContain("safe-area-inset-bottom")
    expect(navigation?.className).toContain("safe-area-inset-left")
  })

  test("FR-01 hides the bottom tabs on Entry detail stack routes", async () => {
    ;({ container, root } = await renderShell("/entries/41147805272531997"))

    expect(container.querySelector('nav[role="tablist"][aria-label="主导航"]')).toBeNull()
    expect(container.querySelector('[data-testid="route-content"]')).not.toBeNull()
  })

  test("FR-01 hides the bottom tabs on Source detail stack routes", async () => {
    ;({ container, root } = await renderShell("/sources/feed-1"))

    expect(container.querySelector('nav[role="tablist"][aria-label="主导航"]')).toBeNull()
  })
})
