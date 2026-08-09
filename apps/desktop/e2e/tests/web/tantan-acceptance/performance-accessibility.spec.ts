import AxeBuilder from "@axe-core/playwright"
import type { Page } from "@playwright/test"
import { expect, test } from "@playwright/test"

import { buildWebAppURL, resolveDesktopE2EEnv } from "../../../support/env"

const mockHome = async (page: Page, count: number) => {
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
        user: { id: "user-performance", name: "Performance User", email: null, image: null },
        timezone: "Asia/Shanghai",
      }),
    }),
  )
  await page.route("**/api/tantan/v1/topics", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        version: 1,
        activeFilterId: null,
        topics: [
          {
            id: "recommend",
            name: "推荐",
            kind: "core",
            fixed: true,
            pinned: true,
            hidden: false,
            unreadCount: count,
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
        items: Array.from({ length: count }, (_, index) => ({
          entryId: `performance-${String(index).padStart(3, "0")}`,
          type: index % 4 === 0 ? "image" : "article",
          title: `Performance card ${index}`,
          excerpt: `Fixed performance fixture ${index}`,
          cover: null,
          source: { id: `source-${index % 8}`, name: `Source ${index % 8}`, avatar: null },
          publishedAt: "2026-08-09T12:00:00Z",
          topics: [{ id: "topic-performance", name: "Performance" }],
          translated: false,
        })),
        nextCursor: null,
        queue: {
          id: "queue-performance",
          version: 1,
          total: count,
          unread: count,
          finished: true,
          candidateWindowDays: 7,
          generatedAt: "2026-08-09T12:00:00Z",
        },
      }),
    }),
  )
}

test.describe("Tantan acceptance Performance and Accessibility", () => {
  test("FE:TC-018 a 60-card queue stays virtualized within the 500-candidate pipeline", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await mockHome(page, 60)
    const started = Date.now()
    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })
    await expect(page.getByText("Performance card 0", { exact: true })).toBeVisible()
    expect(Date.now() - started).toBeLessThan(5_000)
    const rendered = await page.getByTestId("home-card").count()
    expect(rendered).toBeGreaterThan(0)
    expect(rendered).toBeLessThan(60)
    expect(rendered).toBeLessThanOrEqual(30)
  })

  test("FE:TC-021 has no critical axe violation, honors reduced motion and keeps sensitive caches empty", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.emulateMedia({ reducedMotion: "reduce" })
    await mockHome(page, 4)
    await page.goto(buildWebAppURL(resolveDesktopE2EEnv()), { waitUntil: "domcontentloaded" })
    await expect(page.getByRole("navigation", { name: "Mobile navigation" })).toBeVisible()

    const reduced = await page
      .getByTestId("home-card")
      .first()
      .evaluate((element) => {
        const style = getComputedStyle(element)
        return {
          duration: style.transitionDuration,
          preference: matchMedia("(prefers-reduced-motion: reduce)").matches,
        }
      })
    expect(reduced.preference).toBe(true)
    expect(Number.parseFloat(reduced.duration)).toBeLessThanOrEqual(0.00001)

    const accessibility = await new AxeBuilder({ page }).analyze()
    expect(accessibility.violations.filter((violation) => violation.impact === "critical")).toEqual(
      [],
    )

    const sensitiveCacheEntries = await page.evaluate(async () => {
      const values: string[] = []
      for (const name of await caches.keys()) {
        const cache = await caches.open(name)
        for (const request of await cache.keys()) {
          if (request.url.includes("/tantan/v1/") || request.url.includes("/auth/")) {
            values.push(request.url)
          }
        }
      }
      return values
    })
    expect(sensitiveCacheEntries).toEqual([])

    await page.keyboard.press("Tab")
    const focused = await page.evaluate(() => document.activeElement?.tagName)
    expect(["A", "BUTTON", "TEXTAREA", "INPUT"]).toContain(focused)
  })
})
