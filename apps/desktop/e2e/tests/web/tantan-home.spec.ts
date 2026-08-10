import type { Page } from "@playwright/test"
import { expect, test } from "@playwright/test"

import { buildWebAppURL, resolveDesktopE2EEnv } from "../../support/env"
import { expectDialogToSpanViewport, expectVisibleIconGlyph } from "../../support/visual-assertions"

const readyResponse = {
  ready: true,
  checks: { sqlite: "ok", migrations: "ok", keychain: "ok" },
}

const sessionResponse = {
  user: { id: "user-home", name: "Home User", email: null, image: null },
  timezone: "Asia/Shanghai",
  csrfToken: "csrf-home",
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
  translated: true,
})

const homePage = (
  items: ReturnType<typeof card>[],
  nextCursor: string | null,
  generation = "generation-home",
  total = 60,
) => ({
  items,
  nextCursor,
  queue: {
    id: "queue-home",
    version: 1,
    generation,
    total,
    unread: total,
    finished: false,
    candidateWindowDays: 7,
    generatedAt: "2026-08-09T12:00:00Z",
  },
  queueGeneration: generation,
})

const mockSession = async (page: Page) => {
  await page.route("**/api/readyz", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(readyResponse),
    }),
  )
  await page.route("**/api/tantan/v1/session", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(sessionResponse),
    }),
  )
  await page.route("**/api/folo/collections?**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ code: 0, data: false }),
    }),
  )
}

