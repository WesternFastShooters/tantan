import type { Page } from "@playwright/test"
import { expect, test } from "@playwright/test"

import { buildWebAppURL, resolveDesktopE2EEnv } from "../../../support/env"

const topic = (id: string, name: string) => ({
  id,
  name,
  kind: id === "recommend" ? "core" : "dynamic",
  fixed: id === "recommend",
  pinned: true,
  hidden: false,
  unreadCount: 2,
})

const card = (entryId: string, title: string) => ({
  entryId,
  type: "article" as const,
  title,
  excerpt: `Excerpt ${title}`,
  cover: null,
  source: { id: "source-actions", name: "Action Source", avatar: null },
  publishedAt: "2026-08-09T12:00:00Z",
  topics: [{ id: "topic-ai", name: "AI" }],
  translated: false,
})

const home = (items: ReturnType<typeof card>[], queueId = "queue-default") => ({
  items,
  nextCursor: null,
  queue: {
    id: queueId,
    version: 1,
    generation: `${queueId}-v1`,
    total: items.length,
    unread: items.length,
    finished: true,
    candidateWindowDays: 7,
    generatedAt: "2026-08-09T12:00:00Z",
  },
  queueGeneration: `${queueId}-v1`,
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
        user: { id: "user-actions", name: "Action User", email: null, image: null },
        timezone: "Asia/Shanghai",
      }),
    }),
  )
}

test.describe("Tantan acceptance Home actions", () => {
  test("FE:TC-011 feedback rolls back failures and supports a five-second undo", async ({
    page,
  }) => {
    await mockSession(page)
    const cards = [
      card("feedback-ok", "Feedback succeeds"),
      card("feedback-fail", "Feedback fails"),
      card("feedback-block", "Block this Source"),
    ]
    const actions: string[] = []
    let sourceBlocked = false
    let sourceRestored = false
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
        body: JSON.stringify(home(cards)),
      }),
    )
    await page.route("**/api/tantan/v1/recommendation/feedback", async (route) => {
      const body = route.request().postDataJSON() as { entryId: string; action: string }
      actions.push(`${body.entryId}:${body.action}`)
      if (body.entryId === "feedback-fail") {
        return route.fulfill({ status: 500, contentType: "application/json", body: "{}" })
      }
      if (body.entryId === "feedback-block" && body.action === "block_source") {
        sourceBlocked = true
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ applied: true }),
      })
    })
    await page.route("**/api/tantan/v1/recommendation/blocks/sources/*", (route) => {
      sourceRestored = true
      sourceBlocked = false
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ applied: true }),
      })
    })
    await page.route("**/api/tantan/v1/recommendation/blocks/sources", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          items: sourceBlocked
            ? [
                {
                  sourceId: "source-actions",
                  name: "Action Source",
                  createdAt: "2026-08-09T12:00:00Z",
                },
              ]
            : [],
        }),
      }),
    )

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })
    await page.getByRole("button", { name: "更多操作：Feedback succeeds" }).click()
    await page.getByRole("button", { name: "不感兴趣" }).click()
    await expect(page.getByText("Feedback succeeds", { exact: true })).toHaveCount(0)
    const undoFeedback = page.getByRole("button", { name: "撤销推荐反馈" })
    await expect(undoFeedback).toBeVisible()
    const undoLayer = await undoFeedback.evaluate((element) =>
      Number(getComputedStyle(element.closest("aside")!).zIndex),
    )
    const navigationLayer = await page
      .getByRole("tablist", { name: "主导航" })
      .evaluate((element) => Number(getComputedStyle(element).zIndex))
    expect(undoLayer).toBeGreaterThan(navigationLayer)
    await undoFeedback.click()
    await expect(page.getByText("Feedback succeeds", { exact: true })).toBeVisible()

    await page.getByRole("button", { name: "更多操作：Feedback fails" }).click()
    await page.getByRole("button", { name: "不感兴趣" }).click()
    await expect(page.getByText("Feedback fails", { exact: true })).toBeVisible()

    await page.getByRole("button", { name: "更多操作：Block this Source" }).click()
    await page.getByRole("button", { name: "屏蔽 Source：Action Source" }).click()
    await expect(page.getByText("Block this Source", { exact: true })).toHaveCount(0)
    await page.goto(buildWebAppURL(resolveDesktopE2EEnv(), "/settings/topics"), {
      waitUntil: "domcontentloaded",
    })
    await expect(page.getByRole("heading", { name: "已屏蔽 Source" })).toBeVisible()
    await page.getByRole("button", { name: "恢复 Action Source" }).click()
    await expect.poll(() => sourceRestored).toBe(true)
    await expect(page.getByText("暂无已屏蔽 Source。")).toBeVisible()
    expect(actions).toEqual([
      "feedback-ok:not_interested",
      "feedback-ok:undo",
      "feedback-fail:not_interested",
      "feedback-block:block_source",
    ])
  })

  test("FE:TC-017 active AI Filter can be edited and reset without leaving Home", async ({
    page,
  }) => {
    await mockSession(page)
    let activePrompt: string | null = null
    let filterVersion = 0
    await page.route("**/api/tantan/v1/topics", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          version: 1 + filterVersion,
          topicsRevision: 1 + filterVersion,
          activeFilterId: activePrompt ? `filter-${filterVersion}` : null,
          topics: [topic("recommend", "推荐")],
        }),
      }),
    )
    await page.route("**/api/tantan/v1/home?**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(
          home(
            [card(`filter-card-${filterVersion}`, activePrompt ?? "Default queue")],
            activePrompt ? `queue-${filterVersion}` : "queue-default",
          ),
        ),
      }),
    )
    await page.route("**/api/tantan/v1/filter", async (route) => {
      if (route.request().method() === "DELETE") {
        activePrompt = null
        filterVersion += 1
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            filter: null,
            topics: [topic("recommend", "推荐")],
            topicsRevision: 1 + filterVersion,
            queueId: "queue-default",
            queueGeneration: "queue-default-v1",
          }),
        })
      }
      activePrompt = (route.request().postDataJSON() as { prompt: string }).prompt
      filterVersion += 1
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          filter: {
            id: `filter-${filterVersion}`,
            prompt: activePrompt,
            createdAt: "2026-08-09T12:00:00Z",
          },
          topics: [topic("recommend", "推荐"), topic(`topic-${filterVersion}`, "动态")],
          topicsRevision: 1 + filterVersion,
          queueId: `queue-${filterVersion}`,
          queueGeneration: `queue-${filterVersion}-v1`,
        }),
      })
    })

    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })
    await page.getByRole("button", { name: "AI 智能筛选" }).click()
    await page.getByLabel("筛选要求").fill("多推 Codex")
    await page.getByRole("button", { name: "生成信息流" }).click()
    await expect(page).toHaveURL(/\/$/)
    await expect(page.getByTestId("active-ai-filter")).toContainText("多推 Codex")

    await page.getByRole("button", { name: "编辑筛选" }).click()
    await expect(page.getByLabel("筛选要求")).toHaveValue("多推 Codex")
    await page.getByLabel("筛选要求").fill("只看本地 AI")
    await page.getByRole("button", { name: "生成信息流" }).click()
    await expect(page.getByTestId("active-ai-filter")).toContainText("只看本地 AI")

    await page.getByRole("button", { name: "重置" }).click()
    await expect(page.getByTestId("active-ai-filter")).toHaveCount(0)
    await expect(page.getByText("Default queue", { exact: true })).toBeVisible()
    await expect(page).toHaveURL(/\/$/)
  })
})
