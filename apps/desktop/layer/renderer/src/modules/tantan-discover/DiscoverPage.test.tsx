import * as React from "react"
import { act } from "react"
import type { Root } from "react-dom/client"
import { createRoot } from "react-dom/client"
import { MemoryRouter } from "react-router"
import { afterEach, beforeAll, describe, expect, test, vi } from "vitest"

import { DiscoverPage } from "./DiscoverPage"

const subscriptionBoundary = vi.hoisted(() => ({
  subscribe: vi.fn(),
}))

vi.mock("@follow/store/subscription/hooks", () => ({
  useSubscriptionByFeedId: () => null,
}))

vi.mock("@follow/store/subscription/store", () => ({
  subscriptionSyncService: subscriptionBoundary,
}))

const response = {
  code: 0,
  data: [
    {
      feed: {
        type: "feed",
        id: "feed-1",
        url: "https://example.com/feed.xml",
        title: "Example Feed",
        description: "A useful RSS source",
        siteUrl: "https://example.com",
        image: null,
      },
      subscriptionCount: 12,
      updatesPerWeek: 7,
    },
  ],
}

const flush = () => new Promise((resolve) => setTimeout(resolve, 0))

describe("DiscoverPage mobile boundary", () => {
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
    vi.unstubAllGlobals()
    vi.clearAllMocks()
  })

  test("FR-15 searches feeds through the same-origin Go proxy", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(response), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    )
    vi.stubGlobal("fetch", fetchMock)
    container = document.createElement("div")
    document.body.append(container)
    root = createRoot(container)

    await act(async () => {
      root?.render(
        <MemoryRouter>
          <DiscoverPage />
        </MemoryRouter>,
      )
    })

    const input = container.querySelector<HTMLInputElement>('input[type="search"]')
    expect(input).not.toBeNull()
    await act(async () => {
      const valueSetter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set
      valueSetter?.call(input, "Example")
      input?.dispatchEvent(new Event("input", { bubbles: true }))
    })
    const form = container.querySelector("form")
    await act(async () => {
      form?.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }))
      await flush()
    })

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [path, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(path).toBe("/api/folo/discover")
    expect(init.method).toBe("POST")
    expect(JSON.parse(String(init.body))).toEqual({ keyword: "Example", target: "feeds" })
    expect(container.textContent).toContain("Example Feed")
    expect(container.textContent).toContain("A useful RSS source")
  })

  test("FR-16 prevents duplicate subscription mutations and restores the action after failure", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify(response), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    )
    let rejectSubscription: ((reason: Error) => void) | undefined
    subscriptionBoundary.subscribe.mockReturnValue(
      new Promise<void>((_resolve, reject) => {
        rejectSubscription = reject
      }),
    )
    container = document.createElement("div")
    document.body.append(container)
    root = createRoot(container)
    await act(async () => {
      root?.render(
        <MemoryRouter>
          <DiscoverPage />
        </MemoryRouter>,
      )
    })

    const input = container.querySelector<HTMLInputElement>('input[type="search"]')
    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set
      setter?.call(input, "Example")
      input?.dispatchEvent(new Event("input", { bubbles: true }))
      container!
        .querySelector("form")
        ?.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }))
      await flush()
    })
    const button = Array.from(container.querySelectorAll("button")).find((element) =>
      element.textContent?.includes("订阅"),
    )
    expect(button).toBeDefined()
    await act(async () => {
      button?.click()
      button?.click()
    })

    expect(subscriptionBoundary.subscribe).toHaveBeenCalledTimes(1)
    expect(button?.disabled).toBe(true)
    await act(async () => {
      rejectSubscription?.(new Error("upstream unavailable"))
      await flush()
    })

    expect(container.querySelector('[role="alert"]')?.textContent).toContain("upstream unavailable")
    expect(button?.disabled).toBe(false)
    expect(button?.textContent).toContain("订阅")
  })
})
