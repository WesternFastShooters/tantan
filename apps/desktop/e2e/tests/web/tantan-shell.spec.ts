import { expect, test } from "@playwright/test"

import { buildWebAppURL, resolveDesktopE2EEnv } from "../../support/env"

const readyResponse = {
  ready: true,
  checks: { sqlite: "ok", migrations: "ok", keychain: "ok" },
}

const sessionResponse = {
  user: { id: "user-shell", name: "Shell User", email: null, image: null },
  timezone: "Asia/Shanghai",
}

const mockLocalAPI = async (page: import("@playwright/test").Page) => {
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

test.describe("Tantan responsive shell", () => {
  test("REQ:FE-02 mobile root renders Home and a three-item bottom navigation", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockLocalAPI(page)
    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })

    await expect(page.getByRole("heading", { name: "今日推荐" })).toBeVisible()
    const navigation = page.getByRole("navigation", { name: "Mobile navigation" })
    await expect(navigation.getByRole("link")).toHaveCount(3)
    await expect(navigation).toContainText("首页")
    await expect(navigation).toContainText("订阅")
    await expect(navigation).toContainText("设置")
    await expect(page.locator("body")).not.toContainText(/Download Folo|下载 Folo/)
  })

  test("REQ:FE-02 desktop renders the same primary routes on the left", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await mockLocalAPI(page)
    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })

    const navigation = page.getByRole("navigation", { name: "Primary navigation" })
    await expect(navigation.getByRole("link")).toHaveCount(3)
    await navigation.getByRole("link", { name: "订阅" }).click()
    await expect(page).toHaveURL(/\/subscriptions$/)
    await navigation.getByRole("link", { name: "设置" }).click()
    await expect(page).toHaveURL(/\/settings(?:\/general)?$/)
    await page.goBack()
    await expect(page).toHaveURL(/\/subscriptions$/)
    await expect(navigation.getByRole("link", { name: "订阅" })).toHaveAttribute(
      "aria-current",
      "page",
    )
  })

  test("REQ:FE-02 800px uses the collapsed PC navigation rather than Mobile navigation", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 800, height: 900 })
    await mockLocalAPI(page)
    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })

    await expect(page.getByRole("navigation", { name: "Primary navigation" })).toBeVisible()
    await expect(page.getByRole("navigation", { name: "Mobile navigation" })).toHaveCount(0)
  })

  test("REQ:FE-02 unavailable Go keeps Home visible and offers a working retry", async ({
    page,
  }) => {
    let available = false
    await page.route("http://127.0.0.1:3000/readyz", (route) => {
      if (!available) return route.abort("connectionrefused")
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(readyResponse),
      })
    })
    await page.route("http://127.0.0.1:3000/tantan/v1/session", (route) => route.abort())
    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })

    await expect(page.getByRole("heading", { name: "今日推荐" })).toBeVisible()
    const alert = page.getByRole("alert")
    await expect(alert).toContainText("本地服务未启动")
    available = true
    await alert.getByRole("button", { name: "重试" }).click()
    await expect(alert).toHaveCount(0)
  })

  test("REQ:FE-02 session 401 clears the shell and returns to local login", async ({ page }) => {
    await page.route("http://127.0.0.1:3000/readyz", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(readyResponse),
      }),
    )
    await page.route("http://127.0.0.1:3000/tantan/v1/session", (route) =>
      route.fulfill({
        status: 401,
        contentType: "application/json",
        body: JSON.stringify({
          requestId: "req-shell-401",
          error: { code: "AUTH_REQUIRED", message: "Authentication required" },
        }),
      }),
    )
    await page.route("http://127.0.0.1:3000/auth/folo/start", (route) =>
      route.fulfill({ status: 200, contentType: "text/plain", body: "login-started" }),
    )
    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })

    await expect(page).toHaveURL(/\/login$/)
    const loginButton = page.getByRole("button", { name: "使用 Folo 账号登录" })
    await expect(loginButton).toBeVisible()
    const startRequest = page.waitForRequest("http://127.0.0.1:3000/auth/folo/start")
    await loginButton.click()
    await startRequest
    await expect(page).toHaveURL("http://127.0.0.1:3000/auth/folo/start")
  })

  test("REQ:FE-02 primary navigation is keyboard reachable", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await mockLocalAPI(page)
    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })

    const settingsLink = page
      .getByRole("navigation", { name: "Primary navigation" })
      .getByRole("link", { name: "设置" })
    await settingsLink.focus()
    await expect(settingsLink).toBeFocused()
    await page.keyboard.press("Enter")
    await expect(page).toHaveURL(/\/settings$/)
    await expect(settingsLink).toHaveAttribute("aria-current", "page")
  })
})
