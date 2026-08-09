import { expect, test } from "@playwright/test"

import { buildWebAppURL, resolveDesktopE2EEnv } from "../../../support/env"

test("FE:TC-020 Topic pin, hide/show and ordering are versioned while recommend remains immutable", async ({
  page,
}) => {
  await page.route("http://127.0.0.1:3000/readyz", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        ready: true,
        checks: { sqlite: "ok", migrations: "ok", keychain: "ok" },
      }),
    }),
  )
  await page.route("http://127.0.0.1:3000/tantan/v1/session", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        user: { id: "user-topics", name: "Topics User", email: null, image: null },
        timezone: "Asia/Shanghai",
      }),
    }),
  )
  let version = 7
  let topics = [
    {
      id: "recommend",
      name: "推荐",
      kind: "core",
      fixed: true,
      pinned: true,
      hidden: false,
      unreadCount: 9,
    },
    {
      id: "topic-ai",
      name: "AI",
      kind: "dynamic",
      fixed: false,
      pinned: true,
      hidden: false,
      unreadCount: 4,
    },
    {
      id: "topic-web",
      name: "Web",
      kind: "dynamic",
      fixed: false,
      pinned: false,
      hidden: false,
      unreadCount: 3,
    },
  ]
  const requestVersions: number[] = []
  await page.route("http://127.0.0.1:3000/tantan/v1/topics", async (route) => {
    if (route.request().method() === "GET") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ version, activeFilterId: null, topics }),
      })
    }
    const request = route.request().postDataJSON() as {
      version: number
      operations: Array<{ op: string; topicId: string; afterTopicId?: string | null }>
    }
    requestVersions.push(request.version)
    const operation = request.operations[0]!
    topics = topics.map((item) =>
      item.id !== operation.topicId
        ? item
        : {
            ...item,
            ...(operation.op === "pin" ? { pinned: true } : {}),
            ...(operation.op === "unpin" ? { pinned: false } : {}),
            ...(operation.op === "hide" ? { hidden: true } : {}),
            ...(operation.op === "show" ? { hidden: false } : {}),
          },
    )
    if (operation.op === "move") {
      const moving = topics.find((item) => item.id === operation.topicId)!
      topics = topics.filter((item) => item.id !== operation.topicId)
      const afterIndex = topics.findIndex((item) => item.id === operation.afterTopicId)
      topics.splice(afterIndex + 1, 0, moving)
    }
    version += 1
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ version, activeFilterId: null, topics }),
    })
  })
  await page.route("http://127.0.0.1:3000/tantan/v1/recommendation/blocks/sources", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ items: [] }),
    }),
  )

  await page.goto(buildWebAppURL(resolveDesktopE2EEnv(), "/settings/topics"), {
    waitUntil: "domcontentloaded",
  })
  await expect(
    page.getByRole("article").filter({ hasText: "推荐" }).getByRole("button"),
  ).toHaveCount(0)
  await page.getByRole("button", { name: "取消固定 AI" }).click()
  await expect(page.getByRole("button", { name: "固定 AI" })).toBeVisible()
  await page.getByRole("button", { name: "固定 AI" }).click()
  await page.getByRole("button", { name: "隐藏 AI" }).click()
  await expect(page.getByRole("button", { name: "显示 AI" })).toBeVisible()
  await page.getByRole("button", { name: "显示 AI" }).click()
  await page.getByRole("button", { name: "下移 AI" }).click()
  await expect(page.getByRole("article").nth(2)).toContainText("AI")
  expect(requestVersions).toEqual([7, 8, 9, 10, 11])
})
