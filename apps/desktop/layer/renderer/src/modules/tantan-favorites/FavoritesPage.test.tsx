import * as React from "react"
import { act } from "react"
import type { Root } from "react-dom/client"
import { createRoot } from "react-dom/client"
import { MemoryRouter } from "react-router"
import { afterEach, beforeAll, describe, expect, test, vi } from "vitest"

import { FavoritesPage } from "./FavoritesPage"

const collectionBoundary = vi.hoisted(() => ({
  unstarEntry: vi.fn(),
}))

vi.mock("@follow/store/collection/store", () => ({
  collectionSyncService: collectionBoundary,
}))

vi.mock("@follow/store/entry/hooks", () => ({
  useEntriesQuery: () => ({
    entriesIds: ["entry-favorite"],
    isLoading: false,
    error: null,
    hasNextPage: false,
    isFetchingNextPage: false,
    fetchNextPage: vi.fn(),
  }),
  useEntryList: () => [
    {
      id: "entry-favorite",
      feedId: "feed-favorite",
      inboxHandle: null,
      title: "Keep favorite",
      description: "Favorite description",
      content: "Favorite body",
      url: "https://example.com/favorite",
      author: "Author",
      authorUrl: null,
      authorAvatar: null,
      insertedAt: new Date("2026-08-09T12:00:00Z"),
      publishedAt: new Date("2026-08-09T12:00:00Z"),
      media: null,
      categories: null,
      attachments: null,
      extra: null,
      language: "en",
      read: false,
      sources: null,
      settings: null,
      readabilityContent: null,
      guid: "entry-favorite",
    },
  ],
}))

vi.mock("@follow/store/feed/hooks", () => ({
  useFeedById: () => ({
    id: "feed-favorite",
    title: "Favorite Source",
    url: "https://example.com/rss",
    image: null,
  }),
}))

const flush = () => new Promise((resolve) => setTimeout(resolve, 0))

describe("FavoritesPage mutation reconciliation", () => {
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

  test("TASK-06 failed unstar is single-flight and keeps the favorite visible", async () => {
    let rejectMutation: ((reason: Error) => void) | undefined
    collectionBoundary.unstarEntry.mockReturnValue(
      new Promise<void>((_resolve, reject) => {
        rejectMutation = reject
      }),
    )
    container = document.createElement("div")
    document.body.append(container)
    root = createRoot(container)
    await act(async () => {
      root?.render(
        <MemoryRouter>
          <FavoritesPage />
        </MemoryRouter>,
      )
    })

    const unstar = container.querySelector<HTMLButtonElement>(
      'button[aria-label="取消收藏 Keep favorite"]',
    )
    expect(unstar).not.toBeNull()
    await act(async () => {
      unstar?.click()
      unstar?.click()
    })
    expect(collectionBoundary.unstarEntry).toHaveBeenCalledTimes(1)
    expect(unstar?.disabled).toBe(true)

    await act(async () => {
      rejectMutation?.(new Error("collection unavailable"))
      await flush()
    })

    expect(container.textContent).toContain("Keep favorite")
    expect(container.querySelector('[role="alert"]')?.textContent).toContain(
      "collection unavailable",
    )
    expect(unstar?.disabled).toBe(false)
  })
})
