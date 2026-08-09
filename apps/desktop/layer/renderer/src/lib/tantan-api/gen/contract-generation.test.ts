import { createHash } from "node:crypto"
import { readFileSync } from "node:fs"

import { resolve } from "pathe"
import { describe, expect, it } from "vitest"

const contractInputs = [
  "spec-package/api/openapi.json",
  "spec-package/api/folo-route-policy.json",
  "spec-package/schemas/ai-enrichment-v1.schema.json",
  "spec-package/schemas/filter-spec-v1.schema.json",
  "spec-package/schemas/home-response.schema.json",
  "spec-package/schemas/topic-classification-v1.schema.json",
  "spec-package/db/0001_core.sql",
  "spec-package/db/0002_search_fts.sql",
  "spec-package/db/0003_seed_core_topics.sql",
  "spec-package/db/0004_mobile_web_v2.sql",
] as const

const repositoryRoot = resolve(process.cwd(), "../../../..")

const contractDigest = () => {
  const hash = createHash("sha256")
  for (const name of contractInputs) {
    hash.update(name)
    hash.update(Buffer.from([0]))
    hash.update(readFileSync(resolve(repositoryRoot, name)))
    hash.update(Buffer.from([0]))
  }
  return hash.digest("hex")
}

describe("CONTRACT:Tantan generated frontend API", () => {
  it("contains the approved DTOs and current contract digest", () => {
    const generated = readFileSync(
      resolve(process.cwd(), "src/lib/tantan-api/gen/types.ts"),
      "utf8",
    )

    expect(generated).toContain("export interface HomeResponse")
    expect(generated).toContain("export interface HomeCard")
    expect(generated).toContain("export interface TopicsResponse")
    expect(generated).toContain("export interface FilterMutationResponse")
    expect(generated).toContain("export interface AIProviderResponse")
    expect(generated).toContain("export interface ErrorResponse")
    expect(generated).toMatch(
      new RegExp(`export const CONTRACT_SHA256\\s*=\\s*"${contractDigest()}" as const`),
    )
  })
})
