import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import * as React from "react"
import { act } from "react"
import type { Root } from "react-dom/client"
import { createRoot } from "react-dom/client"
import { MemoryRouter, Route, Routes } from "react-router"
import { afterEach, beforeAll, describe, expect, test, vi } from "vitest"

import { homeViewStore } from "~/modules/tantan-home/home-view-store"

import { SEARCH_DEBOUNCE_MS } from "./search-model"
import { SearchPage } from "./SearchPage"

const searchAPI = vi.hoisted(() => ({ search: vi.fn() }))

vi.mock("./api", () => ({ searchEntries: searchAPI.search }))

const setInputValue = (input: HTMLInputElement, value: string) => {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set
  setter?.call(input, value)
  input.dispatchEvent(new Event("input", { bubbles: true }))
}

const emptyResult = { items: [], nextCursor: null, indexStatus: "ready" }

const renderSearch = async () => {
  const container = document.createElement("div")
  document.body.append(container)
  const root = createRoot(container)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  await act(async () => {
    root.render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={["/", "/search"]} initialIndex={1}>
          <Routes>
            <Route path="/" element={<h1>Home restored</h1>} />
            <Route path="/search" element={<SearchPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    )
  })
  return { container, root, queryClient }
}

describe("SearchPage", () => {
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
    vi.useRealTimers()
    vi.clearAllMocks()
    homeViewStore.getState().reset()
  })

  test("REQ:FE-04 debounces 250ms, aborts stale input and preserves Home state", async () => {
    vi.useFakeTimers()
    const signals: AbortSignal[] = []
    searchAPI.search.mockImplementation(
      ({ q, signal }: { q: string; signal: AbortSignal }) =>
        new Promise((resolve, reject) => {
          signals.push(signal)
          if (q === "second") {
            resolve(emptyResult)
            return
          }
          signal.addEventListener(
            "abort",
            () => reject(new DOMException("aborted", "AbortError")),
            {
              once: true,
            },
          )
        }),
    )
    homeViewStore.getState().setActiveTopic("topic_ai")
    homeViewStore.getState().activateFilter("filter_1", "topic_ai", "保留状态")
    homeViewStore.getState().saveScroll("topic_ai", 432)
    ;({ container, root, queryClient } = await renderSearch())

    const input = container.querySelector<HTMLInputElement>("#tantan-search")!
    await act(async () => setInputValue(input, "first"))
    await act(async () => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS - 1))
    expect(searchAPI.search).not.toHaveBeenCalled()
    await act(async () => vi.advanceTimersByTimeAsync(1))
    expect(searchAPI.search).toHaveBeenCalledTimes(1)

    await act(async () => setInputValue(input, "second"))
    await act(async () => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS))
    expect(searchAPI.search).toHaveBeenCalledTimes(2)
    expect(signals[0]?.aborted).toBe(true)
    expect(searchAPI.search.mock.calls[1]?.[0]).toMatchObject({ q: "second", cursor: null })

    const state = homeViewStore.getState()
    expect(state.activeTopicId).toBe("topic_ai")
    expect(state.activeFilterId).toBe("filter_1")
    expect(state.scrollY.topic_ai).toBe(432)
  })

  test("REQ:FE-04 back navigation restores the prior route without resetting Home", async () => {
    searchAPI.search.mockResolvedValue(emptyResult)
    homeViewStore.getState().setActiveTopic("topic_ai")
    ;({ container, root, queryClient } = await renderSearch())

    const back = container.querySelector<HTMLButtonElement>('button[aria-label="返回"]')
    await act(async () => back?.click())

    expect(container.textContent).toContain("Home restored")
    expect(homeViewStore.getState().activeTopicId).toBe("topic_ai")
  })
})
