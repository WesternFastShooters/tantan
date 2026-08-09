import * as React from "react"
import { act } from "react"
import type { Root } from "react-dom/client"
import { createRoot } from "react-dom/client"
import { afterEach, beforeAll, describe, expect, test, vi } from "vitest"

import { TimelineLayout } from "./TimelineLayout"

const { navigateMock, routeState, useMobileMock, useShowEntryDetailsColumnMock } = vi.hoisted(
  () => ({
    navigateMock: vi.fn(),
    routeState: { entryId: null as string | null, view: 0 },
    useMobileMock: vi.fn(),
    useShowEntryDetailsColumnMock: vi.fn(),
  }),
)

vi.mock("@follow/components/hooks/useMobile.js", () => ({
  useMobile: useMobileMock,
}))

vi.mock("motion/react", () => ({
  AnimatePresence: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

vi.mock("~/components/common/Motion", () => ({
  m: {
    div: ({ children, ...props }: React.ComponentProps<"div">) => <div {...props}>{children}</div>,
  },
}))

vi.mock("~/hooks/biz/useNavigateEntry", () => ({
  useNavigateEntry: () => navigateMock,
}))

vi.mock("~/hooks/biz/useRouteParams", () => ({
  useRouteParamsSelector: (selector: (state: typeof routeState) => unknown) => selector(routeState),
}))

vi.mock("~/hooks/biz/useShowEntryDetailsColumn", () => ({
  useShowEntryDetailsColumn: useShowEntryDetailsColumnMock,
}))

vi.mock("~/modules/entry-column", () => ({
  EntryColumn: () => <div data-testid="entry-column">Entries</div>,
}))

vi.mock("~/modules/entry-content/components/entry-content", () => ({
  EntryContent: ({ entryId }: { entryId: string }) => (
    <article data-testid="entry-content">Entry {entryId}</article>
  ),
}))

vi.mock("~/modules/entry-content/components/entry-header", () => ({
  AIEntryHeader: ({ entryId }: { entryId: string }) => (
    <header data-testid="entry-header">Header {entryId}</header>
  ),
}))

vi.mock("~/modules/app-layout/entry-content/EntryContentPlaceholder", () => ({
  EntryContentPlaceholder: () => <div data-testid="entry-placeholder">Choose an entry</div>,
}))

vi.mock("~/providers/app-grid-layout-container-provider", () => ({
  AppLayoutGridContainerProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

const renderLayout = async () => {
  const container = document.createElement("div")
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(<TimelineLayout />)
  })

  return { container, root }
}

describe("TimelineLayout", () => {
  let container: HTMLElement | null = null
  let root: Root | null = null

  beforeAll(() => {
    ;(globalThis as typeof globalThis & { React: typeof React }).React = React
    ;(
      globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true
  })

  afterEach(async () => {
    if (root) {
      await act(async () => root?.unmount())
    }
    container?.remove()
    container = null
    root = null
    routeState.entryId = null
    routeState.view = 0
    vi.clearAllMocks()
  })

  test("REQ:FE-01 keeps the desktop RSS list and detail columns", async () => {
    useMobileMock.mockReturnValue(false)
    useShowEntryDetailsColumnMock.mockReturnValue(true)
    routeState.entryId = "entry-1"
    ;({ container, root } = await renderLayout())

    expect(container.querySelector('[data-testid="entry-column"]')).not.toBeNull()
    expect(container.querySelector('[data-testid="entry-header"]')?.textContent).toContain(
      "entry-1",
    )
    expect(container.querySelector('[data-testid="entry-content"]')?.textContent).toContain(
      "entry-1",
    )
  })

  test("REQ:FE-01 keeps mobile entry navigation and returns to its RSS list", async () => {
    useMobileMock.mockReturnValue(true)
    routeState.entryId = "entry-mobile"
    routeState.view = 2
    ;({ container, root } = await renderLayout())

    const backButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="Back to entries"]',
    )
    expect(backButton).not.toBeNull()
    expect(container.querySelector('[data-testid="entry-content"]')?.textContent).toContain(
      "entry-mobile",
    )

    await act(async () => backButton?.click())

    expect(navigateMock).toHaveBeenCalledWith({ entryId: null, view: 2 })
  })
})
