import type { Page } from "@playwright/test"
import { expect, test } from "@playwright/test"

import { buildWebAppURL, resolveDesktopE2EEnv } from "../../../support/env"

const entryId = "309246809866240001"

const mockShell = async (page: Page) => {
  await page.route("**/api/readyz", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        ready: true,
        checks: { sqlite: "ok", migrations: "ok", keychain: "ok" },
      }),
    }),
  )
  await page.route("**/api/tantan/v1/session", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        user: { id: "user-entry-retry", name: "Retry User", email: null, image: null },
        timezone: "Asia/Shanghai",
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
            title: "Retry translation",
            url: "https://example.com/retry",
            content: "ORIGINAL CONTENT",
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
          feeds: { id: "feed-retry", type: "feed" },
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
  await page.route("**/api/folo/collections?**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ code: 0, data: false }),
    }),
  )
}

test("FE:TC-013 failed enrichment keeps original content and exposes retry", async ({ page }) => {
  await mockShell(page)
  let retried = false
  let requestedFields: string[] = []
  await page.route(`**/api/tantan/v1/entries/${entryId}/enrichment?**`, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        state: retried ? "queued" : "failed",
        data: null,
        error: retried ? null : { code: "AI_PROVIDER_UNAVAILABLE", message: "offline" },
      }),
    }),
  )
  await page.route(`**/api/tantan/v1/entries/${entryId}/enrichment`, (route) => {
    retried = true
    requestedFields = route.request().postDataJSON().fields
    return route.fulfill({
      status: 202,
      contentType: "application/json",
      body: JSON.stringify({ state: "queued", data: null, error: null }),
    })
  })

  await page.goto(buildWebAppURL(resolveDesktopE2EEnv(), `/entries/${entryId}`), {
    waitUntil: "domcontentloaded",
  })
  await expect(page.getByText("ORIGINAL CONTENT", { exact: true })).toBeVisible()
  await expect(page.getByText("AI 处理失败，已显示原文。")).toBeVisible()
  await page.getByRole("button", { name: "重试翻译与摘要" }).click()
  await expect.poll(() => retried).toBe(true)
  expect(requestedFields).toEqual(["translation", "summary", "keyPoints"])
  await expect(page.getByText("AI 处理中…原文仍可正常阅读。")).toBeVisible()
  await expect(page.getByText("ORIGINAL CONTENT", { exact: true })).toBeVisible()
})
