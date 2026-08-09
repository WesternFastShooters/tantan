import { defineConfig, devices } from "@playwright/test"

const previewURL = "http://127.0.0.1:4173"

const mobileUse = (browserName: "chromium" | "webkit", width: number, height: number) => ({
  ...(browserName === "webkit" ? devices["iPhone 13"] : devices["Pixel 7"]),
  browserName,
  viewport: { width, height },
  screen: { width, height },
  deviceScaleFactor: 2,
  hasTouch: true,
  isMobile: true,
  baseURL: previewURL,
  ignoreHTTPSErrors: true,
  serviceWorkers: "block" as const,
})

export default defineConfig({
  testDir: "./tests/web",
  testMatch: /tantan-production\.spec\.ts/,
  fullyParallel: false,
  workers: 1,
  timeout: 120_000,
  expect: { timeout: 15_000 },
  reporter: [["list"], ["html", { open: "never", outputFolder: "playwright-report/production" }]],
  outputDir: "test-results/production",
  webServer: {
    command:
      "pnpm exec cross-env WEB_BUILD=1 vite preview --host 127.0.0.1 --port 4173 --strictPort",
    cwd: new URL("..", import.meta.url).pathname,
    url: previewURL,
    timeout: 120_000,
    reuseExistingServer: false,
  },
  projects: [
    { name: "chromium-390x844", use: mobileUse("chromium", 390, 844) },
    { name: "chromium-430x932", use: mobileUse("chromium", 430, 932) },
    { name: "webkit-390x844", use: mobileUse("webkit", 390, 844) },
    { name: "webkit-430x932", use: mobileUse("webkit", 430, 932) },
  ],
})
