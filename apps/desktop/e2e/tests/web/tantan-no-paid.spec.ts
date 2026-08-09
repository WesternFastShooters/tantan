import { existsSync, readFileSync, unlinkSync } from "node:fs"

import type { Page } from "@playwright/test"
import { expect, test } from "@playwright/test"

import { buildWebAppURL, resolveDesktopE2EEnv } from "../../support/env"

const removedRoutes = [
  { path: "/ai", forbiddenText: /AI Chat|Ask Folo|Chat with/i },
  { path: "/power", forbiddenText: /My Wallet|Withdraw|Power Token/i },
  { path: "/settings/plan", forbiddenText: /Upgrade|Current Plan|Billing Period/i },
] as const

const deniedFoloPath =
  /^\/(?:ai|wallets|payments|referrals|trending)(?:\/|$)|^\/better-auth\/(?:subscription|stripe)(?:\/|$)|^\/rsshub\/use$/

const navigateInApp = async (page: Page, path: string) => {
  await page.evaluate((targetPath) => {
    window.history.pushState({}, "", targetPath)
    window.dispatchEvent(new PopStateEvent("popstate"))
  }, path)
  await page.evaluate(
    () =>
      new Promise<void>((resolve) => {
        requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
      }),
  )
}

test.describe("Tantan no-paid policy", () => {
  test("REQ:FE-01 paid and Folo AI routes expose no product UI or navigation entry", async ({
    page,
  }) => {
    const env = resolveDesktopE2EEnv()
    const productPageModules: string[] = []
    page.on("request", (request) => {
      const path = decodeURIComponent(new URL(request.url()).pathname)
      if (/\/pages\/.*\/(?:ai|power|plan)(?:\/|\.)/.test(path)) productPageModules.push(path)
    })

    await page.goto(buildWebAppURL(env), { waitUntil: "domcontentloaded" })
    await expect(page.locator("main")).toBeVisible()
    for (const name of ["AI Chat", "Power", "Wallet", "Plan", "Upgrade"]) {
      await expect(page.getByRole("link", { name: new RegExp(`^${name}$`, "i") })).toHaveCount(0)
    }

    for (const route of removedRoutes) {
      await navigateInApp(page, route.path)
      await expect(page.locator("body")).not.toContainText(route.forbiddenText)
    }
    expect([...new Set(productPageModules)].sort()).toEqual([])
  })

  test("REQ:FE-01 browser HAR contains zero denied Folo route", async ({ browser }, testInfo) => {
    const env = resolveDesktopE2EEnv()
    const harPath = testInfo.outputPath("tantan-no-paid.har")
    const context = await browser.newContext({
      ignoreHTTPSErrors: true,
      recordHar: { content: "omit", mode: "minimal", path: harPath },
    })
    const page = await context.newPage()

    try {
      await page.goto(buildWebAppURL(env), { waitUntil: "domcontentloaded" })
      await expect(page.locator("main")).toBeVisible()
      for (const route of removedRoutes) {
        await navigateInApp(page, route.path)
      }
      await context.close()
      const har = JSON.parse(readFileSync(harPath, "utf8")) as {
        log?: { entries?: { request?: { url?: string } }[] }
      }
      const violations = (har.log?.entries ?? []).flatMap((entry) => {
        const rawURL = entry.request?.url
        if (!rawURL) return []
        const url = new URL(rawURL)
        return deniedFoloPath.test(url.pathname) ? [`${url.origin}${url.pathname}`] : []
      })

      expect(violations).toEqual([])
    } finally {
      await context.close().catch(() => {})
      if (existsSync(harPath)) unlinkSync(harPath)
    }
  })
})
