import type { Page } from "@playwright/test"
import { expect, test } from "@playwright/test"

import { buildWebAppURL, resolveDesktopE2EEnv } from "../../support/env"

const readyResponse = {
  ready: true,
  checks: { sqlite: "ok", migrations: "ok", keychain: "ok" },
}

const sessionResponse = {
  user: { id: "user-home", name: "Home User", email: null, image: null },
  timezone: "Asia/Shanghai",
}

const topic = (id: string, name: string) => ({
  id,
  name,
  kind: id === "recommend" ? "core" : "dynamic",
  fixed: id === "recommend",
  pinned: true,
  hidden: false,
  unreadCount: 5,
})

const card = (
  entryId: string,
  type: "article" | "post" | "image" | "video",
  title = `${type} ${entryId}`,
) => ({
  entryId,
  type,
  title,
  excerpt: `Excerpt ${entryId}`,
  cover: type === "post" ? null : `https://images.tantan.test/${entryId}.jpg`,
  source: { id: "source-1", name: "Source", avatar: null },
  publishedAt: "2026-08-09T12:00:00Z",
  topics: [{ id: "topic-ai", name: "AI" }],
  translated: false,
})

const homePage = (items: ReturnType<typeof card>[], nextCursor: string | null) => ({
  items,
  nextCursor,
  queue: {
    id: "queue-home",
    version: 1,
    total: items.length,
    unread: items.length,
    finished: nextCursor === null,
    candidateWindowDays: 7,
    generatedAt: "2026-08-09T12:00:00Z",
  },
})

const mockSession = async (page: Page) => {
  await page.route("http://127.0.0.1:3000/readyz", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(readyResponse),
    }),
  )
  await page.route("http://127.0.0.1:3000/tantan/v1/session", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(sessionResponse),
    }),
  )
}

test.describe("Tantan Home", () => {
  test("REQ:FE-03 uses 2/3/4 columns, deduplicates cursor pages and falls back from broken images", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockSession(page)
    await page.route("https://images.tantan.test/**", (route) => route.abort("failed"))
    await page.route("http://127.0.0.1:3000/tantan/v1/topics", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          version: 1,
          activeFilterId: null,
          topics: [topic("recommend", "推荐"), topic("topic-ai", "AI")],
        }),
      }),
    )
    await page.route("http://127.0.0.1:3000/tantan/v1/home?**", (route) => {
      const cursor = new URL(route.request().url()).searchParams.get("cursor")
      const body = cursor
        ? homePage([card("102", "article"), card("104", "video")], null)
        : homePage([card("101", "article"), card("102", "post"), card("103", "image")], "cursor-2")
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(body),
      })
    })

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })
    const feed = page.getByTestId("masonry-feed")
    await expect(feed).toHaveAttribute("data-columns", "2")
    await expect(page.locator('[data-entry-id="102"]')).toHaveCount(1)
    await expect(page.locator('[data-entry-id="104"]')).toBeVisible()
    await expect(page.getByText("今天已经看完", { exact: true })).toBeVisible()
    await expect(page.locator('[data-entry-id="101"] img')).toHaveCount(0)

    await page.setViewportSize({ width: 800, height: 844 })
    await expect(feed).toHaveAttribute("data-columns", "2")
    await page.setViewportSize({ width: 1024, height: 844 })
    await expect(feed).toHaveAttribute("data-columns", "3")
    await page.setViewportSize({ width: 1440, height: 900 })
    await expect(feed).toHaveAttribute("data-columns", "4")
  })

  test("REQ:FE-03 search navigates while AI opens and atomically applies the Filter Sheet", async ({
    page,
  }) => {
    await mockSession(page)
    let activeFilterId: string | null = null
    let filterCalls = 0
    await page.route("http://127.0.0.1:3000/tantan/v1/topics", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          version: 1,
          activeFilterId,
          topics: [topic("recommend", "推荐")],
        }),
      }),
    )
    await page.route("http://127.0.0.1:3000/tantan/v1/home?**", (route) => {
      const filterId = new URL(route.request().url()).searchParams.get("filterId")
      const item = filterId
        ? card("202", "article", "Filtered Codex")
        : card("201", "article", "Default")
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(homePage([item], null)),
      })
    })
    await page.route("http://127.0.0.1:3000/tantan/v1/filter", async (route) => {
      if (route.request().method() !== "PUT") return route.fallback()
      filterCalls += 1
      expect(route.request().headers()["idempotency-key"]?.length).toBeGreaterThanOrEqual(16)
      expect(route.request().postDataJSON()).toEqual({ prompt: "多推 Codex" })
      activeFilterId = "filter-1"
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          filter: { id: "filter-1", prompt: "多推 Codex", createdAt: "2026-08-09T12:00:00Z" },
          topics: [topic("recommend", "推荐"), topic("topic-codex", "Codex")],
          queueId: "queue-filter-1",
        }),
      })
    })

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })
    await expect(page.getByText("Default", { exact: true })).toBeVisible()
    await page.getByRole("button", { name: "搜索内容" }).click()
    await expect(page).toHaveURL(/\/search$/)
    expect(filterCalls).toBe(0)
    await page.goBack()

    const beforeAI = page.url()
    await page.getByRole("button", { name: "AI 智能筛选" }).click()
    await expect(page.getByRole("dialog", { name: "AI 智能筛选" })).toBeVisible()
    expect(page.url()).toBe(beforeAI)
    await page.getByLabel("筛选要求").fill("多推 Codex")
    await page.getByRole("button", { name: "生成信息流" }).click()

    await expect(page.getByTestId("active-ai-filter")).toContainText("多推 Codex")
    await expect(page.getByRole("tab", { name: "Codex" })).toBeVisible()
    await expect(page.getByText("Filtered Codex", { exact: true })).toBeVisible()
    expect(filterCalls).toBe(1)
  })

  test("REQ:FE-03 detail hides Mobile navigation and read success removes the Home card", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockSession(page)
    const entryId = "41147805272531997"
    let read = false
    await page.route("http://127.0.0.1:3000/tantan/v1/topics", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          version: 1,
          activeFilterId: null,
          topics: [topic("recommend", "推荐")],
        }),
      }),
    )
    await page.route("http://127.0.0.1:3000/tantan/v1/home?**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(homePage(read ? [] : [card(entryId, "article", "Read me")], null)),
      }),
    )
    await page.route("http://localhost:3000/entries?**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            entries: {
              id: entryId,
              title: "Read me",
              url: "https://example.com/read-me",
              content: "Entry body",
              description: "Entry description",
              guid: entryId,
              author: "Author",
              authorUrl: null,
              authorAvatar: null,
              insertedAt: "2026-08-09T12:00:00Z",
              publishedAt: "2026-08-09T12:00:00Z",
              media: null,
              categories: null,
              attachments: null,
              extra: null,
              language: "en",
            },
            feeds: { id: "feed-1", type: "feed" },
            settings: null,
          },
        }),
      }),
    )
    await page.route("http://localhost:3000/reads", (route) => {
      read = true
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: true }),
      })
    })

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })
    await page.getByRole("link", { name: "阅读：Read me" }).click()
    await expect(page).toHaveURL(new RegExp(`/entries/${entryId}$`))
    await expect(page.getByRole("navigation", { name: "Mobile navigation" })).toHaveCount(0)
    await expect(page.getByRole("heading", { name: "Read me" })).toBeVisible()
    await expect.poll(() => read).toBe(true)

    await page.getByRole("button", { name: "返回首页" }).click()
    await expect(page).toHaveURL(/\/$/)
    await expect(page.locator(`[data-entry-id="${entryId}"]`)).toHaveCount(0)
  })
})
