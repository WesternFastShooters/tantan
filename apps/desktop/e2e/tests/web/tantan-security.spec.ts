import { randomUUID } from "node:crypto"
import { existsSync, readFileSync, unlinkSync } from "node:fs"

import { expect, test } from "@playwright/test"
import { resolve } from "pathe"

import { buildWebAppURL, resolveDesktopE2EEnv } from "../../support/env"

const securityCanary = () => {
  const path = process.env.TANTAN_SECURITY_CANARY_FILE
  return path ? readFileSync(path, "utf8").trim() : `AQ.TANTAN_BROWSER_${randomUUID()}`
}

test.describe("TASK-07 security", () => {
  test("Service Worker has an explicit authenticated API denylist", async () => {
    const source = readFileSync(
      resolve(resolveDesktopE2EEnv().desktopAppDir, "layer/renderer/src/workers/sw/index.ts"),
      "utf8",
    )
    expect(source).toContain('url.pathname.startsWith("/api/")')
    expect(source).toContain("denylist: [/^\\/api(?:\\/|$)/]")
    expect(source).toContain('request.destination === "image" && !isSensitiveRequest(url)')
    expect(source).not.toMatch(/NetworkFirst|StaleWhileRevalidate/)
  })

  test("release secret canary is absent from browser traffic, storage, URL and HAR", async ({
    browser,
  }, testInfo) => {
    const canary = securityCanary()
    const env = resolveDesktopE2EEnv()
    const harPath = testInfo.outputPath("tantan-security.har")
    const context = await browser.newContext({
      ignoreHTTPSErrors: true,
      recordHar: { content: "embed", mode: "full", path: harPath },
      serviceWorkers: "allow",
    })
    const page = await context.newPage()
    const browserTraffic: string[] = []
    page.on("request", (request) => {
      browserTraffic.push(
        request.url(),
        request.postData() ?? "",
        JSON.stringify(request.headers()),
      )
    })
    page.on("response", (response) => {
      browserTraffic.push(response.url(), JSON.stringify(response.headers()))
    })
    await page.route("**/api/**", (route) => {
      const path = new URL(route.request().url()).pathname
      const body =
        path === "/api/readyz"
          ? { ready: true, checks: { sqlite: "ok", migrations: "ok", keychain: "ok" } }
          : path === "/api/tantan/v1/session"
            ? {
                user: { id: "security-user", name: "Security User", email: null, image: null },
                timezone: "Asia/Shanghai",
                csrfToken: "csrf-security",
              }
            : path === "/api/tantan/v1/settings/ai-provider"
              ? {
                  configured: true,
                  providerId: "google-gemini-openai",
                  model: "gemini-3.5-flash-lite",
                  baseUrl: "https://generativelanguage.googleapis.com/v1beta/openai",
                  hasApiKey: true,
                  keyFingerprint: "A1B2C3D4",
                }
              : { code: 0, data: [], items: [], topics: [] }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(body),
      })
    })

    try {
      await page.goto(buildWebAppURL(env, "/settings/ai"), { waitUntil: "domcontentloaded" })
      await expect(page.getByText("gemini-3.5-flash-lite")).toBeVisible()
      await expect(page.locator("body")).not.toContainText(canary)
      const storageSnapshot = await page.evaluate(async () => {
        const cacheValues: string[] = []
        for (const name of await caches.keys()) {
          const cache = await caches.open(name)
          for (const request of await cache.keys()) {
            cacheValues.push(name, request.url)
            const response = await cache.match(request)
            if (response) cacheValues.push(await response.clone().text())
          }
        }
        const indexedDBValues: string[] = []
        for (const database of await indexedDB.databases()) {
          if (!database.name) continue
          indexedDBValues.push(database.name)
        }
        return JSON.stringify({
          url: globalThis.location.href,
          cookie: document.cookie,
          local: { ...localStorage },
          session: { ...sessionStorage },
          caches: cacheValues,
          indexedDB: indexedDBValues,
        })
      })
      expect(storageSnapshot).not.toContain(canary)
      expect(browserTraffic.join("\n")).not.toContain(canary)
      const forbiddenOrigins = browserTraffic.flatMap((value) => {
        try {
          const url = new URL(value)
          return ["https://api.folo.is", "https://generativelanguage.googleapis.com"].includes(
            url.origin,
          )
            ? [url.origin]
            : []
        } catch {
          return []
        }
      })
      expect(forbiddenOrigins).toEqual([])

      await context.close()
      expect(readFileSync(harPath, "utf8")).not.toContain(canary)
    } finally {
      await context.close().catch(() => {})
      if (existsSync(harPath)) unlinkSync(harPath)
    }
  })
})
