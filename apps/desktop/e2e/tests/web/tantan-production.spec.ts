import type { Page, Route } from "@playwright/test"
import { expect, test } from "@playwright/test"

import { expectDialogToSpanViewport, expectVisibleIconGlyph } from "../../support/visual-assertions"

const topic = (id: string, name: string) => ({
  id,
  name,
  kind: id === "recommend" ? "core" : "dynamic",
  fixed: id === "recommend",
  pinned: true,
  hidden: false,
  unreadCount: 1,
})

const card = (entryId: string, title: string) => ({
  entryId,
  type: "article",
  title,
  excerpt: "Production PWA acceptance fixture",
  cover: null,
  source: { id: "feed-production", name: "Production Source", avatar: null },
  publishedAt: "2026-08-10T00:00:00Z",
  topics: [{ id: "topic-ai", name: "AI" }],
  translated: false,
})

const fulfill = (route: Route, body: unknown, status = 200) =>
  route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Cache-Control": "no-store" },
    body: JSON.stringify(body),
  })

const installProductionAPI = async (page: Page) => {
  let authenticated = false
  let filtered = false
  let read = false

  await page.route("**/api/**", (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const { pathname } = url

    if (pathname === "/api/readyz") {
      return fulfill(route, {
        ready: true,
        checks: { sqlite: "ok", migrations: "ok", keychain: "ok" },
      })
    }
    if (pathname === "/api/tantan/v1/session") {
      return authenticated
        ? fulfill(route, {
            user: {
              id: "production-user",
              name: "Production User",
              email: "production@example.com",
              image: null,
            },
            timezone: "Asia/Shanghai",
            csrfToken: "csrf-production",
          })
        : fulfill(
            route,
            {
              requestId: "production-unauthorized",
              error: { code: "AUTH_REQUIRED", message: "需要登录", retryable: false },
            },
            401,
          )
    }
    if (pathname === "/api/auth/folo/providers") {
      return fulfill(route, {
        providers: ["google", "github", "apple", "credential", "token"],
      })
    }
    if (pathname === "/api/tantan/v1/topics") {
      return fulfill(route, {
        version: filtered ? 2 : 1,
        topicsRevision: filtered ? 2 : 1,
        activeFilterId: filtered ? "filter-production" : null,
        topics: [topic("recommend", "推荐"), topic("topic-ai", "AI")],
      })
    }
    if (pathname === "/api/tantan/v1/home") {
      const item = filtered
        ? card("production-filtered", "Filtered production card")
        : card("production-default", "Production card")
      const items = read && filtered ? [] : [item]
      return fulfill(route, {
        items,
        nextCursor: null,
        queue: {
          id: filtered ? "queue-production-filtered" : "queue-production-default",
          version: filtered ? 2 : 1,
          generation: filtered ? "generation-production-filtered" : "generation-production-default",
          total: items.length,
          unread: items.length,
          finished: true,
          candidateWindowDays: 7,
          generatedAt: "2026-08-10T00:00:00Z",
        },
        queueGeneration: filtered
          ? "generation-production-filtered"
          : "generation-production-default",
      })
    }
    if (pathname === "/api/tantan/v1/search") {
      return fulfill(route, {
        items: [card("production-search", "Production search result")],
        nextCursor: null,
        indexStatus: "ready",
      })
    }
    if (pathname === "/api/tantan/v1/filter" && request.method() === "PUT") {
      filtered = true
      return fulfill(route, {
        filter: {
          id: "filter-production",
          prompt: "多推 AI 工具",
          createdAt: "2026-08-10T00:00:00Z",
        },
        topics: [topic("recommend", "推荐"), topic("topic-ai", "AI")],
        topicsRevision: 2,
        queueId: "queue-production-filtered",
        queueGeneration: "generation-production-filtered",
      })
    }
    if (pathname === "/api/folo/entries") {
      return fulfill(route, {
        data: {
          entries: {
            id: "production-filtered",
            title: "Filtered production card",
            url: "https://example.com/production",
            content: "Production detail body",
            description: "Production detail description",
            guid: "production-filtered",
            author: "Production Author",
            authorUrl: null,
            authorAvatar: null,
            insertedAt: "2026-08-10T00:00:00Z",
            publishedAt: "2026-08-10T00:00:00Z",
            media: null,
            categories: null,
            attachments: null,
            extra: null,
            language: "en",
            read: false,
          },
          feeds: { id: "feed-production", type: "feed", title: "Production Source" },
          settings: null,
        },
      })
    }
    if (pathname.endsWith("/enrichment")) {
      return fulfill(route, { state: "pending", data: null, error: null })
    }
    if (pathname === "/api/folo/collections") {
      return fulfill(route, { code: 0, data: false })
    }
    if (pathname === "/api/folo/reads") {
      read = true
      return fulfill(route, { code: 0, data: true })
    }
    if (pathname === "/api/folo/subscriptions") {
      return fulfill(route, {
        code: 0,
        data: [
          {
            feedId: "feed-production",
            userId: "production-user",
            view: 0,
            category: "技术",
            isPrivate: false,
            hideFromTimeline: false,
            title: null,
            createdAt: "2026-08-10T00:00:00Z",
            feeds: {
              id: "feed-production",
              title: "Production RSS",
              url: "https://example.com/rss.xml",
              image: null,
              description: "Production subscription",
              ownerUserId: null,
              errorAt: null,
              errorMessage: null,
              siteUrl: "https://example.com",
            },
          },
        ],
      })
    }
    if (pathname === "/api/folo/discover") {
      return fulfill(route, {
        code: 0,
        data: [
          {
            feed: {
              type: "feed",
              id: "feed-discovered",
              url: "https://example.com/discovered.xml",
              title: "Production Discover Result",
              description: "Discover works in the production PWA",
              siteUrl: "https://example.com",
              image: null,
            },
            subscriptionCount: 12,
            updatesPerWeek: 3,
          },
        ],
      })
    }
    if (pathname === "/api/tantan/v1/recommendation/blocks/sources") {
      return fulfill(route, { items: [] })
    }
    return fulfill(route, { code: 0, data: [] })
  })

  return {
    authenticate: () => {
      authenticated = true
    },
  }
}

