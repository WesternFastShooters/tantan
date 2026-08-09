import type { Page } from "@playwright/test"
import { expect, test } from "@playwright/test"

import { buildWebAppURL, resolveDesktopE2EEnv } from "../../support/env"

const readyResponse = {
  ready: true,
  checks: { sqlite: "ok", migrations: "ok", keychain: "ok" },
}

const sessionResponse = {
  user: { id: "user-flows", name: "Flow User", email: null, image: null },
  timezone: "Asia/Shanghai",
}

const topic = (id: string, name: string, extra: Record<string, unknown> = {}) => ({
  id,
  name,
  kind: id === "recommend" ? "core" : "dynamic",
  fixed: id === "recommend",
  pinned: true,
  hidden: false,
  unreadCount: 3,
  ...extra,
})

const card = (entryId: string, title: string, translated = false) => ({
  entryId,
  type: "article" as const,
  title,
  excerpt: `Body ${title}`,
  cover: null,
  source: { id: "source-1", name: "AI Weekly", avatar: null },
  publishedAt: "2026-08-09T12:00:00Z",
  topics: [{ id: "topic-ai", name: "AI" }],
  translated,
})

const homeResponse = (items: ReturnType<typeof card>[]) => ({
  items,
  nextCursor: null,
  queue: {
    id: "queue-flow",
    version: 1,
    generation: "queue-flow-v1",
    total: items.length,
    unread: items.length,
    finished: true,
    candidateWindowDays: 7,
    generatedAt: "2026-08-09T12:00:00Z",
  },
  queueGeneration: "queue-flow-v1",
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
  await page.route("**/api/tantan/v1/recommendation/blocks/sources", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ items: [] }),
    }),
  )
}

