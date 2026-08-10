import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import * as React from "react"
import { act } from "react"
import type { Root } from "react-dom/client"
import { createRoot } from "react-dom/client"
import { MemoryRouter, Route, Routes } from "react-router"
import { afterEach, beforeAll, describe, expect, test, vi } from "vitest"

import { EntryDetailPage } from "./EntryDetailPage"

vi.mock("@follow/store/collection/hooks", () => ({
  useIsEntryStarred: () => false,
}))

vi.mock("@follow/store/collection/store", () => ({
  collectionSyncService: { starEntry: vi.fn(), unstarEntry: vi.fn() },
}))

vi.mock("@follow/store/entry/hooks", () => ({
  useEntry: () => ({
    title: "Original title",
    description: "Original description",
    content: "Original content",
    url: "https://example.com/article",
    author: "Example author",
    publishedAt: new Date("2026-08-09T12:00:00Z"),
    read: true,
  }),
}))

vi.mock("@follow/store/unread/store", () => ({
  unreadSyncService: { markEntryAsRead: vi.fn().mockResolvedValue(undefined) },
}))

vi.mock("~/modules/tantan-home/home-cache", () => ({
  removeEntryFromAllHomeQueries: vi.fn(),
}))

vi.mock("./entry-api", () => ({
  getEntryCollectionStatus: vi.fn().mockResolvedValue(false),
  getEntryDetail: vi.fn().mockResolvedValue({
    title: "Original title",
    description: "Original description",
    content: "Original content",
    url: "https://example.com/article",
    author: "Example author",
    publishedAt: "2026-08-09T12:00:00Z",
    read: true,
  }),
  markEntryAsReadDirect: vi.fn().mockResolvedValue(undefined),
  updateEntryCollectionDirect: vi.fn().mockResolvedValue(undefined),
}))

vi.mock("./useEntryEnrichment", () => ({
  useEntryEnrichment: () => ({
    data: {
      state: "ready",
      data: {
        titleZh: "中文标题",
        contentZh: "中文正文",
        summaryZh: "中文摘要",
        keyPoints: [],
      },
    },
    ensuring: false,
    ensure: vi.fn(),
    ensureError: null,
    isError: false,
  }),
}))

describe("EntryDetailPage", () => {
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

  test("BUG:back-icon renders a visible vector glyph inside the accessible back button", async () => {
    container = document.createElement("div")
    document.body.append(container)
    root = createRoot(container)
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    })

    await act(async () => {
      root?.render(
        <QueryClientProvider client={queryClient}>
          <MemoryRouter initialEntries={["/entries/entry-1"]}>
            <Routes>
              <Route path="/entries/:entryId" element={<EntryDetailPage />} />
            </Routes>
          </MemoryRouter>
        </QueryClientProvider>,
      )
    })

    const backButton = container.querySelector<HTMLButtonElement>('button[aria-label="返回首页"]')
    expect(backButton).not.toBeNull()
    const glyph = backButton?.querySelector('svg[aria-hidden="true"] path')
    expect(glyph).not.toBeNull()
    expect(glyph?.getAttribute("d")).toBeTruthy()
  })
})
