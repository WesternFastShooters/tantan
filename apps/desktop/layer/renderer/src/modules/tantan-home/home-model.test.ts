import { describe, expect, test } from "vitest"

import type { HomeCard } from "~/lib/tantan-api/gen/types"

import { resolveCardPresentation } from "./card-presentation"
import { dedupeHomeCards, homeColumnCount } from "./home-model"

const card = (entryId: string, type: HomeCard["type"] = "article"): HomeCard => ({
  entryId,
  type,
  title: `Title ${entryId}`,
  excerpt: `Excerpt ${entryId}`,
  cover: `https://images.example/${entryId}.jpg`,
  source: { id: "source-1", name: "Source", avatar: null },
  publishedAt: "2026-08-09T12:00:00Z",
  topics: [{ id: "topic-ai", name: "AI" }],
  translated: false,
})

describe("Tantan Home model", () => {
  test("REQ:FE-03 keeps the first card when pages overlap", () => {
    const first = card("entry-1")
    const duplicate = { ...card("entry-1"), title: "Changed duplicate" }

    expect(dedupeHomeCards([first, card("entry-2"), duplicate, card("entry-3")])).toEqual([
      first,
      card("entry-2"),
      card("entry-3"),
    ])
  })

  test("REQ:FE-03 uses 2/3/4 columns at the approved viewport boundaries", () => {
    expect(homeColumnCount(390)).toBe(2)
    expect(homeColumnCount(800)).toBe(2)
    expect(homeColumnCount(1024)).toBe(3)
    expect(homeColumnCount(1439)).toBe(3)
    expect(homeColumnCount(1440)).toBe(4)
  })

  test.each([
    ["article", "4 / 3"],
    ["post", "4 / 3"],
    ["image", "1 / 1"],
    ["video", "16 / 9"],
  ] as const)("REQ:FE-03 resolves %s card fallback deterministically", (type, aspectRatio) => {
    const presentation = resolveCardPresentation(card(`entry-${type}`, type))
    expect(presentation.aspectRatio).toBe(aspectRatio)
    expect(presentation.cover).toContain(`entry-${type}`)
    expect(
      resolveCardPresentation({ ...card(`entry-${type}`, type), cover: null }).cover,
    ).toBeNull()
  })
})
