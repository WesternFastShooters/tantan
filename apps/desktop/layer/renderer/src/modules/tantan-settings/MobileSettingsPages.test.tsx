import * as React from "react"
import { act } from "react"
import type { Root } from "react-dom/client"
import { createRoot } from "react-dom/client"
import { MemoryRouter } from "react-router"
import { afterEach, beforeAll, describe, expect, test, vi } from "vitest"

import { AppearanceSettingsPage, GeneralSettingsPage } from "./MobileSettingsPages"

const general = vi.hoisted(() => ({
  values: {
    unreadOnly: false,
    scrollMarkUnread: true,
    autoGroup: true,
    hideAllReadSubscriptions: false,
    openLinksInExternalApp: false,
  } as Record<string, boolean>,
  set: vi.fn(),
}))

const ui = vi.hoisted(() => ({
  values: {
    uiTextSize: 16,
    contentLineHeight: 1.75,
    thumbnailRatio: "square",
    reduceMotion: false,
  } as Record<string, boolean | number | string>,
  set: vi.fn(),
}))

const theme = vi.hoisted(() => ({
  value: "system",
  set: vi.fn(),
}))

vi.mock("~/atoms/settings/general", () => ({
  useGeneralSettingKey: (key: string) => general.values[key],
  setGeneralSetting: general.set,
}))

vi.mock("~/atoms/settings/ui", () => ({
  useUISettingKey: (key: string) => ui.values[key],
  setUISetting: ui.set,
}))

vi.mock("@follow/hooks", () => ({
  useThemeAtomValue: () => theme.value,
}))

vi.mock("~/hooks/common", () => ({
  useSetTheme: () => theme.set,
}))

describe("mobile-safe settings", () => {
  let container: HTMLDivElement | null = null
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

  const render = async (node: React.ReactNode) => {
    container = document.createElement("div")
    document.body.append(container)
    root = createRoot(container)
    await act(async () => {
      root?.render(<MemoryRouter>{node}</MemoryRouter>)
    })
  }

  test("FR-18 exposes only local general preferences without paid gates", async () => {
    await render(<GeneralSettingsPage />)

    expect(container?.textContent).toContain("仅显示未读")
    expect(container?.textContent).toContain("滚动时标记已读")
    expect(container?.textContent).not.toMatch(/Plan|Power|Wallet|升级|额度|会员/u)

    const unreadToggle = container?.querySelector<HTMLButtonElement>(
      'button[aria-label="仅显示未读"]',
    )
    await act(async () => unreadToggle?.click())
    expect(general.set).toHaveBeenCalledWith("unreadOnly", true)
  })

  test("FR-19 changes theme and local reading appearance without remote sync", async () => {
    await render(<AppearanceSettingsPage />)

    const dark = container?.querySelector<HTMLButtonElement>('button[aria-label="深色主题"]')
    await act(async () => dark?.click())
    expect(theme.set).toHaveBeenCalledWith("dark")

    const larger = container?.querySelector<HTMLButtonElement>('button[aria-label="增大字号"]')
    await act(async () => larger?.click())
    expect(ui.set).toHaveBeenCalledWith("uiTextSize", 17)
    expect(container?.textContent).not.toMatch(/Plan|Power|Wallet|升级|额度|会员/u)
  })
})
