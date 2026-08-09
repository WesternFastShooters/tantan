import { readdirSync, readFileSync } from "node:fs"

import { extname, join, relative } from "pathe"
import { describe, expect, test } from "vitest"

import { isRemovedFoloRoute, removedFoloResponse } from "./removed-folo-routes"

const sourceRoot = join(process.cwd(), "src")
const repositoryRoot = join(process.cwd(), "../../../..")

const productionSources = (directory: string): string[] =>
  readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) return productionSources(path)
    if (![".ts", ".tsx"].includes(extname(entry.name))) return []
    if (/\.(?:test|spec)\.[cm]?[jt]sx?$/.test(entry.name)) return []
    return [path]
  })

const findMatches = (pattern: RegExp) =>
  productionSources(sourceRoot).flatMap((path) => {
    const source = readFileSync(path, "utf8")
    return pattern.test(source) ? [relative(sourceRoot, path)] : []
  })

const removedProductSourceRoots = [
  "modules/plan/",
  "modules/power/",
  "modules/wallet/",
  "modules/ai-chat/",
  "modules/ai-chat-session/",
  "modules/ai-onboarding/",
  "modules/ai-task/",
  "modules/app-layout/ai/",
  "modules/app-layout/ai-enhanced-timeline/",
  "modules/upgrade/",
  "queries/wallet.tsx",
]

describe("Tantan paid-feature policy", () => {
  test("REQ:FE-01 generated routes omit Folo paid/AI products while retaining local AI settings", () => {
    const generatedRoutes = readFileSync(join(sourceRoot, "generated-routes.ts"), "utf8")
    const localAISettingsRoute = readFileSync(
      join(sourceRoot, "pages/settings/(settings)/ai.tsx"),
      "utf8",
    )

    expect(generatedRoutes).not.toMatch(/pages\/settings\/\(settings\)\/plan/)
    expect(generatedRoutes).toMatch(/pages\/settings\/\(settings\)\/ai/)
    expect(localAISettingsRoute).toContain("tantan-settings/AISettingsPage")
    expect(generatedRoutes).not.toMatch(/pages\/\(main\).*\/(?:ai|power)\//)
    expect(generatedRoutes).not.toMatch(/pages\/.*\/(?:wallet|upgrade)\//)
  })

  test("REQ:FE-01 production source has no forbidden Folo AI or payment caller", () => {
    const forbiddenCallers = [
      /\bfollowApi\.ai(?:\.|\s*\()/,
      /\bfollowApi\.aiTask(?:\.|\s*\()/,
      /\bfollowApi\.aiChatSessions(?:\.|\s*\()/,
      /\bfollowClient\.api\.aiChatSessions(?:\.|\s*\()/,
      /\bfollowApi\.wallets(?:\.|\s*\()/,
      /\bfollowClient\.api\.trending(?:\.|\s*\()/,
      /\bsubscription\.upgrade\s*\(/,
      /VITE_API_URL[^\n]*\/ai\//,
      /["'`]\/ai\//,
      /["'`]\/wallets\//,
      /["'`]\/payments\//,
      /["'`]\/better-auth\/subscription(?:[/?"'`]|$)/,
    ]

    const violations = forbiddenCallers.flatMap(findMatches)
    expect([...new Set(violations)].sort()).toEqual([])
  })

  test("REQ:FE-01 production consumers do not import removed Folo product modules", () => {
    const removedProductImport =
      /["'`]~\/(?:modules\/(?:plan|power|wallet|ai-chat|ai-chat-session|ai-onboarding|ai-task|app-layout\/ai(?:-enhanced-timeline)?|upgrade)|queries\/wallet)[/"'`]/
    const violations = productionSources(sourceRoot).flatMap((path) => {
      const sourcePath = relative(sourceRoot, path)
      if (removedProductSourceRoots.some((root) => sourcePath.startsWith(root))) return []
      return removedProductImport.test(readFileSync(path, "utf8")) ? [sourcePath] : []
    })

    expect(violations.sort()).toEqual([])
  })

  test("REQ:FE-01 local bootstrap does not initialize telemetry, push or Folo AI sessions", () => {
    const initializeSource = readFileSync(join(sourceRoot, "initialize/index.ts"), "utf8")
    const mainSource = readFileSync(join(sourceRoot, "main.tsx"), "utf8")

    expect(initializeSource).not.toMatch(/(?:initAnalytics|hydrateSessionsFromLocalDb)/)
    expect(mainSource).not.toMatch(/(?:push-notification|registerWebPushNotifications)/)
  })

  test("REQ:FE-01 desktop auth client does not export Stripe subscription", () => {
    const authSource = readFileSync(join(sourceRoot, "lib/auth.ts"), "utf8")

    expect(authSource).not.toMatch(/^\s*subscription,?\s*$/m)
  })

  test("REQ:FE-01 shared auth client does not register the Stripe plugin", () => {
    const sharedAuthSource = readFileSync(
      join(repositoryRoot, "packages/internal/shared/src/auth.ts"),
      "utf8",
    )

    expect(sharedAuthSource).not.toContain("@better-auth/stripe")
    expect(sharedAuthSource).not.toMatch(/\bstripeClient\s*\(/)
  })

  test("REQ:FE-01 local client blocks only removed Folo product routes before fetch", async () => {
    const apiOrigin = "http://127.0.0.1:3000"
    const removed = [
      "/ai/chat",
      "/wallets",
      "/payments/checkout",
      "/better-auth/subscription/upgrade",
      "/better-auth/stripe/portal",
      "/referrals",
      "/trending",
      "/rsshub/use",
    ]
    const retained = [
      "/better-auth/get-session",
      "/subscriptions",
      "/entries",
      "/collections",
      "/discover/rsshub",
    ]

    for (const path of removed) {
      expect(isRemovedFoloRoute(new URL(path, apiOrigin), apiOrigin)).toBe(true)
    }
    for (const path of retained) {
      expect(isRemovedFoloRoute(new URL(path, apiOrigin), apiOrigin)).toBe(false)
    }
    expect(isRemovedFoloRoute(new URL("https://api.folo.is/ai/chat"), apiOrigin)).toBe(false)

    const response = removedFoloResponse()
    expect(response.status).toBe(410)
    expect(response.headers.get("Cache-Control")).toBe("no-store")
    await expect(response.json()).resolves.toMatchObject({
      error: { code: "FOLO_FEATURE_REMOVED" },
    })
  })
})
