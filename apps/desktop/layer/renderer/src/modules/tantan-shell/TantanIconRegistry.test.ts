import { existsSync, readdirSync, readFileSync } from "node:fs"

import { dirname, join } from "pathe"
import { describe, expect, test } from "vitest"

const collectFiles = (directory: string): string[] =>
  readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) return collectFiles(path)
    return entry.isFile() && /\.[jt]sx?$/u.test(entry.name) ? [path] : []
  })

const findWorkspaceRoot = () => {
  let candidate = process.cwd()
  while (!existsSync(join(candidate, "icons", "mgc"))) {
    const parent = dirname(candidate)
    if (parent === candidate) throw new Error("Cannot locate the Folo icon registry")
    candidate = parent
  }
  return candidate
}

describe("Tantan icon registry", () => {
  test("CONTRACT:mobile-icon-glyphs every Tantan mgc token resolves to a bundled SVG", () => {
    const workspaceRoot = findWorkspaceRoot()
    const modulesRoot = join(
      workspaceRoot,
      "apps",
      "desktop",
      "layer",
      "renderer",
      "src",
      "modules",
    )
    const sourceFiles = readdirSync(modulesRoot, { withFileTypes: true })
      .filter((entry) => entry.isDirectory() && entry.name.startsWith("tantan-"))
      .flatMap((entry) => collectFiles(join(modulesRoot, entry.name)))
    const usedTokens = new Set(
      sourceFiles.flatMap((file) =>
        [...readFileSync(file, "utf8").matchAll(/\bi-mgc-[a-z0-9-]+\b/gu)].map(([token]) => token),
      ),
    )
    const availableTokens = new Set(
      readdirSync(join(workspaceRoot, "icons", "mgc"))
        .filter((file) => file.endsWith(".svg"))
        .map((file) => `i-mgc-${file.slice(0, -4).replaceAll("_", "-")}`),
    )

    const unresolved = [...usedTokens].filter((token) => !availableTokens.has(token)).sort()
    expect(unresolved, "Unresolved icon tokens render as blank controls in production").toEqual([])
  })
})