test("production Mobile PWA covers login and every phase-one route across restart", async ({
  page,
}, testInfo) => {
  const state = await installProductionAPI(page)
  const businessRequests: string[] = []
  page.on("request", (request) => {
    if (["fetch", "xhr"].includes(request.resourceType())) businessRequests.push(request.url())
  })

  await page.goto("/subscriptions", { waitUntil: "domcontentloaded" })
  await expect(page).toHaveURL(/\/login\?returnTo=%2Fsubscriptions$/)
  for (const name of [
    "使用 Google 继续",
    "使用 GitHub 继续",
    "使用 Apple 继续",
    "使用 Email 继续",
    "输入授权令牌继续",
  ]) {
    await expectVisibleIconGlyph(page.getByRole("button", { name }))
  }

  state.authenticate()
  await page.goto("/", { waitUntil: "domcontentloaded" })
  expect(page.viewportSize()).toEqual(testInfo.project.use.viewport)
  await expect(page.getByRole("tablist", { name: "主导航" }).getByRole("tab")).toHaveCount(4)
  await expect(page.getByText("Production card", { exact: true })).toBeVisible()
  await expectVisibleIconGlyph(page.getByRole("button", { name: "搜索内容" }))
  await expectVisibleIconGlyph(page.getByRole("button", { name: "AI 智能筛选" }))
  await page.getByRole("tab", { name: "AI" }).click()
  await expect(page.getByRole("tab", { name: "AI" })).toHaveAttribute("aria-selected", "true")

  await page.getByRole("button", { name: "搜索内容" }).click()
  await page.getByLabel("搜索订阅内容").fill("Production")
  await expect(page.getByText("Production search result", { exact: true })).toBeVisible()
  await page.getByRole("button", { name: "返回" }).click()

  await page.getByRole("button", { name: "AI 智能筛选" }).click()
  await expectDialogToSpanViewport(page, page.getByRole("dialog", { name: "AI 智能筛选" }))
  await page.getByLabel("筛选要求").fill("多推 AI 工具")
  await page.getByRole("button", { name: "生成信息流" }).click()
  await expect(page.getByText("Filtered production card", { exact: true })).toBeVisible()
  await page.getByRole("link", { name: "阅读：Filtered production card" }).click()
  await expect(page.getByText("Production detail body", { exact: true })).toBeVisible()
  const entryBackButton = page.getByRole("button", { name: "返回首页" })
  await expectVisibleIconGlyph(entryBackButton)
  await entryBackButton.click()

  await page.getByRole("tab", { name: /订阅/ }).click()
  await expect(page.getByText("Production RSS", { exact: true })).toBeVisible()
  await page.getByRole("tab", { name: /发现/ }).click()
  await page.getByRole("searchbox", { name: "搜索网站或 RSS" }).fill("Production")
  await page.getByRole("button", { name: "搜索", exact: true }).click()
  await expect(page.getByText("Production Discover Result", { exact: true })).toBeVisible()
  await page.getByRole("tab", { name: /设置/ }).click()
  await expect(page.getByText("Production User", { exact: true })).toBeVisible()
  await expect(page.locator("body")).not.toContainText(/Plan|Power|Wallet|Upgrade|升级会员/u)

  await page.reload({ waitUntil: "domcontentloaded" })
  await expect(page.getByText("Production User", { exact: true })).toBeVisible()
  await page.goto("/", { waitUntil: "domcontentloaded" })
  await expect(page.getByTestId("active-ai-filter")).toContainText("AI 智能筛选已启用")

  const currentOrigin = new URL(page.url()).origin
  expect(businessRequests.length).toBeGreaterThan(0)
  for (const request of businessRequests) {
    const url = new URL(request)
    expect(url.origin).toBe(currentOrigin)
  }
  expect((await page.request.get("/manifest.webmanifest")).status()).toBe(200)
  expect((await page.request.get("/sw.js")).status()).toBe(200)

  const stylesheetHrefs = await page
    .locator('link[rel="stylesheet"]')
    .evaluateAll((links) => links.map((link) => (link as HTMLLinkElement).href))
  let fontReference: string | undefined
  let fontStylesheetURL: string | undefined
  for (const stylesheetHref of stylesheetHrefs) {
    const stylesheetResponse = await page.request.get(stylesheetHref)
    expect(stylesheetResponse.status()).toBe(200)
    fontReference = (await stylesheetResponse.text()).match(
      /url\(((?:\.\/)?files\/sn-pro-latin-400-normal\.woff2)\)/u,
    )?.[1]
    if (fontReference) {
      fontStylesheetURL = stylesheetResponse.url()
      break
    }
  }
  expect(fontReference).toBeTruthy()
  const fontResponse = await page.request.get(new URL(fontReference!, fontStylesheetURL).href)
  expect(fontResponse.status()).toBe(200)
  expect(fontResponse.headers()["content-type"]).toContain("font/woff2")
  expect((await fontResponse.body()).subarray(0, 4).toString("ascii")).toBe("wOF2")
})