test.describe("Tantan Home", () => {
  test("REQ:DYNAMIC-TOPIC-LAZY loads a classified feed only after its Tab is clicked", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockSession(page)
    await page.route("**/api/tantan/v1/topics", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          version: 2,
          topicsRevision: 2,
          activeFilterId: null,
          topics: [
            topic("recommend", "推荐"),
            topic("topic-claude", "Claude"),
            topic("topic-sqlite", "SQLite"),
          ],
        }),
      }),
    )
    const calls = new Map<string, number>()
    await page.route("**/api/tantan/v1/home?**", (route) => {
      const topicId = new URL(route.request().url()).searchParams.get("topicId") ?? "recommend"
      calls.set(topicId, (calls.get(topicId) ?? 0) + 1)
      const title =
        topicId === "topic-claude"
          ? "Claude 智能体工作流"
          : topicId === "topic-sqlite"
            ? "SQLite 文本历史"
            : "今日中文推荐"
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(
          homePage([card(`entry-${topicId}`, "article", title)], null, `generation-${topicId}`, 1),
        ),
      })
    })

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })
    await expect(page.getByText("今日中文推荐", { exact: true })).toBeVisible()
    expect(calls.get("topic-claude") ?? 0).toBe(0)
    expect(calls.get("topic-sqlite") ?? 0).toBe(0)

    await page.getByRole("tab", { name: "Claude" }).click()
    await expect(page.getByText("Claude 智能体工作流", { exact: true })).toBeVisible()
    expect(calls.get("topic-claude")).toBe(1)
    expect(calls.get("topic-sqlite") ?? 0).toBe(0)
  })

  test("REQ:TRANSLATION-GATE polls a pending queue and never renders the English card", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockSession(page)
    await page.route("**/api/tantan/v1/topics", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          version: 1,
          topicsRevision: 1,
          activeFilterId: null,
          topics: [topic("recommend", "推荐")],
        }),
      }),
    )
    let homeCalls = 0
    let translatedReady = false
    await page.route("**/api/tantan/v1/home?**", (route) => {
      homeCalls += 1
      const items = translatedReady ? [card("translated-entry", "article", "已翻译的中文标题")] : []
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(homePage(items, null, "generation-translation", 1)),
      })
    })

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })
    await expect(page.getByText("正在翻译推荐内容", { exact: true })).toBeVisible()
    await expect(page.getByText("English title", { exact: true })).toHaveCount(0)
    translatedReady = true
    await expect(page.getByText("已翻译的中文标题", { exact: true })).toBeVisible({
      timeout: 5_000,
    })
    expect(homeCalls).toBeGreaterThanOrEqual(2)
  })

  test("AC-03 keeps a two-column Mobile feed stable through a 60-item long scroll", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockSession(page)
    await page.route("https://images.tantan.test/**", (route) => route.abort("failed"))
    await page.route("**/api/tantan/v1/topics", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          version: 1,
          topicsRevision: 1,
          activeFilterId: null,
          topics: [topic("recommend", "推荐"), topic("topic-ai", "AI")],
        }),
      }),
    )
    let homeCalls = 0
    await page.route("**/api/tantan/v1/home?**", (route) => {
      homeCalls += 1
      const cursor = new URL(route.request().url()).searchParams.get("cursor")
      const first = Array.from({ length: 20 }, (_, index) =>
        card(String(index + 1).padStart(3, "0"), index === 1 ? "post" : "article"),
      )
      const second = [
        card("020", "article"),
        ...Array.from({ length: 19 }, (_, index) =>
          card(String(index + 21).padStart(3, "0"), "image"),
        ),
      ]
      const third = Array.from({ length: 20 }, (_, index) =>
        card(String(index + 40).padStart(3, "0"), "video"),
      )
      const body =
        cursor === "cursor-3"
          ? homePage(third, null)
          : cursor === "cursor-2"
            ? homePage(second, "cursor-3")
            : homePage(first, "cursor-2")
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(body),
      })
    })

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })
    const feed = page.getByTestId("masonry-feed")
    await expect(feed).toHaveAttribute("data-columns", "2")
    await expect(page.locator('[data-entry-id="001"] img')).toHaveCount(0)
    await page.getByTestId("home-pagination-sentinel").scrollIntoViewIfNeeded()
    await expect.poll(() => homeCalls).toBeGreaterThanOrEqual(2)
    await expect(page.locator('[data-entry-id="020"]')).toHaveCount(1)
    await page.getByTestId("home-pagination-sentinel").scrollIntoViewIfNeeded()
    await expect.poll(() => homeCalls).toBe(3)
    const lastCard = page.locator('[data-entry-id="059"]')
    const viewport = page.getByTestId("home-scroll-viewport")
    await expect
      .poll(async () => {
        await viewport.evaluate(async (element) => {
          element.scrollTop = element.scrollHeight
          element.dispatchEvent(new Event("scroll"))
          await new Promise<void>((resolve) => {
            requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
          })
        })
        return lastCard.count()
      })
      .toBe(1)
    await expect(lastCard).toBeVisible()
    await expect(page.getByText("今天已经看完", { exact: true })).toBeVisible()

    await page.setViewportSize({ width: 800, height: 844 })
    await expect(feed).toHaveAttribute("data-columns", "2")
    await page.setViewportSize({ width: 1024, height: 844 })
    await expect(feed).toHaveAttribute("data-columns", "2")
    await page.setViewportSize({ width: 1440, height: 900 })
    await expect(feed).toHaveAttribute("data-columns", "2")
  })

  test("TC-33 discards a delayed page from another queue generation and reloads page one", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockSession(page)
    await page.route("**/api/tantan/v1/topics", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          version: 1,
          topicsRevision: 1,
          activeFilterId: null,
          topics: [topic("recommend", "推荐")],
        }),
      }),
    )
    let firstPageCalls = 0
    let cursorCalls = 0
    let mismatchDelivered = false
    await page.route("**/api/tantan/v1/home?**", async (route) => {
      const cursor = new URL(route.request().url()).searchParams.get("cursor")
      if (cursor) {
        cursorCalls += 1
        await new Promise((resolve) => setTimeout(resolve, 100))
        mismatchDelivered = true
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(
            homePage(
              [card("stale-page", "article", "Stale delayed page")],
              null,
              "generation-new",
              1,
            ),
          ),
        })
      }
      firstPageCalls += 1
      const body = mismatchDelivered
        ? homePage([card("new-page", "article", "New generation")], null, "generation-new", 1)
        : homePage(
            [card("old-page", "article", "Old generation")],
            "cursor-old",
            "generation-old",
            2,
          )
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(body),
      })
    })

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })
    await expect.poll(() => cursorCalls).toBe(1)
    await expect(page.getByText("推荐已更新", { exact: true })).toBeVisible()
    await expect(page.getByText("New generation", { exact: true })).toBeVisible()
    await expect(page.getByText("Stale delayed page", { exact: true })).toHaveCount(0)
    expect(firstPageCalls).toBeGreaterThanOrEqual(2)
  })

  test("REQ:FE-03 search navigates while AI opens and atomically applies the Filter Sheet", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockSession(page)
    let activeFilterId: string | null = null
    let filterCalls = 0
    await page.route("**/api/tantan/v1/topics", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          version: 1,
          topicsRevision: 1,
          activeFilterId,
          topics: [topic("recommend", "推荐")],
        }),
      }),
    )
    await page.route("**/api/tantan/v1/home?**", (route) => {
      const filterId = new URL(route.request().url()).searchParams.get("filterId")
      const item = filterId
        ? card("202", "article", "Filtered Codex")
        : card("201", "article", "Default")
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(
          homePage([item], null, filterId ? "generation-filter-1" : "generation-default", 1),
        ),
      })
    })
    await page.route("**/api/tantan/v1/filter", async (route) => {
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
          topicsRevision: 2,
          queueId: "queue-filter-1",
          queueGeneration: "generation-filter-1",
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
    const filterDialog = page.getByRole("dialog", { name: "AI 智能筛选" })
    await expectDialogToSpanViewport(page, filterDialog)
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
    const entryId = "309246809866240004"
    let read = false
    await page.route("**/api/tantan/v1/topics", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          version: 1,
          topicsRevision: 1,
          activeFilterId: null,
          topics: [topic("recommend", "推荐")],
        }),
      }),
    )
    await page.route("**/api/tantan/v1/home?**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(
          homePage(read ? [] : [card(entryId, "article", "中文标题")], null, "generation-read", 1),
        ),
      }),
    )
    await page.route("**/api/folo/entries?**", (route) =>
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
              read: false,
            },
            feeds: { id: "feed-1", type: "feed" },
            settings: null,
          },
        }),
      }),
    )
    await page.route(`**/api/tantan/v1/entries/${entryId}/enrichment?**`, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          state: "ready",
          data: {
            version: 1,
            detectedLanguage: "en",
            titleZh: "中文标题",
            contentZh: "已经翻译完成的中文正文",
            summaryZh: "中文摘要",
            keyPoints: ["要点"],
          },
          error: null,
        }),
      }),
    )
    await page.route("**/api/folo/reads", (route) => {
      read = true
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: true }),
      })
    })

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })
    await page.getByRole("link", { name: "阅读：中文标题" }).click()
    await expect(page).toHaveURL(new RegExp(`/entries/${entryId}$`))
    await expect(page.getByRole("tablist", { name: "主导航" })).toHaveCount(0)
    await expect(page.getByRole("heading", { name: "中文标题" })).toBeVisible()
    await expect(page.getByText("已经翻译完成的中文正文", { exact: true })).toBeVisible()
    await expect(page.getByText("Entry body", { exact: true })).toHaveCount(0)

    const entryBackButton = page.getByRole("button", { name: "返回首页" })
    await expectVisibleIconGlyph(entryBackButton)
    await entryBackButton.click()
    await expect(page).toHaveURL(/\/$/)
    await expect(page.locator(`[data-entry-id="${entryId}"]`)).toHaveCount(0)
  })

  test("REQ:FE-03 read failure reports the error and preserves the Home card", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockSession(page)
    const entryId = "309246809866240005"
    await page.route("**/api/tantan/v1/topics", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          version: 1,
          topicsRevision: 1,
          activeFilterId: null,
          topics: [topic("recommend", "推荐")],
        }),
      }),
    )
    await page.route("**/api/tantan/v1/home?**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(
          homePage([card(entryId, "article", "Keep me")], null, "generation-read-fail", 1),
        ),
      }),
    )
    await page.route("**/api/folo/entries?**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            entries: {
              id: entryId,
              title: "Keep me",
              url: "https://example.com/keep-me",
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
              read: false,
            },
            feeds: { id: "feed-1", type: "feed" },
            settings: null,
          },
        }),
      }),
    )
    await page.route(`**/api/tantan/v1/entries/${entryId}/enrichment?**`, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ state: "pending", data: null, error: null }),
      }),
    )
    await page.route("**/api/folo/reads", (route) =>
      route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({
          requestId: "read-failure",
          error: { code: "UPSTREAM_UNAVAILABLE", message: "Folo unavailable", retryable: true },
        }),
      }),
    )

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })
    await page.getByRole("link", { name: "阅读：Keep me" }).click()
    await expect(page.getByRole("alert")).toContainText("卡片已保留")
    await page.getByRole("button", { name: "返回首页" }).click()

    await expect(page).toHaveURL(/\/$/)
    await expect(page.locator(`[data-entry-id="${entryId}"]`)).toHaveCount(1)
  })
})
