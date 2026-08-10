import type { Page } from "@playwright/test"
import { expect, test } from "@playwright/test"

import { buildWebAppURL, resolveDesktopE2EEnv } from "../../../support/env"

const entry = (entryId: string, title: string) => ({
  entryId,
  type: "article" as const,
  title,
  excerpt: `Excerpt ${title}`,
  cover: null,
  source: { id: `source-${entryId}`, name: "Search Source", avatar: null },
  publishedAt: "2026-08-09T12:00:00Z",
  topics: [{ id: "topic-ai", name: "AI" }],
  translated: title.includes("译文"),
})

const mockSession = async (page: Page) => {
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
        user: { id: "user-search-detail", name: "Search User", email: null, image: null },
        timezone: "Asia/Shanghai",
      }),
    }),
  )
}

test.describe("Tantan acceptance Search and Detail", () => {
  test("FE:TC-012 search covers seven indexed fields over stable cursor pages", async ({
    page,
  }) => {
    await mockSession(page)
    const expected = [
      "已读命中",
      "未读命中",
      "原文命中",
      "译文命中",
      "Source 命中",
      "Topic 命中",
      "Tag 命中",
    ]
    await page.route("**/api/tantan/v1/search?**", (route) => {
      const cursor = new URL(route.request().url()).searchParams.get("cursor")
      const items = cursor
        ? [
            entry("search-original", "原文命中"),
            entry("search-source", "Source 命中"),
            entry("search-topic", "Topic 命中"),
            entry("search-tag", "Tag 命中"),
          ]
        : [
            entry("search-read", "已读命中"),
            entry("search-unread", "未读命中"),
            entry("search-original", "原文命中"),
            entry("search-translation", "译文命中"),
          ]
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          items,
          nextCursor: cursor ? null : "search-cursor-page-2",
          indexStatus: "ready",
        }),
      })
    })

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv(), "/search"), {
      waitUntil: "domcontentloaded",
    })
    await page.getByLabel("搜索订阅内容").fill("needle")
    await expect(page.getByText("已读命中", { exact: true })).toBeVisible()
    await page.getByRole("button", { name: "加载更多" }).click()
    for (const title of expected) {
      await expect(page.getByText(title, { exact: true })).toBeVisible()
    }
    await expect(page.getByText("原文命中", { exact: true })).toHaveCount(1)
    await expect(page.getByText("找到 7 条结果", { exact: true })).toBeVisible()
    await expect(page.getByRole("button", { name: "加载更多" })).toHaveCount(0)
  })

  test("FE:TC-015 Detail switches translation/original, exposes summary, collection and a safe source link", async ({
    page,
  }) => {
    await mockSession(page)
    const entryId = "309246809866240002"
    let collectionMethod = ""
    let collectionMutations = 0
    await page.route("**/api/folo/entries?**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            entries: {
              id: entryId,
              title: "Original title",
              url: "https://example.com/safe-source",
              content: "ORIGINAL DETAIL BODY",
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
            feeds: { id: "feed-detail", type: "feed" },
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
      if (collectionMethod === "GET") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ code: 0, data: false }),
        })
      }
      collectionMutations += 1
      if (collectionMutations === 1) {
        return route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({
            requestId: "collection-failure",
            error: { code: "UPSTREAM_UNAVAILABLE", message: "收藏服务暂不可用", retryable: true },
          }),
        })
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ code: 0, data: null }),
      })
    })
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
            contentZh: "中文详情正文",
            summaryZh: "中文摘要",
            keyPoints: ["要点一", "要点二"],
          },
          error: null,
        }),
      }),
    )

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv(), `/entries/${entryId}`), {
      waitUntil: "domcontentloaded",
    })
    await expect(page.getByText("中文详情正文", { exact: true })).toBeVisible()
    await expect(page.getByText("ORIGINAL DETAIL BODY", { exact: true })).toHaveCount(0)
    await expect(page.getByText("中文摘要", { exact: true })).toBeVisible()
    await expect(page.getByText("要点一", { exact: true })).toBeVisible()
    await page.getByRole("button", { name: "显示原文" }).click()
    await expect(page.getByText("ORIGINAL DETAIL BODY", { exact: true })).toBeVisible()
    await page.getByRole("button", { name: "显示中文" }).click()
    await expect(page.getByText("中文详情正文", { exact: true })).toBeVisible()
    const sourceLink = page.getByRole("link", { name: "原文" })
    await expect(sourceLink).toHaveAttribute("target", "_blank")
    await expect(sourceLink).toHaveAttribute("rel", /noreferrer/)
    await expect(sourceLink).toHaveAttribute("rel", /noopener/)
    await page.getByRole("button", { name: "收藏" }).evaluate((button) => {
      button.click()
      button.click()
    })
    await expect.poll(() => collectionMutations).toBe(1)
    await expect(page.getByRole("alert")).toContainText("收藏服务暂不可用")
    await expect(page.getByRole("button", { name: "收藏" })).toBeVisible()

    await page.getByRole("button", { name: "收藏" }).click()
    await expect.poll(() => collectionMethod).toBe("POST")
    await expect.poll(() => collectionMutations).toBe(2)
    await expect(page.getByRole("button", { name: "取消收藏" })).toBeVisible()
  })
})