test("BUG:pwa-stale-assets activates the latest Service Worker and never caches authenticated APIs", async ({
  browser,
}, testInfo) => {
  const viewport = testInfo.project.use.viewport as { width: number; height: number }
  const context = await browser.newContext({
    baseURL: "http://127.0.0.1:4173",
    viewport,
    screen: viewport,
    deviceScaleFactor: 2,
    hasTouch: true,
    isMobile: true,
    serviceWorkers: "allow",
  })
  const page = await context.newPage()

  try {
    await page.goto("/login", { waitUntil: "domcontentloaded" })
    await expect
      .poll(() =>
        page.evaluate(async () => {
          const registration = await globalThis.navigator.serviceWorker.getRegistration()
          return registration?.active?.scriptURL.endsWith("/sw.js") ?? false
        }),
      )
      .toBe(true)

    await expect
      .poll(() =>
        page.evaluate(
          () =>
            globalThis.navigator.serviceWorker.controller?.scriptURL.endsWith("/sw.js") ?? false,
        ),
      )
      .toBe(true)

    const workerSource = await (await page.request.get("/sw.js")).text()
    expect(workerSource).toContain("skipWaiting")
    expect(workerSource).toMatch(/clients\.claim/u)

    await page.evaluate(async () => {
      await fetch("/api/tantan/v1/session", { credentials: "include" }).catch(() => undefined)
    })

    const sensitiveCaches = await page.evaluate(async () => {
      const entries: string[] = []
      for (const name of await caches.keys()) {
        const cache = await caches.open(name)
        for (const request of await cache.keys()) {
          if (new URL(request.url).pathname.startsWith("/api/")) entries.push(request.url)
        }
      }
      return entries
    })
    expect(sensitiveCaches).toEqual([])
  } finally {
    await context.close()
  }
})
