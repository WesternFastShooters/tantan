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
    total: items.length,
    unread: items.length,
    finished: true,
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

test.describe("Tantan phase-one flows", () => {
  test("REQ:FE-04 Mobile search waits 250ms, pages all indexed fields, and preserves Home state", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockSession(page)
    let searchInputAt = 0
    let searchDelay = 0
    let homeMutationCount = 0
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
    await page.route("http://127.0.0.1:3000/tantan/v1/home?**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(homeResponse([card("301", "Home AI")])),
      }),
    )
    await page.route("http://127.0.0.1:3000/tantan/v1/filter", (route) => {
      homeMutationCount += 1
      return route.abort()
    })
    await page.route("http://127.0.0.1:3000/tantan/v1/recommendation/feedback", (route) => {
      homeMutationCount += 1
      return route.abort()
    })
    await page.route("http://127.0.0.1:3000/tantan/v1/search?**", (route) => {
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

  test("REQ:FE-05 desktop Provider uses a preset endpoint and never persists or echoes the Key", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await mockSession(page)
    const secretCanary = "sk-flow-secret-canary-123456"
    let saved = false
    await page.route("http://127.0.0.1:3000/tantan/v1/settings/ai-provider", async (route) => {
      if (route.request().method() === "GET") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            configured: false,
            providerId: null,
            model: null,
            baseUrl: null,
            hasApiKey: false,
            keyFingerprint: null,
          }),
        })
      }
      const body = route.request().postDataJSON() as Record<string, unknown>
      expect(body).toEqual({ providerId: "openai", model: "gpt-5-mini", apiKey: secretCanary })
      saved = true
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          configured: true,
          providerId: "openai",
          model: "gpt-5-mini",
          baseUrl: "https://api.openai.com/v1",
          hasApiKey: true,
          keyFingerprint: "a1b2c3d4e5f6",
        }),
      })
    })
    await page.route("http://127.0.0.1:3000/tantan/v1/settings/ai-provider/test", (route) => {
      const body = route.request().postDataJSON() as Record<string, unknown>
      expect(body.apiKey).toBe(secretCanary)
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true, latencyMs: 42, model: "gpt-5-mini" }),
      })
    })

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv(), "/settings/ai"), {
      waitUntil: "domcontentloaded",
    })
    await expect(page.getByLabel("内置 Endpoint")).toHaveAttribute("readonly", "")
    await expect(page.getByLabel("内置 Endpoint")).toHaveValue("https://api.openai.com/v1")
    await page.getByLabel("模型").fill("gpt-5-mini")
    await page.getByLabel("API Key").fill(secretCanary)
    await page.getByRole("button", { name: "测试连接" }).click()
    await expect(page.getByRole("status")).toContainText("连接成功")
    await expect(page.getByLabel("API Key")).toHaveValue(secretCanary)
    await page.getByRole("button", { name: "保存配置" }).click()
    await expect.poll(() => saved).toBe(true)
    await expect(page.getByLabel("API Key")).toHaveValue("")
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
    await page.route("http://127.0.0.1:3000/tantan/v1/topics", async (route) => {
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
    await page.route("http://localhost:3000/subscriptions**", (route) => {
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
    const entryId = "41147805272531998"
    let collectionMethod = ""
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
        body: JSON.stringify(homeResponse([card(entryId, "Original survives")])),
      }),
    )
    await page.route(`http://127.0.0.1:3000/tantan/v1/entries/${entryId}/enrichment?**`, (route) =>
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
    await page.route("http://localhost:3000/entries?**", (route) =>
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
    await page.route("http://localhost:3000/reads", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: true }),
      }),
    )
    await page.route("http://localhost:3000/collections", (route) => {
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
