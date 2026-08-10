import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import * as React from "react"
import { act, useRef } from "react"
import type { Root } from "react-dom/client"
import { createRoot } from "react-dom/client"
import { MemoryRouter } from "react-router"
import { afterEach, beforeAll, describe, expect, test, vi } from "vitest"

import type { HomeCard, HomeResponse, SyncStatusResponse } from "~/lib/tantan-api/gen/types"

import { homeViewStore } from "./home-view-store"
import { homeQueryKeys } from "./query-keys"
import { translationPollInterval, useHomeController } from "./useHomeController"

const homeAPI = vi.hoisted(() => ({
  deleteFilter: vi.fn(),
  getHome: vi.fn(),
  getSyncStatus: vi.fn(),
  getTopics: vi.fn(),
  postFeedback: vi.fn(),
  putFilter: vi.fn(),
  triggerSync: vi.fn(),
}))

vi.mock("./api", () => ({
  deleteActiveFilter: homeAPI.deleteFilter,
  getHome: homeAPI.getHome,
  getSyncStatus: homeAPI.getSyncStatus,
  getTopics: homeAPI.getTopics,
  postRecommendationFeedback: homeAPI.postFeedback,
  putActiveFilter: homeAPI.putFilter,
  triggerFullSync: homeAPI.triggerSync,
}))

const card: HomeCard = {
  entryId: "entry-after-sync",
  type: "article",
  title: "Synced entry",
  excerpt: "Synced from Folo",
  cover: null,
  source: { id: "feed-1", name: "Folo feed", avatar: null },
  publishedAt: "2026-08-10T01:00:00Z",
  topics: [],
  translated: false,
}

const homeResponse = (items: HomeCard[]): HomeResponse => ({
  items,
  nextCursor: null,
  queue: {
    id: "queue-1",
    version: 1,
    generation: "queue-1-v1",
    total: items.length,
    unread: items.length,
    finished: items.length === 0,
    candidateWindowDays: 7,
    generatedAt: "2026-08-10T01:00:00Z",
  },
  queueGeneration: "queue-1-v1",
})

const syncStatus = (state: SyncStatusResponse["state"], updatedAt: string) => ({
  state,
  scope: "all" as const,
  counts: { processed: 0, total: 0, failed: 0 },
  error: null,
  updatedAt,
})

function ControllerHarness() {
  const scrollRef = useRef<HTMLDivElement>(null)
  const controller = useHomeController(scrollRef)
  return (
    <div ref={scrollRef}>
      <span data-testid="card-count">{controller.cards.length}</span>
      <span data-testid="syncing">{String(controller.syncing)}</span>
    </div>
  )
}

describe("useHomeController sync lifecycle", () => {
  let root: Root | null = null
  let container: HTMLDivElement | null = null
  let queryClient: QueryClient | null = null

  beforeAll(() => {
    ;(globalThis as typeof globalThis & { React: typeof React }).React = React
    ;(
      globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true
    globalThis.requestAnimationFrame ||= (callback) => window.setTimeout(callback, 0)
    globalThis.cancelAnimationFrame ||= (handle) => window.clearTimeout(handle)
  })

  afterEach(async () => {
    if (root) await act(async () => root?.unmount())
    queryClient?.clear()
    container?.remove()
    root = null
    container = null
    queryClient = null
    homeViewStore.getState().reset()
    vi.clearAllMocks()
  })

  const renderController = async () => {
    container = document.createElement("div")
    document.body.append(container)
    root = createRoot(container)
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: 0 } },
    })
    await act(async () => {
      root?.render(
        <QueryClientProvider client={queryClient!}>
          <MemoryRouter>
            <ControllerHarness />
          </MemoryRouter>
        </QueryClientProvider>,
      )
    })
  }

  test("REQ:JOB-01 refreshes the empty Home queue after sync succeeds", async () => {
    homeAPI.getTopics.mockResolvedValue({
      version: 1,
      topicsRevision: "topics-1",
      activeFilterId: null,
      topics: [],
    })
    homeAPI.getHome.mockResolvedValueOnce(homeResponse([])).mockResolvedValue(homeResponse([card]))
    homeAPI.getSyncStatus.mockResolvedValueOnce(syncStatus("queued", "2026-08-10T01:00:00Z"))

    await renderController()
    await vi.waitFor(() =>
      expect(container?.querySelector('[data-testid="syncing"]')?.textContent).toBe("true"),
    )
    expect(container?.querySelector('[data-testid="card-count"]')?.textContent).toBe("0")

    homeAPI.getSyncStatus.mockResolvedValue(syncStatus("succeeded", "2026-08-10T01:00:05Z"))
    await act(async () => {
      await queryClient?.invalidateQueries({ queryKey: homeQueryKeys.sync })
    })

    await vi.waitFor(() => expect(homeAPI.getHome.mock.calls.length).toBeGreaterThanOrEqual(2))
    await vi.waitFor(() =>
      expect(container?.querySelector('[data-testid="card-count"]')?.textContent).toBe("1"),
    )
  })

  test("REQ:JOB-01 repairs an existing session that has never synchronized", async () => {
    homeAPI.getTopics.mockResolvedValue({
      version: 1,
      topicsRevision: "topics-1",
      activeFilterId: null,
      topics: [],
    })
    homeAPI.getHome.mockResolvedValue(homeResponse([]))
    homeAPI.getSyncStatus.mockResolvedValue(syncStatus("idle", "2026-08-10T01:00:00Z"))
    homeAPI.triggerSync.mockResolvedValue({ jobId: "job-1", state: "queued" })

    await renderController()

    await vi.waitFor(() => expect(homeAPI.triggerSync).toHaveBeenCalledOnce())
  })

  test("REQ:TRANSLATION-GATE keeps polling every loaded page until all unread cards are translated", () => {
    const first = homeResponse([card])
    first.queue.total = 3
    first.queue.unread = 3
    first.queue.finished = false
    expect(translationPollInterval({ pages: [first], pageParams: [] })).toBe(1_500)

    const complete = homeResponse([
      card,
      { ...card, entryId: "entry-2" },
      { ...card, entryId: "entry-3" },
    ])
    expect(translationPollInterval({ pages: [complete], pageParams: [] })).toBe(false)

    const paginated = { ...first, nextCursor: "next-page" }
    expect(translationPollInterval({ pages: [paginated], pageParams: [] })).toBe(false)
  })
})
