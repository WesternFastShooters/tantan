import { expect, test } from "@playwright/test"

import { buildWebAppURL, resolveDesktopE2EEnv } from "../../support/env"

const readyResponse = {
  ready: true,
  checks: {
    sqlite: "ok",
    migrations: "ok",
    secretStore: "ok",
    routePolicy: "ok",
    staticAssets: "ok",
  },
}

const sessionResponse = {
  user: { id: "user-shell", name: "Shell User", email: null, image: null },
  timezone: "Asia/Shanghai",
  csrfToken: "csrf-e2e",
}

const mockSession = async (
  page: import("@playwright/test").Page,
  options: { authenticated?: boolean } = {},
) => {
  const authenticated = options.authenticated ?? true
  await page.route("**/api/readyz", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(readyResponse),
    }),
  )
  await page.route("**/api/tantan/v1/session", (route) =>
    route.fulfill({
      status: authenticated ? 200 : 401,
      contentType: "application/json",
      body: JSON.stringify(
        authenticated
          ? sessionResponse
          : {
              requestId: "req-shell-401",
              error: { code: "AUTH_REQUIRED", message: "需要登录", retryable: false },
            },
      ),
    }),
  )
  await page.route("**/api/auth/folo/providers", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        providers: ["google", "github", "apple", "credential", "token"],
      }),
    }),
  )
  await page.route("**/api/folo/subscriptions**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ code: 0, data: [] }),
    }),
  )
  await page.route("**/api/tantan/v1/topics", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        version: 1,
        topicsRevision: 1,
        activeFilterId: null,
        topics: [
          {
            id: "recommend",
            name: "推荐",
            kind: "core",
            fixed: true,
            pinned: true,
            hidden: false,
            unreadCount: 0,
          },
        ],
      }),
    }),
  )
  await page.route("**/api/tantan/v1/home?**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        items: [],
        nextCursor: null,
        queue: {
          id: "queue-shell",
          version: 1,
          generation: "generation-shell",
          total: 0,
          unread: 0,
          finished: true,
          candidateWindowDays: 7,
          generatedAt: "2026-08-10T00:00:00Z",
        },
        queueGeneration: "generation-shell",
      }),
    }),
  )
}

const appURL = () => buildWebAppURL(resolveDesktopE2EEnv())

test.describe("Tantan Mobile Web shell", () => {
  for (const viewport of [
    { width: 390, height: 844 },
    { width: 430, height: 932 },
  ]) {
    test(`FR-01 ${viewport.width}x${viewport.height} renders exactly four Folo Mobile tabs`, async ({
      page,
    }) => {
      await page.setViewportSize(viewport)
      await mockSession(page)
      await page.goto(appURL(), { waitUntil: "domcontentloaded" })

      const navigation = page.getByRole("tablist", { name: "主导航" })
      await expect(navigation.getByRole("tab")).toHaveCount(4)
      await expect(navigation).toContainText("首页")
      await expect(navigation).toContainText("订阅")
      await expect(navigation).toContainText("发现")
      await expect(navigation).toContainText("设置")
      await expect(page.locator("aside")).toHaveCount(0)
      await expect(page.locator("body")).not.toContainText(/Download Folo|升级会员|Wallet/)
    })
  }

  test("FR-01 wide browsers still render the centered Mobile Web shell, never PC navigation", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await mockSession(page)
    await page.goto(`${appURL()}discover`, { waitUntil: "domcontentloaded" })

    const navigation = page.getByRole("tablist", { name: "主导航" })
    await expect(navigation.getByRole("tab")).toHaveCount(4)
    await expect(page.getByRole("navigation", { name: "Primary navigation" })).toHaveCount(0)
    await expect(page.locator("aside")).toHaveCount(0)
    const width = await page
      .getByTestId("tantan-mobile-shell")
      .evaluate((element) => element.getBoundingClientRect().width)
    expect(width).toBeLessThanOrEqual(560)
  })

  test("FR-02 a 401 preserves returnTo and shows all Folo login methods", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockSession(page, { authenticated: false })
    await page.goto(`${appURL()}subscriptions`, { waitUntil: "domcontentloaded" })

    await expect(page).toHaveURL(/\/login\?returnTo=%2Fsubscriptions$/)
    await expect(page.getByRole("button", { name: "使用 Google 继续" })).toBeVisible()
    await expect(page.getByRole("button", { name: "使用 GitHub 继续" })).toBeVisible()
    await expect(page.getByRole("button", { name: "使用 Apple 继续" })).toBeVisible()
    await expect(page.getByRole("button", { name: "使用 Email 继续" })).toBeVisible()
    await expect(page.getByRole("button", { name: "输入授权令牌继续" })).toBeVisible()
    await expect(page.getByRole("tablist", { name: "主导航" })).toHaveCount(0)
  })

  test("SEC-03 every browser API request is same-origin under /api", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    const browserRequests: string[] = []
    page.on("request", (request) => {
      if (["fetch", "xhr"].includes(request.resourceType())) browserRequests.push(request.url())
    })
    await mockSession(page)
    await page.goto(`${appURL()}discover`, { waitUntil: "networkidle" })

    const origin = new URL(appURL()).origin
    expect(browserRequests.length).toBeGreaterThan(0)
    for (const requestURL of browserRequests) {
      const url = new URL(requestURL)
      expect(url.origin).toBe(origin)
      const isSQLiteWasmAsset =
        url.pathname.endsWith("/wa-sqlite-async.wasm") ||
        /^\/assets\/wa-sqlite-async[.-][\w-]+\.wasm$/u.test(url.pathname)
      if (isSQLiteWasmAsset) continue
      expect(url.pathname.startsWith("/api/")).toBe(true)
    }
  })

  test("FR-01 primary tabs are keyboard reachable and preserve browser history", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockSession(page)
    await page.goto(`${appURL()}discover`, { waitUntil: "domcontentloaded" })

    const subscriptions = page.getByRole("tab", { name: /订阅/ })
    await subscriptions.focus()
    await expect(subscriptions).toBeFocused()
    await page.keyboard.press("Enter")
    await expect(page).toHaveURL(/\/subscriptions$/)
    await page.goBack()
    await expect(page).toHaveURL(/\/discover$/)
  })
})
