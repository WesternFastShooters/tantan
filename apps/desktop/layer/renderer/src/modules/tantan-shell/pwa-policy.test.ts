import { readFileSync } from "node:fs"

import { join } from "pathe"
import { describe, expect, test } from "vitest"

const rendererRoot = process.cwd()
const desktopRoot = join(rendererRoot, "../..")

describe("Tantan PWA policy", () => {
  test("REQ:FE-02 precaches the application shell", () => {
    const worker = readFileSync(join(rendererRoot, "src/workers/sw/index.ts"), "utf8")
    const vite = readFileSync(join(desktopRoot, "vite.config.ts"), "utf8")

    expect(worker).toMatch(/precacheAndRoute\s*\(\s*self\.__WB_MANIFEST\s*\)/)
    expect(vite).not.toMatch(/injectionPoint:\s*undefined/)
    expect(vite).toMatch(/name:\s*["']Tantan["']/)
  })

  test("REQ:FE-02 never runtime-caches Tantan API or auth responses", () => {
    const worker = readFileSync(join(rendererRoot, "src/workers/sw/index.ts"), "utf8")

    expect(worker).toMatch(/isSensitiveRequest/)
    expect(worker).toMatch(/url\.pathname\.startsWith\(["']\/api\//)
    expect(worker).toMatch(/denylist:\s*\[\/\^\\\/api/)
    expect(worker).toMatch(/!isSensitiveRequest\(url\)/)
  })

  test("REQ:FE-02 install metadata contains no Folo product or paid marketing", () => {
    const html = readFileSync(join(rendererRoot, "index.html"), "utf8")

    expect(html).not.toMatch(/Folo|POWER token|Upgrade|app\.folo\.is/)
    expect(html).toMatch(/<title>Tantan<\/title>/)
  })
})
