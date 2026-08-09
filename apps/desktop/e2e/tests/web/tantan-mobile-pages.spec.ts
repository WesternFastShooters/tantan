import { expect, test } from "@playwright/test"

import { buildWebAppURL, resolveDesktopE2EEnv } from "../../support/env"

const appURL = () => buildWebAppURL(resolveDesktopE2EEnv())

const bootstrap = async (page: import("@playwright/test").Page) => {
  await page.route("**/api/readyz", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        ready: true,
        checks: {
          sqlite: "ok",
          migrations: "ok",
          secretStore: "ok",
          routePolicy: "ok",
          staticAssets: "ok",
        },
      }),
    }),
  )
  await page.route("**/api/tantan/v1/session", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        user: {
          id: "mobile-pages-user",
          name: "Mobile User",
          email: "mobile@example.com",
          image: null,
        },
        timezone: "Asia/Shanghai",
        csrfToken: "csrf-mobile-pages",
      }),
    }),
  )
}

test.describe("Tantan Folo-Mobile secondary pages", () => {
  test.beforeEach(async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await bootstrap(page)
  })

  test("TASK-06 Discover searches through same-origin Go and renders a subscribable Source", async ({
    page,
  }) => {
    let requestBody: unknown
    await page.route("**/api/folo/discover", async (route) => {
      requestBody = route.request().postDataJSON()
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          code: 0,
          data: [
            {
              feed: {
                type: "feed",
                id: "feed-e2e",
                url: "https://example.com/rss.xml",
                title: "Example Mobile Feed",
                description: "Mobile discovery result",
                siteUrl: "https://example.com",
                image: null,
              },
              subscriptionCount: 23,
              updatesPerWeek: 4,
            },
          ],
        }),
      })
    })

    await page.goto(`${appURL()}discover`, { waitUntil: "domcontentloaded" })
    await page.getByRole("searchbox", { name: "搜索网站或 RSS" }).fill("Example")
    await page.getByRole("button", { name: "搜索", exact: true }).click()

    await expect(page.getByRole("heading", { name: "Example Mobile Feed" })).toBeVisible()
    await expect(page.getByText("Mobile discovery result")).toBeVisible()
    await expect(page.getByRole("button", { name: "订阅", exact: true })).toBeVisible()
    expect(requestBody).toEqual({ keyword: "Example", target: "feeds" })
  })

  test("TASK-06 Settings use grouped Folo-Mobile navigation and every visible destination works", async ({
    page,
  }) => {
    await page.goto(`${appURL()}settings`, { waitUntil: "domcontentloaded" })

    await expect(page.getByText("Mobile User", { exact: true })).toBeVisible()
    await expect(page.locator("[data-settings-group]")).toHaveCount(2)
    await expect(page.locator("body")).not.toContainText(
      /Plan|Power|Wallet|Upgrade|升级|额度|会员/u,
    )

    await page.getByRole("link", { name: /通用/ }).click()
    await expect(page).toHaveURL(/\/settings\/general$/)
    await expect(page.getByRole("heading", { name: "通用" })).toBeVisible()
    const unread = page.getByRole("switch", { name: "仅显示未读" })
    await expect(unread).toHaveAttribute("aria-checked", "false")
    await unread.click()
    await expect(unread).toHaveAttribute("aria-checked", "true")

    await page.getByRole("link", { name: "返回设置" }).click()
    await page.getByRole("link", { name: /外观/ }).click()
    await expect(page).toHaveURL(/\/settings\/appearance$/)
    await page.getByRole("button", { name: "深色主题" }).click()
    await expect(page.getByRole("button", { name: "深色主题" })).toHaveAttribute(
      "aria-pressed",
      "true",
    )
  })

  test("TASK-06 failed unsubscribe is single-flight and restores the subscribed Source", async ({
    page,
  }) => {
    let deleteCalls = 0
    await page.route("**/api/folo/subscriptions**", async (route) => {
      if (route.request().method() === "GET") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            code: 0,
            data: [
              {
                feedId: "feed-rollback",
                userId: "mobile-pages-user",
                view: 0,
                category: "技术",
                isPrivate: false,
                hideFromTimeline: false,
                title: null,
                createdAt: "2026-08-09T12:00:00Z",
                feeds: {
                  id: "feed-rollback",
                  title: "Rollback RSS",
                  url: "https://rollback.test/rss",
                  image: null,
                  description: null,
                  ownerUserId: null,
                  errorAt: null,
                  errorMessage: null,
                  siteUrl: "https://rollback.test",
                },
              },
            ],
          }),
        })
      }
      deleteCalls += 1
      await new Promise((resolve) => setTimeout(resolve, 50))
      return route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ code: 503, message: "取消订阅失败" }),
      })
    })

    await page.goto(`${appURL()}subscriptions`, { waitUntil: "domcontentloaded" })
    await expect(page.getByText("Rollback RSS", { exact: true })).toBeVisible()
    const unsubscribe = page.getByRole("button", { name: "取消订阅 Rollback RSS" })
    await unsubscribe.evaluate((button) => {
      button.click()
      button.click()
    })

    await expect.poll(() => deleteCalls).toBe(1)
    await expect(page.getByRole("alert")).toBeVisible()
    await expect(page.getByText("Rollback RSS", { exact: true })).toBeVisible()
    await expect(unsubscribe).toBeEnabled()
  })
})