test.describe("Tantan phase-one flows", () => {
  test("REQ:FE-04 Mobile search waits 250ms, pages all indexed fields, and preserves Home state", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockSession(page)
    let searchInputAt = 0
    let searchDelay = 0
    let homeMutationCount = 0
    await page.route("**/api/tantan/v1/topics", (route) =>
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
    await page.route("**/api/tantan/v1/home?**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(homeResponse([card("301", "Home AI")])),
      }),
    )
    await page.route("**/api/tantan/v1/filter", (route) => {
      homeMutationCount += 1
      return route.abort()
    })
    await page.route("**/api/tantan/v1/recommendation/feedback", (route) => {
      homeMutationCount += 1
      return route.abort()
    })
    await page.route("**/api/tantan/v1/search?**", (route) => {
      searchDelay = Date.now() - searchInputAt
      const cursor = new URL(route.request().url()).searchParams.get("cursor")
      const response = cursor
        ? {
            items: [card("302", "Translated Topic result", true), card("303", "Tag result")],
            nextCursor: null,
            indexStatus: "ready",
          }
        : {
            items: [card("302", "Translated Topic result", true)],
            nextCursor: "cursor-search-0001",
            indexStatus: "building",
          }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(response),
      })
    })

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })
    await page.getByRole("tab", { name: "AI" }).click()
    await expect(page.getByRole("tab", { name: "AI" })).toHaveAttribute("aria-selected", "true")
    await page.getByRole("button", { name: "搜索内容" }).click()
    searchInputAt = Date.now()
    await page.getByLabel("搜索订阅内容").fill("Claude Code")

    await expect(page.getByText("Translated Topic result", { exact: true })).toBeVisible()
    expect(searchDelay).toBeGreaterThanOrEqual(200)
    await expect(page.getByText("搜索索引仍在构建，结果可能暂时不完整。")).toBeVisible()
    await page.getByRole("button", { name: "加载更多" }).click()
    await expect(page.getByText("Tag result", { exact: true })).toBeVisible()
    await expect(page.getByText("Translated Topic result", { exact: true })).toHaveCount(1)
    expect(homeMutationCount).toBe(0)

    await page.getByRole("button", { name: "返回" }).click()
    await expect(page).toHaveURL(/\/$/)
    await expect(page.getByRole("tab", { name: "AI" })).toHaveAttribute("aria-selected", "true")
  })

  test("REQ:FE-05 Mobile settings expose only the fixed server Gemini status", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockSession(page)
    const secretCanary = "sk-flow-secret-canary-123456"
    let testRequests = 0
    await page.route("**/api/tantan/v1/settings/ai-provider", async (route) => {
      expect(route.request().method()).toBe("GET")
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          configured: true,
          providerId: "google-gemini-openai",
          model: "gemini-3.5-flash-lite",
          baseUrl: "https://generativelanguage.googleapis.com/v1beta/openai",
          hasApiKey: true,
          keyFingerprint: "A1B2C3D4",
        }),
      })
    })
    await page.route("**/api/tantan/v1/settings/ai-provider/test", (route) => {
      testRequests += 1
      expect(route.request().method()).toBe("POST")
      expect(route.request().postData()).toBeNull()
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true, latencyMs: 42, model: "gemini-3.5-flash-lite" }),
      })
    })

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv(), "/settings/ai"), {
      waitUntil: "domcontentloaded",
    })
    await expect(page.getByText("Google Gemini")).toBeVisible()
    await expect(page.getByText("gemini-3.5-flash-lite")).toBeVisible()
    await expect(
      page.getByText("https://generativelanguage.googleapis.com/v1beta/openai"),
    ).toBeVisible()
    await expect(page.locator('input[type="password"]')).toHaveCount(0)
    await expect(page.locator("select")).toHaveCount(0)
    await expect(page.getByText("保存配置")).toHaveCount(0)
    await page.getByRole("button", { name: "测试连接" }).click()
    await expect(page.getByRole("status")).toContainText("连接成功")
    expect(testRequests).toBe(1)
    await expect(page.locator("body")).not.toContainText(secretCanary)

    const leaked = await page.evaluate((canary) => {
      const storage = [localStorage, sessionStorage]
        .flatMap((item) => Object.keys(item).map((key) => `${key}:${item.getItem(key)}`))
        .join("\n")
      return storage.includes(canary) || globalThis.location.href.includes(canary)
    }, secretCanary)
    expect(leaked).toBe(false)
  })

  test("REQ:FE-05 Topic Manager keeps recommend immutable and applies versioned operations", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await mockSession(page)
    let patched = false
    await page.route("**/api/tantan/v1/topics", async (route) => {
      if (route.request().method() === "GET") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            version: 7,
            activeFilterId: null,
            topics: [topic("recommend", "推荐"), topic("topic-ai", "AI")],
          }),
        })
      }
      expect(route.request().postDataJSON()).toEqual({
        version: 7,
        operations: [{ op: "hide", topicId: "topic-ai" }],
      })
      patched = true
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          version: 8,
          activeFilterId: null,
          topics: [topic("recommend", "推荐"), topic("topic-ai", "AI", { hidden: true })],
        }),
      })
    })

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv(), "/settings/topics"), {
      waitUntil: "domcontentloaded",
    })
    const recommend = page.getByRole("article").filter({ hasText: "推荐" })
    await expect(recommend.getByRole("button")).toHaveCount(0)
    await page.getByRole("button", { name: "隐藏 AI" }).click()
    await expect.poll(() => patched).toBe(true)
    await expect(page.getByRole("button", { name: "显示 AI" })).toBeVisible()
  })

  test("REQ:FE-04 Mobile RSS add and cancel use the preserved Folo Subscription Store", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockSession(page)
    let added = false
    let removed = false
    await page.route("**/api/folo/subscriptions**", (route) => {
      if (route.request().method() === "GET") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            code: 0,
            data: [
              {
                feedId: "feed-existing",
                userId: "user-flows",
                view: 0,
                category: "技术",
                isPrivate: false,
                hideFromTimeline: false,
                title: null,
                createdAt: "2026-08-09T12:00:00Z",
                feeds: {
                  id: "feed-existing",
                  title: "Existing RSS",
                  url: "https://existing.test/rss",
                  image: null,
                  description: null,
                  ownerUserId: null,
                  errorAt: null,
                  errorMessage: null,
                  siteUrl: "https://existing.test",
                },
              },
            ],
          }),
        })
      }
      if (route.request().method() === "POST") {
        added = true
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            code: 0,
            feed: {
              id: "feed-new",
              title: "New RSS",
              url: "https://new.test/rss",
              image: null,
              description: null,
              ownerUserId: null,
              errorAt: null,
              errorMessage: null,
              siteUrl: "https://new.test",
            },
            list: null,
            unread: {},
          }),
        })
      }
      removed = true
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ code: 0, data: null }),
      })
    })

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv(), "/subscriptions"), {
      waitUntil: "domcontentloaded",
    })
    await expect(page.getByText("Existing RSS", { exact: true })).toBeVisible()
    await page.getByRole("button", { name: "添加订阅" }).click()
    await page.getByLabel("RSS 或网站地址").fill("https://new.test/rss")
    await page.getByRole("button", { name: "确认添加" }).click()
    await expect.poll(() => added).toBe(true)
    await expect(page.getByText("New RSS", { exact: true })).toBeVisible()
    await page.getByRole("button", { name: "取消订阅 New RSS" }).click()
    await expect.poll(() => removed).toBe(true)
    await expect(page.getByText("New RSS", { exact: true })).toHaveCount(0)
  })

  test("REQ:FE-04 collection remains compatible and AI failure keeps original Entry readable", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockSession(page)
    const entryId = "309246809866240003"
    let collectionMethod = ""
    await page.route("**/api/tantan/v1/topics", (route) =>
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
    await page.route("**/api/tantan/v1/home?**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(homeResponse([card(entryId, "Original survives")])),
      }),
    )
    await page.route(`**/api/tantan/v1/entries/${entryId}/enrichment?**`, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          state: "failed",
          data: null,
          error: { code: "AI_PROVIDER_UNAVAILABLE", message: "offline" },
        }),
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
              title: "Original survives",
              url: "https://example.com/original",
              content: "ORIGINAL BODY REMAINS READABLE",
              description: "Original description",
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
            feeds: { id: "feed-collection", type: "feed" },
            settings: null,
          },
        }),
      }),
    )
    await page.route("**/api/folo/reads", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: true }),
      }),
    )
    await page.route("**/api/folo/collections**", (route) => {
      collectionMethod = route.request().method()
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ code: 0, data: null }),
      })
    })

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })
    await page.getByRole("link", { name: "阅读：Original survives" }).click()
    await expect(page.getByText("AI 处理失败，已显示原文。")).toBeVisible()
    await expect(page.getByText("ORIGINAL BODY REMAINS READABLE")).toBeVisible()

    await page.getByRole("button", { name: "收藏" }).click()
    await expect.poll(() => collectionMethod).toBe("POST")
    await expect(page.getByRole("button", { name: "取消收藏" })).toBeVisible()
    await page.getByRole("button", { name: "取消收藏" }).click()
    await expect.poll(() => collectionMethod).toBe("DELETE")
    await expect(page.getByRole("button", { name: "收藏" })).toBeVisible()
  })
})
