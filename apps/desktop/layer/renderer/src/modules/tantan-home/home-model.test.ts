import { describe, expect, test } from "vitest"

import type { HomeCard, HomeResponse } from "~/lib/tantan-api/gen/types"

import { resolveCardPresentation } from "./card-presentation"
import {
  assertHomePageGeneration,
  dedupeHomeCards,
  homeColumnCount,
  nextHomePageParam,
} from "./home-model"

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

const page = (generation: string, nextCursor: string | null): HomeResponse => ({
  items: [card(`entry-${generation}`)],
  nextCursor,
  queue: {
    id: `queue-${generation}`,
    version: 1,
    generation,
    total: 1,
    unread: 1,
    finished: false,
    candidateWindowDays: 7,
    generatedAt: "2026-08-09T12:00:00Z",
  },
  queueGeneration: generation,
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

  test("FR-03 keeps the deployable Mobile Web feed at exactly two columns on every viewport", () => {
    expect(homeColumnCount(390)).toBe(2)
    expect(homeColumnCount(430)).toBe(2)
    expect(homeColumnCount(800)).toBe(2)
    expect(homeColumnCount(1024)).toBe(2)
    expect(homeColumnCount(1440)).toBe(2)
    expect(homeColumnCount(2560)).toBe(2)
  })

  test("XR-03 binds the next cursor to the response generation", () => {
    expect(nextHomePageParam(page("generation-1", "cursor-2"))).toEqual({
      cursor: "cursor-2",
      generation: "generation-1",
    })
    expect(nextHomePageParam(page("generation-1", null))).toBeUndefined()
  })

  test("TC-33 rejects a delayed page from another generation", () => {
    expect(assertHomePageGeneration(page("generation-1", null), "generation-1")).toEqual(
      page("generation-1", null),
    )
    expect(() => assertHomePageGeneration(page("generation-2", null), "generation-1")).toThrow(
      "HOME_QUEUE_GENERATION_CHANGED",
    )
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
